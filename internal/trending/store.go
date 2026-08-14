package trending

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
)

// Store provides database operations for trending and discovery.
type Store struct {
	queries *gen.Queries
	db      *db.DB
}

// NewStore creates a Store backed by the Writer pool.
func NewStore(database *db.DB) *Store {
	return &Store{
		queries: gen.New(database.Writer),
		db:      database,
	}
}

// NewStoreWithReader creates a Store using the Reader pool.
func NewStoreWithReader(database *db.DB) *Store {
	return &Store{
		queries: gen.New(database.Reader),
		db:      database,
	}
}

// UpsertTrendingSnapshot inserts or updates a trending snapshot for an entity.
func (s *Store) UpsertTrendingSnapshot(
	ctx context.Context,
	tenantID pgtype.UUID,
	entityType string,
	entityID pgtype.UUID,
	score float64,
	window string,
	geoCode *string,
	expiresAt time.Time,
) error {
	return s.queries.UpsertTrendingSnapshot(ctx, gen.UpsertTrendingSnapshotParams{
		TenantID:   tenantID,
		EntityType: entityType,
		EntityID:   entityID,
		Score:      score,
		Window:     window,
		GeoCode:    geoCode,
		ExpiresAt:  pgtype.Timestamptz{Time: expiresAt, Valid: true},
	})
}

// GetTrendingByType returns trending snapshots for a given entity type and window.
func (s *Store) GetTrendingByType(
	ctx context.Context,
	tenantID pgtype.UUID,
	entityType string,
	window string,
	geoCode string,
	limit int32,
) ([]gen.GetTrendingByTypeRow, error) {
	return s.queries.GetTrendingByType(ctx, gen.GetTrendingByTypeParams{
		TenantID:   tenantID,
		EntityType: entityType,
		Window:     window,
		Column4:    geoCode,
		Limit:      limit,
	})
}

// GetTrendingContent returns trending content with joined content details.
func (s *Store) GetTrendingContent(
	ctx context.Context,
	tenantID pgtype.UUID,
	window string,
	geoCode string,
	limit int32,
) ([]gen.GetTrendingContentRow, error) {
	return s.queries.GetTrendingContent(ctx, gen.GetTrendingContentParams{
		TenantID: tenantID,
		Window:   window,
		Column3:  geoCode,
		Limit:    limit,
	})
}

// GetTrendingTags returns trending tags with joined tag details.
func (s *Store) GetTrendingTags(
	ctx context.Context,
	tenantID pgtype.UUID,
	window string,
	geoCode string,
	limit int32,
) ([]gen.GetTrendingTagsRow, error) {
	return s.queries.GetTrendingTags(ctx, gen.GetTrendingTagsParams{
		TenantID: tenantID,
		Window:   window,
		Column3:  geoCode,
		Limit:    limit,
	})
}

// GetTrendingSpaces returns trending spaces with joined space details.
func (s *Store) GetTrendingSpaces(
	ctx context.Context,
	tenantID pgtype.UUID,
	window string,
	geoCode string,
	limit int32,
) ([]gen.GetTrendingSpacesRow, error) {
	return s.queries.GetTrendingSpaces(ctx, gen.GetTrendingSpacesParams{
		TenantID: tenantID,
		Window:   window,
		Column3:  geoCode,
		Limit:    limit,
	})
}

// GetTrendingUsers returns trending users with joined user/profile details.
func (s *Store) GetTrendingUsers(
	ctx context.Context,
	tenantID pgtype.UUID,
	window string,
	geoCode string,
	limit int32,
) ([]gen.GetTrendingUsersRow, error) {
	return s.queries.GetTrendingUsers(ctx, gen.GetTrendingUsersParams{
		TenantID: tenantID,
		Window:   window,
		Column3:  geoCode,
		Limit:    limit,
	})
}

// DeleteExpiredTrendingSnapshots removes all expired trending snapshots.
func (s *Store) DeleteExpiredTrendingSnapshots(ctx context.Context, tenantID pgtype.UUID) error {
	return s.queries.DeleteExpiredTrendingSnapshots(ctx, tenantID)
}

// DeleteTrendingSnapshotsByType removes trending snapshots for a specific type/window/geo combination.
func (s *Store) DeleteTrendingSnapshotsByType(
	ctx context.Context,
	tenantID pgtype.UUID,
	entityType string,
	window string,
	geoCode string,
) error {
	return s.queries.DeleteTrendingSnapshotsByType(ctx, gen.DeleteTrendingSnapshotsByTypeParams{
		TenantID:   tenantID,
		EntityType: entityType,
		Window:     window,
		Column4:    geoCode,
	})
}

// GetTrendingSearchQueries returns trending search queries from the last 24 hours.
func (s *Store) GetTrendingSearchQueries(
	ctx context.Context,
	tenantID pgtype.UUID,
	limit int32,
) ([]gen.GetTrendingSearchQueriesRow, error) {
	return s.queries.GetTrendingSearchQueries(ctx, gen.GetTrendingSearchQueriesParams{
		TenantID: tenantID,
		Limit:    limit,
	})
}

// GetSuggestedUsers returns users the authenticated user might want to follow.
// geoCode (ADR-064) re-weights the ranking to prefer users in spaces matching
// the visitor's ContentCountry; nil = global ranking.
func (s *Store) GetSuggestedUsers(
	ctx context.Context,
	userID pgtype.UUID,
	tenantID pgtype.UUID,
	limit int32,
	geoCode *string,
) ([]gen.GetSuggestedUsersRow, error) {
	return s.queries.GetSuggestedUsers(ctx, gen.GetSuggestedUsersParams{
		UserID:   userID,
		TenantID: tenantID,
		Limit:    limit,
		GeoCode:  geoCode,
	})
}

// GetSuggestedSpaces returns spaces the authenticated user might want to join.
// geoCode (ADR-064) re-weights the ranking to prefer spaces matching the
// visitor's ContentCountry; nil = global ranking.
func (s *Store) GetSuggestedSpaces(
	ctx context.Context,
	userID pgtype.UUID,
	tenantID pgtype.UUID,
	limit int32,
	geoCode *string,
) ([]gen.GetSuggestedSpacesRow, error) {
	return s.queries.GetSuggestedSpaces(ctx, gen.GetSuggestedSpacesParams{
		UserID:   userID,
		TenantID: tenantID,
		Limit:    limit,
		GeoCode:  geoCode,
	})
}

// GetSuggestedTags returns tags the authenticated user might want to follow.
// ADR-064: when geoCode is provided, tags from content in the visitor's geo
// region are preferred (re-weighted, not hidden).
func (s *Store) GetSuggestedTags(
	ctx context.Context,
	userID pgtype.UUID,
	tenantID pgtype.UUID,
	limit int32,
	geoCode *string,
) ([]gen.GetSuggestedTagsRow, error) {
	return s.queries.GetSuggestedTags(ctx, gen.GetSuggestedTagsParams{
		AuthorID: userID,
		TenantID: tenantID,
		Limit:    limit,
		GeoCode:  geoCode,
	})
}

// GetSuggestedContent returns content similar to the seed content (pgvector
// cosine similarity) or top trending content when no seed is provided.
// Section 53: "You might also like" (BUG-003).
func (s *Store) GetSuggestedContent(
	ctx context.Context,
	tenantID pgtype.UUID,
	window string,
	geoCode *string,
	seedContentID pgtype.UUID,
	limit int32,
) ([]gen.GetSuggestedContentRow, error) {
	var gc string
	if geoCode != nil {
		gc = *geoCode
	}
	return s.queries.GetSuggestedContent(ctx, gen.GetSuggestedContentParams{
		TenantID: tenantID,
		Window:   window,
		Column3:  gc,
		Column4:  seedContentID,
		Limit:    limit,
	})
}

// GetSpaceLevels returns a map of space ID → level for the given space IDs.
// Used by GetTrendingSpaces to apply the ?level= filter in Go (BUG-008).
func (s *Store) GetSpaceLevels(
	ctx context.Context,
	tenantID pgtype.UUID,
	spaceIDs []pgtype.UUID,
) (map[[16]byte]string, error) {
	if len(spaceIDs) == 0 {
		return map[[16]byte]string{}, nil
	}
	rows, err := s.db.Reader.Query(ctx, `
		SELECT id, level FROM spaces
		WHERE tenant_id = $1 AND id = ANY($2) AND deleted_at IS NULL
	`, tenantID, spaceIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	levels := make(map[[16]byte]string, len(spaceIDs))
	for rows.Next() {
		var id [16]byte
		var level string
		if err := rows.Scan(&id, &level); err != nil {
			return nil, err
		}
		levels[id] = level
	}
	return levels, rows.Err()
}
