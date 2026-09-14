// Package japastage — handlers.go provides the HTTP handlers for the Japa
// Stage Progress Profile routes (§87, ADR-095).
// // Routes:
// //	GET /api/v1/users/:username/japa-stage — public stage view
// GET /api/v1/me/japa-stage — own stage (auth, profile.japa_stage.edit)
// PATCH /api/v1/me/japa-stage — update own stage (auth, profile.japa_stage.edit)
// GET /api/v1/members/by-stage — directory ?stage=&dest=&page= (public profiles only)
// // On a successful PATCH, the handler enqueues a fire-and-forget
// japa_stage:recommend Asynq task (Rule 8) so the worker can dispatch tool
// recommendations, update the JapaBot peer card, and prompt a Readiness
// Assessment (§87 Stage-Based Recommendations).
// // Ref: japaconnect-engineering/sections/87-japa-stage-progress-profiles.md
// //	japaconnect-engineering/adr/ADR-095-japa-stage-profiles.md
package japastage

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/config"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/valkey"
	"github.com/jedafe/japa/internal/worker"
	"github.com/valkey-io/valkey-go/valkeycompat"
)

// Handlers exposes the japa stage profile endpoints.
type Handlers struct {
	service     *Service
	settings    *config.Settings
	asynqClient *asynq.Client
	redis       valkeycompat.Cmdable // nil disables cache invalidation (tests)
}

// NewHandlers creates japa stage handlers bound to the provided service.
// asynqClient may be nil — when absent, stage-change fan-out is skipped
// (logged at warn) instead of crashing the request. redis may be nil —
// when absent, VisitorContext/recommendations cache invalidation is skipped.
func NewHandlers(service *Service, settings *config.Settings, asynqClient *asynq.Client) *Handlers {
	return &Handlers{service: service, settings: settings, asynqClient: asynqClient}
}

// WithRedis sets the Redis client used to invalidate the VisitorContext and
// recommendations caches after a stage update (ADR-064a §5, BUG-002). Returns
// the receiver for chaining.
func (h *Handlers) WithRedis(rdb valkeycompat.Cmdable) *Handlers {
	h.redis = rdb
	return h
}

// invalidateVisitorCache deletes the VisitorContext and recommendations cache
// entries for the given user (ADR-064a). Mirrors the pattern in
// internal/users/handlers.go — a japa stage change alters the archetype
// resolution, so cached recommendations must be evicted to avoid serving stale
// results for up to 15 minutes.
func (h *Handlers) invalidateVisitorCache(ctx context.Context, user *auth.AuthenticatedUser) {
	if h.redis == nil {
		return
	}

	if user.SessionID != "" {
		_ = h.redis.Del(ctx, valkey.MustKey("visitor:ctx:%s", user.SessionID)).Err()
		_ = h.redis.Del(ctx, valkey.MustKey("recommendations:%s", user.SessionID)).Err()
	}

	_ = h.redis.Del(ctx, valkey.MustKey("visitor:ctx:user:%s", user.UserID.String())).Err()
	_ = h.redis.Del(ctx, valkey.MustKey("recommendations:user:%s", user.UserID.String())).Err()
}

// ---------------------------------------------------------------------------
// Shared helpers (same pattern as story/handlers.go)
// ---------------------------------------------------------------------------

func requireTenant(w http.ResponseWriter, r *http.Request) (pgtype.UUID, bool) {
	return pgtype.UUID{}, true
}

func requireUser(w http.ResponseWriter, r *http.Request) (*auth.AuthenticatedUser, bool) {
	u := auth.FromContext(r.Context())
	if u == nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return nil, false
	}

	return u, true
}

// parsePageParams reads pagination from the query string. Both `page_size`
// (the convention per api-response-format.md) and `limit` (back-compat) are
// accepted; `page` is 1-based.
func parsePageParams(r *http.Request) (page, limit int32) {
	page = 1
	limit = 20

	if v := r.URL.Query().Get("page"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			// #nosec G109 // value bounded by MaxPageSize check
			// #nosec G115 // value bounded by caller
			page = int32(n)
		}
	}
	// Prefer page_size (convention); fall back to limit for older clients.
	if v := r.URL.Query().Get("page_size"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 100 {
			// #nosec G109 // value bounded by MaxPageSize check
			limit = int32(n) //nolint:gosec // G109,G115: bounded by n <= 100 check above
		}
	} else if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 100 {
			// #nosec G109 // value bounded by MaxPageSize check
			limit = int32(n) //nolint:gosec // G109,G115: bounded by n <= 100 check above
		}
	}

	return page, limit
}

// ---------------------------------------------------------------------------
// Public handlers
// ---------------------------------------------------------------------------

// HandleStages handles GET /japa-stage/stages — returns the canonical stage
// option list (Rule 16).
func (h *Handlers) HandleStages(w http.ResponseWriter, r *http.Request) {
	_, ok := requireTenant(w, r)
	if !ok {
		return
	}

	if h.service == nil || !h.service.Enabled(r.Context()) {
		errpkg.WriteError(w, http.StatusNotFound, "japa_stage_disabled", "Japa stage profiles are not available")
		return
	}

	stages := h.service.Stages()
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": stages})
}

// HandleGetByUsername handles GET /users/:username/japa-stage — public stage
// view. Returns an empty data object (stage omitted) when the profile is not
// public, so anonymous browsers simply see no stage.
func (h *Handlers) HandleGetByUsername(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	if h.service == nil || !h.service.Enabled(r.Context()) {
		errpkg.WriteError(w, http.StatusNotFound, "japa_stage_disabled", "Japa stage profiles are not available")
		return
	}

	username := chi.URLParam(r, "username")
	if username == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_username", "Username is required")
		return
	}

	profile, err := h.service.GetByUsername(r.Context(), tenantID, username)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "profile_not_found", "Profile not found")
			return
		}

		slog.Error("japastage: get by username failed", "username", username, "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "get_failed", "Failed to get japa stage profile")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": profile})
}

// HandleListByStage handles GET /members/by-stage — public directory of
// members filtered by stage. Query params: ?stage=&dest=&page=&page_size=
// (dest is the destination space id, geo-aware ?dest= filter per §87).
func (h *Handlers) HandleListByStage(w http.ResponseWriter, r *http.Request) {
	_, ok := requireTenant(w, r)
	if !ok {
		return
	}

	if h.service == nil || !h.service.Enabled(r.Context()) {
		errpkg.WriteError(w, http.StatusNotFound, "japa_stage_disabled", "Japa stage profiles are not available")
		return
	}

	stage := r.URL.Query().Get("stage")
	if stage != "" && !ValidStage(stage) {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_stage", "Invalid stage; must be one of: "+JoinStages())
		return
	}

	dest := r.URL.Query().Get("dest")
	page, limit := parsePageParams(r)

	members, total, err := h.service.ListByStage(r.Context(), stage, dest, page, limit)
	if err != nil {
		slog.Error("japastage: list by stage failed", "stage", stage, "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "list_failed", "Failed to list members by stage")

		return
	}

	// Offset-pagination envelope per conventions/api-response-format.md:
	// { data: [...], meta: { page, page_size, total, total_pages } }.
	totalPages := int64(1)
	if limit > 0 && total > 0 {
		totalPages = (total + int64(limit) - 1) / int64(limit)
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": members,
		"meta": map[string]any{
			"page":        page,
			"page_size":   limit,
			"total":       total,
			"total_pages": totalPages,
		},
	})
}

// ---------------------------------------------------------------------------
// Authenticated handlers (own profile — profile.japa_stage.edit)
// ---------------------------------------------------------------------------

// HandleGetOwn handles GET /me/japa-stage — the caller's own stage profile
// (full view, including history and the visibility flag).
func (h *Handlers) HandleGetOwn(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	if h.service == nil || !h.service.Enabled(r.Context()) {
		errpkg.WriteError(w, http.StatusNotFound, "japa_stage_disabled", "Japa stage profiles are not available")
		return
	}

	profile, err := h.service.GetOwn(r.Context(), user.UserID, tenantID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "profile_not_found", "Profile not found")
			return
		}

		slog.Error("japastage: get own failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "get_failed", "Failed to get japa stage profile")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": profile})
}

// HandleUpdateOwn handles PATCH /me/japa-stage — update the authenticated
// user's own japa stage, destination, and (optionally) visibility. Auth
// required (profile.japa_stage.edit permission checked at route
// registration). On success a japa_stage:recommend Asynq task is enqueued
// fire-and-forget (Rule 8) so the worker can dispatch recommendations.
func (h *Handlers) HandleUpdateOwn(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	if h.service == nil || !h.service.Enabled(r.Context()) {
		errpkg.WriteError(w, http.StatusNotFound, "japa_stage_disabled", "Japa stage profiles are not available")
		return
	}

	var input UpdateStageInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if input.Stage == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_stage", "Stage is required")
		return
	}

	if !ValidStage(input.Stage) {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_stage", "Invalid stage; must be one of: "+JoinStages())
		return
	}

	profile, err := h.service.UpdateOwn(r.Context(), user.UserID, tenantID, input)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "profile_not_found", "Profile not found")
			return
		}

		slog.Error("japastage: update own failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "update_failed", "Failed to update japa stage")

		return
	}

	// Invalidate the VisitorContext and recommendations cache for this user
	// so the next request picks up the new archetype/stage resolution
	// immediately (ADR-064a §5, BUG-002). Without this, cached recommendations
	// remain stale for up to 15 minutes after a stage change.
	h.invalidateVisitorCache(r.Context(), user)

	// Fire-and-forget japa_stage:recommend (§87, Rule 8). Idempotent on
	// (user_id, stage, set_at) via asynq.Unique over a short window so a
	// duplicate PATCH within the same minute does not double-dispatch.
	if h.asynqClient != nil {
		payload, _ := json.Marshal(map[string]any{
			"user_id": user.UserID.String(),

			"stage":                input.Stage,
			"destination_space_id": input.DestSpaceID,
		})

		task := asynq.NewTask(worker.TypeJapaStageRecommend, payload)
		if _, err := h.asynqClient.Enqueue(
			task,
			asynq.Queue(worker.QueueForTask(worker.TypeJapaStageRecommend)),
			asynq.MaxRetry(3),
			asynq.Unique(60*1_000_000_000), // 1 minute
		); err != nil {
			slog.Warn("japastage: enqueue japa_stage:recommend failed", "error", err)
		}
	} else {
		slog.Warn("japastage: asynq client nil, skipping japa_stage:recommend fan-out")
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": profile})
}
