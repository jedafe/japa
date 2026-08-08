// Package japastage — handlers.go provides the HTTP handlers for the Japa
// Stage Progress Profile routes (§87, ADR-095).
//
// Routes:
//
//	GET   /api/v1/users/:username/japa-stage — public stage view
//	PATCH /api/v1/me/japa-stage              — update own stage (auth, profile.japa_stage.edit)
//	GET   /api/v1/members/by-stage           — directory by stage (?stage=)
//
// Ref: japaconnect-engineering/sections/87-japa-stage-progress-profiles.md
//      japaconnect-engineering/adr/ADR-095-japa-stage-progress-profiles.md
package japastage

import (
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
	"github.com/jedafe/japa/internal/tenant"
)

// Handlers exposes the japa stage profile endpoints.
type Handlers struct {
	service  *Service
	settings *config.Settings
}

// NewHandlers creates japa stage handlers bound to the provided service.
func NewHandlers(service *Service, settings *config.Settings) *Handlers {
	return &Handlers{service: service, settings: settings}
}

// ---------------------------------------------------------------------------
// Shared helpers (same pattern as story/handlers.go)
// ---------------------------------------------------------------------------

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("japastage: encode response", "error", err)
	}
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{
		"error": map[string]string{"code": code, "message": message},
	})
}

func requireTenant(w http.ResponseWriter, r *http.Request) (pgtype.UUID, bool) {
	t := tenant.FromContext(r.Context())
	if t == nil {
		writeError(w, http.StatusServiceUnavailable, "tenant_unavailable", "No tenant available")
		return pgtype.UUID{}, false
	}
	return t.ID, true
}

func requireUser(w http.ResponseWriter, r *http.Request) (*auth.AuthenticatedUser, bool) {
	u := auth.FromContext(r.Context())
	if u == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return nil, false
	}
	return u, true
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
// Public handlers
// ---------------------------------------------------------------------------

// HandleGetByUsername handles GET /users/:username/japa-stage — public stage
// view. Returns an empty data object (stage omitted) when the profile is not
// public, so anonymous browsers simply see no stage.
func (h *Handlers) HandleGetByUsername(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	if h.service == nil || !h.service.Enabled(r.Context(), tenantID) {
		writeError(w, http.StatusNotFound, "japa_stage_disabled", "Japa stage profiles are not available")
		return
	}

	username := chi.URLParam(r, "username")
	if username == "" {
		writeError(w, http.StatusBadRequest, "missing_username", "Username is required")
		return
	}

	profile, err := h.service.GetByUsername(r.Context(), tenantID, username)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "profile_not_found", "Profile not found")
			return
		}
		slog.Error("japastage: get by username failed", "username", username, "error", err)
		writeError(w, http.StatusInternalServerError, "get_failed", "Failed to get japa stage profile")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": profile})
}

// HandleListByStage handles GET /members/by-stage — public directory of
// members filtered by stage. Query params: ?stage=&page=&limit=
func (h *Handlers) HandleListByStage(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	if h.service == nil || !h.service.Enabled(r.Context(), tenantID) {
		writeError(w, http.StatusNotFound, "japa_stage_disabled", "Japa stage profiles are not available")
		return
	}

	stage := r.URL.Query().Get("stage")
	if stage == "" {
		writeError(w, http.StatusBadRequest, "missing_stage", "Stage query parameter is required")
		return
	}
	if !ValidStage(stage) {
		writeError(w, http.StatusBadRequest, "invalid_stage", "Invalid stage; must be one of: "+JoinStages())
		return
	}

	page, limit := parsePageParams(r)

	members, total, err := h.service.ListByStage(r.Context(), tenantID, stage, page, limit)
	if err != nil {
		slog.Error("japastage: list by stage failed", "stage", stage, "error", err)
		writeError(w, http.StatusInternalServerError, "list_failed", "Failed to list members by stage")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": members,
		"pagination": map[string]any{
			"page":  page,
			"limit": limit,
			"total": total,
		},
	})
}

// ---------------------------------------------------------------------------
// Authenticated handlers (update own stage — profile.japa_stage.edit)
// ---------------------------------------------------------------------------

// HandleUpdateOwn handles PATCH /me/japa-stage — update the authenticated
// user's own japa stage and destination space. Auth required
// (profile.japa_stage.edit permission checked at route registration).
func (h *Handlers) HandleUpdateOwn(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	if h.service == nil || !h.service.Enabled(r.Context(), tenantID) {
		writeError(w, http.StatusNotFound, "japa_stage_disabled", "Japa stage profiles are not available")
		return
	}

	var input UpdateStageInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if input.Stage == "" {
		writeError(w, http.StatusBadRequest, "missing_stage", "Stage is required")
		return
	}
	if !ValidStage(input.Stage) {
		writeError(w, http.StatusBadRequest, "invalid_stage", "Invalid stage; must be one of: "+JoinStages())
		return
	}

	profile, err := h.service.UpdateOwn(r.Context(), user.UserID, tenantID, input)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "profile_not_found", "Profile not found")
			return
		}
		slog.Error("japastage: update own failed", "error", err)
		writeError(w, http.StatusInternalServerError, "update_failed", "Failed to update japa stage")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": profile})
}
