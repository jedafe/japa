package coupons

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/tenant"
)

// Handlers provides HTTP handlers for coupons and affiliate providers.
type Handlers struct {
	Service *Service
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

func (h *Handlers) requireTenant(w http.ResponseWriter, r *http.Request) (pgtype.UUID, bool) {
	t := tenant.FromContext(r.Context())
	if t == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "tenant_unavailable", "Tenant context not available")
		return pgtype.UUID{}, false
	}

	return t.ID, true
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
	case errors.Is(err, ErrCouponNotFound), errors.Is(err, ErrProviderNotFound):
		errpkg.WriteError(w, http.StatusNotFound, "not_found", err.Error())
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
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	providerID := parseQueryUUID(r, "provider_id")
	couponType := parseQueryString(r, "coupon_type")
	// Accept both `q` (spec §55 documented param) and `search` (legacy alias).
	search := parseQueryString(r, "q")
	if search == nil {
		search = parseQueryString(r, "search")
	}

	page := int32(parseQueryInt(r, "page", 1))                        // #nosec
	pageSize := int32(parseQueryInt(r, "page_size", DefaultPageSize)) // #nosec

	coupons, err := h.Service.ListActiveCoupons(r.Context(), tenantID, providerID, couponType, search, page, pageSize)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": coupons})
}

// GetCoupon handles GET /api/v1/coupons/:id.
func (h *Handlers) GetCoupon(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid coupon ID")
		return
	}

	coupon, err := h.Service.GetCoupon(r.Context(), id, tenantID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": coupon})
}

// RecordCouponClick handles POST /api/v1/coupons/:id/click.
func (h *Handlers) RecordCouponClick(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid coupon ID")
		return
	}
	// Verify the coupon exists (tenant-scoped) before recording the event so
	// a missing coupon returns 404 instead of an FK-violation 500 (BUG-013).
	if _, err := h.Service.GetCoupon(r.Context(), id, tenantID); err != nil {
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

// ── Admin Affiliate Provider Routes ─────────────────────────────────

// ListProviders handles GET /api/v1/admin/affiliate-providers.
func (h *Handlers) ListProviders(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	page := int32(parseQueryInt(r, "page", 1))                        // #nosec
	pageSize := int32(parseQueryInt(r, "page_size", DefaultPageSize)) // #nosec

	providers, err := h.Service.ListProviders(r.Context(), tenantID, page, pageSize)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": providers})
}

// CreateProvider handles POST /api/v1/admin/affiliate-providers.
func (h *Handlers) CreateProvider(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

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

	provider, err := h.Service.CreateProvider(r.Context(), tenantID, input)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": provider})
}

// GetProvider handles GET /api/v1/admin/affiliate-providers/:id.
func (h *Handlers) GetProvider(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid provider ID")
		return
	}

	provider, err := h.Service.GetProvider(r.Context(), id, tenantID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": provider})
}

// UpdateProvider handles PATCH /api/v1/admin/affiliate-providers/:id.
func (h *Handlers) UpdateProvider(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

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

	provider, err := h.Service.UpdateProvider(r.Context(), id, tenantID, input)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": provider})
}

// DeleteProvider handles DELETE /api/v1/admin/affiliate-providers/:id.
func (h *Handlers) DeleteProvider(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid provider ID")
		return
	}

	if err := h.Service.DeleteProvider(r.Context(), id, tenantID); err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusNoContent, nil)
}

// ── Admin Coupon Routes ─────────────────────────────────────────────

// ListCouponsAdmin handles GET /api/v1/admin/coupons.
func (h *Handlers) ListCouponsAdmin(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	status := parseQueryString(r, "status")
	couponType := parseQueryString(r, "coupon_type")
	// Accept both `q` (spec §55 documented param) and `search` (legacy alias).
	search := parseQueryString(r, "q")
	if search == nil {
		search = parseQueryString(r, "search")
	}

	page := int32(parseQueryInt(r, "page", 1))                        // #nosec
	pageSize := int32(parseQueryInt(r, "page_size", DefaultPageSize)) // #nosec

	coupons, err := h.Service.ListCoupons(r.Context(), tenantID, status, couponType, search, page, pageSize)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": coupons})
}

// CreateCouponAdmin handles POST /api/v1/admin/coupons.
func (h *Handlers) CreateCouponAdmin(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

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

	if input.Code == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_field", "code is required")
		return
	}

	if input.CouponType == "" {
		input.CouponType = string(CouponTypeExternal)
	}

	if input.Status == "" {
		input.Status = string(CouponStatusActive)
	}

	coupon, err := h.Service.CreateCoupon(r.Context(), tenantID, user.UserID, input)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": coupon})
}

// GetCouponAdmin handles GET /api/v1/admin/coupons/:id.
func (h *Handlers) GetCouponAdmin(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid coupon ID")
		return
	}

	coupon, err := h.Service.GetCoupon(r.Context(), id, tenantID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": coupon})
}

// UpdateCouponAdmin handles PATCH /api/v1/admin/coupons/:id.
func (h *Handlers) UpdateCouponAdmin(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

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

	if input.Code == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_field", "code is required")
		return
	}

	coupon, err := h.Service.UpdateCoupon(r.Context(), id, tenantID, input)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": coupon})
}

// DeleteCouponAdmin handles DELETE /api/v1/admin/coupons/:id.
func (h *Handlers) DeleteCouponAdmin(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid coupon ID")
		return
	}

	if err := h.Service.DeleteCoupon(r.Context(), id, tenantID); err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusNoContent, nil)
}

// ── Render Helpers ──────────────────────────────────────────────────

// RenderCoupon handles GET /api/v1/render/coupon/:id
// BE-LOW-001: tenant-scoped — the tenant context from the request ensures
// a render request cannot resolve a coupon from another tenant.
func (h *Handlers) RenderCoupon(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid coupon ID")
		return
	}

	rendered, err := h.Service.ResolveCouponForRender(r.Context(), tenantID, id)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": rendered})
}

// RenderAffiliateLink handles GET /api/v1/render/affiliate-link
// BE-LOW-001: tenant-scoped — the tenant context from the request ensures
// a render request cannot resolve a provider from another tenant.
func (h *Handlers) RenderAffiliateLink(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

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

	rendered, err := h.Service.ResolveAffiliateLinkForRender(r.Context(), tenantID, providerID, originalURL)
	if err != nil {
		handleServiceError(w, err)
		return
	}

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
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	page := int32(parseQueryInt(r, "page", 1))                        // #nosec
	pageSize := int32(parseQueryInt(r, "page_size", DefaultPageSize)) // #nosec
	limit, offset := paginate(page, pageSize)

	rows, err := h.Service.store.listAffiliateScanResults(r.Context(), gen.ListAffiliateScanResultsParams{
		TenantID:    tenantID,
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

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": results, "page": page, "page_size": pageSize})
}

// ReviewAffiliateScanResult handles PATCH /admin/affiliate/scan-results/{id}
// BE-LOW-002: uses sqlc-generated query instead of raw SQL.
func (h *Handlers) ReviewAffiliateScanResult(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

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
		TenantID:    tenantID,
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
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

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
		TenantID:   tenantID,
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

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": results, "page": page, "page_size": pageSize})
}

// UpdateProviderAuthorCommission handles PATCH /admin/affiliate/providers/{id}/author-commission
// BE-LOW-002: uses sqlc-generated query instead of raw SQL.
func (h *Handlers) UpdateProviderAuthorCommission(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

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
		TenantID:            tenantID,
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
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

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

	commission, err := h.Service.RecordAffiliateCommission(r.Context(), tenantID, RecordAffiliateCommissionInput{
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
