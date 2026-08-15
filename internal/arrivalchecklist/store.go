// Package arrivalchecklist implements the Arrival Checklist Generator
// (§92, ADR-100).
//
// Arrival checklists are guides keyed by destination space + visa category
// + applicant type, each containing ordered items grouped by arrival
// phase (day_1_7, week_2_4, month_1_3, ongoing). Guides may be authored
// manually or generated as AI drafts by the Content Agent and reviewed
// via a human-in-the-loop approval step. The package provides public
// browse endpoints (selector + guide items) and admin CRUD/HITL
// endpoints. Tenancy uses tenants(id); destination geo entities use
// spaces(id).
//
// Ref: japaconnect-engineering/sections/92-arrival-checklist.md
//
//	japaconnect-engineering/adr/ADR-100-arrival-checklist.md
package arrivalchecklist

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db"
)

// Applicant types (§92).
const (
	ApplicantIndividual = "individual"
	ApplicantCouple     = "couple"
	ApplicantFamily     = "family"
)

// Guide statuses (§92).
const (
	StatusDraft     = "draft"
	StatusAIDraft   = "ai_draft"
	StatusPending   = "pending"
	StatusPublished = "published"
	StatusRejected  = "rejected"
)

// Arrival phases (§92).
const (
	PhaseDay1To7   = "day_1_7"
	PhaseWeek2To4  = "week_2_4"
	PhaseMonth1To3 = "month_1_3"
	PhaseOngoing   = "ongoing"
)

// Store is the data-access layer for arrival_checklist_guides and
// arrival_checklist_items. It uses direct pgx queries because these are
// new tables without sqlc-generated queries yet. No business logic here.
type Store struct {
	pool *db.DB
}

// NewStore creates an arrival checklist store backed by the given database.
func NewStore(database *db.DB) *Store {
	return &Store{pool: database}
}

// GuideRow holds a single arrival_checklist_guides row, joined to spaces
// for the destination name and to users for the HITL reviewer name.
type GuideRow struct {
	ID                 pgtype.UUID
	TenantID           pgtype.UUID
	DestinationSpaceID pgtype.UUID
	DestSpaceName      *string
	DestSpaceCode      *string
	VisaCategory       *string
	ApplicantType      string
	Version            int32
	Status             string
	IsActive           bool
	AIGenerated        bool
	AIModel            *string
	AIPromptHash       *string
	HITLReviewedBy     pgtype.UUID
	HITLReviewedByName *string
	HITLReviewedAt     pgtype.Timestamptz
	HITLReviewNotes    *string
	IsJapabotIndexed   bool
	CreatedAt          pgtype.Timestamptz
	UpdatedAt          pgtype.Timestamptz
}

// ItemRow holds a single arrival_checklist_items row.
type ItemRow struct {
	ID                    pgtype.UUID
	GuideID               pgtype.UUID
	Phase                 string
	Title                 string
	Description           *string
	OfficialLink          *string
	OriginCountryModifier []byte // JSONB
	SortOrder             int32
	IsFlagged             bool
	CreatedAt             pgtype.Timestamptz
}

// SelectorRow is a distinct destination_space_id + visa_category
// combination returned by ListSelectors. Includes GuideID — the first
// published guide for the combo — so the frontend can fetch items without
// a separate list-guides call.
type SelectorRow struct {
	DestinationSpaceID pgtype.UUID
	DestSpaceName      *string
	DestSpaceCode      *string
	VisaCategory       *string
	GuideCount         int64
	GuideID            pgtype.UUID
}

// ListGuidesFilter holds the filter parameters for listing guides.
type ListGuidesFilter struct {
	TenantID           pgtype.UUID
	DestinationSpaceID pgtype.UUID // optional
	VisaCategory       string      // optional; "_all" sentinel handled by caller
	ApplicantType      string      // optional
	Status             string      // optional; empty = no status filter (admin sees all).
	// PublicOnly, when true, forces status='published' regardless of Status.
	// Used by the public selector/guide endpoints; the admin list sets it
	// false so the HITL queue can see draft/ai_draft/pending/rejected rows.
	PublicOnly  bool
	IsActive    *bool  // optional
	AIGenerated *bool  // optional
	Q           string // optional search on destination name
	Sort        string // optional: created_at|updated_at|status|destination (default updated_at)
	Order       string // optional: asc|desc (default desc)
	Limit       int32
	Offset      int32
}

// allowedSortColumns maps the frontend sort keys to qualified SQL columns.
var allowedSortColumns = map[string]string{
	"created_at":  "g.created_at",
	"updated_at":  "g.updated_at",
	"status":      "g.status",
	"destination": "sp.name",
	"version":     "g.version",
}

// ListGuides returns a paginated, filtered list of arrival checklist
// guides. The query joins spaces (destination) and users (HITL reviewer).
func (s *Store) ListGuides(ctx context.Context, f ListGuidesFilter) ([]GuideRow, int64, error) {
	var (
		conditions []string
		args       []any
		argIdx     = 1
	)

	conditions = append(conditions, fmt.Sprintf("g.tenant_id = $%d", argIdx))
	args = append(args, f.TenantID)
	argIdx++

	if f.Status != "" {
		conditions = append(conditions, fmt.Sprintf("g.status = $%d", argIdx))
		args = append(args, f.Status)
		argIdx++
	} else if f.PublicOnly {
		// Public endpoints only see published guides; admin list (PublicOnly=false)
		// sees all statuses so the HITL queue can review drafts.
		conditions = append(conditions, "g.status = 'published'")
	}

	if f.DestinationSpaceID.Valid {
		conditions = append(conditions, fmt.Sprintf("g.destination_space_id = $%d", argIdx))
		args = append(args, f.DestinationSpaceID)
		argIdx++
	}
	if f.VisaCategory != "" {
		conditions = append(conditions, fmt.Sprintf("g.visa_category = $%d", argIdx))
		args = append(args, f.VisaCategory)
		argIdx++
	}
	if f.ApplicantType != "" {
		conditions = append(conditions, fmt.Sprintf("g.applicant_type = $%d", argIdx))
		args = append(args, f.ApplicantType)
		argIdx++
	}
	if f.IsActive != nil {
		conditions = append(conditions, fmt.Sprintf("g.is_active = $%d", argIdx))
		args = append(args, *f.IsActive)
		argIdx++
	}
	if f.AIGenerated != nil {
		conditions = append(conditions, fmt.Sprintf("g.ai_generated = $%d", argIdx))
		args = append(args, *f.AIGenerated)
		argIdx++
	}
	if f.Q != "" {
		conditions = append(conditions, fmt.Sprintf("sp.name ILIKE $%d", argIdx))
		args = append(args, "%"+f.Q+"%")
		argIdx++
	}

	whereClause := strings.Join(conditions, " AND ")

	// Resolve sort column (whitelisted to prevent SQL injection).
	sortCol := allowedSortColumns[f.Sort]
	if sortCol == "" {
		sortCol = "g.updated_at"
	}
	sortDir := strings.ToUpper(f.Order)
	if sortDir != "ASC" && sortDir != "DESC" {
		sortDir = "DESC"
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

	listQuery := fmt.Sprintf(`
SELECT g.id, g.tenant_id, g.destination_space_id, sp.name AS dest_space_name,
       sp.geo_code AS dest_space_code, g.visa_category, g.applicant_type,
       g.version, g.status, g.is_active, g.ai_generated, g.ai_model,
       g.ai_prompt_hash, g.hitl_reviewed_by, ru.username AS hitl_reviewed_name,
       g.hitl_reviewed_at, g.hitl_review_notes, g.is_japabot_indexed,
       g.created_at, g.updated_at
FROM arrival_checklist_guides g
LEFT JOIN spaces sp ON sp.id = g.destination_space_id
LEFT JOIN users ru ON ru.id = g.hitl_reviewed_by
WHERE %s
ORDER BY %s %s
LIMIT $%d OFFSET $%d
`, whereClause, sortCol, sortDir, argIdx, argIdx+1)
	args = append(args, limit, offset)

	rows, err := s.pool.Reader.Query(ctx, listQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("arrivalchecklist: list guides: %w", err)
	}
	defer rows.Close()

	var out []GuideRow
	for rows.Next() {
		var r GuideRow
		if err := rows.Scan(
			&r.ID, &r.TenantID, &r.DestinationSpaceID, &r.DestSpaceName,
			&r.DestSpaceCode, &r.VisaCategory, &r.ApplicantType,
			&r.Version, &r.Status, &r.IsActive, &r.AIGenerated, &r.AIModel,
			&r.AIPromptHash, &r.HITLReviewedBy, &r.HITLReviewedByName,
			&r.HITLReviewedAt, &r.HITLReviewNotes, &r.IsJapabotIndexed,
			&r.CreatedAt, &r.UpdatedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("arrivalchecklist: scan guide row: %w", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("arrivalchecklist: iterate guide rows: %w", err)
	}

	countQuery := fmt.Sprintf(`
SELECT COUNT(*) FROM arrival_checklist_guides g
LEFT JOIN spaces sp ON sp.id = g.destination_space_id
WHERE %s
`, whereClause)
	countArgs := args[:len(args)-2]
	var total int64
	if err := s.pool.Reader.QueryRow(ctx, countQuery, countArgs...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("arrivalchecklist: count guides: %w", err)
	}

	return out, total, nil
}

// GetGuideByID returns a single guide by its id, scoped to tenant_id.
// Returns guides of ANY status — use only from admin contexts. For
// public/user-facing endpoints use GetPublishedGuideByID instead.
func (s *Store) GetGuideByID(ctx context.Context, id, tenantID pgtype.UUID) (GuideRow, error) {
	const q = `
SELECT g.id, g.tenant_id, g.destination_space_id, sp.name AS dest_space_name,
       sp.geo_code AS dest_space_code, g.visa_category, g.applicant_type,
       g.version, g.status, g.is_active, g.ai_generated, g.ai_model,
       g.ai_prompt_hash, g.hitl_reviewed_by, ru.username AS hitl_reviewed_name,
       g.hitl_reviewed_at, g.hitl_review_notes, g.is_japabot_indexed,
       g.created_at, g.updated_at
FROM arrival_checklist_guides g
LEFT JOIN spaces sp ON sp.id = g.destination_space_id
LEFT JOIN users ru ON ru.id = g.hitl_reviewed_by
WHERE g.id = $1 AND g.tenant_id = $2
LIMIT 1
`
	var r GuideRow
	err := s.pool.Reader.QueryRow(ctx, q, id, tenantID).Scan(
		&r.ID, &r.TenantID, &r.DestinationSpaceID, &r.DestSpaceName,
		&r.DestSpaceCode, &r.VisaCategory, &r.ApplicantType,
		&r.Version, &r.Status, &r.IsActive, &r.AIGenerated, &r.AIModel,
		&r.AIPromptHash, &r.HITLReviewedBy, &r.HITLReviewedByName,
		&r.HITLReviewedAt, &r.HITLReviewNotes, &r.IsJapabotIndexed,
		&r.CreatedAt, &r.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return GuideRow{}, pgx.ErrNoRows
		}
		return GuideRow{}, fmt.Errorf("arrivalchecklist: get guide by id: %w", err)
	}
	return r, nil
}

// GetPublishedGuideByID returns a single published, active guide by its id,
// scoped to tenant_id. Use this for all public/user-facing endpoints —
// draft/ai_draft/pending/rejected guides must never be visible to
// non-admin callers (§92, ADR-088 HITL gate).
func (s *Store) GetPublishedGuideByID(ctx context.Context, id, tenantID pgtype.UUID) (GuideRow, error) {
	const q = `
SELECT g.id, g.tenant_id, g.destination_space_id, sp.name AS dest_space_name,
       sp.geo_code AS dest_space_code, g.visa_category, g.applicant_type,
       g.version, g.status, g.is_active, g.ai_generated, g.ai_model,
       g.ai_prompt_hash, g.hitl_reviewed_by, ru.username AS hitl_reviewed_name,
       g.hitl_reviewed_at, g.hitl_review_notes, g.is_japabot_indexed,
       g.created_at, g.updated_at
FROM arrival_checklist_guides g
LEFT JOIN spaces sp ON sp.id = g.destination_space_id
LEFT JOIN users ru ON ru.id = g.hitl_reviewed_by
WHERE g.id = $1 AND g.tenant_id = $2
  AND g.status = 'published'
  AND g.is_active = true
LIMIT 1
`
	var r GuideRow
	err := s.pool.Reader.QueryRow(ctx, q, id, tenantID).Scan(
		&r.ID, &r.TenantID, &r.DestinationSpaceID, &r.DestSpaceName,
		&r.DestSpaceCode, &r.VisaCategory, &r.ApplicantType,
		&r.Version, &r.Status, &r.IsActive, &r.AIGenerated, &r.AIModel,
		&r.AIPromptHash, &r.HITLReviewedBy, &r.HITLReviewedByName,
		&r.HITLReviewedAt, &r.HITLReviewNotes, &r.IsJapabotIndexed,
		&r.CreatedAt, &r.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return GuideRow{}, pgx.ErrNoRows
		}
		return GuideRow{}, fmt.Errorf("arrivalchecklist: get published guide by id: %w", err)
	}
	return r, nil
}

// GetGuideItems returns all items for a published guide, ordered by
// sort_order. Only published+active guides expose items publicly (§92).
func (s *Store) GetGuideItems(ctx context.Context, guideID, tenantID pgtype.UUID) ([]ItemRow, error) {
	const q = `
SELECT i.id, i.guide_id, i.phase, i.title, i.description, i.official_link,
       i.origin_country_modifier, i.sort_order, i.is_flagged, i.created_at
FROM arrival_checklist_items i
JOIN arrival_checklist_guides g ON g.id = i.guide_id
WHERE i.guide_id = $1 AND g.tenant_id = $2
  AND g.status = 'published'
  AND g.is_active = true
ORDER BY i.sort_order ASC, i.created_at ASC
`
	rows, err := s.pool.Reader.Query(ctx, q, guideID, tenantID)
	if err != nil {
		return nil, fmt.Errorf("arrivalchecklist: get guide items: %w", err)
	}
	defer rows.Close()

	var out []ItemRow
	for rows.Next() {
		var r ItemRow
		if err := rows.Scan(
			&r.ID, &r.GuideID, &r.Phase, &r.Title, &r.Description,
			&r.OfficialLink, &r.OriginCountryModifier, &r.SortOrder,
			&r.IsFlagged, &r.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("arrivalchecklist: scan item row: %w", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("arrivalchecklist: iterate item rows: %w", err)
	}
	return out, nil
}

// CreateGuideInput is the set of fields for inserting a new guide.
type CreateGuideInput struct {
	TenantID           pgtype.UUID
	DestinationSpaceID pgtype.UUID
	VisaCategory       *string
	ApplicantType      string
	Version            int32
	Status             string
	IsActive           bool
	AIGenerated        bool
	AIModel            *string
	AIPromptHash       *string
	IsJapabotIndexed   bool
}

// CreateGuide inserts a new arrival_checklist_guides row and returns the
// created row.
func (s *Store) CreateGuide(ctx context.Context, in CreateGuideInput) (GuideRow, error) {
	if in.ApplicantType == "" {
		in.ApplicantType = ApplicantIndividual
	}
	if in.Version <= 0 {
		in.Version = 1
	}
	if in.Status == "" {
		in.Status = StatusPublished
	}
	const q = `
INSERT INTO arrival_checklist_guides (
    tenant_id, destination_space_id, visa_category, applicant_type, version,
    status, is_active, ai_generated, ai_model, ai_prompt_hash, is_japabot_indexed
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
RETURNING id, tenant_id, destination_space_id, visa_category, applicant_type,
          version, status, is_active, ai_generated, ai_model, ai_prompt_hash,
          hitl_reviewed_by, hitl_reviewed_at, hitl_review_notes,
          is_japabot_indexed, created_at, updated_at
`
	var r GuideRow
	err := s.pool.Writer.QueryRow(ctx, q,
		in.TenantID, in.DestinationSpaceID, in.VisaCategory, in.ApplicantType,
		in.Version, in.Status, in.IsActive, in.AIGenerated, in.AIModel,
		in.AIPromptHash, in.IsJapabotIndexed,
	).Scan(
		&r.ID, &r.TenantID, &r.DestinationSpaceID, &r.VisaCategory, &r.ApplicantType,
		&r.Version, &r.Status, &r.IsActive, &r.AIGenerated, &r.AIModel,
		&r.AIPromptHash, &r.HITLReviewedBy, &r.HITLReviewedAt, &r.HITLReviewNotes,
		&r.IsJapabotIndexed, &r.CreatedAt, &r.UpdatedAt,
	)
	if err != nil {
		return GuideRow{}, fmt.Errorf("arrivalchecklist: create guide: %w", err)
	}
	return r, nil
}

// CreateItemInput holds the fields for a single arrival_checklist_items row.
type CreateItemInput struct {
	GuideID               pgtype.UUID
	Phase                 string
	Title                 string
	Description           *string
	OfficialLink          *string
	OriginCountryModifier []byte // JSONB
	SortOrder             int32
	IsFlagged             bool
}

// CreateItems batch-inserts arrival_checklist_items rows for a guide.
// Called after GenerateGuide creates the parent guide row with AI-generated
// checklist items parsed from the AI Gateway response.
func (s *Store) CreateItems(ctx context.Context, items []CreateItemInput) error {
	if len(items) == 0 {
		return nil
	}
	const q = `
INSERT INTO arrival_checklist_items
    (guide_id, phase, title, description, official_link, origin_country_modifier, sort_order, is_flagged)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`
	batch := s.pool.Writer
	for i, item := range items {
		if _, err := batch.Exec(ctx, q,
			item.GuideID, item.Phase, item.Title, item.Description,
			item.OfficialLink, item.OriginCountryModifier, item.SortOrder, item.IsFlagged,
		); err != nil {
			return fmt.Errorf("arrivalchecklist: create item %d: %w", i, err)
		}
	}
	return nil
}

// UpdateGuideInput holds the mutable fields for a guide update. Only
// non-nil fields are applied.
type UpdateGuideInput struct {
	DestinationSpaceID *pgtype.UUID
	VisaCategory       *string
	ApplicantType      *string
	Version            *int32
	Status             *string
	IsActive           *bool
	IsJapabotIndexed   *bool
	HITLReviewNotes    *string
}

// UpdateGuide updates mutable fields on a guide, scoped to tenant_id.
func (s *Store) UpdateGuide(ctx context.Context, id, tenantID pgtype.UUID, in UpdateGuideInput) error {
	var (
		sets   []string
		args   []any
		argIdx = 1
	)

	if in.DestinationSpaceID != nil {
		sets = append(sets, fmt.Sprintf("destination_space_id = $%d", argIdx))
		args = append(args, *in.DestinationSpaceID)
		argIdx++
	}
	if in.VisaCategory != nil {
		sets = append(sets, fmt.Sprintf("visa_category = $%d", argIdx))
		args = append(args, *in.VisaCategory)
		argIdx++
	}
	if in.ApplicantType != nil {
		sets = append(sets, fmt.Sprintf("applicant_type = $%d", argIdx))
		args = append(args, *in.ApplicantType)
		argIdx++
	}
	if in.Version != nil {
		sets = append(sets, fmt.Sprintf("version = $%d", argIdx))
		args = append(args, *in.Version)
		argIdx++
	}
	if in.Status != nil {
		sets = append(sets, fmt.Sprintf("status = $%d", argIdx))
		args = append(args, *in.Status)
		argIdx++
	}
	if in.IsActive != nil {
		sets = append(sets, fmt.Sprintf("is_active = $%d", argIdx))
		args = append(args, *in.IsActive)
		argIdx++
	}
	if in.IsJapabotIndexed != nil {
		sets = append(sets, fmt.Sprintf("is_japabot_indexed = $%d", argIdx))
		args = append(args, *in.IsJapabotIndexed)
		argIdx++
	}
	if in.HITLReviewNotes != nil {
		sets = append(sets, fmt.Sprintf("hitl_review_notes = $%d", argIdx))
		args = append(args, *in.HITLReviewNotes)
		argIdx++
	}

	if len(sets) == 0 {
		return nil
	}

	sets = append(sets, "updated_at = now()")
	args = append(args, id, tenantID)

	q := fmt.Sprintf(`
UPDATE arrival_checklist_guides
SET %s
WHERE id = $%d AND tenant_id = $%d
`, strings.Join(sets, ", "), argIdx, argIdx+1)

	cmd, err := s.pool.Writer.Exec(ctx, q, args...)
	if err != nil {
		return fmt.Errorf("arrivalchecklist: update guide: %w", err)
	}
	if cmd.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

// ApproveGuide performs the human-in-the-loop approval: sets
// hitl_reviewed_by, hitl_reviewed_at = now(), status = 'published', and
// optionally records review notes. Scoped to tenant_id.
func (s *Store) ApproveGuide(ctx context.Context, id, tenantID, reviewerID pgtype.UUID, notes *string) error {
	const q = `
UPDATE arrival_checklist_guides
SET hitl_reviewed_by = $3,
    hitl_reviewed_at = now(),
    status = 'published',
    hitl_review_notes = $4,
    is_japabot_indexed = true,
    updated_at = now()
WHERE id = $1 AND tenant_id = $2
`
	cmd, err := s.pool.Writer.Exec(ctx, q, id, tenantID, reviewerID, notes)
	if err != nil {
		return fmt.Errorf("arrivalchecklist: approve guide: %w", err)
	}
	if cmd.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

// UserChecklistRow is a user's saved arrival checklist with progress summary.
// Returned by ListUserChecklists for GET /me/arrival-checklists.
type UserChecklistRow struct {
	GuideID         pgtype.UUID
	DestSpaceName   *string
	DestSpaceCode   *string
	VisaCategory    *string
	ApplicantType   string
	TotalItems      int64
	CompletedItems  int64
	FirstProgressAt pgtype.Timestamptz
	LastProgressAt  pgtype.Timestamptz
}

// ListUserChecklists returns the distinct arrival checklist guides a user has
// progress on, with item counts and completion counts. Uses checklist_progress
// with list_type = 'arrival' (migration 000381).
func (s *Store) ListUserChecklists(ctx context.Context, tenantID, userID pgtype.UUID) ([]UserChecklistRow, error) {
	const q = `
SELECT g.id,
       sp.name AS dest_space_name,
       sp.geo_code AS dest_space_code,
       g.visa_category,
       g.applicant_type,
       COUNT(i.id) AS total_items,
       COUNT(cp.id) FILTER (WHERE cp.completed = true) AS completed_items,
       MIN(cp.created_at) AS first_progress_at,
       MAX(cp.updated_at) AS last_progress_at
FROM arrival_checklist_guides g
JOIN arrival_checklist_items i ON i.guide_id = g.id
LEFT JOIN checklist_progress cp
  ON cp.guide_id = g.id
 AND cp.item_key = i.id::text
 AND cp.user_id = $2
 AND cp.tenant_id = $1
 AND cp.list_type = 'arrival'
WHERE g.tenant_id = $1
  AND g.is_active = true
  AND g.status = 'published'
  AND cp.id IS NOT NULL
GROUP BY g.id, sp.name, sp.geo_code, g.visa_category, g.applicant_type
ORDER BY MAX(cp.updated_at) DESC
`
	rows, err := s.pool.Reader.Query(ctx, q, tenantID, userID)
	if err != nil {
		return nil, fmt.Errorf("arrivalchecklist: list user checklists: %w", err)
	}
	defer rows.Close()

	var out []UserChecklistRow
	for rows.Next() {
		var r UserChecklistRow
		if err := rows.Scan(
			&r.GuideID, &r.DestSpaceName, &r.DestSpaceCode,
			&r.VisaCategory, &r.ApplicantType,
			&r.TotalItems, &r.CompletedItems,
			&r.FirstProgressAt, &r.LastProgressAt,
		); err != nil {
			return nil, fmt.Errorf("arrivalchecklist: scan user checklist row: %w", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("arrivalchecklist: iterate user checklist rows: %w", err)
	}
	return out, nil
}

// UserChecklistDetailRow is a single item in a user's saved arrival checklist,
// with the user's progress state merged in.
type UserChecklistDetailRow struct {
	ID           pgtype.UUID
	GuideID      pgtype.UUID
	Phase        string
	Title        string
	Description  *string
	OfficialLink *string
	SortOrder    int32
	IsFlagged    bool
	Completed    bool
	CompletedAt  pgtype.Timestamptz
	Notes        *string
}

// GetUserChecklistItems returns all items for a guide with the user's progress
// merged in (left join on checklist_progress with list_type = 'arrival').
func (s *Store) GetUserChecklistItems(ctx context.Context, tenantID, userID, guideID pgtype.UUID) ([]UserChecklistDetailRow, error) {
	const q = `
SELECT i.id, i.guide_id, i.phase, i.title, i.description, i.official_link,
       i.sort_order, i.is_flagged,
       COALESCE(cp.completed, false) AS completed,
       cp.completed_at, cp.notes
FROM arrival_checklist_items i
JOIN arrival_checklist_guides g ON g.id = i.guide_id
LEFT JOIN checklist_progress cp
  ON cp.guide_id = i.guide_id
 AND cp.item_key = i.id::text
 AND cp.user_id = $2
 AND cp.tenant_id = $1
 AND cp.list_type = 'arrival'
WHERE i.guide_id = $3
  AND g.tenant_id = $1
  AND g.is_active = true
  AND g.status = 'published'
ORDER BY i.sort_order ASC, i.created_at ASC
`
	rows, err := s.pool.Reader.Query(ctx, q, tenantID, userID, guideID)
	if err != nil {
		return nil, fmt.Errorf("arrivalchecklist: get user checklist items: %w", err)
	}
	defer rows.Close()

	var out []UserChecklistDetailRow
	for rows.Next() {
		var r UserChecklistDetailRow
		if err := rows.Scan(
			&r.ID, &r.GuideID, &r.Phase, &r.Title, &r.Description,
			&r.OfficialLink, &r.SortOrder, &r.IsFlagged,
			&r.Completed, &r.CompletedAt, &r.Notes,
		); err != nil {
			return nil, fmt.Errorf("arrivalchecklist: scan user checklist item: %w", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("arrivalchecklist: iterate user checklist items: %w", err)
	}
	return out, nil
}

// StartChecklist creates an initial progress row for a guide, marking the
// user as having "started" the checklist. Idempotent — if a progress row
// already exists for this user+guide (any item), it does nothing.
// Returns the guide DTO so the frontend can render items immediately.
func (s *Store) StartChecklist(ctx context.Context, tenantID, userID, guideID pgtype.UUID) error {
	// Insert a progress row for the first item of the guide (if any items
	// exist), so the user's checklist appears in ListUserChecklists. Uses
	// ON CONFLICT DO NOTHING for idempotency.
	const q = `
INSERT INTO checklist_progress (tenant_id, user_id, guide_id, item_key, completed, list_type)
SELECT $1, $2, $3, i.id::text, false, 'arrival'
FROM arrival_checklist_items i
JOIN arrival_checklist_guides g ON g.id = i.guide_id
WHERE i.guide_id = $3
  AND g.tenant_id = $1
  AND g.is_active = true
  AND g.status = 'published'
ORDER BY i.sort_order ASC
LIMIT 1
ON CONFLICT (tenant_id, user_id, guide_id, item_key, list_type) DO NOTHING
`
	cmd, err := s.pool.Writer.Exec(ctx, q, tenantID, userID, guideID)
	if err != nil {
		return fmt.Errorf("arrivalchecklist: start checklist: %w", err)
	}
	_ = cmd
	return nil
}

// TickItem marks a single arrival checklist item complete/incomplete with an
// optional note. Uses checklist_progress with list_type = 'arrival'. The
// item_key is the arrival_checklist_items.id (UUID as text).
func (s *Store) TickItem(ctx context.Context, tenantID, userID, guideID pgtype.UUID, itemID string, completed bool, notes *string) error {
	var completedAt pgtype.Timestamptz
	if completed {
		completedAt = pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true}
	}

	const q = `
INSERT INTO checklist_progress (tenant_id, user_id, guide_id, item_key, completed, completed_at, notes, list_type)
VALUES ($1, $2, $3, $4, $5, $6, $7, 'arrival')
ON CONFLICT (tenant_id, user_id, guide_id, item_key, list_type)
DO UPDATE SET
    completed = EXCLUDED.completed,
    completed_at = EXCLUDED.completed_at,
    notes = COALESCE(EXCLUDED.notes, checklist_progress.notes),
    updated_at = now()
`
	_, err := s.pool.Writer.Exec(ctx, q, tenantID, userID, guideID, itemID, completed, completedAt, notes)
	if err != nil {
		return fmt.Errorf("arrivalchecklist: tick item: %w", err)
	}
	return nil
}

// ListSelectors returns distinct destination_space_id + visa_category
// combinations that have at least one active published guide, for the
// public selector endpoint.
func (s *Store) ListSelectors(ctx context.Context, tenantID pgtype.UUID) ([]SelectorRow, error) {
	const q = `
SELECT g.destination_space_id, sp.name AS dest_space_name,
       sp.geo_code AS dest_space_code, g.visa_category,
       COUNT(*) AS guide_count,
       (MIN(g.id::text))::uuid AS guide_id
FROM arrival_checklist_guides g
LEFT JOIN spaces sp ON sp.id = g.destination_space_id
WHERE g.tenant_id = $1 AND g.is_active = true AND g.status = 'published'
GROUP BY g.destination_space_id, sp.name, sp.geo_code, g.visa_category
ORDER BY sp.name ASC, g.visa_category ASC NULLS FIRST
`
	rows, err := s.pool.Reader.Query(ctx, q, tenantID)
	if err != nil {
		return nil, fmt.Errorf("arrivalchecklist: list selectors: %w", err)
	}
	defer rows.Close()

	var out []SelectorRow
	for rows.Next() {
		var r SelectorRow
		if err := rows.Scan(
			&r.DestinationSpaceID, &r.DestSpaceName, &r.DestSpaceCode,
			&r.VisaCategory, &r.GuideCount, &r.GuideID,
		); err != nil {
			return nil, fmt.Errorf("arrivalchecklist: scan selector row: %w", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("arrivalchecklist: iterate selector rows: %w", err)
	}
	return out, nil
}
