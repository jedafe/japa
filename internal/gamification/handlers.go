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
//
//	japaconnect-engineering/adr/ADR-082-advanced-gamification.md
package gamification

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/config"
	errpkg "github.com/jedafe/japa/internal/errors"
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

// parsePageParams extracts limit and offset from query string, with defaults.
func parsePageParams(r *http.Request) (int32, int32) {
	limit := int32(50)
	offset := int32(0)

	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 200 {
			limit = int32(n) // #nosec
		}
	}

	if v := r.URL.Query().Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			offset = int32(n) // #nosec
		}
	}

	return limit, offset
}

// writeDisabled writes the 404 gamification_disabled envelope when a
// gamification surface is toggled off via system_settings (ADR-042 — a
// disabled feature is 404, not 403 or an empty 200). Returns true when the
// error was the disabled sentinel and a response was written.
func writeDisabled(w http.ResponseWriter, err error) bool {
	if errors.Is(err, ErrGamificationDisabled) {
		errpkg.WriteError(w, http.StatusNotFound, "gamification_disabled", "This gamification feature is currently disabled")
		return true
	}

	return false
}

// ---------------------------------------------------------------------------
// Public handlers
// ---------------------------------------------------------------------------

// HandleListBadges handles GET /api/v1/gamification/badges — all active badge
// definitions. Public route (gamification.view permission at registration site).
func (h *Handlers) HandleListBadges(w http.ResponseWriter, r *http.Request) {

	if h.service == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "gamification_unavailable", "Gamification service not configured")
		return
	}

	badges, err := h.service.ListBadges(r.Context())
	if err != nil {
		if writeDisabled(w, err) {
			return
		}

		slog.Error("gamification: list badges failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "badges_fetch_failed", "Failed to fetch badges")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": badges,
	})
}

// HandleListChallenges handles GET /api/v1/gamification/challenges — active
// challenges within their date range. Public route.
func (h *Handlers) HandleListChallenges(w http.ResponseWriter, r *http.Request) {

	if h.service == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "gamification_unavailable", "Gamification service not configured")
		return
	}

	challenges, err := h.service.ListActiveChallenges(r.Context())
	if err != nil {
		if writeDisabled(w, err) {
			return
		}

		slog.Error("gamification: list challenges failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "challenges_fetch_failed", "Failed to fetch challenges")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": challenges,
	})
}

// HandleGetLeaderboard handles GET /api/v1/gamification/leaderboard — the
// points leaderboard. Query params: scope (global|space:{id}), period
// (all_time|monthly|weekly). Public route.
func (h *Handlers) HandleGetLeaderboard(w http.ResponseWriter, r *http.Request) {

	if h.service == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "gamification_unavailable", "Gamification service not configured")
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

	// Optional limit — the service caches the full top-N and slices to the
	// requested size (1..defaultLeaderboardTopN).
	limit := int32(defaultLeaderboardTopN)
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= defaultLeaderboardTopN {
			limit = int32(n) // #nosec
		}
	}

	if _, _, ok := ValidateLeaderboardScope(scope); !ok {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_scope", "Scope must be 'global' or 'space:{uuid}'")
		return
	}

	if !ValidateLeaderboardPeriod(period) {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_period", "Period must be 'all_time', 'monthly', or 'weekly'")
		return
	}

	entries, err := h.service.GetLeaderboard(r.Context(), scope, period, limit)
	if err != nil {
		if writeDisabled(w, err) {
			return
		}

		slog.Error("gamification: get leaderboard failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "leaderboard_fetch_failed", "Failed to fetch leaderboard")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
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

	if h.service == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "gamification_unavailable", "Gamification service not configured")
		return
	}

	badges, err := h.service.ListUserBadges(r.Context(), user.UserID)
	if err != nil {
		if writeDisabled(w, err) {
			return
		}

		slog.Error("gamification: my badges failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "badges_fetch_failed", "Failed to fetch badges")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
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

	if h.service == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "gamification_unavailable", "Gamification service not configured")
		return
	}

	progress, err := h.service.ListUserChallengeProgress(r.Context(), user.UserID)
	if err != nil {
		if writeDisabled(w, err) {
			return
		}

		slog.Error("gamification: my challenges failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "challenges_fetch_failed", "Failed to fetch challenges")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": progress,
	})
}

// HandleUserBadges handles GET /api/v1/users/:username/badges — public badge
// display for a user's profile. Public route.
func (h *Handlers) HandleUserBadges(w http.ResponseWriter, r *http.Request) {

	username := chi.URLParam(r, "username")
	if username == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_username", "Username is required")
		return
	}

	if h.service == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "gamification_unavailable", "Gamification service not configured")
		return
	}

	badges, err := h.service.ListUserBadgesByUsername(r.Context(), username)
	if err != nil {
		if writeDisabled(w, err) {
			return
		}

		slog.Error("gamification: user badges failed", "error", err, "username", username)
		errpkg.WriteError(w, http.StatusInternalServerError, "badges_fetch_failed", "Failed to fetch badges")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
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

	if h.service == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "gamification_unavailable", "Gamification service not configured")
		return
	}

	limit, offset := parsePageParams(r)

	badges, total, err := h.service.ListAllBadges(r.Context(), limit, offset)
	if err != nil {
		slog.Error("gamification: admin list badges failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "badges_fetch_failed", "Failed to fetch badges")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
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

	if h.service == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "gamification_unavailable", "Gamification service not configured")
		return
	}

	var in CreateBadgeInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	badge, err := h.service.CreateBadge(r.Context(), in)
	if err != nil {
		slog.Error("gamification: admin create badge failed", "error", err)
		errpkg.WriteError(w, http.StatusBadRequest, "badge_create_failed", err.Error())

		return
	}

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{
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

	badgeID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	if h.service == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "gamification_unavailable", "Gamification service not configured")
		return
	}

	var in UpdateBadgeInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	badge, err := h.service.UpdateBadge(r.Context(), badgeID, in)
	if err != nil {
		slog.Error("gamification: admin update badge failed", "error", err)
		errpkg.WriteError(w, http.StatusBadRequest, "badge_update_failed", err.Error())

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": badge,
	})
}

// HandleAdminAwardBadge handles POST /api/v1/admin/gamification/badges/:id/award
// — manually award a badge to a user. Admin route (gamification.award_badge).
// Body: { "user_id": "<uuid>" }.
func (h *Handlers) HandleAdminAwardBadge(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	badgeID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	if h.service == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "gamification_unavailable", "Gamification service not configured")
		return
	}

	var body struct {
		UserID string `json:"user_id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if body.UserID == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_user_id", "user_id is required")
		return
	}

	targetUserID, err := scanUUID(body.UserID)
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_user_id", "Invalid user_id")
		return
	}

	badge, err := h.service.AwardBadge(r.Context(), badgeID, targetUserID, user.UserID)
	if err != nil {
		slog.Error("gamification: admin award badge failed", "error", err)
		errpkg.WriteError(w, http.StatusBadRequest, "badge_award_failed", err.Error())

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": badge,
	})
}

// HandleAdminDeleteBadge handles DELETE /api/v1/admin/gamification/badges/:id —
// delete a non-system badge definition. Admin route (gamification.manage_badges).
func (h *Handlers) HandleAdminDeleteBadge(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}

	badgeID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	if h.service == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "gamification_unavailable", "Gamification service not configured")
		return
	}

	if err := h.service.DeleteBadge(r.Context(), badgeID); err != nil {
		slog.Error("gamification: admin delete badge failed", "error", err)
		errpkg.WriteError(w, http.StatusBadRequest, "badge_delete_failed", err.Error())

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{"deleted": true},
	})
}

// HandleAdminListChallenges handles GET /api/v1/admin/gamification/challenges —
// all challenges (including inactive). Admin route
// (gamification.manage_challenges).
func (h *Handlers) HandleAdminListChallenges(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}

	if h.service == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "gamification_unavailable", "Gamification service not configured")
		return
	}

	limit, offset := parsePageParams(r)

	challenges, total, err := h.service.ListAllChallenges(r.Context(), limit, offset)
	if err != nil {
		slog.Error("gamification: admin list challenges failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "challenges_fetch_failed", "Failed to fetch challenges")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
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

	if h.service == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "gamification_unavailable", "Gamification service not configured")
		return
	}

	var in CreateChallengeInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	challenge, err := h.service.CreateChallenge(r.Context(), in)
	if err != nil {
		slog.Error("gamification: admin create challenge failed", "error", err)
		errpkg.WriteError(w, http.StatusBadRequest, "challenge_create_failed", err.Error())

		return
	}

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{
		"data": challenge,
	})
}

// HandleAdminUpdateChallenge handles PATCH
// /api/v1/admin/gamification/challenges/:id — update a challenge definition's
// mutable fields. Admin route (gamification.manage_challenges).
func (h *Handlers) HandleAdminUpdateChallenge(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}

	challengeID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	if h.service == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "gamification_unavailable", "Gamification service not configured")
		return
	}

	var in UpdateChallengeInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	challenge, err := h.service.UpdateChallenge(r.Context(), challengeID, in)
	if err != nil {
		slog.Error("gamification: admin update challenge failed", "error", err)
		errpkg.WriteError(w, http.StatusBadRequest, "challenge_update_failed", err.Error())

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": challenge,
	})
}

// HandleAdminDeleteChallenge handles DELETE
// /api/v1/admin/gamification/challenges/:id — delete a challenge definition.
// Admin route (gamification.manage_challenges).
func (h *Handlers) HandleAdminDeleteChallenge(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}

	challengeID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	if h.service == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "gamification_unavailable", "Gamification service not configured")
		return
	}

	if err := h.service.DeleteChallenge(r.Context(), challengeID); err != nil {
		slog.Error("gamification: admin delete challenge failed", "error", err)
		errpkg.WriteError(w, http.StatusBadRequest, "challenge_delete_failed", err.Error())

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{"deleted": true},
	})
}
