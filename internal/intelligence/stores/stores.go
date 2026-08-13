// Package stores provides sqlc-backed CRUD for the intelligence spine
// tables not owned by the sources sub-package: intel_raw_documents,
// intel_records, intel_review_log, and the scholarships projection table.
//
// It follows the same adapter pattern as internal/scholarships/store.go and
// internal/intelligence/sources/store.go: a thin wrapper around gen.Queries
// that stamps each error with the "stores: <op>: %w" prefix so callers get a
// consistent, traceable error chain.
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

// NewStore creates a stores backed by the given database. Reads and writes
// both go through database.Reader (the primary DSN until a replica is
// provisioned), matching the established scholarships and sources store
// conventions.
func NewStore(database *db.DB) *Store {
	return &Store{
		queries: gen.New(database.Reader),
		pool:    database,
	}
}

// ---------------------------------------------------------------------------
// IntelRawDocuments
// ---------------------------------------------------------------------------

// CreateIntelRawDocument inserts a fetched raw document (headers + body) and
// returns the persisted row, including its server-generated id and
// retrieved_at timestamp.
func (s *Store) CreateIntelRawDocument(ctx context.Context, arg gen.CreateIntelRawDocumentParams) (gen.IntelRawDocument, error) {
	row, err := s.queries.CreateIntelRawDocument(ctx, arg)
	if err != nil {
		return gen.IntelRawDocument{}, fmt.Errorf("stores: create intel raw document: %w", err)
	}
	return row, nil
}

// GetLatestIntelRawDocHash returns the content_hash of the most recently
// retrieved raw document for a source. Callers use this to short-circuit
// unchanged upstream content (dedup, Task 17).
func (s *Store) GetLatestIntelRawDocHash(ctx context.Context, sourceID pgtype.UUID) (string, error) {
	hash, err := s.queries.GetLatestIntelRawDocHash(ctx, sourceID)
	if err != nil {
		return "", fmt.Errorf("stores: get latest intel raw doc hash: %w", err)
	}
	return hash, nil
}

// ---------------------------------------------------------------------------
// IntelSources
// ---------------------------------------------------------------------------

// GetIntelSource fetches a single intel source by its primary key. Used by the
// promoter (Task 19) to resolve source_url, source_type, and trust_level for a
// promoted projection row.
func (s *Store) GetIntelSource(ctx context.Context, id pgtype.UUID) (gen.IntelSource, error) {
	row, err := s.queries.GetIntelSource(ctx, id)
	if err != nil {
		return gen.IntelSource{}, fmt.Errorf("stores: get intel source: %w", err)
	}
	return row, nil
}

// ---------------------------------------------------------------------------
// IntelRecords
// ---------------------------------------------------------------------------

// CreateIntelRecord inserts a newly extracted intel record in the
// 'extracted' status and returns the persisted row.
func (s *Store) CreateIntelRecord(ctx context.Context, arg gen.CreateIntelRecordParams) (gen.IntelRecord, error) {
	row, err := s.queries.CreateIntelRecord(ctx, arg)
	if err != nil {
		return gen.IntelRecord{}, fmt.Errorf("stores: create intel record: %w", err)
	}
	return row, nil
}

// GetIntelRecord fetches a single intel record by its primary key.
func (s *Store) GetIntelRecord(ctx context.Context, id pgtype.UUID) (gen.IntelRecord, error) {
	row, err := s.queries.GetIntelRecord(ctx, id)
	if err != nil {
		return gen.IntelRecord{}, fmt.Errorf("stores: get intel record: %w", err)
	}
	return row, nil
}

// GetIntelRecordByExternalKey fetches a single intel record by the
// (source_id, external_key) natural key. Used by the dedup stage (Task 17)
// to detect re-extractions of the same upstream entity.
func (s *Store) GetIntelRecordByExternalKey(ctx context.Context, arg gen.GetIntelRecordByExternalKeyParams) (gen.IntelRecord, error) {
	row, err := s.queries.GetIntelRecordByExternalKey(ctx, arg)
	if err != nil {
		return gen.IntelRecord{}, fmt.Errorf("stores: get intel record by external key: %w", err)
	}
	return row, nil
}

// ListIntelRecordsByStatus returns a page of intel records filtered by
// entity_type and status, newest extraction first.
func (s *Store) ListIntelRecordsByStatus(ctx context.Context, arg gen.ListIntelRecordsByStatusParams) ([]gen.IntelRecord, error) {
	rows, err := s.queries.ListIntelRecordsByStatus(ctx, arg)
	if err != nil {
		return nil, fmt.Errorf("stores: list intel records by status: %w", err)
	}
	return rows, nil
}

// UpdateIntelRecordStatus sets the status of an intel record and stamps
// last_verified_at to now().
func (s *Store) UpdateIntelRecordStatus(ctx context.Context, arg gen.UpdateIntelRecordStatusParams) error {
	if err := s.queries.UpdateIntelRecordStatus(ctx, arg); err != nil {
		return fmt.Errorf("stores: update intel record status: %w", err)
	}
	return nil
}

// UpdateIntelRecordFreshness sets the freshness score of an intel record and
// automatically transitions the status to 'stale' when the score drops to
// 40 or below.
func (s *Store) UpdateIntelRecordFreshness(ctx context.Context, arg gen.UpdateIntelRecordFreshnessParams) error {
	if err := s.queries.UpdateIntelRecordFreshness(ctx, arg); err != nil {
		return fmt.Errorf("stores: update intel record freshness: %w", err)
	}
	return nil
}

// UpdateIntelRecordPromoted records the promoted projection row id for an
// intel record and flips its status to 'published'. Used by the promoter
// (Task 19).
func (s *Store) UpdateIntelRecordPromoted(ctx context.Context, arg gen.UpdateIntelRecordPromotedParams) error {
	if err := s.queries.UpdateIntelRecordPromoted(ctx, arg); err != nil {
		return fmt.Errorf("stores: update intel record promoted: %w", err)
	}
	return nil
}

// ListStaleIntelRecords returns up to limit published intel records whose
// freshness score has decayed to 40 or below, for re-verification by the
// freshness worker.
func (s *Store) ListStaleIntelRecords(ctx context.Context, limit int32) ([]gen.IntelRecord, error) {
	rows, err := s.queries.ListStaleIntelRecords(ctx, limit)
	if err != nil {
		return nil, fmt.Errorf("stores: list stale intel records: %w", err)
	}
	return rows, nil
}

// ---------------------------------------------------------------------------
// IntelReviewLog
// ---------------------------------------------------------------------------

// CreateIntelReviewLog appends an audit entry to the intel review log for a
// human-in-the-loop action (approve, reject, edit, override).
func (s *Store) CreateIntelReviewLog(ctx context.Context, arg gen.CreateIntelReviewLogParams) error {
	if err := s.queries.CreateIntelReviewLog(ctx, arg); err != nil {
		return fmt.Errorf("stores: create intel review log: %w", err)
	}
	return nil
}

// ListIntelReviewLog returns the review history for an intel record,
// newest action first.
func (s *Store) ListIntelReviewLog(ctx context.Context, intelRecordID pgtype.UUID) ([]gen.IntelReviewLog, error) {
	rows, err := s.queries.ListIntelReviewLog(ctx, intelRecordID)
	if err != nil {
		return nil, fmt.Errorf("stores: list intel review log: %w", err)
	}
	return rows, nil
}

// ---------------------------------------------------------------------------
// Scholarships
// ---------------------------------------------------------------------------

// CreateScholarship inserts a promoted scholarship projection row and
// returns the persisted record.
func (s *Store) CreateScholarship(ctx context.Context, arg gen.CreateScholarshipParams) (gen.Scholarship, error) {
	row, err := s.queries.CreateScholarship(ctx, arg)
	if err != nil {
		return gen.Scholarship{}, fmt.Errorf("stores: create scholarship: %w", err)
	}
	return row, nil
}

// GetScholarship fetches a single scholarship by its primary key.
func (s *Store) GetScholarship(ctx context.Context, id pgtype.UUID) (gen.Scholarship, error) {
	row, err := s.queries.GetScholarship(ctx, id)
	if err != nil {
		return gen.Scholarship{}, fmt.Errorf("stores: get scholarship: %w", err)
	}
	return row, nil
}

// GetScholarshipByIntelRecord fetches the scholarship projection row
// associated with a given intel record. Used by the promoter to check
// whether a record has already been promoted.
func (s *Store) GetScholarshipByIntelRecord(ctx context.Context, intelRecordID pgtype.UUID) (gen.Scholarship, error) {
	row, err := s.queries.GetScholarshipByIntelRecord(ctx, intelRecordID)
	if err != nil {
		return gen.Scholarship{}, fmt.Errorf("stores: get scholarship by intel record: %w", err)
	}
	return row, nil
}

// GetScholarshipByGuide fetches the scholarship projection row for a given
// guide entry, if one has been promoted.
func (s *Store) GetScholarshipByGuide(ctx context.Context, guideID pgtype.UUID) (gen.Scholarship, error) {
	row, err := s.queries.GetScholarshipByGuide(ctx, guideID)
	if err != nil {
		return gen.Scholarship{}, fmt.Errorf("stores: get scholarship by guide: %w", err)
	}
	return row, nil
}

// GetScholarshipBySourceURL fetches a scholarship by its canonical
// source_url. Used for dedup against existing promoted rows.
func (s *Store) GetScholarshipBySourceURL(ctx context.Context, sourceURL string) (gen.Scholarship, error) {
	row, err := s.queries.GetScholarshipBySourceURL(ctx, sourceURL)
	if err != nil {
		return gen.Scholarship{}, fmt.Errorf("stores: get scholarship by source url: %w", err)
	}
	return row, nil
}

// UpdateScholarship overwrites the mutable fields of a scholarship
// projection row and stamps last_verified_at and updated_at to now().
func (s *Store) UpdateScholarship(ctx context.Context, arg gen.UpdateScholarshipParams) error {
	if err := s.queries.UpdateScholarship(ctx, arg); err != nil {
		return fmt.Errorf("stores: update scholarship: %w", err)
	}
	return nil
}

// UpdateScholarshipFreshness sets the freshness score of a scholarship
// projection row and stamps last_verified_at and updated_at to now().
func (s *Store) UpdateScholarshipFreshness(ctx context.Context, arg gen.UpdateScholarshipFreshnessParams) error {
	if err := s.queries.UpdateScholarshipFreshness(ctx, arg); err != nil {
		return fmt.Errorf("stores: update scholarship freshness: %w", err)
	}
	return nil
}

// ListScholarships returns a page of scholarships ordered by created_at
// descending.
func (s *Store) ListScholarships(ctx context.Context, arg gen.ListScholarshipsParams) ([]gen.Scholarship, error) {
	rows, err := s.queries.ListScholarships(ctx, arg)
	if err != nil {
		return nil, fmt.Errorf("stores: list scholarships: %w", err)
	}
	return rows, nil
}

// SearchScholarships runs the multi-filter search against the scholarships
// projection. NULL filter values are treated as wildcards by the query.
func (s *Store) SearchScholarships(ctx context.Context, arg gen.SearchScholarshipsParams) ([]gen.Scholarship, error) {
	rows, err := s.queries.SearchScholarships(ctx, arg)
	if err != nil {
		return nil, fmt.Errorf("stores: search scholarships: %w", err)
	}
	return rows, nil
}

// ListScholarshipsBySpace returns a page of scholarships for a given
// country_space_id, newest first.
func (s *Store) ListScholarshipsBySpace(ctx context.Context, arg gen.ListScholarshipsBySpaceParams) ([]gen.Scholarship, error) {
	rows, err := s.queries.ListScholarshipsBySpace(ctx, arg)
	if err != nil {
		return nil, fmt.Errorf("stores: list scholarships by space: %w", err)
	}
	return rows, nil
}

// ListScholarshipsByCountry returns a page of scholarships for a given
// country, newest first.
func (s *Store) ListScholarshipsByCountry(ctx context.Context, arg gen.ListScholarshipsByCountryParams) ([]gen.Scholarship, error) {
	rows, err := s.queries.ListScholarshipsByCountry(ctx, arg)
	if err != nil {
		return nil, fmt.Errorf("stores: list scholarships by country: %w", err)
	}
	return rows, nil
}

// CountScholarshipsByCountry returns the per-country scholarship counts,
// used to populate the scholarship finder facet panel.
func (s *Store) CountScholarshipsByCountry(ctx context.Context) ([]gen.CountScholarshipsByCountryRow, error) {
	rows, err := s.queries.CountScholarshipsByCountry(ctx)
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
// promoted scholarship projection row.
func (s *Store) CreateContent(ctx context.Context, arg gen.CreateContentParams) (gen.CreateContentRow, error) {
	row, err := s.queries.CreateContent(ctx, arg)
	if err != nil {
		return gen.CreateContentRow{}, fmt.Errorf("stores: create content: %w", err)
	}
	return row, nil
}
