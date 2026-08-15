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

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/config"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/tenant"
)

// Handlers exposes the service directory endpoints.
type Handlers struct {
	service  *Service
	settings *config.Settings
	rbac     RBACResolver
	reviews  ReviewFetcher
}

// RBACResolver is the subset of the RBAC resolver needed to check
// permissions in-handler (e.g. service.edit_any for the edit bypass).
type RBACResolver interface {
	HasPermission(ctx context.Context, userID, tenantID pgtype.UUID, permKey string) bool
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
	ListByEntity(ctx context.Context, tenantID pgtype.UUID, entityType string, entityID pgtype.UUID, limit, offset int32, sort string) ([]ReviewItem, error)
	Aggregate(ctx context.Context, tenantID pgtype.UUID, entityType string, entityID pgtype.UUID) (ReviewAggregate, error)
}

// NewHandlers creates service directory handlers bound to the provided
// service. The rbac resolver is used for in-handler permission checks
// (service.edit_any bypass). The reviews fetcher is optional — when nil,
// GET /services/:slug omits the reviews/aggregate sidecar fields.
func NewHandlers(service *Service, settings *config.Settings, rbac RBACResolver, reviews ReviewFetcher) *Handlers {
	return &Handlers{service: service, settings: settings, rbac: rbac, reviews: reviews}
}

// ---------------------------------------------------------------------------
// Shared helpers (same pattern as story/handlers.go)
// ---------------------------------------------------------------------------

func requireTenant(w http.ResponseWriter, r *http.Request) (pgtype.UUID, bool) {
	t := tenant.FromContext(r.Context())
	if t == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "tenant_unavailable", "No tenant available")
		return pgtype.UUID{}, false
	}
	return t.ID, true
}

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
			page = int32(n)
		}
	}
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 100 {
			limit = int32(n)
		}
	}
	return page, limit
}

// ---------------------------------------------------------------------------
// Public handlers (list + single — anonymous browse per §98)
// ---------------------------------------------------------------------------

// HandleList handles GET /services — list published service listings with
// filters. Query params: ?dest=&city=&category=&origin=&verified=&sort=top|latest&page=&limit=
func (h *Handlers) HandleList(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	if h.service == nil || !h.service.Enabled(r.Context(), tenantID) {
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

	var verifiedPtr *bool
	if v := r.URL.Query().Get("verified"); v != "" {
		b, err := strconv.ParseBool(v)
		if err == nil {
			verifiedPtr = &b
		}
	}

	page, limit := parsePageParams(r)

	services, total, err := h.service.List(r.Context(), tenantID, destSpaceID, citySpaceID, serviceCategory, originCommunity, verifiedPtr, sort, page, limit)
	if err != nil {
		slog.Error("servicedir: list failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "list_failed", "Failed to list services")
		return
	}

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
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	if h.service == nil || !h.service.Enabled(r.Context(), tenantID) {
		errpkg.WriteError(w, http.StatusNotFound, "service_directory_disabled", "Service directory is not available")
		return
	}

	slug := chi.URLParam(r, "slug")
	if slug == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_slug", "Service slug is required")
		return
	}

	service, err := h.service.GetBySlug(r.Context(), slug, tenantID)
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
	resp := map[string]any{"data": service}
	if h.reviews != nil {
		var contentID pgtype.UUID
		if err := contentID.Scan(service.ID); err == nil {
			reviews, revErr := h.reviews.ListByEntity(r.Context(), tenantID, "service_listing", contentID, 10, 0, "latest")
			if revErr != nil {
				slog.Warn("servicedir: fetch reviews for service detail", "slug", slug, "error", revErr)
			} else {
				resp["reviews"] = reviews
			}
			agg, aggErr := h.reviews.Aggregate(r.Context(), tenantID, "service_listing", contentID)
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
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	if h.service == nil || !h.service.Enabled(r.Context(), tenantID) {
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

	service, err := h.service.Create(r.Context(), tenantID, user.UserID, input)
	if err != nil {
		if errors.Is(err, ErrInvalidCategory) {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_category", "service_category must be one of immigration_legal|relocation|finance|education|health|community|other")
			return
		}
		slog.Error("servicedir: create failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "create_failed", "Failed to create service listing")
		return
	}

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
	tenantID, ok := requireTenant(w, r)
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
	isEditAny := h.rbac.HasPermission(r.Context(), user.UserID, tenantID, "service.edit_any")

	service, err := h.service.EditOwn(r.Context(), contentID, tenantID, user.UserID, input, isEditAny)
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
		errpkg.WriteError(w, http.StatusInternalServerError, "edit_failed", "Failed to edit service listing")
		return
	}

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
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}
	contentID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	if h.service == nil || !h.service.Enabled(r.Context(), tenantID) {
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

	if err := h.service.Claim(r.Context(), contentID, tenantID, user.UserID, orgID); err != nil {
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
// ?status=&verified=&claim_status=&category=&q=&page=&limit=.
func (h *Handlers) HandleAdminList(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	if h.service == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "service_directory_unavailable", "Service directory not configured")
		return
	}

	status := r.URL.Query().Get("status")
	claimStatus := r.URL.Query().Get("claim_status")
	serviceCategory := r.URL.Query().Get("category")
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

	services, total, err := h.service.AdminList(r.Context(), tenantID, status, claimStatus, serviceCategory, q, verifiedPtr, destSpaceID, page, limit)
	if err != nil {
		slog.Error("servicedir: admin list failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "admin_list_failed", "Failed to list services")
		return
	}

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
	tenantID, ok := requireTenant(w, r)
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

	if err := h.service.AdminVerify(r.Context(), contentID, tenantID, user.UserID, verificationType); err != nil {
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

// HandleAdminResolveClaim handles PATCH /admin/services/:id/claim/:claim_id
// — resolve a pending organization claim. Admin-only (service.verify
// permission checked at route registration). The :claim_id URL param is the
// org_id of the claimant. Body: {"approve": true|false}
func (h *Handlers) HandleAdminResolveClaim(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}
	tenantID, ok := requireTenant(w, r)
	if !ok {
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

	if err := h.service.AdminResolveClaim(r.Context(), contentID, tenantID, orgID, *body.Approve); err != nil {
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
