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
}

// NewStore creates a Store backed by the Writer pool.
func NewStore(database *db.DB) *Store {
	return &Store{
		queries: gen.New(database.Writer),
	}
}

// NewStoreWithReader creates a Store using the Reader pool.
func NewStoreWithReader(database *db.DB) *Store {
	return &Store{
		queries: gen.New(database.Reader),
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
func (s *Store) GetSuggestedUsers(
	ctx context.Context,
	userID pgtype.UUID,
	tenantID pgtype.UUID,
	limit int32,
) ([]gen.GetSuggestedUsersRow, error) {
	return s.queries.GetSuggestedUsers(ctx, gen.GetSuggestedUsersParams{
		UserID:   userID,
		TenantID: tenantID,
		Limit:    limit,
	})
}

// GetSuggestedSpaces returns spaces the authenticated user might want to join.
func (s *Store) GetSuggestedSpaces(
	ctx context.Context,
	userID pgtype.UUID,
	tenantID pgtype.UUID,
	limit int32,
) ([]gen.GetSuggestedSpacesRow, error) {
	return s.queries.GetSuggestedSpaces(ctx, gen.GetSuggestedSpacesParams{
		UserID:   userID,
		TenantID: tenantID,
		Limit:    limit,
	})
}

// GetSuggestedTags returns tags the authenticated user might want to follow.
func (s *Store) GetSuggestedTags(
	ctx context.Context,
	userID pgtype.UUID,
	tenantID pgtype.UUID,
	limit int32,
) ([]gen.GetSuggestedTagsRow, error) {
	return s.queries.GetSuggestedTags(ctx, gen.GetSuggestedTagsParams{
		AuthorID: userID,
		TenantID: tenantID,
		Limit:    limit,
	})
}
