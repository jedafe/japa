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
}

// UpdateGuideInput holds the mutable fields for updating a housing guide.
// Only non-nil/zero-value-applicable fields are applied by the caller.
type UpdateGuideInput struct {
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
	IsJapabotIndexed     *bool
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

// GetGuideByID returns a single housing guide by its id.
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

	const q = `
INSERT INTO housing_guides (
    destination_space_id, city_space_id, accommodation_type,
    search_platforms, landlord_requirements, rent_ranges, scam_categories,
    tips, status, is_active, ai_generated, ai_model, ai_prompt_hash,
    last_verified_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
RETURNING id
`

	var id pgtype.UUID

	err := s.pool.Writer.QueryRow(
		ctx, q,
		input.DestinationSpaceID, input.CitySpaceID,
		input.AccommodationType, searchPlatforms, input.LandlordRequirements,
		rentRanges, input.ScamCategories, input.Tips, status,
		input.IsActive, input.AIGenerated, input.AIModel, input.AIPromptHash,
		input.LastVerifiedAt,
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
	}

	if input.IsJapabotIndexed != nil {
		sets = append(sets, fmt.Sprintf("is_japabot_indexed = $%d", argIdx))
		args = append(args, *input.IsJapabotIndexed)
		argIdx++
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

// ApproveGuide marks a housing guide as reviewed and published by an
// admin (HITL approval). Sets hitl_reviewed_by, hitl_reviewed_at=now(),
// status='published', and optional review notes.
func (s *Store) ApproveGuide(ctx context.Context, id, reviewerID pgtype.UUID, notes *string) error {
	const q = `
UPDATE housing_guides
SET hitl_reviewed_by = $2,
    hitl_reviewed_at = now(),
    hitl_review_notes = $3,
    status = 'published',
    updated_at = now()
WHERE id = $1
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
