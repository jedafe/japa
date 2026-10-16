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
	EntityTypeContent      = "content"
	EntityTypeForumThread  = "forum_thread"
	EntityTypeSpace        = "space"
	EntityTypeAcademyTrack = "academy_track"
	EntityTypeWikiPage     = "wiki_page"
)

// SourceTier classifies an entity as editorial (Tier 1) or community (Tier 2).
// The LLM prompt uses this label to qualify claims appropriately.
// // For content entities (entity_type = "content"), the actual content_type
// (guide, blog_post, comparison, discussion) determines the tier — the
// entity_type column is generic "content" for all content entities.
// For non-content entities (forum_thread, space, academy_track), the
// entity_type itself determines the tier.
// // Ref: ADR-069 §4 — Source tier weighting.
func SourceTier(entityType, contentType string) string {
	tierKey := entityType
	if entityType == EntityTypeContent && contentType != "" {
		tierKey = contentType
	}

	switch tierKey {
	case "guide", "blog_post", "academy_track", "comparison":
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

// EntityRef carries an indexable entity's identity and visibility scope
// metadata (§69 scoped-retrieval amendment). Every chunk row stores these
// denormalized values so retrieval filters without joins.
type EntityRef struct {
	Type       string
	ID         pgtype.UUID
	Visibility string      // 'public' | 'members' | 'private'
	SpaceID    pgtype.UUID // community space for space/thread entities
	AuthorID   pgtype.UUID // content author / thread author
}

// RetrievalScope describes what the viewer may retrieve — mirrors
// internal/content/visibility.VisibilityFilter plus space membership:
//   - public rows for everyone
//   - members rows for any authenticated viewer (ViewerID valid)
//   - the viewer's own rows always
//   - private rows only when CanSeePrivate (privileged callers)
//   - private space/forum_thread rows for approved member spaces
type RetrievalScope struct {
	ViewerID       pgtype.UUID
	CanSeePrivate  bool
	MemberSpaceIDs []pgtype.UUID
}

// RetrievedChunk is a search result from the HNSW cosine query.
type RetrievedChunk struct {
	EntityID    string
	EntityType  string
	ContentType string
	ChunkText   string
	Similarity  float64
	Tier        string
	// §69 Citation Source Contract — resolvable entity metadata so API
	// consumers can link citations. Empty when the entity row no longer
	// resolves (e.g. a chunk that outlived a hard-deleted entity).
	Title string
	Slug  string
	URL   string
}

// contentRoutePrefix mirrors the canonical frontend route families in
// apps/main/src/lib/utils/contentLink.ts (ADR-058) — keyed by content_type
// for entity_type='content'. Keep the two maps in sync.
var contentRoutePrefix = map[string]string{
	"blog_post":       "/blog",
	"guide":           "/guide",
	"discussion":      "/forum",
	"forum_thread":    "/forum", // legacy alias for 'discussion'
	"comparison":      "/comparisons",
	"story":           "/story",
	"achievement":     "/achievements",
	"page":            "/pages",
	"job_listing":     "/jobs",
	"service_listing": "/services",
	"video":           "/video",
}

// SourceURL returns the canonical frontend-relative path for a retrieved
// entity, per the §69 Citation Source Contract. Empty slug or unknown type
// yields "" — callers render a non-linking fallback.
func SourceURL(entityType, contentType, slug string) string {
	if slug == "" {
		return ""
	}

	switch entityType {
	case EntityTypeContent:
		prefix, ok := contentRoutePrefix[contentType]
		if !ok {
			prefix = "/content"
		}

		return prefix + "/" + slug
	case EntityTypeForumThread:
		return "/forum/" + slug
	case EntityTypeSpace:
		return "/spaces/" + slug
	case EntityTypeAcademyTrack:
		return "/academy/tracks/" + slug
	case EntityTypeWikiPage:
		// entity_slug carries "space_slug/page_slug" — see the wiki_page
		// arm of SearchContentEmbeddings.
		return "/wiki/" + slug
	default:
		return ""
	}
}

// interfaceString extracts a string from sqlc's interface{} columns
// (nullable CASE expressions scan as interface{}).
func interfaceString(v interface{}) string {
	if s, ok := v.(string); ok {
		return s
	}

	return ""
}

// Store wraps the sqlc-generated Queries for content_embeddings operations.
// It is the data-access layer — no business logic here.
type Store struct {
	queries      *gen.Queries
	writeQueries *gen.Queries
	pool         *db.DB
}

// NewStore creates a RAG store backed by the given database.
func NewStore(database *db.DB) *Store {
	return &Store{
		queries:      gen.New(database.Reader),
		writeQueries: gen.New(database.Writer),
		pool:         database,
	}
}

// UpsertChunk inserts or replaces a single chunk embedding (ADR-069: UPSERT,
// never raw INSERT — re-indexing replaces chunks, never duplicates).
// Re-indexing also refreshes the entity's scope metadata.
func (s *Store) UpsertChunk(ctx context.Context, ref EntityRef, chunk Chunk, language string) error {
	vec := pgvector.NewVector(chunk.Vector)

	_, err := s.writeQueries.UpsertContentEmbedding(ctx, gen.UpsertContentEmbeddingParams{
		EntityType: ref.Type,
		EntityID:   ref.ID,
		ChunkIndex: int32(chunk.Index), // #nosec G115 -- chunk index, bounded by chunking logic
		ChunkText:  chunk.Text,
		Embedding:  &vec,
		Language:   language,
		Visibility: ref.Visibility,
		SpaceID:    ref.SpaceID,
		AuthorID:   ref.AuthorID,
	})
	if err != nil {
		return fmt.Errorf("rag: upsert chunk %d: %w", chunk.Index, err)
	}

	return nil
}

// DeleteEntityEmbeddings removes all chunks for a specific entity.
// Used on unpublish/delete and by the admin DELETE route.
func (s *Store) DeleteEntityEmbeddings(ctx context.Context, entityType string, entityID pgtype.UUID) error {
	return s.writeQueries.DeleteContentEmbeddingsByEntity(ctx, gen.DeleteContentEmbeddingsByEntityParams{
		EntityType: entityType,
		EntityID:   entityID,
	})
}

// DeleteEmbeddingsByType removes all chunks for every entity of the given
// type within a platform. Used by the admin by-type aggregate purge.
func (s *Store) DeleteEmbeddingsByType(ctx context.Context, entityType string) error {
	return s.writeQueries.DeleteContentEmbeddingsByType(ctx,

		entityType,
	)
}

// DeleteStaleTail removes chunks with chunk_index >= maxIndex for an entity.
// Called after a re-index that produced fewer chunks than the previous index.
func (s *Store) DeleteStaleTail(ctx context.Context, entityType string, entityID pgtype.UUID, maxIndex int32) error {
	return s.writeQueries.DeleteContentEmbeddingsAboveIndex(ctx, gen.DeleteContentEmbeddingsAboveIndexParams{
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

// ReindexRef is a single entity's scope metadata for re-index batching —
// the corpus stays complete; visibility is enforced at retrieval, so every
// re-index writes correct scope columns (§69 amendment).
type ReindexRef struct {
	ID         pgtype.UUID
	Visibility string
	SpaceID    pgtype.UUID
	AuthorID   pgtype.UUID
}

// ListPublishedContentIDs returns a batch of published content refs for
// full corpus re-indexing. Uses offset pagination to avoid loading all IDs
// into memory at once.
func (s *Store) ListPublishedContentIDs(ctx context.Context, limit, offset int32) ([]ReindexRef, error) {
	rows, err := s.queries.ListPublishedContentIDsForReindex(ctx, gen.ListPublishedContentIDsForReindexParams{
		PageLimit:  limit,
		PageOffset: offset,
	})
	if err != nil {
		return nil, fmt.Errorf("rag: list published content IDs: %w", err)
	}

	results := make([]ReindexRef, 0, len(rows))
	for _, row := range rows {
		results = append(results, ReindexRef{
			ID:         row.ID,
			Visibility: row.Visibility,
			SpaceID:    row.SpaceID,
			AuthorID:   row.AuthorID,
		})
	}

	return results, nil
}

// ListForumThreadIDs returns a batch of non-deleted forum thread refs for
// full corpus re-indexing (ADR-069 §2: forum threads are part of the corpus).
// Threads inherit their parent space's visibility.
func (s *Store) ListForumThreadIDs(ctx context.Context, limit, offset int32) ([]ReindexRef, error) {
	rows, err := s.queries.ListForumThreadIDsForReindex(ctx, gen.ListForumThreadIDsForReindexParams{
		PageLimit:  limit,
		PageOffset: offset,
	})
	if err != nil {
		return nil, fmt.Errorf("rag: list forum thread IDs: %w", err)
	}

	results := make([]ReindexRef, 0, len(rows))
	for _, row := range rows {
		results = append(results, ReindexRef{
			ID:         row.ID,
			Visibility: row.Visibility,
			SpaceID:    row.SpaceID,
			AuthorID:   row.AuthorUserID,
		})
	}

	return results, nil
}

// ListSpaceIDs returns a batch of active space refs for full corpus
// re-indexing (ADR-069 §2: spaces are part of the corpus).
func (s *Store) ListSpaceIDs(ctx context.Context, limit, offset int32) ([]ReindexRef, error) {
	rows, err := s.queries.ListSpaceIDsForReindex(ctx, gen.ListSpaceIDsForReindexParams{
		PageLimit:  limit,
		PageOffset: offset,
	})
	if err != nil {
		return nil, fmt.Errorf("rag: list space IDs: %w", err)
	}

	results := make([]ReindexRef, 0, len(rows))
	for _, row := range rows {
		results = append(results, ReindexRef{
			ID:         row.ID,
			Visibility: row.Visibility,
		})
	}

	return results, nil
}

// ListAcademyTrackIDs returns a batch of published learning track IDs for
// full corpus re-indexing (ADR-069 §2: learning tracks are part of the corpus).
func (s *Store) ListAcademyTrackIDs(ctx context.Context, limit, offset int32) ([]pgtype.UUID, error) {
	rows, err := s.queries.ListAcademyTrackIDsForReindex(ctx, gen.ListAcademyTrackIDsForReindexParams{
		PageLimit:  limit,
		PageOffset: offset,
	})
	if err != nil {
		return nil, fmt.Errorf("rag: list learning track IDs: %w", err)
	}

	return rows, nil
}

// Search performs HNSW cosine similarity retrieval against content_embeddings.
// language may be empty to skip the language filter. The scope restricts
// results to what the viewer may see (§69 scoped-retrieval amendment).
func (s *Store) Search(ctx context.Context, queryVector []float32, language string, topK int32, scope RetrievalScope) ([]RetrievedChunk, error) {
	vec := pgvector.NewVector(queryVector)

	rows, err := s.queries.SearchContentEmbeddings(ctx, gen.SearchContentEmbeddingsParams{
		QueryEmbedding: &vec,

		Language:       language,
		ViewerID:       scope.ViewerID,
		CanSeePrivate:  scope.CanSeePrivate,
		MemberSpaceIds: scope.MemberSpaceIDs,
		TopK:           topK,
	})
	if err != nil {
		return nil, fmt.Errorf("rag: search: %w", err)
	}

	results := make([]RetrievedChunk, 0, len(rows))

	for _, row := range rows {
		contentType := interfaceString(row.ContentType)
		slug := interfaceString(row.EntitySlug)

		results = append(results, RetrievedChunk{
			EntityID:    row.EntityID.String(),
			EntityType:  row.EntityType,
			ContentType: contentType,
			ChunkText:   row.ChunkText,
			Similarity:  row.Similarity,
			Tier:        SourceTier(row.EntityType, contentType),
			Title:       interfaceString(row.EntityTitle),
			Slug:        slug,
			URL:         SourceURL(row.EntityType, contentType, slug),
		})
	}

	return results, nil
}

// SearchGeoFiltered performs HNSW cosine retrieval restricted to chunks
// geo-scoped to the given space or its direct children (§103 Geo-Filtered
// RAG amendment to §69). 'content' chunks scope on contents.geo_space_id;
// 'wiki_page' chunks scope on wiki_pages.space_id — the wiki is the
// canonical geo corpus so it must not be excluded when a geo scope is
// supplied. Other entity types have no geo scope and are excluded.
// geoSpaceID must be non-nil and valid — callers dispatch to Search
// (unfiltered) when no geo scope is resolved. language may be empty to
// skip the language filter.
func (s *Store) SearchGeoFiltered(ctx context.Context, queryVector []float32, language string, topK int32, geoSpaceID pgtype.UUID, scope RetrievalScope) ([]RetrievedChunk, error) {
	vec := pgvector.NewVector(queryVector)

	rows, err := s.queries.SearchContentEmbeddingsGeoFiltered(ctx, gen.SearchContentEmbeddingsGeoFilteredParams{
		QueryEmbedding: &vec,

		Language:      language,
		TopK:          topK,
		GeoSpaceID:    geoSpaceID,
		ViewerID:      scope.ViewerID,
		CanSeePrivate: scope.CanSeePrivate,
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
			Title:       row.EntityTitle,
			Slug:        row.EntitySlug,
			URL:         SourceURL(row.EntityType, row.ContentType, row.EntitySlug),
		})
	}

	return results, nil
}

// ListMemberSpaceIDs returns the viewer's approved space memberships —
// the scope's space-membership clause requires status='approved'
// (mirrors CheckSpaceMembership).
func (s *Store) ListMemberSpaceIDs(ctx context.Context, userID pgtype.UUID) ([]pgtype.UUID, error) {
	return s.queries.ListMemberSpaceIDs(ctx, userID)
}

// UpdateEntityScope refreshes scope metadata on an entity's chunk rows —
// used on visibility/status transitions that keep the entity indexable.
func (s *Store) UpdateEntityScope(ctx context.Context, ref EntityRef) error {
	return s.writeQueries.UpdateEmbeddingScope(ctx, gen.UpdateEmbeddingScopeParams{
		Visibility: ref.Visibility,
		SpaceID:    ref.SpaceID,
		AuthorID:   ref.AuthorID,
		EntityType: ref.Type,
		EntityID:   ref.ID,
	})
}

// ReconcileOrphanThreadScopes self-heals any forum_thread chunk rows whose
// source thread lost its space — runs at the start of every reindex sweep so
// a missed pre-delete reconcile cannot leak stale visibility (§69 amendment).
func (s *Store) ReconcileOrphanThreadScopes(ctx context.Context) error {
	return s.writeQueries.ReconcileOrphanThreadEmbeddings(ctx)
}

// OrphanThreadScopesBySpace fail-closes forum_thread chunk rows whose parent
// space is about to be deleted (threads survive via ON DELETE SET NULL —
// §69 amendment). Must run BEFORE the space delete commits: the FK nulls
// thread.space_id, after which these rows are unmatchable by space.
func (s *Store) OrphanThreadScopesBySpace(ctx context.Context, spaceID pgtype.UUID) error {
	return s.writeQueries.OrphanThreadEmbeddingsBySpace(ctx, spaceID)
}

// UpdateThreadScopesBySpace bulk-updates all forum_thread chunk rows in a
// space — threads inherit the space's visibility, so a space visibility
// flip reconciles every thread row.
func (s *Store) UpdateThreadScopesBySpace(ctx context.Context, spaceID pgtype.UUID, visibility string) error {
	return s.writeQueries.UpdateThreadEmbeddingsBySpace(ctx, gen.UpdateThreadEmbeddingsBySpaceParams{
		Visibility: visibility,
		SpaceID:    spaceID,
	})
}

// ContentEmbeddingMeta is the scope metadata + source text the content
// embed task needs to build an EntityRef.
type ContentEmbeddingMeta struct {
	Title      string
	Body       string
	Status     string
	Visibility string
	SpaceID    pgtype.UUID
	AuthorID   pgtype.UUID
}

// GetContentEmbeddingMeta loads scope metadata for a content entity.
func (s *Store) GetContentEmbeddingMeta(ctx context.Context, contentID pgtype.UUID) (ContentEmbeddingMeta, error) {
	row, err := s.queries.GetContentEmbeddingMeta(ctx, contentID)
	if err != nil {
		return ContentEmbeddingMeta{}, err
	}

	return ContentEmbeddingMeta{
		Title:      row.Title,
		Body:       row.Body,
		Status:     row.Status,
		Visibility: row.Visibility,
		SpaceID:    row.SpaceID,
		AuthorID:   row.AuthorID,
	}, nil
}

// IndexFailure is one exhausted index/purge retry record for admin display.
type IndexFailure struct {
	EntityType  string `json:"entity_type"`
	EntityID    string `json:"entity_id"`
	Operation   string `json:"operation"`
	Error       string `json:"error"`
	AttemptedAt string `json:"attempted_at"`
}

// RecordIndexFailure upserts an exhausted retry record. Best-effort: callers
// log the error but never propagate it (Rule 8).
func (s *Store) RecordIndexFailure(ctx context.Context, entityType string, entityID pgtype.UUID, operation string, indexErr error) error {
	if s == nil || s.writeQueries == nil {
		return nil
	}

	errStr := "<nil>"
	if indexErr != nil {
		errStr = indexErr.Error()
	}

	if err := s.writeQueries.UpsertRAGIndexFailure(ctx, gen.UpsertRAGIndexFailureParams{
		EntityType: entityType,
		EntityID:   entityID,
		Operation:  operation,
		Error:      errStr,
	}); err != nil {
		return fmt.Errorf("rag: record index failure: %w", err)
	}

	return nil
}

// ListIndexFailures returns the most recent failure records, newest first.
func (s *Store) ListIndexFailures(ctx context.Context, limit int32) ([]IndexFailure, error) {
	if s == nil || s.queries == nil {
		return nil, nil
	}

	rows, err := s.queries.ListRAGIndexFailures(ctx, limit)
	if err != nil {
		return nil, fmt.Errorf("rag: list index failures: %w", err)
	}

	out := make([]IndexFailure, 0, len(rows))
	for _, r := range rows {
		out = append(out, IndexFailure{
			EntityType:  r.EntityType,
			EntityID:    r.EntityID.String(),
			Operation:   r.Operation,
			Error:       r.Error,
			AttemptedAt: r.AttemptedAt.Time.UTC().Format("2006-01-02T15:04:05Z07:00"),
		})
	}

	return out, nil
}

// CountIndexFailures24h returns the number of failure records from the last
// 24 hours for the admin status badge.
func (s *Store) CountIndexFailures24h(ctx context.Context) (int64, error) {
	if s == nil || s.queries == nil {
		return 0, nil
	}

	n, err := s.queries.CountRAGIndexFailures24h(ctx)
	if err != nil {
		return 0, fmt.Errorf("rag: count index failures: %w", err)
	}

	return n, nil
}

// ListContentIndexFailureIDs returns the entity IDs of content-type index
// failures so the retry endpoint can re-enqueue them for embedding.
func (s *Store) ListContentIndexFailureIDs(ctx context.Context) ([]pgtype.UUID, error) {
	if s == nil || s.queries == nil {
		return nil, nil
	}

	return s.queries.ListContentRAGIndexFailures(ctx)
}

// ClearIndexFailure deletes a single failure row after a successful retry.
func (s *Store) ClearIndexFailure(ctx context.Context, entityType string, entityID pgtype.UUID, operation string) error {
	if s == nil || s.writeQueries == nil {
		return nil
	}

	if err := s.writeQueries.DeleteRAGIndexFailure(ctx, gen.DeleteRAGIndexFailureParams{
		EntityType: entityType,
		EntityID:   entityID,
		Operation:  operation,
	}); err != nil {
		return fmt.Errorf("rag: clear index failure: %w", err)
	}

	return nil
}

// ClearIndexFailures deletes failure records; empty entityType clears all.
// Returns (cleared_non_content, error); content-type rows are re-enqueued
// first and deleted per-ID from Service.RetryFailures.
func (s *Store) ClearIndexFailures(ctx context.Context, entityType string) error {
	if s == nil || s.writeQueries == nil {
		return nil
	}

	if err := s.writeQueries.DeleteRAGIndexFailures(ctx, entityType); err != nil {
		return fmt.Errorf("rag: clear index failures: %w", err)
	}

	return nil
}

// ClearNonContentIndexFailures deletes all non-content failure rows after a
// retry pass and returns the number cleared.
func (s *Store) ClearNonContentIndexFailures(ctx context.Context) (int64, error) {
	if s == nil || s.writeQueries == nil {
		return 0, nil
	}

	n, err := s.writeQueries.DeleteNonContentRAGIndexFailures(ctx)
	if err != nil {
		return 0, fmt.Errorf("rag: clear non-content index failures: %w", err)
	}

	return n, nil
}

// IndexStats holds aggregate statistics about the embedding index.
type IndexStats struct {
	TotalChunks     int64             `json:"total_chunks"`
	TotalEntities   int64             `json:"total_entities"`
	LastIndexedAt   string            `json:"last_indexed_at"`
	ByType          []EntityTypeStats `json:"by_type"`
	RecentFailures  []IndexFailure    `json:"recent_failures"`
	FailureCount24h int64             `json:"failure_count_24h"`
}

// EntityTypeStats is the per-entity-type breakdown within IndexStats.
type EntityTypeStats struct {
	EntityType  string `json:"entity_type"`
	ChunkCount  int64  `json:"chunk_count"`
	EntityCount int64  `json:"entity_count"`
}

// GetStats returns aggregate index statistics for a platform.
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

	failures, err := s.ListIndexFailures(ctx, 50)
	if err != nil {
		return IndexStats{}, err
	}

	failureCount, err := s.CountIndexFailures24h(ctx)
	if err != nil {
		return IndexStats{}, err
	}

	if failures == nil {
		failures = []IndexFailure{}
	}

	return IndexStats{
		TotalChunks:     row.TotalChunks,
		TotalEntities:   row.TotalEntities,
		LastIndexedAt:   lastIndexed,
		ByType:          byType,
		RecentFailures:  failures,
		FailureCount24h: failureCount,
	}, nil
}

// ValkeyCache is the interface for the retrieval result cache.
// The valkeycompat.Cmdable satisfies this.
type ValkeyCache interface {
	Get(ctx context.Context, key string) *valkeycompat.StringCmd
	Set(ctx context.Context, key string, value any, ttl time.Duration) *valkeycompat.StatusCmd
}
