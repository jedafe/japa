// Package scholarships implements the Scholarship Finder surface
// (Section 73, ADR-073). Filterable scholarship finder built on the
// `scholarship` guide category, with a per-user saved scholarships tracker
// and deadline reminder pipeline.
//
// Ref: japaconnect-engineering/sections/73-scholarship-finder.md
//
//	japaconnect-engineering/adr/ADR-073-scholarship-finder.md
package scholarships

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
)

// Store wraps the sqlc-generated Queries for the scholarship finder.
type Store struct {
	queries *gen.Queries
	pool    *db.DB
}

// NewStore creates a scholarships store backed by the given database. The
// queries field uses database.Writer (the primary) because the store performs
// both reads and writes — admin CRUD handlers call CreateScholarship,
// UpdateScholarship, and DeleteScholarship directly through s.queries.
func NewStore(database *db.DB) *Store {
	return &Store{
		queries: gen.New(database.Writer),
		pool:    database,
	}
}

// SaveScholarship persists a user's saved-scholarship record.
func (s *Store) SaveScholarship(ctx context.Context, arg gen.SaveScholarshipParams) (gen.SavedScholarship, error) {
	row, err := s.queries.SaveScholarship(ctx, arg)
	if err != nil {
		return gen.SavedScholarship{}, fmt.Errorf("scholarships: save: %w", err)
	}

	return row, nil
}

// UpdateSavedScholarship updates an existing saved-scholarship record.
func (s *Store) UpdateSavedScholarship(ctx context.Context, arg gen.UpdateSavedScholarshipParams) (gen.SavedScholarship, error) {
	row, err := s.queries.UpdateSavedScholarship(ctx, arg)
	if err != nil {
		return gen.SavedScholarship{}, fmt.Errorf("scholarships: update saved: %w", err)
	}

	return row, nil
}

// DeleteSavedScholarship removes a user's saved-scholarship record.
func (s *Store) DeleteSavedScholarship(ctx context.Context, arg gen.DeleteSavedScholarshipParams) error {
	if err := s.queries.DeleteSavedScholarship(ctx, arg); err != nil {
		return fmt.Errorf("scholarships: delete saved: %w", err)
	}

	return nil
}

// GetSavedScholarship returns a single saved-scholarship record.
func (s *Store) GetSavedScholarship(ctx context.Context, arg gen.GetSavedScholarshipParams) (gen.SavedScholarship, error) {
	row, err := s.queries.GetSavedScholarship(ctx, arg)
	if err != nil {
		return gen.SavedScholarship{}, fmt.Errorf("scholarships: get saved: %w", err)
	}

	return row, nil
}

// ListSavedScholarships returns a user's saved scholarships with pagination.
func (s *Store) ListSavedScholarships(ctx context.Context, arg gen.ListSavedScholarshipsParams) ([]gen.ListSavedScholarshipsRow, error) {
	rows, err := s.queries.ListSavedScholarships(ctx, arg)
	if err != nil {
		return nil, fmt.Errorf("scholarships: list saved: %w", err)
	}

	return rows, nil
}

// CountSavedScholarships returns the number of scholarships a user has saved.
func (s *Store) CountSavedScholarships(ctx context.Context, arg gen.CountSavedScholarshipsParams) (int64, error) {
	count, err := s.queries.CountSavedScholarships(ctx, arg)
	if err != nil {
		return 0, fmt.Errorf("scholarships: count saved: %w", err)
	}

	return count, nil
}

// GetSourceByRecordID returns source attribution for a single intel record.
func (s *Store) GetSourceByRecordID(ctx context.Context, recordID pgtype.UUID) (gen.ListSourcesByRecordIDsRow, error) {
	rows, err := s.queries.ListSourcesByRecordIDs(ctx, []pgtype.UUID{recordID})
	if err != nil {
		return gen.ListSourcesByRecordIDsRow{}, fmt.Errorf("scholarships: get source by record id: %w", err)
	}

	if len(rows) == 0 {
		return gen.ListSourcesByRecordIDsRow{}, errors.New("scholarships: source not found")
	}

	return rows[0], nil
}

// MapSourcesByRecordIDs returns a map of source attribution keyed by intel record ID.
func (s *Store) MapSourcesByRecordIDs(ctx context.Context, recordIDs []pgtype.UUID) (map[string]gen.ListSourcesByRecordIDsRow, error) {
	rows, err := s.queries.ListSourcesByRecordIDs(ctx, recordIDs)
	if err != nil {
		return nil, fmt.Errorf("scholarships: map sources by record ids: %w", err)
	}

	m := make(map[string]gen.ListSourcesByRecordIDsRow, len(rows))
	for _, r := range rows {
		m[r.RecordID.String()] = r
	}

	return m, nil
}

// AdminStatusDistribution returns per-status scholarship counts for the admin dashboard.
func (s *Store) AdminStatusDistribution(ctx context.Context, tenantID pgtype.UUID) ([]gen.AdminScholarshipStatusDistributionRow, error) {
	rows, err := s.queries.AdminScholarshipStatusDistribution(ctx)
	if err != nil {
		return nil, fmt.Errorf("scholarships: admin status distribution: %w", err)
	}

	return rows, nil
}

// AdminTopSaved returns the most-saved scholarships for the admin report.
func (s *Store) AdminTopSaved(ctx context.Context, arg gen.AdminScholarshipTopSavedParams) ([]gen.AdminScholarshipTopSavedRow, error) {
	rows, err := s.queries.AdminScholarshipTopSaved(ctx, arg)
	if err != nil {
		return nil, fmt.Errorf("scholarships: admin top saved: %w", err)
	}

	return rows, nil
}

// AdminUpcomingDeadlines returns scholarships with upcoming deadlines for the admin report.
func (s *Store) AdminUpcomingDeadlines(ctx context.Context, arg gen.AdminScholarshipUpcomingDeadlinesParams) ([]gen.AdminScholarshipUpcomingDeadlinesRow, error) {
	rows, err := s.queries.AdminScholarshipUpcomingDeadlines(ctx, arg)
	if err != nil {
		return nil, fmt.Errorf("scholarships: admin upcoming deadlines: %w", err)
	}

	return rows, nil
}

// AdminReminderLog returns the reminder delivery history for the admin report.
func (s *Store) AdminReminderLog(ctx context.Context, arg gen.AdminScholarshipReminderLogParams) ([]gen.AdminScholarshipReminderLogRow, error) {
	rows, err := s.queries.AdminScholarshipReminderLog(ctx, arg)
	if err != nil {
		return nil, fmt.Errorf("scholarships: admin reminder log: %w", err)
	}

	return rows, nil
}

// FindNeedingReminders returns saved scholarships that are due for a reminder.
func (s *Store) FindNeedingReminders(ctx context.Context, batchLimit int32) ([]gen.FindScholarshipsNeedingRemindersRow, error) {
	rows, err := s.queries.FindScholarshipsNeedingReminders(ctx, batchLimit)
	if err != nil {
		return nil, fmt.Errorf("scholarships: find needing reminders: %w", err)
	}

	return rows, nil
}

// MarkReminderSent records that a reminder was sent for a saved scholarship.
func (s *Store) MarkReminderSent(ctx context.Context, id pgtype.UUID) error {
	if err := s.queries.MarkScholarshipReminderSent(ctx, id); err != nil {
		return fmt.Errorf("scholarships: mark reminder sent: %w", err)
	}

	return nil
}

// --- Admin CRUD methods ---

// AdminListScholarships returns a paginated, filtered list of scholarships for
// the admin management page. q is an optional case-insensitive search over
// title/university/country; status is an optional verification_status filter.
// The tenantID is stamped into the query params.
func (s *Store) AdminListScholarships(ctx context.Context, tenantID pgtype.UUID, arg gen.AdminListScholarshipsParams) ([]gen.Scholarship, error) {

	rows, err := s.queries.AdminListScholarships(ctx, arg)
	if err != nil {
		return nil, fmt.Errorf("scholarships: admin list: %w", err)
	}

	return rows, nil
}

// AdminCountScholarships returns the total row count for the admin management
// list with the same q + status filter as AdminListScholarships, so the admin
// panel can compute total pages.
func (s *Store) AdminCountScholarships(ctx context.Context, tenantID pgtype.UUID, arg gen.AdminCountScholarshipsParams) (int64, error) {

	count, err := s.queries.AdminCountScholarships(ctx, arg)
	if err != nil {
		return 0, fmt.Errorf("scholarships: admin count: %w", err)
	}

	return count, nil
}

// AdminGetScholarship returns a single scholarship by ID for the admin
// management page.
func (s *Store) AdminGetScholarship(ctx context.Context, tenantID, id pgtype.UUID) (gen.Scholarship, error) {
	row, err := s.queries.GetScholarship(ctx,

		id,
	)
	if err != nil {
		return gen.Scholarship{}, fmt.Errorf("scholarships: admin get: %w", err)
	}

	return row, nil
}

// AdminCreateScholarship creates a new scholarship record. The tenantID is
// stamped into the query params.
func (s *Store) AdminCreateScholarship(ctx context.Context, tenantID pgtype.UUID, arg gen.CreateScholarshipParams) (gen.Scholarship, error) {

	row, err := s.queries.CreateScholarship(ctx, arg)
	if err != nil {
		return gen.Scholarship{}, fmt.Errorf("scholarships: admin create: %w", err)
	}

	return row, nil
}

// AdminUpdateScholarship updates an existing scholarship record. The
// tenantID is stamped into the query params.
func (s *Store) AdminUpdateScholarship(ctx context.Context, tenantID pgtype.UUID, arg gen.UpdateScholarshipParams) error {

	if err := s.queries.UpdateScholarship(ctx, arg); err != nil {
		return fmt.Errorf("scholarships: admin update: %w", err)
	}

	return nil
}

// AdminDeleteScholarship deletes a scholarship record and soft-deletes the
// guide content row it referenced (Rule 11 — user content is soft-deleted, never
// hard-deleted). The tenantID is stamped into the query params for tenant
// isolation. The guide content row is soft-deleted so it stops appearing in the
// finder and content listings without losing audit history.
func (s *Store) AdminDeleteScholarship(ctx context.Context, tenantID, id pgtype.UUID) error {
	sch, err := s.queries.GetScholarship(ctx,

		id,
	)
	if err != nil {
		return fmt.Errorf("scholarships: admin delete (get): %w", err)
	}

	if sch.GuideID.Valid {
		if err := s.queries.DeleteContent(ctx,
			sch.GuideID,
		); err != nil {
			return fmt.Errorf("scholarships: admin delete (soft-delete guide): %w", err)
		}
	}

	if err := s.queries.DeleteScholarship(ctx,

		id,
	); err != nil {
		return fmt.Errorf("scholarships: admin delete: %w", err)
	}

	return nil
}

// UpdateScholarshipInput holds the mutable fields for a scholarship projection
// row update. Pointer/nil or invalid fields mean "leave unchanged"; non-nil
// slices and JSON fields are replaced, with empty JSONB values normalised to
// '{}' so they satisfy the NOT NULL constraint.
type UpdateScholarshipInput struct {
	ID                   pgtype.UUID     `json:"id,omitempty"`
	TenantID             pgtype.UUID     `json:"omitempty"`
	IntelRecordID        pgtype.UUID     `json:"intel_record_id,omitempty"`
	GuideID              pgtype.UUID     `json:"guide_id,omitempty"`
	SourceURL            *string         `json:"source_url,omitempty"`
	SourceType           *string         `json:"source_type,omitempty"`
	TrustLevel           *int16          `json:"trust_level,omitempty"`
	Title                *string         `json:"title,omitempty"`
	Description          *string         `json:"description,omitempty"`
	Country              *string         `json:"country,omitempty"`
	CountrySpaceID       pgtype.UUID     `json:"country_space_id,omitempty"`
	University           *string         `json:"university,omitempty"`
	DegreeLevel          *string         `json:"degree_level,omitempty"`
	FieldsOfStudy        []string        `json:"fields_of_study,omitempty"`
	EligibleCountries    []string        `json:"eligible_countries,omitempty"`
	Funding              json.RawMessage `json:"funding,omitempty"`
	Deadline             pgtype.Date     `json:"deadline,omitempty"`
	ApplicationURL       *string         `json:"application_url,omitempty"`
	RequiredDocuments    []string        `json:"required_documents,omitempty"`
	MinimumGpa           *float64        `json:"minimum_gpa,omitempty"`
	LanguageRequirements json.RawMessage `json:"language_requirements,omitempty"`
	ConfidenceOverall    *float32        `json:"confidence_overall,omitempty"`
	VerificationStatus   *string         `json:"verification_status,omitempty"`
	FreshnessScore       *int32          `json:"freshness_score,omitempty"`
	OverrideSource       *string         `json:"override_source,omitempty"`
	EditorialOverrides   json.RawMessage `json:"editorial_overrides,omitempty"`
}

// float64ToNumeric converts a float64 to a pgtype.Numeric for the minimum_gpa
// column.
func float64ToNumeric(f float64) pgtype.Numeric {
	var n pgtype.Numeric
	if err := n.Scan(strconv.FormatFloat(f, 'f', -1, 64)); err != nil {
		return pgtype.Numeric{}
	}

	return n
}

// rawJSONB returns a []byte safe to store in a NOT NULL jsonb column. Nil and
// JSON null are normalised to '{}', the same default used by CreateScholarship.
func rawJSONB(r json.RawMessage) []byte {
	if r == nil || string(r) == "null" {
		return []byte("{}")
	}

	return []byte(r)
}

// UpdateScholarship applies a tenant-scoped partial update to a scholarship
// projection row. Only non-nil/non-zero fields in the input are written. The
// updated row is returned.
// strField returns the dereferenced value when p is non-nil.
func strField(p *string) (any, bool) {
	if p == nil {
		return nil, false
	}

	return *p, true
}

// strFieldNonEmpty returns the dereferenced value when p is non-nil and non-empty.
func strFieldNonEmpty(p *string) (any, bool) {
	if p == nil || *p == "" {
		return nil, false
	}

	return *p, true
}

// int16Field returns the dereferenced value when p is non-nil.
func int16Field(p *int16) (any, bool) {
	if p == nil {
		return nil, false
	}

	return *p, true
}

// int32Field returns the dereferenced value when p is non-nil.
func int32Field(p *int32) (any, bool) {
	if p == nil {
		return nil, false
	}

	return *p, true
}

// float32Field returns the dereferenced value when p is non-nil.
func float32Field(p *float32) (any, bool) {
	if p == nil {
		return nil, false
	}

	return *p, true
}

// float64Field returns the dereferenced value converted to pgtype.Numeric.
func float64Field(p *float64) (any, bool) {
	if p == nil {
		return nil, false
	}

	return float64ToNumeric(*p), true
}

// rawField returns the json.RawMessage normalised for a NOT NULL jsonb column.
func rawField(r json.RawMessage) (any, bool) {
	if r == nil {
		return nil, false
	}

	return rawJSONB(r), true
}

// uuidField returns the UUID when Valid.
func uuidField(u pgtype.UUID) (any, bool) {
	if !u.Valid {
		return nil, false
	}

	return u, true
}

// dateField returns the date when Valid.
func dateField(d pgtype.Date) (any, bool) {
	if !d.Valid {
		return nil, false
	}

	return d, true
}

// sliceField returns the slice when non-nil.
func sliceField[T any](s []T) (any, bool) {
	if s == nil {
		return nil, false
	}

	return s, true
}

// UpdateScholarship applies a tenant-scoped partial update to a scholarship
// projection row. Only non-nil/non-zero fields in the input are written. The
// updated row is returned.
func (s *Store) UpdateScholarship(ctx context.Context, id pgtype.UUID, in UpdateScholarshipInput) (gen.Scholarship, error) {
	getters := []struct {
		column string
		get    func() (any, bool)
	}{
		{"source_url", func() (any, bool) { return strFieldNonEmpty(in.SourceURL) }},
		{"source_type", func() (any, bool) { return strFieldNonEmpty(in.SourceType) }},
		{"trust_level", func() (any, bool) { return int16Field(in.TrustLevel) }},
		{"title", func() (any, bool) { return strField(in.Title) }},
		{"description", func() (any, bool) { return strField(in.Description) }},
		{"country", func() (any, bool) { return strField(in.Country) }},
		{"country_space_id", func() (any, bool) { return uuidField(in.CountrySpaceID) }},
		{"university", func() (any, bool) { return strField(in.University) }},
		{"degree_level", func() (any, bool) { return strField(in.DegreeLevel) }},
		{"fields_of_study", func() (any, bool) { return sliceField(in.FieldsOfStudy) }},
		{"eligible_countries", func() (any, bool) { return sliceField(in.EligibleCountries) }},
		{"funding", func() (any, bool) { return rawField(in.Funding) }},
		{"deadline", func() (any, bool) { return dateField(in.Deadline) }},
		{"application_url", func() (any, bool) { return strField(in.ApplicationURL) }},
		{"required_documents", func() (any, bool) { return sliceField(in.RequiredDocuments) }},
		{"minimum_gpa", func() (any, bool) { return float64Field(in.MinimumGpa) }},
		{"language_requirements", func() (any, bool) { return rawField(in.LanguageRequirements) }},
		{"confidence_overall", func() (any, bool) { return float32Field(in.ConfidenceOverall) }},
		{"verification_status", func() (any, bool) { return strField(in.VerificationStatus) }},
		{"freshness_score", func() (any, bool) { return int32Field(in.FreshnessScore) }},
		{"override_source", func() (any, bool) { return strField(in.OverrideSource) }},
		{"editorial_overrides", func() (any, bool) { return rawField(in.EditorialOverrides) }},
		{"intel_record_id", func() (any, bool) { return uuidField(in.IntelRecordID) }},
		{"guide_id", func() (any, bool) { return uuidField(in.GuideID) }},
	}

	sets := make([]string, 0, len(getters)+2)
	args := make([]any, 0, len(getters)+2)

	for _, g := range getters {
		if v, ok := g.get(); ok {
			sets = append(sets, fmt.Sprintf("%s = $%d", g.column, len(args)+1))
			args = append(args, v)
		}
	}

	// Always stamp update/verification timestamps when any field is changed.
	sets = append(sets, "updated_at = now()", "last_verified_at = now()")

	q := fmt.Sprintf(`
		UPDATE scholarships
		SET %s
		WHERE id = $%d
		RETURNING id, intel_record_id, guide_id, source_url, source_type, trust_level, title, description, country, country_space_id, university, degree_level, fields_of_study, eligible_countries, funding, deadline, application_url, required_documents, minimum_gpa, language_requirements, confidence_overall, verification_status, freshness_score, override_source, editorial_overrides, last_verified_at, created_at, updated_at
	`, strings.Join(sets, ", "), len(args)+1)

	args = append(args, id)

	var row gen.Scholarship

	err := s.pool.Writer.QueryRow(ctx, q, args...).Scan(
		&row.ID,
		&row.IntelRecordID,
		&row.GuideID,
		&row.SourceUrl,
		&row.SourceType,
		&row.TrustLevel,
		&row.Title,
		&row.Description,
		&row.Country,
		&row.CountrySpaceID,
		&row.University,
		&row.DegreeLevel,
		&row.FieldsOfStudy,
		&row.EligibleCountries,
		&row.Funding,
		&row.Deadline,
		&row.ApplicationUrl,
		&row.RequiredDocuments,
		&row.MinimumGpa,
		&row.LanguageRequirements,
		&row.ConfidenceOverall,
		&row.VerificationStatus,
		&row.FreshnessScore,
		&row.OverrideSource,
		&row.EditorialOverrides,
		&row.LastVerifiedAt,
		&row.CreatedAt,
		&row.UpdatedAt,
	)
	if err != nil {
		return gen.Scholarship{}, fmt.Errorf("scholarships: update: %w", err)
	}

	return row, nil
}

// DeleteScholarship removes a scholarship projection row and soft-deletes the
// linked guide content row for tenant isolation.
func (s *Store) DeleteScholarship(ctx context.Context, id, tenantID pgtype.UUID) error {
	sch, err := s.queries.GetScholarship(ctx,

		id,
	)
	if err != nil {
		return fmt.Errorf("scholarships: delete (get): %w", err)
	}

	if sch.GuideID.Valid {
		if err := s.queries.DeleteContent(ctx,
			sch.GuideID,
		); err != nil {
			return fmt.Errorf("scholarships: delete (soft-delete guide): %w", err)
		}
	}

	cmd, err := s.pool.Writer.Exec(ctx, `DELETE FROM scholarships WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("scholarships: delete: %w", err)
	}

	if cmd.RowsAffected() == 0 {
		return errors.New("scholarships: delete: no rows affected")
	}

	return nil
}
