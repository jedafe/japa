// Package visaroutes implements the Visa Route Comparison Tool (§90,
// ADR-098).
//
// Visa routes are records in the visa_routes table. Each route describes
// a visa category for a destination space (country), with eligibility
// summary, requirements (JSON), typical fee, path-to-PR, renewal terms,
// restrictions, and a government source URL. The package provides public
// listing, comparison (up to 4 routes), detail, and admin CRUD with
// HITL approval. AI attribution columns (ai_generated, ai_model,
// ai_prompt_hash) and HITL review columns (hitl_reviewed_by,
// hitl_reviewed_at, hitl_review_notes) follow ADR-088.
//
// Ref: japaconnect-engineering/sections/90-visa-route-comparison.md
//
//	japaconnect-engineering/adr/ADR-098-visa-route-comparison.md
package visaroutes

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/customfields"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
)

// Route statuses (§90).
const (
	StatusDraft     = "draft"
	StatusAIDraft   = "ai_draft"
	StatusPending   = "pending"
	StatusPublished = "published"
	StatusRejected  = "rejected"
)

// Store is the data-access layer for visa_routes. It uses direct pgx
// queries because visa_routes is a new table without sqlc-generated
// queries yet. No business logic here.
type Store struct {
	pool *db.DB
}

// NewStore creates a visa routes store backed by the given database.
func NewStore(database *db.DB) *Store {
	return &Store{pool: database}
}

// ListFieldValuesByRouteIDs returns community field values keyed by route ID
// (entity_type='visa_route') in one batched custom_field_values read — the
// canonical source for the §80 Tool Surface Hydration merge.
func (s *Store) ListFieldValuesByRouteIDs(ctx context.Context, ids []pgtype.UUID) map[pgtype.UUID]map[string]any {
	return customfields.EntityFieldValuesForEntities(ctx, gen.New(s.pool.Reader), "visa_route", ids)
}

// RouteRow is the full row shape returned by list, get-by-id, and
// admin-list queries. Joins spaces (destination) for the destination
// name/geo_code.
type RouteRow struct {
	ID                   pgtype.UUID
	DestinationSpaceID   pgtype.UUID
	DestSpaceName        *string
	DestSpaceCode        *string
	VisaCategory         string
	Name                 string
	Slug                 string
	EligibilitySummary   *string
	RequirementsJSON     []byte
	TypicalFeeUSD        *float64
	PathToPR             *string
	RenewalTerms         *string
	Restrictions         *string
	GovSourceURL         string
	SourceURLLastChecked pgtype.Date
	Status               string
	IsActive             bool
	AIGenerated          bool
	AIModel              *string
	AIPromptHash         *string
	HITLReviewedBy       pgtype.UUID
	HITLReviewedAt       pgtype.Timestamptz
	HITLReviewNotes      *string
	IsJapabotIndexed     bool
	CreatedAt            pgtype.Timestamptz
	UpdatedAt            pgtype.Timestamptz
}

// ListFilter holds the filter parameters for public listing.
type ListFilter struct {
	DestinationSpaceID pgtype.UUID // optional
	Profession         string      // optional: filter requirements_json
	Education          string      // optional: filter requirements_json
	ExperienceYears    int32       // optional: filter requirements_json
	Limit              int32
	Offset             int32
}

// List returns published, active visa routes for the public listing,
// optionally filtered by destination space. Profession/education/
// experience_years are matched against the requirements_json blob via
// ILIKE when provided.
func (s *Store) List(ctx context.Context, f ListFilter) ([]RouteRow, int64, error) {
	var (
		conditions []string
		args       []any
		argIdx     = 1
	)

	conditions = append(conditions, "vr.status = 'published'")
	conditions = append(conditions, "vr.is_active = true")

	if f.DestinationSpaceID.Valid {
		conditions = append(conditions, fmt.Sprintf("vr.destination_space_id = $%d", argIdx))
		args = append(args, f.DestinationSpaceID)
		argIdx++
	}

	if f.Profession != "" {
		conditions = append(conditions, fmt.Sprintf("requirements_json::text ILIKE $%d", argIdx))
		args = append(args, "%\"profession\":\"%"+f.Profession+"%\"%")
		argIdx++
	}

	if f.Education != "" {
		conditions = append(conditions, fmt.Sprintf("requirements_json::text ILIKE $%d", argIdx))
		args = append(args, "%\"education\":\"%"+f.Education+"%\"%")
		argIdx++
	}

	if f.ExperienceYears > 0 {
		// Rows without an experience_years key should match (no minimum
		// requirement). NULL::int <= N is NULL (false), so we explicitly
		// include rows that don't carry the key.
		conditions = append(conditions, fmt.Sprintf(
			"(NOT requirements_json ? 'experience_years' OR (requirements_json->>'experience_years')::int <= $%d)",
			argIdx,
		))

		args = append(args, f.ExperienceYears)
		argIdx++
	}

	whereClause := strings.Join(conditions, " AND ")
	if whereClause == "" {
		whereClause = "TRUE"
	}

	limit := f.Limit
	if limit <= 0 {
		limit = 20
	}

	if limit > 100 {
		limit = 100
	}

	offset := f.Offset
	if offset < 0 {
		offset = 0
	}

	listQuery := fmt.Sprintf(`
SELECT vr.id, vr.destination_space_id,
       ds.name AS dest_space_name, ds.geo_code AS dest_space_code,
       vr.visa_category, vr.name, vr.slug, vr.eligibility_summary,
       vr.requirements_json, vr.typical_fee_usd, vr.path_to_pr,
       vr.renewal_terms, vr.restrictions, vr.gov_source_url,
       vr.source_url_last_checked, vr.status, vr.is_active,
       vr.ai_generated, vr.ai_model, vr.ai_prompt_hash,
       vr.hitl_reviewed_by, vr.hitl_reviewed_at, vr.hitl_review_notes,
       vr.is_japabot_indexed, vr.created_at, vr.updated_at
FROM visa_routes vr
LEFT JOIN spaces ds ON ds.id = vr.destination_space_id
WHERE %s
ORDER BY vr.name ASC
LIMIT $%d OFFSET $%d
`, whereClause, argIdx, argIdx+1)

	args = append(args, limit, offset)

	rows, err := s.pool.Reader.Query(ctx, listQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("visaroutes: list: %w", err)
	}
	defer rows.Close()

	var out []RouteRow

	for rows.Next() {
		var r RouteRow
		if err := rows.Scan(
			&r.ID, &r.DestinationSpaceID,
			&r.DestSpaceName, &r.DestSpaceCode,
			&r.VisaCategory, &r.Name, &r.Slug, &r.EligibilitySummary,
			&r.RequirementsJSON, &r.TypicalFeeUSD, &r.PathToPR,
			&r.RenewalTerms, &r.Restrictions, &r.GovSourceURL,
			&r.SourceURLLastChecked, &r.Status, &r.IsActive,
			&r.AIGenerated, &r.AIModel, &r.AIPromptHash,
			&r.HITLReviewedBy, &r.HITLReviewedAt, &r.HITLReviewNotes,
			&r.IsJapabotIndexed, &r.CreatedAt, &r.UpdatedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("visaroutes: scan route row: %w", err)
		}

		out = append(out, r)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("visaroutes: iterate route rows: %w", err)
	}

	countQuery := fmt.Sprintf(`
SELECT COUNT(*) FROM visa_routes vr WHERE %s
`, whereClause)
	countArgs := args[:len(args)-2]

	var total int64

	if err := s.pool.Reader.QueryRow(ctx, countQuery, countArgs...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("visaroutes: count routes: %w", err)
	}

	return out, total, nil
}

// GetByID returns a single visa route by its primary key. Public callers
// should filter to published+active; admin callers may fetch any row.
func (s *Store) GetByID(ctx context.Context, id pgtype.UUID) (RouteRow, error) {
	const q = `
SELECT vr.id, vr.destination_space_id,
       ds.name AS dest_space_name, ds.geo_code AS dest_space_code,
       vr.visa_category, vr.name, vr.slug, vr.eligibility_summary,
       vr.requirements_json, vr.typical_fee_usd, vr.path_to_pr,
       vr.renewal_terms, vr.restrictions, vr.gov_source_url,
       vr.source_url_last_checked, vr.status, vr.is_active,
       vr.ai_generated, vr.ai_model, vr.ai_prompt_hash,
       vr.hitl_reviewed_by, vr.hitl_reviewed_at, vr.hitl_review_notes,
       vr.is_japabot_indexed, vr.created_at, vr.updated_at
FROM visa_routes vr
LEFT JOIN spaces ds ON ds.id = vr.destination_space_id
WHERE vr.id = $1
LIMIT 1
`

	var r RouteRow

	err := s.pool.Reader.QueryRow(ctx, q, id).Scan(
		&r.ID, &r.DestinationSpaceID,
		&r.DestSpaceName, &r.DestSpaceCode,
		&r.VisaCategory, &r.Name, &r.Slug, &r.EligibilitySummary,
		&r.RequirementsJSON, &r.TypicalFeeUSD, &r.PathToPR,
		&r.RenewalTerms, &r.Restrictions, &r.GovSourceURL,
		&r.SourceURLLastChecked, &r.Status, &r.IsActive,
		&r.AIGenerated, &r.AIModel, &r.AIPromptHash,
		&r.HITLReviewedBy, &r.HITLReviewedAt, &r.HITLReviewNotes,
		&r.IsJapabotIndexed, &r.CreatedAt, &r.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return RouteRow{}, pgx.ErrNoRows
		}

		return RouteRow{}, fmt.Errorf("visaroutes: get by id: %w", err)
	}

	return r, nil
}

// GetBySlug returns a single visa route by its slug. Public callers should
// filter to published+active; admin callers may fetch any row.
func (s *Store) GetBySlug(ctx context.Context, slug string) (RouteRow, error) {
	const q = `
SELECT vr.id, vr.destination_space_id,
       ds.name AS dest_space_name, ds.geo_code AS dest_space_code,
       vr.visa_category, vr.name, vr.slug, vr.eligibility_summary,
       vr.requirements_json, vr.typical_fee_usd, vr.path_to_pr,
       vr.renewal_terms, vr.restrictions, vr.gov_source_url,
       vr.source_url_last_checked, vr.status, vr.is_active,
       vr.ai_generated, vr.ai_model, vr.ai_prompt_hash,
       vr.hitl_reviewed_by, vr.hitl_reviewed_at, vr.hitl_review_notes,
       vr.is_japabot_indexed, vr.created_at, vr.updated_at
FROM visa_routes vr
LEFT JOIN spaces ds ON ds.id = vr.destination_space_id
WHERE vr.slug = $1
LIMIT 1
`

	var r RouteRow

	err := s.pool.Reader.QueryRow(ctx, q, slug).Scan(
		&r.ID, &r.DestinationSpaceID,
		&r.DestSpaceName, &r.DestSpaceCode,
		&r.VisaCategory, &r.Name, &r.Slug, &r.EligibilitySummary,
		&r.RequirementsJSON, &r.TypicalFeeUSD, &r.PathToPR,
		&r.RenewalTerms, &r.Restrictions, &r.GovSourceURL,
		&r.SourceURLLastChecked, &r.Status, &r.IsActive,
		&r.AIGenerated, &r.AIModel, &r.AIPromptHash,
		&r.HITLReviewedBy, &r.HITLReviewedAt, &r.HITLReviewNotes,
		&r.IsJapabotIndexed, &r.CreatedAt, &r.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return RouteRow{}, pgx.ErrNoRows
		}

		return RouteRow{}, fmt.Errorf("visaroutes: get by slug: %w", err)
	}

	return r, nil
}

// GetByIDs returns multiple visa routes by their primary keys (used by
// the compare endpoint). Missing IDs are silently omitted.
func (s *Store) GetByIDs(ctx context.Context, ids []pgtype.UUID) ([]RouteRow, error) {
	if len(ids) == 0 {
		return nil, nil
	}

	args := make([]any, 0, len(ids))
	placeholders := make([]string, 0, len(ids))

	for i, id := range ids {
		args = append(args, id)
		placeholders = append(placeholders, fmt.Sprintf("$%d", i+1))
	}

	q := fmt.Sprintf(`
SELECT vr.id, vr.destination_space_id,
       ds.name AS dest_space_name, ds.geo_code AS dest_space_code,
       vr.visa_category, vr.name, vr.slug, vr.eligibility_summary,
       vr.requirements_json, vr.typical_fee_usd, vr.path_to_pr,
       vr.renewal_terms, vr.restrictions, vr.gov_source_url,
       vr.source_url_last_checked, vr.status, vr.is_active,
       vr.ai_generated, vr.ai_model, vr.ai_prompt_hash,
       vr.hitl_reviewed_by, vr.hitl_reviewed_at, vr.hitl_review_notes,
       vr.is_japabot_indexed, vr.created_at, vr.updated_at
FROM visa_routes vr
LEFT JOIN spaces ds ON ds.id = vr.destination_space_id
WHERE vr.id IN (%s)
  AND vr.status = 'published' AND vr.is_active = true
ORDER BY vr.name ASC
`, strings.Join(placeholders, ", "))

	rows, err := s.pool.Reader.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("visaroutes: get by ids: %w", err)
	}

	defer rows.Close()

	var out []RouteRow

	for rows.Next() {
		var r RouteRow
		if err := rows.Scan(
			&r.ID, &r.DestinationSpaceID,
			&r.DestSpaceName, &r.DestSpaceCode,
			&r.VisaCategory, &r.Name, &r.Slug, &r.EligibilitySummary,
			&r.RequirementsJSON, &r.TypicalFeeUSD, &r.PathToPR,
			&r.RenewalTerms, &r.Restrictions, &r.GovSourceURL,
			&r.SourceURLLastChecked, &r.Status, &r.IsActive,
			&r.AIGenerated, &r.AIModel, &r.AIPromptHash,
			&r.HITLReviewedBy, &r.HITLReviewedAt, &r.HITLReviewNotes,
			&r.IsJapabotIndexed, &r.CreatedAt, &r.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("visaroutes: scan route row (by ids): %w", err)
		}

		out = append(out, r)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("visaroutes: iterate route rows (by ids): %w", err)
	}

	return out, nil
}

// AdminListFilter holds filter parameters for the admin list.
type AdminListFilter struct {
	DestinationSpaceID pgtype.UUID // optional
	Status             string      // optional
	IsActive           *bool       // optional
	VisaCategory       string      // optional
	Q                  string      // optional: search name
	Sort               string      // optional: sort column (name, visa_category, status, created_at, updated_at)
	Order              string      // optional: "asc" or "desc" (default desc)
	Limit              int32
	Offset             int32
}

// AdminList returns all visa routes (any status, active or inactive) for
// the admin management view.
func (s *Store) AdminList(ctx context.Context, f AdminListFilter) ([]RouteRow, int64, error) {
	var (
		conditions []string
		args       []any
		argIdx     = 1
	)

	if f.DestinationSpaceID.Valid {
		conditions = append(conditions, fmt.Sprintf("vr.destination_space_id = $%d", argIdx))
		args = append(args, f.DestinationSpaceID)
		argIdx++
	}

	if f.Status != "" {
		conditions = append(conditions, fmt.Sprintf("vr.status = $%d", argIdx))
		args = append(args, f.Status)
		argIdx++
	}

	if f.IsActive != nil {
		conditions = append(conditions, fmt.Sprintf("vr.is_active = $%d", argIdx))
		args = append(args, *f.IsActive)
		argIdx++
	}

	if f.VisaCategory != "" {
		conditions = append(conditions, fmt.Sprintf("vr.visa_category = $%d", argIdx))
		args = append(args, f.VisaCategory)
		argIdx++
	}

	if f.Q != "" {
		conditions = append(conditions, fmt.Sprintf("vr.name ILIKE $%d", argIdx))
		args = append(args, "%"+f.Q+"%")
		argIdx++
	}

	whereClause := strings.Join(conditions, " AND ")
	if whereClause == "" {
		whereClause = "TRUE"
	}

	limit := f.Limit
	if limit <= 0 {
		limit = 20
	}

	if limit > 200 {
		limit = 200
	}

	offset := f.Offset
	if offset < 0 {
		offset = 0
	}

	// Sort: whitelist of allowed columns to prevent SQL injection via
	// user-supplied sort param (geo-aware-tool-pattern.md admin list).
	// Default: created_at DESC.
	orderClause := "vr.created_at DESC"

	if col, ok := adminSortColumn(f.Sort); ok {
		dir := "DESC"
		if strings.EqualFold(f.Order, "asc") {
			dir = "ASC"
		}

		orderClause = col + " " + dir
	}

	listQuery := fmt.Sprintf(`
SELECT vr.id, vr.destination_space_id,
       ds.name AS dest_space_name, ds.geo_code AS dest_space_code,
       vr.visa_category, vr.name, vr.slug, vr.eligibility_summary,
       vr.requirements_json, vr.typical_fee_usd, vr.path_to_pr,
       vr.renewal_terms, vr.restrictions, vr.gov_source_url,
       vr.source_url_last_checked, vr.status, vr.is_active,
       vr.ai_generated, vr.ai_model, vr.ai_prompt_hash,
       vr.hitl_reviewed_by, vr.hitl_reviewed_at, vr.hitl_review_notes,
       vr.is_japabot_indexed, vr.created_at, vr.updated_at
FROM visa_routes vr
LEFT JOIN spaces ds ON ds.id = vr.destination_space_id
WHERE %s
ORDER BY %s
LIMIT $%d OFFSET $%d
`, whereClause, orderClause, argIdx, argIdx+1)

	args = append(args, limit, offset)

	rows, err := s.pool.Reader.Query(ctx, listQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("visaroutes: admin list: %w", err)
	}
	defer rows.Close()

	var out []RouteRow

	for rows.Next() {
		var r RouteRow
		if err := rows.Scan(
			&r.ID, &r.DestinationSpaceID,
			&r.DestSpaceName, &r.DestSpaceCode,
			&r.VisaCategory, &r.Name, &r.Slug, &r.EligibilitySummary,
			&r.RequirementsJSON, &r.TypicalFeeUSD, &r.PathToPR,
			&r.RenewalTerms, &r.Restrictions, &r.GovSourceURL,
			&r.SourceURLLastChecked, &r.Status, &r.IsActive,
			&r.AIGenerated, &r.AIModel, &r.AIPromptHash,
			&r.HITLReviewedBy, &r.HITLReviewedAt, &r.HITLReviewNotes,
			&r.IsJapabotIndexed, &r.CreatedAt, &r.UpdatedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("visaroutes: admin scan route row: %w", err)
		}

		out = append(out, r)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("visaroutes: admin iterate route rows: %w", err)
	}

	countQuery := fmt.Sprintf(`
SELECT COUNT(*) FROM visa_routes vr WHERE %s
`, whereClause)
	countArgs := args[:len(args)-2]

	var total int64

	if err := s.pool.Reader.QueryRow(ctx, countQuery, countArgs...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("visaroutes: admin count routes: %w", err)
	}

	return out, total, nil
}

// CreateRoute holds the fields for inserting a new visa route.
type CreateRoute struct {
	DestinationSpaceID   pgtype.UUID
	VisaCategory         string
	Name                 string
	Slug                 string
	EligibilitySummary   *string
	RequirementsJSON     []byte
	TypicalFeeUSD        *float64
	PathToPR             *string
	RenewalTerms         *string
	Restrictions         *string
	GovSourceURL         string
	SourceURLLastChecked pgtype.Date
	Status               string
	IsActive             bool
	AIGenerated          bool
	AIModel              *string
	AIPromptHash         *string
}

// Create inserts a new visa route and returns the created row.
func (s *Store) Create(ctx context.Context, input CreateRoute) (RouteRow, error) {
	status := input.Status
	if status == "" {
		status = StatusPublished
	}

	requirementsJSON := input.RequirementsJSON
	if requirementsJSON == nil {
		requirementsJSON = []byte("{}")
	}

	const q = `
INSERT INTO visa_routes (
    destination_space_id, visa_category, name, slug,
    eligibility_summary, requirements_json, typical_fee_usd, path_to_pr,
    renewal_terms, restrictions, gov_source_url, source_url_last_checked,
    status, is_active, ai_generated, ai_model, ai_prompt_hash
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)
RETURNING id
`

	var id pgtype.UUID

	err := s.pool.Writer.QueryRow(
		ctx, q,
		input.DestinationSpaceID, input.VisaCategory,
		input.Name, input.Slug, input.EligibilitySummary, requirementsJSON,
		input.TypicalFeeUSD, input.PathToPR, input.RenewalTerms,
		input.Restrictions, input.GovSourceURL, input.SourceURLLastChecked,
		status, input.IsActive, input.AIGenerated, input.AIModel,
		input.AIPromptHash,
	).Scan(&id)
	if err != nil {
		return RouteRow{}, fmt.Errorf("visaroutes: create: %w", err)
	}

	return s.GetByID(ctx, id)
}

// UpdateRoute holds the mutable fields for a PATCH update. Only non-nil
// fields are applied.
type UpdateRoute struct {
	DestinationSpaceID   *pgtype.UUID
	VisaCategory         *string
	Name                 *string
	Slug                 *string
	EligibilitySummary   *string
	RequirementsJSON     []byte
	TypicalFeeUSD        *float64
	PathToPR             *string
	RenewalTerms         *string
	Restrictions         *string
	GovSourceURL         *string
	SourceURLLastChecked *pgtype.Date
	Status               *string
	IsActive             *bool
	AIGenerated          *bool
	AIModel              *string
	AIPromptHash         *string
}

// Update applies a partial update to a visa route and returns the
// updated row.
func (s *Store) Update(ctx context.Context, id pgtype.UUID, input UpdateRoute) (RouteRow, error) {
	var (
		sets   []string
		args   []any
		argIdx = 1
	)

	if input.DestinationSpaceID != nil {
		sets = append(sets, fmt.Sprintf("destination_space_id = $%d", argIdx))
		args = append(args, *input.DestinationSpaceID)
		argIdx++
	}

	if input.VisaCategory != nil {
		sets = append(sets, fmt.Sprintf("visa_category = $%d", argIdx))
		args = append(args, *input.VisaCategory)
		argIdx++
	}

	if input.Name != nil {
		sets = append(sets, fmt.Sprintf("name = $%d", argIdx))
		args = append(args, *input.Name)
		argIdx++
	}

	if input.Slug != nil {
		sets = append(sets, fmt.Sprintf("slug = $%d", argIdx))
		args = append(args, *input.Slug)
		argIdx++
	}

	if input.EligibilitySummary != nil {
		sets = append(sets, fmt.Sprintf("eligibility_summary = $%d", argIdx))
		args = append(args, *input.EligibilitySummary)
		argIdx++
	}

	if input.RequirementsJSON != nil {
		sets = append(sets, fmt.Sprintf("requirements_json = $%d", argIdx))
		args = append(args, input.RequirementsJSON)
		argIdx++
	}

	if input.TypicalFeeUSD != nil {
		sets = append(sets, fmt.Sprintf("typical_fee_usd = $%d", argIdx))
		args = append(args, *input.TypicalFeeUSD)
		argIdx++
	}

	if input.PathToPR != nil {
		sets = append(sets, fmt.Sprintf("path_to_pr = $%d", argIdx))
		args = append(args, *input.PathToPR)
		argIdx++
	}

	if input.RenewalTerms != nil {
		sets = append(sets, fmt.Sprintf("renewal_terms = $%d", argIdx))
		args = append(args, *input.RenewalTerms)
		argIdx++
	}

	if input.Restrictions != nil {
		sets = append(sets, fmt.Sprintf("restrictions = $%d", argIdx))
		args = append(args, *input.Restrictions)
		argIdx++
	}

	if input.GovSourceURL != nil {
		sets = append(sets, fmt.Sprintf("gov_source_url = $%d", argIdx))
		args = append(args, *input.GovSourceURL)
		argIdx++
	}

	if input.SourceURLLastChecked != nil {
		sets = append(sets, fmt.Sprintf("source_url_last_checked = $%d", argIdx))
		args = append(args, *input.SourceURLLastChecked)
		argIdx++
	}

	if input.Status != nil {
		sets = append(sets, fmt.Sprintf("status = $%d", argIdx))
		args = append(args, *input.Status)
		argIdx++
	}

	if input.IsActive != nil {
		sets = append(sets, fmt.Sprintf("is_active = $%d", argIdx))
		args = append(args, *input.IsActive)
		argIdx++
	}

	if input.AIGenerated != nil {
		sets = append(sets, fmt.Sprintf("ai_generated = $%d", argIdx))
		args = append(args, *input.AIGenerated)
		argIdx++
	}

	if input.AIModel != nil {
		sets = append(sets, fmt.Sprintf("ai_model = $%d", argIdx))
		args = append(args, *input.AIModel)
		argIdx++
	}

	if input.AIPromptHash != nil {
		sets = append(sets, fmt.Sprintf("ai_prompt_hash = $%d", argIdx))
		args = append(args, *input.AIPromptHash)
		argIdx++
	}

	if len(sets) == 0 {
		return s.GetByID(ctx, id)
	}

	sets = append(sets, "updated_at = now()")

	args = append(args, id)
	q := fmt.Sprintf(`
UPDATE visa_routes
SET %s
WHERE id = $%d
`, strings.Join(sets, ", "), argIdx)

	cmd, err := s.pool.Writer.Exec(ctx, q, args...)
	if err != nil {
		return RouteRow{}, fmt.Errorf("visaroutes: update: %w", err)
	}

	if cmd.RowsAffected() == 0 {
		return RouteRow{}, pgx.ErrNoRows
	}

	return s.GetByID(ctx, id)
}

// Approve sets the HITL review fields: hitl_reviewed_by, hitl_reviewed_at
// = now(), status = 'published', and optional review notes. Returns the
// updated row so the caller can return the full record per ADR-088.
func (s *Store) Approve(ctx context.Context, id, reviewerID pgtype.UUID, notes *string) (RouteRow, error) {
	const q = `
UPDATE visa_routes
SET hitl_reviewed_by = $2,
    hitl_reviewed_at = now(),
    hitl_review_notes = $3,
    status = 'published',
    updated_at = now()
WHERE id = $1
`

	cmd, err := s.pool.Writer.Exec(ctx, q, id, reviewerID, notes)
	if err != nil {
		return RouteRow{}, fmt.Errorf("visaroutes: approve: %w", err)
	}

	if cmd.RowsAffected() == 0 {
		return RouteRow{}, pgx.ErrNoRows
	}

	return s.GetByID(ctx, id)
}

// Deactivate soft-deletes a visa route by setting is_active = false.
func (s *Store) Deactivate(ctx context.Context, id pgtype.UUID) error {
	const q = `
UPDATE visa_routes
SET is_active = false, updated_at = now()
WHERE id = $1
`

	cmd, err := s.pool.Writer.Exec(ctx, q, id)
	if err != nil {
		return fmt.Errorf("visaroutes: deactivate: %w", err)
	}

	if cmd.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}

	return nil
}

// adminSortColumn maps a user-supplied sort key to a qualified column
// expression. Returns false for unknown columns to prevent SQL injection
// via the sort query param (geo-aware-tool-pattern.md admin list pattern).
func adminSortColumn(key string) (string, bool) {
	switch strings.ToLower(key) {
	case "name":
		return "vr.name", true
	case "visa_category", "visacategory":
		return "vr.visa_category", true
	case "status":
		return "vr.status", true
	case "created_at", "createdat":
		return "vr.created_at", true
	case "updated_at", "updatedat":
		return "vr.updated_at", true
	case "destination_space_name", "destination":
		return "ds.name", true
	}

	return "", false
}
