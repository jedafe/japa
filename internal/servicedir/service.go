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
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/valkey-io/valkey-go/valkeycompat"

	"github.com/jedafe/japa/internal/config"
	"github.com/jedafe/japa/internal/content"
	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/media"
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

	// Marketplace card surfaces — populated from the media +
	// review_aggregates JOINs on the listing queries (§98 detail card /
	// marketplace listing spec). FeaturedMediaID serializes
	// contents.featured_image_id so the authoring UI can round-trip the
	// picked media row; FeaturedImageURL is the imgproxy URL, omitted when
	// no live media row exists. RatingAvg/ReviewCount come from the §29
	// review_aggregates rollup (entity_type='service_listing',
	// entity_id=contents.id); omitted when the listing has no reviews yet.
	FeaturedMediaID  string  `json:"featured_media_id,omitempty"`
	FeaturedImageURL string  `json:"featured_image_url,omitempty"`
	RatingAvg        float64 `json:"rating_avg,omitempty"`
	ReviewCount      int32   `json:"review_count,omitempty"`

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

	// Marketplace coupon surfaces — gated on service_coupons.enabled.
	// Coupons is populated on the public detail payload (active coupons
	// only); the field is omitted entirely when the flag is off.
	// HasCoupon rides list rows only (the SQL EXISTS badge) and is likewise
	// suppressed when the flag is off.
	Coupons   []ListingCoupon `json:"coupons,omitempty"`
	HasCoupon bool            `json:"has_coupon,omitempty"`

	// Marketplace tier surfaces — gated on service_packages.tiers_enabled.
	// Tiers is populated on the package detail payload (active tiers only,
	// position order). MinPrice rides list rows only — the "From $X" card
	// price (lowest active tier, fallback price_amount).
	Tiers    []PackageTierDTO `json:"tiers,omitempty"`
	MinPrice *int64           `json:"min_price,omitempty"`

	// §114 structured content — faqs[]/steps[] ride the listing payloads
	// (detail + list rows); omitted when the stored set is empty.
	Faqs  []ServiceFaqDTO  `json:"faqs,omitempty"`
	Steps []ServiceStepDTO `json:"steps,omitempty"`

	// §114 derived status ribbon — 'popular' once the listing clears
	// service_marketplace.badges.min_reviews, else 'best_value' when it
	// carries a live coupon and clears service_marketplace.badges.min_rating.
	// 'popular' wins; omitted when neither threshold is met. Stamped on
	// list rows and the detail payload by the call sites that already
	// stamp has_coupon/min_price (rowToDTO has no settings context).
	Badge string `json:"badge,omitempty"`

	// Mentor block (000824 unification) — populated on list + detail
	// payloads when listing_kind='mentorship' so mentor surfaces render
	// capacity/areas without a second fetch. Omitted on all other kinds.
	Mentor *MentorDTO `json:"mentor,omitempty"`
}

// MentorDTO is the mentor-profile card serialized on
// listing_kind='mentorship' payloads (000824 unification). The fields ride
// the author user_profiles join on the listing queries — is_mentor=false
// means the author has lapsed out of the program and the frontend treats
// the listing as unavailable.
type MentorDTO struct {
	Username          string   `json:"username"`
	DisplayName       string   `json:"display_name,omitempty"`
	AvatarURL         string   `json:"avatar_url,omitempty"`
	Areas             []string `json:"areas,omitempty"`
	Capacity          int32    `json:"capacity"`
	ActiveMenteeCount int32    `json:"active_mentee_count"`
	AvailableSlots    int32    `json:"available_slots"`
	MentorSince       string   `json:"mentor_since,omitempty"`
	IsMentor          bool     `json:"is_mentor"`
}

// ServiceFaqDTO is one FAQ entry stored in service_listing_meta.faqs (§114).
type ServiceFaqDTO struct {
	Question string `json:"question"`
	Answer   string `json:"answer"`
}

// ServiceStepDTO is one "how it works" step stored in
// service_listing_meta.steps (§114).
type ServiceStepDTO struct {
	Title       string `json:"title"`
	Description string `json:"description"`
}

// ServiceFaqInput / ServiceStepInput are the write-side shapes for the
// faqs[]/steps[] payloads. On PATCH a nil slice pointer means "keep the
// stored set", an empty array clears it — the same replace-on-save
// semantics as items[]/tiers[] (§111).
type ServiceFaqInput struct {
	Question string `json:"question"`
	Answer   string `json:"answer"`
}

// ServiceStepInput — see ServiceFaqInput for the replace-on-save contract.
type ServiceStepInput struct {
	Title       string `json:"title"`
	Description string `json:"description"`
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
// payload. On PATCH, an id referencing an existing departure of the same
// package updates that row in place (preserving booking links); rows
// without an id insert, and existing rows absent from the payload delete —
// except departures carrying bookings, which are always preserved.
type PackageDepartureInput struct {
	ID       string `json:"id,omitempty"` // edit only: existing departure id
	StartsOn string `json:"starts_on"`    // ISO date
	EndsOn   string `json:"ends_on"`      // optional ISO date
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
	// FeaturedImageID sets contents.featured_image_id (media row id); the
	// payload name featured_media_id matches the serialized DTO field so
	// the authoring UI round-trips one key. Empty = no featured image.
	FeaturedImageID string `json:"featured_media_id"`

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
	Tiers          []TierInput             `json:"tiers"`

	// §114 structured content (all listing kinds).
	Faqs  []ServiceFaqInput  `json:"faqs"`
	Steps []ServiceStepInput `json:"steps"`
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
	// FeaturedImageID patches contents.featured_image_id — nil keeps the
	// stored media row, "" clears it, a UUID string replaces it.
	FeaturedImageID *string `json:"featured_media_id"`

	// §111 package fields. items[]/departures[]/tiers[] are replace-on-save:
	// a non-nil slice replaces the whole set.
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
	Tiers          *[]TierInput             `json:"tiers"`

	// §114 structured content — replace-on-save: nil keeps the stored set,
	// an empty array clears it. Available on all listing kinds.
	Faqs  *[]ServiceFaqInput  `json:"faqs"`
	Steps *[]ServiceStepInput `json:"steps"`
}

// SettingsReader is the narrow settings contract servicedir needs
// (§98/§111 flag + service_packages.* keys). *config.Settings satisfies
// it; tests substitute an in-memory fake.
type SettingsReader interface {
	GetBoolOr(ctx context.Context, key string, fallback bool) bool
	GetIntOr(ctx context.Context, key string, fallback int) int
	GetFloatOr(ctx context.Context, key string, fallback float64) float64
	GetStringOr(ctx context.Context, key, fallback string) string
}

// Service holds the business logic for the Service Directory.
type Service struct {
	store     *Store
	content   ContentService
	settings  SettingsReader
	checker   FeatureChecker
	vk        valkeycompat.Cmdable  // §111 package detail cache (jc:svc_package:*)
	enqueuer  TaskEnqueuer          // §111 booking notifications/fee settlement
	payments  PaymentCreator        // §17 fee payment intents
	dmCreator DMConversationCreator // §48 DM seam — mentorship accept auto-open (wired by server)
}

// TaskEnqueuer enqueues Asynq tasks (§111 booking lifecycle).
type TaskEnqueuer interface {
	Enqueue(taskType string, payload []byte, opts ...interface{}) error
}

// PaymentCreator is the narrow §17 payments seam for platform-fee intents.
type PaymentCreator interface {
	CreatePayment(ctx context.Context, userID pgtype.UUID, input PaymentInput) (PaymentResult, error)
	// ResumePayment re-initiates a hosted checkout on an existing pending
	// payment row (§111 POST /me/service-bookings/:id/pay).
	ResumePayment(ctx context.Context, paymentID, userID pgtype.UUID, email, returnURL string) (PaymentResult, error)
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

// DMConversationCreator opens a direct-message thread between two users
// (§48/ADR-110 seam). The websocket.DMStore satisfies this signature —
// wired by the server package; until then the field is nil and the
// mentorship-accept auto-open is skipped silently.
type DMConversationCreator interface {
	GetOrCreateConversation(ctx context.Context, participant1ID, participant2ID pgtype.UUID) (gen.DmConversation, error)
}

// SetDMCreator injects the §48 DM creator used to auto-open a thread
// between mentor and mentee on mentorship-request accept.
func (s *Service) SetDMCreator(d DMConversationCreator) { s.dmCreator = d }

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

// MentorshipEnabled reads the mentorship.enabled flag (§102/000824) —
// gates mentorship-kind bookings, the provider respond route, the
// role=provider inbox, and the kind=mentorship list filter. Same shape
// as PackagesEnabled.
func (s *Service) MentorshipEnabled(ctx context.Context) bool {
	if s.checker != nil {
		return s.checker.IsEnabled(ctx, "mentorship.enabled")
	}

	if s.settings == nil {
		return false
	}

	return s.settings.GetBoolOr(ctx, "mentorship.enabled", false)
}

// Option is a single selectable value exposed by the option endpoints.
// The taxonomy fields (description/icon/accent_color/applies_to) are
// additive — populated only by DB-backed sources such as
// service_categories; older consumers see the same {value,label} shape.
type Option struct {
	Value       string `json:"value"`
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
	Icon        string `json:"icon,omitempty"`
	AccentColor string `json:"accent_color,omitempty"`
	AppliesTo   string `json:"applies_to,omitempty"`
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

// Categories returns the service category options for pickers (Rule 16).
// Primary source is the service_categories table — the dynamic
// marketplace taxonomy, active rows only, in position order; the
// hardcoded §98 enum is the fallback when the store is unwired or the
// query errors so the endpoint never hard-fails.
func (s *Service) Categories(ctx context.Context) []Option {
	if s.store != nil {
		if rows, err := s.store.listCategories(ctx, false); err == nil && len(rows) > 0 {
			out := make([]Option, 0, len(rows))
			for _, r := range rows {
				if !r.IsActive {
					continue
				}

				out = append(out, Option{
					Value:       r.Slug,
					Label:       r.Label,
					Description: strOrEmpty(r.Description),
					Icon:        strOrEmpty(r.Icon),
					AccentColor: strOrEmpty(r.AccentColor),
					AppliesTo:   r.AppliesTo,
				})
			}

			return out
		}
	}

	out := make([]Option, 0, len(categoryLabels))
	for v, label := range categoryLabels {
		out = append(out, Option{Value: v, Label: label})
	}

	sortOptions(out)

	return out
}

// categorySet resolves the valid service_category set. Primary source is
// the service_categories table (dynamic taxonomy); the hardcoded §98 enum
// is the fallback when the store is unwired or the table errors — callers
// must never see a hard failure for category validation.
func (s *Service) categorySet(ctx context.Context) map[string]struct{} {
	if s.store != nil {
		if rows, err := s.store.listCategories(ctx, false); err == nil && len(rows) > 0 {
			set := make(map[string]struct{}, len(rows))
			for _, r := range rows {
				if r.IsActive {
					set[r.Slug] = struct{}{}
				}
			}
			return set
		}
	}
	return validCategories
}

// IsValidCategoryDB reports whether slug is a valid service_category in
// the dynamic taxonomy. Shares categorySet's fallback to the §98 static
// enum when the store is unwired or the query errors — the slug check is
// identical, this exported form exists for admin category tooling.
func (s *Service) IsValidCategoryDB(ctx context.Context, slug string) bool {
	_, ok := s.categorySet(ctx)[slug]
	return ok
}

// ErrInvalidCategory is returned when a create/edit input supplies a
// service_category slug that is not an active row in the dynamic
// service_categories taxonomy (with the §98 enum as fallback).
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
	// ErrInvalidItem — malformed package item input (missing title, bad
	// component reference).
	ErrInvalidItem = errors.New("invalid package item")
	// ErrInvalidDeparture — malformed departure input.
	ErrInvalidDeparture = errors.New("invalid departure")
	// ErrNotPackage — a package-scoped route hit a listing_kind='service' row.
	ErrNotPackage = errors.New("not a package listing")
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
	// ErrMentorshipDisabled — a mentorship-kind surface was hit while the
	// mentorship.enabled flag is off (404 mentorship_disabled).
	ErrMentorshipDisabled = errors.New("mentorship is not enabled")
	// ErrDuplicateRequest — the booker already holds an open mentorship
	// request (pending|confirmed) against the mentor's shadow listing
	// (409 duplicate_request). Replaces the dropped mentorship_connections
	// dedupe (000824).
	ErrDuplicateRequest = errors.New("duplicate_request")
	// ErrMentorAtCapacity — the provider's active_mentee_count has reached
	// mentor_capacity (409 mentor_at_capacity).
	ErrMentorAtCapacity = errors.New("mentor_at_capacity")
	// ErrSelfMentorship — the booker is the listing's own author/provider
	// (400 cannot_self_mentor).
	ErrSelfMentorship = errors.New("cannot request mentorship from yourself")
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
	// ErrInvalidFeaturedMedia — featured_media_id failed UUID parsing on a
	// create/edit payload (400 invalid_featured_media_id).
	ErrInvalidFeaturedMedia = errors.New("invalid featured_media_id")
)

// §114 structured-content sentinels — faqs[]/steps[] write validation
// (400 too_many_faqs / invalid_faq / too_many_steps / invalid_step).
var (
	// ErrTooManyFaqs — exceeds service_marketplace.content.max_faqs.
	ErrTooManyFaqs = errors.New("too many faqs")
	// ErrInvalidFaq — malformed faq entry (empty/oversized question,
	// oversized answer).
	ErrInvalidFaq = errors.New("invalid faq")
	// ErrTooManySteps — exceeds service_marketplace.content.max_steps.
	ErrTooManySteps = errors.New("too many steps")
	// ErrInvalidStep — malformed step entry (empty/oversized title,
	// oversized description).
	ErrInvalidStep = errors.New("invalid step")
)

// ---------------------------------------------------------------------------
// Admin service_categories CRUD (marketplace upgrade — dynamic taxonomy)
// ---------------------------------------------------------------------------

// CategoryInput is the admin payload for creating or patching a
// service_categories row. Slug is immutable after creation — listings
// reference it by value via service_listing_meta.service_category — so
// PATCH rejects a body slug that disagrees with the path slug. Position
// and IsActive are pointers so "not supplied" can ride the update query's
// COALESCE; the text fields use "" as not-supplied (clearing a field is
// not supported — set a new value instead).
type CategoryInput struct {
	Slug        string `json:"slug"`
	Label       string `json:"label"`
	Description string `json:"description"`
	Icon        string `json:"icon"`
	AccentColor string `json:"accent_color"`
	AppliesTo   string `json:"applies_to"`
	Position    *int32 `json:"position"`
	IsActive    *bool  `json:"is_active"`
}

// CategoryInputError is a validation failure on CategoryInput. Msg is
// safe to surface verbatim in the API error envelope (400 invalid_input).
type CategoryInputError struct{ Msg string }

func (e *CategoryInputError) Error() string { return e.Msg }

// ErrCategoryExists is returned when a category create hits the slug
// unique constraint (SQLSTATE 23505) — mapped to 409 category_exists.
var ErrCategoryExists = errors.New("service category slug already exists")

// categorySlugRe constrains taxonomy slugs to lowercase snake_case
// tokens, matching every seeded row in migration 000803.
var categorySlugRe = regexp.MustCompile(`^[a-z][a-z0-9_]{1,48}$`)

// categoryAppliesTo enumerates the CHECK constraint values on
// service_categories.applies_to ('mentorship' added by 000824).
var categoryAppliesTo = map[string]struct{}{
	"service":    {},
	"package":    {},
	"mentorship": {},
	"both":       {},
}

// categoryAccentPalette is the token-mapped palette admins pick from —
// free hex is rejected (marketplace spec §2.1). 'cyan' is included for
// compatibility with the seeded 'insurance' row (migration 000803).
var categoryAccentPalette = map[string]struct{}{
	"violet": {}, "green": {}, "amber": {}, "blue": {}, "sky": {},
	"emerald": {}, "rose": {}, "orange": {}, "slate": {}, "indigo": {},
	"gray": {}, "cyan": {},
}

func categoryInputErr(format string, args ...any) *CategoryInputError {
	return &CategoryInputError{Msg: fmt.Sprintf(format, args...)}
}

// validateCategoryInput enforces the taxonomy constraints shared by
// create and update. Slug format is checked only on create (the slug is
// immutable thereafter); label is required on every write because the
// update query sets it unconditionally.
func validateCategoryInput(in CategoryInput, creating bool) error {
	if creating && !categorySlugRe.MatchString(in.Slug) {
		return categoryInputErr("slug must match ^[a-z][a-z0-9_]{1,48}$")
	}

	if strings.TrimSpace(in.Label) == "" {
		return categoryInputErr("label is required")
	}

	if len(in.Label) > 80 {
		return categoryInputErr("label must be 80 characters or fewer")
	}

	if in.AppliesTo != "" {
		if _, ok := categoryAppliesTo[in.AppliesTo]; !ok {
			return categoryInputErr("applies_to must be one of service|package|mentorship|both")
		}
	}

	if in.AccentColor != "" {
		if _, ok := categoryAccentPalette[in.AccentColor]; !ok {
			return categoryInputErr("accent_color must be one of violet|green|amber|blue|sky|emerald|rose|orange|slate|indigo|gray|cyan")
		}
	}

	return nil
}

// AdminListCategories returns every service_categories row — active and
// inactive — in position order for the admin taxonomy manager.
func (s *Service) AdminListCategories(ctx context.Context) ([]gen.ServiceCategory, error) {
	return s.store.listCategories(ctx, false)
}

// AdminCreateCategory inserts a service_categories row after validation.
// A slug unique-violation maps to ErrCategoryExists (409 category_exists).
func (s *Service) AdminCreateCategory(ctx context.Context, in CategoryInput) (gen.ServiceCategory, error) {
	if err := validateCategoryInput(in, true); err != nil {
		return gen.ServiceCategory{}, err
	}

	// applies_to is COALESCE'd to 'both' — an explicit "" would land in the
	// CHECK constraint, so collapse it to NULL (which means "defaulted").
	var appliesTo interface{}
	if in.AppliesTo != "" {
		appliesTo = in.AppliesTo
	}

	row, err := s.store.createCategory(ctx, gen.CreateServiceCategoryParams{
		Slug:        in.Slug,
		Label:       in.Label,
		Description: nilIfEmpty(&in.Description),
		Icon:        nilIfEmpty(&in.Icon),
		AccentColor: nilIfEmpty(&in.AccentColor),
		AppliesTo:   appliesTo,
		Position:    in.Position, // *int32 — nil encodes as NULL → COALESCE default
		IsActive:    in.IsActive, // *bool — nil encodes as NULL → COALESCE default
	})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return gen.ServiceCategory{}, ErrCategoryExists
		}

		return gen.ServiceCategory{}, err
	}

	return row, nil
}

// AdminUpdateCategory patches a service_categories row. The slug is
// immutable — a body slug that disagrees with the path slug is rejected.
// description/icon/accent_color are unconditional SETs (sqlc.narg), so an
// omitted field carries the current value forward instead of clearing to
// NULL. Unknown slug surfaces pgx.ErrNoRows (404).
func (s *Service) AdminUpdateCategory(ctx context.Context, slug string, in CategoryInput) (gen.ServiceCategory, error) {
	if in.Slug != "" && in.Slug != slug {
		return gen.ServiceCategory{}, categoryInputErr("slug is immutable and cannot be changed")
	}

	if err := validateCategoryInput(in, false); err != nil {
		return gen.ServiceCategory{}, err
	}

	current, err := s.store.getCategoryBySlug(ctx, slug)
	if err != nil {
		return gen.ServiceCategory{}, err
	}

	description := current.Description
	if in.Description != "" {
		description = &in.Description
	}

	icon := current.Icon
	if in.Icon != "" {
		icon = &in.Icon
	}

	accentColor := current.AccentColor
	if in.AccentColor != "" {
		accentColor = &in.AccentColor
	}

	var appliesTo *string
	if in.AppliesTo != "" {
		appliesTo = &in.AppliesTo
	}

	return s.store.updateCategory(ctx, gen.UpdateServiceCategoryParams{
		Label:       in.Label,
		Description: description,
		Icon:        icon,
		AccentColor: accentColor,
		AppliesTo:   appliesTo,
		Position:    in.Position,
		IsActive:    in.IsActive,
		Slug:        slug,
	})
}

// AdminDeleteCategory removes a service_categories row. Slugs still
// referenced by listings are soft-disabled (is_active=false) so those
// listings keep resolving a label; unreferenced slugs are hard-deleted.
// Unknown slug surfaces pgx.ErrNoRows (404).
func (s *Service) AdminDeleteCategory(ctx context.Context, slug string) error {
	current, err := s.store.getCategoryBySlug(ctx, slug)
	if err != nil {
		return err
	}

	count, err := s.store.countListingsByCategory(ctx, slug)
	if err != nil {
		return err
	}

	if count > 0 {
		// In use → soft-disable via the update query. label/description/
		// icon/accent_color are unconditional SETs, so carry the current
		// values; only is_active actually changes.
		_, err = s.store.updateCategory(ctx, gen.UpdateServiceCategoryParams{
			Label:       current.Label,
			Description: current.Description,
			Icon:        current.Icon,
			AccentColor: current.AccentColor,
			IsActive:    boolPtr(false),
			Slug:        slug,
		})

		return err
	}

	return s.store.deleteCategory(ctx, slug)
}

// boolPtr returns a pointer to b.
func boolPtr(b bool) *bool { return &b }

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
	// BUG-013: validate service_category against the dynamic taxonomy —
	// categorySet resolves the service_categories table with the §98 enum
	// as fallback. Empty is rejected because the column is NOT NULL (and
	// "" never matches a slug).
	if _, ok := s.categorySet(ctx)[input.ServiceCategory]; input.ServiceCategory == "" || !ok {
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

	// contents.featured_image_id is a media row reference — parse only;
	// media existence/ownership is not validated (no FK on the column).
	var featuredImageID pgtype.UUID
	if input.FeaturedImageID != "" {
		if err := featuredImageID.Scan(input.FeaturedImageID); err != nil {
			return ServiceDTO{}, ErrInvalidFeaturedMedia
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

	// §114: faqs[]/steps[] apply to all listing kinds — validate + encode
	// to the jsonb column shape before any write.
	faqsJSON, err := s.encodeListingFaqs(ctx, input.Faqs)
	if err != nil {
		return ServiceDTO{}, err
	}

	stepsJSON, err := s.encodeListingSteps(ctx, input.Steps)
	if err != nil {
		return ServiceDTO{}, err
	}

	// tiers[] is gated on service_packages.tiers_enabled — with the flag
	// off the field is silently ignored (no validation, no write).
	var tierParams []gen.CreateServicePackageTierParams
	if isPackage && s.TiersEnabled(ctx) {
		tierParams, err = s.validatePackageTiers(ctx, input.Tiers)
		if err != nil {
			return ServiceDTO{}, err
		}
	}

	// BUG-013: create as 'in_review' so the listing enters the admin
	// review queue and can be published. 'draft' would make it
	// invisible with no publish path. Packages additionally require
	// review_before_publish per §111 — when that flag is off, a package
	// goes live immediately instead of entering the queue.
	createStatus := content.StatusInReview
	if isPackage && !s.settings.GetBoolOr(ctx, "service_packages.review_before_publish", true) {
		createStatus = content.StatusPublished
	}

	contentRow, err := s.content.CreateContent(ctx, content.CreateContentInput{
		AuthorID:        authorID,
		OrganizationID:  orgID,
		ContentType:     content.TypeServiceListing,
		Title:           input.Title,
		Body:            input.Body,
		Status:          createStatus,
		Visibility:      content.VisibilityPublic,
		GeoSpaceID:      destSpaceID,
		FeaturedImageID: featuredImageID,
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
		Faqs:               faqsJSON,
		Steps:              stepsJSON,
	}

	// §111: packages are org-authored only — they never enter the §98
	// claim flow, so the row starts pre-claimed (approved) by the owning
	// org rather than 'unclaimed'.
	if isPackage {
		approved := ClaimStatusApproved
		params.ClaimStatus = &approved
		params.ClaimedByOrgID = orgID
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
		// Compensating cleanup: without a meta row the content is an
		// orphaned 'service_listing' that no query can reach.
		if delErr := s.content.DeleteContent(ctx, contentRow.ID); delErr != nil {
			slog.Error("servicedir: failed to clean up orphaned content", "content_id", contentRow.ID, "error", delErr)
		}
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

		if s.TiersEnabled(ctx) {
			if err := s.store.ReplacePackageTiers(ctx, metaRow.ID, tierParams); err != nil {
				slog.Error("servicedir: create package tiers failed", "meta_id", metaRow.ID, "error", err)
				return ServiceDTO{}, fmt.Errorf("servicedir: create package tiers: %w", err)
			}
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
			return nil, fmt.Errorf("items[%d]: title is required: %w", i, ErrInvalidItem)
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
				return nil, fmt.Errorf("items[%d]: invalid component_content_id: %w", i, ErrInvalidItem)
			}

			ownerOrg, ownerKind, err := s.store.GetListingOrgID(ctx, cid)
			if err != nil {
				return nil, fmt.Errorf("items[%d]: component listing not found: %w", i, ErrInvalidItem)
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

// §114 badge values — the derived status ribbon on listing payloads.
const (
	BadgePopular   = "popular"
	BadgeBestValue = "best_value"
)

// deriveBadge computes the §114 marketplace ribbon: 'popular' once the
// listing's review count clears service_marketplace.badges.min_reviews
// (default 10), else 'best_value' when it carries a live coupon and its
// rating clears service_marketplace.badges.min_rating (default 4.0).
// 'popular' takes precedence; "" means no badge. hasCoupon must already
// reflect the service_coupons.enabled gate (call sites that suppress
// has_coupon when the flag is off pass the gated value through).
func (s *Service) deriveBadge(ctx context.Context, reviewCount int32, ratingAvg float64, hasCoupon bool) string {
	if s.settings == nil {
		return ""
	}

	minReviews := s.settings.GetIntOr(ctx, "service_marketplace.badges.min_reviews", 10)
	if minReviews > 0 && int64(reviewCount) >= int64(minReviews) {
		return BadgePopular
	}

	minRating := s.settings.GetFloatOr(ctx, "service_marketplace.badges.min_rating", 4.0)
	if hasCoupon && ratingAvg >= minRating {
		return BadgeBestValue
	}

	return ""
}

// §114 faqs[]/steps[] field length caps (system_settings cap the count;
// these bound the per-entry text — same convention as the §98 label
// length checks).
const (
	maxFaqStepTitleLen = 300  // question / step title
	maxFaqStepBodyLen  = 2000 // answer / step description
)

// encodeListingFaqs validates the faqs[] write payload against
// service_marketplace.content.max_faqs and the per-entry field caps, then
// marshals to the jsonb column shape. An empty (or nil) set encodes '[]'
// — callers clearing the field on PATCH rely on that.
func (s *Service) encodeListingFaqs(ctx context.Context, faqs []ServiceFaqInput) ([]byte, error) {
	maxFaqs := 12
	if s.settings != nil {
		maxFaqs = s.settings.GetIntOr(ctx, "service_marketplace.content.max_faqs", 12)
	}

	if len(faqs) > maxFaqs {
		return nil, ErrTooManyFaqs
	}

	out := make([]ServiceFaqDTO, 0, len(faqs))

	for i, f := range faqs {
		q := strings.TrimSpace(f.Question)
		if q == "" || len(q) > maxFaqStepTitleLen {
			return nil, fmt.Errorf("faqs[%d]: question must be 1-%d characters: %w", i, maxFaqStepTitleLen, ErrInvalidFaq)
		}

		if len(f.Answer) > maxFaqStepBodyLen {
			return nil, fmt.Errorf("faqs[%d]: answer must be %d characters or fewer: %w", i, maxFaqStepBodyLen, ErrInvalidFaq)
		}

		out = append(out, ServiceFaqDTO{Question: q, Answer: f.Answer})
	}

	b, err := json.Marshal(out)
	if err != nil {
		return nil, fmt.Errorf("servicedir: marshal faqs: %w", err)
	}

	return b, nil
}

// encodeListingSteps validates the steps[] write payload against
// service_marketplace.content.max_steps and the per-entry field caps,
// then marshals to the jsonb column shape.
func (s *Service) encodeListingSteps(ctx context.Context, steps []ServiceStepInput) ([]byte, error) {
	maxSteps := 8
	if s.settings != nil {
		maxSteps = s.settings.GetIntOr(ctx, "service_marketplace.content.max_steps", 8)
	}

	if len(steps) > maxSteps {
		return nil, ErrTooManySteps
	}

	out := make([]ServiceStepDTO, 0, len(steps))

	for i, st := range steps {
		title := strings.TrimSpace(st.Title)
		if title == "" || len(title) > maxFaqStepTitleLen {
			return nil, fmt.Errorf("steps[%d]: title must be 1-%d characters: %w", i, maxFaqStepTitleLen, ErrInvalidStep)
		}

		if len(st.Description) > maxFaqStepBodyLen {
			return nil, fmt.Errorf("steps[%d]: description must be %d characters or fewer: %w", i, maxFaqStepBodyLen, ErrInvalidStep)
		}

		out = append(out, ServiceStepDTO{Title: title, Description: st.Description})
	}

	b, err := json.Marshal(out)
	if err != nil {
		return nil, fmt.Errorf("servicedir: marshal steps: %w", err)
	}

	return b, nil
}

// decodeListingFaqs / decodeListingSteps read the jsonb columns back into
// DTO slices — malformed or empty payloads produce nil (omitted from the
// response) rather than failing the read.
func decodeListingFaqs(raw []byte) []ServiceFaqDTO {
	if len(raw) == 0 {
		return nil
	}

	var out []ServiceFaqDTO
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil
	}

	return out
}

func decodeListingSteps(raw []byte) []ServiceStepDTO {
	if len(raw) == 0 {
		return nil
	}

	var out []ServiceStepDTO
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil
	}

	return out
}

// validatePackageDepartures validates the §111 departures[] payload.
func validatePackageDepartures(deps []PackageDepartureInput) ([]gen.CreateServicePackageDepartureParams, error) {
	if len(deps) == 0 {
		return nil, nil
	}

	params := make([]gen.CreateServicePackageDepartureParams, 0, len(deps))

	// Every rejection wraps ErrInvalidDeparture so the error envelope maps
	// to 400 invalid_departure — a plain fmt.Errorf would surface as a 500.
	for i, d := range deps {
		if d.StartsOn == "" {
			return nil, fmt.Errorf("departures[%d]: starts_on is required: %w", i, ErrInvalidDeparture)
		}

		startsOn, err := parseOptionalDate(d.StartsOn)
		if err != nil || !startsOn.Valid {
			return nil, fmt.Errorf("departures[%d]: invalid starts_on (%v): %w", i, err, ErrInvalidDeparture)
		}

		endsOn, err := parseOptionalDate(d.EndsOn)
		if err != nil {
			return nil, fmt.Errorf("departures[%d]: invalid ends_on (%v): %w", i, err, ErrInvalidDeparture)
		}

		if endsOn.Valid && endsOn.Time.Before(startsOn.Time) {
			return nil, fmt.Errorf("departures[%d]: ends_on precedes starts_on: %w", i, ErrInvalidDeparture)
		}

		// status defaults to open; 'full' is accepted so a loaded row
		// round-trips unchanged (the editor displays it but doesn't offer
		// it — capacity hitting 0 flips it automatically).
		status := DepartureStatusOpen
		if d.Status == DepartureStatusCancelled || d.Status == DepartureStatusFull {
			status = d.Status
		} else if d.Status != "" && d.Status != DepartureStatusOpen {
			return nil, fmt.Errorf("departures[%d]: invalid status %q: %w", i, d.Status, ErrInvalidDeparture)
		}

		if d.Capacity != nil && *d.Capacity <= 0 {
			return nil, fmt.Errorf("departures[%d]: capacity must be positive: %w", i, ErrInvalidDeparture)
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

	// Attached coupons ride the public detail payload when the coupons
	// flag is on (active-window rows only — the same set a redemption
	// could consume). Best-effort like the reviews sidecar: a failure
	// degrades to no coupons rather than failing the detail read.
	if s.CouponsEnabled(ctx) {
		if coupons, err := s.ListingCouponsDTO(ctx, row.MetaID, true); err != nil {
			slog.Warn("servicedir: attach coupons failed", "content_id", row.ID, "error", err)
		} else {
			dto.Coupons = coupons
		}
	}

	// §114: stamp the derived ribbon on the detail payload too — the
	// detail row carries no has_coupon column, so the attached (flag-gated)
	// coupons set stands in for it.
	dto.Badge = s.deriveBadge(ctx, dto.ReviewCount, dto.RatingAvg, dto.HasCoupon || len(dto.Coupons) > 0)

	return dto, nil
}

// GetListingByContentID fetches the raw listing row by public content id —
// handlers that must branch on listing_kind before the feature gate
// (booking create: mentorship vs package) read it here.
func (s *Service) GetListingByContentID(ctx context.Context, contentID pgtype.UUID) (ServiceRow, error) {
	return s.store.GetByContentID(ctx, contentID)
}

// EnsurePublishedListing applies the same public-visibility gate GetBySlug
// enforces — published status, non-deleted, and the packages flag for
// listing_kind='package' — for the id-keyed public subroutes
// (GET /services/:id/reviews). Returns pgx.ErrNoRows when the listing is
// not publicly visible.
func (s *Service) EnsurePublishedListing(ctx context.Context, contentID pgtype.UUID) error {
	row, err := s.store.GetByContentID(ctx, contentID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return pgx.ErrNoRows
		}

		return fmt.Errorf("servicedir: get by content id: %w", err)
	}

	if row.Status != content.StatusPublished {
		return pgx.ErrNoRows
	}

	if row.ListingKind == ListingKindPackage && !s.PackagesEnabled(ctx) {
		return pgx.ErrNoRows
	}

	return nil
}

// GetBySlugEditor returns a listing by slug without the published-status
// gate — the /services/:slug?raw=1 editor view. Same permission shape as
// EditOwn / ListPackageDeparturesForEdit: author, owning-org member, or
// service.edit_any. Non-published listings have no public URL, so this is
// the only way an owner can reload an in_review/draft listing.
func (s *Service) GetBySlugEditor(ctx context.Context, slug string, userID pgtype.UUID, isEditAny bool) (ServiceDTO, error) {
	row, err := s.store.GetBySlug(ctx, slug)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ServiceDTO{}, pgx.ErrNoRows
		}

		return ServiceDTO{}, fmt.Errorf("servicedir: get by slug: %w", err)
	}

	meta, err := s.store.GetMeta(ctx, row.ID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ServiceDTO{}, pgx.ErrNoRows
		}

		return ServiceDTO{}, fmt.Errorf("servicedir: raw get: get meta: %w", err)
	}

	if !isEditAny && row.AuthorID != userID {
		if !meta.OrgID.Valid {
			return ServiceDTO{}, ErrNotOwner
		}

		isMember, err := s.store.IsOrgMember(ctx, meta.OrgID, userID)
		if err != nil {
			return ServiceDTO{}, fmt.Errorf("servicedir: raw get: check org membership: %w", err)
		}

		if !isMember {
			return ServiceDTO{}, ErrNotOwner
		}
	}

	// Package listings stay gated on the feature flag even in the editor —
	// a disabled feature leaves no surface, public or owner-scoped.
	if row.ListingKind == ListingKindPackage && !s.PackagesEnabled(ctx) {
		return ServiceDTO{}, ErrPackagesDisabled
	}

	dto := rowToDTO(row)

	if row.ListingKind == ListingKindPackage {
		// Uncached — editor reads shouldn't hit the public detail cache.
		if err := s.attachPackageData(ctx, row.MetaID, &dto); err != nil {
			slog.Warn("servicedir: attach package data failed", "content_id", row.ID, "error", err)
		}
	}

	// §114: same coupon + badge surface as the public detail so the editor
	// sees the ribbon its listing earned. Best-effort like GetBySlug.
	if s.CouponsEnabled(ctx) {
		if coupons, err := s.ListingCouponsDTO(ctx, row.MetaID, true); err == nil {
			dto.Coupons = coupons
		}
	}
	dto.Badge = s.deriveBadge(ctx, dto.ReviewCount, dto.RatingAvg, dto.HasCoupon || len(dto.Coupons) > 0)

	return dto, nil
}

// packageDetailCache is the cached slice of a package detail response —
// items, open departures, active pricing tiers, and the quoted platform
// fee (jc:svc_package:*, 15min per conventions/valkey-keys-catalog.md).
type packageDetailCache struct {
	Items           []PackageItemDTO      `json:"items"`
	Departures      []PackageDepartureDTO `json:"departures"`
	Tiers           []PackageTierDTO      `json:"tiers,omitempty"`
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

				// Tiers ride the cache but stay suppressed while the flag
				// is off (a flag flip can lag behind cached content).
				if s.TiersEnabled(ctx) {
					dto.Tiers = cached.Tiers
				}

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
		Tiers:           dto.Tiers,
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
		dto.Departures = append(dto.Departures, departureToDTO(d))
	}

	// Active pricing tiers ride the package detail (marketplace spec §2.4)
	// — gated on service_packages.tiers_enabled; flag off → field omitted.
	if s.TiersEnabled(ctx) {
		tiers, err := s.listPackageTiers(ctx, metaID)
		if err != nil {
			return err
		}

		dto.Tiers = tiers
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

// departureToDTO maps one departure row to the API shape — used by both
// the public detail (open rows only) and the editor list (all rows).
func departureToDTO(d gen.ServicePackageDeparture) PackageDepartureDTO {
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

	return dep
}

// ListPackageDeparturesForEdit returns ALL departures of a package —
// including past/cancelled/full rows the public detail hides — for the
// §111 editor. Same permission shape as EditOwn: author, org member, or
// service.edit_any.
func (s *Service) ListPackageDeparturesForEdit(ctx context.Context, contentID, userID pgtype.UUID, isEditAny bool) ([]PackageDepartureDTO, error) {
	if !s.PackagesEnabled(ctx) {
		return nil, ErrPackagesDisabled
	}

	existing, err := s.content.GetContent(ctx, contentID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, pgx.ErrNoRows
		}

		return nil, fmt.Errorf("servicedir: departures: get content: %w", err)
	}

	meta, err := s.store.GetMeta(ctx, contentID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, pgx.ErrNoRows
		}

		return nil, fmt.Errorf("servicedir: departures: get meta: %w", err)
	}

	if meta.ListingKind != ListingKindPackage {
		return nil, ErrNotPackage
	}

	if !isEditAny && existing.AuthorID != userID {
		if !meta.OrgID.Valid {
			return nil, ErrNotOwner
		}

		isMember, err := s.store.IsOrgMember(ctx, meta.OrgID, userID)
		if err != nil {
			return nil, fmt.Errorf("servicedir: departures: check org membership: %w", err)
		}

		if !isMember {
			return nil, ErrNotOwner
		}
	}

	rows, err := s.store.ListPackageDepartures(ctx, meta.ID)
	if err != nil {
		return nil, err
	}

	out := make([]PackageDepartureDTO, 0, len(rows))
	for _, d := range rows {
		out = append(out, departureToDTO(d))
	}

	return out, nil
}

// List returns a paginated, filtered list of published service listings.
// destSpaceID, citySpaceID, serviceCategory, originCommunity are optional
// geo-aware filters (§98: ?dest=&city=&category=&origin=&verified=&sort=).
func (s *Service) List(ctx context.Context, destSpaceID, citySpaceID pgtype.UUID, serviceCategory, originCommunity string, isVerified *bool, listingKind string, priceMax *int64, dateFrom pgtype.Date, q, sort string, page, limit int32) ([]ServiceDTO, int64, error) {
	if limit <= 0 {
		limit = 20
	}

	if page <= 0 {
		page = 1
	}

	offset := (page - 1) * limit

	// §111/000824 kind gates: an explicit kind=package needs
	// service_packages.enabled and kind=mentorship needs
	// mentorship.enabled — a request for a dark-launched kind is a 404.
	packagesOn := s.PackagesEnabled(ctx)
	mentorshipOn := s.MentorshipEnabled(ctx)

	if listingKind == ListingKindPackage && !packagesOn {
		return nil, 0, ErrPackagesDisabled
	}

	if listingKind == ListingKindMentorship && !mentorshipOn {
		return nil, 0, ErrMentorshipDisabled
	}

	// The unfiltered pin keeps flag-gated kinds out of the directory: with
	// both flags off the list collapses to kind=service (the pre-000824
	// behaviour); with one flag off the unfiltered query excludes that
	// kind and still surfaces the rest together.
	var excludeKind string
	if listingKind == "" {
		switch {
		case !packagesOn && !mentorshipOn:
			listingKind = ListingKindService
		case !packagesOn:
			excludeKind = ListingKindPackage
		case !mentorshipOn:
			excludeKind = ListingKindMentorship
		}
	}

	rows, total, err := s.store.ListServices(ctx, ListServicesFilter{

		DestSpaceID:     destSpaceID,
		CitySpaceID:     citySpaceID,
		ServiceCategory: serviceCategory,
		OriginCommunity: originCommunity,
		IsVerified:      isVerified,
		ListingKind:     listingKind,
		ExcludeKind:     excludeKind,
		PriceMax:        priceMax,
		DateFrom:        dateFrom,
		Q:               q,
		Sort:            sort,
		Status:          content.StatusPublished,
		Limit:           limit,
		Offset:          offset,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("servicedir: list: %w", err)
	}

	couponsOn := s.CouponsEnabled(ctx)
	tiersOn := s.TiersEnabled(ctx)

	dtos := make([]ServiceDTO, 0, len(rows))
	for _, r := range rows {
		dto := rowToDTO(r)
		if couponsOn {
			dto.HasCoupon = r.HasCoupon
		}
		// min_price COALESCEs the lowest active tier over price_amount —
		// with the tiers flag off a stale tier row would leak a hidden
		// price surface, so fall back to the meta price instead.
		if tiersOn {
			dto.MinPrice = r.MinPrice
		} else {
			dto.MinPrice = r.PriceAmount
		}

		// §114 derived ribbon — has_coupon is already flag-gated above.
		dto.Badge = s.deriveBadge(ctx, dto.ReviewCount, dto.RatingAvg, dto.HasCoupon)

		dtos = append(dtos, dto)
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
	// Ownership/shape check shared with the other listing-scoped writes
	// (loadListingForEdit): author, owning-org member, or service.edit_any
	// (BUG-008, BUG-009).
	existing, meta, err := s.loadListingForEdit(ctx, contentID, userID, isEditAny)
	if err != nil {
		return ServiceDTO{}, err
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
		Faqs:               meta.Faqs,
		Steps:              meta.Steps,
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

	if input.CitySpaceID != nil {
		// Present-but-empty clears the city (the column is nullable); only a
		// non-empty value is validated as a UUID.
		if *input.CitySpaceID == "" {
			params.CitySpaceID = pgtype.UUID{}
		} else if err := params.CitySpaceID.Scan(*input.CitySpaceID); err != nil {
			return ServiceDTO{}, fmt.Errorf("servicedir: edit: invalid city_space_id: %w", err)
		}
	}

	if input.ServiceCategory != nil {
		// BUG-013: validate category on edit when supplied — a nil pointer
		// keeps the stored value; a present-but-empty string is rejected
		// (no matching slug) because the column is NOT NULL.
		if _, ok := s.categorySet(ctx)[*input.ServiceCategory]; !ok {
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
		params.WebsiteUrl = nilIfEmpty(input.WebsiteURL)
	}

	if input.BookingURL != nil {
		params.BookingUrl = nilIfEmpty(input.BookingURL)
	}

	if input.PriceRange != nil {
		params.PriceRange = nilIfEmpty(input.PriceRange)
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

		// §111 clear convention: price_amount/capacity take 0 to clear (both
		// are positive-only fields, so 0 is unambiguous); '' clears
		// price_currency via nilIfEmpty and the date fields via
		// parseOptionalDate. Absent fields keep the stored value (JSON null
		// is indistinguishable from absent on pointer fields).
		if input.PriceAmount != nil {
			if *input.PriceAmount < 0 {
				return ServiceDTO{}, fmt.Errorf("servicedir: edit: price_amount must be positive")
			}

			if *input.PriceAmount == 0 {
				params.PriceAmount = nil
			} else {
				params.PriceAmount = input.PriceAmount
			}
		}

		if input.PriceCurrency != nil {
			params.PriceCurrency = nilIfEmpty(input.PriceCurrency)
		}

		if input.PricePer != nil {
			if !IsValidPricePer(*input.PricePer) {
				return ServiceDTO{}, ErrInvalidPricePer
			}

			params.PricePer = input.PricePer
		}

		if input.Capacity != nil {
			if *input.Capacity < 0 {
				return ServiceDTO{}, fmt.Errorf("servicedir: edit: capacity must be positive")
			}

			if *input.Capacity == 0 {
				params.Capacity = nil
			} else {
				params.Capacity = input.Capacity
			}
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

	// §114 faqs[]/steps[] — replace-on-save like items[]: nil keeps the
	// stored set (carried into params above), [] clears it. Not
	// package-gated — plain service listings carry these too.
	if input.Faqs != nil {
		params.Faqs, err = s.encodeListingFaqs(ctx, *input.Faqs)
		if err != nil {
			return ServiceDTO{}, err
		}
	}

	if input.Steps != nil {
		params.Steps, err = s.encodeListingSteps(ctx, *input.Steps)
		if err != nil {
			return ServiceDTO{}, err
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

	// tiers[] mirrors items[] replace-on-save semantics, gated on
	// service_packages.tiers_enabled — flag off → field silently ignored.
	var tierParams []gen.CreateServicePackageTierParams
	if isPackage && input.Tiers != nil && s.TiersEnabled(ctx) {
		tierParams, err = s.validatePackageTiers(ctx, *input.Tiers)
		if err != nil {
			return ServiceDTO{}, err
		}
	}

	var depInserts []gen.CreateServicePackageDepartureParams
	var depUpdates []gen.UpdateServicePackageDepartureParams
	var keepIDs []pgtype.UUID
	if isPackage && input.Departures != nil {
		depParams, err := validatePackageDepartures(*input.Departures)
		if err != nil {
			return ServiceDTO{}, err
		}

		// §111 id-aware replace: departures carrying an existing row id
		// update in place — delete+reinsert would break the booking
		// departure_id association (and re-sending a booked row would
		// duplicate it). Ids must belong to this package, and capacity
		// can't shrink below seats already booked.
		var existingDeps []gen.ServicePackageDeparture
		hasIDs := false
		for i := range *input.Departures {
			if (*input.Departures)[i].ID != "" {
				hasIDs = true
				break
			}
		}
		if hasIDs {
			existingDeps, err = s.store.ListPackageDepartures(ctx, meta.ID)
			if err != nil {
				return ServiceDTO{}, fmt.Errorf("servicedir: edit: list package departures: %w", err)
			}
		}

		byID := make(map[pgtype.UUID]gen.ServicePackageDeparture, len(existingDeps))
		for _, e := range existingDeps {
			byID[e.ID] = e
		}
		seenUpdate := make(map[pgtype.UUID]bool)

		for i, d := range *input.Departures {
			if d.ID == "" {
				depInserts = append(depInserts, depParams[i])
				continue
			}

			var depID pgtype.UUID
			if err := depID.Scan(d.ID); err != nil {
				return ServiceDTO{}, ErrInvalidDeparture
			}

			ex, ok := byID[depID]
			if !ok || seenUpdate[depID] {
				return ServiceDTO{}, ErrInvalidDeparture
			}
			seenUpdate[depID] = true

			if depParams[i].Capacity != nil && *depParams[i].Capacity < ex.BookedCount {
				return ServiceDTO{}, fmt.Errorf("departures[%d]: capacity below booked seats (%d): %w", i, ex.BookedCount, ErrInvalidDeparture)
			}

			depUpdates = append(depUpdates, gen.UpdateServicePackageDepartureParams{
				ID:            depID,
				PackageMetaID: meta.ID,
				StartsOn:      depParams[i].StartsOn,
				EndsOn:        depParams[i].EndsOn,
				Capacity:      depParams[i].Capacity,
				Status:        depParams[i].Status,
			})
			keepIDs = append(keepIDs, depID)
		}

		// §111: departures that already have bookings are preserved —
		// the keep set also covers rows absent from the payload.
		bookedIDs, err := s.store.DeparturesWithBookings(ctx, meta.ID)
		if err != nil {
			return ServiceDTO{}, fmt.Errorf("servicedir: edit: list booked departures: %w", err)
		}
		keepIDs = append(keepIDs, bookedIDs...)
	}

	// All input validated — start writing. Title/body/featured image go
	// through the §14 content service. UpdateContent preserves fields the
	// input leaves unset; featured image clears go through ClearFeaturedImageID
	// because an invalid UUID now means "keep the stored image".
	updateInput := content.UpdateContentInput{
		Title:           existing.Title,
		Body:            existing.Body,
		FeaturedImageID: existing.FeaturedImageID,
	}
	if input.Title != nil {
		updateInput.Title = *input.Title
	}

	if input.Body != nil {
		updateInput.Body = *input.Body
	}

	// featured_media_id patch: nil keeps the stored row, "" clears the
	// image, a UUID string replaces it. Media existence/ownership is not
	// validated (no FK on contents.featured_image_id).
	if input.FeaturedImageID != nil {
		if *input.FeaturedImageID == "" {
			updateInput.FeaturedImageID = pgtype.UUID{}
			updateInput.ClearFeaturedImageID = true
		} else if err := updateInput.FeaturedImageID.Scan(*input.FeaturedImageID); err != nil {
			return ServiceDTO{}, ErrInvalidFeaturedMedia
		}
	}

	// §111: commercial edits (price_amount, items, purchase_mode, tiers) on
	// a published package re-enter the review queue — same gate as create.
	// A tiers[] change counts only when the tiers flag is on; with it off
	// the field is ignored and must not bounce the listing into review.
	if isPackage && existing.Status == content.StatusPublished &&
		s.settings.GetBoolOr(ctx, "service_packages.review_before_publish", true) &&
		(input.PriceAmount != nil || input.PurchaseMode != nil || input.Items != nil ||
			(input.Tiers != nil && s.TiersEnabled(ctx))) {
		updateInput.Status = content.StatusInReview
	}

	// Skip the contents write when nothing it carries changed — a
	// departures-only or meta-only save must not hit the §14 edit-window
	// guard, or package editors would be locked out of schedule updates
	// once the window expires. The edit form always echoes title/body, so
	// compare values, not field presence.
	contentChanged := updateInput.Status != "" ||
		updateInput.Title != existing.Title ||
		updateInput.Body != existing.Body ||
		updateInput.FeaturedImageID != existing.FeaturedImageID
	if contentChanged {
		// The §49 edit window guards post-publication bait-and-switch —
		// non-published listings (in_review/draft) aren't public yet, so
		// the author must stay able to fix them during review. Bypass the
		// window for anything that isn't published.
		bypassEditWindow := existing.Status != content.StatusPublished
		if _, err := s.content.UpdateContent(ctx, contentID, userID, updateInput, bypassEditWindow); err != nil {
			return ServiceDTO{}, fmt.Errorf("servicedir: edit: update content: %w", err)
		}
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
		for _, u := range depUpdates {
			if _, err := s.store.UpdatePackageDeparture(ctx, u); err != nil {
				return ServiceDTO{}, fmt.Errorf("servicedir: edit: update package departure: %w", err)
			}
		}

		if err := s.store.ReplacePackageDepartures(ctx, meta.ID, keepIDs, depInserts); err != nil {
			return ServiceDTO{}, fmt.Errorf("servicedir: edit: replace package departures: %w", err)
		}
	}

	if isPackage && input.Tiers != nil && s.TiersEnabled(ctx) {
		if err := s.store.ReplacePackageTiers(ctx, meta.ID, tierParams); err != nil {
			return ServiceDTO{}, fmt.Errorf("servicedir: edit: replace package tiers: %w", err)
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
		dto := rowToDTO(r)
		// AdminListServiceListings does not project the tier COALESCE —
		// the admin surface emits the meta price unconditionally (no
		// flag gating on the moderation queue).
		dto.MinPrice = r.PriceAmount

		dtos = append(dtos, dto)
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

	// Featured image + review aggregate (media / review_aggregates JOINs).
	// Pattern: internal/story/service.go (media.MediaURL over storage refs);
	// Numeric→float64 coercion mirrors the server review adapter.
	if r.FeaturedImageID.Valid {
		dto.FeaturedMediaID = r.FeaturedImageID.String()
	}

	if r.FeaturedStorageBucket != nil && r.FeaturedStoragePath != nil {
		dto.FeaturedImageURL = media.MediaURL(*r.FeaturedStorageBucket, *r.FeaturedStoragePath, media.ImageProxyOpts{})
	}

	if r.RatingAvg.Valid {
		if avg, err := r.RatingAvg.Float64Value(); err == nil && avg.Valid {
			dto.RatingAvg = avg.Float64
		}
	}

	if r.RatingCount != nil {
		dto.ReviewCount = *r.RatingCount
	}

	// §111 package fields.
	dto.ListingKind = r.ListingKind
	dto.PriceAmount = r.PriceAmount
	dto.PurchaseMode = r.PurchaseMode
	dto.Capacity = r.Capacity
	// MinPrice is stamped by the list call sites (gated on
	// service_packages.tiers_enabled) — rowToDTO has no flag context.
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

	// §114 structured content (jsonb arrays on service_listing_meta).
	dto.Faqs = decodeListingFaqs(r.Faqs)
	dto.Steps = decodeListingSteps(r.Steps)

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

	// contents.featured_image_id → featured_media_id so create/edit write
	// responses round-trip the picked media row (the GetContentByID query
	// has no media JOIN, so featured_image_url stays empty here).
	if c.FeaturedImageID.Valid {
		dto.FeaturedMediaID = c.FeaturedImageID.String()
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

	// §114 structured content.
	dto.Faqs = decodeListingFaqs(m.Faqs)
	dto.Steps = decodeListingSteps(m.Steps)

	return dto
}

// strOrEmpty dereferences an optional text column to "" when NULL.
func strOrEmpty(s *string) string {
	if s == nil {
		return ""
	}

	return *s
}

// nilIfEmpty collapses a present-but-empty optional text field to NULL so
// PATCH callers can clear website_url / booking_url / price_range instead
// of persisting ”.
func nilIfEmpty(s *string) *string {
	if s == nil || *s == "" {
		return nil
	}

	return s
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
