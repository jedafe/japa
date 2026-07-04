// Package referral — handlers.go provides HTTP handlers for referral admin CRUD.
//
// Routes:
//
//	GET    /admin/referral/program          — get referral program settings
//	PUT    /admin/referral/program          — update referral program settings
//	GET    /admin/referral/referrals        — list all referrals
//	GET    /admin/referral/referrals/{id}   — get referral by ID
package referral

import (
	"encoding/json"
	"log/slog"
	"math"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/tenant"
)

const (
	defaultPageSize int32 = 20
	maxPageSize     int32 = 100
)

// Handlers holds dependencies for referral admin HTTP handlers.
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

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Warn("json encode error", "error", err)
	}
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{
		"error": map[string]string{"code": code, "message": message},
	})
}

func (h *Handlers) requireTenant(w http.ResponseWriter, r *http.Request) (pgtype.UUID, bool) {
	t := tenant.FromContext(r.Context())
	if t == nil {
		writeError(w, http.StatusServiceUnavailable, "tenant_unavailable", "No tenant available")
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

// ---------------------------------------------------------------------------
// Admin Handlers — Referral Program
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
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to get program")

		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": program})
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
		writeError(w, http.StatusBadRequest, "invalid_json", "Invalid JSON body")
		return
	}

	program, err := h.Service.UpdateProgram(r.Context(), tenantID, input.ReferrerRewardCents, input.RefereeRewardCents, input.MaxReferrals, input.CookieDays, input.IsActive, input.Description)
	if err != nil {
		slog.Error("updating referral program", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to update program")

		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": program})
}

// ListReferrals returns all referrals for the tenant.
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
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to list referrals")

		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": referrals})
}

// GetReferral returns a single referral by ID.
// GET /admin/referral/referrals/{id}.
func (h *Handlers) GetReferral(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	var id pgtype.UUID
	if err := id.Scan(chi.URLParam(r, "id")); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid referral ID")
		return
	}

	referral, err := h.Service.GetReferral(r.Context(), tenantID, id)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "Referral not found")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": referral})
}
