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
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/config"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/httpx"
	"github.com/jedafe/japa/internal/literals"
)

// Handlers exposes the gamification HTTP endpoints.
type Handlers struct {
	service  *Service
	settings *config.Settings
	lit      literalResolver // nil disables literal resolution (§14)
}

// literalResolver resolves {{...}} template literals in user-facing text
// fields (§14). Implemented by *literals.Catalog; nil disables resolution.
type literalResolver interface {
	ResolveFields(ctx context.Context, v any, rc literals.RenderContext)
}

// SetLiteralResolver injects the template-literal catalog so user-facing
// titles/descriptions render {{...}} placeholders (§14).
func (h *Handlers) SetLiteralResolver(l literalResolver) { h.lit = l }

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

// validationMessage strips the ErrInvalidInput sentinel prefix so the
// error envelope carries only the human-readable detail.
func validationMessage(err error) string {
	return strings.TrimPrefix(err.Error(), ErrInvalidInput.Error()+": ")
}

// parsePageParams extracts limit and offset from query string, with defaults.
// The canonical ?page_size/?page_offset names (api-response-format convention)
// take precedence; ?limit/?offset remain accepted as legacy aliases.
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

	if v := r.URL.Query().Get("page_size"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 200 {
			limit = int32(n) // #nosec
		}
	}

	if v := r.URL.Query().Get("page_offset"); v != "" {
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

	badges, err := h.service.ListBadges(r.Context(), h.service.CallerScope(r))
	if err != nil {
		if writeDisabled(w, err) {
			return
		}

		slog.Error("gamification: list badges failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "badges_fetch_failed", "Failed to fetch badges")

		return
	}

	h.resolveLiterals(r, &badges)

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

	challenges, err := h.service.ListActiveChallenges(r.Context(), h.service.CallerScope(r))
	if err != nil {
		if writeDisabled(w, err) {
			return
		}

		slog.Error("gamification: list challenges failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "challenges_fetch_failed", "Failed to fetch challenges")

		return
	}

	h.resolveLiterals(r, &challenges)

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

	// PTS-017: a space-scoped leaderboard reveals that space's member list +
	// points. Non-public spaces are member-only; a missing/forbidden space
	// must 404 (not 403) so the route cannot be used to probe space privacy.
	var viewer pgtype.UUID
	if u := auth.FromContext(r.Context()); u != nil {
		viewer = u.UserID
	}

	allowed, aerr := h.service.CanViewSpaceLeaderboard(r.Context(), scope, viewer)
	if aerr != nil {
		slog.Error("gamification: leaderboard scope check failed", "error", aerr)
		errpkg.WriteError(w, http.StatusInternalServerError, "leaderboard_fetch_failed", "Failed to fetch leaderboard")
		return
	}
	if !allowed {
		errpkg.WriteError(w, http.StatusNotFound, "leaderboard_not_found", "Leaderboard not found")
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

	h.resolveLiterals(r, &entries)

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

	h.resolveLiterals(r, &badges)

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": badges,
	})
}

// HandleMyBadgeCollection handles GET /api/v1/me/badge-collection — the
// achievement-centre payload: the active catalogue grouped by category with
// the caller's earned state and per-badge progress. Auth required.
func (h *Handlers) HandleMyBadgeCollection(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	if h.service == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "gamification_unavailable", "Gamification service not configured")
		return
	}

	collection, err := h.service.GetBadgeCollection(r.Context(), user.UserID, h.service.CallerScope(r))
	if err != nil {
		if writeDisabled(w, err) {
			return
		}

		slog.Error("gamification: badge collection failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "badges_fetch_failed", "Failed to fetch badges")

		return
	}

	h.resolveLiterals(r, &collection)

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": collection,
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

	progress, err := h.service.ListUserChallengeProgress(r.Context(), user.UserID, h.service.CallerScope(r))
	if err != nil {
		if writeDisabled(w, err) {
			return
		}

		slog.Error("gamification: my challenges failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "challenges_fetch_failed", "Failed to fetch challenges")

		return
	}

	h.resolveLiterals(r, &progress)

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": progress,
	})
}

// HandleUserBadges handles GET /api/v1/users/:username/badges — public badge
// display for a user's profile. Public route.
func (h *Handlers) HandleUserBadges(w http.ResponseWriter, r *http.Request) {
	username := httpx.URLParamDecoded(r, "username")
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

	h.resolveLiterals(r, &badges)

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

	h.resolveLiterals(r, &badges)

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
		switch {
		case errors.Is(err, ErrInvalidInput):
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_input", validationMessage(err))
		case errors.Is(err, ErrSlugExists):
			errpkg.WriteError(w, http.StatusConflict, "slug_exists", "A badge with this slug already exists")
		default:
			slog.Error("gamification: admin create badge failed", "error", err)
			errpkg.WriteError(w, http.StatusBadRequest, "badge_create_failed", "Failed to create badge")
		}

		return
	}

	h.resolveLiterals(r, &badge)

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
		switch {
		case errors.Is(err, ErrInvalidInput):
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_input", validationMessage(err))
		case errors.Is(err, ErrBadgeNotFound):
			errpkg.WriteError(w, http.StatusNotFound, "badge_not_found", "Badge not found")
		default:
			slog.Error("gamification: admin update badge failed", "error", err)
			errpkg.WriteError(w, http.StatusBadRequest, "badge_update_failed", "Failed to update badge")
		}

		return
	}

	h.resolveLiterals(r, &badge)

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
		if errors.Is(err, ErrBadgeNotFound) {
			errpkg.WriteError(w, http.StatusNotFound, "badge_not_found", "Badge not found")
			return
		}
		if errors.Is(err, ErrUserNotFound) {
			errpkg.WriteError(w, http.StatusNotFound, "user_not_found", "User not found")
			return
		}

		errpkg.WriteError(w, http.StatusBadRequest, "badge_award_failed", "Failed to award badge")

		return
	}

	h.resolveLiterals(r, &badge)

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
		errpkg.WriteError(w, http.StatusBadRequest, "badge_delete_failed", "Failed to delete badge")

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

	h.resolveLiterals(r, &challenges)

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
		switch {
		case errors.Is(err, ErrInvalidInput):
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_input", validationMessage(err))
		case errors.Is(err, ErrSlugExists):
			errpkg.WriteError(w, http.StatusConflict, "slug_exists", "A challenge with this slug already exists")
		default:
			slog.Error("gamification: admin create challenge failed", "error", err)
			errpkg.WriteError(w, http.StatusBadRequest, "challenge_create_failed", "Failed to create challenge")
		}

		return
	}

	h.resolveLiterals(r, &challenge)

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
		switch {
		case errors.Is(err, ErrInvalidInput):
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_input", validationMessage(err))
		case errors.Is(err, ErrChallengeNotFound):
			errpkg.WriteError(w, http.StatusNotFound, "challenge_not_found", "Challenge not found")
		default:
			slog.Error("gamification: admin update challenge failed", "error", err)
			errpkg.WriteError(w, http.StatusBadRequest, "challenge_update_failed", "Failed to update challenge")
		}

		return
	}

	h.resolveLiterals(r, &challenge)

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
		errpkg.WriteError(w, http.StatusBadRequest, "challenge_delete_failed", "Failed to delete challenge")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{"deleted": true},
	})
}

// HandleAdminListAwardEvents handles GET /api/v1/admin/gamification/award-events —
// the badge-award audit trail, optionally scoped via ?badge_id=.
func (h *Handlers) HandleAdminListAwardEvents(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}
	if h.service == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "gamification_unavailable", "Gamification service not configured")
		return
	}

	var badgeID pgtype.UUID
	if raw := r.URL.Query().Get("badge_id"); raw != "" {
		if err := badgeID.Scan(raw); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_badge_id", "Invalid badge_id")
			return
		}
	}

	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if len(q) > 100 {
		q = q[:100]
	}
	sortBy := r.URL.Query().Get("sort_by")
	switch sortBy {
	case "username", "badge_name", "source", "created_at":
	default:
		sortBy = ""
	}
	sortDir := r.URL.Query().Get("sort_dir")
	if sortDir != "asc" && sortDir != "desc" {
		sortDir = ""
	}

	limit, offset := parsePageParams(r)
	events, total, err := h.service.ListAwardEvents(r.Context(), badgeID, q, sortBy, sortDir, limit, offset)
	if err != nil {
		slog.Error("gamification: list award events failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "award_events_fetch_failed", "Failed to fetch award events")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data":       events,
		"pagination": map[string]any{"total": total, "limit": limit, "offset": offset},
	})
}

// HandleAdminListBadgeGroups handles GET /api/v1/admin/gamification/badge-groups —
// distinct badge_group values for the tier editor.
func (h *Handlers) HandleAdminListBadgeGroups(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}
	if h.service == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "gamification_unavailable", "Gamification service not configured")
		return
	}

	groups, err := h.service.ListBadgeGroups(r.Context())
	if err != nil {
		slog.Error("gamification: list badge groups failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "badge_groups_fetch_failed", "Failed to fetch badge groups")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": groups})
}

// HandleAdminListSeasons handles GET /api/v1/admin/gamification/seasons.
func (h *Handlers) HandleAdminListSeasons(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}
	if h.service == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "gamification_unavailable", "Gamification service not configured")
		return
	}

	seasons, err := h.service.ListSeasons(r.Context())
	if err != nil {
		slog.Error("gamification: list seasons failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "seasons_fetch_failed", "Failed to fetch seasons")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": seasons})
}

// HandleAdminCreateSeason handles POST /api/v1/admin/gamification/seasons.
func (h *Handlers) HandleAdminCreateSeason(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}
	if h.service == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "gamification_unavailable", "Gamification service not configured")
		return
	}

	var in CreateSeasonInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_json", "Invalid request body")
		return
	}

	season, err := h.service.CreateSeason(r.Context(), in)
	if err != nil {
		switch {
		case errors.Is(err, ErrInvalidInput):
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_input", validationMessage(err))
		case errors.Is(err, ErrSlugExists):
			errpkg.WriteError(w, http.StatusConflict, "slug_exists", "A season with this slug already exists")
		default:
			slog.Error("gamification: create season failed", "error", err)
			errpkg.WriteError(w, http.StatusBadRequest, "season_create_failed", "Failed to create season")
		}
		return
	}

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": season})
}

// HandleAdminUpdateSeason handles PATCH /api/v1/admin/gamification/seasons/:id.
func (h *Handlers) HandleAdminUpdateSeason(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}
	if h.service == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "gamification_unavailable", "Gamification service not configured")
		return
	}

	var id pgtype.UUID
	if err := id.Scan(chi.URLParam(r, "id")); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid season id")
		return
	}

	var in UpdateSeasonInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_json", "Invalid request body")
		return
	}

	season, err := h.service.UpdateSeason(r.Context(), id, in)
	if err != nil {
		switch {
		case errors.Is(err, ErrInvalidInput):
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_input", validationMessage(err))
		case errors.Is(err, ErrSeasonNotFound):
			errpkg.WriteError(w, http.StatusNotFound, "season_not_found", "Season not found")
		default:
			slog.Error("gamification: update season failed", "id", id.String(), "error", err)
			errpkg.WriteError(w, http.StatusBadRequest, "season_update_failed", "Failed to update season")
		}
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": season})
}

// HandleAdminDeleteSeason handles DELETE /api/v1/admin/gamification/seasons/:id.
func (h *Handlers) HandleAdminDeleteSeason(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}
	if h.service == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "gamification_unavailable", "Gamification service not configured")
		return
	}

	var id pgtype.UUID
	if err := id.Scan(chi.URLParam(r, "id")); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid season id")
		return
	}

	if err := h.service.DeleteSeason(r.Context(), id); err != nil {
		slog.Warn("gamification: delete season failed", "id", id.String(), "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "season_delete_failed", "Failed to delete season")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"deleted": true}})
}

// HandleAdminListTiers handles GET /api/v1/admin/gamification/tiers.
func (h *Handlers) HandleAdminListTiers(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}
	if h.service == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "gamification_unavailable", "Gamification service not configured")
		return
	}

	tiers, err := h.service.ListTiers(r.Context())
	if err != nil {
		slog.Error("gamification: list tiers failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "tiers_fetch_failed", "Failed to fetch tiers")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": tiers})
}

// HandleAdminCreateTier handles POST /api/v1/admin/gamification/tiers.
func (h *Handlers) HandleAdminCreateTier(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}
	if h.service == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "gamification_unavailable", "Gamification service not configured")
		return
	}

	var in CreateTierInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_json", "Invalid request body")
		return
	}

	tier, err := h.service.CreateTier(r.Context(), in)
	if err != nil {
		switch {
		case errors.Is(err, ErrInvalidInput):
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_input", validationMessage(err))
		case errors.Is(err, ErrTierExists):
			errpkg.WriteError(w, http.StatusConflict, "tier_exists", "A tier with this name already exists for this badge group")
		default:
			slog.Error("gamification: create tier failed", "error", err)
			errpkg.WriteError(w, http.StatusBadRequest, "tier_create_failed", "Failed to create tier")
		}
		return
	}

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": tier})
}

// HandleAdminUpdateTier handles PATCH /api/v1/admin/gamification/tiers/:id.
func (h *Handlers) HandleAdminUpdateTier(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}
	if h.service == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "gamification_unavailable", "Gamification service not configured")
		return
	}

	var id pgtype.UUID
	if err := id.Scan(chi.URLParam(r, "id")); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid tier id")
		return
	}

	var in UpdateTierInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_json", "Invalid request body")
		return
	}

	tier, err := h.service.UpdateTier(r.Context(), id, in)
	if err != nil {
		switch {
		case errors.Is(err, ErrInvalidInput):
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_input", validationMessage(err))
		case errors.Is(err, ErrTierNotFound):
			errpkg.WriteError(w, http.StatusNotFound, "tier_not_found", "Tier not found")
		default:
			slog.Error("gamification: update tier failed", "id", id.String(), "error", err)
			errpkg.WriteError(w, http.StatusBadRequest, "tier_update_failed", "Failed to update tier")
		}
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": tier})
}

// HandleAdminDeleteTier handles DELETE /api/v1/admin/gamification/tiers/:id.
func (h *Handlers) HandleAdminDeleteTier(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}
	if h.service == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "gamification_unavailable", "Gamification service not configured")
		return
	}

	var id pgtype.UUID
	if err := id.Scan(chi.URLParam(r, "id")); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid tier id")
		return
	}

	if err := h.service.DeleteTier(r.Context(), id); err != nil {
		slog.Warn("gamification: delete tier failed", "id", id.String(), "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "tier_delete_failed", "Failed to delete tier")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"deleted": true}})
}
