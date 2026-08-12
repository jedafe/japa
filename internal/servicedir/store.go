// Package servicedir implements the Service Directory (§98, ADR-106).
//
// Service listings are records in the contents table with
// content_type = 'service_listing', extended by the
// service_listing_meta companion table for structured fields
// (service category, destination/city spaces, origin communities,
// remote flag, website/booking URLs, price range, verification,
// and organization claim tracking). The package provides CRUD,
// filtered listing, admin verification, and claim workflow —
// inheriting reactions, comments, bookmarks, space filing, and
// analytics from the Content Type System (§14).
//
// Ref: japaconnect-engineering/sections/98-service-directory.md
//      japaconnect-engineering/adr/ADR-106-service-directory.md
package servicedir

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db"
)

// Service categories (§98 schema: immigration_legal|relocation|finance|education|health|community|other).
const (
	CategoryImmigrationLegal = "immigration_legal"
	CategoryRelocation       = "relocation"
	CategoryFinance          = "finance"
	CategoryEducation        = "education"
	CategoryHealth           = "health"
	CategoryCommunity        = "community"
	CategoryOther            = "other"
)

// validCategories is the canonical set used for input validation (BUG-013).
var validCategories = map[string]struct{}{
	CategoryImmigrationLegal: {},
	CategoryRelocation:       {},
	CategoryFinance:          {},
	CategoryEducation:        {},
	CategoryHealth:           {},
	CategoryCommunity:        {},
	CategoryOther:            {},
}

// IsValidCategory reports whether s is a canonical §98 service_category.
func IsValidCategory(s string) bool {
	_, ok := validCategories[s]
	return ok
}

// Claim statuses (§98 schema: unclaimed|pending|approved|rejected).
const (
	ClaimStatusUnclaimed = "unclaimed"
	ClaimStatusPending   = "pending"
	ClaimStatusApproved  = "approved"
	ClaimStatusRejected  = "rejected"
)

// Verification types (§98 schema: self|community|admin|professional_body).
const (
	VerificationTypeSelf             = "self"
	VerificationTypeCommunity        = "community"
	VerificationTypeAdmin            = "admin"
	VerificationTypeProfessionalBody = "professional_body"
)

// Store is the data-access layer for service_listing_meta and service
// listing queries. It uses direct pgx queries because
// service_listing_meta is a new table without sqlc-generated queries
// yet. No business logic here.
type Store struct {
	pool *db.DB
}

// NewStore creates a service directory store backed by the given database.
func NewStore(database *db.DB) *Store {
	return &Store{pool: database}
}

// ServiceMeta holds the structured fields for a single service listing.
type ServiceMeta struct {
	ID                  pgtype.UUID
	TenantID            pgtype.UUID
	ContentID           pgtype.UUID
	OrgID               pgtype.UUID
	ServiceCategory     *string
	DestinationSpaceID  pgtype.UUID
	CitySpaceID         pgtype.UUID
	OriginCommunities   []string // §98 schema: text[]
	IsRemote            bool
	WebsiteURL          *string
	BookingURL          *string
	PriceRange          *string
	IsVerified          bool
	VerificationType    *string
	ClaimStatus         *string
	ClaimedByOrgID      pgtype.UUID
	VerifiedBy          pgtype.UUID
	VerifiedAt          pgtype.Timestamptz
	CreatedAt           pgtype.Timestamptz
	UpdatedAt           pgtype.Timestamptz
}

// ServiceRow is the joined result of contents + service_listing_meta +
// spaces + organizations, returned by list and get-by-slug queries.
type ServiceRow struct {
	ID                 pgtype.UUID
	TenantID           pgtype.UUID
	AuthorID           pgtype.UUID
	Title              string
	Slug               string
	Body               string
	BodyHTML           string
	Status             string
	Visibility         string
	ViewCount          int32
	CommentCount       int32
	PublishedAt        pgtype.Timestamptz
	CreatedAt          pgtype.Timestamptz
	UpdatedAt          pgtype.Timestamptz
	OrgID              pgtype.UUID
	OrgName            *string
	OrgSlug            *string
	ServiceCategory    *string
	DestSpaceID        pgtype.UUID
	DestSpaceName      *string
	DestSpaceCode      *string
	CitySpaceID        pgtype.UUID
	CitySpaceName      *string
	CitySpaceCode      *string
	OriginCommunities  []string
	IsRemote           bool
	WebsiteURL         *string
	BookingURL         *string
	PriceRange         *string
	IsVerified         bool
	VerificationType   *string
	ClaimStatus        *string
	ClaimedByOrgID     pgtype.UUID
	ClaimedOrgName     *string
	VerifiedBy         pgtype.UUID
	VerifiedAt         pgtype.Timestamptz
	AuthorName         *string
	AuthorAvatar       *string
}

// CreateMeta inserts a new service_listing_meta row linked to a contents row.
func (s *Store) CreateMeta(ctx context.Context, meta ServiceMeta) error {
	const q = `
INSERT INTO service_listing_meta (
    tenant_id, content_id, org_id, service_category, destination_space_id,
    city_space_id, origin_communities, is_remote, website_url, booking_url,
    price_range, is_verified, verification_type, claim_status, claimed_by_org_id
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
`
	originCommunities := meta.OriginCommunities
	if originCommunities == nil {
		originCommunities = []string{}
	}
	claimStatus := meta.ClaimStatus
	if claimStatus == nil {
		s := ClaimStatusUnclaimed
		claimStatus = &s
	}
	_, err := s.pool.Writer.Exec(ctx, q,
		meta.TenantID, meta.ContentID, meta.OrgID, meta.ServiceCategory,
		meta.DestinationSpaceID, meta.CitySpaceID, originCommunities,
		meta.IsRemote, meta.WebsiteURL, meta.BookingURL, meta.PriceRange,
		meta.IsVerified, meta.VerificationType, claimStatus, meta.ClaimedByOrgID,
	)
	if err != nil {
		return fmt.Errorf("servicedir: create service_listing_meta: %w", err)
	}
	return nil
}

// GetMeta retrieves the service_listing_meta row for a given content_id.
func (s *Store) GetMeta(ctx context.Context, contentID, tenantID pgtype.UUID) (ServiceMeta, error) {
	const q = `
SELECT id, tenant_id, content_id, org_id, service_category, destination_space_id,
       city_space_id, origin_communities, is_remote, website_url, booking_url,
       price_range, is_verified, verification_type, claim_status, claimed_by_org_id,
       verified_by, verified_at,
       created_at, updated_at
FROM service_listing_meta
WHERE content_id = $1 AND tenant_id = $2
`
	var m ServiceMeta
	err := s.pool.Reader.QueryRow(ctx, q, contentID, tenantID).Scan(
		&m.ID, &m.TenantID, &m.ContentID, &m.OrgID, &m.ServiceCategory,
		&m.DestinationSpaceID, &m.CitySpaceID, &m.OriginCommunities,
		&m.IsRemote, &m.WebsiteURL, &m.BookingURL, &m.PriceRange,
		&m.IsVerified, &m.VerificationType, &m.ClaimStatus, &m.ClaimedByOrgID,
		&m.VerifiedBy, &m.VerifiedAt,
		&m.CreatedAt, &m.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return ServiceMeta{}, pgx.ErrNoRows
		}
		return ServiceMeta{}, fmt.Errorf("servicedir: get service_listing_meta: %w", err)
	}
	return m, nil
}

// UpdateMeta updates the mutable structured fields on a service_listing_meta
// row. Called when the author edits their service listing.
func (s *Store) UpdateMeta(ctx context.Context, contentID, tenantID pgtype.UUID, meta ServiceMeta) error {
	originCommunities := meta.OriginCommunities
	if originCommunities == nil {
		originCommunities = []string{}
	}
	const q = `
UPDATE service_listing_meta
SET org_id = $3,
    service_category = $4,
    destination_space_id = $5,
    city_space_id = $6,
    origin_communities = $7,
    is_remote = $8,
    website_url = $9,
    booking_url = $10,
    price_range = $11,
    updated_at = now()
WHERE content_id = $1 AND tenant_id = $2
`
	_, err := s.pool.Writer.Exec(ctx, q,
		contentID, tenantID,
		meta.OrgID, meta.ServiceCategory, meta.DestinationSpaceID,
		meta.CitySpaceID, originCommunities, meta.IsRemote,
		meta.WebsiteURL, meta.BookingURL, meta.PriceRange,
	)
	if err != nil {
		return fmt.Errorf("servicedir: update service_listing_meta: %w", err)
	}
	return nil
}

// ListServicesFilter holds the filter parameters for listing services.
type ListServicesFilter struct {
	TenantID        pgtype.UUID
	DestSpaceID     pgtype.UUID // optional, ?dest=
	CitySpaceID     pgtype.UUID // optional, ?city= (BUG-005)
	ServiceCategory string      // optional, ?category=
	OriginCommunity string      // optional, ?origin= (BUG-005) — single community tag, matched via array contains
	IsVerified      *bool       // optional, ?verified=
	Sort            string      // "top" | "latest" (default: latest)
	Status          string      // filter by status (default: published)
	Limit           int32
	Offset          int32
}

// ListServices returns a paginated, filtered list of published service
// listings. The query joins contents + service_listing_meta + spaces
// (destination + city) + organizations + users (author name/avatar).
// Sort "top" orders by view_count + comment_count; "latest" orders by
// published_at DESC.
func (s *Store) ListServices(ctx context.Context, f ListServicesFilter) ([]ServiceRow, int64, error) {
	var (
		conditions []string
		args       []any
		argIdx     = 1
	)

	conditions = append(conditions, fmt.Sprintf("c.tenant_id = $%d", argIdx))
	args = append(args, f.TenantID)
	argIdx++

	conditions = append(conditions, fmt.Sprintf("c.content_type = $%d", argIdx))
	args = append(args, "service_listing")
	argIdx++

	if f.Status != "" {
		conditions = append(conditions, fmt.Sprintf("c.status = $%d", argIdx))
		args = append(args, f.Status)
		argIdx++
	} else {
		conditions = append(conditions, "c.status = 'published'")
	}

	conditions = append(conditions, "c.deleted_at IS NULL")

	if f.DestSpaceID.Valid {
		conditions = append(conditions, fmt.Sprintf("sm.destination_space_id = $%d", argIdx))
		args = append(args, f.DestSpaceID)
		argIdx++
	}
	if f.CitySpaceID.Valid {
		conditions = append(conditions, fmt.Sprintf("sm.city_space_id = $%d", argIdx))
		args = append(args, f.CitySpaceID)
		argIdx++
	}
	if f.ServiceCategory != "" {
		conditions = append(conditions, fmt.Sprintf("sm.service_category = $%d", argIdx))
		args = append(args, f.ServiceCategory)
		argIdx++
	}
	if f.OriginCommunity != "" {
		// text[] containment: provider serves the requested community.
		conditions = append(conditions, fmt.Sprintf("$%d = ANY(sm.origin_communities)", argIdx))
		args = append(args, f.OriginCommunity)
		argIdx++
	}
	if f.IsVerified != nil {
		conditions = append(conditions, fmt.Sprintf("sm.is_verified = $%d", argIdx))
		args = append(args, *f.IsVerified)
		argIdx++
	}

	whereClause := strings.Join(conditions, " AND ")

	orderClause := "c.published_at DESC NULLS LAST, c.created_at DESC"
	if f.Sort == "top" {
		orderClause = "c.view_count DESC, c.comment_count DESC, c.published_at DESC"
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
SELECT c.id, c.tenant_id, c.author_id, c.title, c.slug, c.body, c.body_html,
       c.status, c.visibility, c.view_count, c.comment_count,
       c.published_at, c.created_at, c.updated_at,
       sm.org_id, o.name AS org_name, o.slug AS org_slug,
       sm.service_category,
       sm.destination_space_id, ds.name AS dest_space_name, ds.geo_code AS dest_space_code,
       sm.city_space_id, cs.name AS city_space_name, cs.geo_code AS city_space_code,
       sm.origin_communities, sm.is_remote, sm.website_url, sm.booking_url,
       sm.price_range, sm.is_verified, sm.verification_type,
       sm.claim_status, sm.claimed_by_org_id, co.name AS claimed_org_name,
       sm.verified_by, sm.verified_at,
       u.username AS author_name, up.avatar_url AS author_avatar
FROM contents c
JOIN service_listing_meta sm ON sm.content_id = c.id AND sm.tenant_id = c.tenant_id
LEFT JOIN organizations o ON o.id = sm.org_id
LEFT JOIN spaces ds ON ds.id = sm.destination_space_id
LEFT JOIN spaces cs ON cs.id = sm.city_space_id
LEFT JOIN organizations co ON co.id = sm.claimed_by_org_id
LEFT JOIN users u ON u.id = c.author_id
LEFT JOIN user_profiles up ON up.user_id = c.author_id
WHERE %s
ORDER BY %s
LIMIT $%d OFFSET $%d
`, whereClause, orderClause, argIdx, argIdx+1)
	args = append(args, limit, offset)

	rows, err := s.pool.Reader.Query(ctx, listQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("servicedir: list services: %w", err)
	}
	defer rows.Close()

	var out []ServiceRow
	for rows.Next() {
		var r ServiceRow
		if err := rows.Scan(
			&r.ID, &r.TenantID, &r.AuthorID, &r.Title, &r.Slug, &r.Body, &r.BodyHTML,
			&r.Status, &r.Visibility, &r.ViewCount, &r.CommentCount,
			&r.PublishedAt, &r.CreatedAt, &r.UpdatedAt,
			&r.OrgID, &r.OrgName, &r.OrgSlug,
			&r.ServiceCategory,
			&r.DestSpaceID, &r.DestSpaceName, &r.DestSpaceCode,
			&r.CitySpaceID, &r.CitySpaceName, &r.CitySpaceCode,
			&r.OriginCommunities, &r.IsRemote, &r.WebsiteURL, &r.BookingURL,
			&r.PriceRange, &r.IsVerified, &r.VerificationType,
			&r.ClaimStatus, &r.ClaimedByOrgID, &r.ClaimedOrgName,
			&r.VerifiedBy, &r.VerifiedAt,
			&r.AuthorName, &r.AuthorAvatar,
		); err != nil {
			return nil, 0, fmt.Errorf("servicedir: scan service row: %w", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("servicedir: iterate service rows: %w", err)
	}

	countQuery := fmt.Sprintf(`
SELECT COUNT(*) FROM contents c
JOIN service_listing_meta sm ON sm.content_id = c.id AND sm.tenant_id = c.tenant_id
WHERE %s
`, whereClause)
	countArgs := args[:len(args)-2]
	var total int64
	if err := s.pool.Reader.QueryRow(ctx, countQuery, countArgs...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("servicedir: count services: %w", err)
	}

	return out, total, nil
}

// GetBySlug returns a single service listing by its slug, joining contents +
// service_listing_meta + spaces + organizations + users.
func (s *Store) GetBySlug(ctx context.Context, slug string, tenantID pgtype.UUID) (ServiceRow, error) {
	const q = `
SELECT c.id, c.tenant_id, c.author_id, c.title, c.slug, c.body, c.body_html,
       c.status, c.visibility, c.view_count, c.comment_count,
       c.published_at, c.created_at, c.updated_at,
       sm.org_id, o.name AS org_name, o.slug AS org_slug,
       sm.service_category,
       sm.destination_space_id, ds.name AS dest_space_name, ds.geo_code AS dest_space_code,
       sm.city_space_id, cs.name AS city_space_name, cs.geo_code AS city_space_code,
       sm.origin_communities, sm.is_remote, sm.website_url, sm.booking_url,
       sm.price_range, sm.is_verified, sm.verification_type,
       sm.claim_status, sm.claimed_by_org_id, co.name AS claimed_org_name,
       sm.verified_by, sm.verified_at,
       u.username AS author_name, up.avatar_url AS author_avatar
FROM contents c
JOIN service_listing_meta sm ON sm.content_id = c.id AND sm.tenant_id = c.tenant_id
LEFT JOIN organizations o ON o.id = sm.org_id
LEFT JOIN spaces ds ON ds.id = sm.destination_space_id
LEFT JOIN spaces cs ON cs.id = sm.city_space_id
LEFT JOIN organizations co ON co.id = sm.claimed_by_org_id
LEFT JOIN users u ON u.id = c.author_id
LEFT JOIN user_profiles up ON up.user_id = c.author_id
WHERE c.tenant_id = $1 AND c.slug = $2 AND c.content_type = 'service_listing'
  AND c.deleted_at IS NULL
LIMIT 1
`
	var r ServiceRow
	err := s.pool.Reader.QueryRow(ctx, q, tenantID, slug).Scan(
		&r.ID, &r.TenantID, &r.AuthorID, &r.Title, &r.Slug, &r.Body, &r.BodyHTML,
		&r.Status, &r.Visibility, &r.ViewCount, &r.CommentCount,
		&r.PublishedAt, &r.CreatedAt, &r.UpdatedAt,
		&r.OrgID, &r.OrgName, &r.OrgSlug,
		&r.ServiceCategory,
		&r.DestSpaceID, &r.DestSpaceName, &r.DestSpaceCode,
		&r.CitySpaceID, &r.CitySpaceName, &r.CitySpaceCode,
		&r.OriginCommunities, &r.IsRemote, &r.WebsiteURL, &r.BookingURL,
		&r.PriceRange, &r.IsVerified, &r.VerificationType,
		&r.ClaimStatus, &r.ClaimedByOrgID, &r.ClaimedOrgName,
		&r.VerifiedBy, &r.VerifiedAt,
		&r.AuthorName, &r.AuthorAvatar,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return ServiceRow{}, pgx.ErrNoRows
		}
		return ServiceRow{}, fmt.Errorf("servicedir: get by slug: %w", err)
	}
	return r, nil
}

// AdminListFilter holds filter parameters for the admin moderation list.
type AdminListFilter struct {
	TenantID        pgtype.UUID
	Status          string // optional: filter by content status
	IsVerified      *bool  // optional: filter by verified state
	ClaimStatus     string // optional: filter by claim status
	ServiceCategory string // optional: filter by service category
	Q               string // optional: search title
	Limit           int32
	Offset          int32
}

// AdminList returns all service listings (any status) for the moderation
// queue.
func (s *Store) AdminList(ctx context.Context, f AdminListFilter) ([]ServiceRow, int64, error) {
	var (
		conditions []string
		args       []any
		argIdx     = 1
	)

	conditions = append(conditions, fmt.Sprintf("c.tenant_id = $%d", argIdx))
	args = append(args, f.TenantID)
	argIdx++

	conditions = append(conditions, fmt.Sprintf("c.content_type = $%d", argIdx))
	args = append(args, "service_listing")
	argIdx++

	conditions = append(conditions, "c.deleted_at IS NULL")

	if f.Status != "" {
		conditions = append(conditions, fmt.Sprintf("c.status = $%d", argIdx))
		args = append(args, f.Status)
		argIdx++
	}
	if f.IsVerified != nil {
		conditions = append(conditions, fmt.Sprintf("sm.is_verified = $%d", argIdx))
		args = append(args, *f.IsVerified)
		argIdx++
	}
	if f.ClaimStatus != "" {
		conditions = append(conditions, fmt.Sprintf("sm.claim_status = $%d", argIdx))
		args = append(args, f.ClaimStatus)
		argIdx++
	}
	if f.ServiceCategory != "" {
		conditions = append(conditions, fmt.Sprintf("sm.service_category = $%d", argIdx))
		args = append(args, f.ServiceCategory)
		argIdx++
	}
	if f.Q != "" {
		conditions = append(conditions, fmt.Sprintf("c.title ILIKE $%d", argIdx))
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
SELECT c.id, c.tenant_id, c.author_id, c.title, c.slug, c.body, c.body_html,
       c.status, c.visibility, c.view_count, c.comment_count,
       c.published_at, c.created_at, c.updated_at,
       sm.org_id, o.name AS org_name, o.slug AS org_slug,
       sm.service_category,
       sm.destination_space_id, ds.name AS dest_space_name, ds.geo_code AS dest_space_code,
       sm.city_space_id, cs.name AS city_space_name, cs.geo_code AS city_space_code,
       sm.origin_communities, sm.is_remote, sm.website_url, sm.booking_url,
       sm.price_range, sm.is_verified, sm.verification_type,
       sm.claim_status, sm.claimed_by_org_id, co.name AS claimed_org_name,
       sm.verified_by, sm.verified_at,
       u.username AS author_name, up.avatar_url AS author_avatar
FROM contents c
JOIN service_listing_meta sm ON sm.content_id = c.id AND sm.tenant_id = c.tenant_id
LEFT JOIN organizations o ON o.id = sm.org_id
LEFT JOIN spaces ds ON ds.id = sm.destination_space_id
LEFT JOIN spaces cs ON cs.id = sm.city_space_id
LEFT JOIN organizations co ON co.id = sm.claimed_by_org_id
LEFT JOIN users u ON u.id = c.author_id
LEFT JOIN user_profiles up ON up.user_id = c.author_id
WHERE %s
ORDER BY c.created_at DESC
LIMIT $%d OFFSET $%d
`, whereClause, argIdx, argIdx+1)
	args = append(args, limit, offset)

	rows, err := s.pool.Reader.Query(ctx, listQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("servicedir: admin list services: %w", err)
	}
	defer rows.Close()

	var out []ServiceRow
	for rows.Next() {
		var r ServiceRow
		if err := rows.Scan(
			&r.ID, &r.TenantID, &r.AuthorID, &r.Title, &r.Slug, &r.Body, &r.BodyHTML,
			&r.Status, &r.Visibility, &r.ViewCount, &r.CommentCount,
			&r.PublishedAt, &r.CreatedAt, &r.UpdatedAt,
			&r.OrgID, &r.OrgName, &r.OrgSlug,
			&r.ServiceCategory,
			&r.DestSpaceID, &r.DestSpaceName, &r.DestSpaceCode,
			&r.CitySpaceID, &r.CitySpaceName, &r.CitySpaceCode,
			&r.OriginCommunities, &r.IsRemote, &r.WebsiteURL, &r.BookingURL,
			&r.PriceRange, &r.IsVerified, &r.VerificationType,
			&r.ClaimStatus, &r.ClaimedByOrgID, &r.ClaimedOrgName,
			&r.VerifiedBy, &r.VerifiedAt,
			&r.AuthorName, &r.AuthorAvatar,
		); err != nil {
			return nil, 0, fmt.Errorf("servicedir: admin scan service row: %w", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("servicedir: admin iterate service rows: %w", err)
	}

	countQuery := fmt.Sprintf(`
SELECT COUNT(*) FROM contents c
JOIN service_listing_meta sm ON sm.content_id = c.id AND sm.tenant_id = c.tenant_id
WHERE %s
`, whereClause)
	countArgs := args[:len(args)-2]
	var total int64
	if err := s.pool.Reader.QueryRow(ctx, countQuery, countArgs...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("servicedir: admin count services: %w", err)
	}

	return out, total, nil
}

// VerifyService marks a service listing as verified by an admin, recording
// the verification type (§98: self|community|admin|professional_body) and
// the admin user_id + timestamp for audit (ADR-088 HITL, BUG-015).
func (s *Store) VerifyService(ctx context.Context, contentID, tenantID, verifiedBy pgtype.UUID, verificationType string) error {
	const q = `
UPDATE service_listing_meta
SET is_verified = true,
    verification_type = $4,
    verified_by = $3,
    verified_at = now(),
    updated_at = now()
WHERE content_id = $1 AND tenant_id = $2
`
	cmd, err := s.pool.Writer.Exec(ctx, q, contentID, tenantID, verifiedBy, verificationType)
	if err != nil {
		return fmt.Errorf("servicedir: verify service: %w", err)
	}
	if cmd.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

// UpdateClaimStatus updates the claim_status and claimed_by_org_id on a
// service_listing_meta row. Used to initiate, approve, or reject an
// organization claim (§98).
func (s *Store) UpdateClaimStatus(ctx context.Context, contentID, tenantID pgtype.UUID, claimStatus string, orgID pgtype.UUID) error {
	const q = `
UPDATE service_listing_meta
SET claim_status = $3, claimed_by_org_id = $4, updated_at = now()
WHERE content_id = $1 AND tenant_id = $2
`
	cmd, err := s.pool.Writer.Exec(ctx, q, contentID, tenantID, claimStatus, orgID)
	if err != nil {
		return fmt.Errorf("servicedir: update claim status: %w", err)
	}
	if cmd.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}
