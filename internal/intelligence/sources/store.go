package sources

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
)

// Store wraps the sqlc-generated Queries for the intel_sources registry.
// It follows the same pattern as internal/scholarships/store.go: a thin
// adapter that wraps each generated call with the "sources: <op>: %w"
// error prefix so callers get a consistent, traceable error chain.
// // All queries are platform-scoped (ADR-112 Amendment 2). Every wrapper method
// takes a ID pgtype.UUID as its first argument (after ctx) and stamps it
// into the sqlc query params, guaranteeing data isolation across platforms
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

// Create inserts a new intel_sources row. The ID is stamped into the
// row regardless of any value the caller set on arg.
func (s *Store) Create(ctx context.Context, arg gen.CreateIntelSourceParams) (Source, error) {
	row, err := s.queries.CreateIntelSource(ctx, arg)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return Source{}, errpkg.ErrAlreadyExists
		}

		return Source{}, fmt.Errorf("sources: create: %w", err)
	}

	return row, nil
}

// ClearPersona removes the persona attribution from a source. UpdateIntelSource
// COALESCEs persona_id so it cannot clear to NULL — this is the explicit clear.
func (s *Store) ClearPersona(ctx context.Context, id SourceID) error {
	return s.queries.ClearIntelSourcePersona(ctx, id)
}

// Get fetches a single source by its primary key.
func (s *Store) Get(ctx context.Context, id SourceID) (Source, error) {
	row, err := s.queries.GetIntelSource(
		ctx,

		id,
	)
	if err != nil {
		return Source{}, fmt.Errorf("sources: get: %w", err)
	}

	return row, nil
}

// GetBySlug fetches a single source by its public slug.
func (s *Store) GetBySlug(ctx context.Context, slug string) (Source, error) {
	row, err := s.queries.GetIntelSourceBySlug(
		ctx,

		&slug,
	)
	if err != nil {
		return Source{}, fmt.Errorf("sources: get by slug: %w", err)
	}

	return row, nil
}

// GetByURL fetches a single source by its canonical source_url.
func (s *Store) GetByURL(ctx context.Context, sourceURL string) (Source, error) {
	row, err := s.queries.GetIntelSourceByURL(
		ctx,

		sourceURL,
	)
	if err != nil {
		return Source{}, fmt.Errorf("sources: get by url: %w", err)
	}

	return row, nil
}

// List returns a page of sources ordered by created_at descending. The
// ID is stamped into the query params regardless of any value the
// caller set on arg.
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
	rows, err := s.queries.ListIntelSourcesByCategory(
		ctx,

		category,
	)
	if err != nil {
		return nil, fmt.Errorf("sources: list by category: %w", err)
	}

	return rows, nil
}

// ListDue returns up to limit enabled sources whose crawl interval has
// elapsed (or that have never been crawled), oldest-first.
func (s *Store) ListDue(ctx context.Context, limit int32) ([]Source, error) {
	rows, err := s.queries.ListDueIntelSources(
		ctx,

		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("sources: list due: %w", err)
	}

	return rows, nil
}

// ListVideoChannelsDue returns enabled video_channel sources due for a
// channel-feed sync: never synced, or last_crawled_at older than
// intervalMins. Used by the video:sync_channels poller (§106 channel-sync
// amendment).
func (s *Store) ListVideoChannelsDue(ctx context.Context, intervalMins, limit int32) ([]Source, error) {
	rows, err := s.queries.ListVideoChannelSourcesDue(ctx, gen.ListVideoChannelSourcesDueParams{
		Column1: intervalMins,
		Limit:   limit,
	})
	if err != nil {
		return nil, fmt.Errorf("sources: list video channels due: %w", err)
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
// callers pass a non-zero value to indicate success. The ID is stamped
// into the query params regardless of any value the caller set on
// arg.
func (s *Store) UpdateCrawlStatus(ctx context.Context, arg gen.UpdateIntelSourceCrawlStatusParams) error {
	if err := s.queries.UpdateIntelSourceCrawlStatus(ctx, arg); err != nil {
		return fmt.Errorf("sources: update crawl status: %w", err)
	}

	return nil
}

// UpdateRobotsPolicy replaces the cached robots.txt policy JSON for a source.
// The ID is stamped into the query params regardless of any value the
// caller set on arg.
func (s *Store) UpdateRobotsPolicy(ctx context.Context, arg gen.UpdateIntelSourceRobotsPolicyParams) error {
	if err := s.queries.UpdateIntelSourceRobotsPolicy(ctx, arg); err != nil {
		return fmt.Errorf("sources: update robots policy: %w", err)
	}

	return nil
}

// UpdateProviderConfig replaces the provider_config JSON for a source. The
// caller is responsible for merging keys it does not own — API sources carry
// endpoint/method/path in the same document, so a wholesale replace with a
// config that omits them would break the next fetch.
func (s *Store) UpdateProviderConfig(ctx context.Context, arg gen.UpdateIntelSourceProviderConfigParams) error {
	if err := s.queries.UpdateIntelSourceProviderConfig(ctx, arg); err != nil {
		return fmt.Errorf("sources: update provider config: %w", err)
	}

	return nil
}

// UpdateCategories replaces a source's categories array. Direct assignment
// (not COALESCE) so an empty array is an explicit clear — the caller decides
// presence semantics (the admin PATCH clears to '{}' on categories:[]).
func (s *Store) UpdateCategories(ctx context.Context, arg gen.UpdateIntelSourceCategoriesParams) (Source, error) {
	row, err := s.queries.UpdateIntelSourceCategories(ctx, arg)
	if err != nil {
		return Source{}, fmt.Errorf("sources: update categories: %w", err)
	}

	return row, nil
}

// MarkLastChanged stamps last_changed_at for a source, signalling that the
// upstream content has changed since the previous crawl.
func (s *Store) MarkLastChanged(ctx context.Context, id SourceID) error {
	if err := s.queries.UpdateIntelSourceLastChanged(
		ctx,

		id,
	); err != nil {
		return fmt.Errorf("sources: mark last changed: %w", err)
	}

	return nil
}

// Delete removes a source from the registry. Existing promoted records are
// preserved by the database (the row is hard-deleted; promoted intel_records
// keep their snapshot).
func (s *Store) Delete(ctx context.Context, id SourceID) error {
	if err := s.queries.DeleteIntelSource(
		ctx,

		id,
	); err != nil {
		return fmt.Errorf("sources: delete: %w", err)
	}

	return nil
}

// Update edits the configurable fields of a source (name, domain, url,
// category, trust level, crawl settings, license, usage restriction, enabled
// toggle). Returns the updated row. The ID is stamped into the params
// regardless of any value the caller set on arg.
func (s *Store) Update(ctx context.Context, arg gen.UpdateIntelSourceParams) (Source, error) {
	row, err := s.queries.UpdateIntelSource(ctx, arg)
	if err != nil {
		return Source{}, fmt.Errorf("sources: update: %w", err)
	}

	return row, nil
}

// ListProposed returns a page of sources with status='proposed' for admin
// review. Platform-scoped.
func (s *Store) ListProposed(ctx context.Context, limit, offset int32) ([]Source, error) {
	rows, err := s.queries.ListProposedIntelSources(ctx, gen.ListProposedIntelSourcesParams{
		Limit:  limit,
		Offset: offset,
	})
	if err != nil {
		return nil, fmt.Errorf("sources: list proposed: %w", err)
	}

	return rows, nil
}

// --- Source blocks ---

// BlockSource inserts a user_source_blocks row. If the row already exists,
// it returns pgx.ErrNoRows because of the ON CONFLICT DO NOTHING clause.
func (s *Store) BlockSource(ctx context.Context, blockerID, sourceID pgtype.UUID) (pgtype.UUID, error) {
	row, err := s.queries.CreateSourceBlock(ctx, gen.CreateSourceBlockParams{
		BlockerID: blockerID,
		SourceID:  sourceID,
	})
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("sources: block: %w", err)
	}
	return row.ID, nil
}

// UnblockSource removes a user_source_blocks row.
func (s *Store) UnblockSource(ctx context.Context, blockerID, sourceID pgtype.UUID) error {
	if err := s.queries.DeleteSourceBlock(ctx, gen.DeleteSourceBlockParams{
		BlockerID: blockerID,
		SourceID:  sourceID,
	}); err != nil {
		return fmt.Errorf("sources: unblock: %w", err)
	}
	return nil
}

// ListBlockedSourceIDs returns the source IDs blocked by the given user.
func (s *Store) ListBlockedSourceIDs(ctx context.Context, blockerID pgtype.UUID) ([]pgtype.UUID, error) {
	ids, err := s.queries.ListBlockedSourceIDs(
		ctx,

		blockerID,
	)
	if err != nil {
		return nil, fmt.Errorf("sources: list blocked ids: %w", err)
	}
	return ids, nil
}

// IsSourceBlocked returns whether the user has blocked the given source.
func (s *Store) IsSourceBlocked(ctx context.Context, blockerID, sourceID pgtype.UUID) (bool, error) {
	blocked, err := s.queries.IsSourceBlocked(ctx, gen.IsSourceBlockedParams{
		BlockerID: blockerID,
		SourceID:  sourceID,
	})
	if err != nil {
		return false, fmt.Errorf("sources: is blocked: %w", err)
	}
	return blocked, nil
}

// ApproveProposed activates a proposed source (status='active', enabled=true)
// and records the approver. Platform-scoped.
func (s *Store) ApproveProposed(ctx context.Context, id, approvedBy pgtype.UUID) (Source, error) {
	row, err := s.queries.ApproveProposedIntelSource(ctx, gen.ApproveProposedIntelSourceParams{
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
