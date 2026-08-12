package sources

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
)

// Store wraps the sqlc-generated Queries for the intel_sources registry.
// It follows the same pattern as internal/scholarships/store.go: a thin
// adapter that wraps each generated call with the "sources: <op>: %w"
// error prefix so callers get a consistent, traceable error chain.
type Store struct {
	queries *gen.Queries
	pool    *db.DB
}

// NewStore creates a sources store backed by the given database. Reads and
// writes both go through database.Reader (the primary DSN until a replica
// is provisioned), matching the established scholarships store convention.
func NewStore(database *db.DB) *Store {
	return &Store{
		queries: gen.New(database.Reader),
		pool:    database,
	}
}

// Create inserts a new intel_sources row.
func (s *Store) Create(ctx context.Context, arg gen.CreateIntelSourceParams) (Source, error) {
	row, err := s.queries.CreateIntelSource(ctx, arg)
	if err != nil {
		return Source{}, fmt.Errorf("sources: create: %w", err)
	}
	return row, nil
}

// Get fetches a single source by its primary key.
func (s *Store) Get(ctx context.Context, id SourceID) (Source, error) {
	row, err := s.queries.GetIntelSource(ctx, id)
	if err != nil {
		return Source{}, fmt.Errorf("sources: get: %w", err)
	}
	return row, nil
}

// GetByURL fetches a single source by its canonical source_url.
func (s *Store) GetByURL(ctx context.Context, sourceURL string) (Source, error) {
	row, err := s.queries.GetIntelSourceByURL(ctx, sourceURL)
	if err != nil {
		return Source{}, fmt.Errorf("sources: get by url: %w", err)
	}
	return row, nil
}

// List returns a page of sources ordered by created_at descending.
func (s *Store) List(ctx context.Context, arg gen.ListIntelSourcesParams) ([]Source, error) {
	rows, err := s.queries.ListIntelSources(ctx, arg)
	if err != nil {
		return nil, fmt.Errorf("sources: list: %w", err)
	}
	return rows, nil
}

// ListByCategory returns enabled sources for a given category, ordered by
// trust level then name.
func (s *Store) ListByCategory(ctx context.Context, category string) ([]Source, error) {
	rows, err := s.queries.ListIntelSourcesByCategory(ctx, category)
	if err != nil {
		return nil, fmt.Errorf("sources: list by category: %w", err)
	}
	return rows, nil
}

// ListDue returns up to limit enabled sources whose crawl interval has
// elapsed (or that have never been crawled), oldest-first.
func (s *Store) ListDue(ctx context.Context, limit int32) ([]Source, error) {
	rows, err := s.queries.ListDueIntelSources(ctx, limit)
	if err != nil {
		return nil, fmt.Errorf("sources: list due: %w", err)
	}
	return rows, nil
}

// UpdateEnabled toggles the enabled flag for a source.
func (s *Store) UpdateEnabled(ctx context.Context, id SourceID, enabled bool) error {
	if err := s.queries.UpdateIntelSourceEnabled(ctx, gen.UpdateIntelSourceEnabledParams{
		ID:      id,
		Enabled: enabled,
	}); err != nil {
		return fmt.Errorf("sources: update enabled: %w", err)
	}
	return nil
}

// UpdateCrawlStatus records the outcome of a crawl attempt: on success
// lastSuccessAt is stamped and consecutive_failures reset; on failure
// lastErrorAt is stamped, the message stored, and the failure counter
// incremented. The LastSuccessAt field on the params is a pgtype timestamp;
// callers pass a non-zero value to indicate success.
func (s *Store) UpdateCrawlStatus(ctx context.Context, arg gen.UpdateIntelSourceCrawlStatusParams) error {
	if err := s.queries.UpdateIntelSourceCrawlStatus(ctx, arg); err != nil {
		return fmt.Errorf("sources: update crawl status: %w", err)
	}
	return nil
}

// UpdateRobotsPolicy replaces the cached robots.txt policy JSON for a source.
func (s *Store) UpdateRobotsPolicy(ctx context.Context, arg gen.UpdateIntelSourceRobotsPolicyParams) error {
	if err := s.queries.UpdateIntelSourceRobotsPolicy(ctx, arg); err != nil {
		return fmt.Errorf("sources: update robots policy: %w", err)
	}
	return nil
}

// MarkLastChanged stamps last_changed_at for a source, signalling that the
// upstream content has changed since the previous crawl.
func (s *Store) MarkLastChanged(ctx context.Context, id SourceID) error {
	if err := s.queries.UpdateIntelSourceLastChanged(ctx, id); err != nil {
		return fmt.Errorf("sources: mark last changed: %w", err)
	}
	return nil
}

// Delete removes a source from the registry. Existing promoted records are
// preserved by the database (the row is hard-deleted; promoted intel_records
// keep their snapshot).
func (s *Store) Delete(ctx context.Context, id SourceID) error {
	if err := s.queries.DeleteIntelSource(ctx, id); err != nil {
		return fmt.Errorf("sources: delete: %w", err)
	}
	return nil
}

// Compile-time guard that pgtype.UUID satisfies SourceID's underlying type.
var _ SourceID = pgtype.UUID{}
