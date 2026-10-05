package coupons

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/literals"
)

// Handlers provides HTTP handlers for coupons and affiliate providers.
type Handlers struct {
	Service *Service
	lit     literalResolver // nil disables literal resolution (§14)
	orgs    OrgResolver     // nil disables org deal endpoints (§55)
}

// literalResolver resolves {{...}} template literals in user-facing text
// fields (§14). Implemented by *literals.Catalog; nil disables resolution.
type literalResolver interface {
	ResolveFields(ctx context.Context, v any, rc literals.RenderContext)
}

// OrgResolver is the slice of organizations.Service the deals handlers need —
// defined here to avoid an import cycle (same adapter pattern as
// couponRenderAdapter in server.go).
type OrgResolver interface {
	MemberRole(ctx context.Context, orgID, userID pgtype.UUID) (string, error)
	GetOrganization(ctx context.Context, id pgtype.UUID) (gen.Organization, error)
	// ActiveOrgExists mirrors requireActiveOrgByID (internal/server/org_gate.go):
	// reports whether an ACTIVE, non-deleted org exists in the platform.
	ActiveOrgExists(ctx context.Context, orgID pgtype.UUID) (bool, error)
}

// SetLiteralResolver injects the template-literal catalog so user-facing
// titles/descriptions render {{...}} placeholders (§14).
func (h *Handlers) SetLiteralResolver(l literalResolver) { h.lit = l }

// SetOrgResolver injects the organizations adapter so the org deal
// endpoints can verify membership roles (§55).
func (h *Handlers) SetOrgResolver(o OrgResolver) { h.orgs = o }

// resolveLiterals resolves {{...}} placeholders on a response DTO or slice.
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

// NewHandlers creates a Handlers backed by the given service.
func NewHandlers(service *Service) *Handlers {
	return &Handlers{Service: service}
}

// ── Helpers ─────────────────────────────────────────────────────────

func (h *Handlers) requireUser(w http.ResponseWriter, r *http.Request) (*auth.AuthenticatedUser, bool) {
	user := auth.FromContext(r.Context())
	if user == nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return nil, false
	}

	return user, true
}

func parseUUIDParam(r *http.Request, name string) (pgtype.UUID, error) {
	var id pgtype.UUID
	if err := id.Scan(chi.URLParam(r, name)); err != nil {
		return pgtype.UUID{}, err
	}

	return id, nil
}

func parseQueryInt(r *http.Request, key string, defaultVal int) int {
	v := r.URL.Query().Get(key)
	if v == "" {
		return defaultVal
	}

	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return defaultVal
	}

	return n
}

func parseQueryString(r *http.Request, key string) *string {
	v := r.URL.Query().Get(key)
	if v == "" {
		return nil
	}

	return &v
}

func handleServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrCouponNotFound), errors.Is(err, ErrProviderNotFound),
		errors.Is(err, ErrDealRedirectUnavailable), errors.Is(err, ErrFeedSourceNotFound):
		errpkg.WriteError(w, http.StatusNotFound, "not_found", err.Error())
	case errors.Is(err, ErrInvalidFeedSource):
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_feed_source", err.Error())
	case errors.Is(err, ErrInvalidDetails):
		errpkg.WriteError(w, http.StatusBadRequest, "validation_error", err.Error())
	case errors.Is(err, ErrFeedSyncLocked):
		errpkg.WriteError(w, http.StatusConflict, "feed_sync_locked", err.Error())
	case errors.Is(err, ErrProviderHasCoupons):
		errpkg.WriteError(w, http.StatusConflict, "provider_has_coupons", err.Error())
	case errors.Is(err, ErrCouponAlreadyExists):
		errpkg.WriteError(w, http.StatusConflict, "already_exists", err.Error())
	case errors.Is(err, errpkg.ErrDuplicate):
		errpkg.WriteError(w, http.StatusConflict, "already_exists", "Resource already exists")
	case errors.Is(err, errpkg.ErrRequired):
		errpkg.WriteError(w, http.StatusBadRequest, "validation_error", err.Error())
	default:
		errpkg.HandleServiceError(w, err)
	}
}

// ── Public Coupon Routes ────────────────────────────────────────────

// ListPublicCoupons handles GET /api/v1/coupons.
func (h *Handlers) ListPublicCoupons(w http.ResponseWriter, r *http.Request) {

	providerID := parseQueryUUID(r, "provider_id")
	couponType := parseQueryString(r, "coupon_type")
	// Accept both `q` (spec §55 documented param) and `search` (legacy alias).
	search := parseQueryString(r, "q")
	if search == nil {
		search = parseQueryString(r, "search")
	}

	page := int32(parseQueryInt(r, "page", 1))                        // #nosec
	pageSize := int32(parseQueryInt(r, "page_size", DefaultPageSize)) // #nosec

	coupons, err := h.Service.ListActiveCoupons(r.Context(), providerID, couponType, search, page, pageSize)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	h.resolveLiterals(r, &coupons)

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": coupons})
}

// GetCoupon handles GET /api/v1/coupons/:id.
func (h *Handlers) GetCoupon(w http.ResponseWriter, r *http.Request) {

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid coupon ID")
		return
	}

	coupon, err := h.Service.GetCoupon(r.Context(), id)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	// Org-submitted deals sit at status='pending' until staff review —
	// never expose them through the public endpoint (§55).
	if coupon.Status == string(CouponStatusPending) {
		errpkg.WriteError(w, http.StatusNotFound, "not_found", ErrCouponNotFound.Error())
		return
	}

	h.resolveLiterals(r, &coupon)

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": coupon})
}

// RecordCouponClick handles POST /api/v1/coupons/:id/click.
func (h *Handlers) RecordCouponClick(w http.ResponseWriter, r *http.Request) {

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid coupon ID")
		return
	}
	// Verify the coupon exists (platform-scoped) before recording the event so
	// a missing coupon returns 404 instead of an FK-violation 500 (BUG-013).
	if _, err := h.Service.GetCoupon(r.Context(), id); err != nil {
		handleServiceError(w, err)
		return
	}

	var input struct {
		EventType string       `json:"event_type"`
		ContentID *pgtype.UUID `json:"content_id,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if input.EventType != "click" && input.EventType != "copy" {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_event_type", "event_type must be 'click' or 'copy'")
		return
	}

	var userID *pgtype.UUID
	if user := auth.FromContext(r.Context()); user != nil && user.UserID.Valid {
		userID = &user.UserID
	}

	if err := h.Service.RecordCouponEvent(r.Context(), id, input.EventType, userID, input.ContentID); err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusAccepted, map[string]any{"data": map[string]string{"status": "recorded"}})
}

// ── Public Deal Routes (Section 55 — Deal Pages) ────────────────────

// ListDealPartners handles GET /api/v1/deals/partners — the public index of
// active deal providers (§55). Bounded by ?page_size= (default/cap
// MaxPageSize) — the query applies LIMIT directly.
func (h *Handlers) ListDealPartners(w http.ResponseWriter, r *http.Request) {
	pageSize := int32(parseQueryInt(r, "page_size", MaxPageSize)) // #nosec
	partners, err := h.Service.ListDealPartners(r.Context(), pageSize)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	h.resolveLiterals(r, &partners)

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": partners})
}

// GetDealPartner handles GET /api/v1/deals/partners/{slug} — the provider
// profile plus its active coupons for the /deal/{provider} landing page.
func (h *Handlers) GetDealPartner(w http.ResponseWriter, r *http.Request) {

	slug := chi.URLParam(r, "slug")

	partner, err := h.Service.GetDealPartner(r.Context(), slug)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	h.resolveLiterals(r, partner)

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": partner})
}

// GetDealBySlug handles GET /api/v1/deals/{slug} — coupon detail by slug.
// Renamed slugs resolve through slug_history; the payload's slug and
// provider_slug fields let the frontend 301 to the canonical
// /deal/{provider}/{slug} URL.
func (h *Handlers) GetDealBySlug(w http.ResponseWriter, r *http.Request) {

	slug := chi.URLParam(r, "slug")

	coupon, err := h.Service.ResolveCouponBySlug(r.Context(), slug)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	h.resolveLiterals(r, &coupon)

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": coupon})
}

// DealGo handles GET /api/v1/deals/{slug}/go — records a 'click'
// coupon_event (best-effort, mirroring POST /coupons/:id/click) then 302s to
// the resolved click-through target (referral_url, or the provider base_url
// for expired/untargeted deals).
func (h *Handlers) DealGo(w http.ResponseWriter, r *http.Request) {

	slug := chi.URLParam(r, "slug")

	target, coupon, err := h.Service.ResolveDealClick(r.Context(), slug)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	var userID *pgtype.UUID
	if user := auth.FromContext(r.Context()); user != nil && user.UserID.Valid {
		userID = &user.UserID
	}
	if err := h.Service.RecordCouponEvent(r.Context(), coupon.ID, "click", userID, nil); err != nil {
		slog.Warn("deals: failed to record go click", "slug", slug, "error", err)
	}

	http.Redirect(w, r, target, http.StatusFound)
}

// ── Admin Affiliate Provider Routes ─────────────────────────────────

// ListProviders handles GET /api/v1/admin/affiliate-providers.
func (h *Handlers) ListProviders(w http.ResponseWriter, r *http.Request) {

	page := int32(parseQueryInt(r, "page", 1))                        // #nosec
	pageSize := int32(parseQueryInt(r, "page_size", DefaultPageSize)) // #nosec

	providers, err := h.Service.ListProviders(r.Context(), page, pageSize)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	h.resolveLiterals(r, &providers)

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": providers})
}

// CreateProvider handles POST /api/v1/admin/affiliate-providers.
func (h *Handlers) CreateProvider(w http.ResponseWriter, r *http.Request) {

	var input CreateProviderInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if input.Name == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_field", "name is required")
		return
	}

	if input.Slug == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_field", "slug is required")
		return
	}

	if input.ProviderType == "" {
		input.ProviderType = string(ProviderTypeAffiliate)
	}

	input.IsActive = true

	provider, err := h.Service.CreateProvider(r.Context(), input)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": provider})
}

// GetProvider handles GET /api/v1/admin/affiliate-providers/:id.
func (h *Handlers) GetProvider(w http.ResponseWriter, r *http.Request) {

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid provider ID")
		return
	}

	provider, err := h.Service.GetProvider(r.Context(), id)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": provider})
}

// UpdateProvider handles PATCH /api/v1/admin/affiliate-providers/:id.
func (h *Handlers) UpdateProvider(w http.ResponseWriter, r *http.Request) {

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid provider ID")
		return
	}

	var input UpdateProviderInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if input.Name == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_field", "name is required")
		return
	}

	provider, err := h.Service.UpdateProvider(r.Context(), id, input)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": provider})
}

// DeleteProvider handles DELETE /api/v1/admin/affiliate-providers/:id.
func (h *Handlers) DeleteProvider(w http.ResponseWriter, r *http.Request) {

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid provider ID")
		return
	}

	if err := h.Service.DeleteProvider(r.Context(), id); err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusNoContent, nil)
}

// ── Admin Coupon Routes ─────────────────────────────────────────────

// ListCouponsAdmin handles GET /api/v1/admin/coupons.
func (h *Handlers) ListCouponsAdmin(w http.ResponseWriter, r *http.Request) {

	status := parseQueryString(r, "status")
	couponType := parseQueryString(r, "coupon_type")
	// Accept both `q` (spec §55 documented param) and `search` (legacy alias).
	search := parseQueryString(r, "q")
	if search == nil {
		search = parseQueryString(r, "search")
	}

	page := int32(parseQueryInt(r, "page", 1))                        // #nosec
	pageSize := int32(parseQueryInt(r, "page_size", DefaultPageSize)) // #nosec

	coupons, err := h.Service.ListCoupons(r.Context(), status, couponType, search, page, pageSize)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	h.resolveLiterals(r, &coupons)

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": coupons})
}

// CreateCouponAdmin handles POST /api/v1/admin/coupons.
func (h *Handlers) CreateCouponAdmin(w http.ResponseWriter, r *http.Request) {

	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	var input CreateCouponInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if input.Title == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_field", "title is required")
		return
	}

	if input.CouponType == "" {
		input.CouponType = string(CouponTypeExternal)
	}

	if input.Status == "" {
		input.Status = string(CouponStatusActive)
	}

	coupon, err := h.Service.CreateCoupon(r.Context(), user.UserID, input)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": coupon})
}

// GetCouponAdmin handles GET /api/v1/admin/coupons/:id.
func (h *Handlers) GetCouponAdmin(w http.ResponseWriter, r *http.Request) {

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid coupon ID")
		return
	}

	coupon, err := h.Service.GetCoupon(r.Context(), id)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": coupon})
}

// UpdateCouponAdmin handles PATCH /api/v1/admin/coupons/:id.
func (h *Handlers) UpdateCouponAdmin(w http.ResponseWriter, r *http.Request) {

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid coupon ID")
		return
	}

	var input UpdateCouponInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if input.Title == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_field", "title is required")
		return
	}

	coupon, err := h.Service.UpdateCoupon(r.Context(), id, input)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": coupon})
}

// DeleteCouponAdmin handles DELETE /api/v1/admin/coupons/:id.
func (h *Handlers) DeleteCouponAdmin(w http.ResponseWriter, r *http.Request) {

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid coupon ID")
		return
	}

	if err := h.Service.DeleteCoupon(r.Context(), id); err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusNoContent, nil)
}

// ── Render Helpers ──────────────────────────────────────────────────

// RenderCoupon handles GET /api/v1/render/coupon/:id
// BE-LOW-001: platform-scoped — the platform context from the request ensures
// a render request cannot resolve a coupon from another platform.
func (h *Handlers) RenderCoupon(w http.ResponseWriter, r *http.Request) {

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid coupon ID")
		return
	}

	rendered, err := h.Service.ResolveCouponForRender(r.Context(), id)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	h.resolveLiterals(r, &rendered)

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": rendered})
}

// RenderAffiliateLink handles GET /api/v1/render/affiliate-link
// BE-LOW-001: platform-scoped — the platform context from the request ensures
// a render request cannot resolve a provider from another platform.
func (h *Handlers) RenderAffiliateLink(w http.ResponseWriter, r *http.Request) {

	providerIDStr := r.URL.Query().Get("provider_id")
	originalURL := r.URL.Query().Get("url")

	if providerIDStr == "" || originalURL == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_params", "url and provider_id are required")
		return
	}

	var providerID pgtype.UUID
	if err := providerID.Scan(providerIDStr); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_provider_id", "Invalid provider ID")
		return
	}

	rendered, err := h.Service.ResolveAffiliateLinkForRender(r.Context(), providerID, originalURL)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	h.resolveLiterals(r, &rendered)

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": rendered})
}

// parseQueryUUID is a helper to parse a UUID query parameter.
func parseQueryUUID(r *http.Request, key string) *pgtype.UUID {
	v := r.URL.Query().Get(key)
	if v == "" {
		return nil
	}

	var id pgtype.UUID
	if err := id.Scan(v); err != nil {
		return nil
	}

	return &id
}

// ─── Affiliate Enhancement Handlers ─────────────────────────────────

// ListAffiliateScanResults handles GET /admin/affiliate/scan-results
// BE-LOW-002: uses sqlc-generated query instead of raw SQL.
func (h *Handlers) ListAffiliateScanResults(w http.ResponseWriter, r *http.Request) {

	page := int32(parseQueryInt(r, "page", 1))                        // #nosec
	pageSize := int32(parseQueryInt(r, "page_size", DefaultPageSize)) // #nosec
	limit, offset := paginate(page, pageSize)

	rows, err := h.Service.store.listAffiliateScanResults(r.Context(), gen.ListAffiliateScanResultsParams{

		Domain:      parseQueryString(r, "domain"),
		ActionTaken: parseQueryString(r, "action"),
		PageLimit:   limit,
		PageOffset:  offset,
	})
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "query_error", "Failed to query scan results")
		return
	}

	results := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		results = append(results, map[string]any{
			"id": row.ID.String(), "content_id": row.ContentID.String(), "url": row.Url,
			"domain": row.Domain, "detection_method": row.DetectionMethod, "action_taken": row.ActionTaken,
			"created_at": row.CreatedAt.Time,
		})
	}

	h.resolveLiterals(r, &results)

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": results, "page": page, "page_size": pageSize})
}

// ReviewAffiliateScanResult handles PATCH /admin/affiliate/scan-results/{id}
// BE-LOW-002: uses sqlc-generated query instead of raw SQL.
func (h *Handlers) ReviewAffiliateScanResult(w http.ResponseWriter, r *http.Request) {

	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid scan result ID")
		return
	}

	var input struct {
		Action string  `json:"action"`
		Notes  *string `json:"notes"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	var reviewerID pgtype.UUID
	if err := reviewerID.Scan(user.UserID); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_user_id", "Invalid reviewer ID")
		return
	}

	rowsAffected, err := h.Service.store.reviewAffiliateScanResult(r.Context(), gen.ReviewAffiliateScanResultParams{
		ActionTaken: input.Action,
		ReviewedBy:  reviewerID,
		Notes:       input.Notes,
		ID:          id,
	})
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "update_failed", "Failed to review scan result")
		return
	}

	if rowsAffected == 0 {
		errpkg.WriteError(w, http.StatusNotFound, "scan_result_not_found", "Scan result not found")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]bool{"updated": true}})
}

// ListAffiliateCommissions handles GET /admin/affiliate/commissions
// BE-LOW-002: uses sqlc-generated query instead of raw SQL.
func (h *Handlers) ListAffiliateCommissions(w http.ResponseWriter, r *http.Request) {

	page := int32(parseQueryInt(r, "page", 1))                        // #nosec
	pageSize := int32(parseQueryInt(r, "page_size", DefaultPageSize)) // #nosec
	limit, offset := paginate(page, pageSize)

	// Parse optional credited filter into a *bool for the sqlc narg.
	var creditedFilter *bool

	if raw := parseQueryString(r, "credited"); raw != nil {
		switch *raw {
		case "true", "1":
			t := true
			creditedFilter = &t
		case "false", "0":
			f := false
			creditedFilter = &f
		default:
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_filter", "credited must be 'true' or 'false'")
			return
		}
	}

	rows, err := h.Service.store.listAffiliateCommissions(r.Context(), gen.ListAffiliateCommissionsParams{

		Credited:   creditedFilter,
		PageLimit:  limit,
		PageOffset: offset,
	})
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "query_error", "Failed to query commissions")
		return
	}

	results := make([]map[string]any, 0, len(rows))

	for _, row := range rows {
		extCommF, _ := row.ExternalCommission.Float64Value()
		authorShareF, _ := row.AuthorShare.Float64Value()
		pctF, _ := row.AuthorCommissionPct.Float64Value()
		results = append(results, map[string]any{
			"id": row.ID.String(), "content_id": row.ContentID.String(),
			"author_user_id": row.AuthorUserID.String(), "author_username": row.AuthorUsername,
			"provider_name":       row.ProviderName,
			"external_commission": extCommF.Float64, "author_share": authorShareF.Float64,
			"author_commission_pct": pctF.Float64, "credited": row.Credited, "created_at": row.CreatedAt.Time,
		})
	}

	h.resolveLiterals(r, &results)

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": results, "page": page, "page_size": pageSize})
}

// UpdateProviderAuthorCommission handles PATCH /admin/affiliate/providers/{id}/author-commission
// BE-LOW-002: uses sqlc-generated query instead of raw SQL.
func (h *Handlers) UpdateProviderAuthorCommission(w http.ResponseWriter, r *http.Request) {

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid provider ID")
		return
	}

	var input struct {
		AuthorCommissionPct float64 `json:"author_commission_pct"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if input.AuthorCommissionPct < 0 || input.AuthorCommissionPct > 100 {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_pct", "Percentage must be 0-100")
		return
	}

	rowsAffected, err := h.Service.store.updateProviderAuthorCommission(r.Context(), gen.UpdateProviderAuthorCommissionParams{
		AuthorCommissionPct: numericFromFloat(input.AuthorCommissionPct),
		ID:                  id,
	})
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "update_failed", "Failed to update author commission percentage")
		return
	}

	if rowsAffected == 0 {
		errpkg.WriteError(w, http.StatusNotFound, "provider_not_found", "Affiliate provider not found")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]bool{"updated": true}})
}

// RecordAffiliateCommission handles POST /admin/affiliate/commissions.
//
// Records an external affiliate commission earned from a content's affiliate
// link. The admin provides the provider_id, content_id, and the external
// commission amount. The service looks up the content's author and the
// provider's author_commission_pct, computes the author's share, inserts
// the commission row, and enqueues the affiliate:credit_author_commission
// task to credit the author's wallet.
//
// AUDIT-BE-TASK-001: this is the missing INSERT site for
// affiliate_content_commissions and the enqueue trigger for the
// affiliate:credit_author_commission task.
func (h *Handlers) RecordAffiliateCommission(w http.ResponseWriter, r *http.Request) {

	var input struct {
		ProviderID         string  `json:"provider_id"`
		ContentID          string  `json:"content_id"`
		ExternalCommission float64 `json:"external_commission"`
		EventDescription   *string `json:"event_description,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	var providerID pgtype.UUID
	if err := providerID.Scan(input.ProviderID); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_provider_id", "Invalid provider_id")
		return
	}

	var contentID pgtype.UUID
	if err := contentID.Scan(input.ContentID); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_content_id", "Invalid content_id")
		return
	}

	if input.ExternalCommission <= 0 {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_commission", "External commission must be positive")
		return
	}

	commission, err := h.Service.RecordAffiliateCommission(r.Context(), RecordAffiliateCommissionInput{
		ProviderID:         providerID,
		ContentID:          contentID,
		ExternalCommission: input.ExternalCommission,
		EventDescription:   input.EventDescription,
	})
	if err != nil {
		// Distinguish not-found (content or provider missing) from internal errors.
		errMsg := err.Error()
		if strings.Contains(errMsg, "content not found") {
			errpkg.WriteError(w, http.StatusNotFound, "content_not_found", "Content not found")
			return
		}

		if strings.Contains(errMsg, "provider not found") {
			errpkg.WriteError(w, http.StatusNotFound, "provider_not_found", "Affiliate provider not found")
			return
		}

		if strings.Contains(errMsg, "no author_id") {
			errpkg.WriteError(w, http.StatusBadRequest, "no_author", "Content has no author")
			return
		}

		errpkg.CaptureException(err)
		errpkg.WriteError(w, http.StatusInternalServerError, "record_failed", "Failed to record commission")

		return
	}

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": commission})
}

// ── Deal Feed Sources (Section 55 — Deal Feed Import) ───────────────

// ListCouponFeeds handles GET /api/v1/admin/coupon-feeds.
func (h *Handlers) ListCouponFeeds(w http.ResponseWriter, r *http.Request) {
	feeds, err := h.Service.ListFeedSources(r.Context())
	if err != nil {
		handleServiceError(w, err)
		return
	}
	if feeds == nil {
		feeds = []FeedSourceView{}
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": feeds})
}

// CreateCouponFeed handles POST /api/v1/admin/coupon-feeds.
func (h *Handlers) CreateCouponFeed(w http.ResponseWriter, r *http.Request) {
	var input FeedSourceInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}
	if strings.TrimSpace(input.Name) == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_field", "name is required")
		return
	}
	if strings.TrimSpace(input.EndpointURL) == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_field", "endpoint_url is required")
		return
	}

	feed, err := h.Service.CreateFeedSource(r.Context(), input)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": feed})
}

// GetCouponFeed handles GET /api/v1/admin/coupon-feeds/:id.
func (h *Handlers) GetCouponFeed(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid feed ID")
		return
	}
	feed, err := h.Service.GetFeedSource(r.Context(), id)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": feed})
}

// UpdateCouponFeed handles PATCH /api/v1/admin/coupon-feeds/:id.
func (h *Handlers) UpdateCouponFeed(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid feed ID")
		return
	}
	var input FeedSourceInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}
	if strings.TrimSpace(input.Name) == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_field", "name is required")
		return
	}
	if strings.TrimSpace(input.EndpointURL) == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_field", "endpoint_url is required")
		return
	}

	feed, err := h.Service.UpdateFeedSource(r.Context(), id, input)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": feed})
}

// DeleteCouponFeed handles DELETE /api/v1/admin/coupon-feeds/:id. Imported
// coupons are detached (feed_id → NULL), never deleted (§55).
func (h *Handlers) DeleteCouponFeed(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid feed ID")
		return
	}
	if err := h.Service.DeleteFeedSource(r.Context(), id); err != nil {
		handleServiceError(w, err)
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"deleted": true})
}

// SyncCouponFeedNow handles POST /api/v1/admin/coupon-feeds/:id/sync —
// enqueues coupon:import_feed (fire-and-forget per Rule 8).
func (h *Handlers) SyncCouponFeedNow(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid feed ID")
		return
	}
	if err := h.Service.EnqueueFeedSync(r.Context(), id); err != nil {
		handleServiceError(w, err)
		return
	}
	errpkg.WriteJSON(w, http.StatusAccepted, map[string]any{"data": map[string]bool{"queued": true}})
}

// ── Organization Deal Submissions (Section 55 — Deals Redesign) ─────

// requireOrgEditor resolves the org from {id} and verifies the caller holds an
// owner/admin/editor role. Returns (org, user, ok); writes the error response
// on failure. Plain 'member' cannot submit deals.
func (h *Handlers) requireOrgEditor(w http.ResponseWriter, r *http.Request) (gen.Organization, *auth.AuthenticatedUser, bool) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return gen.Organization{}, nil, false
	}
	if h.orgs == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "feature_unavailable", "Organization deals are not available")
		return gen.Organization{}, nil, false
	}
	orgID, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid organization ID")
		return gen.Organization{}, nil, false
	}
	// Parity with requireActiveOrgByID: only ACTIVE, non-deleted orgs in this
	// platform may submit or list deals.
	exists, err := h.orgs.ActiveOrgExists(r.Context(), orgID)
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to resolve organization")
		return gen.Organization{}, nil, false
	}
	if !exists {
		errpkg.WriteError(w, http.StatusNotFound, "not_found", "Organization not found")
		return gen.Organization{}, nil, false
	}
	org, err := h.orgs.GetOrganization(r.Context(), orgID)
	if err != nil {
		errpkg.WriteError(w, http.StatusNotFound, "not_found", "Organization not found")
		return gen.Organization{}, nil, false
	}
	role, err := h.orgs.MemberRole(r.Context(), orgID, user.UserID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// Non-member → 403 (BUG-021: never 404 for a permission denial).
			errpkg.WriteError(w, http.StatusForbidden, "forbidden", "You must be an organization editor to manage deals")
		} else {
			// A real DB failure must not masquerade as a permission denial.
			errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to verify organization membership")
		}
		return gen.Organization{}, nil, false
	}
	if role != "owner" && role != "admin" && role != "editor" {
		errpkg.WriteError(w, http.StatusForbidden, "forbidden", "Organization deal management requires an editor role")
		return gen.Organization{}, nil, false
	}
	return org, user, true
}

// SubmitOrgDeal handles POST /api/v1/organizations/{id}/deals — org members
// submit deals that land as status='pending' for staff moderation (§55).
func (h *Handlers) SubmitOrgDeal(w http.ResponseWriter, r *http.Request) {
	org, user, ok := h.requireOrgEditor(w, r)
	if !ok {
		return
	}
	var input SubmitDealInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}
	if input.Title == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_field", "title is required")
		return
	}
	coupon, err := h.Service.SubmitOrgDeal(r.Context(), org, user.UserID, input)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": coupon})
}

// ListOrgDeals handles GET /api/v1/organizations/{id}/deals — the org's own
// submissions in every status (§55).
func (h *Handlers) ListOrgDeals(w http.ResponseWriter, r *http.Request) {
	org, _, ok := h.requireOrgEditor(w, r)
	if !ok {
		return
	}
	page := int32(parseQueryInt(r, "page", 1))                        // #nosec
	pageSize := int32(parseQueryInt(r, "page_size", DefaultPageSize)) // #nosec
	coupons, err := h.Service.ListOrgDeals(r.Context(), org.ID, page, pageSize)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	h.resolveLiterals(r, &coupons)
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": coupons, "page": page, "page_size": pageSize})
}
