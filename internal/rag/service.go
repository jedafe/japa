// Package rag — service.go orchestrates the RAG pipeline: indexing (chunk →
// embed → upsert) and retrieval (embed query → HNSW search → cache → format
// context). It reads rag.* system_settings via the hot-reloadable
// config.Settings and caches retrieval results in Valkey (jc:rag:query:{hash}).
// // Ref: japaconnect-engineering/sections/69-rag-knowledge-layer.md
// //	japaconnect-engineering/adr/ADR-069-japabot-rag-knowledge-layer.md
package rag

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/config"
	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/valkey"
	"github.com/jedafe/japa/internal/worker"
)

// EmbeddingGenerator is the interface for generating vector embeddings via
// the AI gateway. The ai.ContentEmbedder / ai.GenerateContentEmbedding
// function satisfies this.
type EmbeddingGenerator interface {
	GenerateEmbedding(ctx context.Context, text, model string) ([]float32, error)
}

// TaskEnqueuer is the minimal interface for enqueueing Asynq tasks.
type TaskEnqueuer interface {
	Enqueue(taskType string, payload []byte, opts ...interface{}) error
}

// Service holds the business logic for the RAG knowledge layer.
type Service struct {
	store    *Store
	settings *config.Settings
	embedder EmbeddingGenerator
	enqueuer TaskEnqueuer
	cache    ValkeyCache // nil = caching disabled
}

// NewService creates a RAG service.
func NewService(store *Store, settings *config.Settings) *Service {
	return &Service{
		store:    store,
		settings: settings,
	}
}

// SetEmbedder injects the embedding generator (AI gateway adapter).
func (s *Service) SetEmbedder(e EmbeddingGenerator) { s.embedder = e }

// SetEnqueuer injects the Asynq task enqueuer for re-index jobs.
func (s *Service) SetEnqueuer(e TaskEnqueuer) { s.enqueuer = e }

// SetCache injects the Valkey cache for retrieval result caching.
func (s *Service) SetCache(c ValkeyCache) { s.cache = c }

// ragQueryCacheTTL is the TTL for jc:rag:query:{hash} (ADR-069: 10 min).
const ragQueryCacheTTL = 10 * time.Minute

// ragQueryCacheKey builds the Valkey key for a cached retrieval result.
// The hash is over (, query_embedding, language, top_k, geo_space_id)
// so identical queries hit the cache. geo_space_id is included so a
// geo-filtered retrieval and an unfiltered retrieval never share a cache
// entry (§103 Geo-Filtered RAG amendment to §69).
func ragQueryCacheKey(tenantID pgtype.UUID, queryEmbedding []float32, language string, topK int32, geoSpaceID *pgtype.UUID) string {
	h := sha256.New()
	h.Write(tenantID.Bytes[:])

	for _, f := range queryEmbedding {
		bits := math.Float32bits(f)

		var buf [4]byte
		buf[0] = byte(bits)       // #nosec G115 -- bit extraction, no overflow
		buf[1] = byte(bits >> 8)  // #nosec G115 -- bit extraction, no overflow
		buf[2] = byte(bits >> 16) // #nosec G115 -- bit extraction, no overflow
		buf[3] = byte(bits >> 24) // #nosec G115 -- bit extraction, no overflow
		h.Write(buf[:])
	}

	h.Write([]byte(language))
	h.Write([]byte{byte(topK), byte(topK >> 8), byte(topK >> 16), byte(topK >> 24)}) // #nosec G115 -- bit extraction, no overflow

	if geoSpaceID != nil && geoSpaceID.Valid {
		h.Write(geoSpaceID.Bytes[:])
	} else {
		h.Write([]byte{0})
	}

	return valkey.MustKey("rag:query:%s", hex.EncodeToString(h.Sum(nil)))
}

// IndexEntity chunks the source text, generates embeddings via the AI
// gateway, and upserts each chunk into content_embeddings. After upserting,
// stale tail chunks (from a previous, longer index) are deleted.
// // This is called by the extended content:generate_embedding task handler.
func (s *Service) IndexEntity(ctx context.Context, tenantID pgtype.UUID, entityType string, entityID pgtype.UUID, body, language string) error {
	if s.embedder == nil {
		slog.Warn("rag: index: no embedder configured, skipping", "entity_type", entityType, "entity_id", entityID.String())
		return nil
	}

	plainText := ExtractPlainText(body)
	if plainText == "" {
		// No text to index — purge any existing chunks for this entity.
		return s.store.DeleteEntityEmbeddings(ctx, tenantID, entityType, entityID)
	}

	maxTokens := s.settings.GetIntOr(ctx, pgtype.UUID{}, "rag.chunk_max_tokens", 400)
	// Defense-in-depth: clamp to [1, 8000] even if catalog validation is
	// bypassed. 0 or negative causes ChunkText to return the whole text as
	// one giant chunk (may exceed the embedding model's token limit).
	if maxTokens < 1 {
		maxTokens = 1
	} else if maxTokens > 8000 {
		maxTokens = 8000
	}

	chunkTexts := ChunkText(plainText, maxTokens, maxTokens/5)
	if len(chunkTexts) == 0 {
		return s.store.DeleteEntityEmbeddings(ctx, tenantID, entityType, entityID)
	}

	model := s.settings.GetStringOr(ctx, pgtype.UUID{}, "rag.embedding_model", "text-embedding-3-small")

	for i, chunkText := range chunkTexts {
		vec, err := s.embedder.GenerateEmbedding(ctx, chunkText, model)
		if err != nil {
			return fmt.Errorf("rag: index: embed chunk %d: %w", i, err)
		}

		if len(vec) == 0 {
			return fmt.Errorf("rag: index: empty embedding for chunk %d", i)
		}

		if err := s.store.UpsertChunk(ctx, tenantID, entityType, entityID, Chunk{
			Index:  i,
			Text:   chunkText,
			Vector: vec,
		}, language); err != nil {
			return fmt.Errorf("rag: index: upsert chunk %d: %w", i, err)
		}
	}

	// Delete stale tail chunks from a previous, longer index.
	// nolint:gosec // G115: bounded
	if err := s.store.DeleteStaleTail(ctx, tenantID, entityType, entityID, int32(len(chunkTexts))); err != nil { // #nosec G115 -- len() of a chunked text slice // #nosec //nolint:gosec // G115: bounded
		slog.Warn("rag: index: failed to delete stale tail", "entity_id", entityID.String(), "error", err)
	}

	slog.Info("rag: index: complete", "entity_type", entityType, "entity_id", entityID.String(), "chunks", len(chunkTexts))

	return nil
}

// PurgeEntity deletes all chunks for an entity. Used on unpublish/delete.
func (s *Service) PurgeEntity(ctx context.Context, tenantID pgtype.UUID, entityType string, entityID pgtype.UUID) error {
	return s.store.DeleteEntityEmbeddings(ctx, tenantID, entityType, entityID)
}

// PurgeEntityType deletes all chunks for every entity of the given type
// within a tenant. Used by the admin DELETE /admin/rag/entity/:type/all
// route (the by-type aggregate purge from the dashboard).
func (s *Service) PurgeEntityType(ctx context.Context, tenantID pgtype.UUID, entityType string) error {
	return s.store.DeleteEmbeddingsByType(ctx, tenantID, entityType)
}

// Retrieve performs the RAG retrieval step: embed the query, search HNSW,
// and return formatted context passages. Results are cached in Valkey
// (jc:rag:query:{hash}, 10-min TTL) for identical query embeddings.
// // geoSpaceID optionally restricts retrieval to content chunks whose source
// contents row is geo-scoped to the given space or its direct children
// (§103 Geo-Filtered RAG amendment to §69). When geoSpaceID is nil or
// invalid, retrieval is unfiltered (backward-compatible with §69).
// // If rag.enabled is false, returns nil (the chat handler falls back to
// non-RAG LLM call).
func (s *Service) Retrieve(ctx context.Context, tenantID pgtype.UUID, query, language string, geoSpaceID *pgtype.UUID) ([]RetrievedChunk, error) {
	if s.settings != nil {
		if enabled := s.settings.GetBoolOr(ctx, pgtype.UUID{}, "rag.enabled", true); !enabled {
			return nil, nil
		}
	}

	if s.embedder == nil {
		slog.Warn("rag: retrieve: no embedder configured")
		return nil, nil
	}

	if query == "" {
		return nil, nil
	}

	model := s.settings.GetStringOr(ctx, pgtype.UUID{}, "rag.embedding_model", "text-embedding-3-small")
	topK := int32(s.settings.GetIntOr(ctx, pgtype.UUID{}, "rag.retrieval_top_k", 5)) // #nosec G115 -- bounded by settings validation (1-50)
	// Defense-in-depth: clamp topK to [1, 50] even if the catalog validation
	// is bypassed (e.g. direct DB write). LIMIT 0 in PostgreSQL returns no
	// rows, silently breaking RAG; LIMIT -1 returns all rows (unbounded).
	if topK < 1 {
		topK = 1
	} else if topK > 50 {
		topK = 50
	}

	queryVec, err := s.embedder.GenerateEmbedding(ctx, query, model)
	if err != nil {
		return nil, fmt.Errorf("rag: retrieve: embed query: %w", err)
	}

	if len(queryVec) == 0 {
		return nil, errors.New("rag: retrieve: empty query embedding")
	}

	geoActive := geoSpaceID != nil && geoSpaceID.Valid

	// Check cache.
	cacheKey := ragQueryCacheKey(tenantID, queryVec, language, topK, geoSpaceID)
	if s.cache != nil {
		if cached, err := s.cache.Get(ctx, cacheKey).Result(); err == nil && cached != "" {
			var results []RetrievedChunk
			if err := json.Unmarshal([]byte(cached), &results); err == nil {
				return results, nil
			}
		}
	}

	var results []RetrievedChunk
	if geoActive {
		results, err = s.store.SearchGeoFiltered(ctx, tenantID, queryVec, language, topK, *geoSpaceID)
	} else {
		results, err = s.store.Search(ctx, tenantID, queryVec, language, topK)
	}

	if err != nil {
		return nil, err
	}

	// Cache the results.
	if s.cache != nil && len(results) > 0 {
		if data, err := json.Marshal(results); err == nil {
			if err := s.cache.Set(ctx, cacheKey, data, ragQueryCacheTTL).Err(); err != nil {
				slog.Warn("rag: retrieve: cache set failed", "error", err)
			}
		}
	}

	return results, nil
}

// FormatContext formats retrieved chunks into the sourced-context string
// that is injected into the LLM system prompt. Each passage is labeled with
// its source tier (editorial vs community) per ADR-069 §4.
func (s *Service) FormatContext(chunks []RetrievedChunk) string {
	if len(chunks) == 0 {
		return ""
	}

	var sb strings.Builder

	sb.WriteString("The following passages are sourced from JapaConnect's published content. ")
	sb.WriteString("Cite the source when using these passages. ")
	sb.WriteString("Qualify claims from community-sourced content (discussions, forum threads) ")
	sb.WriteString("differently from editorial content (guides, blog posts).\n\n")

	for i, chunk := range chunks {
		sb.WriteString(fmt.Sprintf("[Source %d: %s (%s tier, similarity %.2f)]\n%s\n\n",
			i+1, chunk.EntityType, chunk.Tier, chunk.Similarity, chunk.ChunkText))
	}

	return sb.String()
}

// GetStats returns the index statistics for the admin status route.
func (s *Service) GetStats(ctx context.Context, tenantID pgtype.UUID) (IndexStats, error) {
	return s.store.GetStats(ctx, tenantID)
}

// QueueReindex enqueues a full corpus re-index by enumerating all published
// content IDs and enqueueing the content:generate_embedding task for each.
// The admin POST /admin/rag/reindex route calls this. Fire-and-forget per
// Rule 8 — the handler returns 202 immediately.
func (s *Service) QueueReindex(ctx context.Context, tenantID pgtype.UUID) error {
	if s.enqueuer == nil {
		return errors.New("rag: reindex: no enqueuer configured")
	}

	// Enumerate published content in batches of 100 to avoid loading all
	// IDs into memory at once. Each batch enqueues embedding tasks
	// fire-and-forget.
	const batchSize int32 = 100

	var offset int32 = 0

	totalEnqueued := 0

	for {
		ids, err := s.store.ListPublishedContentIDs(ctx, tenantID, batchSize, offset)
		if err != nil {
			return fmt.Errorf("rag: reindex: list batch at offset %d: %w", offset, err)
		}

		if len(ids) == 0 {
			break
		}

		for _, id := range ids {
			payload, err := json.Marshal(map[string]string{
				"content_id": id.ID.String(),
				"tenant_id":  id.TenantID.String(),
			})
			if err != nil {
				slog.Warn("rag: reindex: marshal payload failed", "content_id", id.ID.String(), "error", err)
				continue
			}

			if err := s.enqueuer.Enqueue(worker.TypeAIGenerateContentEmbedding, payload); err != nil {
				slog.Warn("rag: reindex: enqueue failed", "content_id", id.ID.String(), "error", err)
				continue
			}

			totalEnqueued++
		}

		offset += batchSize
	}

	slog.Info("rag: reindex: queued", "total_enqueued", totalEnqueued)

	// BUG-001 fix: also re-index forum_threads, spaces, and learning_tracks
	// (ADR-069 §2 corpus). These entity types are indexed inline (not via the
	// content:generate_embedding worker task), so we enumerate their IDs and
	// fire-and-forget IndexEntity calls in background goroutines. The HTTP
	// response returns immediately after enumeration; the actual embedding
	// work happens asynchronously.
	go s.reindexForumThreads(tenantID)   // #nosec G118 -- fire-and-forget: request ctx is cancelled on HTTP return (Rule 8)
	go s.reindexSpaces(tenantID)         // #nosec G118 -- fire-and-forget: request ctx is cancelled on HTTP return (Rule 8)
	go s.reindexLearningTracks(tenantID) // #nosec G118 -- fire-and-forget: request ctx is cancelled on HTTP return (Rule 8)

	return nil
}

// reindexForumThreads enumerates all non-deleted forum threads for the tenant
// and re-indexes them into content_embeddings. Fire-and-forget — called in a
// goroutine from QueueReindex.
func (s *Service) reindexForumThreads(tenantID pgtype.UUID) {
	ctx := context.Background()
	language := "en"

	if s.settings != nil {
		language = s.settings.GetStringOr(ctx, pgtype.UUID{}, "i18n.default_language", "en")
	}

	const batchSize int32 = 100

	var offset int32 = 0

	total := 0

	for {
		ids, err := s.store.ListForumThreadIDs(ctx, tenantID, batchSize, offset)
		if err != nil {
			slog.Warn("rag: reindex forum threads: list batch failed", "offset", offset, "error", err)
			return
		}

		if len(ids) == 0 {
			break
		}

		for _, id := range ids {
			// Fetch thread title + body from DB for embedding.
			thread, err := s.fetchForumThreadText(ctx, tenantID, id.ID)
			if err != nil {
				slog.Warn("rag: reindex forum threads: fetch failed", "thread_id", id.ID.String(), "error", err)
				continue
			}

			if thread == "" {
				continue
			}

			if err := s.IndexEntity(ctx, tenantID, "forum_thread", id.ID, thread, language); err != nil {
				slog.Warn("rag: reindex forum threads: index failed", "thread_id", id.ID.String(), "error", err)
				continue
			}

			total++
		}

		offset += batchSize
	}

	if total > 0 {
		slog.Info("rag: reindex forum threads: done", "total_indexed", total)
	}
}

// reindexSpaces enumerates all active spaces for the tenant and re-indexes
// them into content_embeddings. Fire-and-forget — called in a goroutine from
// QueueReindex.
func (s *Service) reindexSpaces(tenantID pgtype.UUID) {
	ctx := context.Background()
	language := "en"

	if s.settings != nil {
		language = s.settings.GetStringOr(ctx, pgtype.UUID{}, "i18n.default_language", "en")
	}

	const batchSize int32 = 100

	var offset int32 = 0

	total := 0

	for {
		ids, err := s.store.ListSpaceIDs(ctx, tenantID, batchSize, offset)
		if err != nil {
			slog.Warn("rag: reindex spaces: list batch failed", "offset", offset, "error", err)
			return
		}

		if len(ids) == 0 {
			break
		}

		for _, id := range ids {
			text, err := s.fetchSpaceText(ctx, tenantID, id.ID)
			if err != nil {
				slog.Warn("rag: reindex spaces: fetch failed", "space_id", id.ID.String(), "error", err)
				continue
			}

			if text == "" {
				continue
			}

			if err := s.IndexEntity(ctx, tenantID, "space", id.ID, text, language); err != nil {
				slog.Warn("rag: reindex spaces: index failed", "space_id", id.ID.String(), "error", err)
				continue
			}

			total++
		}

		offset += batchSize
	}

	if total > 0 {
		slog.Info("rag: reindex spaces: done", "total_indexed", total)
	}
}

// reindexLearningTracks enumerates all published learning tracks for the
// tenant and re-indexes them into content_embeddings. Fire-and-forget —
// called in a goroutine from QueueReindex.
func (s *Service) reindexLearningTracks(tenantID pgtype.UUID) {
	ctx := context.Background()
	language := "en"

	if s.settings != nil {
		language = s.settings.GetStringOr(ctx, pgtype.UUID{}, "i18n.default_language", "en")
	}

	const batchSize int32 = 100

	var offset int32 = 0

	total := 0

	for {
		ids, err := s.store.ListLearningTrackIDs(ctx, tenantID, batchSize, offset)
		if err != nil {
			slog.Warn("rag: reindex learning tracks: list batch failed", "offset", offset, "error", err)
			return
		}

		if len(ids) == 0 {
			break
		}

		for _, id := range ids {
			text, err := s.fetchLearningTrackText(ctx, tenantID, id.ID)
			if err != nil {
				slog.Warn("rag: reindex learning tracks: fetch failed", "track_id", id.ID.String(), "error", err)
				continue
			}

			if text == "" {
				continue
			}

			if err := s.IndexEntity(ctx, tenantID, "learning_track", id.ID, text, language); err != nil {
				slog.Warn("rag: reindex learning tracks: index failed", "track_id", id.ID.String(), "error", err)
				continue
			}

			total++
		}

		offset += batchSize
	}

	if total > 0 {
		slog.Info("rag: reindex learning tracks: done", "total_indexed", total)
	}
}

// fetchForumThreadText loads a forum thread's title + body from the database
// for RAG embedding. Returns empty string if the thread has no text.
func (s *Service) fetchForumThreadText(ctx context.Context, tenantID, threadID pgtype.UUID) (string, error) {
	q := gen.New(s.store.pool.Reader)

	row, err := q.GetForumThreadByID(ctx,
		threadID,
	)
	if err != nil {
		return "", err
	}

	text := row.Title
	if len(row.Body) > 0 {
		if text != "" {
			text += "\n\n"
		}

		text += string(row.Body)
	}

	return text, nil
}

// fetchSpaceText loads a space's name + description + wiki_content from the
// database for RAG embedding. Returns empty string if the space has no text.
func (s *Service) fetchSpaceText(ctx context.Context, tenantID, spaceID pgtype.UUID) (string, error) {
	q := gen.New(s.store.pool.Reader)

	row, err := q.GetSpaceByID(ctx,
		spaceID,
	)
	if err != nil {
		return "", err
	}

	text := row.Name
	if row.Description != nil && *row.Description != "" {
		if text != "" {
			text += "\n\n"
		}

		text += *row.Description
	}

	if len(row.WikiContent) > 0 {
		if text != "" {
			text += "\n\n"
		}

		text += string(row.WikiContent)
	}

	return text, nil
}

// fetchLearningTrackText loads a learning track's title + description from
// the database for RAG embedding. Returns empty string if the track has no text.
func (s *Service) fetchLearningTrackText(ctx context.Context, tenantID, trackID pgtype.UUID) (string, error) {
	q := gen.New(s.store.pool.Reader)

	row, err := q.GetLearningTrackByID(ctx,
		trackID,
	)
	if err != nil {
		return "", err
	}

	text := row.Title
	if row.Description != nil && *row.Description != "" {
		if text != "" {
			text += "\n\n"
		}

		text += *row.Description
	}

	return text, nil
}
