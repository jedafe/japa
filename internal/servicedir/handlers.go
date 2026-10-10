// Package servicedir — handlers.go provides the HTTP handlers for the
// Service Directory routes (§98, ADR-106).
//
// Routes:
//
//	GET   /api/v1/services                     — list ?dest=&city=&category=&origin=&verified=&sort=top|latest
//	GET   /api/v1/services/:slug               — single service listing
//	POST  /api/v1/services                     — create service listing (auth, service.create)
//	PATCH /api/v1/services/:id                 — edit own service listing (auth, service.edit_own)
//	POST  /api/v1/services/:id/claim           — initiate org claim (auth, service.claim)
//	GET   /api/v1/admin/services               — moderation queue (auth, service.verify)
//	PATCH /api/v1/admin/services/:id/verify    — mark verified (auth, service.verify)
//	PATCH /api/v1/admin/services/:id/claim/:claim_id — resolve claim (auth, service.verify)
//	GET   /api/v1/admin/service-categories         — taxonomy list (auth, service.categories.manage)
//	POST  /api/v1/admin/service-categories         — create category (auth, service.categories.manage)
//	PATCH /api/v1/admin/service-categories/:slug   — update category (auth, service.categories.manage)
//	DELETE /api/v1/admin/service-categories/:slug  — delete/disable category (auth, service.categories.manage)
//
// Ref: japaconnect-engineering/sections/98-service-directory.md
//
//	japaconnect-engineering/adr/ADR-106-service-directory.md
package servicedir

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/config"
	"github.com/jedafe/japa/internal/content"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/literals"
)

// literalResolver resolves {{...}} template literals in user-facing text
// fields (§14). Implemented by *literals.Catalog; nil disables resolution.
type literalResolver interface {
	ResolveFields(ctx context.Context, v any, rc literals.RenderContext)
}

// Handlers exposes the service directory endpoints.
type Handlers struct {
	service  *Service
	settings *config.Settings
	rbac     RBACResolver
	reviews  ReviewFetcher
	lit      literalResolver // nil disables literal resolution (§14)
}

// RBACResolver is the subset of the RBAC resolver needed to check
// permissions in-handler (e.g. service.edit_any for the edit bypass).
type RBACResolver interface {
	HasPermission(ctx context.Context, userID pgtype.UUID, permKey string) bool
}

// ReviewItem is a single review returned by ReviewFetcher.ListByEntity.
// Mirrors the relevant fields from §29 reviews joined with author info.
type ReviewItem struct {
	ID                string  `json:"id"`
	AuthorID          string  `json:"author_id"`
	AuthorUsername    string  `json:"author_username"`
	AuthorDisplayName *string `json:"author_display_name,omitempty"`
	AuthorAvatarURL   *string `json:"author_avatar_url,omitempty"`
	Rating            int16   `json:"rating"`
	Title             *string `json:"title,omitempty"`
	Body              *string `json:"body,omitempty"`
	CreatedAt         string  `json:"created_at"`
}

// ReviewAggregate holds the count + average rating for an entity (§29).
type ReviewAggregate struct {
	Count int32   `json:"count"`
	Avg   float64 `json:"avg"`
}

// ReviewFetcher is the subset of the §29 reviews.Service needed by the
// service directory to embed reviews + aggregate in GET /services/:slug
// (BUG-014: §98 spec says "service detail + reviews").
type ReviewFetcher interface {
	ListByEntity(ctx context.Context, entityType string, entityID pgtype.UUID, limit, offset int32, sort string) ([]ReviewItem, error)
	Aggregate(ctx context.Context, entityType string, entityID pgtype.UUID) (ReviewAggregate, error)
}

// NewHandlers creates service directory handlers bound to the provided
// service. The rbac resolver is used for in-handler permission checks
// (service.edit_any bypass). The reviews fetcher is optional — when nil,
// GET /services/:slug omits the reviews/aggregate sidecar fields.
func NewHandlers(service *Service, settings *config.Settings, rbac RBACResolver, reviews ReviewFetcher) *Handlers {
	return &Handlers{service: service, settings: settings, rbac: rbac, reviews: reviews}
}

// SetLiteralResolver injects the template-literal catalog so listing
// titles/bodies render {{...}} placeholders (§14).
func (h *Handlers) SetLiteralResolver(l literalResolver) { h.lit = l }

// resolveLiterals resolves {{...}} placeholders on a DTO or slice.
// Best-effort — failures are logged inside the resolver.
func (h *Handlers) resolveLiterals(r *http.Request, v any) {
	if literals.IsRawRequest(r) {
		return
	}

	if h.lit == nil {
		return
	}

	h.lit.ResolveFields(r.Context(), v, literals.NewRequestContext(r, ""))
}

// ---------------------------------------------------------------------------
// Shared helpers (same pattern as story/handlers.go)
// ---------------------------------------------------------------------------

func requireUser(w http.ResponseWriter, r *http.Request) (*auth.AuthenticatedUser, bool) {
	u := auth.FromContext(r.Context())
	if u == nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return nil, false
	}

	return u, true
}

func scanUUIDParam(w http.ResponseWriter, r *http.Request, name string) (pgtype.UUID, bool) {
	var id pgtype.UUID
	if err := id.Scan(chi.URLParam(r, name)); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid "+name)
		return pgtype.UUID{}, false
	}

	return id, true
}

func parsePageParams(r *http.Request) (page, limit int32) {
	page = 1
	limit = 20

	if v := r.URL.Query().Get("page"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			page = int32(n) // #nosec
		}
	}

	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 100 {
			limit = int32(n) // #nosec
		}
	}

	return page, limit
}

// writeServiceError maps §111 service-layer sentinels to the standard
// snake_case API error contract (conventions/api-response-format.md).
// Falls through to the provided 500 code/message for unexpected errors.
func writeServiceError(w http.ResponseWriter, err error, code, msg string) {
	switch {
	case errors.Is(err, ErrPackagesDisabled):
		// Disabled feature → 404 per convention.
		errpkg.WriteError(w, http.StatusNotFound, "service_packages_disabled", "Service packages are not available")
	case errors.Is(err, ErrMentorshipDisabled):
		errpkg.WriteError(w, http.StatusNotFound, "mentorship_disabled", "Mentorship is not available")
	case errors.Is(err, ErrSelfMentorship):
		errpkg.WriteError(w, http.StatusBadRequest, "self_mentorship", "You cannot request mentorship from yourself")
	case errors.Is(err, ErrMentorshipListingManaged):
		errpkg.WriteError(w, http.StatusBadRequest, "mentorship_listing_managed", "Mentorship listings are managed through the mentor profile")
	case errors.Is(err, ErrDuplicateRequest):
		errpkg.WriteError(w, http.StatusConflict, "duplicate_mentorship_request", "You already have an open mentorship request with this provider")
	case errors.Is(err, ErrMentorAtCapacity):
		errpkg.WriteError(w, http.StatusConflict, "mentor_at_capacity", "This mentor is not accepting new mentees right now")
	case errors.Is(err, ErrCouponsDisabled):
		errpkg.WriteError(w, http.StatusNotFound, "service_coupons_disabled", "Service coupons are not available")
	case errors.Is(err, ErrInvalidCoupon):
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_coupon", "The coupon is invalid or no longer redeemable")
	case errors.Is(err, ErrCouponNotApplicable):
		errpkg.WriteError(w, http.StatusBadRequest, "coupon_not_applicable", "The coupon does not apply to this listing")
	case errors.Is(err, ErrCouponMinSpend):
		errpkg.WriteError(w, http.StatusBadRequest, "coupon_min_spend", "The booking total is below the coupon's minimum spend")
	case errors.Is(err, ErrCouponCapReached):
		errpkg.WriteError(w, http.StatusBadRequest, "coupon_cap_reached", "The coupon redemption limit has been reached")
	case errors.Is(err, ErrCouponUserLimit):
		errpkg.WriteError(w, http.StatusBadRequest, "coupon_user_limit", "You have already used this coupon the maximum number of times")
	case errors.Is(err, ErrCouponFirstTime):
		errpkg.WriteError(w, http.StatusBadRequest, "coupon_first_time_only", "This coupon is only available to first-time customers")
	case errors.Is(err, ErrInvalidTier):
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_tier", "The selected pricing tier is not available")
	case errors.Is(err, ErrInvalidTiers):
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_tiers", "The pricing tier list is invalid")
	case errors.Is(err, ErrTooManyCoupons):
		errpkg.WriteError(w, http.StatusBadRequest, "too_many_coupons", "Too many coupons attached to this listing")
	case errors.Is(err, ErrOrgNotVerified):
		errpkg.WriteError(w, http.StatusForbidden, "organization_not_verified", "Only verified organizations can publish packages")
	case errors.Is(err, ErrOrgRequired):
		errpkg.WriteError(w, http.StatusBadRequest, "org_required", "org_id is required for package listings")
	case errors.Is(err, ErrPurchaseModeUnavailable):
		errpkg.WriteError(w, http.StatusForbidden, "purchase_mode_unavailable", "This purchase mode is not available")
	case errors.Is(err, ErrInvalidListingKind):
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_listing_kind", "listing_kind must be service|package|mentorship and cannot change after creation")
	case errors.Is(err, ErrInvalidPurchaseMode):
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_purchase_mode", "purchase_mode must be inquiry|external|paid")
	case errors.Is(err, ErrInvalidPricePer):
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_price_per", "price_per must be person|package|group")
	case errors.Is(err, ErrMissingPackagePrice):
		errpkg.WriteError(w, http.StatusBadRequest, "missing_package_price", "Paid packages require a positive price_amount and price_currency")
	case errors.Is(err, ErrTooManyPackageItems):
		errpkg.WriteError(w, http.StatusBadRequest, "too_many_items", "Package exceeds the maximum number of items")
	case errors.Is(err, ErrItemCrossOrg):
		errpkg.WriteError(w, http.StatusBadRequest, "component_cross_org", "Package items can only link to the same organization's listings")
	case errors.Is(err, ErrItemIsPackage):
		errpkg.WriteError(w, http.StatusBadRequest, "component_is_package", "A package cannot nest another package as a component")
	case errors.Is(err, ErrNotOrgMember):
		errpkg.WriteError(w, http.StatusForbidden, "not_org_member", "You are not a member of this organization")
	case errors.Is(err, ErrNotOwner):
		errpkg.WriteError(w, http.StatusForbidden, "not_owner", "Insufficient permissions")
	case errors.Is(err, ErrDepartureFull):
		errpkg.WriteError(w, http.StatusConflict, "departure_full", "The selected departure is full or no longer available")
	case errors.Is(err, ErrInvalidBookingTransition):
		errpkg.WriteError(w, http.StatusConflict, "invalid_booking_transition", "The booking cannot transition to that status from its current state")
	case errors.Is(err, ErrBookingUnavailable):
		errpkg.WriteError(w, http.StatusConflict, "booking_unavailable", "This listing does not accept bookings")
	case errors.Is(err, ErrIdempotencyRequired):
		errpkg.WriteError(w, http.StatusBadRequest, "idempotency_key_required", "Idempotency-Key header is required")
	case errors.Is(err, ErrInvalidDeparture):
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_departure", "The selected departure is invalid for this package")
	case errors.Is(err, ErrNotPackage):
		errpkg.WriteError(w, http.StatusBadRequest, "not_package", "This listing is not a package")
	case errors.Is(err, ErrInvalidItem):
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_item", "A package item is invalid")
	case errors.Is(err, errpkg.ErrInsufficientBalance):
		// Credit-provider checkout with too small a wallet balance — the
		// payment service surfaces the wallet sentinel wrapped inside the
		// booking error chain.
		errpkg.WriteError(w, http.StatusUnprocessableEntity, "insufficient_balance", "Insufficient credits balance")
	case errors.Is(err, ErrInvalidIntent):
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_intent", "intent must be inquiry|booking and match the listing's purchase_mode")
	case errors.Is(err, ErrCheckoutFieldsRequired):
		errpkg.WriteError(w, http.StatusBadRequest, "checkout_fields_required", "provider and email are required for paid bookings")
	case errors.Is(err, ErrInvalidProvider):
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_provider", "provider must be one of: paystack, stripe, nowpayments, manual_transfer, credit")
	case errors.Is(err, ErrInvalidPreferredDates):
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_preferred_dates", "preferred_dates must be {start_on, end_on, flexible} with start_on <= end_on")
	case errors.Is(err, ErrInvalidInput):
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_input", "The request contains a field this booking type does not accept")
	case errors.Is(err, ErrPackageOrgImmutable):
		errpkg.WriteError(w, http.StatusBadRequest, "package_org_immutable", "A package's selling organization cannot change after creation")
	case errors.Is(err, ErrInvalidFeaturedMedia):
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_featured_media_id", "Invalid featured_media_id format")
	case errors.Is(err, ErrTooManyFaqs):
		errpkg.WriteError(w, http.StatusBadRequest, "too_many_faqs", "Listing exceeds the maximum number of FAQs")
	case errors.Is(err, ErrInvalidFaq):
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_faq", "An FAQ entry is invalid")
	case errors.Is(err, ErrTooManySteps):
		errpkg.WriteError(w, http.StatusBadRequest, "too_many_steps", "Listing exceeds the maximum number of steps")
	case errors.Is(err, ErrInvalidStep):
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_step", "A step entry is invalid")
	case errors.Is(err, ErrTooManySellingPoints):
		errpkg.WriteError(w, http.StatusBadRequest, "too_many_selling_points", "Listing exceeds the maximum number of selling points")
	case errors.Is(err, ErrInvalidSellingPoint):
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_selling_point", "A selling point entry is invalid")
	case errors.Is(err, ErrTooManyFacts):
		errpkg.WriteError(w, http.StatusBadRequest, "too_many_facts", "Listing exceeds the maximum number of facts")
	case errors.Is(err, ErrInvalidFact):
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_fact", "A fact entry is invalid")
	case errors.Is(err, ErrTooManyIncluded):
		errpkg.WriteError(w, http.StatusBadRequest, "too_many_included", "Listing exceeds the maximum number of included entries")
	case errors.Is(err, ErrInvalidIncluded):
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_included", "An included entry is invalid")
	case errors.Is(err, ErrTooManyHighlights):
		errpkg.WriteError(w, http.StatusBadRequest, "too_many_highlights", "Listing exceeds the maximum number of highlights")
	case errors.Is(err, ErrInvalidHighlight):
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_highlight", "A highlight entry is invalid")
	case errors.Is(err, ErrTooManyAudience):
		errpkg.WriteError(w, http.StatusBadRequest, "too_many_audience", "Listing exceeds the maximum number of audience entries")
	case errors.Is(err, ErrInvalidAudience):
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_audience", "An audience entry is invalid")
	case errors.Is(err, ErrInvalidID):
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid id")
	case errors.Is(err, ErrInputTooLong):
		errpkg.WriteError(w, http.StatusBadRequest, "input_too_long", "A free-text field exceeds the 4000-character limit")
	case errors.Is(err, content.ErrEditWindowExpired):
		// Author edits to a *published* listing after the shared §49 content
		// edit window. Non-published listings bypass this (see EditOwn).
		errpkg.WriteError(w, http.StatusForbidden, "edit_window_expired", "The edit window for this content has expired")
	case errors.Is(err, pgx.ErrNoRows):
		errpkg.WriteError(w, http.StatusNotFound, "not_found", "Not found")
	default:
		errpkg.WriteError(w, http.StatusInternalServerError, code, msg)
	}
}

// ---------------------------------------------------------------------------
// Public handlers (list + single — anonymous browse per §98)
// ---------------------------------------------------------------------------

// HandleCategories handles GET /services/categories — returns the canonical
// service category option list (Rule 16). The route sits under the
// composite service_directory.enabled OR mentorship.enabled gate (§111
// hardening): when the directory flag is off, only applies_to='mentorship'
// rows are returned so the rail never advertises unreachable kinds.
func (h *Handlers) HandleCategories(w http.ResponseWriter, r *http.Request) {

	directoryOn := h.service != nil && h.service.Enabled(r.Context())
	if !directoryOn && (h.service == nil || !h.service.MentorshipEnabled(r.Context())) {
		errpkg.WriteError(w, http.StatusNotFound, "service_directory_disabled", "Service directory is not available")
		return
	}

	cats := h.service.Categories(r.Context())

	if !directoryOn {
		// Mentorship-only deployment (000824): serve the mentor-surface
		// taxonomy only — service/package categories describe kinds the
		// deployment can't reach.
		scoped := make([]Option, 0, len(cats))
		for _, c := range cats {
			// 'both' means every kind per the applies_to convention
			// (pricing.go, coupons.go), so it stays visible here.
			if c.AppliesTo == "mentorship" || c.AppliesTo == "both" {
				scoped = append(scoped, c)
			}
		}
		cats = scoped
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": cats})
}

// HandleList handles GET /services — list published service listings with
// filters. Query params: ?dest=&city=&category=&origin=&verified=&sort=top|latest&q=&page=&limit=.
func (h *Handlers) HandleList(w http.ResponseWriter, r *http.Request) {

	// The bare list is directory-gated, but an explicit kind=mentorship
	// request gets the kind-appropriate gate: Task 12 redirects
	// /mentors → /services?kind=mentorship and that surface must stay live
	// in mentorship-only deployments (000824 — same OR-gate as HandleGet).
	// List() itself enforces mentorship.enabled on the kind filter below,
	// so passing the handler gate with the mentorship flag off still 404s.
	if h.service == nil || (!h.service.Enabled(r.Context()) && r.URL.Query().Get("kind") != "mentorship") {
		errpkg.WriteError(w, http.StatusNotFound, "service_directory_disabled", "Service directory is not available")
		return
	}

	var destSpaceID pgtype.UUID
	if v := r.URL.Query().Get("dest"); v != "" {
		if err := destSpaceID.Scan(v); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid destination space ID")
			return
		}
	}

	var citySpaceID pgtype.UUID
	if v := r.URL.Query().Get("city"); v != "" {
		if err := citySpaceID.Scan(v); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid city space ID")
			return
		}
	}

	serviceCategory := r.URL.Query().Get("category")
	originCommunity := r.URL.Query().Get("origin")
	sort := r.URL.Query().Get("sort")
	q := r.URL.Query().Get("q")

	var verifiedPtr *bool

	if v := r.URL.Query().Get("verified"); v != "" {
		b, err := strconv.ParseBool(v)
		if err == nil {
			verifiedPtr = &b
		}
	}

	// §111 filters — kind is gated by service_packages.enabled in the
	// service layer (flag off → 404). 'all' is the explicit no-filter
	// value per the §111 route contract.
	listingKind := r.URL.Query().Get("kind")
	if listingKind == "all" {
		listingKind = ""
	}

	var priceMax *int64
	if v := r.URL.Query().Get("price_max"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n >= 0 {
			priceMax = &n
		}
	}

	var dateFrom pgtype.Date
	if v := r.URL.Query().Get("date_from"); v != "" {
		if t, err := time.Parse("2006-01-02", v); err == nil {
			dateFrom = pgtype.Date{Time: t, Valid: true}
		}
	}

	page, limit := parsePageParams(r)

	services, total, err := h.service.List(r.Context(), destSpaceID, citySpaceID, serviceCategory, originCommunity, verifiedPtr, listingKind, priceMax, dateFrom, q, sort, page, limit)
	if err != nil {
		if errors.Is(err, ErrPackagesDisabled) {
			errpkg.WriteError(w, http.StatusNotFound, "service_packages_disabled", "Service packages are not available")
			return
		}
		if errors.Is(err, ErrMentorshipDisabled) {
			errpkg.WriteError(w, http.StatusNotFound, "mentorship_disabled", "Mentorship is not available")
			return
		}

		slog.Error("servicedir: list failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "list_failed", "Failed to list services")

		return
	}

	h.resolveLiterals(r, &services)

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": services,
		"meta": map[string]any{
			"page":  page,
			"limit": limit,
			"total": total,
		},
	})
}

// HandleGet handles GET /services/:slug — single published service listing.
func (h *Handlers) HandleGet(w http.ResponseWriter, r *http.Request) {

	// The cheap pre-check only rejects when EVERY relevant flag is off —
	// the kind-appropriate gate runs after the listing resolves below:
	// mentorship-kind listings (000824) are reachable while
	// mentorship.enabled OR service_directory.enabled is on so the
	// /messages/mentorship inbox's listing links stay live in
	// mentorship-only deployments. Same resolve-then-gate order as
	// HandleCreateBooking.
	if h.service == nil || (!h.service.Enabled(r.Context()) && !h.service.MentorshipEnabled(r.Context())) {
		errpkg.WriteError(w, http.StatusNotFound, "service_directory_disabled", "Service directory is not available")
		return
	}

	slug := chi.URLParam(r, "slug")
	if slug == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_slug", "Service slug is required")
		return
	}

	var service ServiceDTO

	// ?raw=1 — editor view: serves non-published listings to the author,
	// owning-org members, and service.edit_any holders. The route runs
	// under OptionalAuth so anonymous callers still get the public path.
	if r.URL.Query().Get("raw") == "1" {
		user := auth.FromContext(r.Context())
		if user == nil {
			errpkg.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
			return
		}

		isEditAny := h.rbac != nil && h.rbac.HasPermission(r.Context(), user.UserID, "service.edit_any")

		rawService, rawErr := h.service.GetBySlugEditor(r.Context(), slug, user.UserID, isEditAny)
		if rawErr != nil {
			if errors.Is(rawErr, pgx.ErrNoRows) || errors.Is(rawErr, ErrNotOwner) || errors.Is(rawErr, ErrPackagesDisabled) || errors.Is(rawErr, ErrMentorshipDisabled) {
				errpkg.WriteError(w, http.StatusNotFound, "service_not_found", "Service not found")
				return
			}

			slog.Error("servicedir: raw get by slug failed", "slug", slug, "error", rawErr)
			errpkg.WriteError(w, http.StatusInternalServerError, "get_failed", "Failed to get service")

			return
		}

		service = rawService
	} else {
		var err error
		service, err = h.service.GetBySlug(r.Context(), slug)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				errpkg.WriteError(w, http.StatusNotFound, "service_not_found", "Service not found")
				return
			}

			slog.Error("servicedir: get by slug failed", "slug", slug, "error", err)
			errpkg.WriteError(w, http.StatusInternalServerError, "get_failed", "Failed to get service")

			return
		}
	}

	// Kind-appropriate gate (000824): mentorship-kind listings pass when
	// either flag is on — the service layer already refused them when both
	// are off; every other kind still requires the directory flag.
	if service.ListingKind != ListingKindMentorship && !h.service.Enabled(r.Context()) {
		errpkg.WriteError(w, http.StatusNotFound, "service_directory_disabled", "Service directory is not available")
		return
	}

	// BUG-014: §98 spec says "service detail + reviews". Embed the
	// published reviews + aggregate rating from the §29 polymorphic
	// reviews table (entity_type='service_listing', entity_id=content_id).
	// Reviews are fetched best-effort: a failure logs but does not fail
	// the detail request — the listing still renders without reviews.
	h.resolveLiterals(r, &service)

	resp := map[string]any{"data": service}

	if h.reviews != nil {
		var contentID pgtype.UUID
		if err := contentID.Scan(service.ID); err == nil {
			reviews, revErr := h.reviews.ListByEntity(r.Context(), "service_listing", contentID, 10, 0, "latest")
			if revErr != nil {
				slog.Warn("servicedir: fetch reviews for service detail", "slug", slug, "error", revErr)
			} else {
				h.resolveLiterals(r, &reviews)
				resp["reviews"] = reviews
			}

			agg, aggErr := h.reviews.Aggregate(r.Context(), "service_listing", contentID)
			if aggErr != nil {
				slog.Warn("servicedir: fetch review aggregate for service detail", "slug", slug, "error", aggErr)
			} else {
				resp["review_aggregate"] = agg
			}
		}
	}

	errpkg.WriteJSON(w, http.StatusOK, resp)
}

// parseReviewPageParams parses ?page=&limit= for the reviews subroute —
// tighter clamp than the listing default (limit 10, max 50).
func parseReviewPageParams(r *http.Request) (page, limit int32) {
	page = 1
	limit = 10

	if v := r.URL.Query().Get("page"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			page = int32(n) // #nosec
		}
	}

	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			if n > 50 {
				n = 50
			}

			limit = int32(n) // #nosec
		}
	}

	return page, limit
}

// HandleListReviews handles GET /services/:id/reviews — paginated public
// reviews for a listing. {id} is the listing's contents id (the same param
// shape as price-preview); the entity is the §29 polymorphic reviews table
// with entity_type='service_listing'. Anonymous-readable like
// /spaces/:id/reviews; the published-listing gate mirrors GetBySlug so
// reviews never leak for in_review/draft rows. When the reviews fetcher
// is unwired the response carries meta only (no data key) — the same
// nil-fallback the detail payload uses.
//
// The route sits under the composite service_directory.enabled OR
// mentorship.enabled gate (§111 hardening): when the directory flag is
// off, only listing_kind='mentorship' listings resolve — the same
// kind-appropriate gate HandleGet runs.
func (h *Handlers) HandleListReviews(w http.ResponseWriter, r *http.Request) {
	if h.service == nil || (!h.service.Enabled(r.Context()) && !h.service.MentorshipEnabled(r.Context())) {
		errpkg.WriteError(w, http.StatusNotFound, "service_directory_disabled", "Service directory is not available")
		return
	}

	contentID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	// Kind-appropriate gate (mirrors HandleGet): with the directory flag
	// off, reviews resolve for mentorship-kind listings only — every
	// other kind still requires service_directory.enabled.
	if !h.service.Enabled(r.Context()) {
		listing, err := h.service.GetListingByContentID(r.Context(), contentID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				errpkg.WriteError(w, http.StatusNotFound, "service_not_found", "Service not found")
				return
			}

			slog.Error("servicedir: reviews listing kind lookup failed", "content_id", contentID, "error", err)
			errpkg.WriteError(w, http.StatusInternalServerError, "reviews_failed", "Failed to list reviews")

			return
		}

		if listing.ListingKind != ListingKindMentorship {
			errpkg.WriteError(w, http.StatusNotFound, "service_directory_disabled", "Service directory is not available")
			return
		}
	}

	if err := h.service.EnsurePublishedListing(r.Context(), contentID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "service_not_found", "Service not found")
			return
		}

		slog.Error("servicedir: reviews listing lookup failed", "content_id", contentID, "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "reviews_failed", "Failed to list reviews")

		return
	}

	page, limit := parseReviewPageParams(r)
	offset := (page - 1) * limit

	var total int32
	meta := map[string]any{"page": page, "limit": limit, "total": total}
	resp := map[string]any{"meta": meta}

	if h.reviews != nil {
		// The aggregate doubles as the total — one fetch serves both.
		if agg, aggErr := h.reviews.Aggregate(r.Context(), "service_listing", contentID); aggErr != nil {
			slog.Warn("servicedir: fetch review aggregate for reviews list", "content_id", contentID, "error", aggErr)
		} else {
			total = agg.Count
			meta["total"] = total
		}

		reviews, revErr := h.reviews.ListByEntity(r.Context(), "service_listing", contentID, limit, offset, "latest")
		if revErr != nil {
			slog.Error("servicedir: list reviews failed", "content_id", contentID, "error", revErr)
			errpkg.WriteError(w, http.StatusInternalServerError, "reviews_failed", "Failed to list reviews")

			return
		}

		h.resolveLiterals(r, &reviews)
		resp["data"] = reviews
	}

	errpkg.WriteJSON(w, http.StatusOK, resp)
}

// ---------------------------------------------------------------------------
// Authenticated handlers (create, edit, claim — auth required)
// ---------------------------------------------------------------------------

// HandleCreate handles POST /services — create a new service listing. Auth
// required (service.create permission checked at route registration).
func (h *Handlers) HandleCreate(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	if h.service == nil || !h.service.Enabled(r.Context()) {
		errpkg.WriteError(w, http.StatusNotFound, "service_directory_disabled", "Service directory is not available")
		return
	}

	var input CreateServiceInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if input.Title == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_title", "Title is required")
		return
	}

	// §111: package creation needs the service.package.create node —
	// resolved in-handler so plain listings still only need service.create.
	canCreatePackage := h.rbac != nil && h.rbac.HasPermission(r.Context(), user.UserID, "service.package.create")

	service, err := h.service.Create(r.Context(), user.UserID, input, canCreatePackage)
	if err != nil {
		if errors.Is(err, ErrInvalidCategory) {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_category", "service_category is not a valid active category")
			return
		}

		writeServiceError(w, err, "create_failed", "Failed to create service listing")
		slog.Error("servicedir: create failed", "error", err)

		return
	}

	h.resolveLiterals(r, &service)

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": service})
}

// HandleEdit handles PATCH /services/:id — edit a service listing. Auth
// required. The route accepts either service.edit_own or service.edit_any
// (BUG-009). The feature flag is enforced by the features.Gate middleware
// at route registration (BUG-014: no redundant in-handler check).
func (h *Handlers) HandleEdit(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	contentID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	if h.service == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "service_directory_unavailable", "Service directory not configured")
		return
	}

	var input EditServiceInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	// Determine whether the user holds service.edit_any (admins bypass
	// the ownership check — BUG-009). The route middleware already verified
	// the user has at least one of edit_own/edit_any.
	isEditAny := h.rbac != nil && h.rbac.HasPermission(r.Context(), user.UserID, "service.edit_any")

	// §111: editing package fields requires service.package.create.
	canCreatePackage := isEditAny || (h.rbac != nil && h.rbac.HasPermission(r.Context(), user.UserID, "service.package.create"))

	service, err := h.service.EditOwn(r.Context(), contentID, user.UserID, input, isEditAny, canCreatePackage)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "service_not_found", "Service not found")
			return
		}

		if errors.Is(err, ErrNotOwner) {
			errpkg.WriteError(w, http.StatusForbidden, "not_owner", "You can only edit your own service listings")
			return
		}

		if errors.Is(err, ErrInvalidCategory) {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_category", "service_category is not a valid active category")
			return
		}

		slog.Error("servicedir: edit failed", "error", err)
		writeServiceError(w, err, "edit_failed", "Failed to edit service listing")

		return
	}

	h.resolveLiterals(r, &service)

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": service})
}

// HandleClaim handles POST /services/:id/claim — initiate an organization
// claim on a service listing. Auth required (service.claim permission
// checked at route registration).
func (h *Handlers) HandleClaim(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	contentID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	if h.service == nil || !h.service.Enabled(r.Context()) {
		errpkg.WriteError(w, http.StatusNotFound, "service_directory_disabled", "Service directory is not available")
		return
	}

	var body struct {
		OrgID string `json:"org_id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if body.OrgID == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_org_id", "org_id is required")
		return
	}

	var orgID pgtype.UUID
	if err := orgID.Scan(body.OrgID); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid org_id")
		return
	}

	if err := h.service.Claim(r.Context(), contentID, user.UserID, orgID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "service_not_found", "Service not found")
			return
		}

		if errors.Is(err, ErrNotOrgMember) {
			errpkg.WriteError(w, http.StatusForbidden, "not_org_member", "You are not a member of the claiming organization")
			return
		}

		if errors.Is(err, ErrAlreadyClaimed) {
			errpkg.WriteError(w, http.StatusConflict, "already_claimed", "This service listing is already claimed or has a pending claim")
			return
		}

		slog.Error("servicedir: claim failed", "error", err, "user_id", user.UserID)
		errpkg.WriteError(w, http.StatusInternalServerError, "claim_failed", "Failed to initiate claim")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"claim_status": ClaimStatusPending}})
}

// ---------------------------------------------------------------------------
// Listing coupon attach (marketplace — GET/PUT /services/:id/coupons;
// service.edit_own|service.edit_any checked at route registration, and the
// service layer re-verifies the caller owns *this* listing)
// ---------------------------------------------------------------------------

// HandleGetCoupons handles GET /services/:id/coupons — the listing's
// attached coupon set (all statuses) for the editor. :id is the listing's
// content id (same param PATCH /services/:id uses).
func (h *Handlers) HandleGetCoupons(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	contentID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	if h.service == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "service_directory_unavailable", "Service directory not configured")
		return
	}

	isEditAny := h.rbac != nil && h.rbac.HasPermission(r.Context(), user.UserID, "service.edit_any")

	coupons, err := h.service.ListingCouponsForEdit(r.Context(), contentID, user.UserID, isEditAny)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "service_not_found", "Service not found")
			return
		}

		writeServiceError(w, err, "coupons_failed", "Failed to list listing coupons")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": coupons})
}

// HandleSetCoupons handles PUT /services/:id/coupons — replace-on-save of
// the listing's attached coupon set. Body: {"coupon_ids": ["uuid", ...]};
// an empty/absent list clears the attachments. Returns the resulting set.
func (h *Handlers) HandleSetCoupons(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	contentID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	if h.service == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "service_directory_unavailable", "Service directory not configured")
		return
	}

	var body struct {
		CouponIDs []string `json:"coupon_ids"`
	}

	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	couponIDs := make([]pgtype.UUID, 0, len(body.CouponIDs))
	for _, raw := range body.CouponIDs {
		var id pgtype.UUID
		if err := id.Scan(raw); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid coupon id")
			return
		}

		couponIDs = append(couponIDs, id)
	}

	// Same pattern as HandleEdit: the route middleware verified at least
	// one of edit_own/edit_any; service.edit_any additionally bypasses the
	// ownership check in the service layer.
	isEditAny := h.rbac != nil && h.rbac.HasPermission(r.Context(), user.UserID, "service.edit_any")

	coupons, err := h.service.SetListingCoupons(r.Context(), contentID, user.UserID, isEditAny, couponIDs)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "service_not_found", "Service not found")
			return
		}

		slog.Error("servicedir: set coupons failed", "content_id", contentID, "error", err)
		writeServiceError(w, err, "coupons_update_failed", "Failed to update listing coupons")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": coupons})
}

// ---------------------------------------------------------------------------
// Admin handlers (verify + moderation queue + claim resolution —
// service.verify permission)
// ---------------------------------------------------------------------------

// HandleAdminList handles GET /admin/services — moderation queue listing
// all service listings (any status). Admin-only (service.verify permission
// checked at route registration). Supports
// ?status=&verified=&claim_status=&category=&kind=&q=&page=&limit=.
// §111: kind=package powers the package review queue.
func (h *Handlers) HandleAdminList(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}

	if h.service == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "service_directory_unavailable", "Service directory not configured")
		return
	}

	status := r.URL.Query().Get("status")
	claimStatus := r.URL.Query().Get("claim_status")
	serviceCategory := r.URL.Query().Get("category")
	listingKind := r.URL.Query().Get("kind")
	if listingKind == "all" {
		listingKind = ""
	}

	q := r.URL.Query().Get("q")

	var destSpaceID pgtype.UUID
	if v := r.URL.Query().Get("dest"); v != "" {
		if err := destSpaceID.Scan(v); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid destination space ID")
			return
		}
	}

	var verifiedPtr *bool

	if v := r.URL.Query().Get("verified"); v != "" {
		b, err := strconv.ParseBool(v)
		if err == nil {
			verifiedPtr = &b
		}
	}

	page, limit := parsePageParams(r)
	if limit > 200 {
		limit = 200
	}

	services, total, err := h.service.AdminList(r.Context(), status, claimStatus, serviceCategory, listingKind, q, verifiedPtr, destSpaceID, page, limit)
	if err != nil {
		slog.Error("servicedir: admin list failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "admin_list_failed", "Failed to list services")

		return
	}

	h.resolveLiterals(r, &services)

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": services,
		"meta": map[string]any{
			"page":  page,
			"limit": limit,
			"total": total,
		},
	})
}

// HandleAdminVerify handles PATCH /admin/services/:id/verify — mark a
// service listing as verified. Admin-only (service.verify permission
// checked at route registration). The admin user_id is recorded in
// verified_by for audit (ADR-088, BUG-015). Optional body:
// {"verification_type":"admin"} — defaults to "admin" since an admin is
// performing the verification (§98 enum: self|community|admin|professional_body).
func (h *Handlers) HandleAdminVerify(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	contentID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	if h.service == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "service_directory_unavailable", "Service directory not configured")
		return
	}

	verificationType := VerificationTypeAdmin

	var body struct {
		VerificationType *string `json:"verification_type"`
	}

	if err := json.NewDecoder(r.Body).Decode(&body); err == nil && body.VerificationType != nil && *body.VerificationType != "" {
		verificationType = *body.VerificationType
	}

	if err := h.service.AdminVerify(r.Context(), contentID, user.UserID, verificationType); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "service_not_found", "Service not found")
			return
		}

		if errors.Is(err, ErrInvalidVerificationType) {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_verification_type", "verification_type must be one of self|community|admin|professional_body")
			return
		}

		slog.Error("servicedir: admin verify failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "verify_failed", "Failed to verify service")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"verified": true, "verification_type": verificationType, "verified_by": user.UserID.String()}})
}

// HandleAdminUnverify handles PATCH /admin/services/:id/unverify — mark a
// service listing as unverified. Admin-only (service.verify permission
// checked at route registration). Clears the verified flag and audit trail.
func (h *Handlers) HandleAdminUnverify(w http.ResponseWriter, r *http.Request) {
	_, ok := requireUser(w, r)
	if !ok {
		return
	}

	contentID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	if h.service == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "service_directory_unavailable", "Service directory not configured")
		return
	}

	if err := h.service.AdminUnverify(r.Context(), contentID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "service_not_found", "Service not found")
			return
		}

		slog.Error("servicedir: admin unverify failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "unverify_failed", "Failed to unverify service")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"verified": false}})
}

// HandleAdminResolveClaim handles PATCH /admin/services/:id/claim/:claim_id
// — resolve a pending organization claim. Admin-only (service.verify
// permission checked at route registration). The :claim_id URL param is the
// org_id of the claimant. Body: {"approve": true|false}.
func (h *Handlers) HandleAdminResolveClaim(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}

	contentID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	orgID, ok := scanUUIDParam(w, r, "claim_id")
	if !ok {
		return
	}

	if h.service == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "service_directory_unavailable", "Service directory not configured")
		return
	}

	var body struct {
		Approve *bool `json:"approve"`
	}

	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if body.Approve == nil {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_approve", "approve field is required")
		return
	}

	if err := h.service.AdminResolveClaim(r.Context(), contentID, orgID, *body.Approve); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "service_not_found", "Service not found")
			return
		}

		slog.Error("servicedir: admin resolve claim failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "resolve_claim_failed", "Failed to resolve claim")

		return
	}

	claimStatus := ClaimStatusApproved
	if !*body.Approve {
		claimStatus = ClaimStatusRejected
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"claim_status": claimStatus}})
}

// ---------------------------------------------------------------------------
// Admin service_categories CRUD (marketplace upgrade — dynamic taxonomy,
// service.categories.manage permission checked at route registration)
// ---------------------------------------------------------------------------

// writeCategoryError maps category service-layer errors to the standard
// snake_case API error contract. Validation failures carry their own
// message; unexpected errors fall through to the provided 500.
func writeCategoryError(w http.ResponseWriter, err error, code, msg string) {
	var inputErr *CategoryInputError
	switch {
	case errors.As(err, &inputErr):
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_input", inputErr.Msg)
	case errors.Is(err, ErrCategoryExists):
		errpkg.WriteError(w, http.StatusConflict, "category_exists", "A category with that slug already exists")
	case errors.Is(err, pgx.ErrNoRows):
		errpkg.WriteError(w, http.StatusNotFound, "not_found", "Category not found")
	default:
		slog.Error("servicedir: category request failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, code, msg)
	}
}

// HandleAdminListCategories handles GET /admin/service-categories — the
// full taxonomy (active + inactive rows) in position order.
func (h *Handlers) HandleAdminListCategories(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}

	if h.service == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "service_directory_unavailable", "Service directory not configured")
		return
	}

	cats, err := h.service.AdminListCategories(r.Context())
	if err != nil {
		slog.Error("servicedir: admin list categories failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "list_failed", "Failed to list service categories")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": cats})
}

// HandleAdminCreateCategory handles POST /admin/service-categories —
// create a taxonomy row. Body: CategoryInput (slug + label required).
// 201 on success; duplicate slug → 409 category_exists.
func (h *Handlers) HandleAdminCreateCategory(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}

	if h.service == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "service_directory_unavailable", "Service directory not configured")
		return
	}

	var input CategoryInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	cat, err := h.service.AdminCreateCategory(r.Context(), input)
	if err != nil {
		writeCategoryError(w, err, "create_failed", "Failed to create service category")
		return
	}

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": cat})
}

// HandleAdminUpdateCategory handles PATCH
// /admin/service-categories/{slug} — patch a taxonomy row. The slug is
// immutable: a body slug that disagrees with the path is rejected.
// Unknown slug → 404 not_found.
func (h *Handlers) HandleAdminUpdateCategory(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}

	if h.service == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "service_directory_unavailable", "Service directory not configured")
		return
	}

	slug := chi.URLParam(r, "slug")
	if slug == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_slug", "Category slug is required")
		return
	}

	var input CategoryInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	cat, err := h.service.AdminUpdateCategory(r.Context(), slug, input)
	if err != nil {
		writeCategoryError(w, err, "update_failed", "Failed to update service category")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": cat})
}

// HandleAdminDeleteCategory handles DELETE
// /admin/service-categories/{slug} — removes a taxonomy row. Slugs still
// referenced by listings are soft-disabled (is_active=false) instead of
// hard-deleted. 204 on success; unknown slug → 404 not_found.
func (h *Handlers) HandleAdminDeleteCategory(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}

	if h.service == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "service_directory_unavailable", "Service directory not configured")
		return
	}

	slug := chi.URLParam(r, "slug")
	if slug == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_slug", "Category slug is required")
		return
	}

	if err := h.service.AdminDeleteCategory(r.Context(), slug); err != nil {
		writeCategoryError(w, err, "delete_failed", "Failed to delete service category")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
