// Package rag implements the JapaBot RAG Knowledge Layer (Section 69, ADR-069).
//
// It provides chunked embedding storage in content_embeddings (pgvector HNSW),
// cosine-similarity retrieval at chat time, and admin management routes for
// re-indexing, status, and entity-level purges. The text-extraction utility
// strips TipTap JSON block structure to plain text before chunking and
// embedding — never embed raw JSON markup.
//
// Ref: japaconnect-engineering/sections/69-rag-knowledge-layer.md
//      japaconnect-engineering/adr/ADR-069-japabot-rag-knowledge-layer.md
package rag

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/pgvector/pgvector-go"
	"github.com/valkey-io/valkey-go/valkeycompat"

	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
)

// EntityType constants — the set of indexable on-platform content types.
const (
	EntityTypeContent      = "content"
	EntityTypeForumThread  = "forum_thread"
	EntityTypeSpace        = "space"
	EntityTypeLearningTrack = "learning_track"
)

// SourceTier classifies an entity type as editorial (Tier 1) or community
// (Tier 2). The LLM prompt uses this label to qualify claims appropriately.
func SourceTier(entityType string) string {
	switch entityType {
	case "guide", "blog_post", "learning_track", "comparison":
		return "editorial"
	case "discussion", "forum_thread", "space":
		return "community"
	default:
		return "community"
	}
}

// Chunk is a single text chunk with its embedding, ready for upsert.
type Chunk struct {
	Index  int
	Text   string
	Vector []float32
}

// RetrievedChunk is a search result from the HNSW cosine query.
type RetrievedChunk struct {
	EntityID   string
	EntityType string
	ChunkText  string
	Similarity float64
	Tier       string
}

// Store wraps the sqlc-generated Queries for content_embeddings operations.
// It is the data-access layer — no business logic here.
type Store struct {
	queries *gen.Queries
	pool    *db.DB
}

// NewStore creates a RAG store backed by the given database.
func NewStore(database *db.DB) *Store {
	return &Store{
		queries: gen.New(database.Reader),
		pool:    database,
	}
}

// UpsertChunk inserts or replaces a single chunk embedding (ADR-069: UPSERT,
// never raw INSERT — re-indexing replaces chunks, never duplicates).
func (s *Store) UpsertChunk(ctx context.Context, tenantID pgtype.UUID, entityType string, entityID pgtype.UUID, chunk Chunk, language string) error {
	vec := pgvector.NewVector(chunk.Vector)
	_, err := s.queries.UpsertContentEmbedding(ctx, gen.UpsertContentEmbeddingParams{
		TenantID:   tenantID,
		EntityType: entityType,
		EntityID:   entityID,
		ChunkIndex: int32(chunk.Index),
		ChunkText:  chunk.Text,
		Embedding:  &vec,
		Language:   language,
	})
	if err != nil {
		return fmt.Errorf("rag: upsert chunk %d: %w", chunk.Index, err)
	}
	return nil
}

// DeleteEntityEmbeddings removes all chunks for a specific entity.
// Used on unpublish/delete and by the admin DELETE route.
func (s *Store) DeleteEntityEmbeddings(ctx context.Context, tenantID pgtype.UUID, entityType string, entityID pgtype.UUID) error {
	return s.queries.DeleteContentEmbeddingsByEntity(ctx, gen.DeleteContentEmbeddingsByEntityParams{
		TenantID:   tenantID,
		EntityType: entityType,
		EntityID:   entityID,
	})
}

// DeleteStaleTail removes chunks with chunk_index >= maxIndex for an entity.
// Called after a re-index that produced fewer chunks than the previous index.
func (s *Store) DeleteStaleTail(ctx context.Context, tenantID pgtype.UUID, entityType string, entityID pgtype.UUID, maxIndex int32) error {
	return s.queries.DeleteContentEmbeddingsAboveIndex(ctx, gen.DeleteContentEmbeddingsAboveIndexParams{
		TenantID:   tenantID,
		EntityType: entityType,
		EntityID:   entityID,
		MaxIndex:   maxIndex,
	})
}

// CountEntityChunks returns the number of chunks stored for an entity.
func (s *Store) CountEntityChunks(ctx context.Context, tenantID pgtype.UUID, entityType string, entityID pgtype.UUID) (int64, error) {
	return s.queries.CountContentEmbeddingsByEntity(ctx, gen.CountContentEmbeddingsByEntityParams{
		TenantID:   tenantID,
		EntityType: entityType,
		EntityID:   entityID,
	})
}

// PublishedContentID is a single published content ID for re-index batching.
type PublishedContentID struct {
	ID       pgtype.UUID
	TenantID pgtype.UUID
}

// ListPublishedContentIDs returns a batch of published content IDs for
// full corpus re-indexing. Uses offset pagination to avoid loading all IDs
// into memory at once.
func (s *Store) ListPublishedContentIDs(ctx context.Context, tenantID pgtype.UUID, limit, offset int32) ([]PublishedContentID, error) {
	rows, err := s.queries.ListPublishedContentIDsForReindex(ctx, gen.ListPublishedContentIDsForReindexParams{
		TenantID:   tenantID,
		PageLimit:  limit,
		PageOffset: offset,
	})
	if err != nil {
		return nil, fmt.Errorf("rag: list published content IDs: %w", err)
	}

	results := make([]PublishedContentID, 0, len(rows))
	for _, row := range rows {
		results = append(results, PublishedContentID{
			ID:       row.ID,
			TenantID: row.TenantID,
		})
	}
	return results, nil
}

// Search performs HNSW cosine similarity retrieval against content_embeddings.
// language may be empty to skip the language filter.
func (s *Store) Search(ctx context.Context, tenantID pgtype.UUID, queryVector []float32, language string, topK int32) ([]RetrievedChunk, error) {
	vec := pgvector.NewVector(queryVector)
	rows, err := s.queries.SearchContentEmbeddings(ctx, gen.SearchContentEmbeddingsParams{
		QueryEmbedding: &vec,
		TenantID:       tenantID,
		Language:       language,
		TopK:           topK,
	})
	if err != nil {
		return nil, fmt.Errorf("rag: search: %w", err)
	}

	results := make([]RetrievedChunk, 0, len(rows))
	for _, row := range rows {
		results = append(results, RetrievedChunk{
			EntityID:   row.EntityID.String(),
			EntityType: row.EntityType,
			ChunkText:  row.ChunkText,
			Similarity: row.Similarity,
			Tier:       SourceTier(row.EntityType),
		})
	}
	return results, nil
}

// IndexStats holds aggregate statistics about the embedding index.
type IndexStats struct {
	TotalChunks   int64  `json:"total_chunks"`
	TotalEntities int64  `json:"total_entities"`
	LastIndexedAt string `json:"last_indexed_at"`
	ByType        []EntityTypeStats `json:"by_type"`
}

// EntityTypeStats is the per-entity-type breakdown within IndexStats.
type EntityTypeStats struct {
	EntityType  string `json:"entity_type"`
	ChunkCount  int64  `json:"chunk_count"`
	EntityCount int64  `json:"entity_count"`
}

// GetStats returns aggregate index statistics for a tenant.
func (s *Store) GetStats(ctx context.Context, tenantID pgtype.UUID) (IndexStats, error) {
	row, err := s.queries.GetContentEmbeddingStats(ctx, tenantID)
	if err != nil {
		return IndexStats{}, fmt.Errorf("rag: get stats: %w", err)
	}

	lastIndexed := ""
	if row.LastIndexedAt.Valid {
		lastIndexed = row.LastIndexedAt.Time.UTC().Format("2006-01-02T15:04:05Z07:00")
	}

	byTypeRows, err := s.queries.GetContentEmbeddingStatsByType(ctx, tenantID)
	if err != nil {
		return IndexStats{}, fmt.Errorf("rag: get stats by type: %w", err)
	}

	byType := make([]EntityTypeStats, 0, len(byTypeRows))
	for _, r := range byTypeRows {
		byType = append(byType, EntityTypeStats{
			EntityType:  r.EntityType,
			ChunkCount:  r.ChunkCount,
			EntityCount: r.EntityCount,
		})
	}

	return IndexStats{
		TotalChunks:   row.TotalChunks,
		TotalEntities: row.TotalEntities,
		LastIndexedAt: lastIndexed,
		ByType:        byType,
	}, nil
}

// ValkeyCache is the interface for the retrieval result cache.
// The valkeycompat.Cmdable satisfies this.
type ValkeyCache interface {
	Get(ctx context.Context, key string) *valkeycompat.StringCmd
	Set(ctx context.Context, key string, value any, ttl time.Duration) *valkeycompat.StatusCmd
}
