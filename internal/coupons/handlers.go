package coupons

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
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
		writeError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return nil, false
	}
	return user, true
}

func (h *Handlers) requireTenant(w http.ResponseWriter, r *http.Request) (pgtype.UUID, bool) {
	t := tenant.FromContext(r.Context())
	if t == nil {
		writeError(w, http.StatusServiceUnavailable, "tenant_unavailable", "Tenant context not available")
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

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if v != nil {
		json.NewEncoder(w).Encode(v)
	}
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{
		"error": map[string]string{"code": code, "message": message},
	})
}

func handleServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrCouponNotFound), errors.Is(err, ErrProviderNotFound):
		writeError(w, http.StatusNotFound, "not_found", err.Error())
	case errors.Is(err, ErrProviderHasCoupons):
		writeError(w, http.StatusConflict, "provider_has_coupons", err.Error())
	case errors.Is(err, ErrCouponAlreadyExists):
		writeError(w, http.StatusConflict, "already_exists", err.Error())
	case errors.Is(err, errpkg.ErrDuplicate):
		writeError(w, http.StatusConflict, "already_exists", "Resource already exists")
	case errors.Is(err, errpkg.ErrRequired):
		writeError(w, http.StatusBadRequest, "validation_error", err.Error())
	default:
		writeError(w, http.StatusInternalServerError, "internal_error", "Something went wrong")
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
	writeJSON(w, http.StatusOK, map[string]any{"data": coupons})
}

// GetCoupon handles GET /api/v1/coupons/:id
func (h *Handlers) GetCoupon(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	id, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid coupon ID")
		return
	}
	coupon, err := h.Service.GetCoupon(r.Context(), id, tenantID)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": coupon})
}

// RecordCouponClick handles POST /api/v1/coupons/:id/click
func (h *Handlers) RecordCouponClick(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid coupon ID")
		return
	}
	var input struct {
		EventType string       `json:"event_type"`
		ContentID *pgtype.UUID `json:"content_id,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}
	if input.EventType != "click" && input.EventType != "copy" {
		writeError(w, http.StatusBadRequest, "invalid_event_type", "event_type must be 'click' or 'copy'")
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
	writeJSON(w, http.StatusAccepted, map[string]any{"data": map[string]string{"status": "recorded"}})
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
	writeJSON(w, http.StatusOK, map[string]any{"data": providers})
}

// CreateProvider handles POST /api/v1/admin/affiliate-providers
func (h *Handlers) CreateProvider(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	var input CreateProviderInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}
	if input.Name == "" {
		writeError(w, http.StatusBadRequest, "missing_field", "name is required")
		return
	}
	if input.Slug == "" {
		writeError(w, http.StatusBadRequest, "missing_field", "slug is required")
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
	writeJSON(w, http.StatusCreated, map[string]any{"data": provider})
}

// GetProvider handles GET /api/v1/admin/affiliate-providers/:id
func (h *Handlers) GetProvider(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	id, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid provider ID")
		return
	}
	provider, err := h.Service.GetProvider(r.Context(), id, tenantID)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": provider})
}

// UpdateProvider handles PATCH /api/v1/admin/affiliate-providers/:id
func (h *Handlers) UpdateProvider(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	id, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid provider ID")
		return
	}
	var input UpdateProviderInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}
	if input.Name == "" {
		writeError(w, http.StatusBadRequest, "missing_field", "name is required")
		return
	}

	provider, err := h.Service.UpdateProvider(r.Context(), id, tenantID, input)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": provider})
}

// DeleteProvider handles DELETE /api/v1/admin/affiliate-providers/:id
func (h *Handlers) DeleteProvider(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	id, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid provider ID")
		return
	}
	if err := h.Service.DeleteProvider(r.Context(), id, tenantID); err != nil {
		handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusNoContent, nil)
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
	writeJSON(w, http.StatusOK, map[string]any{"data": coupons})
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
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}
	if input.Title == "" {
		writeError(w, http.StatusBadRequest, "missing_field", "title is required")
		return
	}
	if input.Code == "" {
		writeError(w, http.StatusBadRequest, "missing_field", "code is required")
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
	writeJSON(w, http.StatusCreated, map[string]any{"data": coupon})
}

// GetCouponAdmin handles GET /api/v1/admin/coupons/:id
func (h *Handlers) GetCouponAdmin(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	id, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid coupon ID")
		return
	}
	coupon, err := h.Service.GetCoupon(r.Context(), id, tenantID)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": coupon})
}

// UpdateCouponAdmin handles PATCH /api/v1/admin/coupons/:id
func (h *Handlers) UpdateCouponAdmin(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	id, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid coupon ID")
		return
	}
	var input UpdateCouponInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}
	if input.Title == "" {
		writeError(w, http.StatusBadRequest, "missing_field", "title is required")
		return
	}
	if input.Code == "" {
		writeError(w, http.StatusBadRequest, "missing_field", "code is required")
		return
	}

	coupon, err := h.Service.UpdateCoupon(r.Context(), id, tenantID, input)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": coupon})
}

// DeleteCouponAdmin handles DELETE /api/v1/admin/coupons/:id
func (h *Handlers) DeleteCouponAdmin(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	id, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid coupon ID")
		return
	}
	if err := h.Service.DeleteCoupon(r.Context(), id, tenantID); err != nil {
		handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusNoContent, nil)
}

// ── Render Helpers ──────────────────────────────────────────────────

// RenderCoupon handles GET /api/v1/render/coupon/:id
func (h *Handlers) RenderCoupon(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid coupon ID")
		return
	}
	rendered, err := h.Service.ResolveCouponForRender(r.Context(), id)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": rendered})
}

// RenderAffiliateLink handles GET /api/v1/render/affiliate-link
func (h *Handlers) RenderAffiliateLink(w http.ResponseWriter, r *http.Request) {
	providerIDStr := r.URL.Query().Get("provider_id")
	originalURL := r.URL.Query().Get("url")
	if providerIDStr == "" || originalURL == "" {
		writeError(w, http.StatusBadRequest, "missing_params", "url and provider_id are required")
		return
	}
	var providerID pgtype.UUID
	if err := providerID.Scan(providerIDStr); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_provider_id", "Invalid provider ID")
		return
	}
	rendered, err := h.Service.ResolveAffiliateLinkForRender(r.Context(), providerID, originalURL)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": rendered})
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
func (h *Handlers) ListAffiliateScanResults(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	rows, err := h.Service.db.Reader.Query(r.Context(),
		"SELECT id, content_id, url, domain, detection_method, action_taken, created_at "+
			"FROM content_affiliate_scan_results WHERE tenant_id = $1 ORDER BY created_at DESC LIMIT 50", tenantID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "query_error", "Failed to query scan results")
		return
	}
	defer rows.Close()
	var results []map[string]any
	for rows.Next() {
		var id, contentID pgtype.UUID
		var url, domain, method, action string
		var createdAt pgtype.Timestamptz
		if err := rows.Scan(&id, &contentID, &url, &domain, &method, &action, &createdAt); err != nil {
			continue
		}
		results = append(results, map[string]any{
			"id": id.String(), "content_id": contentID.String(), "url": url,
			"domain": domain, "detection_method": method, "action_taken": action,
			"created_at": createdAt.Time,
		})
	}
	if results == nil {
		results = []map[string]any{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": results})
}

// ReviewAffiliateScanResult handles PATCH /admin/affiliate/scan-results/{id}
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
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid scan result ID")
		return
	}
	var input struct {
		Action string  `json:"action"`
		Notes  *string `json:"notes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}
	var reviewerID pgtype.UUID
	reviewerID.Scan(user.UserID)
	h.Service.db.Writer.Exec(r.Context(),
		"UPDATE content_affiliate_scan_results SET action_taken = $1, reviewed_by = $2, reviewed_at = now(), notes = $3 WHERE id = $4 AND tenant_id = $5",
		input.Action, reviewerID, input.Notes, id, tenantID)
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]bool{"updated": true}})
}

// ListAffiliateCommissions handles GET /admin/affiliate/commissions
func (h *Handlers) ListAffiliateCommissions(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	rows, err := h.Service.db.Reader.Query(r.Context(),
		"SELECT ac.id, ac.content_id, ac.author_user_id, ac.external_commission, ac.author_share, "+
			"ac.author_commission_pct, ac.event_description, ac.credited, ac.created_at, ap.name, u.username "+
			"FROM affiliate_content_commissions ac "+
			"LEFT JOIN affiliate_providers ap ON ap.id = ac.provider_id "+
			"LEFT JOIN users u ON u.id = ac.author_user_id "+
			"WHERE ac.tenant_id = $1 ORDER BY ac.created_at DESC LIMIT 50", tenantID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "query_error", "Failed to query commissions")
		return
	}
	defer rows.Close()
	var results []map[string]any
	for rows.Next() {
		var id, contentID, authorUserID pgtype.UUID
		var extComm, authorShare, pct float64
		var desc *string
		var credited bool
		var createdAt pgtype.Timestamptz
		var providerName, username *string
		if err := rows.Scan(&id, &contentID, &authorUserID,
			&extComm, &authorShare, &pct, &desc, &credited, &createdAt, &providerName, &username); err != nil {
			continue
		}
		results = append(results, map[string]any{
			"id": id.String(), "content_id": contentID.String(),
			"author_user_id": authorUserID.String(), "author_username": username,
			"provider_name": providerName,
			"external_commission": extComm, "author_share": authorShare,
			"author_commission_pct": pct, "credited": credited, "created_at": createdAt.Time,
		})
	}
	if results == nil {
		results = []map[string]any{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": results})
}

// UpdateProviderAuthorCommission handles PATCH /admin/affiliate/providers/{id}/author-commission
func (h *Handlers) UpdateProviderAuthorCommission(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	id, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid provider ID")
		return
	}
	var input struct {
		AuthorCommissionPct float64 `json:"author_commission_pct"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}
	if input.AuthorCommissionPct < 0 || input.AuthorCommissionPct > 100 {
		writeError(w, http.StatusBadRequest, "invalid_pct", "Percentage must be 0-100")
		return
	}
	h.Service.db.Writer.Exec(r.Context(),
		"UPDATE affiliate_providers SET author_commission_pct = $1 WHERE id = $2 AND tenant_id = $3",
		input.AuthorCommissionPct, id, tenantID)
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]bool{"updated": true}})
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
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	var providerID pgtype.UUID
	if err := providerID.Scan(input.ProviderID); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_provider_id", "Invalid provider_id")
		return
	}

	var contentID pgtype.UUID
	if err := contentID.Scan(input.ContentID); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_content_id", "Invalid content_id")
		return
	}

	if input.ExternalCommission <= 0 {
		writeError(w, http.StatusBadRequest, "invalid_commission", "External commission must be positive")
		return
	}

	commission, err := h.Service.RecordAffiliateCommission(r.Context(), tenantID, RecordAffiliateCommissionInput{
		ProviderID:         providerID,
		ContentID:          contentID,
		ExternalCommission: input.ExternalCommission,
		EventDescription:   input.EventDescription,
	})
	if err != nil {
		errpkg.CaptureException(err)
		writeError(w, http.StatusInternalServerError, "record_failed", err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{"data": commission})
}
