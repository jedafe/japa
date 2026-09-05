// Package stores provides sqlc-backed CRUD for the intelligence spine
// tables not owned by the sources sub-package: intel_raw_documents,
// intel_records, intel_review_log, and the scholarships projection table.
//
// It follows the same adapter pattern as internal/scholarships/store.go and
// internal/intelligence/sources/store.go: a thin wrapper around gen.Queries
// that stamps each error with the "stores: <op>: %w" prefix so callers get a
// consistent, traceable error chain.
//
// All queries are tenant-scoped (ADR-112 Amendment 2). Every wrapper method
// takes a tenantID pgtype.UUID as its first argument (after ctx) and stamps it
// into the sqlc query params, guaranteeing data isolation across tenants
// regardless of what the caller populated in the params struct.
//
// Ref: docs/superpowers/specs/2026-08-12-japa-intelligence-system-design.md
package stores

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
)

// Store wraps the sqlc-generated Queries for the intelligence spine tables
// (raw documents, records, review log) and the scholarships projection.
// All four table groups are served by one store because they are
// intelligence-related and commonly used together by the extractor,
// dedup, promoter, and freshness workers.
type Store struct {
	queries *gen.Queries
	pool    *db.DB
}

// NewStore creates a stores backed by the given database. The queries field
// uses database.Writer (the primary) because the store performs both reads
// and writes through s.queries — approve/reject/edit handlers call
// UpdateIntelRecordStatus and UpdateIntelRecordEditorialOverrides directly
// (not through WithTx). Using database.Reader (the read-only replica) for
// these writes caused "cannot execute UPDATE in a read-only transaction"
// errors (SQLSTATE 25006) on every review mutation. WithTx already uses
// database.Writer, so transactional writes were unaffected.
func NewStore(database *db.DB) *Store {
	return &Store{
		queries: gen.New(database.Writer),
		pool:    database,
	}
}

// WithTx executes fn inside a database transaction. If fn returns an error,
// the transaction rolls back; otherwise it commits. The fn callback receives
// a transactional *Store whose queries run against the transaction, so all
// writes inside fn are atomic (BUG-01/02 fix: §6.2 transaction boundary).
//
// The transactional Store does not carry tenantID itself: the caller passes
// tenantID to each method invoked inside fn, preserving the explicit
// per-call tenant-scoping contract (ADR-112 Amendment 2).
func (s *Store) WithTx(ctx context.Context, fn func(*Store) error) error {
	return db.WithTx(ctx, s.pool.Writer, func(q *gen.Queries) error {
		return fn(&Store{queries: q, pool: s.pool})
	})
}

// ---------------------------------------------------------------------------
// IntelRawDocuments
// ---------------------------------------------------------------------------

// CreateIntelRawDocument inserts a fetched raw document (headers + body) and
// returns the persisted row, including its server-generated id and
// retrieved_at timestamp. The tenantID is stamped into the row regardless of
// any value the caller set on arg.TenantID.
func (s *Store) CreateIntelRawDocument(ctx context.Context, tenantID pgtype.UUID, arg gen.CreateIntelRawDocumentParams) (gen.IntelRawDocument, error) {
	arg.TenantID = tenantID

	row, err := s.queries.CreateIntelRawDocument(ctx, arg)
	if err != nil {
		return gen.IntelRawDocument{}, fmt.Errorf("stores: create intel raw document: %w", err)
	}

	return row, nil
}

// GetLatestIntelRawDocHash returns the content_hash of the most recently
// retrieved raw document for a source. Callers use this to short-circuit
// unchanged upstream content (dedup, Task 17).
func (s *Store) GetLatestIntelRawDocHash(ctx context.Context, tenantID, sourceID pgtype.UUID) (string, error) {
	hash, err := s.queries.GetLatestIntelRawDocHash(ctx, gen.GetLatestIntelRawDocHashParams{
		TenantID: tenantID,
		SourceID: sourceID,
	})
	if err != nil {
		return "", fmt.Errorf("stores: get latest intel raw doc hash: %w", err)
	}

	return hash, nil
}

// GetIntelRawDocument fetches a single raw document by its primary key. Used
// by the control panel moderation surface (raw content pane) to render the
// fetched HTML/JSON/PDF body for reviewer inspection.
func (s *Store) GetIntelRawDocument(ctx context.Context, tenantID, id pgtype.UUID) (gen.IntelRawDocument, error) {
	row, err := s.queries.GetIntelRawDocument(ctx, gen.GetIntelRawDocumentParams{
		TenantID: tenantID,
		ID:       id,
	})
	if err != nil {
		return gen.IntelRawDocument{}, fmt.Errorf("stores: get intel raw document: %w", err)
	}

	return row, nil
}

// ---------------------------------------------------------------------------
// IntelSources
// ---------------------------------------------------------------------------

// GetIntelSource fetches a single intel source by its primary key. Used by the
// promoter (Task 19) to resolve source_url, source_type, and trust_level for a
// promoted projection row.
func (s *Store) GetIntelSource(ctx context.Context, tenantID, id pgtype.UUID) (gen.IntelSource, error) {
	row, err := s.queries.GetIntelSource(ctx, gen.GetIntelSourceParams{
		TenantID: tenantID,
		ID:       id,
	})
	if err != nil {
		return gen.IntelSource{}, fmt.Errorf("stores: get intel source: %w", err)
	}

	return row, nil
}

// GetPersonaUserID resolves the backing user_id for an AI persona. Used by
// promoters to stamp contents.author_id with the persona's bot user instead
// of the invalid defaultTenantID (FK violation fix). Returns an invalid
// pgtype.UUID when the persona does not exist so the caller can fall back
// to the default author.
func (s *Store) GetPersonaUserID(ctx context.Context, tenantID, personaID pgtype.UUID) (pgtype.UUID, error) {
	row, err := s.queries.GetIntelPersonaUserID(ctx, gen.GetIntelPersonaUserIDParams{
		TenantID: tenantID,
		ID:       personaID,
	})
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("stores: get persona user_id: %w", err)
	}

	return row, nil
}

// GetCountrySpace resolves the country-level geo space (§05) id for an ISO-3
// country code via the existing GetSpaceByGeoCode query. Returns an invalid
// pgtype.UUID (NULL) when no country space exists for the code so the caller
// can leave scholarships.country_space_id nullable. BUG-005 fix (spec §455).
func (s *Store) GetCountrySpace(ctx context.Context, tenantID pgtype.UUID, countryCode string) (pgtype.UUID, error) {
	code := countryCode

	row, err := s.queries.GetSpaceByGeoCode(ctx, gen.GetSpaceByGeoCodeParams{
		GeoCode:  &code,
		TenantID: tenantID,
	})
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("stores: get country space: %w", err)
	}

	return row.ID, nil
}

// ---------------------------------------------------------------------------
// IntelRecords
// ---------------------------------------------------------------------------

// CreateIntelRecord inserts a newly extracted intel record in the
// 'extracted' status and returns the persisted row. The tenantID is stamped
// into the row regardless of any value the caller set on arg.TenantID.
func (s *Store) CreateIntelRecord(ctx context.Context, tenantID pgtype.UUID, arg gen.CreateIntelRecordParams) (gen.IntelRecord, error) {
	arg.TenantID = tenantID

	row, err := s.queries.CreateIntelRecord(ctx, arg)
	if err != nil {
		return gen.IntelRecord{}, fmt.Errorf("stores: create intel record: %w", err)
	}

	return row, nil
}

// GetIntelRecord fetches a single intel record by its primary key.
func (s *Store) GetIntelRecord(ctx context.Context, tenantID, id pgtype.UUID) (gen.IntelRecord, error) {
	row, err := s.queries.GetIntelRecord(ctx, gen.GetIntelRecordParams{
		TenantID: tenantID,
		ID:       id,
	})
	if err != nil {
		return gen.IntelRecord{}, fmt.Errorf("stores: get intel record: %w", err)
	}

	return row, nil
}

// GetIntelRecordByExternalKey fetches a single intel record by the
// (source_id, external_key) natural key. Used by the dedup stage (Task 17)
// to detect re-extractions of the same upstream entity. The tenantID is
// stamped into the query params regardless of any value the caller set on
// arg.TenantID.
func (s *Store) GetIntelRecordByExternalKey(ctx context.Context, tenantID pgtype.UUID, arg gen.GetIntelRecordByExternalKeyParams) (gen.IntelRecord, error) {
	arg.TenantID = tenantID

	row, err := s.queries.GetIntelRecordByExternalKey(ctx, arg)
	if err != nil {
		return gen.IntelRecord{}, fmt.Errorf("stores: get intel record by external key: %w", err)
	}

	return row, nil
}

// ListIntelRecordsByExternalKey fetches all intel records matching the given
// external_key across ALL sources (within the tenant). Used by the dedup
// stage's CheckAcrossSources (BUG-20 fix) to detect when the same upstream
// entity has already been extracted from a different source — a cross-source
// conflict that should be held for human review.
func (s *Store) ListIntelRecordsByExternalKey(ctx context.Context, tenantID pgtype.UUID, externalKey string) ([]gen.IntelRecord, error) {
	rows, err := s.queries.ListIntelRecordsByExternalKey(ctx, gen.ListIntelRecordsByExternalKeyParams{
		TenantID:    tenantID,
		ExternalKey: externalKey,
	})
	if err != nil {
		return nil, fmt.Errorf("stores: list intel records by external key: %w", err)
	}

	return rows, nil
}

// ListIntelRecordsByStatus returns a page of intel records filtered by
// entity_type and status, newest extraction first. The tenantID is stamped
// into the query params regardless of any value the caller set on
// arg.TenantID.
func (s *Store) ListIntelRecordsByStatus(ctx context.Context, tenantID pgtype.UUID, arg gen.ListIntelRecordsByStatusParams) ([]gen.IntelRecord, error) {
	arg.TenantID = tenantID

	rows, err := s.queries.ListIntelRecordsByStatus(ctx, arg)
	if err != nil {
		return nil, fmt.Errorf("stores: list intel records by status: %w", err)
	}

	return rows, nil
}

// ListIntelRecordsByStatusAll returns a page of intel records filtered by
// status only (across all entity types), newest extraction first. Used by
// the review-queue handler when no entity_type filter is supplied
// (BUG-001 fix). The tenantID is stamped into the query params regardless
// of any value the caller set on arg.TenantID.
func (s *Store) ListIntelRecordsByStatusAll(ctx context.Context, tenantID pgtype.UUID, arg gen.ListIntelRecordsByStatusAllParams) ([]gen.IntelRecord, error) {
	arg.TenantID = tenantID

	rows, err := s.queries.ListIntelRecordsByStatusAll(ctx, arg)
	if err != nil {
		return nil, fmt.Errorf("stores: list intel records by status (all types): %w", err)
	}

	return rows, nil
}

// UpdateIntelRecordStatus sets the status of an intel record and stamps
// last_verified_at to now(). The tenantID is stamped into the query params
// regardless of any value the caller set on arg.TenantID.
func (s *Store) UpdateIntelRecordStatus(ctx context.Context, tenantID pgtype.UUID, arg gen.UpdateIntelRecordStatusParams) error {
	arg.TenantID = tenantID
	if err := s.queries.UpdateIntelRecordStatus(ctx, arg); err != nil {
		return fmt.Errorf("stores: update intel record status: %w", err)
	}

	return nil
}

// UpdateIntelRecordFreshness sets the freshness score of an intel record and
// automatically transitions the status to 'stale' when the score drops to
// 40 or below. The tenantID is stamped into the query params regardless of
// any value the caller set on arg.TenantID.
func (s *Store) UpdateIntelRecordFreshness(ctx context.Context, tenantID pgtype.UUID, arg gen.UpdateIntelRecordFreshnessParams) error {
	arg.TenantID = tenantID
	if err := s.queries.UpdateIntelRecordFreshness(ctx, arg); err != nil {
		return fmt.Errorf("stores: update intel record freshness: %w", err)
	}

	return nil
}

// UpdateIntelRecordPromoted records the promoted projection row id for an
// intel record and flips its status to 'published'. Used by the promoter
// (Task 19). The tenantID is stamped into the query params regardless of any
// value the caller set on arg.TenantID.
func (s *Store) UpdateIntelRecordPromoted(ctx context.Context, tenantID pgtype.UUID, arg gen.UpdateIntelRecordPromotedParams) error {
	arg.TenantID = tenantID
	if err := s.queries.UpdateIntelRecordPromoted(ctx, arg); err != nil {
		return fmt.Errorf("stores: update intel record promoted: %w", err)
	}

	return nil
}

// ListStaleIntelRecords returns up to limit published intel records whose
// freshness score has decayed to 40 or below, for re-verification by the
// freshness worker.
func (s *Store) ListStaleIntelRecords(ctx context.Context, tenantID pgtype.UUID, limit int32) ([]gen.IntelRecord, error) {
	rows, err := s.queries.ListStaleIntelRecords(ctx, gen.ListStaleIntelRecordsParams{
		TenantID: tenantID,
		Limit:    limit,
	})
	if err != nil {
		return nil, fmt.Errorf("stores: list stale intel records: %w", err)
	}

	return rows, nil
}

// CountPromotedIntelRecordsBySource returns the number of intel_records for a
// source that have been promoted (promoted_record_id IS NOT NULL). The
// source-delete handler uses this to enforce the spec §650 invariant
// "promoted records preserved" — deleting a source with promoted records
// would CASCADE-wipe the promoted scholarships + their contents guides
// (BUG-004 fix).
func (s *Store) CountPromotedIntelRecordsBySource(ctx context.Context, tenantID, sourceID pgtype.UUID) (int64, error) {
	count, err := s.queries.CountPromotedIntelRecordsBySource(ctx, gen.CountPromotedIntelRecordsBySourceParams{
		TenantID: tenantID,
		SourceID: sourceID,
	})
	if err != nil {
		return 0, fmt.Errorf("stores: count promoted intel records by source: %w", err)
	}

	return count, nil
}

// ---------------------------------------------------------------------------
// IntelReviewLog
// ---------------------------------------------------------------------------

// CreateIntelReviewLog appends an audit entry to the intel review log for a
// human-in-the-loop action (approve, reject, edit, override). The tenantID
// is stamped into the row regardless of any value the caller set on
// arg.TenantID.
func (s *Store) CreateIntelReviewLog(ctx context.Context, tenantID pgtype.UUID, arg gen.CreateIntelReviewLogParams) error {
	arg.TenantID = tenantID
	if err := s.queries.CreateIntelReviewLog(ctx, arg); err != nil {
		return fmt.Errorf("stores: create intel review log: %w", err)
	}

	return nil
}

// ListIntelReviewLog returns the review history for an intel record,
// newest action first.
func (s *Store) ListIntelReviewLog(ctx context.Context, tenantID, intelRecordID pgtype.UUID) ([]gen.IntelReviewLog, error) {
	rows, err := s.queries.ListIntelReviewLog(ctx, gen.ListIntelReviewLogParams{
		TenantID:      tenantID,
		IntelRecordID: intelRecordID,
	})
	if err != nil {
		return nil, fmt.Errorf("stores: list intel review log: %w", err)
	}

	return rows, nil
}

// ---------------------------------------------------------------------------
// Scholarships
// ---------------------------------------------------------------------------

// CreateScholarship inserts a promoted scholarship projection row and
// returns the persisted record. The tenantID is stamped into the row
// regardless of any value the caller set on arg.TenantID.
func (s *Store) CreateScholarship(ctx context.Context, tenantID pgtype.UUID, arg gen.CreateScholarshipParams) (gen.Scholarship, error) {
	arg.TenantID = tenantID

	row, err := s.queries.CreateScholarship(ctx, arg)
	if err != nil {
		return gen.Scholarship{}, fmt.Errorf("stores: create scholarship: %w", err)
	}

	return row, nil
}

// GetScholarship fetches a single scholarship by its primary key.
func (s *Store) GetScholarship(ctx context.Context, tenantID, id pgtype.UUID) (gen.Scholarship, error) {
	row, err := s.queries.GetScholarship(ctx, gen.GetScholarshipParams{
		TenantID: tenantID,
		ID:       id,
	})
	if err != nil {
		return gen.Scholarship{}, fmt.Errorf("stores: get scholarship: %w", err)
	}

	return row, nil
}

// GetScholarshipByIntelRecord fetches the scholarship projection row
// associated with a given intel record. Used by the promoter to check
// whether a record has already been promoted.
func (s *Store) GetScholarshipByIntelRecord(ctx context.Context, tenantID, intelRecordID pgtype.UUID) (gen.Scholarship, error) {
	row, err := s.queries.GetScholarshipByIntelRecord(ctx, gen.GetScholarshipByIntelRecordParams{
		TenantID:      tenantID,
		IntelRecordID: intelRecordID,
	})
	if err != nil {
		return gen.Scholarship{}, fmt.Errorf("stores: get scholarship by intel record: %w", err)
	}

	return row, nil
}

// GetScholarshipByGuide fetches the scholarship projection row for a given
// guide entry, if one has been promoted.
func (s *Store) GetScholarshipByGuide(ctx context.Context, tenantID, guideID pgtype.UUID) (gen.Scholarship, error) {
	row, err := s.queries.GetScholarshipByGuide(ctx, gen.GetScholarshipByGuideParams{
		TenantID: tenantID,
		GuideID:  guideID,
	})
	if err != nil {
		return gen.Scholarship{}, fmt.Errorf("stores: get scholarship by guide: %w", err)
	}

	return row, nil
}

// GetScholarshipBySourceURL fetches a scholarship by its canonical
// source_url. Used for dedup against existing promoted rows.
func (s *Store) GetScholarshipBySourceURL(ctx context.Context, tenantID pgtype.UUID, sourceURL string) (gen.Scholarship, error) {
	row, err := s.queries.GetScholarshipBySourceURL(ctx, gen.GetScholarshipBySourceURLParams{
		TenantID:  tenantID,
		SourceUrl: sourceURL,
	})
	if err != nil {
		return gen.Scholarship{}, fmt.Errorf("stores: get scholarship by source url: %w", err)
	}

	return row, nil
}

// UpdateScholarship overwrites the mutable fields of a scholarship
// projection row and stamps last_verified_at and updated_at to now(). The
// tenantID is stamped into the query params regardless of any value the
// caller set on arg.TenantID.
func (s *Store) UpdateScholarship(ctx context.Context, tenantID pgtype.UUID, arg gen.UpdateScholarshipParams) error {
	arg.TenantID = tenantID
	if err := s.queries.UpdateScholarship(ctx, arg); err != nil {
		return fmt.Errorf("stores: update scholarship: %w", err)
	}

	return nil
}

// UpdateScholarshipFreshness sets the freshness score of a scholarship
// projection row and stamps last_verified_at and updated_at to now(). The
// tenantID is stamped into the query params regardless of any value the
// caller set on arg.TenantID.
func (s *Store) UpdateScholarshipFreshness(ctx context.Context, tenantID pgtype.UUID, arg gen.UpdateScholarshipFreshnessParams) error {
	arg.TenantID = tenantID
	if err := s.queries.UpdateScholarshipFreshness(ctx, arg); err != nil {
		return fmt.Errorf("stores: update scholarship freshness: %w", err)
	}

	return nil
}

// UpdateScholarshipEditorialOverrides merges admin-edited fields into the
// scholarship's editorial_overrides JSONB and flips override_source to
// 'editorial' (or 'mixed' if the row was previously 'auto'-overridden). This
// is the persistence side of the "Approve with edits" / "Edit published"
// moderation action (spec §6). The tenantID is stamped into the query params.
func (s *Store) UpdateScholarshipEditorialOverrides(ctx context.Context, tenantID, id pgtype.UUID, overrides []byte) error {
	if err := s.queries.UpdateScholarshipEditorialOverrides(ctx, gen.UpdateScholarshipEditorialOverridesParams{
		TenantID:           tenantID,
		ID:                 id,
		EditorialOverrides: overrides,
	}); err != nil {
		return fmt.Errorf("stores: update scholarship editorial overrides: %w", err)
	}

	return nil
}

// ListScholarships returns a page of scholarships ordered by created_at
// descending. The tenantID is stamped into the query params regardless of
// any value the caller set on arg.TenantID.
func (s *Store) ListScholarships(ctx context.Context, tenantID pgtype.UUID, arg gen.ListScholarshipsParams) ([]gen.Scholarship, error) {
	arg.TenantID = tenantID

	rows, err := s.queries.ListScholarships(ctx, arg)
	if err != nil {
		return nil, fmt.Errorf("stores: list scholarships: %w", err)
	}

	return rows, nil
}

// ListScholarshipsKeyset returns a page of scholarships ordered by
// (created_at DESC, id DESC), resuming after the (cursorCreatedAt, cursorID)
// tuple for keyset pagination (spec §731-733, BUG-006 fix). When the cursor
// timestamp is invalid (first page), the keyset clause is skipped.
func (s *Store) ListScholarshipsKeyset(ctx context.Context, tenantID pgtype.UUID, arg gen.ListScholarshipsKeysetParams) ([]gen.Scholarship, error) {
	arg.TenantID = tenantID

	rows, err := s.queries.ListScholarshipsKeyset(ctx, arg)
	if err != nil {
		return nil, fmt.Errorf("stores: list scholarships keyset: %w", err)
	}

	return rows, nil
}

// ListScholarshipsBySpaces returns scholarships whose country_space_id is in
// the given set, for the followed-countries radar feed (spec §739,
// BUG-008b fix — replaces the per-space N+1 loop).
func (s *Store) ListScholarshipsBySpaces(ctx context.Context, tenantID pgtype.UUID, arg gen.ListScholarshipsBySpacesParams) ([]gen.Scholarship, error) {
	arg.TenantID = tenantID

	rows, err := s.queries.ListScholarshipsBySpaces(ctx, arg)
	if err != nil {
		return nil, fmt.Errorf("stores: list scholarships by spaces: %w", err)
	}

	return rows, nil
}

// SearchScholarships runs the multi-filter search against the scholarships
// projection. NULL filter values are treated as wildcards by the query. The
// tenantID is stamped into the query params regardless of any value the
// caller set on arg.TenantID.
func (s *Store) SearchScholarships(ctx context.Context, tenantID pgtype.UUID, arg gen.SearchScholarshipsParams) ([]gen.Scholarship, error) {
	arg.TenantID = tenantID

	rows, err := s.queries.SearchScholarships(ctx, arg)
	if err != nil {
		return nil, fmt.Errorf("stores: search scholarships: %w", err)
	}

	return rows, nil
}

// ListScholarshipsBySpace returns a page of scholarships for a given
// country_space_id, newest first. The tenantID is stamped into the query
// params regardless of any value the caller set on arg.TenantID.
func (s *Store) ListScholarshipsBySpace(ctx context.Context, tenantID pgtype.UUID, arg gen.ListScholarshipsBySpaceParams) ([]gen.Scholarship, error) {
	arg.TenantID = tenantID

	rows, err := s.queries.ListScholarshipsBySpace(ctx, arg)
	if err != nil {
		return nil, fmt.Errorf("stores: list scholarships by space: %w", err)
	}

	return rows, nil
}

// ListScholarshipsByCountry returns a page of scholarships for a given
// country, newest first. The tenantID is stamped into the query params
// regardless of any value the caller set on arg.TenantID.
func (s *Store) ListScholarshipsByCountry(ctx context.Context, tenantID pgtype.UUID, arg gen.ListScholarshipsByCountryParams) ([]gen.Scholarship, error) {
	arg.TenantID = tenantID

	rows, err := s.queries.ListScholarshipsByCountry(ctx, arg)
	if err != nil {
		return nil, fmt.Errorf("stores: list scholarships by country: %w", err)
	}

	return rows, nil
}

// CountScholarshipsByCountry returns the per-country scholarship counts,
// used to populate the scholarship finder facet panel.
func (s *Store) CountScholarshipsByCountry(ctx context.Context, tenantID pgtype.UUID) ([]gen.CountScholarshipsByCountryRow, error) {
	rows, err := s.queries.CountScholarshipsByCountry(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("stores: count scholarships by country: %w", err)
	}

	return rows, nil
}

// ---------------------------------------------------------------------------
// Contents (guide projection)
// ---------------------------------------------------------------------------

// CreateContent inserts a contents row and returns the persisted projection.
// Used by the promoter (Task 19) to create the guide entry that backs a
// promoted scholarship projection row. The tenantID is stamped into the row
// regardless of any value the caller set on arg.TenantID.
func (s *Store) CreateContent(ctx context.Context, tenantID pgtype.UUID, arg gen.CreateContentParams) (gen.CreateContentRow, error) {
	arg.TenantID = tenantID

	row, err := s.queries.CreateContent(ctx, arg)
	if err != nil {
		return gen.CreateContentRow{}, fmt.Errorf("stores: create content: %w", err)
	}

	return row, nil
}

// GetContentByID fetches a single contents row by primary key. Used by the
// promoter (Task 3) to load an existing guide for update.
func (s *Store) GetContentByID(ctx context.Context, tenantID, id pgtype.UUID) (gen.GetContentByIDRow, error) {
	row, err := s.queries.GetContentByID(ctx, gen.GetContentByIDParams{TenantID: tenantID, ID: id})
	if err != nil {
		return gen.GetContentByIDRow{}, fmt.Errorf("stores: get content by id: %w", err)
	}

	return row, nil
}

// GetContentBySlug fetches a single contents row by slug and tenant. Used by
// the promoter (Task 3) to check whether an existing non-deleted guide already
// occupies a candidate slug.
func (s *Store) GetContentBySlug(ctx context.Context, tenantID pgtype.UUID, slug string) (gen.GetContentBySlugRow, error) {
	row, err := s.queries.GetContentBySlug(ctx, gen.GetContentBySlugParams{TenantID: tenantID, Slug: slug})
	if err != nil {
		return gen.GetContentBySlugRow{}, fmt.Errorf("stores: get content by slug: %w", err)
	}

	return row, nil
}

// ContentSlugExists reports whether any contents row in the tenant uses the
// given slug, including soft-deleted rows, because the (tenant_id, slug)
// unique index is not partial.
func (s *Store) ContentSlugExists(ctx context.Context, tenantID pgtype.UUID, slug string) (bool, error) {
	exists, err := s.queries.ContentSlugExists(ctx, gen.ContentSlugExistsParams{TenantID: tenantID, Slug: slug})
	if err != nil {
		return false, fmt.Errorf("stores: check content slug exists: %w", err)
	}

	return exists, nil
}

// UpdateContent overwrites the mutable fields of a contents row and stamps
// updated_at to now(). Used by the promoter (Task 3) to sync guide content
// with a re-crawled scholarship extraction. The tenantID is stamped into the
// query params regardless of any value the caller set on arg.TenantID.
func (s *Store) UpdateContent(ctx context.Context, tenantID pgtype.UUID, arg gen.UpdateContentParams) (gen.UpdateContentRow, error) {
	arg.TenantID = tenantID

	row, err := s.queries.UpdateContent(ctx, arg)
	if err != nil {
		return gen.UpdateContentRow{}, fmt.Errorf("stores: update content: %w", err)
	}

	return row, nil
}

// ---------------------------------------------------------------------------
// IntelJobs (domain projection for the jobs domain)
// ---------------------------------------------------------------------------

// CreateIntelJob is an exported method.
func (s *Store) CreateIntelJob(ctx context.Context, tenantID pgtype.UUID, arg gen.CreateIntelJobParams) (gen.IntelJob, error) {
	arg.TenantID = tenantID

	row, err := s.queries.CreateIntelJob(ctx, arg)
	if err != nil {
		return gen.IntelJob{}, fmt.Errorf("stores: create intel job: %w", err)
	}

	return row, nil
}

// GetIntelJob is an exported method.
func (s *Store) GetIntelJob(ctx context.Context, tenantID, id pgtype.UUID) (gen.IntelJob, error) {
	row, err := s.queries.GetIntelJob(ctx, gen.GetIntelJobParams{TenantID: tenantID, ID: id})
	if err != nil {
		return gen.IntelJob{}, fmt.Errorf("stores: get intel job: %w", err)
	}

	return row, nil
}

// GetIntelJobByIntelRecord is an exported method.
func (s *Store) GetIntelJobByIntelRecord(ctx context.Context, tenantID, intelRecordID pgtype.UUID) (gen.IntelJob, error) {
	row, err := s.queries.GetIntelJobByIntelRecord(ctx, gen.GetIntelJobByIntelRecordParams{TenantID: tenantID, IntelRecordID: intelRecordID})
	if err != nil {
		return gen.IntelJob{}, fmt.Errorf("stores: get intel job by intel record: %w", err)
	}

	return row, nil
}

// UpdateIntelJob is an exported method.
func (s *Store) UpdateIntelJob(ctx context.Context, tenantID pgtype.UUID, arg gen.UpdateIntelJobParams) error {
	arg.TenantID = tenantID
	if err := s.queries.UpdateIntelJob(ctx, arg); err != nil {
		return fmt.Errorf("stores: update intel job: %w", err)
	}

	return nil
}

// ListIntelJobsKeyset is an exported method.
func (s *Store) ListIntelJobsKeyset(ctx context.Context, tenantID pgtype.UUID, arg gen.ListIntelJobsKeysetParams) ([]gen.IntelJob, error) {
	arg.TenantID = tenantID

	rows, err := s.queries.ListIntelJobsKeyset(ctx, arg)
	if err != nil {
		return nil, fmt.Errorf("stores: list intel jobs keyset: %w", err)
	}

	return rows, nil
}

// ListIntelJobsBySpaces is an exported method.
func (s *Store) ListIntelJobsBySpaces(ctx context.Context, tenantID pgtype.UUID, arg gen.ListIntelJobsBySpacesParams) ([]gen.IntelJob, error) {
	arg.TenantID = tenantID

	rows, err := s.queries.ListIntelJobsBySpaces(ctx, arg)
	if err != nil {
		return nil, fmt.Errorf("stores: list intel jobs by spaces: %w", err)
	}

	return rows, nil
}

// ---------------------------------------------------------------------------
// VisaRoutes (intelligence columns — domain projection for the visa domain)
// ---------------------------------------------------------------------------

// CreateVisaRouteIntel is an exported method.
func (s *Store) CreateVisaRouteIntel(ctx context.Context, tenantID pgtype.UUID, arg gen.CreateVisaRouteIntelParams) (gen.VisaRoute, error) {
	arg.TenantID = tenantID

	row, err := s.queries.CreateVisaRouteIntel(ctx, arg)
	if err != nil {
		return gen.VisaRoute{}, fmt.Errorf("stores: create visa route intel: %w", err)
	}

	return row, nil
}

// GetVisaRouteByIntelRecord is an exported method.
func (s *Store) GetVisaRouteByIntelRecord(ctx context.Context, tenantID, intelRecordID pgtype.UUID) (gen.VisaRoute, error) {
	row, err := s.queries.GetVisaRouteByIntelRecord(ctx, gen.GetVisaRouteByIntelRecordParams{TenantID: tenantID, IntelRecordID: intelRecordID})
	if err != nil {
		return gen.VisaRoute{}, fmt.Errorf("stores: get visa route by intel record: %w", err)
	}

	return row, nil
}

// UpdateVisaRouteIntel is an exported method.
func (s *Store) UpdateVisaRouteIntel(ctx context.Context, tenantID pgtype.UUID, arg gen.UpdateVisaRouteIntelParams) error {
	arg.TenantID = tenantID
	if err := s.queries.UpdateVisaRouteIntel(ctx, arg); err != nil {
		return fmt.Errorf("stores: update visa route intel: %w", err)
	}

	return nil
}

// ListVisaRoutesIntelKeyset is an exported method.
func (s *Store) ListVisaRoutesIntelKeyset(ctx context.Context, tenantID pgtype.UUID, arg gen.ListVisaRoutesIntelKeysetParams) ([]gen.VisaRoute, error) {
	arg.TenantID = tenantID

	rows, err := s.queries.ListVisaRoutesIntelKeyset(ctx, arg)
	if err != nil {
		return nil, fmt.Errorf("stores: list visa routes intel keyset: %w", err)
	}

	return rows, nil
}

// ---------------------------------------------------------------------------
// HousingGuides (intelligence columns — domain projection for the housing domain)
// ---------------------------------------------------------------------------

// CreateHousingGuideIntel is an exported method.
func (s *Store) CreateHousingGuideIntel(ctx context.Context, tenantID pgtype.UUID, arg gen.CreateHousingGuideIntelParams) (gen.HousingGuide, error) {
	arg.TenantID = tenantID

	row, err := s.queries.CreateHousingGuideIntel(ctx, arg)
	if err != nil {
		return gen.HousingGuide{}, fmt.Errorf("stores: create housing guide intel: %w", err)
	}

	return row, nil
}

// GetHousingGuideByIntelRecord is an exported method.
func (s *Store) GetHousingGuideByIntelRecord(ctx context.Context, tenantID, intelRecordID pgtype.UUID) (gen.HousingGuide, error) {
	row, err := s.queries.GetHousingGuideByIntelRecord(ctx, gen.GetHousingGuideByIntelRecordParams{TenantID: tenantID, IntelRecordID: intelRecordID})
	if err != nil {
		return gen.HousingGuide{}, fmt.Errorf("stores: get housing guide by intel record: %w", err)
	}

	return row, nil
}

// UpdateHousingGuideIntel is an exported method.
func (s *Store) UpdateHousingGuideIntel(ctx context.Context, tenantID pgtype.UUID, arg gen.UpdateHousingGuideIntelParams) error {
	arg.TenantID = tenantID
	if err := s.queries.UpdateHousingGuideIntel(ctx, arg); err != nil {
		return fmt.Errorf("stores: update housing guide intel: %w", err)
	}

	return nil
}

// ListHousingGuidesIntelKeyset is an exported method.
func (s *Store) ListHousingGuidesIntelKeyset(ctx context.Context, tenantID pgtype.UUID, arg gen.ListHousingGuidesIntelKeysetParams) ([]gen.HousingGuide, error) {
	arg.TenantID = tenantID

	rows, err := s.queries.ListHousingGuidesIntelKeyset(ctx, arg)
	if err != nil {
		return nil, fmt.Errorf("stores: list housing guides intel keyset: %w", err)
	}

	return rows, nil
}

// ---------------------------------------------------------------------------
// RemittanceRates (intelligence columns — domain projection for the remittance domain)
// ---------------------------------------------------------------------------

// CreateRemittanceRateIntel is an exported method.
func (s *Store) CreateRemittanceRateIntel(ctx context.Context, tenantID pgtype.UUID, arg gen.CreateRemittanceRateIntelParams) (gen.RemittanceRate, error) {
	arg.TenantID = tenantID

	row, err := s.queries.CreateRemittanceRateIntel(ctx, arg)
	if err != nil {
		return gen.RemittanceRate{}, fmt.Errorf("stores: create remittance rate intel: %w", err)
	}

	return row, nil
}

// GetRemittanceRateByIntelRecord is an exported method.
func (s *Store) GetRemittanceRateByIntelRecord(ctx context.Context, tenantID, intelRecordID pgtype.UUID) (gen.RemittanceRate, error) {
	row, err := s.queries.GetRemittanceRateByIntelRecord(ctx, gen.GetRemittanceRateByIntelRecordParams{TenantID: tenantID, IntelRecordID: intelRecordID})
	if err != nil {
		return gen.RemittanceRate{}, fmt.Errorf("stores: get remittance rate by intel record: %w", err)
	}

	return row, nil
}

// UpdateRemittanceRateIntel is an exported method.
func (s *Store) UpdateRemittanceRateIntel(ctx context.Context, tenantID pgtype.UUID, arg gen.UpdateRemittanceRateIntelParams) error {
	arg.TenantID = tenantID
	if err := s.queries.UpdateRemittanceRateIntel(ctx, arg); err != nil {
		return fmt.Errorf("stores: update remittance rate intel: %w", err)
	}

	return nil
}

// ListRemittanceRatesIntelKeyset is an exported method.
func (s *Store) ListRemittanceRatesIntelKeyset(ctx context.Context, tenantID pgtype.UUID, arg gen.ListRemittanceRatesIntelKeysetParams) ([]gen.RemittanceRate, error) {
	arg.TenantID = tenantID

	rows, err := s.queries.ListRemittanceRatesIntelKeyset(ctx, arg)
	if err != nil {
		return nil, fmt.Errorf("stores: list remittance rates intel keyset: %w", err)
	}

	return rows, nil
}

// ---------------------------------------------------------------------------
// ComparisonItems (intelligence columns — domain projection for the cost_of_living domain)
// ---------------------------------------------------------------------------

// CreateComparisonItemIntel is an exported method.
func (s *Store) CreateComparisonItemIntel(ctx context.Context, tenantID pgtype.UUID, arg gen.CreateComparisonItemIntelParams) (gen.ComparisonItem, error) {
	arg.TenantID = tenantID

	row, err := s.queries.CreateComparisonItemIntel(ctx, arg)
	if err != nil {
		return gen.ComparisonItem{}, fmt.Errorf("stores: create comparison item intel: %w", err)
	}

	return row, nil
}

// GetComparisonItemByIntelRecord is an exported method.
func (s *Store) GetComparisonItemByIntelRecord(ctx context.Context, tenantID, intelRecordID pgtype.UUID) (gen.ComparisonItem, error) {
	row, err := s.queries.GetComparisonItemByIntelRecord(ctx, gen.GetComparisonItemByIntelRecordParams{TenantID: tenantID, IntelRecordID: intelRecordID})
	if err != nil {
		return gen.ComparisonItem{}, fmt.Errorf("stores: get comparison item by intel record: %w", err)
	}

	return row, nil
}

// UpdateComparisonItemIntel is an exported method.
func (s *Store) UpdateComparisonItemIntel(ctx context.Context, tenantID pgtype.UUID, arg gen.UpdateComparisonItemIntelParams) error {
	arg.TenantID = tenantID
	if err := s.queries.UpdateComparisonItemIntel(ctx, arg); err != nil {
		return fmt.Errorf("stores: update comparison item intel: %w", err)
	}

	return nil
}

// ListComparisonItemsIntelKeyset is an exported method.
func (s *Store) ListComparisonItemsIntelKeyset(ctx context.Context, tenantID pgtype.UUID, arg gen.ListComparisonItemsIntelKeysetParams) ([]gen.ComparisonItem, error) {
	arg.TenantID = tenantID

	rows, err := s.queries.ListComparisonItemsIntelKeyset(ctx, arg)
	if err != nil {
		return nil, fmt.Errorf("stores: list comparison items intel keyset: %w", err)
	}

	return rows, nil
}

// ---------------------------------------------------------------------------
// WikiPages (intelligence columns — domain projection for the wiki domain)
// ---------------------------------------------------------------------------

// CreateWikiPageIntel is an exported method.
func (s *Store) CreateWikiPageIntel(ctx context.Context, tenantID pgtype.UUID, arg gen.CreateWikiPageIntelParams) (gen.WikiPage, error) {
	arg.TenantID = tenantID

	row, err := s.queries.CreateWikiPageIntel(ctx, arg)
	if err != nil {
		return gen.WikiPage{}, fmt.Errorf("stores: create wiki page intel: %w", err)
	}

	return row, nil
}

// GetWikiPageByIntelRecord is an exported method.
func (s *Store) GetWikiPageByIntelRecord(ctx context.Context, tenantID, intelRecordID pgtype.UUID) (gen.WikiPage, error) {
	row, err := s.queries.GetWikiPageByIntelRecord(ctx, gen.GetWikiPageByIntelRecordParams{TenantID: tenantID, IntelRecordID: intelRecordID})
	if err != nil {
		return gen.WikiPage{}, fmt.Errorf("stores: get wiki page by intel record: %w", err)
	}

	return row, nil
}

// UpdateWikiPageIntel is an exported method.
func (s *Store) UpdateWikiPageIntel(ctx context.Context, tenantID pgtype.UUID, arg gen.UpdateWikiPageIntelParams) error {
	arg.TenantID = tenantID
	if err := s.queries.UpdateWikiPageIntel(ctx, arg); err != nil {
		return fmt.Errorf("stores: update wiki page intel: %w", err)
	}

	return nil
}

// ---------------------------------------------------------------------------
// LanguageTestRequirements (intelligence columns — domain projection for the language test domain)
// ---------------------------------------------------------------------------

// CreateLanguageTestRequirementIntel is an exported method.
func (s *Store) CreateLanguageTestRequirementIntel(ctx context.Context, tenantID pgtype.UUID, arg gen.CreateLanguageTestRequirementIntelParams) (gen.LanguageTestRequirement, error) {
	arg.TenantID = tenantID

	row, err := s.queries.CreateLanguageTestRequirementIntel(ctx, arg)
	if err != nil {
		return gen.LanguageTestRequirement{}, fmt.Errorf("stores: create language test requirement intel: %w", err)
	}

	return row, nil
}

// GetLanguageTestRequirementByIntelRecord is an exported method.
func (s *Store) GetLanguageTestRequirementByIntelRecord(ctx context.Context, tenantID, intelRecordID pgtype.UUID) (gen.LanguageTestRequirement, error) {
	row, err := s.queries.GetLanguageTestRequirementByIntelRecord(ctx, gen.GetLanguageTestRequirementByIntelRecordParams{TenantID: tenantID, IntelRecordID: intelRecordID})
	if err != nil {
		return gen.LanguageTestRequirement{}, fmt.Errorf("stores: get language test requirement by intel record: %w", err)
	}

	return row, nil
}

// UpdateLanguageTestRequirementIntel is an exported method.
func (s *Store) UpdateLanguageTestRequirementIntel(ctx context.Context, tenantID pgtype.UUID, arg gen.UpdateLanguageTestRequirementIntelParams) error {
	arg.TenantID = tenantID
	if err := s.queries.UpdateLanguageTestRequirementIntel(ctx, arg); err != nil {
		return fmt.Errorf("stores: update language test requirement intel: %w", err)
	}

	return nil
}

// GetLanguageTestByName resolves the language_tests(id) FK from a test name
// extracted as free text. language_tests is a global reference table (no
// tenant_id), so the tenantID is accepted for interface consistency with the
// promoter store contract but is not used in the query filter.
func (s *Store) GetLanguageTestByName(ctx context.Context, _ pgtype.UUID, name string) (gen.LanguageTest, error) {
	row, err := s.queries.GetLanguageTestByName(ctx, name)
	if err != nil {
		return gen.LanguageTest{}, fmt.Errorf("stores: get language test by name: %w", err)
	}

	return row, nil
}

// ---------------------------------------------------------------------------
// TaxObligationGuides (intelligence columns — domain projection for the tax domain)
// ---------------------------------------------------------------------------

// CreateTaxObligationGuideIntel is an exported method.
func (s *Store) CreateTaxObligationGuideIntel(ctx context.Context, tenantID pgtype.UUID, arg gen.CreateTaxObligationGuideIntelParams) (gen.TaxObligationGuide, error) {
	arg.TenantID = tenantID

	row, err := s.queries.CreateTaxObligationGuideIntel(ctx, arg)
	if err != nil {
		return gen.TaxObligationGuide{}, fmt.Errorf("stores: create tax obligation guide intel: %w", err)
	}

	return row, nil
}

// GetTaxObligationGuideByIntelRecord is an exported method.
func (s *Store) GetTaxObligationGuideByIntelRecord(ctx context.Context, tenantID, intelRecordID pgtype.UUID) (gen.TaxObligationGuide, error) {
	row, err := s.queries.GetTaxObligationGuideByIntelRecord(ctx, gen.GetTaxObligationGuideByIntelRecordParams{TenantID: tenantID, IntelRecordID: intelRecordID})
	if err != nil {
		return gen.TaxObligationGuide{}, fmt.Errorf("stores: get tax obligation guide by intel record: %w", err)
	}

	return row, nil
}

// UpdateTaxObligationGuideIntel is an exported method.
func (s *Store) UpdateTaxObligationGuideIntel(ctx context.Context, tenantID pgtype.UUID, arg gen.UpdateTaxObligationGuideIntelParams) error {
	arg.TenantID = tenantID
	if err := s.queries.UpdateTaxObligationGuideIntel(ctx, arg); err != nil {
		return fmt.Errorf("stores: update tax obligation guide intel: %w", err)
	}

	return nil
}

// ---------------------------------------------------------------------------
// CommunityOrganisations (intelligence columns — domain projection for the community org domain)
// ---------------------------------------------------------------------------

// CreateCommunityOrganisationIntel is an exported method.
func (s *Store) CreateCommunityOrganisationIntel(ctx context.Context, tenantID pgtype.UUID, arg gen.CreateCommunityOrganisationIntelParams) (gen.CommunityOrganisation, error) {
	arg.TenantID = tenantID

	row, err := s.queries.CreateCommunityOrganisationIntel(ctx, arg)
	if err != nil {
		return gen.CommunityOrganisation{}, fmt.Errorf("stores: create community organisation intel: %w", err)
	}

	return row, nil
}

// GetCommunityOrganisationByIntelRecord is an exported method.
func (s *Store) GetCommunityOrganisationByIntelRecord(ctx context.Context, tenantID, intelRecordID pgtype.UUID) (gen.CommunityOrganisation, error) {
	row, err := s.queries.GetCommunityOrganisationByIntelRecord(ctx, gen.GetCommunityOrganisationByIntelRecordParams{TenantID: tenantID, IntelRecordID: intelRecordID})
	if err != nil {
		return gen.CommunityOrganisation{}, fmt.Errorf("stores: get community organisation by intel record: %w", err)
	}

	return row, nil
}

// UpdateCommunityOrganisationIntel is an exported method.
func (s *Store) UpdateCommunityOrganisationIntel(ctx context.Context, tenantID pgtype.UUID, arg gen.UpdateCommunityOrganisationIntelParams) error {
	arg.TenantID = tenantID
	if err := s.queries.UpdateCommunityOrganisationIntel(ctx, arg); err != nil {
		return fmt.Errorf("stores: update community organisation intel: %w", err)
	}

	return nil
}

// ---------------------------------------------------------------------------
// CredentialPathways (intelligence columns — domain projection for the credential checker)
// ---------------------------------------------------------------------------

// CreateCredentialPathwayIntel creates a credential_pathways row from an intelligence extraction.
func (s *Store) CreateCredentialPathwayIntel(ctx context.Context, tenantID pgtype.UUID, arg gen.CreateCredentialPathwayIntelParams) (gen.CredentialPathway, error) {
	arg.TenantID = tenantID

	row, err := s.queries.CreateCredentialPathwayIntel(ctx, arg)
	if err != nil {
		return gen.CredentialPathway{}, fmt.Errorf("stores: create credential pathway intel: %w", err)
	}

	return row, nil
}

// GetCredentialPathwayByIntelRecord returns the credential_pathways row linked to an intel record.
func (s *Store) GetCredentialPathwayByIntelRecord(ctx context.Context, tenantID, intelRecordID pgtype.UUID) (gen.CredentialPathway, error) {
	row, err := s.queries.GetCredentialPathwayByIntelRecord(ctx, gen.GetCredentialPathwayByIntelRecordParams{TenantID: tenantID, IntelRecordID: intelRecordID})
	if err != nil {
		return gen.CredentialPathway{}, fmt.Errorf("stores: get credential pathway by intel record: %w", err)
	}

	return row, nil
}

// UpdateCredentialPathwayIntel updates a credential_pathways projection row from a re-crawl.
func (s *Store) UpdateCredentialPathwayIntel(ctx context.Context, tenantID pgtype.UUID, arg gen.UpdateCredentialPathwayIntelParams) error {
	arg.TenantID = tenantID
	if err := s.queries.UpdateCredentialPathwayIntel(ctx, arg); err != nil {
		return fmt.Errorf("stores: update credential pathway intel: %w", err)
	}

	return nil
}

// GetCredentialProfessionBySlug resolves a credential profession by slug.
func (s *Store) GetCredentialProfessionBySlug(ctx context.Context, slug string) (gen.CredentialProfession, error) {
	row, err := s.queries.GetCredentialProfessionBySlug(ctx, slug)
	if err != nil {
		return gen.CredentialProfession{}, fmt.Errorf("stores: get credential profession by slug: %w", err)
	}

	return row, nil
}

// GetCredentialProfessionByName resolves a credential profession by display name.
func (s *Store) GetCredentialProfessionByName(ctx context.Context, name string) (gen.CredentialProfession, error) {
	row, err := s.queries.GetCredentialProfessionByName(ctx, name)
	if err != nil {
		return gen.CredentialProfession{}, fmt.Errorf("stores: get credential profession by name: %w", err)
	}

	return row, nil
}
