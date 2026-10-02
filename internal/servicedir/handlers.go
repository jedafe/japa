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
	case errors.Is(err, ErrOrgNotVerified):
		errpkg.WriteError(w, http.StatusForbidden, "organization_not_verified", "Only verified organizations can publish packages")
	case errors.Is(err, ErrOrgRequired):
		errpkg.WriteError(w, http.StatusBadRequest, "org_required", "org_id is required for package listings")
	case errors.Is(err, ErrPurchaseModeUnavailable):
		errpkg.WriteError(w, http.StatusForbidden, "purchase_mode_unavailable", "This purchase mode is not available")
	case errors.Is(err, ErrInvalidListingKind):
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_listing_kind", "listing_kind must be service|package and cannot change after creation")
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
	case errors.Is(err, ErrInvalidIntent):
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_intent", "intent must be inquiry|booking and match the listing's purchase_mode")
	case errors.Is(err, ErrCheckoutFieldsRequired):
		errpkg.WriteError(w, http.StatusBadRequest, "checkout_fields_required", "provider and email are required for paid bookings")
	case errors.Is(err, ErrInvalidProvider):
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_provider", "provider must be one of: paystack, stripe, nowpayments, manual_transfer, credit")
	case errors.Is(err, ErrInvalidPreferredDates):
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_preferred_dates", "preferred_dates must be {start_on, end_on, flexible} with start_on <= end_on")
	case errors.Is(err, ErrPackageOrgImmutable):
		errpkg.WriteError(w, http.StatusBadRequest, "package_org_immutable", "A package's selling organization cannot change after creation")
	case errors.Is(err, ErrInvalidID):
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid id")
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
// service category option list (Rule 16).
func (h *Handlers) HandleCategories(w http.ResponseWriter, r *http.Request) {

	if h.service == nil || !h.service.Enabled(r.Context()) {
		errpkg.WriteError(w, http.StatusNotFound, "service_directory_disabled", "Service directory is not available")
		return
	}

	cats := h.service.Categories()
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": cats})
}

// HandleList handles GET /services — list published service listings with
// filters. Query params: ?dest=&city=&category=&origin=&verified=&sort=top|latest&q=&page=&limit=.
func (h *Handlers) HandleList(w http.ResponseWriter, r *http.Request) {

	if h.service == nil || !h.service.Enabled(r.Context()) {
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

	if h.service == nil || !h.service.Enabled(r.Context()) {
		errpkg.WriteError(w, http.StatusNotFound, "service_directory_disabled", "Service directory is not available")
		return
	}

	slug := chi.URLParam(r, "slug")
	if slug == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_slug", "Service slug is required")
		return
	}

	service, err := h.service.GetBySlug(r.Context(), slug)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "service_not_found", "Service not found")
			return
		}

		slog.Error("servicedir: get by slug failed", "slug", slug, "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "get_failed", "Failed to get service")

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
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_category", "service_category must be one of immigration_legal|relocation|finance|education|health|community|other")
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
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_category", "service_category must be one of immigration_legal|relocation|finance|education|health|community|other")
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
