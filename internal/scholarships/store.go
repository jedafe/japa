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

func (s *Store) SaveScholarship(ctx context.Context, arg gen.SaveScholarshipParams) (gen.SavedScholarship, error) {
	row, err := s.queries.SaveScholarship(ctx, arg)
	if err != nil {
		return gen.SavedScholarship{}, fmt.Errorf("scholarships: save: %w", err)
	}
	return row, nil
}

func (s *Store) UpdateSavedScholarship(ctx context.Context, arg gen.UpdateSavedScholarshipParams) (gen.SavedScholarship, error) {
	row, err := s.queries.UpdateSavedScholarship(ctx, arg)
	if err != nil {
		return gen.SavedScholarship{}, fmt.Errorf("scholarships: update saved: %w", err)
	}
	return row, nil
}

func (s *Store) DeleteSavedScholarship(ctx context.Context, arg gen.DeleteSavedScholarshipParams) error {
	if err := s.queries.DeleteSavedScholarship(ctx, arg); err != nil {
		return fmt.Errorf("scholarships: delete saved: %w", err)
	}
	return nil
}

func (s *Store) GetSavedScholarship(ctx context.Context, arg gen.GetSavedScholarshipParams) (gen.SavedScholarship, error) {
	row, err := s.queries.GetSavedScholarship(ctx, arg)
	if err != nil {
		return gen.SavedScholarship{}, fmt.Errorf("scholarships: get saved: %w", err)
	}
	return row, nil
}

func (s *Store) ListSavedScholarships(ctx context.Context, arg gen.ListSavedScholarshipsParams) ([]gen.ListSavedScholarshipsRow, error) {
	rows, err := s.queries.ListSavedScholarships(ctx, arg)
	if err != nil {
		return nil, fmt.Errorf("scholarships: list saved: %w", err)
	}
	return rows, nil
}

func (s *Store) CountSavedScholarships(ctx context.Context, arg gen.CountSavedScholarshipsParams) (int64, error) {
	count, err := s.queries.CountSavedScholarships(ctx, arg)
	if err != nil {
		return 0, fmt.Errorf("scholarships: count saved: %w", err)
	}
	return count, nil
}

func (s *Store) AdminStatusDistribution(ctx context.Context, tenantID pgtype.UUID) ([]gen.AdminScholarshipStatusDistributionRow, error) {
	rows, err := s.queries.AdminScholarshipStatusDistribution(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("scholarships: admin status distribution: %w", err)
	}
	return rows, nil
}

func (s *Store) AdminTopSaved(ctx context.Context, arg gen.AdminScholarshipTopSavedParams) ([]gen.AdminScholarshipTopSavedRow, error) {
	rows, err := s.queries.AdminScholarshipTopSaved(ctx, arg)
	if err != nil {
		return nil, fmt.Errorf("scholarships: admin top saved: %w", err)
	}
	return rows, nil
}

func (s *Store) AdminUpcomingDeadlines(ctx context.Context, arg gen.AdminScholarshipUpcomingDeadlinesParams) ([]gen.AdminScholarshipUpcomingDeadlinesRow, error) {
	rows, err := s.queries.AdminScholarshipUpcomingDeadlines(ctx, arg)
	if err != nil {
		return nil, fmt.Errorf("scholarships: admin upcoming deadlines: %w", err)
	}
	return rows, nil
}

func (s *Store) AdminReminderLog(ctx context.Context, arg gen.AdminScholarshipReminderLogParams) ([]gen.AdminScholarshipReminderLogRow, error) {
	rows, err := s.queries.AdminScholarshipReminderLog(ctx, arg)
	if err != nil {
		return nil, fmt.Errorf("scholarships: admin reminder log: %w", err)
	}
	return rows, nil
}

func (s *Store) FindNeedingReminders(ctx context.Context, arg gen.FindScholarshipsNeedingRemindersParams) ([]gen.FindScholarshipsNeedingRemindersRow, error) {
	rows, err := s.queries.FindScholarshipsNeedingReminders(ctx, arg)
	if err != nil {
		return nil, fmt.Errorf("scholarships: find needing reminders: %w", err)
	}
	return rows, nil
}

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
	arg.TenantID = tenantID
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
	arg.TenantID = tenantID
	count, err := s.queries.AdminCountScholarships(ctx, arg)
	if err != nil {
		return 0, fmt.Errorf("scholarships: admin count: %w", err)
	}
	return count, nil
}

// AdminGetScholarship returns a single scholarship by ID for the admin
// management page.
func (s *Store) AdminGetScholarship(ctx context.Context, tenantID, id pgtype.UUID) (gen.Scholarship, error) {
	row, err := s.queries.GetScholarship(ctx, gen.GetScholarshipParams{
		TenantID: tenantID,
		ID:       id,
	})
	if err != nil {
		return gen.Scholarship{}, fmt.Errorf("scholarships: admin get: %w", err)
	}
	return row, nil
}

// AdminCreateScholarship creates a new scholarship record. The tenantID is
// stamped into the query params.
func (s *Store) AdminCreateScholarship(ctx context.Context, tenantID pgtype.UUID, arg gen.CreateScholarshipParams) (gen.Scholarship, error) {
	arg.TenantID = tenantID
	row, err := s.queries.CreateScholarship(ctx, arg)
	if err != nil {
		return gen.Scholarship{}, fmt.Errorf("scholarships: admin create: %w", err)
	}
	return row, nil
}

// AdminUpdateScholarship updates an existing scholarship record. The
// tenantID is stamped into the query params.
func (s *Store) AdminUpdateScholarship(ctx context.Context, tenantID pgtype.UUID, arg gen.UpdateScholarshipParams) error {
	arg.TenantID = tenantID
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
	sch, err := s.queries.GetScholarship(ctx, gen.GetScholarshipParams{
		TenantID: tenantID,
		ID:       id,
	})
	if err != nil {
		return fmt.Errorf("scholarships: admin delete (get): %w", err)
	}
	if sch.GuideID.Valid {
		if err := s.queries.DeleteContent(ctx, gen.DeleteContentParams{
			ID:       sch.GuideID,
			TenantID: tenantID,
		}); err != nil {
			return fmt.Errorf("scholarships: admin delete (soft-delete guide): %w", err)
		}
	}
	if err := s.queries.DeleteScholarship(ctx, gen.DeleteScholarshipParams{
		TenantID: tenantID,
		ID:       id,
	}); err != nil {
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
	TenantID             pgtype.UUID     `json:"tenant_id,omitempty"`
	IntelRecordID        pgtype.UUID     `json:"intel_record_id,omitempty"`
	GuideID              pgtype.UUID     `json:"guide_id,omitempty"`
	SourceUrl            *string         `json:"source_url,omitempty"`
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
	ApplicationUrl       *string         `json:"application_url,omitempty"`
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
func (s *Store) UpdateScholarship(ctx context.Context, id, tenantID pgtype.UUID, in UpdateScholarshipInput) (gen.Scholarship, error) {
	var (
		sets   []string
		args   []any
		argIdx = 1
	)

	if in.SourceUrl != nil && *in.SourceUrl != "" {
		sets = append(sets, fmt.Sprintf("source_url = $%d", argIdx))
		args = append(args, *in.SourceUrl)
		argIdx++
	}
	if in.SourceType != nil && *in.SourceType != "" {
		sets = append(sets, fmt.Sprintf("source_type = $%d", argIdx))
		args = append(args, *in.SourceType)
		argIdx++
	}
	if in.TrustLevel != nil {
		sets = append(sets, fmt.Sprintf("trust_level = $%d", argIdx))
		args = append(args, *in.TrustLevel)
		argIdx++
	}
	if in.Title != nil {
		sets = append(sets, fmt.Sprintf("title = $%d", argIdx))
		args = append(args, *in.Title)
		argIdx++
	}
	if in.Description != nil {
		sets = append(sets, fmt.Sprintf("description = $%d", argIdx))
		args = append(args, *in.Description)
		argIdx++
	}
	if in.Country != nil {
		sets = append(sets, fmt.Sprintf("country = $%d", argIdx))
		args = append(args, *in.Country)
		argIdx++
	}
	if in.CountrySpaceID.Valid {
		sets = append(sets, fmt.Sprintf("country_space_id = $%d", argIdx))
		args = append(args, in.CountrySpaceID)
		argIdx++
	}
	if in.University != nil {
		sets = append(sets, fmt.Sprintf("university = $%d", argIdx))
		args = append(args, *in.University)
		argIdx++
	}
	if in.DegreeLevel != nil {
		sets = append(sets, fmt.Sprintf("degree_level = $%d", argIdx))
		args = append(args, *in.DegreeLevel)
		argIdx++
	}
	if in.FieldsOfStudy != nil {
		sets = append(sets, fmt.Sprintf("fields_of_study = $%d", argIdx))
		args = append(args, in.FieldsOfStudy)
		argIdx++
	}
	if in.EligibleCountries != nil {
		sets = append(sets, fmt.Sprintf("eligible_countries = $%d", argIdx))
		args = append(args, in.EligibleCountries)
		argIdx++
	}
	if in.Funding != nil {
		sets = append(sets, fmt.Sprintf("funding = $%d", argIdx))
		args = append(args, rawJSONB(in.Funding))
		argIdx++
	}
	if in.Deadline.Valid {
		sets = append(sets, fmt.Sprintf("deadline = $%d", argIdx))
		args = append(args, in.Deadline)
		argIdx++
	}
	if in.ApplicationUrl != nil {
		sets = append(sets, fmt.Sprintf("application_url = $%d", argIdx))
		args = append(args, *in.ApplicationUrl)
		argIdx++
	}
	if in.RequiredDocuments != nil {
		sets = append(sets, fmt.Sprintf("required_documents = $%d", argIdx))
		args = append(args, in.RequiredDocuments)
		argIdx++
	}
	if in.MinimumGpa != nil {
		sets = append(sets, fmt.Sprintf("minimum_gpa = $%d", argIdx))
		args = append(args, float64ToNumeric(*in.MinimumGpa))
		argIdx++
	}
	if in.LanguageRequirements != nil {
		sets = append(sets, fmt.Sprintf("language_requirements = $%d", argIdx))
		args = append(args, rawJSONB(in.LanguageRequirements))
		argIdx++
	}
	if in.ConfidenceOverall != nil {
		sets = append(sets, fmt.Sprintf("confidence_overall = $%d", argIdx))
		args = append(args, *in.ConfidenceOverall)
		argIdx++
	}
	if in.VerificationStatus != nil {
		sets = append(sets, fmt.Sprintf("verification_status = $%d", argIdx))
		args = append(args, *in.VerificationStatus)
		argIdx++
	}
	if in.FreshnessScore != nil {
		sets = append(sets, fmt.Sprintf("freshness_score = $%d", argIdx))
		args = append(args, *in.FreshnessScore)
		argIdx++
	}
	if in.OverrideSource != nil {
		sets = append(sets, fmt.Sprintf("override_source = $%d", argIdx))
		args = append(args, *in.OverrideSource)
		argIdx++
	}
	if in.EditorialOverrides != nil {
		sets = append(sets, fmt.Sprintf("editorial_overrides = $%d", argIdx))
		args = append(args, rawJSONB(in.EditorialOverrides))
		argIdx++
	}
	if in.IntelRecordID.Valid {
		sets = append(sets, fmt.Sprintf("intel_record_id = $%d", argIdx))
		args = append(args, in.IntelRecordID)
		argIdx++
	}
	if in.GuideID.Valid {
		sets = append(sets, fmt.Sprintf("guide_id = $%d", argIdx))
		args = append(args, in.GuideID)
		argIdx++
	}

	// Always stamp update/verification timestamps when any field is changed.
	sets = append(sets, "updated_at = now()", "last_verified_at = now()")

	q := fmt.Sprintf(`
		UPDATE scholarships
		SET %s
		WHERE id = $%d AND tenant_id = $%d
		RETURNING id, intel_record_id, guide_id, source_url, source_type, trust_level, title, description, country, country_space_id, university, degree_level, fields_of_study, eligible_countries, funding, deadline, application_url, required_documents, minimum_gpa, language_requirements, confidence_overall, verification_status, freshness_score, override_source, editorial_overrides, last_verified_at, created_at, updated_at, tenant_id
	`, strings.Join(sets, ", "), argIdx, argIdx+1)
	args = append(args, id, tenantID)

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
		&row.TenantID,
	)
	if err != nil {
		return gen.Scholarship{}, fmt.Errorf("scholarships: update: %w", err)
	}
	return row, nil
}

// DeleteScholarship removes a scholarship projection row and soft-deletes the
// linked guide content row for tenant isolation.
func (s *Store) DeleteScholarship(ctx context.Context, id, tenantID pgtype.UUID) error {
	sch, err := s.queries.GetScholarship(ctx, gen.GetScholarshipParams{
		TenantID: tenantID,
		ID:       id,
	})
	if err != nil {
		return fmt.Errorf("scholarships: delete (get): %w", err)
	}
	if sch.GuideID.Valid {
		if err := s.queries.DeleteContent(ctx, gen.DeleteContentParams{
			ID:       sch.GuideID,
			TenantID: tenantID,
		}); err != nil {
			return fmt.Errorf("scholarships: delete (soft-delete guide): %w", err)
		}
	}

	cmd, err := s.pool.Writer.Exec(ctx, `DELETE FROM scholarships WHERE id = $1 AND tenant_id = $2`, id, tenantID)
	if err != nil {
		return fmt.Errorf("scholarships: delete: %w", err)
	}
	if cmd.RowsAffected() == 0 {
		return fmt.Errorf("scholarships: delete: no rows affected")
	}
	return nil
}
