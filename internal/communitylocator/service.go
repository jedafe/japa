// Package communitylocator — service.go orchestrates community
// organisation submission, filtered listing with geo bounding-box search,
// admin moderation (publish/archive), verification, and organization
// claim workflow. Feature-flagged via community_locator.enabled
// system_setting (§101).
//
// Ref: japaconnect-engineering/sections/101-diaspora-community-locator.md
//      japaconnect-engineering/adr/ADR-109-community-locator.md
package communitylocator

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/config"
)

// OrgDTO is the API response shape for a single community organisation.
type OrgDTO struct {
	ID                   string  `json:"id"`
	Name                 string  `json:"name"`
	Slug                 string  `json:"slug"`
	OrgType              string  `json:"org_type"`
	OriginCommunity      string  `json:"origin_community"`
	DestinationSpaceID   string  `json:"destination_space_id"`
	DestSpaceName        string  `json:"destination_space_name,omitempty"`
	DestSpaceCode        string  `json:"destination_space_code,omitempty"`
	CitySpaceID          string  `json:"city_space_id,omitempty"`
	CitySpaceName        string  `json:"city_space_name,omitempty"`
	CitySpaceCode        string  `json:"city_space_code,omitempty"`
	Address              string  `json:"address,omitempty"`
	Latitude             float64 `json:"latitude,omitempty"`
	Longitude            float64 `json:"longitude,omitempty"`
	WebsiteURL           string  `json:"website_url,omitempty"`
	MeetingFrequency     string  `json:"meeting_frequency,omitempty"`
	ContactInfo          string  `json:"contact_info,omitempty"`
	JapaconnectSpaceID   string  `json:"japaconnect_space_id,omitempty"`
	JapaconnectOrgID     string  `json:"japaconnect_org_id,omitempty"`
	ClaimStatus          string  `json:"claim_status"`
	IsVerified           bool    `json:"is_verified"`
	IsActive             bool    `json:"is_active"`
	IsCommunitySubmitted bool    `json:"is_community_submitted"`
	Status               string  `json:"status"`
	SubmittedBy          string  `json:"submitted_by,omitempty"`
	SubmitterName        string  `json:"submitter_name,omitempty"`
	ReviewedBy           string  `json:"reviewed_by,omitempty"`
	ReviewedAt           string  `json:"reviewed_at,omitempty"`
	// ADR-088 universal AI content generation metadata.
	AiGenerated      bool   `json:"ai_generated"`
	AiModel          string `json:"ai_model,omitempty"`
	AiPromptHash     string `json:"ai_prompt_hash,omitempty"`
	HitlReviewedBy   string `json:"hitl_reviewed_by,omitempty"`
	HitlReviewedAt   string `json:"hitl_reviewed_at,omitempty"`
	HitlReviewNotes  string `json:"hitl_review_notes,omitempty"`
	IsJapabotIndexed bool   `json:"is_japabot_indexed"`
	CreatedAt        string `json:"created_at"`
	UpdatedAt        string `json:"updated_at"`
}

// SubmitInput is the request body for POST /community-locator.
type SubmitInput struct {
	Name               string  `json:"name"`
	Slug               string  `json:"slug"`
	OrgType            string  `json:"org_type"`
	OriginCommunity    string  `json:"origin_community"`
	DestinationSpaceID string  `json:"destination_space_id"`
	CitySpaceID        string  `json:"city_space_id"`
	Address            string  `json:"address"`
	Latitude           float64 `json:"latitude"`
	Longitude          float64 `json:"longitude"`
	WebsiteURL         string  `json:"website_url"`
	MeetingFrequency   string  `json:"meeting_frequency"`
	ContactInfo        string  `json:"contact_info"`
	JapaconnectSpaceID string  `json:"japaconnect_space_id"`
	JapaconnectOrgID   string  `json:"japaconnect_org_id"`
}

// Service holds the business logic for the Diaspora Community Locator.
type Service struct {
	store    *Store
	settings *config.Settings
}

// NewService creates a community locator service.
func NewService(store *Store, settings *config.Settings) *Service {
	return &Service{
		store:    store,
		settings: settings,
	}
}

// Enabled reads the community_locator.enabled system_setting toggle
// (§101 feature flag).
func (s *Service) Enabled(ctx context.Context, tenantID pgtype.UUID) bool {
	if s.settings == nil {
		return false
	}
	return s.settings.GetBoolOr(ctx, tenantID, "community_locator.enabled", false)
}

// ErrDuplicateSlug is returned when a submission's slug already exists for
// the tenant. The handler maps this to 409 duplicate_slug.
var ErrDuplicateSlug = errors.New("duplicate slug")

// validOrgTypes is the allow-list of organisation types per §101.
var validOrgTypes = map[string]bool{
	"cultural_association":  true,
	"church":                true,
	"mosque":                true,
	"professional_network":  true,
	"student_union":         true,
	"informal_group":        true,
	"other":                 true,
}

// Submit creates a new community organisation submission. The row is
// created with status='pending' (awaiting admin review). Auth required
// (community_locator.submit permission checked at route level).
func (s *Service) Submit(ctx context.Context, tenantID, userID pgtype.UUID, input SubmitInput) (OrgDTO, error) {
	if input.Name == "" {
		return OrgDTO{}, fmt.Errorf("name is required")
	}
	if input.Slug == "" {
		return OrgDTO{}, fmt.Errorf("slug is required")
	}
	if input.OrgType == "" {
		return OrgDTO{}, fmt.Errorf("org_type is required")
	}
	if !validOrgTypes[input.OrgType] {
		return OrgDTO{}, fmt.Errorf("invalid org_type: %s", input.OrgType)
	}
	if input.OriginCommunity == "" {
		return OrgDTO{}, fmt.Errorf("origin_community is required")
	}
	if input.DestinationSpaceID == "" {
		return OrgDTO{}, fmt.Errorf("destination_space_id is required")
	}

	var destSpaceID pgtype.UUID
	if err := destSpaceID.Scan(input.DestinationSpaceID); err != nil {
		return OrgDTO{}, fmt.Errorf("invalid destination_space_id: %w", err)
	}

	var citySpaceID pgtype.UUID
	if input.CitySpaceID != "" {
		if err := citySpaceID.Scan(input.CitySpaceID); err != nil {
			return OrgDTO{}, fmt.Errorf("invalid city_space_id: %w", err)
		}
	}

	var japaconnectSpaceID pgtype.UUID
	if input.JapaconnectSpaceID != "" {
		if err := japaconnectSpaceID.Scan(input.JapaconnectSpaceID); err != nil {
			return OrgDTO{}, fmt.Errorf("invalid japaconnect_space_id: %w", err)
		}
	}

	var japaconnectOrgID pgtype.UUID
	if input.JapaconnectOrgID != "" {
		if err := japaconnectOrgID.Scan(input.JapaconnectOrgID); err != nil {
			return OrgDTO{}, fmt.Errorf("invalid japaconnect_org_id: %w", err)
		}
	}

	in := CreateOrgInput{
		TenantID:             tenantID,
		Name:                 input.Name,
		Slug:                 input.Slug,
		OrgType:              input.OrgType,
		OriginCommunity:      input.OriginCommunity,
		DestinationSpaceID:   destSpaceID,
		CitySpaceID:          citySpaceID,
		JapaconnectSpaceID:   japaconnectSpaceID,
		JapaconnectOrgID:     japaconnectOrgID,
		IsCommunitySubmitted: true,
		SubmittedBy:          userID,
	}
	if input.Address != "" {
		in.Address = &input.Address
	}
	if input.Latitude != 0 {
		lat := input.Latitude
		in.Latitude = &lat
	}
	if input.Longitude != 0 {
		lng := input.Longitude
		in.Longitude = &lng
	}
	if input.WebsiteURL != "" {
		in.WebsiteURL = &input.WebsiteURL
	}
	if input.MeetingFrequency != "" {
		in.MeetingFrequency = &input.MeetingFrequency
	}
	if input.ContactInfo != "" {
		in.ContactInfo = &input.ContactInfo
	}

	id, err := s.store.CreateOrg(ctx, in)
	if err != nil {
		// Detect unique constraint violation on (tenant_id, slug) and return
		// a typed error so the handler can map it to 409 duplicate_slug.
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return OrgDTO{}, ErrDuplicateSlug
		}
		return OrgDTO{}, fmt.Errorf("communitylocator: submit: %w", err)
	}

	row, err := s.store.GetByID(ctx, id, tenantID)
	if err != nil {
		return OrgDTO{}, fmt.Errorf("communitylocator: submit: fetch created: %w", err)
	}
	return rowToDTO(row), nil
}

// GetBySlug returns a single published community organisation by its slug.
// Anonymous-accessible.
func (s *Service) GetBySlug(ctx context.Context, slug string, tenantID pgtype.UUID) (OrgDTO, error) {
	row, err := s.store.GetBySlug(ctx, slug, tenantID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return OrgDTO{}, pgx.ErrNoRows
		}
		return OrgDTO{}, fmt.Errorf("communitylocator: get by slug: %w", err)
	}
	return rowToDTO(row), nil
}

// List returns a paginated, filtered list of published community
// organisations. When lat, lng, and radiusKm are all non-zero, results are
// filtered by a simplified bounding box.
func (s *Service) List(ctx context.Context, tenantID, destSpaceID, citySpaceID pgtype.UUID, originCommunity, orgType string, lat, lng, radiusKm float64, page, limit int32) ([]OrgDTO, int64, error) {
	if limit <= 0 {
		limit = 20
	}
	if page <= 0 {
		page = 1
	}
	offset := (page - 1) * limit

	f := ListOrgsFilter{
		TenantID:           tenantID,
		DestinationSpaceID: destSpaceID,
		CitySpaceID:        citySpaceID,
		OriginCommunity:    originCommunity,
		OrgType:            orgType,
		Status:             StatusPublished,
		Limit:              limit,
		Offset:             offset,
	}
	if lat != 0 && lng != 0 && radiusKm > 0 {
		f.Lat = &lat
		f.Lng = &lng
		f.RadiusKm = &radiusKm
	}

	rows, total, err := s.store.ListOrgs(ctx, f)
	if err != nil {
		return nil, 0, fmt.Errorf("communitylocator: list: %w", err)
	}

	dtos := make([]OrgDTO, 0, len(rows))
	for _, r := range rows {
		dtos = append(dtos, rowToDTO(r))
	}
	return dtos, total, nil
}

// InitiateClaim begins the claim flow on a community organisation, setting
// claim_status to 'pending'. Auth required (community_locator.claim
// permission checked at route level).
func (s *Service) InitiateClaim(ctx context.Context, id, tenantID pgtype.UUID) error {
	if err := s.store.InitiateClaim(ctx, id, tenantID); err != nil {
		if err == pgx.ErrNoRows {
			return pgx.ErrNoRows
		}
		return fmt.Errorf("communitylocator: initiate claim: %w", err)
	}
	return nil
}

// AdminList returns all community organisations (any status) for the
// moderation queue.
func (s *Service) AdminList(ctx context.Context, tenantID pgtype.UUID, status, q, sort, dir string, page, limit int32) ([]OrgDTO, int64, error) {
	if limit <= 0 {
		limit = 20
	}
	if page <= 0 {
		page = 1
	}
	offset := (page - 1) * limit

	rows, total, err := s.store.AdminList(ctx, AdminListFilter{
		TenantID: tenantID,
		Status:   status,
		Q:        q,
		Sort:     sort,
		Dir:      dir,
		Limit:    limit,
		Offset:   offset,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("communitylocator: admin list: %w", err)
	}

	dtos := make([]OrgDTO, 0, len(rows))
	for _, r := range rows {
		dtos = append(dtos, rowToDTO(r))
	}
	return dtos, total, nil
}

// ApproveOrg publishes a community organisation (admin,
// community_locator.review). Sets status='published', reviewed_by, and
// reviewed_at.
func (s *Service) ApproveOrg(ctx context.Context, id, tenantID, reviewedBy pgtype.UUID) (OrgDTO, error) {
	if err := s.store.ApproveOrg(ctx, id, tenantID, reviewedBy); err != nil {
		if err == pgx.ErrNoRows {
			return OrgDTO{}, pgx.ErrNoRows
		}
		return OrgDTO{}, fmt.Errorf("communitylocator: approve org: %w", err)
	}
	row, err := s.store.GetByID(ctx, id, tenantID)
	if err != nil {
		return OrgDTO{}, fmt.Errorf("communitylocator: approve org: fetch: %w", err)
	}
	return rowToDTO(row), nil
}

// VerifyOrg marks a community organisation as verified (admin,
// community_locator.manage).
func (s *Service) VerifyOrg(ctx context.Context, id, tenantID pgtype.UUID) (OrgDTO, error) {
	if err := s.store.VerifyOrg(ctx, id, tenantID); err != nil {
		if err == pgx.ErrNoRows {
			return OrgDTO{}, pgx.ErrNoRows
		}
		return OrgDTO{}, fmt.Errorf("communitylocator: verify org: %w", err)
	}
	row, err := s.store.GetByID(ctx, id, tenantID)
	if err != nil {
		return OrgDTO{}, fmt.Errorf("communitylocator: verify org: fetch: %w", err)
	}
	return rowToDTO(row), nil
}

// ResolveClaim resolves a pending claim on a community organisation
// (admin, community_locator.manage). Approving sets claim_status to
// 'claimed'; rejecting resets it to 'unclaimed'.
func (s *Service) ResolveClaim(ctx context.Context, id, tenantID pgtype.UUID, approve bool) (OrgDTO, error) {
	if err := s.store.ResolveClaim(ctx, id, tenantID, approve); err != nil {
		if err == pgx.ErrNoRows {
			return OrgDTO{}, pgx.ErrNoRows
		}
		return OrgDTO{}, fmt.Errorf("communitylocator: resolve claim: %w", err)
	}
	row, err := s.store.GetByID(ctx, id, tenantID)
	if err != nil {
		return OrgDTO{}, fmt.Errorf("communitylocator: resolve claim: fetch: %w", err)
	}
	return rowToDTO(row), nil
}

// rowToDTO converts an OrgRow (joined query result) to an OrgDTO.
func rowToDTO(r OrgRow) OrgDTO {
	dto := OrgDTO{
		ID:                   r.ID.String(),
		Name:                 r.Name,
		Slug:                 r.Slug,
		OrgType:              r.OrgType,
		OriginCommunity:      r.OriginCommunity,
		ClaimStatus:          r.ClaimStatus,
		IsVerified:           r.IsVerified,
		IsActive:             r.IsActive,
		IsCommunitySubmitted: r.IsCommunitySubmitted,
		Status:               r.Status,
		AiGenerated:          r.AiGenerated,
		IsJapabotIndexed:     r.IsJapabotIndexed,
		CreatedAt:            formatTs(r.CreatedAt),
		UpdatedAt:            formatTs(r.UpdatedAt),
		ReviewedAt:           formatTs(r.ReviewedAt),
		HitlReviewedAt:       formatTs(r.HitlReviewedAt),
	}
	if r.DestinationSpaceID.Valid {
		dto.DestinationSpaceID = r.DestinationSpaceID.String()
	}
	if r.DestSpaceName != nil {
		dto.DestSpaceName = *r.DestSpaceName
	}
	if r.DestSpaceCode != nil {
		dto.DestSpaceCode = *r.DestSpaceCode
	}
	if r.CitySpaceID.Valid {
		dto.CitySpaceID = r.CitySpaceID.String()
	}
	if r.CitySpaceName != nil {
		dto.CitySpaceName = *r.CitySpaceName
	}
	if r.CitySpaceCode != nil {
		dto.CitySpaceCode = *r.CitySpaceCode
	}
	if r.Address != nil {
		dto.Address = *r.Address
	}
	if r.Latitude != nil {
		dto.Latitude = *r.Latitude
	}
	if r.Longitude != nil {
		dto.Longitude = *r.Longitude
	}
	if r.WebsiteURL != nil {
		dto.WebsiteURL = *r.WebsiteURL
	}
	if r.MeetingFrequency != nil {
		dto.MeetingFrequency = *r.MeetingFrequency
	}
	if r.ContactInfo != nil {
		dto.ContactInfo = *r.ContactInfo
	}
	if r.JapaconnectSpaceID.Valid {
		dto.JapaconnectSpaceID = r.JapaconnectSpaceID.String()
	}
	if r.JapaconnectOrgID.Valid {
		dto.JapaconnectOrgID = r.JapaconnectOrgID.String()
	}
	if r.SubmittedBy.Valid {
		dto.SubmittedBy = r.SubmittedBy.String()
	}
	if r.SubmitterName != nil {
		dto.SubmitterName = *r.SubmitterName
	}
	if r.ReviewedBy.Valid {
		dto.ReviewedBy = r.ReviewedBy.String()
	}
	// ADR-088 AI generation metadata.
	if r.AiModel != nil {
		dto.AiModel = *r.AiModel
	}
	if r.AiPromptHash != nil {
		dto.AiPromptHash = *r.AiPromptHash
	}
	if r.HitlReviewedBy.Valid {
		dto.HitlReviewedBy = r.HitlReviewedBy.String()
	}
	if r.HitlReviewNotes != nil {
		dto.HitlReviewNotes = *r.HitlReviewNotes
	}
	return dto
}

// formatTs renders a pgtype.Timestamptz as RFC 3339 UTC, or "" if NULL.
func formatTs(t pgtype.Timestamptz) string {
	if !t.Valid {
		return ""
	}
	return t.Time.UTC().Format(time.RFC3339)
}

// slugInvalidChars matches any character that is not a lowercase letter,
// digit, or hyphen. These are stripped during slug normalization.
var slugInvalidChars = regexp.MustCompile(`[^a-z0-9-]+`)

// slugMultipleHyphens matches consecutive hyphens, replaced with a single one.
var slugMultipleHyphens = regexp.MustCompile(`-{2,}`)

// slugLeadingTrailingHyphens matches hyphens at the start or end of a string.
var slugLeadingTrailingHyphens = regexp.MustCompile(`^-+|-+$`)

// slugify produces a URL-safe slug from a name: lowercase, hyphenated,
// alphanumeric only. Used as a fallback when the submitter does not provide
// an explicit slug. Matches the content.Slugify pattern.
func slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = slugInvalidChars.ReplaceAllString(s, "-")
	s = slugMultipleHyphens.ReplaceAllString(s, "-")
	s = slugLeadingTrailingHyphens.ReplaceAllString(s, "")
	if s == "" {
		return "untitled"
	}
	return s
}
