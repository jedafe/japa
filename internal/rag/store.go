// Package rag implements the JapaBot RAG Knowledge Layer (Section 69, ADR-069).
// // It provides chunked embedding storage in content_embeddings (pgvector HNSW),
// cosine-similarity retrieval at chat time, and admin management routes for
// re-indexing, status, and entity-level purges. The text-extraction utility
// strips TipTap JSON block structure to plain text before chunking and
// embedding — never embed raw JSON markup.
// // Ref: japaconnect-engineering/sections/69-rag-knowledge-layer.md
// //	japaconnect-engineering/adr/ADR-069-japabot-rag-knowledge-layer.md
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
	EntityTypeContent       = "content"
	EntityTypeForumThread   = "forum_thread"
	EntityTypeSpace         = "space"
	EntityTypeLearningTrack = "learning_track"
)

// SourceTier classifies an entity as editorial (Tier 1) or community (Tier 2).
// The LLM prompt uses this label to qualify claims appropriately.
// // For content entities (entity_type = "content"), the actual content_type
// (guide, blog_post, comparison, discussion) determines the tier — the
// entity_type column is generic "content" for all content entities.
// For non-content entities (forum_thread, space, learning_track), the
// entity_type itself determines the tier.
// // Ref: ADR-069 §4 — Source tier weighting.
func SourceTier(entityType, contentType string) string {
	tierKey := entityType
	if entityType == EntityTypeContent && contentType != "" {
		tierKey = contentType
	}

	switch tierKey {
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
	EntityID    string
	EntityType  string
	ContentType string
	ChunkText   string
	Similarity  float64
	Tier        string
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
func (s *Store) UpsertChunk(ctx context.Context, entityType string, entityID pgtype.UUID, chunk Chunk, language string) error {
	vec := pgvector.NewVector(chunk.Vector)

	_, err := s.queries.UpsertContentEmbedding(ctx, gen.UpsertContentEmbeddingParams{

		EntityType: entityType,
		EntityID:   entityID,
		ChunkIndex: int32(chunk.Index), // #nosec G115 -- chunk index, bounded by chunking logic
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
func (s *Store) DeleteEntityEmbeddings(ctx context.Context, entityType string, entityID pgtype.UUID) error {
	return s.queries.DeleteContentEmbeddingsByEntity(ctx, gen.DeleteContentEmbeddingsByEntityParams{

		EntityType: entityType,
		EntityID:   entityID,
	})
}

// DeleteEmbeddingsByType removes all chunks for every entity of the given
// type within a tenant. Used by the admin by-type aggregate purge.
func (s *Store) DeleteEmbeddingsByType(ctx context.Context, entityType string) error {
	return s.queries.DeleteContentEmbeddingsByType(ctx,

		entityType,
	)
}

// DeleteStaleTail removes chunks with chunk_index >= maxIndex for an entity.
// Called after a re-index that produced fewer chunks than the previous index.
func (s *Store) DeleteStaleTail(ctx context.Context, entityType string, entityID pgtype.UUID, maxIndex int32) error {
	return s.queries.DeleteContentEmbeddingsAboveIndex(ctx, gen.DeleteContentEmbeddingsAboveIndexParams{

		EntityType: entityType,
		EntityID:   entityID,
		MaxIndex:   maxIndex,
	})
}

// CountEntityChunks returns the number of chunks stored for an entity.
func (s *Store) CountEntityChunks(ctx context.Context, entityType string, entityID pgtype.UUID) (int64, error) {
	return s.queries.CountContentEmbeddingsByEntity(ctx, gen.CountContentEmbeddingsByEntityParams{

		EntityType: entityType,
		EntityID:   entityID,
	})
}

// PublishedContentID is a single published content ID for re-index batching.
type PublishedContentID struct {
	ID pgtype.UUID
}

// ListPublishedContentIDs returns a batch of published content IDs for
// full corpus re-indexing. Uses offset pagination to avoid loading all IDs
// into memory at once.
func (s *Store) ListPublishedContentIDs(ctx context.Context, limit, offset int32) ([]PublishedContentID, error) {
	rows, err := s.queries.ListPublishedContentIDsForReindex(ctx, gen.ListPublishedContentIDsForReindexParams{

		PageLimit:  limit,
		PageOffset: offset,
	})
	if err != nil {
		return nil, fmt.Errorf("rag: list published content IDs: %w", err)
	}

	results := make([]PublishedContentID, 0, len(rows))
	for _, row := range rows {
		results = append(results, PublishedContentID{
			ID: row,
		})
	}

	return results, nil
}

// ListForumThreadIDs returns a batch of non-deleted forum thread IDs for
// full corpus re-indexing (ADR-069 §2: forum threads are part of the corpus).
func (s *Store) ListForumThreadIDs(ctx context.Context, limit, offset int32) ([]PublishedContentID, error) {
	rows, err := s.queries.ListForumThreadIDsForReindex(ctx, gen.ListForumThreadIDsForReindexParams{

		PageLimit:  limit,
		PageOffset: offset,
	})
	if err != nil {
		return nil, fmt.Errorf("rag: list forum thread IDs: %w", err)
	}

	results := make([]PublishedContentID, 0, len(rows))
	for _, row := range rows {
		results = append(results, PublishedContentID{
			ID: row,
		})
	}

	return results, nil
}

// ListSpaceIDs returns a batch of active space IDs for full corpus
// re-indexing (ADR-069 §2: spaces are part of the corpus).
func (s *Store) ListSpaceIDs(ctx context.Context, limit, offset int32) ([]PublishedContentID, error) {
	rows, err := s.queries.ListSpaceIDsForReindex(ctx, gen.ListSpaceIDsForReindexParams{

		PageLimit:  limit,
		PageOffset: offset,
	})
	if err != nil {
		return nil, fmt.Errorf("rag: list space IDs: %w", err)
	}

	results := make([]PublishedContentID, 0, len(rows))
	for _, row := range rows {
		results = append(results, PublishedContentID{
			ID: row,
		})
	}

	return results, nil
}

// ListLearningTrackIDs returns a batch of published learning track IDs for
// full corpus re-indexing (ADR-069 §2: learning tracks are part of the corpus).
func (s *Store) ListLearningTrackIDs(ctx context.Context, limit, offset int32) ([]PublishedContentID, error) {
	rows, err := s.queries.ListLearningTrackIDsForReindex(ctx, gen.ListLearningTrackIDsForReindexParams{

		PageLimit:  limit,
		PageOffset: offset,
	})
	if err != nil {
		return nil, fmt.Errorf("rag: list learning track IDs: %w", err)
	}

	results := make([]PublishedContentID, 0, len(rows))
	for _, row := range rows {
		results = append(results, PublishedContentID{
			ID: row,
		})
	}

	return results, nil
}

// Search performs HNSW cosine similarity retrieval against content_embeddings.
// language may be empty to skip the language filter.
func (s *Store) Search(ctx context.Context, queryVector []float32, language string, topK int32) ([]RetrievedChunk, error) {
	vec := pgvector.NewVector(queryVector)

	rows, err := s.queries.SearchContentEmbeddings(ctx, gen.SearchContentEmbeddingsParams{
		QueryEmbedding: &vec,

		Language: language,
		TopK:     topK,
	})
	if err != nil {
		return nil, fmt.Errorf("rag: search: %w", err)
	}

	results := make([]RetrievedChunk, 0, len(rows))

	for _, row := range rows {
		contentType := ""

		if row.ContentType != nil {
			if s, ok := row.ContentType.(string); ok {
				contentType = s
			}
		}

		results = append(results, RetrievedChunk{
			EntityID:    row.EntityID.String(),
			EntityType:  row.EntityType,
			ContentType: contentType,
			ChunkText:   row.ChunkText,
			Similarity:  row.Similarity,
			Tier:        SourceTier(row.EntityType, contentType),
		})
	}

	return results, nil
}

// SearchGeoFiltered performs HNSW cosine retrieval restricted to content
// chunks whose source contents row is geo-scoped to the given space or its
// direct children (§103 Geo-Filtered RAG amendment to §69). Only
// entity_type = 'content' chunks are geo-filterable; other entity types
// are excluded when a geo scope is supplied. geoSpaceID must be non-nil
// and valid — callers dispatch to Search (unfiltered) when no geo scope
// is resolved. language may be empty to skip the language filter.
func (s *Store) SearchGeoFiltered(ctx context.Context, queryVector []float32, language string, topK int32, geoSpaceID pgtype.UUID) ([]RetrievedChunk, error) {
	vec := pgvector.NewVector(queryVector)

	rows, err := s.queries.SearchContentEmbeddingsGeoFiltered(ctx, gen.SearchContentEmbeddingsGeoFilteredParams{
		QueryEmbedding: &vec,

		Language:   language,
		TopK:       topK,
		GeoSpaceID: geoSpaceID,
	})
	if err != nil {
		return nil, fmt.Errorf("rag: search geo-filtered: %w", err)
	}

	results := make([]RetrievedChunk, 0, len(rows))
	for _, row := range rows {
		results = append(results, RetrievedChunk{
			EntityID:    row.EntityID.String(),
			EntityType:  row.EntityType,
			ContentType: row.ContentType,
			ChunkText:   row.ChunkText,
			Similarity:  row.Similarity,
			Tier:        SourceTier(row.EntityType, row.ContentType),
		})
	}

	return results, nil
}

// IndexStats holds aggregate statistics about the embedding index.
type IndexStats struct {
	TotalChunks   int64             `json:"total_chunks"`
	TotalEntities int64             `json:"total_entities"`
	LastIndexedAt string            `json:"last_indexed_at"`
	ByType        []EntityTypeStats `json:"by_type"`
}

// EntityTypeStats is the per-entity-type breakdown within IndexStats.
type EntityTypeStats struct {
	EntityType  string `json:"entity_type"`
	ChunkCount  int64  `json:"chunk_count"`
	EntityCount int64  `json:"entity_count"`
}

// GetStats returns aggregate index statistics for a tenant.
func (s *Store) GetStats(ctx context.Context) (IndexStats, error) {
	row, err := s.queries.GetContentEmbeddingStats(ctx)
	if err != nil {
		return IndexStats{}, fmt.Errorf("rag: get stats: %w", err)
	}

	lastIndexed := ""
	if row.LastIndexedAt.Valid {
		lastIndexed = row.LastIndexedAt.Time.UTC().Format("2006-01-02T15:04:05Z07:00")
	}

	byTypeRows, err := s.queries.GetContentEmbeddingStatsByType(ctx)
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
