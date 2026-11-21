// Package housingguide implements the Housing & Accommodation Finder
// Guide (§96, ADR-104).
//
// Housing guides are standalone records in the housing_guides table
// keyed by (destination_space_id, city_space_id,
// accommodation_type). A NULL city_space_id denotes a country-level
// guide. The package provides public filtered listing, single-guide
// retrieval, admin CRUD, Content Agent draft generation, and HITL
// approval. Platform scoping uses spaces(id); destination/city geo entities use
// spaces(id) per ADR-053.
//
// Ref: japaconnect-engineering/sections/96-housing-guide.md
//
//	japaconnect-engineering/adr/ADR-104-housing-guide.md
package housingguide

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

// Accommodation types (§96).
const (
	AccommodationPrivateRental    = "private_rental"
	AccommodationFlatshare        = "flatshare"
	AccommodationStudent          = "student"
	AccommodationEmployerProvided = "employer_provided"
	AccommodationAll              = "all"
)

// Guide statuses (§96).
const (
	StatusDraft     = "draft"
	StatusAIDraft   = "ai_draft"
	StatusPending   = "pending"
	StatusPublished = "published"
	StatusRejected  = "rejected"
)

// Store is the data-access layer for the housing_guides table. It uses
// direct pgx queries because housing_guides is a new table without
// sqlc-generated queries yet. No business logic here.
type Store struct {
	pool *db.DB
}

// NewStore creates a housing guide store backed by the given database.
func NewStore(database *db.DB) *Store {
	return &Store{pool: database}
}

// GetSpaceName returns the name of a space scoped by platform. It is used by
// the service to build human-readable, location-specific web research queries.
// If the space is missing, not valid, or soft-deleted, it returns an empty
// string with no error.
func (s *Store) GetSpaceName(ctx context.Context, spaceID pgtype.UUID) (string, error) {
	if !spaceID.Valid {
		return "", nil
	}

	const q = `
SELECT name FROM spaces
WHERE id = $1 AND deleted_at IS NULL
LIMIT 1
`

	var name string

	err := s.pool.Reader.QueryRow(ctx, q, spaceID).Scan(&name)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", nil
		}

		return "", fmt.Errorf("housingguide: get space name: %w", err)
	}

	return name, nil
}

// ListFieldValuesByGuideIDs returns community field values keyed by guide ID
// (entity_type='housing_guide') in one batched custom_field_values read — the
// canonical source for the §80 Tool Surface Hydration merge, matching the
// housing_guide JCE resolver's (entity_type, entity_id) addressing.
func (s *Store) ListFieldValuesByGuideIDs(ctx context.Context, ids []pgtype.UUID) map[pgtype.UUID]map[string]any {
	return customfields.EntityFieldValuesForEntities(ctx, gen.New(s.pool.Reader), "housing_guide", ids)
}

// GuideRow is the row shape returned by list and get-by-id queries,
// joining housing_guides with spaces (destination + city) and users
// (HITL reviewer).
type GuideRow struct {
	ID                   pgtype.UUID
	DestinationSpaceID   pgtype.UUID
	CitySpaceID          pgtype.UUID
	AccommodationType    *string
	SearchPlatforms      []byte // jsonb
	LandlordRequirements *string
	RentRanges           []byte // jsonb
	ScamCategories       []string
	Tips                 *string
	Status               string
	IsActive             bool
	AIGenerated          bool
	AIModel              *string
	AIPromptHash         *string
	HITLReviewedBy       pgtype.UUID
	HITLReviewedAt       pgtype.Timestamptz
	HITLReviewNotes      *string
	LastVerifiedAt       pgtype.Date
	IsJapabotIndexed     bool
	CreatedAt            pgtype.Timestamptz
	UpdatedAt            pgtype.Timestamptz
	DestSpaceName        *string
	DestSpaceCode        *string
	CitySpaceName        *string
	CitySpaceCode        *string
	ReviewerName         *string
}

// CreateGuideInput holds the fields for inserting a new housing guide.
type CreateGuideInput struct {
	DestinationSpaceID   pgtype.UUID
	CitySpaceID          pgtype.UUID
	AccommodationType    *string
	SearchPlatforms      []byte
	LandlordRequirements *string
	RentRanges           []byte
	ScamCategories       []string
	Tips                 *string
	Status               string
	IsActive             bool
	AIGenerated          bool
	AIModel              *string
	AIPromptHash         *string
	LastVerifiedAt       pgtype.Date
	GuideID              pgtype.UUID // editorial guide cross-link (§96)
	HITLReviewNotes      *string     // auto-provision provenance
}

// UpdateGuideInput holds the mutable fields for updating a housing guide.
// Only non-nil/zero-value-applicable fields are applied by the caller.
type UpdateGuideInput struct {
	DestinationSpaceID pgtype.UUID
	CitySpaceID        pgtype.UUID
	// ClearCitySpaceID sets city_space_id = NULL (demote to a
	// country-level row) — a NULL-able column can't express "clear"
	// through CitySpaceID alone.
	ClearCitySpaceID     bool
	AccommodationType    *string
	SearchPlatforms      []byte
	LandlordRequirements *string
	RentRanges           []byte
	ScamCategories       []string
	Tips                 *string
	Status               *string
	IsActive             *bool
	AIModel              *string
	AIPromptHash         *string
	LastVerifiedAt       pgtype.Date
	// ClearLastVerified sets last_verified_at = NULL — needed because a
	// NULL-able date can't express "clear" through LastVerifiedAt alone.
	ClearLastVerified bool
	// is_japabot_indexed is GENERATED (status × is_active) — not writable.
}

// ListGuidesFilter holds the filter parameters for listing published
// housing guides.
type ListGuidesFilter struct {
	DestSpaceID       pgtype.UUID // optional
	CitySpaceID       pgtype.UUID // optional
	AccommodationType string      // optional
	Status            string      // default: published
	IsActive          *bool       // default: true
	Limit             int32
	Offset            int32
}

// ListGuides returns a paginated, filtered list of housing guides. The
// query joins housing_guides with spaces (destination + city) and users
// (HITL reviewer name).
func (s *Store) ListGuides(ctx context.Context, f ListGuidesFilter) ([]GuideRow, int64, error) {
	var (
		conditions []string
		args       []any
		argIdx     = 1
	)

	if f.Status != "" {
		conditions = append(conditions, fmt.Sprintf("hg.status = $%d", argIdx))
		args = append(args, f.Status)
		argIdx++
	} else {
		conditions = append(conditions, "hg.status = 'published'")
	}

	active := true
	if f.IsActive != nil {
		active = *f.IsActive
	}

	conditions = append(conditions, fmt.Sprintf("hg.is_active = $%d", argIdx))
	args = append(args, active)
	argIdx++

	if f.DestSpaceID.Valid {
		conditions = append(conditions, fmt.Sprintf("hg.destination_space_id = $%d", argIdx))
		args = append(args, f.DestSpaceID)
		argIdx++
	}

	if f.CitySpaceID.Valid {
		conditions = append(conditions, fmt.Sprintf("hg.city_space_id = $%d", argIdx))
		args = append(args, f.CitySpaceID)
		argIdx++
	}

	if f.AccommodationType != "" {
		conditions = append(conditions, fmt.Sprintf("hg.accommodation_type = $%d", argIdx))
		args = append(args, f.AccommodationType)
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
SELECT hg.id, hg.destination_space_id, hg.city_space_id,
       hg.accommodation_type, hg.search_platforms, hg.landlord_requirements,
       hg.rent_ranges, hg.scam_categories, hg.tips, hg.status, hg.is_active,
       hg.ai_generated, hg.ai_model, hg.ai_prompt_hash,
       hg.hitl_reviewed_by, hg.hitl_reviewed_at, hg.hitl_review_notes,
       hg.last_verified_at, hg.is_japabot_indexed,
       hg.created_at, hg.updated_at,
       ds.name AS dest_space_name, ds.geo_code AS dest_space_code,
       cs.name AS city_space_name, cs.geo_code AS city_space_code,
       u.username AS reviewer_name
FROM housing_guides hg
LEFT JOIN spaces ds ON ds.id = hg.destination_space_id
LEFT JOIN spaces cs ON cs.id = hg.city_space_id
LEFT JOIN users u ON u.id = hg.hitl_reviewed_by
WHERE %s
ORDER BY hg.updated_at DESC
LIMIT $%d OFFSET $%d
`, whereClause, argIdx, argIdx+1)

	args = append(args, limit, offset)

	rows, err := s.pool.Reader.Query(ctx, listQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("housingguide: list guides: %w", err)
	}
	defer rows.Close()

	var out []GuideRow

	for rows.Next() {
		var r GuideRow
		if err := rows.Scan(
			&r.ID, &r.DestinationSpaceID, &r.CitySpaceID,
			&r.AccommodationType, &r.SearchPlatforms, &r.LandlordRequirements,
			&r.RentRanges, &r.ScamCategories, &r.Tips, &r.Status, &r.IsActive,
			&r.AIGenerated, &r.AIModel, &r.AIPromptHash,
			&r.HITLReviewedBy, &r.HITLReviewedAt, &r.HITLReviewNotes,
			&r.LastVerifiedAt, &r.IsJapabotIndexed,
			&r.CreatedAt, &r.UpdatedAt,
			&r.DestSpaceName, &r.DestSpaceCode,
			&r.CitySpaceName, &r.CitySpaceCode,
			&r.ReviewerName,
		); err != nil {
			return nil, 0, fmt.Errorf("housingguide: scan guide row: %w", err)
		}

		out = append(out, r)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("housingguide: iterate guide rows: %w", err)
	}

	countQuery := fmt.Sprintf(`
SELECT COUNT(*) FROM housing_guides hg
WHERE %s
`, whereClause)
	countArgs := args[:len(args)-2]

	var total int64

	if err := s.pool.Reader.QueryRow(ctx, countQuery, countArgs...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("housingguide: count guides: %w", err)
	}

	return out, total, nil
}

// GetGuideByID returns a single housing guide by its id — any status
// (used by admin flows and the editorial claim lookup). Public handlers
// must use GetPublishedGuideByID so drafts/ai_drafts stay invisible
// (§97 precedent, geo-aware pattern "AI drafts must not be visible").
func (s *Store) GetGuideByID(ctx context.Context, id pgtype.UUID) (GuideRow, error) {
	const q = `
SELECT hg.id, hg.destination_space_id, hg.city_space_id,
       hg.accommodation_type, hg.search_platforms, hg.landlord_requirements,
       hg.rent_ranges, hg.scam_categories, hg.tips, hg.status, hg.is_active,
       hg.ai_generated, hg.ai_model, hg.ai_prompt_hash,
       hg.hitl_reviewed_by, hg.hitl_reviewed_at, hg.hitl_review_notes,
       hg.last_verified_at, hg.is_japabot_indexed,
       hg.created_at, hg.updated_at,
       ds.name AS dest_space_name, ds.geo_code AS dest_space_code,
       cs.name AS city_space_name, cs.geo_code AS city_space_code,
       u.username AS reviewer_name
FROM housing_guides hg
LEFT JOIN spaces ds ON ds.id = hg.destination_space_id
LEFT JOIN spaces cs ON cs.id = hg.city_space_id
LEFT JOIN users u ON u.id = hg.hitl_reviewed_by
WHERE hg.id = $1
LIMIT 1
`

	return s.scanGuideRow(ctx, q, id)
}

// GetPublishedGuideByID returns a single PUBLISHED, ACTIVE housing guide
// by id — the only read the public GET /housing-guide/:id handler may
// use. Non-published rows resolve as pgx.ErrNoRows → 404.
func (s *Store) GetPublishedGuideByID(ctx context.Context, id pgtype.UUID) (GuideRow, error) {
	const q = `
SELECT hg.id, hg.destination_space_id, hg.city_space_id,
       hg.accommodation_type, hg.search_platforms, hg.landlord_requirements,
       hg.rent_ranges, hg.scam_categories, hg.tips, hg.status, hg.is_active,
       hg.ai_generated, hg.ai_model, hg.ai_prompt_hash,
       hg.hitl_reviewed_by, hg.hitl_reviewed_at, hg.hitl_review_notes,
       hg.last_verified_at, hg.is_japabot_indexed,
       hg.created_at, hg.updated_at,
       ds.name AS dest_space_name, ds.geo_code AS dest_space_code,
       cs.name AS city_space_name, cs.geo_code AS city_space_code,
       u.username AS reviewer_name
FROM housing_guides hg
LEFT JOIN spaces ds ON ds.id = hg.destination_space_id
LEFT JOIN spaces cs ON cs.id = hg.city_space_id
LEFT JOIN users u ON u.id = hg.hitl_reviewed_by
WHERE hg.id = $1
  AND hg.status = 'published'
  AND hg.is_active = true
LIMIT 1
`

	return s.scanGuideRow(ctx, q, id)
}

func (s *Store) scanGuideRow(ctx context.Context, q string, id pgtype.UUID) (GuideRow, error) {
	var r GuideRow

	err := s.pool.Reader.QueryRow(ctx, q, id).Scan(
		&r.ID, &r.DestinationSpaceID, &r.CitySpaceID,
		&r.AccommodationType, &r.SearchPlatforms, &r.LandlordRequirements,
		&r.RentRanges, &r.ScamCategories, &r.Tips, &r.Status, &r.IsActive,
		&r.AIGenerated, &r.AIModel, &r.AIPromptHash,
		&r.HITLReviewedBy, &r.HITLReviewedAt, &r.HITLReviewNotes,
		&r.LastVerifiedAt, &r.IsJapabotIndexed,
		&r.CreatedAt, &r.UpdatedAt,
		&r.DestSpaceName, &r.DestSpaceCode,
		&r.CitySpaceName, &r.CitySpaceCode,
		&r.ReviewerName,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return GuideRow{}, pgx.ErrNoRows
		}

		return GuideRow{}, fmt.Errorf("housingguide: get guide by id: %w", err)
	}

	return r, nil
}

// CreateGuide inserts a new housing_guides row and returns the id of the
// newly created guide.
func (s *Store) CreateGuide(ctx context.Context, input CreateGuideInput) (pgtype.UUID, error) {
	searchPlatforms := input.SearchPlatforms
	if searchPlatforms == nil {
		searchPlatforms = []byte("[]")
	}

	rentRanges := input.RentRanges
	if rentRanges == nil {
		rentRanges = []byte("{}")
	}
	// Per ADR-088, admin-created guides enter as 'draft' and require HITL
	// approval before publishing. Only the Approve handler sets 'published'.
	status := input.Status
	if status == "" {
		status = StatusDraft
	}

	// ON CONFLICT on the NULLS NOT DISTINCT natural-key index
	// (migration 001222) — a concurrent insert of the same
	// destination × city × accommodation_type combination yields zero
	// RETURNING rows, surfacing as pgx.ErrNoRows so callers can either
	// claim the winner (editorial hook) or report a duplicate (admin).
	const q = `
INSERT INTO housing_guides (
    destination_space_id, city_space_id, accommodation_type,
    search_platforms, landlord_requirements, rent_ranges, scam_categories,
    tips, status, is_active, ai_generated, ai_model, ai_prompt_hash,
    last_verified_at, guide_id, hitl_review_notes
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, NULLIF($15,'')::uuid, $16)
ON CONFLICT (destination_space_id, city_space_id, accommodation_type) DO NOTHING
RETURNING id
`

	var id pgtype.UUID

	err := s.pool.Writer.QueryRow(
		ctx, q,
		input.DestinationSpaceID, input.CitySpaceID,
		input.AccommodationType, searchPlatforms, input.LandlordRequirements,
		rentRanges, input.ScamCategories, input.Tips, status,
		input.IsActive, input.AIGenerated, input.AIModel, input.AIPromptHash,
		input.LastVerifiedAt, uuidOrEmpty(input.GuideID), input.HITLReviewNotes,
	).Scan(&id)
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("housingguide: create guide: %w", err)
	}

	return id, nil
}

// UpdateGuide updates the mutable fields on a housing_guides row. Only
// the fields set on UpdateGuideInput are applied.
func (s *Store) UpdateGuide(ctx context.Context, id pgtype.UUID, input UpdateGuideInput) error {
	var (
		sets   []string
		args   []any
		argIdx = 1
	)

	if input.DestinationSpaceID.Valid {
		sets = append(sets, fmt.Sprintf("destination_space_id = $%d", argIdx))
		args = append(args, input.DestinationSpaceID)
		argIdx++
	}

	if input.ClearCitySpaceID {
		sets = append(sets, "city_space_id = NULL")
	} else if input.CitySpaceID.Valid {
		sets = append(sets, fmt.Sprintf("city_space_id = $%d", argIdx))
		args = append(args, input.CitySpaceID)
		argIdx++
	}

	if input.AccommodationType != nil {
		sets = append(sets, fmt.Sprintf("accommodation_type = $%d", argIdx))
		args = append(args, *input.AccommodationType)
		argIdx++
	}

	if input.SearchPlatforms != nil {
		sets = append(sets, fmt.Sprintf("search_platforms = $%d", argIdx))
		args = append(args, input.SearchPlatforms)
		argIdx++
	}

	if input.LandlordRequirements != nil {
		sets = append(sets, fmt.Sprintf("landlord_requirements = $%d", argIdx))
		args = append(args, *input.LandlordRequirements)
		argIdx++
	}

	if input.RentRanges != nil {
		sets = append(sets, fmt.Sprintf("rent_ranges = $%d", argIdx))
		args = append(args, input.RentRanges)
		argIdx++
	}

	if input.ScamCategories != nil {
		sets = append(sets, fmt.Sprintf("scam_categories = $%d", argIdx))
		args = append(args, input.ScamCategories)
		argIdx++
	}

	if input.Tips != nil {
		sets = append(sets, fmt.Sprintf("tips = $%d", argIdx))
		args = append(args, *input.Tips)
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

	if input.LastVerifiedAt.Valid {
		sets = append(sets, fmt.Sprintf("last_verified_at = $%d", argIdx))
		args = append(args, input.LastVerifiedAt)
		argIdx++
	} else if input.ClearLastVerified {
		sets = append(sets, "last_verified_at = NULL")
	}

	if len(sets) == 0 {
		return nil
	}

	sets = append(sets, "updated_at = now()")

	args = append(args, id)
	q := fmt.Sprintf(`
UPDATE housing_guides
SET %s
WHERE id = $%d
`, strings.Join(sets, ", "), argIdx)

	cmd, err := s.pool.Writer.Exec(ctx, q, args...)
	if err != nil {
		return fmt.Errorf("housingguide: update guide: %w", err)
	}

	if cmd.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}

	return nil
}

// ApproveGuide marks a housing guide as reviewed, active, and published
// by an admin (HITL approval). Sets hitl_reviewed_by,
// hitl_reviewed_at=now(), status='published', is_active=true, and
// optional review notes.
func (s *Store) ApproveGuide(ctx context.Context, id, reviewerID pgtype.UUID, notes *string) error {
	// status != 'published' matches the attestation ApprovePathway guard:
	// re-approving a live guide must not silently re-stamp the reviewer.
	// is_active = true revives AI drafts (GenerateDraft lands them
	// is_active=false) — approval must make the row publicly visible, and
	// last_verified_at is stamped by the same act: HITL approval IS the
	// content verification that ADR-104's freshness display reports.
	const q = `
UPDATE housing_guides
SET hitl_reviewed_by = $2,
    hitl_reviewed_at = now(),
    hitl_review_notes = $3,
    status = 'published',
    is_active = true,
    last_verified_at = CURRENT_DATE,
    updated_at = now()
WHERE id = $1
  AND status != 'published'
`

	cmd, err := s.pool.Writer.Exec(ctx, q, id, reviewerID, notes)
	if err != nil {
		return fmt.Errorf("housingguide: approve guide: %w", err)
	}

	if cmd.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}

	return nil
}

// RejectGuide rejects a housing guide, setting its status to 'rejected'
// and recording the reviewer and review notes. A guide may be rejected
// from any non-published, non-rejected status (draft, ai_draft, pending)
// per the ADR-088 lifecycle.
func (s *Store) RejectGuide(ctx context.Context, id, reviewerID pgtype.UUID, notes *string) error {
	const q = `
UPDATE housing_guides
SET hitl_reviewed_by = $2,
    hitl_reviewed_at = now(),
    hitl_review_notes = $3,
    status = 'rejected',
    updated_at = now()
WHERE id = $1
  AND status IN ('draft', 'ai_draft', 'pending')
`

	cmd, err := s.pool.Writer.Exec(ctx, q, id, reviewerID, notes)
	if err != nil {
		return fmt.Errorf("housingguide: reject guide: %w", err)
	}

	if cmd.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}

	return nil
}

// AdminListFilter holds filter parameters for the admin management list.
type AdminListFilter struct {
	Status            string      // optional
	AccommodationType string      // optional
	DestSpaceID       pgtype.UUID // optional
	CitySpaceID       pgtype.UUID // optional
	Q                 string      // optional: search tips/landlord_requirements
	IsActive          *bool       // optional
	Limit             int32
	Offset            int32
}

// AdminList returns all housing guides (any status) for the admin
// management queue.
func (s *Store) AdminList(ctx context.Context, f AdminListFilter) ([]GuideRow, int64, error) {
	var (
		conditions []string
		args       []any
		argIdx     = 1
	)

	if f.Status != "" {
		conditions = append(conditions, fmt.Sprintf("hg.status = $%d", argIdx))
		args = append(args, f.Status)
		argIdx++
	}

	if f.AccommodationType != "" {
		conditions = append(conditions, fmt.Sprintf("hg.accommodation_type = $%d", argIdx))
		args = append(args, f.AccommodationType)
		argIdx++
	}

	if f.DestSpaceID.Valid {
		conditions = append(conditions, fmt.Sprintf("hg.destination_space_id = $%d", argIdx))
		args = append(args, f.DestSpaceID)
		argIdx++
	}

	if f.CitySpaceID.Valid {
		conditions = append(conditions, fmt.Sprintf("hg.city_space_id = $%d", argIdx))
		args = append(args, f.CitySpaceID)
		argIdx++
	}

	if f.IsActive != nil {
		conditions = append(conditions, fmt.Sprintf("hg.is_active = $%d", argIdx))
		args = append(args, *f.IsActive)
		argIdx++
	}

	if f.Q != "" {
		conditions = append(conditions, fmt.Sprintf("(hg.tips ILIKE $%d OR hg.landlord_requirements ILIKE $%d)", argIdx, argIdx))
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

	listQuery := fmt.Sprintf(`
SELECT hg.id, hg.destination_space_id, hg.city_space_id,
       hg.accommodation_type, hg.search_platforms, hg.landlord_requirements,
       hg.rent_ranges, hg.scam_categories, hg.tips, hg.status, hg.is_active,
       hg.ai_generated, hg.ai_model, hg.ai_prompt_hash,
       hg.hitl_reviewed_by, hg.hitl_reviewed_at, hg.hitl_review_notes,
       hg.last_verified_at, hg.is_japabot_indexed,
       hg.created_at, hg.updated_at,
       ds.name AS dest_space_name, ds.geo_code AS dest_space_code,
       cs.name AS city_space_name, cs.geo_code AS city_space_code,
       u.username AS reviewer_name
FROM housing_guides hg
LEFT JOIN spaces ds ON ds.id = hg.destination_space_id
LEFT JOIN spaces cs ON cs.id = hg.city_space_id
LEFT JOIN users u ON u.id = hg.hitl_reviewed_by
WHERE %s
ORDER BY hg.created_at DESC
LIMIT $%d OFFSET $%d
`, whereClause, argIdx, argIdx+1)

	args = append(args, limit, offset)

	rows, err := s.pool.Reader.Query(ctx, listQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("housingguide: admin list guides: %w", err)
	}
	defer rows.Close()

	var out []GuideRow

	for rows.Next() {
		var r GuideRow
		if err := rows.Scan(
			&r.ID, &r.DestinationSpaceID, &r.CitySpaceID,
			&r.AccommodationType, &r.SearchPlatforms, &r.LandlordRequirements,
			&r.RentRanges, &r.ScamCategories, &r.Tips, &r.Status, &r.IsActive,
			&r.AIGenerated, &r.AIModel, &r.AIPromptHash,
			&r.HITLReviewedBy, &r.HITLReviewedAt, &r.HITLReviewNotes,
			&r.LastVerifiedAt, &r.IsJapabotIndexed,
			&r.CreatedAt, &r.UpdatedAt,
			&r.DestSpaceName, &r.DestSpaceCode,
			&r.CitySpaceName, &r.CitySpaceCode,
			&r.ReviewerName,
		); err != nil {
			return nil, 0, fmt.Errorf("housingguide: admin scan guide row: %w", err)
		}

		out = append(out, r)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("housingguide: admin iterate guide rows: %w", err)
	}

	countQuery := fmt.Sprintf(`
SELECT COUNT(*) FROM housing_guides hg
WHERE %s
`, whereClause)
	countArgs := args[:len(args)-2]

	var total int64

	if err := s.pool.Reader.QueryRow(ctx, countQuery, countArgs...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("housingguide: admin count guides: %w", err)
	}

	return out, total, nil
}

// ── Editorial integration helpers (§96 — /studio/create?type=guide) ────
// Mirror the §97 attestation / §109 ECA pattern: guide→tool resolution runs
// on the (destination_space, city_space, accommodation_type) natural key;
// the tool→guide half resolves an explicit claim (contents.field_values.
// guide_id or housing_guides.guide_id) before falling back to the
// combination match.

// SpaceIDByCountryGeoCode resolves a country-level geo space by its
// geo_code (case-insensitive) — the reverse of the /geo/countries list
// the editorial guide fields are sourced from.
func (s *Store) SpaceIDByCountryGeoCode(ctx context.Context, geoCode string) (pgtype.UUID, error) {
	const q = `
SELECT id FROM spaces
WHERE level = 'country'
  AND UPPER(geo_code) = UPPER($1)
  AND deleted_at IS NULL
ORDER BY created_at, id
LIMIT 1
`

	var id pgtype.UUID
	if err := s.pool.Reader.QueryRow(ctx, q, geoCode).Scan(&id); err != nil {
		return pgtype.UUID{}, err
	}

	return id, nil
}

// CitySpaceIDByName resolves a city-level space by name inside the given
// destination country. The spaces hierarchy is city → state → country via
// parent_id, so a city matches when its parent is the destination or its
// parent's parent is. Returns pgx.ErrNoRows when no city matches — the
// caller treats that as a country-level guide (NULL city_space_id).
func (s *Store) CitySpaceIDByName(ctx context.Context, destSpaceID pgtype.UUID, name string) (pgtype.UUID, error) {
	const q = `
SELECT c.id FROM spaces c
LEFT JOIN spaces p ON p.id = c.parent_id
WHERE c.level = 'city'
  AND LOWER(BTRIM(c.name)) = LOWER(BTRIM($2))
  AND c.deleted_at IS NULL
  AND (c.parent_id = $1 OR p.parent_id = $1)
ORDER BY c.created_at, c.id
LIMIT 1
`

	var id pgtype.UUID
	if err := s.pool.Reader.QueryRow(ctx, q, destSpaceID, name).Scan(&id); err != nil {
		return pgtype.UUID{}, err
	}

	return id, nil
}

// GuideIDForCombination returns the id of the guide matching the
// (destination, city, accommodation_type) natural key, in any status — a
// draft still claims the combination so the publish hook never
// duplicates it. NULL-safe on city_space_id/accommodation_type: an unset
// city is part of the key (country-level guide). Read off Writer: this
// feeds a write decision. Returns pgx.ErrNoRows when no guide matches.
func (s *Store) GuideIDForCombination(ctx context.Context, destSpaceID, citySpaceID pgtype.UUID, accommodationType string) (pgtype.UUID, error) {
	const q = `
SELECT id FROM housing_guides
WHERE destination_space_id = $1
  AND city_space_id IS NOT DISTINCT FROM $2
  AND COALESCE(accommodation_type, '') = COALESCE($3, '')
ORDER BY created_at, id
LIMIT 1
`

	var typeArg *string
	if accommodationType != "" {
		typeArg = &accommodationType
	}

	var id pgtype.UUID
	if err := s.pool.Writer.QueryRow(ctx, q, destSpaceID, citySpaceID, typeArg).Scan(&id); err != nil {
		return pgtype.UUID{}, err
	}

	return id, nil
}

// LinkGuideToContent stamps the bidirectional editorial link: the
// housing_guides row's guide_id points at the guide content, and the
// guide's field_values.guide_id is backfilled so the link becomes
// explicit rather than computed (§97 precedent). field_values is
// JSONB-merged, not rewritten — concurrent edits to other keys survive.
func (s *Store) LinkGuideToContent(ctx context.Context, guideID, contentID pgtype.UUID) error {
	// Both writes commit together (attestation tx precedent): a
	// half-applied link would leave the housing row claiming a guide
	// that has no backfilled guide_id to claim it back.
	tx, err := s.pool.Writer.Begin(ctx)
	if err != nil {
		return fmt.Errorf("housingguide: link tx begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `
		UPDATE housing_guides SET guide_id = $2, updated_at = now()
		WHERE id = $1`, guideID, contentID); err != nil {
		return fmt.Errorf("housingguide: link guide to content: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		UPDATE contents
		SET field_values = COALESCE(field_values, '{}'::jsonb) || jsonb_build_object('guide_id', $2::text),
		    updated_at = now()
		WHERE id = $1
		  AND content_type = 'guide'
		  AND guide_category = 'accommodation'
		  AND deleted_at IS NULL`, contentID, guideID.String()); err != nil {
		return fmt.Errorf("housingguide: backfill guide guide_id: %w", err)
	}

	// Re-claim hygiene: a previous guide that still holds this row in
	// field_values.guide_id would keep matching the tool's slug resolver
	// even though the row's own guide_id column now points here. Strip the
	// stale field-side claim so exactly one guide owns the link.
	if _, err := tx.Exec(ctx, `
		UPDATE contents
		SET field_values = field_values - 'guide_id',
		    updated_at = now()
		WHERE id != $1
		  AND content_type = 'guide'
		  AND guide_category = 'accommodation'
		  AND deleted_at IS NULL
		  AND field_values->>'guide_id' = $2`, contentID, guideID.String()); err != nil {
		return fmt.Errorf("housingguide: clear stale guide_id claims: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("housingguide: link tx commit: %w", err)
	}

	return nil
}

// GuideSlugForGuide returns the slug of the published editorial guide
// (guide_category='accommodation') linked to the given housing_guides row
// — the tool→guide half of the cross-link. A guide claims the row either
// explicitly (field_values.guide_id, or the row's guide_id column
// pointing at the content) or via the matching natural key (destination
// geo code + city name + accommodation_type). Explicit claims win over
// combination matches. Returns "" with nil error when unclaimed.
func (s *Store) GuideSlugForGuide(ctx context.Context, g GuideRow) (string, error) {
	if !g.ID.Valid {
		return "", nil
	}

	var destGeo, cityName *string
	if err := s.pool.Reader.QueryRow(ctx, `
		SELECT geo_code FROM spaces WHERE id = $1`, g.DestinationSpaceID).Scan(&destGeo); err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return "", fmt.Errorf("housingguide: dest geo lookup: %w", err)
	}
	if g.CitySpaceID.Valid {
		if err := s.pool.Reader.QueryRow(ctx, `
			SELECT name FROM spaces WHERE id = $1`, g.CitySpaceID).Scan(&cityName); err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return "", fmt.Errorf("housingguide: city name lookup: %w", err)
		}
	}

	// The combination arm matches on the editorial field values. City is
	// symmetric: a country-level row (NULL city) requires an empty guide
	// city field; a city row requires the same city name. An unset/empty
	// accommodation_type on either side means "not type-scoped" and only
	// matches the same emptiness — an editorial guide typed 'flatshare'
	// must not claim the generic country guide.
	const q = `
SELECT c.slug FROM contents c
WHERE c.content_type = 'guide'
  AND c.guide_category = 'accommodation'
  AND c.status = 'published'
  AND c.deleted_at IS NULL
  AND (
      c.field_values->>'guide_id' = $1
      OR c.id = (SELECT guide_id FROM housing_guides WHERE id = $1::uuid)
      OR (
          COALESCE(BTRIM(c.field_values->>'guide_id'), '') = ''
          AND UPPER(c.field_values->>'destination_country') = UPPER($2::text)
          AND LOWER(COALESCE(BTRIM(c.field_values->>'city'), '')) = LOWER(COALESCE($3::text, ''))
          AND LOWER(COALESCE(BTRIM(c.field_values->>'accommodation_type'), '')) = LOWER(COALESCE($4::text, ''))
      )
  )
ORDER BY (c.id = (SELECT guide_id FROM housing_guides WHERE id = $1::uuid)) DESC,
         (c.field_values->>'guide_id' = $1) DESC,
         c.created_at, c.id
LIMIT 1
`

	var slug string
	err := s.pool.Reader.QueryRow(ctx, q, g.ID.String(), destGeo, cityName, g.AccommodationType).Scan(&slug)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", nil
		}

		return "", fmt.Errorf("housingguide: guide slug for guide: %w", err)
	}

	return slug, nil
}

// uuidOrEmpty renders a pgtype.UUID for NULLIF inserts — an invalid UUID
// yields "" so the column lands NULL instead of a zero-value.
func uuidOrEmpty(id pgtype.UUID) string {
	if !id.Valid {
		return ""
	}

	return id.String()
}
