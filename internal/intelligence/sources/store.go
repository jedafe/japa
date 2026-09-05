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
//
// All queries are tenant-scoped (ADR-112 Amendment 2). Every wrapper method
// takes a tenantID pgtype.UUID as its first argument (after ctx) and stamps it
// into the sqlc query params, guaranteeing data isolation across tenants
// regardless of what the caller populated in the params struct.
type Store struct {
	queries *gen.Queries
	pool    *db.DB
}

// NewStore creates a sources store backed by the given database. The
// queries field uses database.Writer (the primary) because the store
// performs both reads and writes through s.queries — Create, UpdateEnabled,
// UpdateCrawlStatus, UpdateRobotsPolicy, and Delete all run direct UPDATE/
// INSERT/DELETE statements that fail on the read-only replica with
// SQLSTATE 25006 ("cannot execute UPDATE in a read-only transaction").
func NewStore(database *db.DB) *Store {
	return &Store{
		queries: gen.New(database.Writer),
		pool:    database,
	}
}

// Create inserts a new intel_sources row. The tenantID is stamped into the
// row regardless of any value the caller set on arg.TenantID.
func (s *Store) Create(ctx context.Context, tenantID pgtype.UUID, arg gen.CreateIntelSourceParams) (Source, error) {
	arg.TenantID = tenantID

	row, err := s.queries.CreateIntelSource(ctx, arg)
	if err != nil {
		return Source{}, fmt.Errorf("sources: create: %w", err)
	}

	return row, nil
}

// Get fetches a single source by its primary key.
func (s *Store) Get(ctx context.Context, tenantID pgtype.UUID, id SourceID) (Source, error) {
	row, err := s.queries.GetIntelSource(ctx, gen.GetIntelSourceParams{
		TenantID: tenantID,
		ID:       id,
	})
	if err != nil {
		return Source{}, fmt.Errorf("sources: get: %w", err)
	}

	return row, nil
}

// GetByURL fetches a single source by its canonical source_url.
func (s *Store) GetByURL(ctx context.Context, tenantID pgtype.UUID, sourceURL string) (Source, error) {
	row, err := s.queries.GetIntelSourceByURL(ctx, gen.GetIntelSourceByURLParams{
		TenantID:  tenantID,
		SourceUrl: sourceURL,
	})
	if err != nil {
		return Source{}, fmt.Errorf("sources: get by url: %w", err)
	}

	return row, nil
}

// List returns a page of sources ordered by created_at descending. The
// tenantID is stamped into the query params regardless of any value the
// caller set on arg.TenantID.
func (s *Store) List(ctx context.Context, tenantID pgtype.UUID, arg gen.ListIntelSourcesParams) ([]Source, error) {
	arg.TenantID = tenantID

	rows, err := s.queries.ListIntelSources(ctx, arg)
	if err != nil {
		return nil, fmt.Errorf("sources: list: %w", err)
	}

	return rows, nil
}

// ListByCategory returns enabled sources for a given category, ordered by
// trust level then name.
func (s *Store) ListByCategory(ctx context.Context, tenantID pgtype.UUID, category string) ([]Source, error) {
	rows, err := s.queries.ListIntelSourcesByCategory(ctx, gen.ListIntelSourcesByCategoryParams{
		TenantID: tenantID,
		Category: category,
	})
	if err != nil {
		return nil, fmt.Errorf("sources: list by category: %w", err)
	}

	return rows, nil
}

// ListDue returns up to limit enabled sources whose crawl interval has
// elapsed (or that have never been crawled), oldest-first.
func (s *Store) ListDue(ctx context.Context, tenantID pgtype.UUID, limit int32) ([]Source, error) {
	rows, err := s.queries.ListDueIntelSources(ctx, gen.ListDueIntelSourcesParams{
		TenantID: tenantID,
		Limit:    limit,
	})
	if err != nil {
		return nil, fmt.Errorf("sources: list due: %w", err)
	}

	return rows, nil
}

// UpdateEnabled toggles the enabled flag for a source.
func (s *Store) UpdateEnabled(ctx context.Context, tenantID pgtype.UUID, id SourceID, enabled bool) error {
	if err := s.queries.UpdateIntelSourceEnabled(ctx, gen.UpdateIntelSourceEnabledParams{
		TenantID: tenantID,
		ID:       id,
		Enabled:  enabled,
	}); err != nil {
		return fmt.Errorf("sources: update enabled: %w", err)
	}

	return nil
}

// UpdateCrawlStatus records the outcome of a crawl attempt: on success
// lastSuccessAt is stamped and consecutive_failures reset; on failure
// lastErrorAt is stamped, the message stored, and the failure counter
// incremented. The LastSuccessAt field on the params is a pgtype timestamp;
// callers pass a non-zero value to indicate success. The tenantID is stamped
// into the query params regardless of any value the caller set on
// arg.TenantID.
func (s *Store) UpdateCrawlStatus(ctx context.Context, tenantID pgtype.UUID, arg gen.UpdateIntelSourceCrawlStatusParams) error {
	arg.TenantID = tenantID
	if err := s.queries.UpdateIntelSourceCrawlStatus(ctx, arg); err != nil {
		return fmt.Errorf("sources: update crawl status: %w", err)
	}

	return nil
}

// UpdateRobotsPolicy replaces the cached robots.txt policy JSON for a source.
// The tenantID is stamped into the query params regardless of any value the
// caller set on arg.TenantID.
func (s *Store) UpdateRobotsPolicy(ctx context.Context, tenantID pgtype.UUID, arg gen.UpdateIntelSourceRobotsPolicyParams) error {
	arg.TenantID = tenantID
	if err := s.queries.UpdateIntelSourceRobotsPolicy(ctx, arg); err != nil {
		return fmt.Errorf("sources: update robots policy: %w", err)
	}

	return nil
}

// MarkLastChanged stamps last_changed_at for a source, signalling that the
// upstream content has changed since the previous crawl.
func (s *Store) MarkLastChanged(ctx context.Context, tenantID pgtype.UUID, id SourceID) error {
	if err := s.queries.UpdateIntelSourceLastChanged(ctx, gen.UpdateIntelSourceLastChangedParams{
		TenantID: tenantID,
		ID:       id,
	}); err != nil {
		return fmt.Errorf("sources: mark last changed: %w", err)
	}

	return nil
}

// Delete removes a source from the registry. Existing promoted records are
// preserved by the database (the row is hard-deleted; promoted intel_records
// keep their snapshot).
func (s *Store) Delete(ctx context.Context, tenantID pgtype.UUID, id SourceID) error {
	if err := s.queries.DeleteIntelSource(ctx, gen.DeleteIntelSourceParams{
		TenantID: tenantID,
		ID:       id,
	}); err != nil {
		return fmt.Errorf("sources: delete: %w", err)
	}

	return nil
}

// Update edits the configurable fields of a source (name, domain, url,
// category, trust level, crawl settings, license, usage restriction, enabled
// toggle). Returns the updated row. The tenantID is stamped into the params
// regardless of any value the caller set on arg.TenantID.
func (s *Store) Update(ctx context.Context, tenantID pgtype.UUID, arg gen.UpdateIntelSourceParams) (Source, error) {
	arg.TenantID = tenantID

	row, err := s.queries.UpdateIntelSource(ctx, arg)
	if err != nil {
		return Source{}, fmt.Errorf("sources: update: %w", err)
	}

	return row, nil
}

// ListProposed returns a page of sources with status='proposed' for admin
// review. Tenant-scoped.
func (s *Store) ListProposed(ctx context.Context, tenantID pgtype.UUID, limit, offset int32) ([]Source, error) {
	rows, err := s.queries.ListProposedIntelSources(ctx, gen.ListProposedIntelSourcesParams{
		TenantID: tenantID,
		Limit:    limit,
		Offset:   offset,
	})
	if err != nil {
		return nil, fmt.Errorf("sources: list proposed: %w", err)
	}

	return rows, nil
}

// ApproveProposed activates a proposed source (status='active', enabled=true)
// and records the approver. Tenant-scoped.
func (s *Store) ApproveProposed(ctx context.Context, tenantID, id, approvedBy pgtype.UUID) (Source, error) {
	row, err := s.queries.ApproveProposedIntelSource(ctx, gen.ApproveProposedIntelSourceParams{
		TenantID:   tenantID,
		ID:         id,
		ApprovedBy: approvedBy,
	})
	if err != nil {
		return Source{}, fmt.Errorf("sources: approve proposed: %w", err)
	}

	return row, nil
}

// Compile-time guard that pgtype.UUID satisfies SourceID's underlying type.
var _ SourceID = pgtype.UUID{}
