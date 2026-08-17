// Package communitylocator implements the Diaspora Community Locator
// (§101, ADR-109).
//
// Community organisations are records in the community_organisations table
// — a standalone table (not part of the Content Type System) tracking
// diaspora groups (cultural associations, churches, mosques, professional
// networks, student unions, informal groups) by origin community and
// destination/city spaces. The package provides filtered listing with
// geo bounding-box search, public detail by slug, community submission
// (status='pending'), admin moderation (publish/archive), verification,
// and an organization claim workflow.
//
// Ref: japaconnect-engineering/sections/101-diaspora-community-locator.md
//
//	japaconnect-engineering/adr/ADR-109-community-locator.md
package communitylocator

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db"
)

// Organisation types (§101).
const (
	OrgTypeCulturalAssociation = "cultural_association"
	OrgTypeChurch              = "church"
	OrgTypeMosque              = "mosque"
	OrgTypeProfessionalNetwork = "professional_network"
	OrgTypeStudentUnion        = "student_union"
	OrgTypeInformalGroup       = "informal_group"
	OrgTypeOther               = "other"
)

// Claim statuses (§101).
const (
	ClaimStatusUnclaimed = "unclaimed"
	ClaimStatusPending   = "pending"
	ClaimStatusClaimed   = "claimed"
)

// Publication statuses (§101 + ADR-088 lifecycle).
const (
	StatusDraft    = "draft"
	StatusAIDraft  = "ai_draft" // ADR-088: AI-generated draft, never visible to end users
	StatusPending  = "pending"
	StatusPublished = "published"
	StatusArchived  = "archived"
)

// Store is the data-access layer for community_organisations. It uses
// direct pgx queries because community_organisations is a new table
// without sqlc-generated queries yet. No business logic here.
type Store struct {
	pool *db.DB
}

// NewStore creates a community locator store backed by the given database.
func NewStore(database *db.DB) *Store {
	return &Store{pool: database}
}

// OrgRow is the joined result of community_organisations + spaces
// (destination + city) + users (submitter), returned by list and
// get-by-slug queries.
type OrgRow struct {
	ID                   pgtype.UUID
	TenantID             pgtype.UUID
	Name                 string
	Slug                 string
	OrgType              string
	OriginCommunity      string
	DestinationSpaceID   pgtype.UUID
	DestSpaceName        *string
	DestSpaceCode        *string
	CitySpaceID          pgtype.UUID
	CitySpaceName        *string
	CitySpaceCode        *string
	Address              *string
	Latitude             *float64
	Longitude            *float64
	WebsiteURL           *string
	MeetingFrequency     *string
	ContactInfo          *string
	JapaconnectSpaceID   pgtype.UUID
	JapaconnectOrgID     pgtype.UUID
	ClaimStatus          string
	IsVerified           bool
	IsActive             bool
	IsCommunitySubmitted bool
	Status               string
	SubmittedBy          pgtype.UUID
	SubmitterName        *string
	ReviewedBy           pgtype.UUID
	ReviewedAt           pgtype.Timestamptz
	// ADR-088 universal AI content generation columns.
	AiGenerated      bool
	AiModel          *string
	AiPromptHash     *string
	HitlReviewedBy   pgtype.UUID
	HitlReviewedAt   pgtype.Timestamptz
	HitlReviewNotes  *string
	IsJapabotIndexed bool
	CreatedAt        pgtype.Timestamptz
	UpdatedAt        pgtype.Timestamptz
}

// CreateOrgInput holds the fields for inserting a new community
// organisation row.
type CreateOrgInput struct {
	TenantID             pgtype.UUID
	Name                 string
	Slug                 string
	OrgType              string
	OriginCommunity      string
	DestinationSpaceID   pgtype.UUID
	CitySpaceID          pgtype.UUID
	Address              *string
	Latitude             *float64
	Longitude            *float64
	WebsiteURL           *string
	MeetingFrequency     *string
	ContactInfo          *string
	JapaconnectSpaceID   pgtype.UUID
	JapaconnectOrgID     pgtype.UUID
	IsCommunitySubmitted bool
	SubmittedBy          pgtype.UUID
	// ADR-088 AI generation metadata (defaults to ai_generated=false for
	// community-submitted orgs).
	AiGenerated   bool
	AiModel       *string
	AiPromptHash  *string
	// Status overrides the default 'pending' insert status. Used by
	// Generate (ADR-088) to insert AI drafts as 'ai_draft' so they are
	// never visible to end users until HITL approval. Empty → 'pending'.
	Status string
}

// CreateOrg inserts a new community_organisations row. Community submissions
// default to status='pending'; AI-generated drafts (ADR-088) pass
// Status='ai_draft' so they are excluded from the public list endpoint and
// only appear in the admin HITL queue. claim_status defaults to 'unclaimed'.
func (s *Store) CreateOrg(ctx context.Context, in CreateOrgInput) (pgtype.UUID, error) {
	status := in.Status
	if status == "" {
		status = StatusPending
	}
	const q = `
INSERT INTO community_organisations (
    tenant_id, name, slug, org_type, origin_community,
    destination_space_id, city_space_id, address, latitude, longitude,
    website_url, meeting_frequency, contact_info,
    japaconnect_space_id, japaconnect_org_id,
    is_community_submitted, submitted_by, status, claim_status,
    ai_generated, ai_model, ai_prompt_hash
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10,
    $11, $12, $13, $14, $15, $16, $17, $18, 'unclaimed',
    $19, $20, $21
)
RETURNING id
`
	var id pgtype.UUID
	err := s.pool.Writer.QueryRow(ctx, q,
		in.TenantID, in.Name, in.Slug, in.OrgType, in.OriginCommunity,
		in.DestinationSpaceID, in.CitySpaceID, in.Address, in.Latitude, in.Longitude,
		in.WebsiteURL, in.MeetingFrequency, in.ContactInfo,
		in.JapaconnectSpaceID, in.JapaconnectOrgID,
		in.IsCommunitySubmitted, in.SubmittedBy, status,
		in.AiGenerated, in.AiModel, in.AiPromptHash,
	).Scan(&id)
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("communitylocator: create org: %w", err)
	}
	return id, nil
}

// ListOrgsFilter holds the filter parameters for listing community
// organisations.
type ListOrgsFilter struct {
	TenantID           pgtype.UUID
	DestinationSpaceID pgtype.UUID // optional
	CitySpaceID        pgtype.UUID // optional
	OriginCommunity    string      // optional
	OrgType            string      // optional
	Lat                *float64    // optional — proximity centre latitude
	Lng                *float64    // optional — proximity centre longitude
	RadiusKm           *float64    // optional — proximity radius in km
	Status             string      // default: published
	Limit              int32
	Offset             int32
}

// ListOrgs returns a paginated, filtered list of community organisations.
// When lat, lng, and radius (km) are all provided, results are filtered
// by the GiST <-> distance operator against point(longitude, latitude)
// (BUG-007: migration 000511 added the GiST index using PostgreSQL's
// core point type — no PostGIS required). Results are ordered nearest-
// first. When no proximity filter is given, results are ordered by
// created_at DESC as before.
func (s *Store) ListOrgs(ctx context.Context, f ListOrgsFilter) ([]OrgRow, int64, error) {
	var (
		conditions []string
		args       []any
		argIdx     = 1
	)

	conditions = append(conditions, fmt.Sprintf("co.tenant_id = $%d", argIdx))
	args = append(args, f.TenantID)
	argIdx++

	if f.Status != "" {
		conditions = append(conditions, fmt.Sprintf("co.status = $%d", argIdx))
		args = append(args, f.Status)
		argIdx++
	} else {
		conditions = append(conditions, "co.status = 'published'")
	}

	conditions = append(conditions, "co.is_active = true")
	conditions = append(conditions, "co.deleted_at IS NULL")

	if f.DestinationSpaceID.Valid {
		conditions = append(conditions, fmt.Sprintf("co.destination_space_id = $%d", argIdx))
		args = append(args, f.DestinationSpaceID)
		argIdx++
	}
	if f.CitySpaceID.Valid {
		conditions = append(conditions, fmt.Sprintf("co.city_space_id = $%d", argIdx))
		args = append(args, f.CitySpaceID)
		argIdx++
	}
	if f.OriginCommunity != "" {
		conditions = append(conditions, fmt.Sprintf("co.origin_community = $%d", argIdx))
		args = append(args, f.OriginCommunity)
		argIdx++
	}
	if f.OrgType != "" {
		conditions = append(conditions, fmt.Sprintf("co.org_type = $%d", argIdx))
		args = append(args, f.OrgType)
		argIdx++
	}

	// BUG-007: proximity filter using the GiST <-> distance operator on
	// point(longitude, latitude). The <-> operator returns Euclidean
	// distance in degrees; radius_deg = radius_km / 111.0 converts to the
	// same degree-based approximation the previous bounding-box code used.
	// The GiST index (idx_community_orgs_gist_point) accelerates both the
	// filter and the nearest-first ORDER BY.
	hasProximity := f.Lat != nil && f.Lng != nil && f.RadiusKm != nil
	if hasProximity {
		radiusDeg := *f.RadiusKm / 111.0
		conditions = append(conditions, "co.latitude IS NOT NULL AND co.longitude IS NOT NULL")
		// point(lng, lat) <-> point($lng, $lat) <= radius_deg
		conditions = append(conditions, fmt.Sprintf(
			"point(co.longitude, co.latitude) <-> point($%d, $%d) <= $%d",
			argIdx, argIdx+1, argIdx+2,
		))
		args = append(args, *f.Lng, *f.Lat, radiusDeg)
		argIdx += 3
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

	// ORDER BY: nearest-first when proximity is active (using the same
	// <-> expression so the GiST index can serve both filter and sort);
	// otherwise by created_at DESC.
	orderClause := "co.created_at DESC"
	if hasProximity {
		orderClause = fmt.Sprintf(
			"point(co.longitude, co.latitude) <-> point($%d, $%d)",
			argIdx, argIdx+1,
		)
		args = append(args, *f.Lng, *f.Lat)
		argIdx += 2
	}

	listQuery := fmt.Sprintf(`
SELECT co.id, co.tenant_id, co.name, co.slug, co.org_type, co.origin_community,
       co.destination_space_id, ds.name AS dest_space_name, ds.geo_code AS dest_space_code,
       co.city_space_id, cs.name AS city_space_name, cs.geo_code AS city_space_code,
       co.address, co.latitude, co.longitude, co.website_url, co.meeting_frequency,
       co.contact_info, co.japaconnect_space_id, co.japaconnect_org_id,
       co.claim_status, co.is_verified, co.is_active, co.is_community_submitted,
       co.status, co.submitted_by, u.username AS submitter_name,
       co.reviewed_by, co.reviewed_at,
       co.ai_generated, co.ai_model, co.ai_prompt_hash,
       co.hitl_reviewed_by, co.hitl_reviewed_at, co.hitl_review_notes,
       co.is_japabot_indexed,
       co.created_at, co.updated_at
FROM community_organisations co
LEFT JOIN spaces ds ON ds.id = co.destination_space_id
LEFT JOIN spaces cs ON cs.id = co.city_space_id
LEFT JOIN users u ON u.id = co.submitted_by
WHERE %s
ORDER BY %s
LIMIT $%d OFFSET $%d
`, whereClause, orderClause, argIdx, argIdx+1)
	args = append(args, limit, offset)

	rows, err := s.pool.Reader.Query(ctx, listQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("communitylocator: list orgs: %w", err)
	}
	defer rows.Close()

	var out []OrgRow
	for rows.Next() {
		var r OrgRow
		if err := rows.Scan(
			&r.ID, &r.TenantID, &r.Name, &r.Slug, &r.OrgType, &r.OriginCommunity,
			&r.DestinationSpaceID, &r.DestSpaceName, &r.DestSpaceCode,
			&r.CitySpaceID, &r.CitySpaceName, &r.CitySpaceCode,
			&r.Address, &r.Latitude, &r.Longitude, &r.WebsiteURL, &r.MeetingFrequency,
			&r.ContactInfo, &r.JapaconnectSpaceID, &r.JapaconnectOrgID,
			&r.ClaimStatus, &r.IsVerified, &r.IsActive, &r.IsCommunitySubmitted,
			&r.Status, &r.SubmittedBy, &r.SubmitterName,
			&r.ReviewedBy, &r.ReviewedAt,
			&r.AiGenerated, &r.AiModel, &r.AiPromptHash,
			&r.HitlReviewedBy, &r.HitlReviewedAt, &r.HitlReviewNotes,
			&r.IsJapabotIndexed,
			&r.CreatedAt, &r.UpdatedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("communitylocator: scan org row: %w", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("communitylocator: iterate org rows: %w", err)
	}

	countQuery := fmt.Sprintf(`
SELECT COUNT(*) FROM community_organisations co
WHERE %s
`, whereClause)
	countArgs := args[:len(args)-2]
	var total int64
	if err := s.pool.Reader.QueryRow(ctx, countQuery, countArgs...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("communitylocator: count orgs: %w", err)
	}

	return out, total, nil
}

// GetBySlug returns a single community organisation by its slug.
func (s *Store) GetBySlug(ctx context.Context, slug string, tenantID pgtype.UUID) (OrgRow, error) {
	const q = `
SELECT co.id, co.tenant_id, co.name, co.slug, co.org_type, co.origin_community,
       co.destination_space_id, ds.name AS dest_space_name, ds.geo_code AS dest_space_code,
       co.city_space_id, cs.name AS city_space_name, cs.geo_code AS city_space_code,
       co.address, co.latitude, co.longitude, co.website_url, co.meeting_frequency,
       co.contact_info, co.japaconnect_space_id, co.japaconnect_org_id,
       co.claim_status, co.is_verified, co.is_active, co.is_community_submitted,
       co.status, co.submitted_by, u.username AS submitter_name,
       co.reviewed_by, co.reviewed_at,
       co.ai_generated, co.ai_model, co.ai_prompt_hash,
       co.hitl_reviewed_by, co.hitl_reviewed_at, co.hitl_review_notes,
       co.is_japabot_indexed,
       co.created_at, co.updated_at
FROM community_organisations co
LEFT JOIN spaces ds ON ds.id = co.destination_space_id
LEFT JOIN spaces cs ON cs.id = co.city_space_id
LEFT JOIN users u ON u.id = co.submitted_by
WHERE co.tenant_id = $1 AND co.slug = $2 AND co.deleted_at IS NULL
LIMIT 1
`
	var r OrgRow
	err := s.pool.Reader.QueryRow(ctx, q, tenantID, slug).Scan(
		&r.ID, &r.TenantID, &r.Name, &r.Slug, &r.OrgType, &r.OriginCommunity,
		&r.DestinationSpaceID, &r.DestSpaceName, &r.DestSpaceCode,
		&r.CitySpaceID, &r.CitySpaceName, &r.CitySpaceCode,
		&r.Address, &r.Latitude, &r.Longitude, &r.WebsiteURL, &r.MeetingFrequency,
		&r.ContactInfo, &r.JapaconnectSpaceID, &r.JapaconnectOrgID,
		&r.ClaimStatus, &r.IsVerified, &r.IsActive, &r.IsCommunitySubmitted,
		&r.Status, &r.SubmittedBy, &r.SubmitterName,
		&r.ReviewedBy, &r.ReviewedAt,
		&r.AiGenerated, &r.AiModel, &r.AiPromptHash,
		&r.HitlReviewedBy, &r.HitlReviewedAt, &r.HitlReviewNotes,
		&r.IsJapabotIndexed,
		&r.CreatedAt, &r.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return OrgRow{}, pgx.ErrNoRows
		}
		return OrgRow{}, fmt.Errorf("communitylocator: get by slug: %w", err)
	}
	return r, nil
}

// GetByID returns a single community organisation by its id.
func (s *Store) GetByID(ctx context.Context, id, tenantID pgtype.UUID) (OrgRow, error) {
	const q = `
SELECT co.id, co.tenant_id, co.name, co.slug, co.org_type, co.origin_community,
       co.destination_space_id, ds.name AS dest_space_name, ds.geo_code AS dest_space_code,
       co.city_space_id, cs.name AS city_space_name, cs.geo_code AS city_space_code,
       co.address, co.latitude, co.longitude, co.website_url, co.meeting_frequency,
       co.contact_info, co.japaconnect_space_id, co.japaconnect_org_id,
       co.claim_status, co.is_verified, co.is_active, co.is_community_submitted,
       co.status, co.submitted_by, u.username AS submitter_name,
       co.reviewed_by, co.reviewed_at,
       co.ai_generated, co.ai_model, co.ai_prompt_hash,
       co.hitl_reviewed_by, co.hitl_reviewed_at, co.hitl_review_notes,
       co.is_japabot_indexed,
       co.created_at, co.updated_at
FROM community_organisations co
LEFT JOIN spaces ds ON ds.id = co.destination_space_id
LEFT JOIN spaces cs ON cs.id = co.city_space_id
LEFT JOIN users u ON u.id = co.submitted_by
WHERE co.tenant_id = $1 AND co.id = $2 AND co.deleted_at IS NULL
LIMIT 1
`
	var r OrgRow
	err := s.pool.Reader.QueryRow(ctx, q, tenantID, id).Scan(
		&r.ID, &r.TenantID, &r.Name, &r.Slug, &r.OrgType, &r.OriginCommunity,
		&r.DestinationSpaceID, &r.DestSpaceName, &r.DestSpaceCode,
		&r.CitySpaceID, &r.CitySpaceName, &r.CitySpaceCode,
		&r.Address, &r.Latitude, &r.Longitude, &r.WebsiteURL, &r.MeetingFrequency,
		&r.ContactInfo, &r.JapaconnectSpaceID, &r.JapaconnectOrgID,
		&r.ClaimStatus, &r.IsVerified, &r.IsActive, &r.IsCommunitySubmitted,
		&r.Status, &r.SubmittedBy, &r.SubmitterName,
		&r.ReviewedBy, &r.ReviewedAt,
		&r.AiGenerated, &r.AiModel, &r.AiPromptHash,
		&r.HitlReviewedBy, &r.HitlReviewedAt, &r.HitlReviewNotes,
		&r.IsJapabotIndexed,
		&r.CreatedAt, &r.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return OrgRow{}, pgx.ErrNoRows
		}
		return OrgRow{}, fmt.Errorf("communitylocator: get by id: %w", err)
	}
	return r, nil
}

// InitiateClaim sets claim_status to 'pending' on a community organisation,
// beginning the claim flow. Only published, unclaimed orgs can be claimed.
func (s *Store) InitiateClaim(ctx context.Context, id, tenantID pgtype.UUID) error {
	const q = `
UPDATE community_organisations
SET claim_status = 'pending', updated_at = now()
WHERE id = $1 AND tenant_id = $2 AND deleted_at IS NULL
  AND status = 'published' AND claim_status = 'unclaimed'
`
	cmd, err := s.pool.Writer.Exec(ctx, q, id, tenantID)
	if err != nil {
		return fmt.Errorf("communitylocator: initiate claim: %w", err)
	}
	if cmd.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

// AdminListFilter holds filter parameters for the admin moderation list.
type AdminListFilter struct {
	TenantID           pgtype.UUID
	Status             string      // optional
	Q                  string      // optional: search name
	DestinationSpaceID pgtype.UUID // optional: geo-aware ?dest= filter
	OriginCommunity    string      // optional: filter by origin community
	OrgType            string      // optional: filter by organisation type
	Sort               string      // optional: column name (allow-listed)
	Dir                string      // optional: "asc" or "desc" (default "desc")
	Limit              int32
	Offset             int32
}

// adminSortColumns is the allow-list of sortable column names for the admin
// list. Values map to the qualified SQL column expression. This prevents SQL
// injection through the sort parameter.
var adminSortColumns = map[string]string{
	"name":                   "co.name",
	"created_at":             "co.created_at",
	"updated_at":             "co.updated_at",
	"status":                 "co.status",
	"org_type":               "co.org_type",
	"origin_community":       "co.origin_community",
	"destination_space_name": "ds.name",
	"claim_status":           "co.claim_status",
	"is_verified":            "co.is_verified",
}

// adminSortDir returns "ASC" or "DESC" for the given direction string.
// Defaults to "DESC" for any non-"asc" value.
func adminSortDir(dir string) string {
	if strings.EqualFold(dir, "asc") {
		return "ASC"
	}
	return "DESC"
}

// AdminList returns all community organisations (any status) for the
// moderation queue.
func (s *Store) AdminList(ctx context.Context, f AdminListFilter) ([]OrgRow, int64, error) {
	var (
		conditions []string
		args       []any
		argIdx     = 1
	)

	conditions = append(conditions, fmt.Sprintf("co.tenant_id = $%d", argIdx))
	args = append(args, f.TenantID)
	argIdx++

	conditions = append(conditions, "co.deleted_at IS NULL")

	if f.Status != "" {
		conditions = append(conditions, fmt.Sprintf("co.status = $%d", argIdx))
		args = append(args, f.Status)
		argIdx++
	}
	if f.DestinationSpaceID.Valid {
		conditions = append(conditions, fmt.Sprintf("co.destination_space_id = $%d", argIdx))
		args = append(args, f.DestinationSpaceID)
		argIdx++
	}
	if f.OriginCommunity != "" {
		conditions = append(conditions, fmt.Sprintf("co.origin_community = $%d", argIdx))
		args = append(args, f.OriginCommunity)
		argIdx++
	}
	if f.OrgType != "" {
		conditions = append(conditions, fmt.Sprintf("co.org_type = $%d", argIdx))
		args = append(args, f.OrgType)
		argIdx++
	}
	if f.Q != "" {
		conditions = append(conditions, fmt.Sprintf("co.name ILIKE $%d", argIdx))
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

	// Build ORDER BY from allow-listed sort column + direction.
	orderExpr := "co.created_at DESC" // default
	if col, ok := adminSortColumns[f.Sort]; ok {
		orderExpr = col + " " + adminSortDir(f.Dir)
	}

	listQuery := fmt.Sprintf(`
SELECT co.id, co.tenant_id, co.name, co.slug, co.org_type, co.origin_community,
       co.destination_space_id, ds.name AS dest_space_name, ds.geo_code AS dest_space_code,
       co.city_space_id, cs.name AS city_space_name, cs.geo_code AS city_space_code,
       co.address, co.latitude, co.longitude, co.website_url, co.meeting_frequency,
       co.contact_info, co.japaconnect_space_id, co.japaconnect_org_id,
       co.claim_status, co.is_verified, co.is_active, co.is_community_submitted,
       co.status, co.submitted_by, u.username AS submitter_name,
       co.reviewed_by, co.reviewed_at,
       co.ai_generated, co.ai_model, co.ai_prompt_hash,
       co.hitl_reviewed_by, co.hitl_reviewed_at, co.hitl_review_notes,
       co.is_japabot_indexed,
       co.created_at, co.updated_at
FROM community_organisations co
LEFT JOIN spaces ds ON ds.id = co.destination_space_id
LEFT JOIN spaces cs ON cs.id = co.city_space_id
LEFT JOIN users u ON u.id = co.submitted_by
WHERE %s
ORDER BY %s
LIMIT $%d OFFSET $%d
`, whereClause, orderExpr, argIdx, argIdx+1)
	args = append(args, limit, offset)

	rows, err := s.pool.Reader.Query(ctx, listQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("communitylocator: admin list orgs: %w", err)
	}
	defer rows.Close()

	var out []OrgRow
	for rows.Next() {
		var r OrgRow
		if err := rows.Scan(
			&r.ID, &r.TenantID, &r.Name, &r.Slug, &r.OrgType, &r.OriginCommunity,
			&r.DestinationSpaceID, &r.DestSpaceName, &r.DestSpaceCode,
			&r.CitySpaceID, &r.CitySpaceName, &r.CitySpaceCode,
			&r.Address, &r.Latitude, &r.Longitude, &r.WebsiteURL, &r.MeetingFrequency,
			&r.ContactInfo, &r.JapaconnectSpaceID, &r.JapaconnectOrgID,
			&r.ClaimStatus, &r.IsVerified, &r.IsActive, &r.IsCommunitySubmitted,
			&r.Status, &r.SubmittedBy, &r.SubmitterName,
			&r.ReviewedBy, &r.ReviewedAt,
			&r.AiGenerated, &r.AiModel, &r.AiPromptHash,
			&r.HitlReviewedBy, &r.HitlReviewedAt, &r.HitlReviewNotes,
			&r.IsJapabotIndexed,
			&r.CreatedAt, &r.UpdatedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("communitylocator: admin scan org row: %w", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("communitylocator: admin iterate org rows: %w", err)
	}

	countQuery := fmt.Sprintf(`
SELECT COUNT(*) FROM community_organisations co
WHERE %s
`, whereClause)
	countArgs := args[:len(args)-2]
	var total int64
	if err := s.pool.Reader.QueryRow(ctx, countQuery, countArgs...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("communitylocator: admin count orgs: %w", err)
	}

	return out, total, nil
}

// ApproveOrg publishes a community organisation: sets status='published',
// records the reviewer and review timestamp. For AI-generated entries
// (ADR-088), also sets hitl_reviewed_by and hitl_reviewed_at — the
// approve action IS the HITL review gate. Accepts both 'pending'
// (community submission) and 'ai_draft' (ADR-088 AI draft) sources so
// the HITL gate can publish AI-generated drafts. Archived or already-
// published orgs are not re-approvable.
func (s *Store) ApproveOrg(ctx context.Context, id, tenantID, reviewedBy pgtype.UUID) error {
	const q = `
UPDATE community_organisations
SET status = 'published',
    reviewed_by = $3,
    reviewed_at = now(),
    hitl_reviewed_by = CASE WHEN ai_generated = true THEN $3 ELSE hitl_reviewed_by END,
    hitl_reviewed_at = CASE WHEN ai_generated = true THEN now() ELSE hitl_reviewed_at END,
    updated_at = now()
WHERE id = $1 AND tenant_id = $2 AND deleted_at IS NULL AND status IN ('pending','ai_draft')
`
	cmd, err := s.pool.Writer.Exec(ctx, q, id, tenantID, reviewedBy)
	if err != nil {
		return fmt.Errorf("communitylocator: approve org: %w", err)
	}
	if cmd.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

// ArchiveOrg archives a published community organisation: sets
// status='archived', is_active=false. Only published orgs can be
// archived.
func (s *Store) ArchiveOrg(ctx context.Context, id, tenantID pgtype.UUID) error {
	const q = `
UPDATE community_organisations
SET status = 'archived', is_active = false, updated_at = now()
WHERE id = $1 AND tenant_id = $2 AND deleted_at IS NULL AND status = 'published'
`
	cmd, err := s.pool.Writer.Exec(ctx, q, id, tenantID)
	if err != nil {
		return fmt.Errorf("communitylocator: archive org: %w", err)
	}
	if cmd.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

// DeleteOrg soft-deletes a community organisation: sets deleted_at=now()
// per Rule 11 (soft deletes only for user content). The org is excluded
// from all list/get queries via the deleted_at IS NULL filter.
func (s *Store) DeleteOrg(ctx context.Context, id, tenantID pgtype.UUID) error {
	const q = `
UPDATE community_organisations
SET deleted_at = now(), is_active = false, updated_at = now()
WHERE id = $1 AND tenant_id = $2 AND deleted_at IS NULL
`
	cmd, err := s.pool.Writer.Exec(ctx, q, id, tenantID)
	if err != nil {
		return fmt.Errorf("communitylocator: delete org: %w", err)
	}
	if cmd.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

// VerifyOrg marks a community organisation as verified by an admin. Only
// published orgs can be verified (prevents verifying pending/archived orgs).
func (s *Store) VerifyOrg(ctx context.Context, id, tenantID pgtype.UUID) error {
	const q = `
UPDATE community_organisations
SET is_verified = true, updated_at = now()
WHERE id = $1 AND tenant_id = $2 AND deleted_at IS NULL AND status = 'published'
`
	cmd, err := s.pool.Writer.Exec(ctx, q, id, tenantID)
	if err != nil {
		return fmt.Errorf("communitylocator: verify org: %w", err)
	}
	if cmd.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

// ResolveClaim resolves a pending claim on a community organisation.
// Approving sets claim_status to 'claimed'; rejecting resets it to
// 'unclaimed'. Only orgs with claim_status='pending' can be resolved.
func (s *Store) ResolveClaim(ctx context.Context, id, tenantID pgtype.UUID, approve bool) error {
	status := ClaimStatusUnclaimed
	if approve {
		status = ClaimStatusClaimed
	}
	const q = `
UPDATE community_organisations
SET claim_status = $3, updated_at = now()
WHERE id = $1 AND tenant_id = $2 AND deleted_at IS NULL AND claim_status = 'pending'
`
	cmd, err := s.pool.Writer.Exec(ctx, q, id, tenantID, status)
	if err != nil {
		return fmt.Errorf("communitylocator: resolve claim: %w", err)
	}
	if cmd.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}
