// Package rag — service.go orchestrates the RAG pipeline: indexing (chunk →
// embed → upsert) and retrieval (embed query → HNSW search → cache → format
// context). It reads rag.* system_settings via the hot-reloadable
// config.Settings and caches retrieval results in Valkey (jc:rag:query:{hash}).
// // Ref: japaconnect-engineering/sections/69-rag-knowledge-layer.md
// //	japaconnect-engineering/adr/ADR-069-japabot-rag-knowledge-layer.md
package rag

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"sort"
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

// FeatureChecker resolves feature flag state from the feature_flags table —
// the single source of truth for feature reachability (rag.enabled).
type FeatureChecker interface {
	IsEnabled(ctx context.Context, key string) bool
}

// Service holds the business logic for the RAG knowledge layer.
type Service struct {
	store    *Store
	settings *config.Settings
	checker  FeatureChecker
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

// SetFeatureChecker injects the feature flag checker so the rag.enabled
// reachability check reads feature_flags instead of system_settings.
// Falls back to system_settings when nil.
func (s *Service) SetFeatureChecker(c FeatureChecker) { s.checker = c }

// ragEnabled resolves the rag.enabled feature flag; system_settings is
// consulted only as a fallback when no checker is wired.
func (s *Service) ragEnabled(ctx context.Context) bool {
	if s.checker != nil {
		return s.checker.IsEnabled(ctx, "rag.enabled")
	}

	if s.settings == nil {
		return true
	}

	return s.settings.GetBoolOr(ctx, "rag.enabled", true)
}

// SetEmbedder injects the embedding generator (AI gateway adapter).
func (s *Service) SetEmbedder(e EmbeddingGenerator) { s.embedder = e }

// EmbedderConfigured reports whether an embedding provider is wired.
// The public semantic-search endpoint uses this to answer 503 honestly —
// Retrieve returns nil, nil without an embedder and cannot be distinguished
// from a legitimately empty result set. The embedder wrapper is always
// non-nil once wired, so when it exposes a Configured check (ai.RAGEmbedder
// → dispatcher.HasCategory("embedding")) we defer to it.
func (s *Service) EmbedderConfigured() bool {
	if s.embedder == nil {
		return false
	}

	if c, ok := s.embedder.(interface{ Configured() bool }); ok {
		return c.Configured()
	}

	return true
}

// SetEnqueuer injects the Asynq task enqueuer for re-index jobs.
func (s *Service) SetEnqueuer(e TaskEnqueuer) { s.enqueuer = e }

// SetCache injects the Valkey cache for retrieval result caching.
func (s *Service) SetCache(c ValkeyCache) { s.cache = c }

// ragQueryCacheTTL is the TTL for jc:rag:query:{hash} (ADR-069: 10 min).
const ragQueryCacheTTL = 10 * time.Minute

// scopeForViewer builds the RetrievalScope for a viewer: public rows for
// everyone; authenticated viewers additionally see 'members' rows, their
// own rows, and private space/forum_thread rows for approved memberships.
// The rag.scoped_retrieval.enabled kill switch degrades to public-only.
func (s *Service) scopeForViewer(ctx context.Context, viewerID pgtype.UUID) RetrievalScope {
	scope := RetrievalScope{ViewerID: viewerID}

	if s.settings != nil && !s.settings.GetBoolOr(ctx, "rag.scoped_retrieval.enabled", true) {
		scope.ViewerID = pgtype.UUID{}
		return scope
	}

	if viewerID.Valid {
		ids, err := s.store.ListMemberSpaceIDs(ctx, viewerID)
		if err != nil {
			slog.Warn("rag: retrieve: list member spaces failed — degrading to non-member scope", "error", err)
		} else {
			scope.MemberSpaceIDs = ids
		}
	}

	return scope
}

// scopeHash digests a RetrievalScope for cache keys — scoped results must
// never cross viewers through cache. Folding memberships into the hash
// self-heals on membership changes (a new approved space produces a
// different key; no invalidation needed).
func scopeHash(scope RetrievalScope) []byte {
	h := sha256.New()

	if scope.ViewerID.Valid {
		h.Write(scope.ViewerID.Bytes[:])
	}

	if scope.CanSeePrivate {
		h.Write([]byte{1})
	}

	ids := make([]pgtype.UUID, len(scope.MemberSpaceIDs))
	copy(ids, scope.MemberSpaceIDs)
	sort.Slice(ids, func(i, j int) bool {
		return bytes.Compare(ids[i].Bytes[:], ids[j].Bytes[:]) < 0
	})

	for _, id := range ids {
		h.Write(id.Bytes[:])
	}

	return h.Sum(nil)
}

// ScopeHash returns the viewer's scope digest for callers that maintain
// their own scoped caches (e.g. jc:api_semantic:* in the public API).
func (s *Service) ScopeHash(ctx context.Context, viewerID pgtype.UUID) string {
	return hex.EncodeToString(scopeHash(s.scopeForViewer(ctx, viewerID)))
}

// ragQueryCacheKey builds the Valkey key for a cached retrieval result.
// The hash is over ( query_embedding, language, top_k, geo_space_id,
// viewer scope) so identical queries hit the cache and scoped results
// never cross viewers (§69 scoped-retrieval amendment).
func ragQueryCacheKey(queryEmbedding []float32, language string, topK int32, geoSpaceID *pgtype.UUID, scope RetrievalScope) string {
	h := sha256.New()
	h.Write(make([]byte, 16))

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

	h.Write(scopeHash(scope))

	return valkey.MustKey("rag:query:%s", hex.EncodeToString(h.Sum(nil)))
}

// IndexEntity chunks the source text, generates embeddings via the AI
// gateway, and upserts each chunk into content_embeddings with the
// entity's scope metadata (§69 amendment): visibility, spaceID (community
// space for space/thread entities), authorID. After upserting, stale tail
// chunks (from a previous, longer index) are deleted.
// // Scalar params rather than a shared struct: callers like internal/worker
// cannot import internal/rag (rag imports worker for task-type constants),
// so the caller-side interfaces declare plain parameters.
// // This is called by the extended content:generate_embedding task handler.
func (s *Service) IndexEntity(ctx context.Context, entityType string, entityID pgtype.UUID, visibility string, spaceID, authorID pgtype.UUID, body, language string) error {
	// Fail closed: an unset visibility would violate the CHECK constraint
	// anyway, and 'private' never leaks a row that should have been hidden.
	if visibility == "" {
		slog.Warn("rag: index: entity missing visibility — defaulting to private", "entity_type", entityType, "entity_id", entityID.String())

		visibility = "private"
	}

	ref := EntityRef{
		Type:       entityType,
		ID:         entityID,
		Visibility: visibility,
		SpaceID:    spaceID,
		AuthorID:   authorID,
	}

	if s.embedder == nil {
		slog.Warn("rag: index: no embedder configured, skipping", "entity_type", ref.Type, "entity_id", ref.ID.String())
		return nil
	}

	plainText := ExtractPlainText(body)
	if plainText == "" {
		// No text to index — purge any existing chunks for this entity.
		return s.store.DeleteEntityEmbeddings(ctx, ref.Type, ref.ID)
	}

	maxTokens := s.settings.GetIntOr(ctx, "rag.chunk_max_tokens", 400)
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
		return s.store.DeleteEntityEmbeddings(ctx, ref.Type, ref.ID)
	}

	model := s.settings.GetStringOr(ctx, "rag.embedding_model", "text-embedding-3-small")

	for i, chunkText := range chunkTexts {
		vec, err := s.embedder.GenerateEmbedding(ctx, chunkText, model)
		if err != nil {
			return fmt.Errorf("rag: index: embed chunk %d: %w", i, err)
		}

		if len(vec) == 0 {
			return fmt.Errorf("rag: index: empty embedding for chunk %d", i)
		}

		if err := s.store.UpsertChunk(ctx, ref, Chunk{
			Index:  i,
			Text:   chunkText,
			Vector: vec,
		}, language); err != nil {
			return fmt.Errorf("rag: index: upsert chunk %d: %w", i, err)
		}
	}

	// Delete stale tail chunks from a previous, longer index.
	//nolint:gosec // G115: bounded
	if err := s.store.DeleteStaleTail(ctx, ref.Type, ref.ID, int32(len(chunkTexts))); err != nil { // #nosec G115 -- len() of a chunked text slice // #nosec //nolint:gosec // G115: bounded
		slog.Warn("rag: index: failed to delete stale tail", "entity_id", ref.ID.String(), "error", err)
	}

	slog.Info("rag: index: complete", "entity_type", ref.Type, "entity_id", ref.ID.String(), "chunks", len(chunkTexts))

	return nil
}

// ReconcileSpaceDelete fail-closes forum_thread chunk rows whose parent
// space is being deleted. Threads survive the delete (space_id → NULL), so
// this must run BEFORE the space row is removed — after the FK fires the
// rows are unmatchable by space. Fail closed to private (§69 amendment).
func (s *Service) ReconcileSpaceDelete(ctx context.Context, spaceID pgtype.UUID) error {
	if s.store == nil {
		return nil
	}

	return s.store.OrphanThreadScopesBySpace(ctx, spaceID)
}

// ReconcileSpaceScope refreshes scope metadata when a space's visibility
// or status changes (§69 amendment): the space's own entity row plus every
// forum_thread row inside it (threads inherit space visibility).
func (s *Service) ReconcileSpaceScope(ctx context.Context, spaceID pgtype.UUID, visibility string) error {
	if err := s.store.UpdateEntityScope(ctx, EntityRef{
		Type:       EntityTypeSpace,
		ID:         spaceID,
		Visibility: visibility,
		SpaceID:    spaceID,
	}); err != nil {
		return fmt.Errorf("rag: reconcile space scope: %w", err)
	}

	if err := s.store.UpdateThreadScopesBySpace(ctx, spaceID, visibility); err != nil {
		return fmt.Errorf("rag: reconcile thread scopes: %w", err)
	}

	return nil
}

// PurgeEntity deletes all chunks for an entity. Used on unpublish/delete.
func (s *Service) PurgeEntity(ctx context.Context, entityType string, entityID pgtype.UUID) error {
	return s.store.DeleteEntityEmbeddings(ctx, entityType, entityID)
}

// PurgeEntityType deletes all chunks for every entity of the given type
// within a platform. Used by the admin DELETE /admin/rag/entity/:type/all
// route (the by-type aggregate purge from the dashboard).
func (s *Service) PurgeEntityType(ctx context.Context, entityType string) error {
	return s.store.DeleteEmbeddingsByType(ctx, entityType)
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
func (s *Service) Retrieve(ctx context.Context, query, language string, geoSpaceID *pgtype.UUID, viewerID pgtype.UUID) ([]RetrievedChunk, error) {
	if !s.ragEnabled(ctx) {
		return nil, nil
	}

	if s.embedder == nil {
		slog.Warn("rag: retrieve: no embedder configured")
		return nil, nil
	}

	if query == "" {
		return nil, nil
	}

	model := s.settings.GetStringOr(ctx, "rag.embedding_model", "text-embedding-3-small")
	topK := int32(s.settings.GetIntOr(ctx, "rag.retrieval_top_k", 5)) // #nosec G115 -- bounded by settings validation (1-50)
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

	// Build the viewer's retrieval scope (§69 scoped-retrieval amendment).
	scope := s.scopeForViewer(ctx, viewerID)

	// Check cache — the key folds in the scope hash so scoped results
	// never cross viewers.
	cacheKey := ragQueryCacheKey(queryVec, language, topK, geoSpaceID, scope)
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
		results, err = s.store.SearchGeoFiltered(ctx, queryVec, language, topK, *geoSpaceID, scope)
	} else {
		results, err = s.store.Search(ctx, queryVec, language, topK, scope)
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
func (s *Service) GetStats(ctx context.Context) (IndexStats, error) {
	return s.store.GetStats(ctx)
}

// RecordIndexFailure records an exhausted index/purge retry for admin
// visibility. Nil-safe: returns nil when the store is not wired. Callers
// invoke this from fire-and-forget goroutines; the error return is only for
// logging (Rule 8 — never block the HTTP response).
func (s *Service) RecordIndexFailure(ctx context.Context, entityType string, entityID pgtype.UUID, operation string, indexErr error) error {
	if s == nil || s.store == nil {
		return nil
	}

	return s.store.RecordIndexFailure(ctx, entityType, entityID, operation, indexErr)
}

// RetryResult summarizes a POST /admin/rag/retry-failures pass.
type RetryResult struct {
	Requeued          int      `json:"requeued"`
	ClearedNonContent int64    `json:"cleared_non_content"`
	Note              string   `json:"note,omitempty"`
	RequeueErrors     []string `json:"requeue_errors,omitempty"`
}

// RetryFailures re-enqueues content:generate_embedding tasks for content-type
// index failures via the existing enqueuer (same payload shape as
// QueueReindex) and clears non-content (wiki_page/space/...) rows
// best-effort — those entity types have no generic re-indexer (out of
// scope). Each successfully re-enqueued content ID is deleted; a per-ID
// delete keeps the rows whose enqueue failed visible for the next pass.
func (s *Service) RetryFailures(ctx context.Context) (RetryResult, error) {
	var res RetryResult
	if s.store == nil {
		return res, errors.New("rag: retry-failures: no store configured")
	}

	if s.enqueuer == nil {
		return res, errors.New("rag: retry-failures: no enqueuer configured")
	}

	ids, err := s.store.ListContentIndexFailureIDs(ctx)
	if err != nil {
		return res, err
	}

	for _, id := range ids {
		payload, err := json.Marshal(map[string]string{
			"content_id": id.String(),
		})
		if err != nil {
			slog.Warn("rag: retry-failures: marshal payload failed", "content_id", id.String(), "error", err)
			res.RequeueErrors = append(res.RequeueErrors, id.String())

			continue
		}

		if err := s.enqueuer.Enqueue(worker.TypeAIGenerateContentEmbedding, payload); err != nil {
			slog.Warn("rag: retry-failures: enqueue failed", "content_id", id.String(), "error", err)
			res.RequeueErrors = append(res.RequeueErrors, id.String())

			continue
		}

		if err := s.store.ClearIndexFailure(ctx, EntityTypeContent, id, "index"); err != nil {
			slog.Warn("rag: retry-failures: clear after requeue failed", "content_id", id.String(), "error", err)
			continue
		}

		res.Requeued++
	}

	cleared, err := s.store.ClearNonContentIndexFailures(ctx)
	if err != nil {
		return res, err
	}

	res.ClearedNonContent = cleared

	if cleared > 0 {
		res.Note = "non-content failures (wiki_page, space, ...) were cleared without re-index: no generic entity re-indexer exists"
	}

	return res, nil
}

// QueueReindex enqueues a full corpus re-index by enumerating all published
// content IDs and enqueueing the content:generate_embedding task for each.
// The admin POST /admin/rag/reindex route calls this. Fire-and-forget per
// Rule 8 — the handler returns 202 immediately.
func (s *Service) QueueReindex(ctx context.Context) error {
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
		ids, err := s.store.ListPublishedContentIDs(ctx, batchSize, offset)
		if err != nil {
			return fmt.Errorf("rag: reindex: list batch at offset %d: %w", offset, err)
		}

		if len(ids) == 0 {
			break
		}

		for _, ref := range ids {
			payload, err := json.Marshal(map[string]string{
				"content_id": ref.ID.String(),
			})
			if err != nil {
				slog.Warn("rag: reindex: marshal payload failed", "content_id", ref.ID.String(), "error", err)
				continue
			}

			if err := s.enqueuer.Enqueue(worker.TypeAIGenerateContentEmbedding, payload); err != nil {
				slog.Warn("rag: reindex: enqueue failed", "content_id", ref.ID.String(), "error", err)
				continue
			}

			totalEnqueued++
		}

		offset += batchSize
	}

	slog.Info("rag: reindex: queued", "total_enqueued", totalEnqueued)

	// BUG-001 fix: also re-index forum_threads, spaces, and academy_tracks
	// (ADR-069 §2 corpus). These entity types are indexed inline (not via the
	// content:generate_embedding worker task), so we enumerate their IDs and
	// fire-and-forget IndexEntity calls in background goroutines. The HTTP
	// response returns immediately after enumeration; the actual embedding
	// work happens asynchronously.
	go s.reindexForumThreads()  // #nosec G118 -- fire-and-forget: request ctx is cancelled on HTTP return (Rule 8)
	go s.reindexSpaces()        // #nosec G118 -- fire-and-forget: request ctx is cancelled on HTTP return (Rule 8)
	go s.reindexAcademyTracks() // #nosec G118 -- fire-and-forget: request ctx is cancelled on HTTP return (Rule 8)

	return nil
}

// reindexForumThreads enumerates all non-deleted forum threads for the platform
// and re-indexes them into content_embeddings. Fire-and-forget — called in a
// goroutine from QueueReindex.
func (s *Service) reindexForumThreads() {
	ctx := context.Background()

	// §69 amendment: self-heal any thread rows orphaned since the last
	// sweep (e.g. a ReconcileSpaceDelete miss) before re-indexing.
	if err := s.store.ReconcileOrphanThreadScopes(ctx); err != nil {
		slog.Warn("rag: reindex forum threads: orphan reconcile failed", "error", err)
	}

	language := "en"

	if s.settings != nil {
		language = s.settings.GetStringOr(ctx, "i18n.default_language", "en")
	}

	const batchSize int32 = 100

	var offset int32 = 0

	total := 0

	for {
		ids, err := s.store.ListForumThreadIDs(ctx, batchSize, offset)
		if err != nil {
			slog.Warn("rag: reindex forum threads: list batch failed", "offset", offset, "error", err)
			return
		}

		if len(ids) == 0 {
			break
		}

		for _, ref := range ids {
			// Fetch thread title + body from DB for embedding.
			thread, err := s.fetchForumThreadText(ctx, ref.ID)
			if err != nil {
				slog.Warn("rag: reindex forum threads: fetch failed", "thread_id", ref.ID.String(), "error", err)
				continue
			}

			if thread == "" {
				continue
			}

			if err := s.IndexEntity(ctx, EntityTypeForumThread, ref.ID,
				ref.Visibility, ref.SpaceID, ref.AuthorID, thread, language); err != nil {
				slog.Warn("rag: reindex forum threads: index failed", "thread_id", ref.ID.String(), "error", err)
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

// reindexSpaces enumerates all active spaces for the platform and re-indexes
// them into content_embeddings. Fire-and-forget — called in a goroutine from
// QueueReindex.
func (s *Service) reindexSpaces() {
	ctx := context.Background()
	language := "en"

	if s.settings != nil {
		language = s.settings.GetStringOr(ctx, "i18n.default_language", "en")
	}

	const batchSize int32 = 100

	var offset int32 = 0

	total := 0

	for {
		ids, err := s.store.ListSpaceIDs(ctx, batchSize, offset)
		if err != nil {
			slog.Warn("rag: reindex spaces: list batch failed", "offset", offset, "error", err)
			return
		}

		if len(ids) == 0 {
			break
		}

		for _, ref := range ids {
			text, err := s.fetchSpaceText(ctx, ref.ID)
			if err != nil {
				slog.Warn("rag: reindex spaces: fetch failed", "space_id", ref.ID.String(), "error", err)
				continue
			}

			if text == "" {
				continue
			}

			if err := s.IndexEntity(ctx, EntityTypeSpace, ref.ID,
				ref.Visibility, ref.ID, pgtype.UUID{}, text, language); err != nil {
				slog.Warn("rag: reindex spaces: index failed", "space_id", ref.ID.String(), "error", err)
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

// reindexAcademyTracks enumerates all published learning tracks for the
// platform and re-indexes them into content_embeddings. Fire-and-forget —
// called in a goroutine from QueueReindex.
func (s *Service) reindexAcademyTracks() {
	ctx := context.Background()
	language := "en"

	if s.settings != nil {
		language = s.settings.GetStringOr(ctx, "i18n.default_language", "en")
	}

	const batchSize int32 = 100

	var offset int32 = 0

	total := 0

	for {
		ids, err := s.store.ListAcademyTrackIDs(ctx, batchSize, offset)
		if err != nil {
			slog.Warn("rag: reindex learning tracks: list batch failed", "offset", offset, "error", err)
			return
		}

		if len(ids) == 0 {
			break
		}

		for _, id := range ids {
			text, err := s.fetchAcademyTrackText(ctx, id)
			if err != nil {
				slog.Warn("rag: reindex learning tracks: fetch failed", "track_id", id.String(), "error", err)
				continue
			}

			if text == "" {
				continue
			}

			if err := s.IndexEntity(ctx, EntityTypeAcademyTrack, id,
				"public", pgtype.UUID{}, pgtype.UUID{}, text, language); err != nil {
				slog.Warn("rag: reindex learning tracks: index failed", "track_id", id.String(), "error", err)
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
func (s *Service) fetchForumThreadText(ctx context.Context, threadID pgtype.UUID) (string, error) {
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
func (s *Service) fetchSpaceText(ctx context.Context, spaceID pgtype.UUID) (string, error) {
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

// fetchAcademyTrackText loads a learning track's title + description from
// the database for RAG embedding. Returns empty string if the track has no text.
func (s *Service) fetchAcademyTrackText(ctx context.Context, trackID pgtype.UUID) (string, error) {
	q := gen.New(s.store.pool.Reader)

	row, err := q.GetAcademyTrackByID(ctx,
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
