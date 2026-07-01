// Package ai — embedder.go implements the worker.ContentEmbedder interface.
// It calls the AI dispatcher to generate embeddings and stores the result in
// content_scores.relevance_embedding using a direct SQL UPDATE (the column
// is not covered by sqlc-generated queries).
package ai

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pgvector/pgvector-go"
)

// ContentEmbedder implements worker.ContentEmbedder by calling the AI
// dispatcher and writing the result to PostgreSQL.
type ContentEmbedder struct {
	dispatcher *Dispatcher
	writer     *pgxpool.Pool
}

// NewContentEmbedder creates a ContentEmbedder backed by the given AI
// dispatcher and database writer pool.
func NewContentEmbedder(dispatcher *Dispatcher, writer *pgxpool.Pool) *ContentEmbedder {
	return &ContentEmbedder{
		dispatcher: dispatcher,
		writer:     writer,
	}
}

// EmbedContent generates a vector embedding for the content body and stores
// it in content_scores.relevance_embedding. If the content does not yet have
// a content_scores row, one is created via INSERT ... ON CONFLICT.
func (ce *ContentEmbedder) EmbedContent(ctx context.Context, contentID, tenantID [16]byte, text string) error {
	if ce.dispatcher == nil {
		slog.Warn("ai:embedder: no dispatcher configured, skipping")
		return nil
	}
	if ce.writer == nil {
		return fmt.Errorf("ai:embedder: database pool not configured")
	}

	embedding, err := GenerateContentEmbedding(ctx, ce.dispatcher, text, DefaultEmbeddingModel)
	if err != nil {
		return fmt.Errorf("ai:embedder: generating embedding: %w", err)
	}

	if len(embedding) == 0 {
		return fmt.Errorf("ai:embedder: embedding returned empty vector")
	}

	vec := pgvector.NewVector(embedding)
	id := pgtype.UUID{Bytes: contentID, Valid: true}

	// UPSERT into content_scores so a row always exists for embedding writes.
	// The ON CONFLICT clause preserves existing score values.
	_, err = ce.writer.Exec(ctx, `
		INSERT INTO content_scores (content_id, relevance_embedding, last_scored_at)
		VALUES ($1, $2, now())
		ON CONFLICT (content_id) DO UPDATE
		SET relevance_embedding = EXCLUDED.relevance_embedding,
		    last_scored_at = now()
	`, id, vec)
	if err != nil {
		return fmt.Errorf("ai:embedder: storing embedding: %w", err)
	}

	slog.Info("ai:embedder: embedding stored",
		"content_id", id.String(),
		"dimensions", len(embedding),
	)
	return nil
}
