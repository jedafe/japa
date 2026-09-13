// Package checklist implements the Visa Document Checklist Generator
// (Section 70, ADR-070). Checklist items are extracted from the
// required_documents custom field on published visa guides — no parallel
// document data store. Per-user progress is persisted in checklist_progress.
// // Ref: japaconnect-engineering/sections/70-visa-checklist.md
// //	japaconnect-engineering/adr/ADR-070-visa-document-checklist-generator.md
package checklist

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
)

// Store wraps the sqlc-generated Queries for checklist_progress operations
// and provides direct pgx queries against contents + custom_field_values for
// the visa-guide selector and item extraction. It is the data-access layer —
// no business logic here.
type Store struct {
	queries *gen.Queries
	pool    *db.DB
}

// NewStore creates a checklist store backed by the given database.
func NewStore(database *db.DB) *Store {
	return &Store{
		queries: gen.New(database.Reader),
		pool:    database,
	}
}

// VisaGuideField holds a single field value extracted from a published visa
// guide's custom_field_values row.
type VisaGuideField struct {
	GuideID   pgtype.UUID
	FieldKey  string
	ValueText string
	ValueJSON []byte
}

// ListPublishedVisaGuideFields queries published visa guides and their
// custom_field_values for the requested field keys (e.g.
// destination_country, visa_type, required_documents). The query joins
// contents with custom_field_values + custom_fields so tenancy is enforced
// transitively via custom_fields. (BUG-014 pattern).
func (s *Store) ListPublishedVisaGuideFields(ctx context.Context, tenantID pgtype.UUID, fieldKeys []string) ([]VisaGuideField, error) {
	if len(fieldKeys) == 0 {
		return nil, nil
	}

	const q = `
SELECT c.id, cf.key, cfv.value_text, cfv.value_json
FROM contents c
JOIN custom_field_values cfv
  ON cfv.entity_type = 'content' AND cfv.entity_id = c.id
JOIN custom_fields cf
  ON cf.id = cfv.field_id AND cf.tenant_id = c.tenant_id
WHERE c.content_type = 'guide'
  AND c.guide_category = 'visa'
  AND c.status = 'published'
  AND c.deleted_at IS NULL
  AND cf.key = ANY($1)
ORDER BY c.id, cf.key
`

	rows, err := s.pool.Reader.Query(ctx, q, fieldKeys)
	if err != nil {
		return nil, fmt.Errorf("checklist: list visa guide fields: %w", err)
	}
	defer rows.Close()

	var out []VisaGuideField

	for rows.Next() {
		var f VisaGuideField

		var valueText *string
		if err := rows.Scan(&f.GuideID, &f.FieldKey, &valueText, &f.ValueJSON); err != nil {
			return nil, fmt.Errorf("checklist: scan visa guide field: %w", err)
		}

		if valueText != nil {
			f.ValueText = *valueText
		}

		out = append(out, f)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("checklist: iterate visa guide fields: %w", err)
	}

	return out, nil
}

// CountrySpace is a country-level Geo Space (§05) used as an origin selector
// option. Only the fields needed by the selector are returned.
type CountrySpace struct {
	ID   pgtype.UUID
	Name string
	Slug string
}

// ListCountrySpaces returns all country-level Geo Spaces for the tenant,
// ordered by name. Used to populate the origin selector (§70 step 1).
func (s *Store) ListCountrySpaces(ctx context.Context, tenantID pgtype.UUID) ([]CountrySpace, error) {
	const q = `
SELECT id, name, slug
FROM spaces
WHERE level = 'country' AND deleted_at IS NULL
ORDER BY name ASC
`

	rows, err := s.pool.Reader.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("checklist: list country spaces: %w", err)
	}
	defer rows.Close()

	var out []CountrySpace

	for rows.Next() {
		var c CountrySpace
		if err := rows.Scan(&c.ID, &c.Name, &c.Slug); err != nil {
			return nil, fmt.Errorf("checklist: scan country space: %w", err)
		}

		out = append(out, c)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("checklist: iterate country spaces: %w", err)
	}

	return out, nil
}

// GetVisaGuideField returns a single field value for a specific guide + key.
// Used by GetChecklistItems to fetch the required_documents field.
func (s *Store) GetVisaGuideField(ctx context.Context, tenantID, guideID pgtype.UUID, fieldKey string) (VisaGuideField, error) {
	const q = `
SELECT c.id, cf.key, cfv.value_text, cfv.value_json
FROM contents c
JOIN custom_field_values cfv
  ON cfv.entity_type = 'content' AND cfv.entity_id = c.id
JOIN custom_fields cf
  ON cf.id = cfv.field_id AND cf.tenant_id = c.tenant_id
WHERE c.id = $1
  AND c.content_type = 'guide'
  AND c.guide_category = 'visa'
  AND c.status = 'published'
  AND c.deleted_at IS NULL
  AND cf.key = $2
LIMIT 1
`

	row := s.pool.Reader.QueryRow(ctx, q, guideID, fieldKey)

	var f VisaGuideField

	var valueText *string
	if err := row.Scan(&f.GuideID, &f.FieldKey, &valueText, &f.ValueJSON); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return VisaGuideField{}, pgx.ErrNoRows
		}

		return VisaGuideField{}, fmt.Errorf("checklist: get visa guide field: %w", err)
	}

	if valueText != nil {
		f.ValueText = *valueText
	}

	return f, nil
}

// GetVisaGuideTitle returns the title of a published visa guide. Used by
// GetGuide to populate the ChecklistGuideResult title (§70 contract).
func (s *Store) GetVisaGuideTitle(ctx context.Context, tenantID, guideID pgtype.UUID) (string, error) {
	const q = `
SELECT c.title
FROM contents c
WHERE c.id = $1
  AND c.content_type = 'guide'
  AND c.guide_category = 'visa'
  AND c.status = 'published'
  AND c.deleted_at IS NULL
LIMIT 1
`

	var title string
	if err := s.pool.Reader.QueryRow(ctx, q, guideID).Scan(&title); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", pgx.ErrNoRows
		}

		return "", fmt.Errorf("checklist: get visa guide title: %w", err)
	}

	return title, nil
}

// ListChecklistProgress returns all progress rows for a user + guide.
func (s *Store) ListChecklistProgress(ctx context.Context, tenantID, userID, guideID pgtype.UUID) ([]gen.ChecklistProgress, error) {
	return s.queries.ListChecklistProgressByUserGuide(ctx, gen.ListChecklistProgressByUserGuideParams{

		UserID:  userID,
		GuideID: guideID,
	})
}

// GetSingleProgress returns a single progress row for a user + guide +
// item_key. Used by UpsertProgress to resolve the existing completed state
// for a note-only PATCH (where completed is nil in the request body).
func (s *Store) GetSingleProgress(ctx context.Context, tenantID, userID, guideID pgtype.UUID, itemKey string) (gen.ChecklistProgress, error) {
	return s.queries.GetSingleChecklistProgress(ctx, gen.GetSingleChecklistProgressParams{

		UserID:  userID,
		GuideID: guideID,
		ItemKey: itemKey,
	})
}

// UpsertChecklistProgress marks a single item complete/incomplete + note.
func (s *Store) UpsertChecklistProgress(ctx context.Context, arg gen.UpsertChecklistProgressParams) (gen.ChecklistProgress, error) {
	return s.queries.UpsertChecklistProgress(ctx, arg)
}

// DeleteChecklistProgress resets all progress for a user + guide.
func (s *Store) DeleteChecklistProgress(ctx context.Context, tenantID, userID, guideID pgtype.UUID) error {
	return s.queries.DeleteChecklistProgressByUserGuide(ctx, gen.DeleteChecklistProgressByUserGuideParams{

		UserID:  userID,
		GuideID: guideID,
	})
}

// CountCompletedChecklistItems returns the number of completed items for a
// user + guide (used for the full-completion check before awarding points).
func (s *Store) CountCompletedChecklistItems(ctx context.Context, tenantID, userID, guideID pgtype.UUID) (int64, error) {
	return s.queries.CountCompletedChecklistItems(ctx, gen.CountCompletedChecklistItemsParams{

		UserID:  userID,
		GuideID: guideID,
	})
}

// AdminListChecklistProgress lists all users' progress rows (paginated,
// filterable by guide_id and user_id). Admin-only.
func (s *Store) AdminListChecklistProgress(ctx context.Context, arg gen.AdminListChecklistProgressParams) ([]gen.AdminListChecklistProgressRow, error) {
	return s.queries.AdminListChecklistProgress(ctx, arg)
}

// AdminCountChecklistProgress returns the total count for the admin list
// (with the same guide_id/user_id filters applied).
func (s *Store) AdminCountChecklistProgress(ctx context.Context, tenantID, guideID, userID pgtype.UUID) (int64, error) {
	return s.queries.AdminCountChecklistProgress(ctx, gen.AdminCountChecklistProgressParams{

		GuideID: guideID,
		UserID:  userID,
	})
}

// AdminCompletionStats returns per-guide completion aggregates (admin).
func (s *Store) AdminCompletionStats(ctx context.Context, tenantID pgtype.UUID, pageOffset, pageLimit int32) ([]gen.AdminChecklistCompletionStatsRow, error) {
	return s.queries.AdminChecklistCompletionStats(ctx, gen.AdminChecklistCompletionStatsParams{

		PageOffset: pageOffset,
		PageLimit:  pageLimit,
	})
}
