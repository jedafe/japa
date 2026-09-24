// Package worker — rag_indexer.go defines the RAGIndexer interface that the
// content:generate_embedding task delegates to for chunked embedding storage
// in content_embeddings (pgvector HNSW). The rag.Service implements this
// interface and registers itself at startup via RegisterRAGIndexer, mirroring
// the ContentEmbedder and SearchIndexer patterns.
//
// Ref: japaconnect-engineering/sections/69-rag-knowledge-layer.md
//
//	japaconnect-engineering/adr/ADR-069-japabot-rag-knowledge-layer.md
package worker

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/jackc/pgx/v5/pgtype"
)

// RAGIndexer defines the contract for chunking, embedding, and upserting
// content into content_embeddings (the RAG knowledge layer). The worker calls
// IndexEntity from the content:generate_embedding task handler after the
// legacy content_scores embedding is written; the rag package owns the
// chunking, AI gateway call, and DB upsert.
type RAGIndexer interface {
	// IndexEntity chunks the source text, generates embeddings via the AI
	// gateway, and upserts each chunk into content_embeddings with the
	// entity's scope metadata (§69 amendment — visibility/space/author are
	// denormalized per chunk so retrieval can filter without joins). If the
	// text is empty, existing chunks for the entity are purged.
	IndexEntity(ctx context.Context, entityType string, entityID pgtype.UUID, visibility string, spaceID, authorID pgtype.UUID, body, language string) error

	// PurgeEntity deletes all chunks for an entity. Used on unpublish/delete.
	PurgeEntity(ctx context.Context, entityType string, entityID pgtype.UUID) error
}

var ragIndexer RAGIndexer

// RegisterRAGIndexer registers the RAG indexer for the
// content:generate_embedding background task. Call once at startup before
// the worker begins processing tasks. If never called, the embedding handler
// skips the RAG indexing step (graceful degradation).
func RegisterRAGIndexer(r RAGIndexer) {
	ragIndexer = r
}

// EnqueueContentEmbedding enqueues the content:generate_embedding task so a
// content row is (re-)embedded and its scope metadata refreshed — §69
// scoped-retrieval amendment: every publish transition must re-index, not
// just those that flow through the HTTP update handler. Fire-and-forget:
// errors are logged but never propagated; a nil enqueuer is a no-op.
func EnqueueContentEmbedding(ctx context.Context, contentID pgtype.UUID) {
	if taskEnqueuer == nil {
		return
	}

	payload, err := json.Marshal(map[string]string{
		"content_id": contentID.String(),
	})
	if err != nil {
		slog.Warn("worker: RAG enqueue marshal failed",
			"content_id", contentID.String(), "error", err)
		return
	}

	if err := taskEnqueuer.Enqueue(TypeAIGenerateContentEmbedding, payload); err != nil {
		slog.Warn("worker: RAG embed enqueue failed",
			"content_id", contentID.String(), "error", err)
	}
}

// PurgeRAGEntity deletes all RAG embeddings for a specific entity. This is
// called by the content handlers on delete/archive/unpublish (ADR-069 §2:
// "Content is unpublished or deleted — deletes its content_embeddings rows").
// Fire-and-forget: errors are logged but never propagated to the caller.
// If no RAG indexer is registered, the call is a no-op (graceful degradation).
func PurgeRAGEntity(ctx context.Context, entityType string, entityID pgtype.UUID) {
	if ragIndexer == nil {
		return
	}

	if err := ragIndexer.PurgeEntity(ctx, entityType, entityID); err != nil {
		slog.Warn("worker: RAG purge failed",
			"entity_type", entityType, "entity_id", entityID.String(), "error", err)
	}
}
