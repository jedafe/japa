// Package referral — handlers.go provides HTTP handlers for the Referral
// System (Section 60, ADR-078). Covers both the legacy admin program CRUD
// and the V2 referral code, attribution, and conversion tracking routes.
//
// Routes:
//
//	GET    /admin/referral/program          — get referral program settings
//	PUT    /admin/referral/program          — update referral program settings
//	GET    /admin/referral/referrals        — list all referrals (legacy)
//	GET    /admin/referral/referrals/{id}   — get referral by ID (legacy)
//	DELETE /admin/referral/referrals/{id}   — soft-delete referral (legacy)
//	GET    /me/referral                     — get/generate personal code + stats
//	GET    /r/{code}                        — public redirect + click tracking
//	GET    /me/referral/history             — referrer's referral list
//	GET    /admin/referrals                 — all referrals ?status=
//	PATCH  /admin/referrals/{id}/resolve    — resolve flagged (credit or reject)
//	GET    /admin/referrals/stats           — totals by status, fraud rate
//	GET    /admin/referrals/flagged         — flagged queue
package referral

import (
	"encoding/json"
	"log/slog"
	"math"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/clientip"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/tenant"
)

const (
	defaultPageSize int32 = 20
	maxPageSize     int32 = 100
)

// Handlers holds dependencies for referral HTTP handlers.
type Handlers struct {
	Service *Service
}

// NewHandlers creates Handlers with the given Service.
func NewHandlers(service *Service) *Handlers {
	return &Handlers{Service: service}
}

// ---------------------------------------------------------------------------
// Shared helpers
// ---------------------------------------------------------------------------

func (h *Handlers) requireTenant(w http.ResponseWriter, r *http.Request) (pgtype.UUID, bool) {
	t := tenant.FromContext(r.Context())
	if t == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "tenant_unavailable", "No tenant available")
		return pgtype.UUID{}, false
	}

	return t.ID, true
}

// tenantID returns the tenant ID from the request context without writing an
// error response. Used by handlers that need the tenant for a killswitch check
// but must not produce an HTTP error when the tenant is absent (e.g. the public
// redirect handler, which falls back to a plain redirect).
func (h *Handlers) tenantID(r *http.Request) (pgtype.UUID, bool) {
	t := tenant.FromContext(r.Context())
	if t == nil {
		return pgtype.UUID{}, false
	}
	return t.ID, true
}

func parsePageSize(r *http.Request) int32 {
	s := r.URL.Query().Get("page_size")
	if s == "" {
		return defaultPageSize
	}

	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 {
		return defaultPageSize
	}

	if n > int(maxPageSize) {
		return maxPageSize
	}

	return int32(n) //nolint:gosec // G109: bounded by MaxPageSize check above
}

func parseOffset(r *http.Request) int32 {
	if v := r.URL.Query().Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			if n > math.MaxInt32 {
				return defaultPageSize
			}

			return int32(n) //nolint:gosec // G109: bounded by MaxPageSize check above
		}
	}

	return 0
}

func scanUUIDParam(w http.ResponseWriter, r *http.Request, param string) (pgtype.UUID, bool) {
	raw := chi.URLParam(r, param)
	if raw == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_param", param+" is required")
		return pgtype.UUID{}, false
	}
	var id pgtype.UUID
	if err := id.Scan(raw); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_uuid", "Invalid "+param)
		return pgtype.UUID{}, false
	}
	return id, true
}

// ---------------------------------------------------------------------------
// Legacy Admin Handlers — Referral Program
// ---------------------------------------------------------------------------

// GetProgram returns the referral program settings for the tenant.
// GET /admin/referral/program.
func (h *Handlers) GetProgram(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	program, err := h.Service.GetProgram(r.Context(), tenantID)
	if err != nil {
		slog.Error("getting referral program", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to get program")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": program})
}

// UpdateProgram updates the referral program settings.
// PUT /admin/referral/program.
func (h *Handlers) UpdateProgram(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	var input struct {
		ReferrerRewardCents *int32  `json:"referrer_reward_cents,omitempty"`
		RefereeRewardCents  *int32  `json:"referee_reward_cents,omitempty"`
		MaxReferrals        *int32  `json:"max_referrals,omitempty"`
		CookieDays          *int32  `json:"cookie_days,omitempty"`
		IsActive            *bool   `json:"is_active,omitempty"`
		Description         *string `json:"description,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_json", "Invalid JSON body")
		return
	}

	program, err := h.Service.UpdateProgram(r.Context(), tenantID, input.ReferrerRewardCents, input.RefereeRewardCents, input.MaxReferrals, input.CookieDays, input.IsActive, input.Description)
	if err != nil {
		slog.Error("updating referral program", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to update program")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": program})
}

// ListReferrals returns all referrals for the tenant (legacy).
// GET /admin/referral/referrals.
func (h *Handlers) ListReferrals(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	limit := parsePageSize(r)
	offset := parseOffset(r)

	referrals, err := h.Service.ListReferrals(r.Context(), tenantID, limit, offset)
	if err != nil {
		slog.Error("listing referrals", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to list referrals")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": referrals})
}

// GetReferral returns a single referral by ID (legacy).
// GET /admin/referral/referrals/{id}.
func (h *Handlers) GetReferral(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	var id pgtype.UUID
	if err := id.Scan(chi.URLParam(r, "id")); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid referral ID")
		return
	}

	referral, err := h.Service.GetReferral(r.Context(), tenantID, id)
	if err != nil {
		errpkg.WriteError(w, http.StatusNotFound, "not_found", "Referral not found")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": referral})
}

// DeleteReferral soft-deletes a referral by setting status to 'rejected'.
// DELETE /admin/referral/referrals/{id}.
func (h *Handlers) DeleteReferral(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	var id pgtype.UUID
	if err := id.Scan(chi.URLParam(r, "id")); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid referral ID")
		return
	}

	if err := h.Service.DeleteReferral(r.Context(), tenantID, id); err != nil {
		slog.Error("deleting referral", "error", err)
		errpkg.WriteError(w, http.StatusNotFound, "not_found", "Referral not found")
		return
	}

	errpkg.WriteJSON(w, http.StatusNoContent, nil)
}

// ---------------------------------------------------------------------------
// V2 Handlers — Referral codes, attribution, and conversion tracking
// ---------------------------------------------------------------------------

// HandleGetMyCode returns the user's referral code and stats.
// GET /me/referral
func (h *Handlers) HandleGetMyCode(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	u := auth.FromContext(r.Context())
	if u == nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	// Global killswitch (referral.enabled, ADR-078 §60).
	if !h.Service.Enabled(r.Context(), tenantID) {
		errpkg.WriteError(w, http.StatusNotFound, "referral_disabled", "Referral program is not available")
		return
	}

	stats, err := h.Service.GetReferralStats(r.Context(), tenantID, u.UserID)
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "stats_failed", err.Error())
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": stats})
}

// HandleRedirect handles the public /r/:code redirect with click tracking.
// GET /r/{code}
func (h *Handlers) HandleRedirect(w http.ResponseWriter, r *http.Request) {
	code := chi.URLParam(r, "code")
	if code == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_param", "code is required")
		return
	}

	// Global killswitch (referral.enabled, ADR-078 §60). When disabled, the
	// redirect still resolves the code but does not track clicks or set the
	// attribution cookie — the user is simply sent to the homepage.
	if tenantID, ok := h.tenantID(r); ok && !h.Service.Enabled(r.Context(), tenantID) {
		http.Redirect(w, r, "/?ref="+code, http.StatusFound)
		return
	}

	sessionID := r.URL.Query().Get("sid")
	if sessionID == "" {
		sessionID = r.Header.Get("X-Device-Id")
	}
	if sessionID == "" {
		sessionID = "anonymous"
	}

	// Record the click via Asynq fire-and-forget (Rule 8). BUG-005 fix: the
	// previous code spawned a goroutine using r.Context(), which is cancelled
	// the moment the redirect response is written — the Valkey SET was racing
	// with context cancellation and frequently dropped. Enqueuing a task
	// decouples the click recording from the request lifecycle.
	if err := h.Service.EnqueueRecordClick(code, sessionID, clientip.FromRequest(r), r.UserAgent()); err != nil {
		slog.Warn("referral: enqueue record_click failed", "code", code, "error", err)
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "jc_ref",
		Value:    code,
		Path:     "/",
		MaxAge:   30 * 24 * 60 * 60,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	http.Redirect(w, r, "/?ref="+code, http.StatusFound)
}

// HandleMyHistory returns the user's referral history.
// GET /me/referral/history
func (h *Handlers) HandleMyHistory(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	u := auth.FromContext(r.Context())
	if u == nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	// Global killswitch (referral.enabled, ADR-078 §60).
	if !h.Service.Enabled(r.Context(), tenantID) {
		errpkg.WriteError(w, http.StatusNotFound, "referral_disabled", "Referral program is not available")
		return
	}

	status := r.URL.Query().Get("status")
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))

	rows, total, err := h.Service.ListReferralHistory(r.Context(), tenantID, u.UserID, status, page, limit)
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "list_failed", err.Error())
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data":  rows,
		"total": total,
	})
}

// HandleAdminListReferrals returns all referrals for admin oversight.
// GET /admin/referrals
func (h *Handlers) HandleAdminListReferrals(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	if auth.FromContext(r.Context()) == nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	status := r.URL.Query().Get("status")
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))

	rows, total, err := h.Service.AdminListReferrals(r.Context(), tenantID, status, page, limit)
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "list_failed", err.Error())
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data":  rows,
		"total": total,
	})
}

// HandleAdminResolveFlagged resolves a flagged referral.
// PATCH /admin/referrals/{id}/resolve
func (h *Handlers) HandleAdminResolveFlagged(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	if auth.FromContext(r.Context()) == nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	referralID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	var body struct {
		Action string `json:"action"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_json", "Invalid JSON body")
		return
	}
	if body.Action == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_fields", "action is required (credit or reject)")
		return
	}

	if err := h.Service.AdminResolveFlagged(r.Context(), tenantID, referralID, body.Action); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "resolve_failed", err.Error())
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"resolved": true}})
}

// HandleAdminReferralStats returns referral stats by status.
// GET /admin/referrals/stats
func (h *Handlers) HandleAdminReferralStats(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	if auth.FromContext(r.Context()) == nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	stats, err := h.Service.AdminReferralStats(r.Context(), tenantID)
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "stats_failed", err.Error())
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": stats})
}

// HandleAdminFlagged returns the flagged referral queue.
// GET /admin/referrals/flagged
func (h *Handlers) HandleAdminFlagged(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	if auth.FromContext(r.Context()) == nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))

	rows, total, err := h.Service.AdminListFlagged(r.Context(), tenantID, page, limit)
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "list_failed", err.Error())
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data":  rows,
		"total": total,
	})
}
