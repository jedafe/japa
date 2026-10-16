// Package contentimport implements the Content Import & Migration Tools
// (Section 65, ADR-083). It lets users import existing blog content from
// Medium (RSS or export ZIP), Substack (RSS), and generic RSS feeds into
// JapaConnect as blog_post draft content. Each imported post becomes a
// standard contents row with canonical_url set to the original URL,
// preserving SEO credit via <link rel="canonical"> (Section 26).
//
// Ref: japaconnect-engineering/sections/65-content-import.md
//
//	japaconnect-engineering/adr/ADR-083-content-import.md
package contentimport

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
)

// Store wraps the sqlc-generated Queries for content_imports operations.
// It is the data-access layer — no business logic here.
type Store struct {
	queries      *gen.Queries
	writeQueries *gen.Queries
	pool         *db.DB
}

// NewStore creates a content-import store backed by the given database.
func NewStore(database *db.DB) *Store {
	return &Store{
		queries:      gen.New(database.Reader),
		writeQueries: gen.New(database.Writer),
		pool:         database,
	}
}

// Create inserts a new content_imports row with status='pending' and
// returns the full record. The source_url may be nil for ZIP uploads.
func (s *Store) Create(ctx context.Context, userID pgtype.UUID, sourceType string, sourceURL *string) (gen.ContentImport, error) {
	imp, err := s.writeQueries.CreateContentImport(ctx, gen.CreateContentImportParams{
		UserID:     userID,
		SourceType: sourceType,
		SourceUrl:  sourceURL,
	})
	if err != nil {
		return gen.ContentImport{}, fmt.Errorf("contentimport: create: %w", err)
	}

	return imp, nil
}

// GetByID returns a single content_imports row scoped to the platform.
func (s *Store) GetByID(ctx context.Context, importID pgtype.UUID) (gen.ContentImport, error) {
	imp, err := s.queries.GetContentImportByID(ctx,

		importID,
	)
	if err != nil {
		return gen.ContentImport{}, fmt.Errorf("contentimport: get by id: %w", err)
	}

	return imp, nil
}

// ListUserImports returns a page of a user's import history, newest first.
func (s *Store) ListUserImports(ctx context.Context, userID pgtype.UUID, offset, limit int32) ([]gen.ContentImport, error) {
	rows, err := s.queries.ListUserContentImports(ctx, gen.ListUserContentImportsParams{
		UserID:     userID,
		PageOffset: offset,
		PageLimit:  limit,
	})
	if err != nil {
		return nil, fmt.Errorf("contentimport: list user imports: %w", err)
	}

	return rows, nil
}

// CountUserImports returns the total number of imports for a user.
func (s *Store) CountUserImports(ctx context.Context, userID pgtype.UUID) (int64, error) {
	count, err := s.queries.CountUserContentImports(ctx,

		userID,
	)
	if err != nil {
		return 0, fmt.Errorf("contentimport: count user imports: %w", err)
	}

	return count, nil
}

// AdminListImports returns a page of all imports for a platform, optionally
// filtered by status. An empty status string returns all statuses.
func (s *Store) AdminListImports(ctx context.Context, status string, offset, limit int32) ([]gen.AdminListContentImportsRow, error) {
	rows, err := s.queries.AdminListContentImports(ctx, gen.AdminListContentImportsParams{
		Status:     status,
		PageOffset: offset,
		PageLimit:  limit,
	})
	if err != nil {
		return nil, fmt.Errorf("contentimport: admin list imports: %w", err)
	}

	return rows, nil
}

// AdminCountImports returns the total number of imports for a platform,
// optionally filtered by status.
func (s *Store) AdminCountImports(ctx context.Context, status string) (int64, error) {
	count, err := s.queries.AdminCountContentImports(ctx,

		status,
	)
	if err != nil {
		return 0, fmt.Errorf("contentimport: admin count imports: %w", err)
	}

	return count, nil
}

// ResetForRetry resets a failed or partial import back to pending status,
// zeroing the counts and clearing the error log. Only imports with status
// 'failed' or 'partial' can be retried.
func (s *Store) ResetForRetry(ctx context.Context, importID pgtype.UUID) (gen.ContentImport, error) {
	imp, err := s.writeQueries.ResetContentImportForRetry(ctx,

		importID,
	)
	if err != nil {
		return gen.ContentImport{}, fmt.Errorf("contentimport: reset for retry: %w", err)
	}

	return imp, nil
}

// UpdateStatus updates the status, counts, error log, and completed_at
// timestamp for an import. Called by the import:process_source task handler.
func (s *Store) UpdateStatus(ctx context.Context, importID pgtype.UUID, status string, totalItems, importedCount, failedCount *int32, errorLog []byte, completedAt pgtype.Timestamptz) (gen.ContentImport, error) {
	imp, err := s.writeQueries.UpdateContentImportStatus(ctx, gen.UpdateContentImportStatusParams{
		Status:        status,
		TotalItems:    totalItems,
		ImportedCount: importedCount,
		FailedCount:   failedCount,
		ErrorLog:      errorLog,
		CompletedAt:   completedAt,

		ID: importID,
	})
	if err != nil {
		return gen.ContentImport{}, fmt.Errorf("contentimport: update status: %w", err)
	}

	return imp, nil
}
