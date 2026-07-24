package coupons

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/httpx"
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
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return nil, false
	}
	return user, true
}

func (h *Handlers) requireTenant(w http.ResponseWriter, r *http.Request) (pgtype.UUID, bool) {
	t := tenant.FromContext(r.Context())
	if t == nil {
		httpx.WriteError(w, http.StatusServiceUnavailable, "tenant_unavailable", "Tenant context not available")
		return pgtype.UUID{}, false
	}
	return t.ID, true
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
		httpx.WriteError(w, http.StatusNotFound, "not_found", err.Error())
	case errors.Is(err, ErrProviderHasCoupons):
		httpx.WriteError(w, http.StatusConflict, "provider_has_coupons", err.Error())
	case errors.Is(err, ErrCouponAlreadyExists):
		httpx.WriteError(w, http.StatusConflict, "already_exists", err.Error())
	case errors.Is(err, errpkg.ErrDuplicate):
		httpx.WriteError(w, http.StatusConflict, "already_exists", "Resource already exists")
	case errors.Is(err, errpkg.ErrRequired):
		httpx.WriteError(w, http.StatusBadRequest, "validation_error", err.Error())
	default:
		httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "Something went wrong")
	}
}

// ── Public Coupon Routes ────────────────────────────────────────────

// ListPublicCoupons handles GET /api/v1/coupons
func (h *Handlers) ListPublicCoupons(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	providerID := parseQueryUUID(r, "provider_id")
	couponType := parseQueryString(r, "coupon_type")
	search := parseQueryString(r, "search")
	page := int32(parseQueryInt(r, "page", 1))
	pageSize := int32(parseQueryInt(r, "page_size", DefaultPageSize))

	coupons, err := h.Service.ListActiveCoupons(r.Context(), tenantID, providerID, couponType, search, page, pageSize)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": coupons})
}

// GetCoupon handles GET /api/v1/coupons/:id
func (h *Handlers) GetCoupon(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	id, err := httpx.ParseUUIDParam(r, "id")
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid coupon ID")
		return
	}
	coupon, err := h.Service.GetCoupon(r.Context(), id, tenantID)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": coupon})
}

// RecordCouponClick handles POST /api/v1/coupons/:id/click
func (h *Handlers) RecordCouponClick(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.ParseUUIDParam(r, "id")
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid coupon ID")
		return
	}
	var input struct {
		EventType string       `json:"event_type"`
		ContentID *pgtype.UUID `json:"content_id,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}
	if input.EventType != "click" && input.EventType != "copy" {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_event_type", "event_type must be 'click' or 'copy'")
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
	httpx.WriteJSON(w, http.StatusAccepted, map[string]any{"data": map[string]string{"status": "recorded"}})
}

// ── Admin Affiliate Provider Routes ─────────────────────────────────

// ListProviders handles GET /api/v1/admin/affiliate-providers
func (h *Handlers) ListProviders(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	page := int32(parseQueryInt(r, "page", 1))
	pageSize := int32(parseQueryInt(r, "page_size", DefaultPageSize))

	providers, err := h.Service.ListProviders(r.Context(), tenantID, page, pageSize)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": providers})
}

// CreateProvider handles POST /api/v1/admin/affiliate-providers
func (h *Handlers) CreateProvider(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	var input CreateProviderInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}
	if input.Name == "" {
		httpx.WriteError(w, http.StatusBadRequest, "missing_field", "name is required")
		return
	}
	if input.Slug == "" {
		httpx.WriteError(w, http.StatusBadRequest, "missing_field", "slug is required")
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
	httpx.WriteJSON(w, http.StatusCreated, map[string]any{"data": provider})
}

// GetProvider handles GET /api/v1/admin/affiliate-providers/:id
func (h *Handlers) GetProvider(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	id, err := httpx.ParseUUIDParam(r, "id")
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid provider ID")
		return
	}
	provider, err := h.Service.GetProvider(r.Context(), id, tenantID)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": provider})
}

// UpdateProvider handles PATCH /api/v1/admin/affiliate-providers/:id
func (h *Handlers) UpdateProvider(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	id, err := httpx.ParseUUIDParam(r, "id")
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid provider ID")
		return
	}
	var input UpdateProviderInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}
	if input.Name == "" {
		httpx.WriteError(w, http.StatusBadRequest, "missing_field", "name is required")
		return
	}

	provider, err := h.Service.UpdateProvider(r.Context(), id, tenantID, input)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": provider})
}

// DeleteProvider handles DELETE /api/v1/admin/affiliate-providers/:id
func (h *Handlers) DeleteProvider(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	id, err := httpx.ParseUUIDParam(r, "id")
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid provider ID")
		return
	}
	if err := h.Service.DeleteProvider(r.Context(), id, tenantID); err != nil {
		handleServiceError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusNoContent, nil)
}

// ── Admin Coupon Routes ─────────────────────────────────────────────

// ListCouponsAdmin handles GET /api/v1/admin/coupons
func (h *Handlers) ListCouponsAdmin(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	status := parseQueryString(r, "status")
	couponType := parseQueryString(r, "coupon_type")
	search := parseQueryString(r, "search")
	page := int32(parseQueryInt(r, "page", 1))
	pageSize := int32(parseQueryInt(r, "page_size", DefaultPageSize))

	coupons, err := h.Service.ListCoupons(r.Context(), tenantID, status, couponType, search, page, pageSize)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": coupons})
}

// CreateCouponAdmin handles POST /api/v1/admin/coupons
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
		httpx.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}
	if input.Title == "" {
		httpx.WriteError(w, http.StatusBadRequest, "missing_field", "title is required")
		return
	}
	if input.Code == "" {
		httpx.WriteError(w, http.StatusBadRequest, "missing_field", "code is required")
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
	httpx.WriteJSON(w, http.StatusCreated, map[string]any{"data": coupon})
}

// GetCouponAdmin handles GET /api/v1/admin/coupons/:id
func (h *Handlers) GetCouponAdmin(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	id, err := httpx.ParseUUIDParam(r, "id")
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid coupon ID")
		return
	}
	coupon, err := h.Service.GetCoupon(r.Context(), id, tenantID)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": coupon})
}

// UpdateCouponAdmin handles PATCH /api/v1/admin/coupons/:id
func (h *Handlers) UpdateCouponAdmin(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	id, err := httpx.ParseUUIDParam(r, "id")
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid coupon ID")
		return
	}
	var input UpdateCouponInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}
	if input.Title == "" {
		httpx.WriteError(w, http.StatusBadRequest, "missing_field", "title is required")
		return
	}
	if input.Code == "" {
		httpx.WriteError(w, http.StatusBadRequest, "missing_field", "code is required")
		return
	}

	coupon, err := h.Service.UpdateCoupon(r.Context(), id, tenantID, input)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": coupon})
}

// DeleteCouponAdmin handles DELETE /api/v1/admin/coupons/:id
func (h *Handlers) DeleteCouponAdmin(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	id, err := httpx.ParseUUIDParam(r, "id")
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid coupon ID")
		return
	}
	if err := h.Service.DeleteCoupon(r.Context(), id, tenantID); err != nil {
		handleServiceError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusNoContent, nil)
}

// ── Render Helpers ──────────────────────────────────────────────────

// RenderCoupon handles GET /api/v1/render/coupon/:id
func (h *Handlers) RenderCoupon(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.ParseUUIDParam(r, "id")
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid coupon ID")
		return
	}
	rendered, err := h.Service.ResolveCouponForRender(r.Context(), id)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": rendered})
}

// RenderAffiliateLink handles GET /api/v1/render/affiliate-link
func (h *Handlers) RenderAffiliateLink(w http.ResponseWriter, r *http.Request) {
	providerIDStr := r.URL.Query().Get("provider_id")
	originalURL := r.URL.Query().Get("url")
	if providerIDStr == "" || originalURL == "" {
		httpx.WriteError(w, http.StatusBadRequest, "missing_params", "url and provider_id are required")
		return
	}
	var providerID pgtype.UUID
	if err := providerID.Scan(providerIDStr); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_provider_id", "Invalid provider ID")
		return
	}
	rendered, err := h.Service.ResolveAffiliateLinkForRender(r.Context(), providerID, originalURL)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": rendered})
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
