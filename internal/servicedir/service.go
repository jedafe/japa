// Package servicedir — service.go orchestrates service listing creation,
// editing, filtered listing, admin verification, and organization claim
// workflow. It delegates content row creation to the content.Service (§14)
// and manages the service_listing_meta extension table via the local Store.
// Feature-flagged via service_directory.enabled system_setting (§98).
//
// Ref: japaconnect-engineering/sections/98-service-directory.md
//
//	japaconnect-engineering/adr/ADR-106-service-directory.md
package servicedir

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/config"
	"github.com/jedafe/japa/internal/content"
	"github.com/jedafe/japa/internal/db/gen"
)

// ContentService is the subset of content.Service the servicedir package
// needs. Defined as an interface so the servicedir package depends on a
// narrow contract, not the full content service.
type ContentService interface {
	CreateContent(ctx context.Context, tenantID pgtype.UUID, input content.CreateContentInput) (gen.CreateContentRow, error)
	GetContent(ctx context.Context, id, tenantID pgtype.UUID) (gen.GetContentByIDRow, error)
	UpdateContent(ctx context.Context, id, tenantID, editedBy pgtype.UUID, input content.UpdateContentInput, bypassEditWindow bool) (gen.UpdateContentRow, error)
	DeleteContent(ctx context.Context, id, tenantID pgtype.UUID) error
}

// ServiceDTO is the API response shape for a single service listing.
type ServiceDTO struct {
	ID                string   `json:"id"`
	Title             string   `json:"title"`
	Slug              string   `json:"slug"`
	Body              string   `json:"body"`
	BodyHTML          string   `json:"body_html"`
	Status            string   `json:"status"`
	Visibility        string   `json:"visibility"`
	ViewCount         int32    `json:"view_count"`
	CommentCount      int32    `json:"comment_count"`
	PublishedAt       string   `json:"published_at"`
	CreatedAt         string   `json:"created_at"`
	UpdatedAt         string   `json:"updated_at"`
	OrgID             string   `json:"org_id,omitempty"`
	OrgName           string   `json:"org_name,omitempty"`
	OrgSlug           string   `json:"org_slug,omitempty"`
	ServiceCategory   string   `json:"service_category,omitempty"`
	DestSpaceID       string   `json:"destination_space_id,omitempty"`
	DestSpaceName     string   `json:"destination_space_name,omitempty"`
	DestSpaceCode     string   `json:"destination_space_code,omitempty"`
	CitySpaceID       string   `json:"city_space_id,omitempty"`
	CitySpaceName     string   `json:"city_space_name,omitempty"`
	CitySpaceCode     string   `json:"city_space_code,omitempty"`
	OriginCommunities []string `json:"origin_communities,omitempty"`
	IsRemote          bool     `json:"is_remote"`
	WebsiteURL        string   `json:"website_url,omitempty"`
	BookingURL        string   `json:"booking_url,omitempty"`
	PriceRange        string   `json:"price_range,omitempty"`
	IsVerified        bool     `json:"is_verified"`
	VerificationType  string   `json:"verification_type,omitempty"`
	ClaimStatus       string   `json:"claim_status,omitempty"`
	ClaimedByOrgID    string   `json:"claimed_by_org_id,omitempty"`
	ClaimedOrgName    string   `json:"claimed_org_name,omitempty"`
	VerifiedBy        string   `json:"verified_by,omitempty"`
	VerifiedAt        string   `json:"verified_at,omitempty"`
	AuthorID          string   `json:"author_id,omitempty"`
	AuthorName        string   `json:"author_name,omitempty"`
	AuthorAvatar      string   `json:"author_avatar,omitempty"`
}

// CreateServiceInput is the request body for POST /services.
type CreateServiceInput struct {
	Title              string `json:"title"`
	Body               string `json:"body"`
	OrgID              string `json:"org_id"`
	ServiceCategory    string `json:"service_category"`
	DestinationSpaceID string `json:"destination_space_id"`
	CitySpaceID        string `json:"city_space_id"`
	OriginCommunities  any    `json:"origin_communities"`
	IsRemote           bool   `json:"is_remote"`
	WebsiteURL         string `json:"website_url"`
	BookingURL         string `json:"booking_url"`
	PriceRange         string `json:"price_range"`
}

// EditServiceInput is the request body for PATCH /services/:id.
type EditServiceInput struct {
	Title              *string `json:"title"`
	Body               *string `json:"body"`
	OrgID              *string `json:"org_id"`
	ServiceCategory    *string `json:"service_category"`
	DestinationSpaceID *string `json:"destination_space_id"`
	CitySpaceID        *string `json:"city_space_id"`
	OriginCommunities  any     `json:"origin_communities"`
	IsRemote           *bool   `json:"is_remote"`
	WebsiteURL         *string `json:"website_url"`
	BookingURL         *string `json:"booking_url"`
	PriceRange         *string `json:"price_range"`
}

// Service holds the business logic for the Service Directory.
type Service struct {
	store    *Store
	content  ContentService
	settings *config.Settings
}

// NewService creates a service directory service.
func NewService(store *Store, contentSvc ContentService, settings *config.Settings) *Service {
	return &Service{
		store:    store,
		content:  contentSvc,
		settings: settings,
	}
}

// Enabled reads the service_directory.enabled system_setting toggle
// (§98 feature flag).
func (s *Service) Enabled(ctx context.Context, tenantID pgtype.UUID) bool {
	if s.settings == nil {
		return false
	}

	return s.settings.GetBoolOr(ctx, tenantID, "service_directory.enabled", false)
}

// Option is a single selectable value exposed by the option endpoints.
type Option struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

var categoryLabels = map[string]string{
	CategoryImmigrationLegal: "Immigration & legal",
	CategoryRelocation:       "Relocation",
	CategoryFinance:          "Finance",
	CategoryEducation:        "Education",
	CategoryHealth:           "Health",
	CategoryCommunity:        "Community",
	CategoryOther:            "Other",
}

func sortOptions(opts []Option) {
	sort.Slice(opts, func(i, j int) bool { return opts[i].Value < opts[j].Value })
}

// Categories returns the canonical service category options (Rule 16).
func (s *Service) Categories() []Option {
	out := make([]Option, 0, len(categoryLabels))
	for v, label := range categoryLabels {
		out = append(out, Option{Value: v, Label: label})
	}

	sortOptions(out)

	return out
}

// ErrInvalidCategory is returned when a create/edit input supplies a
// service_category that is not in the §98 canonical enum.
var ErrInvalidCategory = errors.New("invalid service_category")

// ErrInvalidVerificationType is returned when an admin verify request
// supplies a verification_type that is not in the §98 canonical enum
// (BUG-010).
var ErrInvalidVerificationType = errors.New("invalid verification_type")

// Create creates a new service listing — a contents row with
// content_type='service_listing' plus a service_listing_meta row. The
// listing is created in 'draft' status; the author can submit for review
// (set status to 'pending') via the content update flow. Auth required
// (service.create permission checked at route level).
func (s *Service) Create(ctx context.Context, tenantID, authorID pgtype.UUID, input CreateServiceInput) (ServiceDTO, error) {
	if input.Title == "" {
		return ServiceDTO{}, errors.New("title is required")
	}
	// BUG-013: validate service_category against the §98 enum. Empty is
	// rejected because the column is NOT NULL.
	if input.ServiceCategory == "" || !IsValidCategory(input.ServiceCategory) {
		return ServiceDTO{}, ErrInvalidCategory
	}

	var orgID pgtype.UUID
	if input.OrgID != "" {
		if err := orgID.Scan(input.OrgID); err != nil {
			return ServiceDTO{}, fmt.Errorf("invalid org_id: %w", err)
		}
	}

	var destSpaceID pgtype.UUID
	if input.DestinationSpaceID != "" {
		if err := destSpaceID.Scan(input.DestinationSpaceID); err != nil {
			return ServiceDTO{}, fmt.Errorf("invalid destination_space_id: %w", err)
		}
	}

	var citySpaceID pgtype.UUID
	if input.CitySpaceID != "" {
		if err := citySpaceID.Scan(input.CitySpaceID); err != nil {
			return ServiceDTO{}, fmt.Errorf("invalid city_space_id: %w", err)
		}
	}

	originCommunities, err := normalizeOriginCommunities(input.OriginCommunities)
	if err != nil {
		return ServiceDTO{}, fmt.Errorf("invalid origin_communities: %w", err)
	}

	contentRow, err := s.content.CreateContent(ctx, tenantID, content.CreateContentInput{
		AuthorID:       authorID,
		OrganizationID: orgID,
		ContentType:    content.TypeServiceListing,
		Title:          input.Title,
		Body:           input.Body,
		// BUG-013: create as 'in_review' so the listing enters the admin
		// review queue and can be published. 'draft' would make it
		// invisible with no publish path.
		Status:     content.StatusInReview,
		Visibility: content.VisibilityPublic,
		GeoSpaceID: destSpaceID,
	})
	if err != nil {
		return ServiceDTO{}, fmt.Errorf("servicedir: create content: %w", err)
	}

	meta := ServiceMeta{
		TenantID:           tenantID,
		ContentID:          contentRow.ID,
		OrgID:              orgID,
		DestinationSpaceID: destSpaceID,
		CitySpaceID:        citySpaceID,
		OriginCommunities:  originCommunities,
		IsRemote:           input.IsRemote,
		IsVerified:         false,
	}
	if input.ServiceCategory != "" {
		meta.ServiceCategory = &input.ServiceCategory
	}

	if input.WebsiteURL != "" {
		meta.WebsiteURL = &input.WebsiteURL
	}

	if input.BookingURL != "" {
		meta.BookingURL = &input.BookingURL
	}

	if input.PriceRange != "" {
		meta.PriceRange = &input.PriceRange
	}

	if err := s.store.CreateMeta(ctx, meta); err != nil {
		slog.Error("servicedir: create service_listing_meta failed after content creation", "content_id", contentRow.ID, "error", err)
		return ServiceDTO{}, fmt.Errorf("servicedir: create service_listing_meta: %w", err)
	}

	return s.getServiceDTOByID(ctx, contentRow.ID, tenantID)
}

// GetBySlug returns a single published service listing by its slug.
// Anonymous-accessible.
func (s *Service) GetBySlug(ctx context.Context, slug string, tenantID pgtype.UUID) (ServiceDTO, error) {
	row, err := s.store.GetBySlug(ctx, slug, tenantID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ServiceDTO{}, pgx.ErrNoRows
		}

		return ServiceDTO{}, fmt.Errorf("servicedir: get by slug: %w", err)
	}

	return rowToDTO(row), nil
}

// List returns a paginated, filtered list of published service listings.
// destSpaceID, citySpaceID, serviceCategory, originCommunity are optional
// geo-aware filters (§98: ?dest=&city=&category=&origin=&verified=&sort=).
func (s *Service) List(ctx context.Context, tenantID, destSpaceID, citySpaceID pgtype.UUID, serviceCategory, originCommunity string, isVerified *bool, sort string, page, limit int32) ([]ServiceDTO, int64, error) {
	if limit <= 0 {
		limit = 20
	}

	if page <= 0 {
		page = 1
	}

	offset := (page - 1) * limit

	rows, total, err := s.store.ListServices(ctx, ListServicesFilter{
		TenantID:        tenantID,
		DestSpaceID:     destSpaceID,
		CitySpaceID:     citySpaceID,
		ServiceCategory: serviceCategory,
		OriginCommunity: originCommunity,
		IsVerified:      isVerified,
		Sort:            sort,
		Status:          content.StatusPublished,
		Limit:           limit,
		Offset:          offset,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("servicedir: list: %w", err)
	}

	dtos := make([]ServiceDTO, 0, len(rows))
	for _, r := range rows {
		dtos = append(dtos, rowToDTO(r))
	}

	return dtos, total, nil
}

// EditOwn updates the author's own service listing, or a listing owned by
// the user's organization (§98: service.edit_own = "listing owner org").
// An admin with service.edit_any (checked at route level via
// RequireAnyPermission) bypasses the ownership check (BUG-008, BUG-009).
// Only the provided (non-nil) fields are updated.
func (s *Service) EditOwn(ctx context.Context, contentID, tenantID, userID pgtype.UUID, input EditServiceInput, isEditAny bool) (ServiceDTO, error) {
	existing, err := s.content.GetContent(ctx, contentID, tenantID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ServiceDTO{}, pgx.ErrNoRows
		}

		return ServiceDTO{}, fmt.Errorf("servicedir: edit: get content: %w", err)
	}

	// service.edit_any holders (admins) bypass the ownership check (BUG-009).
	if !isEditAny {
		// Check author ownership first (BUG-008).
		if existing.AuthorID != userID {
			// Not the author — check if the user is a member of the org
			// that owns the listing (§98: service.edit_own = "listing owner org").
			meta, err := s.store.GetMeta(ctx, contentID, tenantID)
			if err != nil {
				return ServiceDTO{}, fmt.Errorf("servicedir: edit: get meta: %w", err)
			}

			if !meta.OrgID.Valid {
				return ServiceDTO{}, ErrNotOwner
			}

			isMember, err := s.store.IsOrgMember(ctx, meta.OrgID, userID)
			if err != nil {
				return ServiceDTO{}, fmt.Errorf("servicedir: edit: check org membership: %w", err)
			}

			if !isMember {
				return ServiceDTO{}, ErrNotOwner
			}
		}
	}

	updateInput := content.UpdateContentInput{
		Title: existing.Title,
		Body:  existing.Body,
	}
	if input.Title != nil {
		updateInput.Title = *input.Title
	}

	if input.Body != nil {
		updateInput.Body = *input.Body
	}

	if _, err := s.content.UpdateContent(ctx, contentID, tenantID, userID, updateInput, false); err != nil {
		return ServiceDTO{}, fmt.Errorf("servicedir: edit: update content: %w", err)
	}

	meta := ServiceMeta{}
	if input.OrgID != nil && *input.OrgID != "" {
		if err := meta.OrgID.Scan(*input.OrgID); err != nil {
			return ServiceDTO{}, fmt.Errorf("servicedir: edit: invalid org_id: %w", err)
		}
	}

	if input.DestinationSpaceID != nil && *input.DestinationSpaceID != "" {
		if err := meta.DestinationSpaceID.Scan(*input.DestinationSpaceID); err != nil {
			return ServiceDTO{}, fmt.Errorf("servicedir: edit: invalid destination_space_id: %w", err)
		}
	}

	if input.CitySpaceID != nil && *input.CitySpaceID != "" {
		if err := meta.CitySpaceID.Scan(*input.CitySpaceID); err != nil {
			return ServiceDTO{}, fmt.Errorf("servicedir: edit: invalid city_space_id: %w", err)
		}
	}

	if input.ServiceCategory != nil {
		// BUG-013: validate category on edit when supplied.
		if *input.ServiceCategory == "" || !IsValidCategory(*input.ServiceCategory) {
			return ServiceDTO{}, ErrInvalidCategory
		}

		meta.ServiceCategory = input.ServiceCategory
	}

	if input.OriginCommunities != nil {
		oc, err := normalizeOriginCommunities(input.OriginCommunities)
		if err != nil {
			return ServiceDTO{}, fmt.Errorf("servicedir: edit: invalid origin_communities: %w", err)
		}

		meta.OriginCommunities = oc
	}

	if input.IsRemote != nil {
		meta.IsRemote = *input.IsRemote
	}

	if input.WebsiteURL != nil {
		meta.WebsiteURL = input.WebsiteURL
	}

	if input.BookingURL != nil {
		meta.BookingURL = input.BookingURL
	}

	if input.PriceRange != nil {
		meta.PriceRange = input.PriceRange
	}

	if err := s.store.UpdateMeta(ctx, contentID, tenantID, meta); err != nil {
		return ServiceDTO{}, fmt.Errorf("servicedir: edit: update service_listing_meta: %w", err)
	}

	return s.getServiceDTOByID(ctx, contentID, tenantID)
}

// ErrNotOrgMember is returned when a user tries to claim a listing on
// behalf of an organization they are not a member of (BUG-006).
var ErrNotOrgMember = errors.New("not a member of the claiming organization")

// ErrAlreadyClaimed is returned when a listing is already claimed
// (pending or approved) and a new claim is attempted (BUG-007).
var ErrAlreadyClaimed = errors.New("service listing is already claimed")

// Claim initiates an organization claim on a service listing. Sets
// claim_status to 'pending' and records the claiming org_id. The caller
// must have the service.claim permission (checked at route level). This
// method verifies the user is a member of the claiming org (BUG-006) and
// that the listing is not already claimed (BUG-007).
func (s *Service) Claim(ctx context.Context, contentID, tenantID, userID, orgID pgtype.UUID) error {
	isMember, err := s.store.IsOrgMember(ctx, orgID, userID)
	if err != nil {
		return fmt.Errorf("servicedir: claim: check org membership: %w", err)
	}

	if !isMember {
		return ErrNotOrgMember
	}

	if err := s.store.ClaimIfUnclaimed(ctx, contentID, tenantID, orgID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrAlreadyClaimed
		}

		return fmt.Errorf("servicedir: claim: %w", err)
	}

	return nil
}

// AdminList returns all service listings (any status) for the moderation
// queue. Supports optional dest filter (BUG-011: geo-aware pattern on
// admin list endpoints).
func (s *Service) AdminList(ctx context.Context, tenantID pgtype.UUID, status, claimStatus, serviceCategory, q string, isVerified *bool, destSpaceID pgtype.UUID, page, limit int32) ([]ServiceDTO, int64, error) {
	if limit <= 0 {
		limit = 20
	}

	if page <= 0 {
		page = 1
	}

	offset := (page - 1) * limit

	rows, total, err := s.store.AdminList(ctx, AdminListFilter{
		TenantID:        tenantID,
		DestSpaceID:     destSpaceID,
		Status:          status,
		IsVerified:      isVerified,
		ClaimStatus:     claimStatus,
		ServiceCategory: serviceCategory,
		Q:               q,
		Limit:           limit,
		Offset:          offset,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("servicedir: admin list: %w", err)
	}

	dtos := make([]ServiceDTO, 0, len(rows))
	for _, r := range rows {
		dtos = append(dtos, rowToDTO(r))
	}

	return dtos, total, nil
}

// AdminVerify marks a service listing as verified by an admin
// (service.verify permission). The verificationType records how the
// verification was performed (§98: self|community|admin|professional_body);
// the admin user_id is recorded in verified_by for audit (ADR-088, BUG-015).
// The verificationType is validated against the §98 enum (BUG-010).
func (s *Service) AdminVerify(ctx context.Context, contentID, tenantID, verifiedBy pgtype.UUID, verificationType string) error {
	if verificationType == "" {
		verificationType = VerificationTypeAdmin
	}

	if !IsValidVerificationType(verificationType) {
		return ErrInvalidVerificationType
	}

	if err := s.store.VerifyService(ctx, contentID, tenantID, verifiedBy, verificationType); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return pgx.ErrNoRows
		}

		return fmt.Errorf("servicedir: admin verify: %w", err)
	}

	return nil
}

// AdminUnverify marks a service listing as unverified by an admin
// (service.verify permission). Clears the verified flag and audit trail.
func (s *Service) AdminUnverify(ctx context.Context, contentID, tenantID pgtype.UUID) error {
	if err := s.store.UnverifyService(ctx, contentID, tenantID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return pgx.ErrNoRows
		}

		return fmt.Errorf("servicedir: admin unverify: %w", err)
	}

	return nil
}

// AdminResolveClaim resolves a pending organization claim on a service
// listing. Approving sets claim_status to 'approved' and links the org;
// rejecting sets claim_status to 'rejected' and clears claimed_by_org_id
// (service.verify permission). Only pending claims can be resolved
// (BUG-015).
func (s *Service) AdminResolveClaim(ctx context.Context, contentID, tenantID, orgID pgtype.UUID, approve bool) error {
	if err := s.store.ResolveClaimIfPending(ctx, contentID, tenantID, orgID, approve); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return pgx.ErrNoRows
		}

		return fmt.Errorf("servicedir: admin resolve claim: %w", err)
	}

	return nil
}

// getServiceDTOByID fetches a service listing by content_id and converts
// to DTO.
func (s *Service) getServiceDTOByID(ctx context.Context, contentID, tenantID pgtype.UUID) (ServiceDTO, error) {
	contentRow, err := s.content.GetContent(ctx, contentID, tenantID)
	if err != nil {
		return ServiceDTO{}, fmt.Errorf("servicedir: get by id: %w", err)
	}

	meta, err := s.store.GetMeta(ctx, contentID, tenantID)
	if err != nil {
		return ServiceDTO{}, fmt.Errorf("servicedir: get meta by id: %w", err)
	}

	return metaToDTO(contentRow, meta), nil
}

// ErrNotOwner is returned when a user tries to edit a service listing they
// did not author.
var ErrNotOwner = errors.New("not the service listing author")

// rowToDTO converts a ServiceRow (joined query result) to a ServiceDTO.
func rowToDTO(r ServiceRow) ServiceDTO {
	dto := ServiceDTO{
		ID:           r.ID.String(),
		Title:        r.Title,
		Slug:         r.Slug,
		Body:         r.Body,
		BodyHTML:     r.BodyHTML,
		Status:       r.Status,
		Visibility:   r.Visibility,
		ViewCount:    r.ViewCount,
		CommentCount: r.CommentCount,
		PublishedAt:  formatTs(r.PublishedAt),
		CreatedAt:    formatTs(r.CreatedAt),
		UpdatedAt:    formatTs(r.UpdatedAt),
		IsRemote:     r.IsRemote,
		IsVerified:   r.IsVerified,
	}
	if r.OrgID.Valid {
		dto.OrgID = r.OrgID.String()
	}

	if r.OrgName != nil {
		dto.OrgName = *r.OrgName
	}

	if r.OrgSlug != nil {
		dto.OrgSlug = *r.OrgSlug
	}

	if r.ServiceCategory != nil {
		dto.ServiceCategory = *r.ServiceCategory
	}

	if r.DestSpaceID.Valid {
		dto.DestSpaceID = r.DestSpaceID.String()
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

	dto.OriginCommunities = r.OriginCommunities
	if r.WebsiteURL != nil {
		dto.WebsiteURL = *r.WebsiteURL
	}

	if r.BookingURL != nil {
		dto.BookingURL = *r.BookingURL
	}

	if r.PriceRange != nil {
		dto.PriceRange = *r.PriceRange
	}

	if r.VerificationType != nil {
		dto.VerificationType = *r.VerificationType
	}

	if r.ClaimStatus != nil {
		dto.ClaimStatus = *r.ClaimStatus
	}

	if r.ClaimedByOrgID.Valid {
		dto.ClaimedByOrgID = r.ClaimedByOrgID.String()
	}

	if r.ClaimedOrgName != nil {
		dto.ClaimedOrgName = *r.ClaimedOrgName
	}

	if r.VerifiedBy.Valid {
		dto.VerifiedBy = r.VerifiedBy.String()
	}

	dto.VerifiedAt = formatTs(r.VerifiedAt)
	if r.AuthorName != nil {
		dto.AuthorName = *r.AuthorName
	}

	if r.AuthorAvatar != nil {
		dto.AuthorAvatar = *r.AuthorAvatar
	}

	if r.AuthorID.Valid {
		dto.AuthorID = r.AuthorID.String()
	}

	return dto
}

// metaToDTO converts a content row + service_listing_meta into a ServiceDTO.
func metaToDTO(c gen.GetContentByIDRow, m ServiceMeta) ServiceDTO {
	dto := ServiceDTO{
		ID:           c.ID.String(),
		Title:        c.Title,
		Slug:         c.Slug,
		Body:         c.Body,
		BodyHTML:     c.BodyHtml,
		Status:       c.Status,
		Visibility:   c.Visibility,
		ViewCount:    c.ViewCount,
		CommentCount: c.CommentCount,
		PublishedAt:  formatTs(c.PublishedAt),
		CreatedAt:    formatTs(c.CreatedAt),
		UpdatedAt:    formatTs(c.UpdatedAt),
		IsRemote:     m.IsRemote,
		IsVerified:   m.IsVerified,
	}
	if m.OrgID.Valid {
		dto.OrgID = m.OrgID.String()
	}

	if m.ServiceCategory != nil {
		dto.ServiceCategory = *m.ServiceCategory
	}

	if m.DestinationSpaceID.Valid {
		dto.DestSpaceID = m.DestinationSpaceID.String()
	}

	if m.CitySpaceID.Valid {
		dto.CitySpaceID = m.CitySpaceID.String()
	}

	dto.OriginCommunities = m.OriginCommunities
	if m.WebsiteURL != nil {
		dto.WebsiteURL = *m.WebsiteURL
	}

	if m.BookingURL != nil {
		dto.BookingURL = *m.BookingURL
	}

	if m.PriceRange != nil {
		dto.PriceRange = *m.PriceRange
	}

	if m.VerificationType != nil {
		dto.VerificationType = *m.VerificationType
	}

	if m.ClaimStatus != nil {
		dto.ClaimStatus = *m.ClaimStatus
	}

	if m.ClaimedByOrgID.Valid {
		dto.ClaimedByOrgID = m.ClaimedByOrgID.String()
	}

	if m.VerifiedBy.Valid {
		dto.VerifiedBy = m.VerifiedBy.String()
	}

	dto.VerifiedAt = formatTs(m.VerifiedAt)
	if c.AuthorID.Valid {
		dto.AuthorID = c.AuthorID.String()
	}

	return dto
}

// normalizeOriginCommunities converts the input (any) into a []string for
// storage in the text[] column (§98 schema, BUG-006). Accepts []string,
// []any, or a JSON array string. nil/empty values produce an empty slice.
func normalizeOriginCommunities(v any) ([]string, error) {
	if v == nil {
		return []string{}, nil
	}

	switch val := v.(type) {
	case []string:
		if val == nil {
			return []string{}, nil
		}

		return val, nil
	case []any:
		out := make([]string, 0, len(val))

		for _, item := range val {
			s, ok := item.(string)
			if !ok {
				return nil, errors.New("origin_communities: non-string element")
			}

			if s != "" {
				out = append(out, s)
			}
		}

		return out, nil
	case string:
		if val == "" {
			return []string{}, nil
		}
		// Treat as JSON array string for backward compatibility.
		var arr []string
		if err := json.Unmarshal([]byte(val), &arr); err == nil {
			return arr, nil
		}
		// Fall back: single comma-separated string.
		parts := strings.Split(val, ",")
		out := make([]string, 0, len(parts))

		for _, p := range parts {
			if t := strings.TrimSpace(p); t != "" {
				out = append(out, t)
			}
		}

		return out, nil
	default:
		// Last resort: JSON-encode then decode as []string.
		b, err := json.Marshal(v)
		if err != nil {
			return nil, err
		}

		var arr []string
		if err := json.Unmarshal(b, &arr); err != nil {
			return nil, err
		}

		return arr, nil
	}
}

// formatTs renders a pgtype.Timestamptz as RFC 3339 UTC, or "" if NULL.
func formatTs(t pgtype.Timestamptz) string {
	if !t.Valid {
		return ""
	}

	return t.Time.UTC().Format(time.RFC3339)
}
