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
	"github.com/jackc/pgx/v5/pgconn"
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

// ErrDuplicateSlug is returned when a create/update violates the
// (destination_space_id, slug) unique constraint — mapped to a 409
// conflict envelope by the handler (Rule 5).
var ErrDuplicateSlug = errors.New("visa route slug already exists for this destination")

// ErrInvalidStatus is returned when a create/update supplies a status
// outside the §90 lifecycle enum, or 'published' through a write path
// that bypasses the HITL approve endpoint (ADR-088).
var ErrInvalidStatus = errors.New("invalid status")

// ErrInvalidDestination is returned when a destination value doesn't
// resolve to a country space (UUID, geo_code, or slug).
var ErrInvalidDestination = errors.New("invalid destination")

// IsValidStatus reports whether s is a lifecycle status in §90.
func IsValidStatus(s string) bool {
	switch s {
	case StatusDraft, StatusAIDraft, StatusPending, StatusPublished, StatusRejected:
		return true
	}

	return false
}

// isDuplicateKey reports whether err wraps a PG unique-constraint
// violation (SQLSTATE 23505).
func isDuplicateKey(err error) bool {
	var pge *pgconn.PgError
	return errors.As(err, &pge) && pge.Code == "23505"
}

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
// name/geo_code, visa_processing_stats (latest bucket) for the §83
// "typical processing time" column, and users for the HITL reviewer
// handle on admin views.
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
	GuideID              pgtype.UUID
	GuideTitle           *string
	ChecklistGuideID     pgtype.UUID
	ProcessingMedianDays pgtype.Numeric
	ProcessingSampleCnt  pgtype.Int4
	ProcessingPeriod     pgtype.Date
	HITLReviewerUsername *string
	CreatedAt            pgtype.Timestamptz
	UpdatedAt            pgtype.Timestamptz
}

// ListFilter holds the filter parameters for public listing.
type ListFilter struct {
	DestinationSpaceID pgtype.UUID // optional
	Profession         string      // optional: requirements_json.profession_groups match
	Education          string      // optional: requirements_json.min_education ordinal match
	ExperienceYears    int32       // optional: requirements_json.experience_years
	Age                int32       // optional: requirements_json.age_range {min,max}
	OriginTerms        []string    // optional: geo_code/name for eligible_* list match
	Limit              int32
	Offset             int32
}

// routeSelect is the shared SELECT for all route queries. It joins
// spaces for the destination name/geo_code, the latest
// visa_processing_stats bucket for the §83 "typical processing time"
// column, contents guide_id for the §70 checklist deep link, and users
// for the HITL reviewer handle.
const routeSelect = `
SELECT vr.id, vr.destination_space_id,
       ds.name AS dest_space_name, ds.geo_code AS dest_space_code,
       vr.visa_category, vr.name, vr.slug, vr.eligibility_summary,
       vr.requirements_json, vr.typical_fee_usd, vr.path_to_pr,
       vr.renewal_terms, vr.restrictions, vr.gov_source_url,
       vr.source_url_last_checked, vr.status, vr.is_active,
       vr.ai_generated, vr.ai_model, vr.ai_prompt_hash,
       vr.hitl_reviewed_by, vr.hitl_reviewed_at, vr.hitl_review_notes,
       vr.is_japabot_indexed, vr.guide_id, cg.title AS guide_title,
       CASE WHEN cg.content_type = 'guide'
             AND cg.guide_category = 'visa'
             AND cg.status = 'published'
             AND cg.deleted_at IS NULL
            THEN cg.id
       END AS checklist_guide_id,
       vps.median_days, vps.sample_count, vps.period_start,
       rev.username AS hitl_reviewer_username,
       vr.created_at, vr.updated_at
FROM visa_routes vr
LEFT JOIN spaces ds ON ds.id = vr.destination_space_id
-- §70 checklist deep link: guide_title/checklist_guide_id join the raw
-- guide_id without status predicates — the CASE is the effective-link
-- gate (the CTA must never surface a guide the checklist API can't
-- resolve: unpublished guides 404 there).
LEFT JOIN contents cg ON cg.id = vr.guide_id
LEFT JOIN LATERAL (
    SELECT median_days, sample_count, period_start
    FROM visa_processing_stats vps
    WHERE vps.destination_space_id = vr.destination_space_id
      AND vps.visa_category = vr.visa_category
    ORDER BY vps.period_start DESC, vps.sample_count DESC
    LIMIT 1
) vps ON true
LEFT JOIN users rev ON rev.id = vr.hitl_reviewed_by
`

// VisaGuideExists reports whether id is a live visa guide — the write-side
// contract for guide_id (§70). Any non-deleted status qualifies: the
// read-side checklist_guide_id gate already hides the link until the
// guide publishes, so linking a draft is a valid workflow.
func (s *Store) VisaGuideExists(ctx context.Context, id pgtype.UUID) (bool, error) {
	const q = `
SELECT EXISTS (
    SELECT 1 FROM contents
    WHERE id = $1
      AND content_type = 'guide'
      AND guide_category = 'visa'
      AND deleted_at IS NULL
)
`

	var ok bool
	if err := s.pool.Reader.QueryRow(ctx, q, id).Scan(&ok); err != nil {
		return false, fmt.Errorf("visaroutes: guide exists: %w", err)
	}

	return ok, nil
}

// routeScanDests returns the scan destinations matching routeSelect's
// column order, for use with both QueryRow.Scan and Rows.Scan.
func routeScanDests(r *RouteRow) []any {
	return []any{
		&r.ID, &r.DestinationSpaceID,
		&r.DestSpaceName, &r.DestSpaceCode,
		&r.VisaCategory, &r.Name, &r.Slug, &r.EligibilitySummary,
		&r.RequirementsJSON, &r.TypicalFeeUSD, &r.PathToPR,
		&r.RenewalTerms, &r.Restrictions, &r.GovSourceURL,
		&r.SourceURLLastChecked, &r.Status, &r.IsActive,
		&r.AIGenerated, &r.AIModel, &r.AIPromptHash,
		&r.HITLReviewedBy, &r.HITLReviewedAt, &r.HITLReviewNotes,
		&r.IsJapabotIndexed, &r.GuideID, &r.GuideTitle, &r.ChecklistGuideID,
		&r.ProcessingMedianDays, &r.ProcessingSampleCnt, &r.ProcessingPeriod,
		&r.HITLReviewerUsername,
		&r.CreatedAt, &r.UpdatedAt,
	}
}

// educationOrdinal maps a free-text education level to a rank so the
// ?education= filter expresses "route's min_education <= user's level"
// (rule-based matching, §90). Unknown route values rank 0 (no
// constraint); an unrecognized user level ranks high (matches all).
const educationOrdinalSQL = `
CASE lower(COALESCE(%s, ''))
    WHEN 'none' THEN 0 WHEN 'high_school' THEN 1 WHEN 'secondary' THEN 1
    WHEN 'diploma' THEN 2 WHEN 'associate' THEN 2
    WHEN 'bachelor' THEN 3 WHEN 'degree' THEN 3 WHEN 'bachelors' THEN 3
    WHEN 'master' THEN 4 WHEN 'masters' THEN 4 WHEN 'mba' THEN 4
    WHEN 'phd' THEN 5 WHEN 'doctorate' THEN 5
    ELSE %d
END`

// List returns published, active visa routes for the public listing,
// optionally filtered by destination space. Profile attributes are
// matched against the spec'd requirements_json shape
// {profession_groups, min_education, age_range, experience_years,
// eligible_countries/eligible_nationalities/origin_countries} — legacy
// flat keys ("profession"/"education") are honored as fallbacks.
func (s *Store) List(ctx context.Context, f ListFilter) ([]RouteRow, int64, error) {
	var (
		conditions []string
		args       []any
		argIdx     = 1
	)

	// reqJSON guards every requirements_json access below: a stored
	// scalar or array would make -> and jsonb_each raise "cannot extract
	// field from a non-object" and fail the whole list. Malformed rows
	// degrade to '{}' — i.e. "no constraints" — rather than erroring.
	reqJSON := "CASE WHEN jsonb_typeof(requirements_json) = 'object' THEN requirements_json ELSE '{}'::jsonb END"

	conditions = append(conditions, "vr.status = 'published'")
	conditions = append(conditions, "vr.is_active = true")

	if f.DestinationSpaceID.Valid {
		conditions = append(conditions, fmt.Sprintf("vr.destination_space_id = $%d", argIdx))
		args = append(args, f.DestinationSpaceID)
		argIdx++
	}

	if f.Profession != "" {
		conditions = append(conditions, fmt.Sprintf(`(
  (jsonb_typeof(%[1]s->'profession_groups') = 'array'
   AND EXISTS (SELECT 1 FROM jsonb_array_elements_text(%[1]s->'profession_groups') AS pg
               WHERE pg ILIKE '%%' || $%[2]d || '%%'))
  OR COALESCE(%[1]s->>'profession', '') ILIKE '%%' || $%[2]d || '%%'
)`, reqJSON, argIdx))
		args = append(args, f.Profession)
		argIdx++
	}

	if f.Education != "" {
		routeOrd := fmt.Sprintf(educationOrdinalSQL, reqJSON+"->>'min_education'", 0)
		userOrd := fmt.Sprintf(educationOrdinalSQL, "$"+fmt.Sprint(argIdx), 99)
		conditions = append(conditions, fmt.Sprintf(`(
  %s <= %s
  OR COALESCE(%[3]s->>'education', '') ILIKE '%%' || $%[4]d || '%%'
  OR (jsonb_typeof(%[3]s->'education_levels') = 'array'
      AND EXISTS (SELECT 1 FROM jsonb_array_elements_text(%[3]s->'education_levels') AS el
                  WHERE el ILIKE '%%' || $%[4]d || '%%'))
)`, routeOrd, userOrd, reqJSON, argIdx))
		args = append(args, f.Education)
		argIdx++
	}

	if f.ExperienceYears > 0 {
		// Rows without an experience_years key should match (no minimum
		// requirement). The ~ '^\d+$' guard keeps non-numeric stored
		// values ("3-5", "5+") from erroring the whole list — malformed
		// values are treated as "no constraint" rather than excluded.
		conditions = append(conditions, fmt.Sprintf(
			"(NOT %[1]s ? 'experience_years' OR COALESCE(%[1]s->>'experience_years', '') !~ '^\\d+$' OR (%[1]s->>'experience_years')::int <= $%[2]d)",
			reqJSON, argIdx,
		))

		args = append(args, f.ExperienceYears)
		argIdx++
	}

	if f.Age > 0 {
		// age_range {min,max}: missing/malformed bound = unbounded.
		conditions = append(conditions, fmt.Sprintf(`(NOT %[1]s ? 'age_range'
  OR (
    (%[1]s->'age_range'->>'min' IS NULL
     OR %[1]s->'age_range'->>'min' !~ '^\d+$'
     OR (%[1]s->'age_range'->>'min')::int <= $%[2]d)
    AND
    (%[1]s->'age_range'->>'max' IS NULL
     OR %[1]s->'age_range'->>'max' !~ '^\d+$'
     OR (%[1]s->'age_range'->>'max')::int >= $%[2]d)
  ))`, reqJSON, argIdx, argIdx))
		args = append(args, f.Age)
		argIdx++
	}

	if len(f.OriginTerms) > 0 {
		// Nationality-specific eligibility (§90 Geo Awareness): routes
		// carrying an eligible_* origin restriction only match when the
		// resolved origin geo_code or name appears in the restriction
		// list (array or scalar); unrestricted routes always match.
		ph1 := argIdx
		ph2 := argIdx + 1
		// The geo_code term ($ph1) is matched by equality — a code is an
		// atomic token, so substring matching lets "US" hit "AUSTRALIA" or
		// "IN" hit "GUINEA"/"CHINA"/"SPAIN". The display-name term ($ph2)
		// stays substring so stored demonyms ("Nigerian citizens") still
		// match "Nigeria".
		conditions = append(conditions, fmt.Sprintf(`(NOT (%[5]s ?| '{eligible_countries,eligible_nationalities,origin_countries}'::text[])
  OR EXISTS (
    SELECT 1 FROM jsonb_each(%[5]s) AS e(k, v)
    WHERE e.k = ANY('{eligible_countries,eligible_nationalities,origin_countries}'::text[])
      AND (
        (jsonb_typeof(e.v) = 'array' AND EXISTS (
           SELECT 1 FROM jsonb_array_elements_text(e.v) AS c
           WHERE lower(c) = lower($%[1]d) OR c ILIKE '%%' || $%[2]d || '%%'))
        OR (jsonb_typeof(e.v) = 'string' AND (
           lower(e.v #>> '{}') = lower($%[3]d) OR e.v #>> '{}' ILIKE '%%' || $%[4]d || '%%'))
      )
  ))`, ph1, ph2, ph1, ph2, reqJSON))
		code, name := f.OriginTerms[0], f.OriginTerms[0]
		if len(f.OriginTerms) > 1 {
			name = f.OriginTerms[1]
		}
		args = append(args, code, name)
		argIdx += 2
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

	listQuery := routeSelect + fmt.Sprintf(`
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
		if err := rows.Scan(routeScanDests(&r)...); err != nil {
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
	const q = routeSelect + `
WHERE vr.id = $1
LIMIT 1
`

	var r RouteRow

	err := s.pool.Reader.QueryRow(ctx, q, id).Scan(routeScanDests(&r)...)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return RouteRow{}, pgx.ErrNoRows
		}

		return RouteRow{}, fmt.Errorf("visaroutes: get by id: %w", err)
	}

	return r, nil
}

// GetBySlug returns a single visa route by its slug. The slug is unique
// per (destination_space_id, slug) — when destSpaceID is provided the
// lookup is scoped to that destination; without one the oldest matching
// route wins deterministically (same slug across destinations otherwise
// resolves arbitrarily). Public callers should filter to
// published+active; admin callers may fetch any row.
func (s *Store) GetBySlug(ctx context.Context, slug string, destSpaceID pgtype.UUID) (RouteRow, error) {
	var r RouteRow
	var err error

	if destSpaceID.Valid {
		const q = routeSelect + `
WHERE vr.slug = $1 AND vr.destination_space_id = $2
LIMIT 1
`
		err = s.pool.Reader.QueryRow(ctx, q, slug, destSpaceID).Scan(routeScanDests(&r)...)
	} else {
		const q = routeSelect + `
WHERE vr.slug = $1
ORDER BY vr.created_at ASC
LIMIT 1
`
		err = s.pool.Reader.QueryRow(ctx, q, slug).Scan(routeScanDests(&r)...)
	}

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

	q := routeSelect + fmt.Sprintf(`
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
		if err := rows.Scan(routeScanDests(&r)...); err != nil {
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

	listQuery := routeSelect + fmt.Sprintf(`
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
		if err := rows.Scan(routeScanDests(&r)...); err != nil {
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
	GuideID              pgtype.UUID // zero → NULL
}

// Create inserts a new visa route and returns the created row.
// V2: an omitted status defaults to draft (inactive) — publish is
// an explicit Approve transition, never a side effect of create.
func (s *Store) Create(ctx context.Context, input CreateRoute) (RouteRow, error) {
	status := input.Status
	if status == "" {
		status = StatusDraft
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
    status, is_active, ai_generated, ai_model, ai_prompt_hash, guide_id
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18)
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
		input.AIPromptHash, input.GuideID,
	).Scan(&id)
	if err != nil {
		if isDuplicateKey(err) {
			return RouteRow{}, ErrDuplicateSlug
		}

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
	GuideID              *pgtype.UUID // nil → unchanged; invalid UUID → NULL
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

	if input.GuideID != nil {
		sets = append(sets, fmt.Sprintf("guide_id = $%d", argIdx))
		args = append(args, *input.GuideID)
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
		if isDuplicateKey(err) {
			return RouteRow{}, ErrDuplicateSlug
		}

		return RouteRow{}, fmt.Errorf("visaroutes: update: %w", err)
	}

	if cmd.RowsAffected() == 0 {
		return RouteRow{}, pgx.ErrNoRows
	}

	return s.GetByID(ctx, id)
}

// Approve sets the HITL review fields: hitl_reviewed_by, hitl_reviewed_at
// = now(), status = 'published', is_active = true, and optional review
// notes. Activating on approve is what actually publishes the route to
// the public surface (list filters on status + is_active) and flips the
// generated is_japabot_indexed column. Returns the updated row so the
// caller can return the full record per ADR-088.
func (s *Store) Approve(ctx context.Context, id, reviewerID pgtype.UUID, notes *string) (RouteRow, error) {
	const q = `
UPDATE visa_routes
SET hitl_reviewed_by = $2,
    hitl_reviewed_at = now(),
    hitl_review_notes = $3,
    status = 'published',
    is_active = true,
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

// ResolveSpaceID resolves a ?dest=/?origin= query value to a space ID.
// Accepts a space UUID, a country geo_code ("CA", "NG"), or a space slug
// ("canada") so public callers can address a destination without first
// resolving space internals (geo-aware pattern — destination is the
// primary axis). Returns an invalid pgtype.UUID when raw is empty.
// ErrNoRows when the value matches nothing.
func (s *Store) ResolveSpaceID(ctx context.Context, raw string) (pgtype.UUID, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return pgtype.UUID{}, nil
	}

	var id pgtype.UUID
	if err := id.Scan(raw); err == nil && id.Valid {
		return id, nil
	}

	const q = `
SELECT id
FROM spaces
WHERE level = 'country'
  AND status = 'active'
  AND deleted_at IS NULL
  AND (upper(geo_code) = upper($1) OR slug = $1)
ORDER BY created_at ASC
LIMIT 1
`

	err := s.pool.Reader.QueryRow(ctx, q, raw).Scan(&id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return pgtype.UUID{}, pgx.ErrNoRows
		}

		return pgtype.UUID{}, fmt.Errorf("visaroutes: resolve space: %w", err)
	}

	return id, nil
}

// OriginTerms resolves an ?origin= country value to the terms matched
// against requirements_json eligible_* lists: the space geo_code and
// display name. Returns nil when the origin doesn't resolve — callers
// should then ignore the filter rather than match nothing.
func (s *Store) OriginTerms(ctx context.Context, raw string) ([]string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}

	var id pgtype.UUID
	if err := id.Scan(raw); err == nil && id.Valid {
		const q = `SELECT geo_code, name FROM spaces WHERE id = $1`

		var code, name *string
		if err := s.pool.Reader.QueryRow(ctx, q, id).Scan(&code, &name); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, nil
			}

			return nil, fmt.Errorf("visaroutes: resolve origin: %w", err)
		}

		return originTerms(code, name), nil
	}

	const q = `
SELECT geo_code, name
FROM spaces
WHERE level = 'country'
  AND status = 'active'
  AND deleted_at IS NULL
  AND (upper(geo_code) = upper($1) OR slug = $1 OR name ILIKE $1)
ORDER BY created_at ASC
LIMIT 1
`

	var code, name *string

	err := s.pool.Reader.QueryRow(ctx, q, raw).Scan(&code, &name)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return []string{raw}, nil // unmatched — match the raw term only
		}

		return nil, fmt.Errorf("visaroutes: resolve origin: %w", err)
	}

	return originTerms(code, name), nil
}

func originTerms(code, name *string) []string {
	terms := make([]string, 0, 2)
	if code != nil && *code != "" {
		terms = append(terms, *code)
	}

	if name != nil && *name != "" {
		if len(terms) == 0 || !strings.EqualFold(terms[0], *name) {
			terms = append(terms, *name)
		}
	}

	return terms
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
