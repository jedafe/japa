// Package rag — service.go orchestrates the RAG pipeline: indexing (chunk →
// embed → upsert) and retrieval (embed query → HNSW search → cache → format
// context). It reads rag.* system_settings via the hot-reloadable
// config.Settings and caches retrieval results in Valkey (jc:rag:query:{hash}).
//
// Ref: japaconnect-engineering/sections/69-rag-knowledge-layer.md
//      japaconnect-engineering/adr/ADR-069-japabot-rag-knowledge-layer.md
package rag

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/config"
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
	store     *Store
	settings  *config.Settings
	embedder  EmbeddingGenerator
	enqueuer  TaskEnqueuer
	cache     ValkeyCache // nil = caching disabled
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
// The hash is over (tenant_id, query_embedding, language, top_k) so identical
// queries hit the cache.
func ragQueryCacheKey(tenantID pgtype.UUID, queryEmbedding []float32, language string, topK int32) string {
	h := sha256.New()
	h.Write(tenantID.Bytes[:])
	for _, f := range queryEmbedding {
		bits := math.Float32bits(f)
		var buf [4]byte
		buf[0] = byte(bits)
		buf[1] = byte(bits >> 8)
		buf[2] = byte(bits >> 16)
		buf[3] = byte(bits >> 24)
		h.Write(buf[:])
	}
	h.Write([]byte(language))
	h.Write([]byte{byte(topK), byte(topK >> 8), byte(topK >> 16), byte(topK >> 24)})
	return valkey.MustKey("rag:query:%s", hex.EncodeToString(h.Sum(nil)))
}

// IndexEntity chunks the source text, generates embeddings via the AI
// gateway, and upserts each chunk into content_embeddings. After upserting,
// stale tail chunks (from a previous, longer index) are deleted.
//
// This is called by the extended content:generate_embedding task handler.
func (s *Service) IndexEntity(ctx context.Context, tenantID pgtype.UUID, entityType string, entityID pgtype.UUID, body, language string) error {
	if s.embedder == nil {
		slog.Warn("rag: index: no embedder configured, skipping",
			"entity_type", entityType, "entity_id", entityID.String())
		return nil
	}

	plainText := ExtractPlainText(body)
	if plainText == "" {
		// No text to index — purge any existing chunks for this entity.
		return s.store.DeleteEntityEmbeddings(ctx, tenantID, entityType, entityID)
	}

	maxTokens := s.settings.GetIntOr(ctx, tenantID, "rag.chunk_max_tokens", 400)
	chunkTexts := ChunkText(plainText, maxTokens, maxTokens/5)
	if len(chunkTexts) == 0 {
		return s.store.DeleteEntityEmbeddings(ctx, tenantID, entityType, entityID)
	}

	model := s.settings.GetStringOr(ctx, tenantID, "rag.embedding_model", "text-embedding-3-small")

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
	if err := s.store.DeleteStaleTail(ctx, tenantID, entityType, entityID, int32(len(chunkTexts))); err != nil {
		slog.Warn("rag: index: failed to delete stale tail",
			"entity_id", entityID.String(), "error", err)
	}

	slog.Info("rag: index: complete",
		"entity_type", entityType, "entity_id", entityID.String(), "chunks", len(chunkTexts))

	return nil
}

// PurgeEntity deletes all chunks for an entity. Used on unpublish/delete.
func (s *Service) PurgeEntity(ctx context.Context, tenantID pgtype.UUID, entityType string, entityID pgtype.UUID) error {
	return s.store.DeleteEntityEmbeddings(ctx, tenantID, entityType, entityID)
}

// Retrieve performs the RAG retrieval step: embed the query, search HNSW,
// and return formatted context passages. Results are cached in Valkey
// (jc:rag:query:{hash}, 10-min TTL) for identical query embeddings.
//
// If rag.enabled is false, returns nil (the chat handler falls back to
// non-RAG LLM call).
func (s *Service) Retrieve(ctx context.Context, tenantID pgtype.UUID, query, language string) ([]RetrievedChunk, error) {
	if s.settings != nil {
		if enabled := s.settings.GetBoolOr(ctx, tenantID, "rag.enabled", true); !enabled {
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

	model := s.settings.GetStringOr(ctx, tenantID, "rag.embedding_model", "text-embedding-3-small")
	topK := int32(s.settings.GetIntOr(ctx, tenantID, "rag.retrieval_top_k", 5))

	queryVec, err := s.embedder.GenerateEmbedding(ctx, query, model)
	if err != nil {
		return nil, fmt.Errorf("rag: retrieve: embed query: %w", err)
	}
	if len(queryVec) == 0 {
		return nil, fmt.Errorf("rag: retrieve: empty query embedding")
	}

	// Check cache.
	cacheKey := ragQueryCacheKey(tenantID, queryVec, language, topK)
	if s.cache != nil {
		if cached, err := s.cache.Get(ctx, cacheKey).Result(); err == nil && cached != "" {
			var results []RetrievedChunk
			if err := json.Unmarshal([]byte(cached), &results); err == nil {
				return results, nil
			}
		}
	}

	results, err := s.store.Search(ctx, tenantID, queryVec, language, topK)
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
		return fmt.Errorf("rag: reindex: no enqueuer configured")
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
				slog.Warn("rag: reindex: marshal payload failed",
					"content_id", id.ID.String(), "error", err)
				continue
			}

			if err := s.enqueuer.Enqueue(worker.TypeAIGenerateContentEmbedding, payload); err != nil {
				slog.Warn("rag: reindex: enqueue failed",
					"content_id", id.ID.String(), "error", err)
				continue
			}
			totalEnqueued++
		}

		offset += batchSize
	}

	slog.Info("rag: reindex: queued", "tenant_id", tenantID.String(), "total_enqueued", totalEnqueued)
	return nil
}
