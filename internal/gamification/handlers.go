// Package gamification — handlers.go provides the HTTP handlers for the
// gamification routes: public badge/challenge/leaderboard listing, user
// badge/challenge views, and admin badge/challenge management.
//
// Routes (Section 64, ADR-082):
//
//	GET   /api/v1/gamification/badges              — all badge definitions
//	GET   /api/v1/gamification/challenges          — active challenges
//	GET   /api/v1/gamification/leaderboard         — ?scope=&period=
//	GET   /api/v1/me/badges                        — user's earned badges
//	GET   /api/v1/me/challenges                    — user's challenge progress
//	GET   /api/v1/users/:username/badges           — public badge display
//	GET   /api/v1/admin/gamification/badges        — all definitions
//	POST  /api/v1/admin/gamification/badges        — create badge
//	PATCH /api/v1/admin/gamification/badges/:id    — edit badge
//	POST  /api/v1/admin/gamification/badges/:id/award — manually award
//	GET   /api/v1/admin/gamification/challenges    — all challenges
//	POST  /api/v1/admin/gamification/challenges    — create challenge
//
// Ref: japaconnect-engineering/sections/64-gamification.md
//      japaconnect-engineering/adr/ADR-082-advanced-gamification.md
package gamification

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/config"
	"github.com/jedafe/japa/internal/tenant"
)

// Handlers exposes the gamification HTTP endpoints.
type Handlers struct {
	service  *Service
	settings *config.Settings
}

// NewHandlers creates gamification handlers bound to the provided service.
func NewHandlers(service *Service, settings *config.Settings) *Handlers {
	return &Handlers{service: service, settings: settings}
}

// ---------------------------------------------------------------------------
// Shared helpers (same pattern as rag/handlers.go)
// ---------------------------------------------------------------------------

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("gamification: encode response", "error", err)
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

func scanUUIDParam(w http.ResponseWriter, r *http.Request, name string) (pgtype.UUID, bool) {
	var id pgtype.UUID
	if err := id.Scan(chi.URLParam(r, name)); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid "+name)
		return pgtype.UUID{}, false
	}
	return id, true
}

// parsePageParams extracts limit and offset from query string, with defaults.
func parsePageParams(r *http.Request) (int32, int32) {
	limit := int32(50)
	offset := int32(0)
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 200 {
			limit = int32(n)
		}
	}
	if v := r.URL.Query().Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			offset = int32(n)
		}
	}
	return limit, offset
}

// ---------------------------------------------------------------------------
// Public handlers
// ---------------------------------------------------------------------------

// HandleListBadges handles GET /api/v1/gamification/badges — all active badge
// definitions. Public route (gamification.view permission at registration site).
func (h *Handlers) HandleListBadges(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	if h.service == nil {
		writeError(w, http.StatusServiceUnavailable, "gamification_unavailable", "Gamification service not configured")
		return
	}

	badges, err := h.service.ListBadges(r.Context(), tenantID)
	if err != nil {
		slog.Error("gamification: list badges failed", "error", err)
		writeError(w, http.StatusInternalServerError, "badges_fetch_failed", "Failed to fetch badges")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": badges,
	})
}

// HandleListChallenges handles GET /api/v1/gamification/challenges — active
// challenges within their date range. Public route.
func (h *Handlers) HandleListChallenges(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	if h.service == nil {
		writeError(w, http.StatusServiceUnavailable, "gamification_unavailable", "Gamification service not configured")
		return
	}

	challenges, err := h.service.ListActiveChallenges(r.Context(), tenantID)
	if err != nil {
		slog.Error("gamification: list challenges failed", "error", err)
		writeError(w, http.StatusInternalServerError, "challenges_fetch_failed", "Failed to fetch challenges")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": challenges,
	})
}

// HandleGetLeaderboard handles GET /api/v1/gamification/leaderboard — the
// points leaderboard. Query params: scope (global|space:{id}), period
// (all_time|monthly|weekly). Public route.
func (h *Handlers) HandleGetLeaderboard(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	if h.service == nil {
		writeError(w, http.StatusServiceUnavailable, "gamification_unavailable", "Gamification service not configured")
		return
	}

	scope := r.URL.Query().Get("scope")
	if scope == "" {
		scope = ScopeGlobal
	}
	period := r.URL.Query().Get("period")
	if period == "" {
		period = PeriodAllTime
	}

	if _, _, ok := ValidateLeaderboardScope(scope); !ok {
		writeError(w, http.StatusBadRequest, "invalid_scope", "Scope must be 'global' or 'space:{id}'")
		return
	}
	if !ValidateLeaderboardPeriod(period) {
		writeError(w, http.StatusBadRequest, "invalid_period", "Period must be 'all_time', 'monthly', or 'weekly'")
		return
	}

	entries, err := h.service.GetLeaderboard(r.Context(), tenantID, scope, period)
	if err != nil {
		slog.Error("gamification: get leaderboard failed", "error", err)
		writeError(w, http.StatusInternalServerError, "leaderboard_fetch_failed", "Failed to fetch leaderboard")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": entries,
	})
}

// HandleMyBadges handles GET /api/v1/me/badges — the authenticated user's
// earned badges. Auth required.
func (h *Handlers) HandleMyBadges(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	if h.service == nil {
		writeError(w, http.StatusServiceUnavailable, "gamification_unavailable", "Gamification service not configured")
		return
	}

	badges, err := h.service.ListUserBadges(r.Context(), tenantID, user.UserID)
	if err != nil {
		slog.Error("gamification: my badges failed", "error", err)
		writeError(w, http.StatusInternalServerError, "badges_fetch_failed", "Failed to fetch badges")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": badges,
	})
}

// HandleMyChallenges handles GET /api/v1/me/challenges — the authenticated
// user's challenge progress. Auth required.
func (h *Handlers) HandleMyChallenges(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	if h.service == nil {
		writeError(w, http.StatusServiceUnavailable, "gamification_unavailable", "Gamification service not configured")
		return
	}

	progress, err := h.service.ListUserChallengeProgress(r.Context(), tenantID, user.UserID)
	if err != nil {
		slog.Error("gamification: my challenges failed", "error", err)
		writeError(w, http.StatusInternalServerError, "challenges_fetch_failed", "Failed to fetch challenges")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": progress,
	})
}

// HandleUserBadges handles GET /api/v1/users/:username/badges — public badge
// display for a user's profile. Public route.
func (h *Handlers) HandleUserBadges(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	username := chi.URLParam(r, "username")
	if username == "" {
		writeError(w, http.StatusBadRequest, "missing_username", "Username is required")
		return
	}

	if h.service == nil {
		writeError(w, http.StatusServiceUnavailable, "gamification_unavailable", "Gamification service not configured")
		return
	}

	badges, err := h.service.ListUserBadgesByUsername(r.Context(), tenantID, username)
	if err != nil {
		slog.Error("gamification: user badges failed", "error", err, "username", username)
		writeError(w, http.StatusInternalServerError, "badges_fetch_failed", "Failed to fetch badges")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": badges,
	})
}

// ---------------------------------------------------------------------------
// Admin handlers
// ---------------------------------------------------------------------------

// HandleAdminListBadges handles GET /api/v1/admin/gamification/badges — all
// badge definitions (including inactive). Admin route
// (gamification.manage_badges permission).
func (h *Handlers) HandleAdminListBadges(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}

	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	if h.service == nil {
		writeError(w, http.StatusServiceUnavailable, "gamification_unavailable", "Gamification service not configured")
		return
	}

	limit, offset := parsePageParams(r)
	badges, total, err := h.service.ListAllBadges(r.Context(), tenantID, limit, offset)
	if err != nil {
		slog.Error("gamification: admin list badges failed", "error", err)
		writeError(w, http.StatusInternalServerError, "badges_fetch_failed", "Failed to fetch badges")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": badges,
		"pagination": map[string]any{
			"total":  total,
			"limit":  limit,
			"offset": offset,
		},
	})
}

// HandleAdminCreateBadge handles POST /api/v1/admin/gamification/badges —
// create a new badge definition. Admin route (gamification.manage_badges).
func (h *Handlers) HandleAdminCreateBadge(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}

	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	if h.service == nil {
		writeError(w, http.StatusServiceUnavailable, "gamification_unavailable", "Gamification service not configured")
		return
	}

	var in CreateBadgeInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	badge, err := h.service.CreateBadge(r.Context(), tenantID, in)
	if err != nil {
		slog.Error("gamification: admin create badge failed", "error", err)
		writeError(w, http.StatusBadRequest, "badge_create_failed", err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"data": badge,
	})
}

// HandleAdminUpdateBadge handles PATCH /api/v1/admin/gamification/badges/:id —
// update a badge definition's mutable fields. Admin route
// (gamification.manage_badges).
func (h *Handlers) HandleAdminUpdateBadge(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}

	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	badgeID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	if h.service == nil {
		writeError(w, http.StatusServiceUnavailable, "gamification_unavailable", "Gamification service not configured")
		return
	}

	var in UpdateBadgeInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	badge, err := h.service.UpdateBadge(r.Context(), tenantID, badgeID, in)
	if err != nil {
		slog.Error("gamification: admin update badge failed", "error", err)
		writeError(w, http.StatusBadRequest, "badge_update_failed", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": badge,
	})
}

// HandleAdminAwardBadge handles POST /api/v1/admin/gamification/badges/:id/award
// — manually award a badge to a user. Admin route (gamification.award_badge).
// Body: { "user_id": "<uuid>" }
func (h *Handlers) HandleAdminAwardBadge(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	badgeID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	if h.service == nil {
		writeError(w, http.StatusServiceUnavailable, "gamification_unavailable", "Gamification service not configured")
		return
	}

	var body struct {
		UserID string `json:"user_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}
	if body.UserID == "" {
		writeError(w, http.StatusBadRequest, "missing_user_id", "user_id is required")
		return
	}

	targetUserID, err := scanUUID(body.UserID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_user_id", "Invalid user_id")
		return
	}

	badge, err := h.service.AwardBadge(r.Context(), tenantID, badgeID, targetUserID, user.UserID)
	if err != nil {
		slog.Error("gamification: admin award badge failed", "error", err)
		writeError(w, http.StatusBadRequest, "badge_award_failed", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": badge,
	})
}

// HandleAdminListChallenges handles GET /api/v1/admin/gamification/challenges —
// all challenges (including inactive). Admin route
// (gamification.manage_challenges).
func (h *Handlers) HandleAdminListChallenges(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}

	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	if h.service == nil {
		writeError(w, http.StatusServiceUnavailable, "gamification_unavailable", "Gamification service not configured")
		return
	}

	limit, offset := parsePageParams(r)
	challenges, total, err := h.service.ListAllChallenges(r.Context(), tenantID, limit, offset)
	if err != nil {
		slog.Error("gamification: admin list challenges failed", "error", err)
		writeError(w, http.StatusInternalServerError, "challenges_fetch_failed", "Failed to fetch challenges")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": challenges,
		"pagination": map[string]any{
			"total":  total,
			"limit":  limit,
			"offset": offset,
		},
	})
}

// HandleAdminCreateChallenge handles POST /api/v1/admin/gamification/challenges
// — create a new challenge definition. Admin route
// (gamification.manage_challenges).
func (h *Handlers) HandleAdminCreateChallenge(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}

	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	if h.service == nil {
		writeError(w, http.StatusServiceUnavailable, "gamification_unavailable", "Gamification service not configured")
		return
	}

	var in CreateChallengeInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	challenge, err := h.service.CreateChallenge(r.Context(), tenantID, in)
	if err != nil {
		slog.Error("gamification: admin create challenge failed", "error", err)
		writeError(w, http.StatusBadRequest, "challenge_create_failed", err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"data": challenge,
	})
}
