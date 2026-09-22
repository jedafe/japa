// Package crscalculator — handlers.go provides the HTTP handlers for the
// Express Entry CRS Calculator routes (§107, ADR-118).
//
// Routes:
//
//	GET   /api/v1/crs/factors                    — factor tables ?version= (public)
//	POST  /api/v1/crs/calculate                  — score a profile (OptionalAuth)
//	GET   /api/v1/crs/draws                      — draw history ?category=&limit= (public)
//	GET   /api/v1/crs/draws/latest               — latest draw per category (public)
//	GET   /api/v1/me/crs-scores                  — score history (auth, crs_calculator.history)
//	GET   /api/v1/admin/crs/rulesets             — manage rulesets (admin)
//	POST  /api/v1/admin/crs/rulesets             — create ruleset + factors (admin)
//	POST  /api/v1/admin/crs/rulesets/generate    — Content Agent draft placeholder (admin)
//	PATCH /api/v1/admin/crs/rulesets/:id/approve — activate + supersede (admin)
//	GET   /api/v1/admin/crs/draws                — manage draws (admin)
//	POST  /api/v1/admin/crs/draws                — create draw (admin)
//	PATCH /api/v1/admin/crs/draws/:id            — update draw (admin)
//
// Ref: japaconnect-engineering/sections/107-crs-calculator.md
//
//	japaconnect-engineering/adr/ADR-118-crs-calculator.md
package crscalculator

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
	errpkg "github.com/jedafe/japa/internal/errors"
)

// Handlers exposes the CRS calculator endpoints.
type Handlers struct {
	service  *Service
	settings *config.Settings
}

// NewHandlers creates CRS calculator handlers bound to the service.
func NewHandlers(service *Service, settings *config.Settings) *Handlers {
	return &Handlers{service: service, settings: settings}
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
	page, limit = 1, 20
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

func unavailable(w http.ResponseWriter) bool {
	errpkg.WriteError(w, http.StatusServiceUnavailable, "crs_calculator_unavailable", "CRS calculator not configured")
	return true
}

// HandleGetFactors handles GET /crs/factors — the factor table for the
// requested ruleset version (active ruleset when ?version= omitted).
// Public.
func (h *Handlers) HandleGetFactors(w http.ResponseWriter, r *http.Request) {
	if h.service == nil {
		unavailable(w)
		return
	}

	resp, err := h.service.GetFactors(r.Context(), r.URL.Query().Get("version"))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "ruleset_not_found", "No matching CRS ruleset")
			return
		}
		slog.Error("crscalculator: get factors failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "factors_failed", "Failed to load factors")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": resp})
}

// HandleCalculate handles POST /crs/calculate — score a profile.
// OptionalAuth: anonymous users get the score; authenticated users also
// persist a score-history row.
func (h *Handlers) HandleCalculate(w http.ResponseWriter, r *http.Request) {
	if h.service == nil {
		unavailable(w)
		return
	}

	var input CalculateRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	var userID *pgtype.UUID
	if u := auth.FromContext(r.Context()); u != nil {
		uid := u.UserID
		userID = &uid
	}

	result, err := h.service.Calculate(r.Context(), userID, input)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "ruleset_not_found", "No active CRS ruleset")
			return
		}
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_profile", err.Error())
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": result})
}

// HandleListDraws handles GET /crs/draws — draw history, newest first.
// Public. Query params: ?category=&limit=
func (h *Handlers) HandleListDraws(w http.ResponseWriter, r *http.Request) {
	if h.service == nil {
		unavailable(w)
		return
	}

	category := r.URL.Query().Get("category")
	limit := int32(50)
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = int32(n) // #nosec
		}
	}

	draws, err := h.service.ListDraws(r.Context(), category, limit)
	if err != nil {
		slog.Error("crscalculator: list draws failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "list_failed", "Failed to list draws")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": draws})
}

// HandleLatestDraws handles GET /crs/draws/latest — the most recent draw
// per category. Public.
func (h *Handlers) HandleLatestDraws(w http.ResponseWriter, r *http.Request) {
	if h.service == nil {
		unavailable(w)
		return
	}

	draws, err := h.service.LatestDraws(r.Context())
	if err != nil {
		slog.Error("crscalculator: latest draws failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "list_failed", "Failed to load latest draws")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": draws})
}

// HandleScoreHistory handles GET /me/crs-scores — the caller's saved
// score history. Auth required (crs_calculator.history at registration).
func (h *Handlers) HandleScoreHistory(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}
	if h.service == nil {
		unavailable(w)
		return
	}

	page, limit := parsePageParams(r)
	history, total, err := h.service.ScoreHistory(r.Context(), user.UserID, page, limit)
	if err != nil {
		slog.Error("crscalculator: score history failed", "error", err, "user_id", user.UserID)
		errpkg.WriteError(w, http.StatusInternalServerError, "history_failed", "Failed to load score history")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data":       history,
		"pagination": map[string]any{"page": page, "limit": limit, "total": total},
	})
}

// HandleAdminListRulesets handles GET /admin/crs/rulesets. Admin-only.
func (h *Handlers) HandleAdminListRulesets(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}
	if h.service == nil {
		unavailable(w)
		return
	}

	rulesets, err := h.service.ListRulesets(r.Context())
	if err != nil {
		slog.Error("crscalculator: list rulesets failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "list_failed", "Failed to list rulesets")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": rulesets})
}

// HandleAdminCreateRuleset handles POST /admin/crs/rulesets — create a
// draft ruleset plus its factor rows. Admin-only.
func (h *Handlers) HandleAdminCreateRuleset(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}
	if h.service == nil {
		unavailable(w)
		return
	}

	var input CreateRulesetRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	ruleset, err := h.service.CreateRuleset(r.Context(), input)
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "create_failed", err.Error())
		return
	}

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": ruleset})
}

// HandleAdminGenerateRuleset handles POST /admin/crs/rulesets/generate —
// Content Agent draft placeholder. Returns 202. Admin-only.
func (h *Handlers) HandleAdminGenerateRuleset(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}
	errpkg.WriteJSON(w, http.StatusAccepted, map[string]any{"data": map[string]any{"status": "queued"}})
}

// HandleAdminApproveRuleset handles PATCH /admin/crs/rulesets/:id/approve
// — activate a ruleset, superseding the prior active one. Admin-only.
func (h *Handlers) HandleAdminApproveRuleset(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}
	if h.service == nil {
		unavailable(w)
		return
	}

	id, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	if err := h.service.ApproveRuleset(r.Context(), id, user.UserID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "ruleset_not_found", "Ruleset not found")
			return
		}
		slog.Error("crscalculator: approve ruleset failed", "id", id, "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "approve_failed", "Failed to approve ruleset")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"status": RulesetActive}})
}

// HandleAdminListDraws handles GET /admin/crs/draws — all draws (any
// status) for admin management.
func (h *Handlers) HandleAdminListDraws(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}
	if h.service == nil {
		unavailable(w)
		return
	}

	draws, err := h.service.ListDraws(r.Context(), "", 500)
	if err != nil {
		slog.Error("crscalculator: admin list draws failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "list_failed", "Failed to list draws")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": draws})
}

// HandleAdminCreateDraw handles POST /admin/crs/draws. Admin-only.
func (h *Handlers) HandleAdminCreateDraw(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}
	if h.service == nil {
		unavailable(w)
		return
	}

	var input DrawRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	draw, err := h.service.CreateDraw(r.Context(), input)
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "create_failed", err.Error())
		return
	}

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": draw})
}

// HandleAdminUpdateDraw handles PATCH /admin/crs/draws/:id. Admin-only.
func (h *Handlers) HandleAdminUpdateDraw(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}
	if h.service == nil {
		unavailable(w)
		return
	}

	id, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	var input DrawRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if err := h.service.UpdateDraw(r.Context(), id, input); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "draw_not_found", "Draw not found")
			return
		}
		slog.Error("crscalculator: update draw failed", "id", id, "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "update_failed", "Failed to update draw")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"id": chi.URLParam(r, "id")}})
}
