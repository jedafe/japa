// Package worker — rag_indexer.go defines the RAGIndexer interface that the
// content:generate_embedding task delegates to for chunked embedding storage
// in content_embeddings (pgvector HNSW). The rag.Service implements this
// interface and registers itself at startup via RegisterRAGIndexer, mirroring
// the ContentEmbedder and SearchIndexer patterns.
//
// Ref: japaconnect-engineering/sections/69-rag-knowledge-layer.md
//      japaconnect-engineering/adr/ADR-069-japabot-rag-knowledge-layer.md
package worker

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"
)

// RAGIndexer defines the contract for chunking, embedding, and upserting
// content into content_embeddings (the RAG knowledge layer). The worker calls
// IndexEntity from the content:generate_embedding task handler after the
// legacy content_scores embedding is written; the rag package owns the
// chunking, AI gateway call, and DB upsert.
type RAGIndexer interface {
	// IndexEntity chunks the source text, generates embeddings via the AI
	// gateway, and upserts each chunk into content_embeddings. If the text
	// is empty, existing chunks for the entity are purged.
	IndexEntity(ctx context.Context, tenantID pgtype.UUID, entityType string, entityID pgtype.UUID, body, language string) error

	// PurgeEntity deletes all chunks for an entity. Used on unpublish/delete.
	PurgeEntity(ctx context.Context, tenantID pgtype.UUID, entityType string, entityID pgtype.UUID) error
}

var ragIndexer RAGIndexer

// RegisterRAGIndexer registers the RAG indexer for the
// content:generate_embedding background task. Call once at startup before
// the worker begins processing tasks. If never called, the embedding handler
// skips the RAG indexing step (graceful degradation).
func RegisterRAGIndexer(r RAGIndexer) {
	ragIndexer = r
}
