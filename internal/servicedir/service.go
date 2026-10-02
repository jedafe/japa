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

	"github.com/valkey-io/valkey-go/valkeycompat"

	"github.com/jedafe/japa/internal/config"
	"github.com/jedafe/japa/internal/content"
	"github.com/jedafe/japa/internal/db/gen"
)

// ContentService is the subset of content.Service the servicedir package
// needs. Defined as an interface so the servicedir package depends on a
// narrow contract, not the full content service.
type ContentService interface {
	CreateContent(ctx context.Context, input content.CreateContentInput) (gen.CreateContentRow, error)
	GetContent(ctx context.Context, id pgtype.UUID) (gen.GetContentByIDRow, error)
	UpdateContent(ctx context.Context, id, editedBy pgtype.UUID, input content.UpdateContentInput, bypassEditWindow bool) (gen.UpdateContentRow, error)
	DeleteContent(ctx context.Context, id pgtype.UUID) error
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

	// §111 package fields — populated when listing_kind='package'.
	ListingKind    string                `json:"listing_kind"`
	PriceAmount    *int64                `json:"price_amount,omitempty"` // minor units
	PriceCurrency  string                `json:"price_currency,omitempty"`
	PricePer       string                `json:"price_per,omitempty"`
	PurchaseMode   string                `json:"purchase_mode"`
	Capacity       *int32                `json:"capacity,omitempty"`
	AvailableFrom  string                `json:"available_from,omitempty"`
	AvailableUntil string                `json:"available_until,omitempty"`
	Items          []PackageItemDTO      `json:"items,omitempty"`
	Departures     []PackageDepartureDTO `json:"departures,omitempty"`
	// PlatformFee is the computed §17 charge for purchase_mode='paid'
	// bookings — informational on the detail response so the checkout copy
	// can quote it (§111: "Reserve — {fee} platform fee").
	PlatformFee     *int64 `json:"platform_fee,omitempty"`
	AgentBalanceDue *int64 `json:"agent_balance_due,omitempty"`
}

// PackageItemDTO is one ordered component line item of a package (§111).
type PackageItemDTO struct {
	ID                 string `json:"id"`
	Position           int32  `json:"position"`
	Title              string `json:"title"`
	Description        string `json:"description,omitempty"`
	ComponentContentID string `json:"component_content_id,omitempty"`
	Included           bool   `json:"included"`
	Optional           bool   `json:"optional"`
}

// PackageDepartureDTO is one scheduled departure/intake (§111).
type PackageDepartureDTO struct {
	ID          string `json:"id"`
	StartsOn    string `json:"starts_on"`
	EndsOn      string `json:"ends_on,omitempty"`
	Capacity    *int32 `json:"capacity,omitempty"`
	BookedCount int32  `json:"booked_count"`
	Status      string `json:"status"`
}

// PackageItemInput is one item in the create/edit items[] payload.
type PackageItemInput struct {
	Title              string `json:"title"`
	Description        string `json:"description"`
	ComponentContentID string `json:"component_content_id"` // optional link, same-org listings only
	Included           *bool  `json:"included"`
	Optional           *bool  `json:"optional"`
}

// PackageDepartureInput is one departure in the create/edit departures[]
// payload.
type PackageDepartureInput struct {
	StartsOn string `json:"starts_on"` // ISO date
	EndsOn   string `json:"ends_on"`   // optional ISO date
	Capacity *int32 `json:"capacity"`
	Status   string `json:"status"` // open|cancelled ('full' is system-managed)
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

	// §111 package fields (ignored unless listing_kind='package')
	ListingKind    string                  `json:"listing_kind"`
	PriceAmount    *int64                  `json:"price_amount"`
	PriceCurrency  string                  `json:"price_currency"`
	PricePer       string                  `json:"price_per"`
	PurchaseMode   string                  `json:"purchase_mode"`
	Capacity       *int32                  `json:"capacity"`
	AvailableFrom  string                  `json:"available_from"`
	AvailableUntil string                  `json:"available_until"`
	Items          []PackageItemInput      `json:"items"`
	Departures     []PackageDepartureInput `json:"departures"`
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

	// §111 package fields. items[]/departures[] are replace-on-save: a
	// non-nil slice replaces the whole set.
	ListingKind    *string                  `json:"listing_kind"`
	PriceAmount    *int64                   `json:"price_amount"`
	PriceCurrency  *string                  `json:"price_currency"`
	PricePer       *string                  `json:"price_per"`
	PurchaseMode   *string                  `json:"purchase_mode"`
	Capacity       *int32                   `json:"capacity"`
	AvailableFrom  *string                  `json:"available_from"`
	AvailableUntil *string                  `json:"available_until"`
	Items          *[]PackageItemInput      `json:"items"`
	Departures     *[]PackageDepartureInput `json:"departures"`
}

// SettingsReader is the narrow settings contract servicedir needs
// (§98/§111 flag + service_packages.* keys). *config.Settings satisfies
// it; tests substitute an in-memory fake.
type SettingsReader interface {
	GetBoolOr(ctx context.Context, key string, fallback bool) bool
	GetIntOr(ctx context.Context, key string, fallback int) int
	GetStringOr(ctx context.Context, key, fallback string) string
}

// Service holds the business logic for the Service Directory.
type Service struct {
	store    *Store
	content  ContentService
	settings SettingsReader
	checker  FeatureChecker
	vk       valkeycompat.Cmdable // §111 package detail cache (jc:svc_package:*)
	enqueuer TaskEnqueuer         // §111 booking notifications/fee settlement
	payments PaymentCreator       // §17 fee payment intents
}

// TaskEnqueuer enqueues Asynq tasks (§111 booking lifecycle).
type TaskEnqueuer interface {
	Enqueue(taskType string, payload []byte, opts ...interface{}) error
}

// PaymentCreator is the narrow §17 payments seam for platform-fee intents.
type PaymentCreator interface {
	CreatePayment(ctx context.Context, userID pgtype.UUID, input PaymentInput) (PaymentResult, error)
}

// PaymentInput mirrors payments.CreatePaymentInput (declared here to keep
// the servicedir package free of a payments import cycle — the server
// package wires the adapter). Idempotency is implicit: when the booking
// route runs behind mw.IdempotencyMiddleware the header propagates to the
// payment row via ctx (mw.IdempotencyKeyFromContext → payments.idempotency_key).
type PaymentInput struct {
	Provider    string
	PurposeType string
	PurposeID   string
	Amount      int64
	Currency    string
	Email       string
	Metadata    map[string]interface{}
	ReturnURL   string
}

// PaymentResult mirrors payments.CreatePaymentResult.
type PaymentResult struct {
	PaymentID   string
	CheckoutURL string
	Status      string
}

// NewService creates a service directory service.
func NewService(store *Store, contentSvc ContentService, settings *config.Settings) *Service {
	return &Service{
		store:    store,
		content:  contentSvc,
		settings: settings,
	}
}

// SetCache injects the Valkey client for the §111 package detail cache.
func (s *Service) SetCache(vk valkeycompat.Cmdable) { s.vk = vk }

// SetEnqueuer injects the Asynq enqueuer for §111 booking tasks.
func (s *Service) SetEnqueuer(e TaskEnqueuer) { s.enqueuer = e }

// SetPayments injects the §17 payment creator for platform-fee intents.
func (s *Service) SetPayments(p PaymentCreator) { s.payments = p }

// §111 booking notifications are dispatched inside the Asynq worker
// handlers (service:booking_created / service:booking_status_changed),
// not through a service-level notifier seam — payloads carry booking IDs
// and the worker fans out to org members / the booker.

// FeatureChecker resolves feature flag state from the feature_flags table —
// the single source of truth for reachability (Ref: internal/features/features.go).
// system_settings is consulted only as a fallback when no checker is injected.
type FeatureChecker interface {
	IsEnabled(ctx context.Context, key string) bool
}

// SetFeatureChecker injects the feature flag checker so Enabled reads the
// feature_flags table instead of drifting against system_settings.
func (s *Service) SetFeatureChecker(c FeatureChecker) { s.checker = c }

// Enabled reads the service_directory.enabled flag (§98). Prefers the
// injected FeatureChecker; falls back to system_settings for backward
// compatibility.
func (s *Service) Enabled(ctx context.Context) bool {
	if s.checker != nil {
		return s.checker.IsEnabled(ctx, "service_directory.enabled")
	}

	if s.settings == nil {
		return false
	}

	return s.settings.GetBoolOr(ctx, "service_directory.enabled", false)
}

// PackagesEnabled reads the service_packages.enabled flag (§111) — gates
// package creation, the bookings pipeline, and the kind=package filter.
// Independent of the directory flag so the directory can ship while
// packages remain dark-launched.
func (s *Service) PackagesEnabled(ctx context.Context) bool {
	if s.checker != nil {
		return s.checker.IsEnabled(ctx, "service_packages.enabled")
	}

	if s.settings == nil {
		return false
	}

	return s.settings.GetBoolOr(ctx, "service_packages.enabled", false)
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

// §111 error sentinels.
var (
	// ErrPackagesDisabled — service_packages.enabled flag is off.
	ErrPackagesDisabled = errors.New("service packages are not enabled")
	// ErrOrgRequired — packages must be authored under an organization.
	ErrOrgRequired = errors.New("org_id is required for packages")
	// ErrOrgNotVerified — §111 trust gate: packages require a verified org
	// with business/professional_body verification (403
	// organization_not_verified, same code as §61 self-serve ads).
	ErrOrgNotVerified = errors.New("organization_not_verified")
	// ErrPurchaseModeUnavailable — purchase_mode='full' is deferred (§111
	// Non-Goals); also used when 'paid' is requested while fee_enabled=false.
	ErrPurchaseModeUnavailable = errors.New("purchase_mode_unavailable")
	// ErrInvalidListingKind — unknown listing_kind value.
	ErrInvalidListingKind = errors.New("invalid listing_kind")
	// ErrInvalidPurchaseMode — unknown purchase_mode value.
	ErrInvalidPurchaseMode = errors.New("invalid purchase_mode")
	// ErrInvalidPricePer — unknown price_per value.
	ErrInvalidPricePer = errors.New("invalid price_per")
	// ErrMissingPackagePrice — purchase_mode='paid' requires price_amount +
	// price_currency.
	ErrMissingPackagePrice = errors.New("paid packages require price_amount and price_currency")
	// ErrTooManyPackageItems — exceeds service_packages.max_items.
	ErrTooManyPackageItems = errors.New("too many package items")
	// ErrItemCrossOrg — component_content_id links to a listing owned by a
	// different org (§111: same-org links only).
	ErrItemCrossOrg = errors.New("package items can only link to the same organization's listings")
	// ErrItemIsPackage — a package cannot nest another package as a
	// component (§111: components are services).
	ErrItemIsPackage = errors.New("package items cannot link to another package")
	// ErrInvalidDeparture — malformed departure input.
	ErrInvalidDeparture = errors.New("invalid departure")
	// ErrDepartureFull — capacity guard rejected a confirm (§111: 409
	// departure_full).
	ErrDepartureFull = errors.New("departure_full")
	// ErrInvalidBookingTransition — the requested status transition is not
	// legal from the booking's current state.
	ErrInvalidBookingTransition = errors.New("invalid_booking_transition")
	// ErrBookingUnavailable — the listing does not accept bookings (wrong
	// purchase_mode, unlisted, or suspended).
	ErrBookingUnavailable = errors.New("listing does not accept bookings")
	// ErrIdempotencyRequired — Rule 9: paid booking initiation requires the
	// Idempotency-Key header (400 idempotency_key_required).
	ErrIdempotencyRequired = errors.New("idempotency_key_required")
	// ErrInvalidIntent — intent value or intent/purchase_mode mismatch on
	// POST /services/:id/bookings (400 invalid_intent).
	ErrInvalidIntent = errors.New("invalid_intent")
	// ErrCheckoutFieldsRequired — provider/email are required to open the
	// platform-fee checkout on a paid booking (400 checkout_fields_required).
	ErrCheckoutFieldsRequired = errors.New("checkout_fields_required")
	// ErrInvalidProvider — the checkout provider is not in the §17 enum
	// (400 invalid_provider). Checked before the booking row is written.
	ErrInvalidProvider = errors.New("invalid_provider")
	// ErrInvalidPreferredDates — the preferred_dates object failed shape
	// or ordering validation (400 invalid_preferred_dates).
	ErrInvalidPreferredDates = errors.New("invalid_preferred_dates")
	// ErrInvalidID — a UUID path/query parameter failed to parse
	// (400 invalid_id).
	ErrInvalidID = errors.New("invalid_id")
	// ErrPackageOrgImmutable — the selling org of a package cannot change
	// post-create; the merchant of record is bound to the booking pipeline.
	ErrPackageOrgImmutable = errors.New("package_org_immutable")
)

// Create creates a new service listing — a contents row with
// content_type='service_listing' plus a service_listing_meta row. The
// listing is created in 'in_review' status and enters the admin review
// queue (BUG-013). For listing_kind='package' the §111 gates apply:
// feature flag, org membership, verified-org check, purchase_mode
// validation, items/departures write. canCreatePackage carries the
// caller's service.package.create permission (checked at route level).
func (s *Service) Create(ctx context.Context, authorID pgtype.UUID, input CreateServiceInput, canCreatePackage bool) (ServiceDTO, error) {
	if input.Title == "" {
		return ServiceDTO{}, errors.New("title is required")
	}
	// BUG-013: validate service_category against the §98 enum. Empty is
	// rejected because the column is NOT NULL.
	if input.ServiceCategory == "" || !IsValidCategory(input.ServiceCategory) {
		return ServiceDTO{}, ErrInvalidCategory
	}

	listingKind := input.ListingKind
	if listingKind == "" {
		listingKind = ListingKindService
	}

	if !IsValidListingKind(listingKind) {
		return ServiceDTO{}, ErrInvalidListingKind
	}

	isPackage := listingKind == ListingKindPackage

	if isPackage && !s.PackagesEnabled(ctx) {
		return ServiceDTO{}, ErrPackagesDisabled
	}

	if isPackage && !canCreatePackage {
		return ServiceDTO{}, ErrNotOwner // surfaced as 403 package_create_required upstream
	}

	var orgID pgtype.UUID
	if input.OrgID != "" {
		if err := orgID.Scan(input.OrgID); err != nil {
			return ServiceDTO{}, fmt.Errorf("invalid org_id: %w", err)
		}
	}

	if isPackage {
		// §111: packages are org-authored only.
		if !orgID.Valid {
			return ServiceDTO{}, ErrOrgRequired
		}

		// The caller must belong to the selling org (financial exposure —
		// stricter than plain listings, mirroring the §61 ads gate).
		isMember, err := s.store.IsOrgMember(ctx, orgID, authorID)
		if err != nil {
			return ServiceDTO{}, fmt.Errorf("servicedir: check org membership: %w", err)
		}

		if !isMember {
			return ServiceDTO{}, ErrNotOrgMember
		}

		// §111 verified-org gate: verified=true AND verification_type IN
		// ('business','professional_body'); relaxed by
		// service_packages.require_verified_org=false for testing.
		if s.settings.GetBoolOr(ctx, "service_packages.require_verified_org", true) {
			v, err := s.store.GetOrgVerification(ctx, orgID)
			if err != nil {
				return ServiceDTO{}, fmt.Errorf("servicedir: org verification check: %w", err)
			}

			vtype := ""
			if v.VerificationType != nil {
				vtype = *v.VerificationType
			}

			if !v.Verified || (vtype != "business" && vtype != VerificationTypeProfessionalBody) {
				return ServiceDTO{}, ErrOrgNotVerified
			}
		}
	}

	// §111 purchase_mode validation.
	purchaseMode := input.PurchaseMode
	if purchaseMode == "" {
		purchaseMode = PurchaseModeInquiry
	}

	if purchaseMode == "full" {
		return ServiceDTO{}, ErrPurchaseModeUnavailable
	}

	if !IsValidPurchaseMode(purchaseMode) {
		return ServiceDTO{}, ErrInvalidPurchaseMode
	}

	if !isPackage && purchaseMode != PurchaseModeInquiry && purchaseMode != PurchaseModeExternal {
		// Plain services can link out (external) or take inquiries; the
		// paid mode is package-only in this version.
		return ServiceDTO{}, ErrPurchaseModeUnavailable
	}

	if input.PricePer != "" && !IsValidPricePer(input.PricePer) {
		return ServiceDTO{}, ErrInvalidPricePer
	}

	if purchaseMode == PurchaseModePaid {
		if input.PriceAmount == nil || *input.PriceAmount <= 0 || input.PriceCurrency == "" {
			return ServiceDTO{}, ErrMissingPackagePrice
		}

		if !s.settings.GetBoolOr(ctx, "service_packages.fee_enabled", true) {
			return ServiceDTO{}, ErrPurchaseModeUnavailable
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

	availableFrom, err := parseOptionalDate(input.AvailableFrom)
	if err != nil {
		return ServiceDTO{}, fmt.Errorf("invalid available_from: %w", err)
	}

	availableUntil, err := parseOptionalDate(input.AvailableUntil)
	if err != nil {
		return ServiceDTO{}, fmt.Errorf("invalid available_until: %w", err)
	}

	itemParams, err := s.validatePackageItems(ctx, orgID, input.Items)
	if err != nil {
		return ServiceDTO{}, err
	}

	depParams, err := validatePackageDepartures(input.Departures)
	if err != nil {
		return ServiceDTO{}, err
	}

	contentRow, err := s.content.CreateContent(ctx, content.CreateContentInput{
		AuthorID:       authorID,
		OrganizationID: orgID,
		ContentType:    content.TypeServiceListing,
		Title:          input.Title,
		Body:           input.Body,
		// BUG-013: create as 'in_review' so the listing enters the admin
		// review queue and can be published. 'draft' would make it
		// invisible with no publish path. Packages additionally require
		// review_before_publish per §111 — same in_review state covers it.
		Status:     content.StatusInReview,
		Visibility: content.VisibilityPublic,
		GeoSpaceID: destSpaceID,
	})
	if err != nil {
		return ServiceDTO{}, fmt.Errorf("servicedir: create content: %w", err)
	}

	isRemote := input.IsRemote
	isVerified := false
	params := gen.CreateServiceListingMetaParams{
		ContentID:          contentRow.ID,
		OrgID:              orgID,
		ServiceCategory:    &input.ServiceCategory,
		DestinationSpaceID: destSpaceID,
		CitySpaceID:        citySpaceID,
		OriginCommunities:  originCommunities,
		IsRemote:           &isRemote,
		IsVerified:         &isVerified,
		ListingKind:        &listingKind,
		PriceAmount:        input.PriceAmount,
		Capacity:           input.Capacity,
		AvailableFrom:      availableFrom,
		AvailableUntil:     availableUntil,
	}

	if input.WebsiteURL != "" {
		params.WebsiteUrl = &input.WebsiteURL
	}

	if input.BookingURL != "" {
		params.BookingUrl = &input.BookingURL
	}

	if input.PriceRange != "" {
		params.PriceRange = &input.PriceRange
	}

	if input.PriceCurrency != "" {
		params.PriceCurrency = &input.PriceCurrency
	}

	if input.PricePer != "" {
		params.PricePer = &input.PricePer
	}

	if purchaseMode != "" {
		params.PurchaseMode = &purchaseMode
	}

	metaRow, err := s.store.CreateMeta(ctx, params)
	if err != nil {
		slog.Error("servicedir: create service_listing_meta failed after content creation", "content_id", contentRow.ID, "error", err)
		return ServiceDTO{}, fmt.Errorf("servicedir: create service_listing_meta: %w", err)
	}

	if isPackage {
		if err := s.store.ReplacePackageItems(ctx, metaRow.ID, itemParams); err != nil {
			slog.Error("servicedir: create package items failed", "meta_id", metaRow.ID, "error", err)
			return ServiceDTO{}, fmt.Errorf("servicedir: create package items: %w", err)
		}

		if err := s.store.ReplacePackageDepartures(ctx, metaRow.ID, nil, depParams); err != nil {
			slog.Error("servicedir: create package departures failed", "meta_id", metaRow.ID, "error", err)
			return ServiceDTO{}, fmt.Errorf("servicedir: create package departures: %w", err)
		}
	}

	return s.getServiceDTOByID(ctx, contentRow.ID)
}

// validatePackageItems validates the §111 items[] payload and converts it
// to insert params. Component links must point at service listings owned
// by the same org.
func (s *Service) validatePackageItems(ctx context.Context, orgID pgtype.UUID, items []PackageItemInput) ([]gen.CreateServicePackageItemParams, error) {
	if len(items) == 0 {
		return nil, nil
	}

	maxItems := int32(s.settings.GetIntOr(ctx, "service_packages.max_items", 25))
	if int32(len(items)) > maxItems {
		return nil, ErrTooManyPackageItems
	}

	params := make([]gen.CreateServicePackageItemParams, 0, len(items))

	for i, item := range items {
		if item.Title == "" {
			return nil, fmt.Errorf("items[%d]: title is required", i)
		}

		p := gen.CreateServicePackageItemParams{
			Position: int32(i), //nolint:gosec // bounded by max_items check
			Title:    item.Title,
		}

		if item.Description != "" {
			p.Description = &item.Description
		}

		if item.ComponentContentID != "" {
			var cid pgtype.UUID
			if err := cid.Scan(item.ComponentContentID); err != nil {
				return nil, fmt.Errorf("items[%d]: invalid component_content_id", i)
			}

			ownerOrg, ownerKind, err := s.store.GetListingOrgID(ctx, cid)
			if err != nil {
				return nil, fmt.Errorf("items[%d]: component listing not found", i)
			}

			if !ownerOrg.Valid || ownerOrg != orgID {
				return nil, ErrItemCrossOrg
			}

			if ownerKind == ListingKindPackage {
				return nil, fmt.Errorf("items[%d]: %w", i, ErrItemIsPackage)
			}

			p.ComponentContentID = cid
		}

		included := true
		if item.Included != nil {
			included = *item.Included
		}

		p.Included = &included

		optional := false
		if item.Optional != nil {
			optional = *item.Optional
		}

		p.Optional = &optional

		params = append(params, p)
	}

	return params, nil
}

// validatePackageDepartures validates the §111 departures[] payload.
func validatePackageDepartures(deps []PackageDepartureInput) ([]gen.CreateServicePackageDepartureParams, error) {
	if len(deps) == 0 {
		return nil, nil
	}

	params := make([]gen.CreateServicePackageDepartureParams, 0, len(deps))

	for i, d := range deps {
		if d.StartsOn == "" {
			return nil, fmt.Errorf("departures[%d]: starts_on is required", i)
		}

		startsOn, err := parseOptionalDate(d.StartsOn)
		if err != nil || !startsOn.Valid {
			return nil, fmt.Errorf("departures[%d]: invalid starts_on: %w", i, err)
		}

		endsOn, err := parseOptionalDate(d.EndsOn)
		if err != nil {
			return nil, fmt.Errorf("departures[%d]: invalid ends_on: %w", i, err)
		}

		if endsOn.Valid && endsOn.Time.Before(startsOn.Time) {
			return nil, fmt.Errorf("departures[%d]: ends_on precedes starts_on", i)
		}

		status := DepartureStatusOpen
		if d.Status == DepartureStatusCancelled {
			status = DepartureStatusCancelled
		} else if d.Status != "" && d.Status != DepartureStatusOpen {
			return nil, fmt.Errorf("departures[%d]: invalid status %q", i, d.Status)
		}

		if d.Capacity != nil && *d.Capacity <= 0 {
			return nil, fmt.Errorf("departures[%d]: capacity must be positive", i)
		}

		params = append(params, gen.CreateServicePackageDepartureParams{
			StartsOn: startsOn,
			EndsOn:   endsOn,
			Capacity: d.Capacity,
			Status:   &status,
		})
	}

	return params, nil
}

// parseOptionalDate parses an ISO 8601 date string into pgtype.Date;
// empty input produces an invalid (NULL) date.
func parseOptionalDate(v string) (pgtype.Date, error) {
	var d pgtype.Date
	if v == "" {
		return d, nil
	}

	t, err := time.Parse("2006-01-02", v)
	if err != nil {
		return d, err
	}

	d = pgtype.Date{Time: t, Valid: true}

	return d, nil
}

// GetBySlug returns a single published service listing by its slug.
// Anonymous-accessible.
func (s *Service) GetBySlug(ctx context.Context, slug string) (ServiceDTO, error) {
	row, err := s.store.GetBySlug(ctx, slug)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ServiceDTO{}, pgx.ErrNoRows
		}

		return ServiceDTO{}, fmt.Errorf("servicedir: get by slug: %w", err)
	}

	// §111 review gate: an in_review/draft listing must not be reachable
	// by direct URL — the public detail route serves published rows only.
	if row.Status != content.StatusPublished {
		return ServiceDTO{}, pgx.ErrNoRows
	}

	// Packages vanish with the feature flag — list, booking, and write
	// routes all gate on service_packages.enabled; the detail route does
	// the same so a disabled feature leaves no public surface.
	if row.ListingKind == ListingKindPackage && !s.PackagesEnabled(ctx) {
		return ServiceDTO{}, pgx.ErrNoRows
	}

	dto := rowToDTO(row)

	if row.ListingKind == ListingKindPackage {
		s.attachPackageDataCached(ctx, row.ID, row.MetaID, &dto)
	}

	return dto, nil
}

// packageDetailCache is the cached slice of a package detail response —
// items, open departures, and the quoted platform fee (jc:svc_package:*,
// 15min per conventions/valkey-keys-catalog.md).
type packageDetailCache struct {
	Items           []PackageItemDTO      `json:"items"`
	Departures      []PackageDepartureDTO `json:"departures"`
	PlatformFee     *int64                `json:"platform_fee,omitempty"`
	AgentBalanceDue *int64                `json:"agent_balance_due,omitempty"`
}

// attachPackageDataCached serves the §111 package detail fields through
// the Valkey cache when available; a miss falls back to
// attachPackageData and populates the key. Cache failures degrade to a
// direct DB read — the cache is an optimization, not a gate.
func (s *Service) attachPackageDataCached(ctx context.Context, contentID, metaID pgtype.UUID, dto *ServiceDTO) {
	if s.vk != nil {
		cacheKey := "jc:svc_package:" + contentID.String()
		if raw, err := s.vk.Get(ctx, cacheKey).Bytes(); err == nil {
			var cached packageDetailCache
			if json.Unmarshal(raw, &cached) == nil {
				dto.Items = cached.Items
				dto.Departures = cached.Departures
				dto.PlatformFee = cached.PlatformFee
				dto.AgentBalanceDue = cached.AgentBalanceDue

				return
			}
		}
	}

	if err := s.attachPackageData(ctx, metaID, dto); err != nil {
		slog.Warn("servicedir: attach package data failed", "content_id", contentID, "error", err)
		return
	}

	if s.vk == nil {
		return
	}

	payload, err := json.Marshal(packageDetailCache{
		Items:           dto.Items,
		Departures:      dto.Departures,
		PlatformFee:     dto.PlatformFee,
		AgentBalanceDue: dto.AgentBalanceDue,
	})
	if err != nil {
		return
	}

	cctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := s.vk.Set(cctx, "jc:svc_package:"+contentID.String(), payload, 15*time.Minute).Err(); err != nil {
		slog.Warn("servicedir: package cache set failed", "content_id", contentID, "error", err)
	}
}

// attachPackageData fills the §111 detail fields — ordered items, open
// departures, and the platform-fee quote for paid packages.
func (s *Service) attachPackageData(ctx context.Context, metaID pgtype.UUID, dto *ServiceDTO) error {
	items, err := s.store.ListPackageItems(ctx, metaID)
	if err != nil {
		return err
	}

	dto.Items = make([]PackageItemDTO, 0, len(items))
	for _, it := range items {
		item := PackageItemDTO{
			ID:       it.ID.String(),
			Position: it.Position,
			Title:    it.Title,
		}
		if it.Description != nil {
			item.Description = *it.Description
		}

		if it.ComponentContentID.Valid {
			item.ComponentContentID = it.ComponentContentID.String()
		}

		item.Included = it.Included
		item.Optional = it.Optional

		dto.Items = append(dto.Items, item)
	}

	deps, err := s.store.ListOpenPackageDepartures(ctx, metaID)
	if err != nil {
		return err
	}

	dto.Departures = make([]PackageDepartureDTO, 0, len(deps))
	for _, d := range deps {
		dep := PackageDepartureDTO{
			ID:          d.ID.String(),
			Capacity:    d.Capacity,
			BookedCount: d.BookedCount,
			Status:      d.Status,
		}
		if d.StartsOn.Valid {
			dep.StartsOn = d.StartsOn.Time.Format("2006-01-02")
		}

		if d.EndsOn.Valid {
			dep.EndsOn = d.EndsOn.Time.Format("2006-01-02")
		}

		dto.Departures = append(dto.Departures, dep)
	}

	// Quote the platform fee for paid packages so the checkout copy can
	// show "Reserve — {fee} platform fee" before the user commits (§111).
	if dto.PurchaseMode == PurchaseModePaid && dto.PriceAmount != nil && *dto.PriceAmount > 0 {
		fee := s.computeFee(ctx, *dto.PriceAmount)
		balance := *dto.PriceAmount - fee
		dto.PlatformFee = &fee
		dto.AgentBalanceDue = &balance
	}

	return nil
}

// List returns a paginated, filtered list of published service listings.
// destSpaceID, citySpaceID, serviceCategory, originCommunity are optional
// geo-aware filters (§98: ?dest=&city=&category=&origin=&verified=&sort=).
func (s *Service) List(ctx context.Context, destSpaceID, citySpaceID pgtype.UUID, serviceCategory, originCommunity string, isVerified *bool, listingKind string, priceMax *int64, dateFrom pgtype.Date, sort string, page, limit int32) ([]ServiceDTO, int64, error) {
	if limit <= 0 {
		limit = 20
	}

	if page <= 0 {
		page = 1
	}

	offset := (page - 1) * limit

	// §111: the kind=package filter is gated on the packages flag — off
	// flag means only plain service rows list.
	if listingKind == ListingKindPackage && !s.PackagesEnabled(ctx) {
		return nil, 0, ErrPackagesDisabled
	}

	rows, total, err := s.store.ListServices(ctx, ListServicesFilter{

		DestSpaceID:     destSpaceID,
		CitySpaceID:     citySpaceID,
		ServiceCategory: serviceCategory,
		OriginCommunity: originCommunity,
		IsVerified:      isVerified,
		ListingKind:     listingKind,
		PriceMax:        priceMax,
		DateFrom:        dateFrom,
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
// Only the provided (non-nil) fields are updated — the meta row is merged
// over the stored row so omitted fields retain their values.
//
// §111: listing_kind is immutable post-create (a service can't be
// retrofitted into a package — component integrity). Package pricing,
// purchase_mode, capacity, availability, items[] and departures[] are
// writable on package listings by service.package.create holders.
func (s *Service) EditOwn(ctx context.Context, contentID, userID pgtype.UUID, input EditServiceInput, isEditAny, canCreatePackage bool) (ServiceDTO, error) {
	existing, err := s.content.GetContent(ctx, contentID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ServiceDTO{}, pgx.ErrNoRows
		}

		return ServiceDTO{}, fmt.Errorf("servicedir: edit: get content: %w", err)
	}

	meta, err := s.store.GetMeta(ctx, contentID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ServiceDTO{}, pgx.ErrNoRows
		}

		return ServiceDTO{}, fmt.Errorf("servicedir: edit: get meta: %w", err)
	}

	// service.edit_any holders (admins) bypass the ownership check (BUG-009).
	if !isEditAny {
		// Check author ownership first (BUG-008).
		if existing.AuthorID != userID {
			// Not the author — check if the user is a member of the org
			// that owns the listing (§98: service.edit_own = "listing owner org").
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

	isPackage := meta.ListingKind == ListingKindPackage

	// §111 package gates run BEFORE any write — the package.create check
	// must precede the content update or a caller without the permission
	// could still change a package's title/body.
	if isPackage {
		if !s.PackagesEnabled(ctx) {
			return ServiceDTO{}, ErrPackagesDisabled
		}

		if !isEditAny && !canCreatePackage {
			return ServiceDTO{}, ErrNotOwner
		}
	}

	// Merge the patch over the stored meta row — pure input validation,
	// no writes yet.
	params := gen.UpdateServiceListingMetaParams{
		ContentID:          contentID,
		OrgID:              meta.OrgID,
		ServiceCategory:    &meta.ServiceCategory,
		DestinationSpaceID: meta.DestinationSpaceID,
		CitySpaceID:        meta.CitySpaceID,
		OriginCommunities:  meta.OriginCommunities,
		IsRemote:           &meta.IsRemote,
		WebsiteUrl:         meta.WebsiteUrl,
		BookingUrl:         meta.BookingUrl,
		PriceRange:         meta.PriceRange,
		ListingKind:        &meta.ListingKind,
		PriceAmount:        meta.PriceAmount,
		PriceCurrency:      meta.PriceCurrency,
		PricePer:           meta.PricePer,
		PurchaseMode:       &meta.PurchaseMode,
		Capacity:           meta.Capacity,
		AvailableFrom:      meta.AvailableFrom,
		AvailableUntil:     meta.AvailableUntil,
	}

	if input.OrgID != nil && *input.OrgID != "" {
		// §111: a package's org_id is the merchant of record — open
		// bookings, the org inbox, and fee settlement all key on it, so
		// the selling org cannot change post-create. (Service listings
		// keep the §98 re-attribution behaviour.)
		if isPackage && *input.OrgID != meta.OrgID.String() {
			return ServiceDTO{}, ErrPackageOrgImmutable
		}

		if err := params.OrgID.Scan(*input.OrgID); err != nil {
			return ServiceDTO{}, fmt.Errorf("servicedir: edit: invalid org_id: %w", err)
		}
	}

	if input.DestinationSpaceID != nil && *input.DestinationSpaceID != "" {
		if err := params.DestinationSpaceID.Scan(*input.DestinationSpaceID); err != nil {
			return ServiceDTO{}, fmt.Errorf("servicedir: edit: invalid destination_space_id: %w", err)
		}
	}

	if input.CitySpaceID != nil && *input.CitySpaceID != "" {
		if err := params.CitySpaceID.Scan(*input.CitySpaceID); err != nil {
			return ServiceDTO{}, fmt.Errorf("servicedir: edit: invalid city_space_id: %w", err)
		}
	}

	if input.ServiceCategory != nil {
		// BUG-013: validate category on edit when supplied.
		if *input.ServiceCategory == "" || !IsValidCategory(*input.ServiceCategory) {
			return ServiceDTO{}, ErrInvalidCategory
		}

		params.ServiceCategory = input.ServiceCategory
	}

	if input.OriginCommunities != nil {
		oc, err := normalizeOriginCommunities(input.OriginCommunities)
		if err != nil {
			return ServiceDTO{}, fmt.Errorf("servicedir: edit: invalid origin_communities: %w", err)
		}

		params.OriginCommunities = oc
	}

	if input.IsRemote != nil {
		params.IsRemote = input.IsRemote
	}

	if input.WebsiteURL != nil {
		params.WebsiteUrl = input.WebsiteURL
	}

	if input.BookingURL != nil {
		params.BookingUrl = input.BookingURL
	}

	if input.PriceRange != nil {
		params.PriceRange = input.PriceRange
	}

	// §111: listing_kind is immutable post-create.
	if input.ListingKind != nil && *input.ListingKind != meta.ListingKind {
		return ServiceDTO{}, ErrInvalidListingKind
	}

	if isPackage {
		if input.PurchaseMode != nil {
			switch {
			case *input.PurchaseMode == "full":
				return ServiceDTO{}, ErrPurchaseModeUnavailable
			case !IsValidPurchaseMode(*input.PurchaseMode):
				return ServiceDTO{}, ErrInvalidPurchaseMode
			default:
				params.PurchaseMode = input.PurchaseMode
			}
		}

		if input.PriceAmount != nil {
			params.PriceAmount = input.PriceAmount
		}

		if input.PriceCurrency != nil {
			params.PriceCurrency = input.PriceCurrency
		}

		if input.PricePer != nil {
			if !IsValidPricePer(*input.PricePer) {
				return ServiceDTO{}, ErrInvalidPricePer
			}

			params.PricePer = input.PricePer
		}

		if input.Capacity != nil {
			params.Capacity = input.Capacity
		}

		if input.AvailableFrom != nil {
			af, err := parseOptionalDate(*input.AvailableFrom)
			if err != nil {
				return ServiceDTO{}, fmt.Errorf("invalid available_from: %w", err)
			}

			params.AvailableFrom = af
		}

		if input.AvailableUntil != nil {
			au, err := parseOptionalDate(*input.AvailableUntil)
			if err != nil {
				return ServiceDTO{}, fmt.Errorf("invalid available_until: %w", err)
			}

			params.AvailableUntil = au
		}

		if *params.PurchaseMode == PurchaseModePaid {
			if params.PriceAmount == nil || *params.PriceAmount <= 0 ||
				params.PriceCurrency == nil || *params.PriceCurrency == "" {
				return ServiceDTO{}, ErrMissingPackagePrice
			}

			if !s.settings.GetBoolOr(ctx, "service_packages.fee_enabled", true) {
				return ServiceDTO{}, ErrPurchaseModeUnavailable
			}
		}
	}

	// Validate the replace-on-save collections before any write so a
	// rejected edit doesn't leave a half-updated listing (title committed,
	// items rejected, status bounced back into review).
	var itemParams []gen.CreateServicePackageItemParams
	if isPackage && input.Items != nil {
		itemParams, err = s.validatePackageItems(ctx, params.OrgID, *input.Items)
		if err != nil {
			return ServiceDTO{}, err
		}
	}

	var depParams []gen.CreateServicePackageDepartureParams
	var keepIDs []pgtype.UUID
	if isPackage && input.Departures != nil {
		depParams, err = validatePackageDepartures(*input.Departures)
		if err != nil {
			return ServiceDTO{}, err
		}

		// §111: departures that already have bookings are preserved —
		// compute the keep set so replace-on-save never drops them.
		keepIDs, err = s.store.DeparturesWithBookings(ctx, meta.ID)
		if err != nil {
			return ServiceDTO{}, fmt.Errorf("servicedir: edit: list booked departures: %w", err)
		}
	}

	// All input validated — start writing. Title/body first via the
	// §14 content service.
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

	// §111: commercial edits (price_amount, items, purchase_mode) on a
	// published package re-enter the review queue — same gate as create.
	if isPackage && existing.Status == content.StatusPublished &&
		s.settings.GetBoolOr(ctx, "service_packages.review_before_publish", true) &&
		(input.PriceAmount != nil || input.PurchaseMode != nil || input.Items != nil) {
		updateInput.Status = content.StatusInReview
	}

	if _, err := s.content.UpdateContent(ctx, contentID, userID, updateInput, false); err != nil {
		return ServiceDTO{}, fmt.Errorf("servicedir: edit: update content: %w", err)
	}

	if _, err := s.store.UpdateMeta(ctx, params); err != nil {
		return ServiceDTO{}, fmt.Errorf("servicedir: edit: update service_listing_meta: %w", err)
	}

	if isPackage && input.Items != nil {
		if err := s.store.ReplacePackageItems(ctx, meta.ID, itemParams); err != nil {
			return ServiceDTO{}, fmt.Errorf("servicedir: edit: replace package items: %w", err)
		}
	}

	if isPackage && input.Departures != nil {
		if err := s.store.ReplacePackageDepartures(ctx, meta.ID, keepIDs, depParams); err != nil {
			return ServiceDTO{}, fmt.Errorf("servicedir: edit: replace package departures: %w", err)
		}
	}

	if isPackage {
		s.invalidatePackageCache(contentID)
	}

	return s.getServiceDTOByID(ctx, contentID)
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
func (s *Service) Claim(ctx context.Context, contentID, userID, orgID pgtype.UUID) error {
	isMember, err := s.store.IsOrgMember(ctx, orgID, userID)
	if err != nil {
		return fmt.Errorf("servicedir: claim: check org membership: %w", err)
	}

	if !isMember {
		return ErrNotOrgMember
	}

	if err := s.store.ClaimIfUnclaimed(ctx, contentID, orgID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrAlreadyClaimed
		}

		return fmt.Errorf("servicedir: claim: %w", err)
	}

	return nil
}

// AdminList returns all service listings (any status) for the moderation
// queue. Supports optional dest filter (BUG-011: geo-aware pattern on
// admin list endpoints) and the §111 ?kind= filter for the package
// review queue.
func (s *Service) AdminList(ctx context.Context, status, claimStatus, serviceCategory, listingKind, q string, isVerified *bool, destSpaceID pgtype.UUID, page, limit int32) ([]ServiceDTO, int64, error) {
	if limit <= 0 {
		limit = 20
	}

	if page <= 0 {
		page = 1
	}

	offset := (page - 1) * limit

	rows, total, err := s.store.AdminList(ctx, AdminListFilter{

		DestSpaceID:     destSpaceID,
		Status:          status,
		IsVerified:      isVerified,
		ClaimStatus:     claimStatus,
		ServiceCategory: serviceCategory,
		ListingKind:     listingKind,
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
func (s *Service) AdminVerify(ctx context.Context, contentID, verifiedBy pgtype.UUID, verificationType string) error {
	if verificationType == "" {
		verificationType = VerificationTypeAdmin
	}

	if !IsValidVerificationType(verificationType) {
		return ErrInvalidVerificationType
	}

	if err := s.store.VerifyService(ctx, contentID, verifiedBy, verificationType); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return pgx.ErrNoRows
		}

		return fmt.Errorf("servicedir: admin verify: %w", err)
	}

	return nil
}

// AdminUnverify marks a service listing as unverified by an admin
// (service.verify permission). Clears the verified flag and audit trail.
func (s *Service) AdminUnverify(ctx context.Context, contentID pgtype.UUID) error {
	if err := s.store.UnverifyService(ctx, contentID); err != nil {
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
func (s *Service) AdminResolveClaim(ctx context.Context, contentID, orgID pgtype.UUID, approve bool) error {
	if err := s.store.ResolveClaimIfPending(ctx, contentID, orgID, approve); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return pgx.ErrNoRows
		}

		return fmt.Errorf("servicedir: admin resolve claim: %w", err)
	}

	return nil
}

// getServiceDTOByID fetches a service listing by content_id and converts
// to DTO.
func (s *Service) getServiceDTOByID(ctx context.Context, contentID pgtype.UUID) (ServiceDTO, error) {
	contentRow, err := s.content.GetContent(ctx, contentID)
	if err != nil {
		return ServiceDTO{}, fmt.Errorf("servicedir: get by id: %w", err)
	}

	meta, err := s.store.GetMeta(ctx, contentID)
	if err != nil {
		return ServiceDTO{}, fmt.Errorf("servicedir: get meta by id: %w", err)
	}

	dto := metaToDTO(contentRow, meta)

	if meta.ListingKind == ListingKindPackage {
		if err := s.attachPackageData(ctx, meta.ID, &dto); err != nil {
			slog.Warn("servicedir: attach package data failed", "content_id", contentID, "error", err)
		}
	}

	return dto, nil
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

	// §111 package fields.
	dto.ListingKind = r.ListingKind
	dto.PriceAmount = r.PriceAmount
	dto.PurchaseMode = r.PurchaseMode
	dto.Capacity = r.Capacity
	if r.PriceCurrency != nil {
		dto.PriceCurrency = *r.PriceCurrency
	}

	if r.PricePer != nil {
		dto.PricePer = *r.PricePer
	}

	if r.AvailableFrom.Valid {
		dto.AvailableFrom = r.AvailableFrom.Time.Format("2006-01-02")
	}

	if r.AvailableUntil.Valid {
		dto.AvailableUntil = r.AvailableUntil.Time.Format("2006-01-02")
	}

	return dto
}

// metaToDTO converts a content row + service_listing_meta into a ServiceDTO.
func metaToDTO(c gen.GetContentByIDRow, m gen.ServiceListingMetum) ServiceDTO {
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

	dto.ServiceCategory = m.ServiceCategory

	if m.DestinationSpaceID.Valid {
		dto.DestSpaceID = m.DestinationSpaceID.String()
	}

	if m.CitySpaceID.Valid {
		dto.CitySpaceID = m.CitySpaceID.String()
	}

	dto.OriginCommunities = m.OriginCommunities
	if m.WebsiteUrl != nil {
		dto.WebsiteURL = *m.WebsiteUrl
	}

	if m.BookingUrl != nil {
		dto.BookingURL = *m.BookingUrl
	}

	if m.PriceRange != nil {
		dto.PriceRange = *m.PriceRange
	}

	if m.VerificationType != nil {
		dto.VerificationType = *m.VerificationType
	}

	dto.ClaimStatus = m.ClaimStatus

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

	// §111 package fields.
	dto.ListingKind = m.ListingKind
	dto.PriceAmount = m.PriceAmount
	dto.PurchaseMode = m.PurchaseMode
	dto.Capacity = m.Capacity
	if m.PriceCurrency != nil {
		dto.PriceCurrency = *m.PriceCurrency
	}

	if m.PricePer != nil {
		dto.PricePer = *m.PricePer
	}

	if m.AvailableFrom.Valid {
		dto.AvailableFrom = m.AvailableFrom.Time.Format("2006-01-02")
	}

	if m.AvailableUntil.Valid {
		dto.AvailableUntil = m.AvailableUntil.Time.Format("2006-01-02")
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
