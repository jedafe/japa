// Package dataimport implements the Platform Data Import feature
// (Section 67). It lets admins import platform data (users, posts, comments,
// orders, etc.) from CSV/JSON files. Supports both migration/restore (from
// a JapaConnect export file) and bulk seed data creation.
//
// Ref: japaconnect-engineering/sections/67-platform-data-portability.md
package dataimport

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
)

// Store wraps the sqlc-generated Queries for platform_data_imports.
type Store struct {
	queries      *gen.Queries
	writeQueries *gen.Queries
	pool         *db.DB
}

// NewStore creates a data-import store backed by the given database.
func NewStore(database *db.DB) *Store {
	return &Store{
		queries:      gen.New(database.Reader),
		writeQueries: gen.New(database.Writer),
		pool:         database,
	}
}

// Create inserts a new platform_data_imports row with status='pending'.
func (s *Store) Create(ctx context.Context, params gen.CreatePlatformDataImportParams) (gen.PlatformDataImport, error) {
	return s.writeQueries.CreatePlatformDataImport(ctx, params)
}

// Get retrieves a single import by ID within a platform.
func (s *Store) Get(ctx context.Context, id pgtype.UUID) (gen.PlatformDataImport, error) {
	return s.queries.GetPlatformDataImport(ctx,
		id,
	)
}

// GetByID retrieves a single import by ID only (no platform filter).
// Used by the Asynq worker, which does not have platform context.
func (s *Store) GetByID(ctx context.Context, id pgtype.UUID) (gen.PlatformDataImport, error) {
	return s.queries.GetPlatformDataImportByID(ctx, id)
}

// List returns paginated imports for a platform, optionally filtered by status.
func (s *Store) List(ctx context.Context, status string, limit, offset int32) ([]gen.PlatformDataImport, error) {
	return s.queries.ListPlatformDataImports(ctx, gen.ListPlatformDataImportsParams{

		Status:    status,
		LimitVal:  limit,
		OffsetVal: offset,
	})
}

// Count returns the total number of imports for a platform, optionally filtered.
func (s *Store) Count(ctx context.Context, status string) (int64, error) {
	return s.queries.CountPlatformDataImports(ctx,

		status,
	)
}

// UpdateStatus updates an import's status, row counts, and error log.
func (s *Store) UpdateStatus(ctx context.Context, params gen.UpdatePlatformDataImportStatusParams) (gen.PlatformDataImport, error) {
	return s.writeQueries.UpdatePlatformDataImportStatus(ctx, params)
}

// Delete removes an import row.
func (s *Store) Delete(ctx context.Context, id pgtype.UUID) error {
	return s.writeQueries.DeletePlatformDataImport(ctx,
		id,
	)
}

// ListPending returns imports with status='pending' for the worker.
func (s *Store) ListPending(ctx context.Context, limit int32) ([]gen.PlatformDataImport, error) {
	return s.queries.ListPendingPlatformDataImports(ctx, limit)
}
