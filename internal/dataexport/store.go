// Package dataexport implements the Platform Data Export feature
// (Section 67). It lets admins export platform data (users, posts, comments,
// orders, etc.) in CSV/JSON/XLSX format. Each export creates a
// platform_data_exports row, enqueues a background task to generate the file,
// and stores the download URL when complete.
//
// Ref: japaconnect-engineering/sections/67-platform-data-portability.md
package dataexport

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
)

// Store wraps the sqlc-generated Queries for platform_data_exports.
type Store struct {
	queries *gen.Queries
	pool    *db.DB
}

// NewStore creates a data-export store backed by the given database.
func NewStore(database *db.DB) *Store {
	return &Store{
		queries: gen.New(database.Reader),
		pool:    database,
	}
}

// Create inserts a new platform_data_exports row with status='pending'.
func (s *Store) Create(ctx context.Context, params gen.CreatePlatformDataExportParams) (gen.PlatformDataExport, error) {
	return s.queries.CreatePlatformDataExport(ctx, params)
}

// Get retrieves a single export by ID within a tenant.
func (s *Store) Get(ctx context.Context, id pgtype.UUID) (gen.PlatformDataExport, error) {
	return s.queries.GetPlatformDataExport(ctx,
		id,
	)
}

// GetByID retrieves a single export by ID only (no tenant filter).
// Used by the Asynq worker, which does not have tenant context.
func (s *Store) GetByID(ctx context.Context, id pgtype.UUID) (gen.PlatformDataExport, error) {
	return s.queries.GetPlatformDataExportByID(ctx, id)
}

// List returns paginated exports for a tenant, optionally filtered by status.
func (s *Store) List(ctx context.Context, status string, limit, offset int32) ([]gen.PlatformDataExport, error) {
	return s.queries.ListPlatformDataExports(ctx, gen.ListPlatformDataExportsParams{

		Status:    status,
		LimitVal:  limit,
		OffsetVal: offset,
	})
}

// Count returns the total number of exports for a tenant, optionally filtered.
func (s *Store) Count(ctx context.Context, status string) (int64, error) {
	return s.queries.CountPlatformDataExports(ctx,

		status,
	)
}

// UpdateStatus updates an export's status, file URL, size, and error message.
func (s *Store) UpdateStatus(ctx context.Context, params gen.UpdatePlatformDataExportStatusParams) (gen.PlatformDataExport, error) {
	return s.queries.UpdatePlatformDataExportStatus(ctx, params)
}

// Delete removes an export row.
func (s *Store) Delete(ctx context.Context, id pgtype.UUID) error {
	return s.queries.DeletePlatformDataExport(ctx,
		id,
	)
}

// ListPending returns exports with status='pending' for the worker.
func (s *Store) ListPending(ctx context.Context, limit int32) ([]gen.PlatformDataExport, error) {
	return s.queries.ListPendingPlatformDataExports(ctx, limit)
}
