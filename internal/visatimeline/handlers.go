// Package visatimeline — handlers.go provides the HTTP handlers for the Visa
// Timeline Tracker routes (§83, ADR-091).
//
// Routes:
//
//	GET    /api/v1/visa-timelines/stats                  — public aggregated stats
//	GET    /api/v1/visa-timelines/stats/trend            — public monthly trend
//	GET    /api/v1/me/visa-timelines                     — list own (auth, visa_timeline.create)
//	POST   /api/v1/me/visa-timelines                     — create (auth, visa_timeline.create)
//	PATCH  /api/v1/me/visa-timelines/:id                 — update own (auth, visa_timeline.manage_own)
//	POST   /api/v1/me/visa-timelines/:id/milestones      — add milestone (auth, visa_timeline.manage_own)
//	PATCH  /api/v1/me/visa-timelines/:id/milestones/:mid — edit milestone (auth, visa_timeline.manage_own)
//	GET    /api/v1/admin/visa-timelines                  — admin list (auth, visa_timeline.manage_any)
//	DELETE /api/v1/admin/visa-timelines/:id              — admin delete (auth, visa_timeline.manage_any)
//
// Ref: japaconnect-engineering/sections/83-visa-timeline-tracker.md
//
//	japaconnect-engineering/adr/ADR-091-visa-timeline-tracker.md
package visatimeline

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/config"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/literals"
)

// pointsChecker returns the user's lifetime points (reputation) for the
// anti-gaming reputation gate on shared timelines (§83: contribute to
// aggregate stats requires reputation ≥ visa_timeline.min_reputation).
type pointsChecker interface {
	GetPointsBalance(ctx context.Context, userID pgtype.UUID) (int64, error)
}

// Handlers exposes the visa-timeline endpoints.
type Handlers struct {
	service  *Service
	settings *config.Settings
	points   pointsChecker
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

// NewHandlers creates visa-timeline handlers bound to the provided service.
func NewHandlers(service *Service, settings *config.Settings) *Handlers {
	return &Handlers{service: service, settings: settings}
}

// SetPointsChecker injects the wallet service for the anti-gaming reputation
// gate on shared timelines (§83). Called once at startup from server.go.
func (h *Handlers) SetPointsChecker(pc pointsChecker) {
	h.points = pc
}

// ---------------------------------------------------------------------------
// Shared helpers (same pattern as story/handlers.go)
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

func parsePageParams(r *http.Request) (page, limit int32) {
	page = 1
	limit = 20

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

// ---------------------------------------------------------------------------
// Public handlers (stats + trend — anonymous per §83)
// ---------------------------------------------------------------------------

// HandleStats handles GET /visa-timelines/stats — public aggregated
// processing-time statistics. Query params: ?origin=&dest=&visa=&consulate=&months=.
func (h *Handlers) HandleStats(w http.ResponseWriter, r *http.Request) {

	if h.service == nil || !h.service.Enabled(r.Context()) {
		errpkg.WriteError(w, http.StatusNotFound, "visa_timeline_disabled", "Visa Timeline Tracker is not available")
		return
	}

	var originSpaceID pgtype.UUID
	if v := r.URL.Query().Get("origin"); v != "" {
		if err := originSpaceID.Scan(v); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid origin space ID")
			return
		}
	}

	var destSpaceID pgtype.UUID
	if v := r.URL.Query().Get("dest"); v != "" {
		if err := destSpaceID.Scan(v); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid destination space ID")
			return
		}
	}

	visaCategory := r.URL.Query().Get("visa")
	consulate := r.URL.Query().Get("consulate")
	months := 0

	if v := r.URL.Query().Get("months"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 60 {
			months = n
		}
	}

	stats, err := h.service.GetStats(r.Context(), originSpaceID, destSpaceID, visaCategory, consulate, months)
	if err != nil {
		slog.Error("visatimeline: stats failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "stats_failed", "Failed to get stats")

		return
	}

	h.resolveLiterals(r, &stats)

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": stats})
}

// HandleTrend handles GET /visa-timelines/stats/trend — public monthly trend.
// Query params: ?dest=&visa=&consulate=&months= (default 12).
func (h *Handlers) HandleTrend(w http.ResponseWriter, r *http.Request) {

	if h.service == nil || !h.service.Enabled(r.Context()) {
		errpkg.WriteError(w, http.StatusNotFound, "visa_timeline_disabled", "Visa Timeline Tracker is not available")
		return
	}

	var destSpaceID pgtype.UUID
	if v := r.URL.Query().Get("dest"); v != "" {
		if err := destSpaceID.Scan(v); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid destination space ID")
			return
		}
	}

	visaCategory := r.URL.Query().Get("visa")
	consulate := r.URL.Query().Get("consulate")
	months := 12

	if v := r.URL.Query().Get("months"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 60 {
			months = n
		}
	}

	points, err := h.service.GetTrend(r.Context(), destSpaceID, visaCategory, consulate, months)
	if err != nil {
		slog.Error("visatimeline: trend failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "trend_failed", "Failed to get trend")

		return
	}

	h.resolveLiterals(r, &points)

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": points})
}

// ---------------------------------------------------------------------------
// Authenticated handlers (own timelines + milestones — auth required)
// ---------------------------------------------------------------------------

// HandleListOwn handles GET /me/visa-timelines — list the authenticated user's
// own timelines. Auth required (visa_timeline.create permission checked at
// route registration).
func (h *Handlers) HandleListOwn(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	if h.service == nil || !h.service.Enabled(r.Context()) {
		errpkg.WriteError(w, http.StatusNotFound, "visa_timeline_disabled", "Visa Timeline Tracker is not available")
		return
	}

	timelines, err := h.service.ListOwn(r.Context(), user.UserID)
	if err != nil {
		slog.Error("visatimeline: list own failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "list_failed", "Failed to list timelines")

		return
	}

	h.resolveLiterals(r, &timelines)

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": timelines})
}

// HandleCreate handles POST /me/visa-timelines — create a new timeline. Auth
// required (visa_timeline.create permission checked at route registration).
func (h *Handlers) HandleCreate(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	if h.service == nil || !h.service.Enabled(r.Context()) {
		errpkg.WriteError(w, http.StatusNotFound, "visa_timeline_disabled", "Visa Timeline Tracker is not available")
		return
	}

	var input CreateTimelineInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if input.DestinationSpaceID == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_destination", "Destination space ID is required")
		return
	}

	if input.VisaCategory == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_visa_category", "Visa category is required")
		return
	}

	// Anti-gaming gate (§83 Privacy & Anti-Gaming): contributing to the
	// community aggregate requires (a) account age ≥
	// visa_timeline.min_account_age_days (default 30) and (b) reputation ≥
	// visa_timeline.min_reputation (default 50). The timeline itself can
	// always be created privately; only the share flag is gated. If the
	// user does not meet the thresholds, we force is_shared=false rather
	// than rejecting the whole request, so members can still track their
	// own journey privately.
	if input.IsShared {
		if !h.checkAntiGaming(w, r, user.UserID) {
			// Gate failed — the helper already wrote the error response
			// when the check itself errored. If the thresholds simply
			// aren't met, downgrade to private and continue.
			if input.IsShared {
				input.IsShared = false
			}
		}
	}

	timeline, err := h.service.Create(r.Context(), user.UserID, input)
	if err != nil {
		slog.Error("visatimeline: create failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "create_failed", "Failed to create timeline")

		return
	}

	h.resolveLiterals(r, &timeline)

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": timeline})
}

// checkAntiGaming verifies the account-age and reputation thresholds for
// sharing a timeline in the community aggregate (§83). Returns true if the
// user meets both thresholds (or if the checker is not wired, in which case
// the gate is a no-op for graceful degradation). Returns false if the
// thresholds are not met (the caller should downgrade is_shared to false) or
// if the check itself failed (the caller should also downgrade + the error
// has already been written to the response).
func (h *Handlers) checkAntiGaming(w http.ResponseWriter, r *http.Request, userID pgtype.UUID) bool {
	minAgeDays := 30
	if h.settings != nil {
		minAgeDays = h.settings.GetIntOr(r.Context(), "visa_timeline.min_account_age_days", 30)
	}

	minReputation := 50
	if h.settings != nil {
		minReputation = h.settings.GetIntOr(r.Context(), "visa_timeline.min_reputation", 50)
	}

	// Account-age check via the store.
	createdAt, err := h.service.store.UserAccountAge(r.Context(), userID)
	if err != nil {
		slog.Error("visatimeline: account age check failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "anti_gaming_check_failed", "Failed to verify account age")

		return false
	}

	if createdAt.Valid {
		ageDays := int(time.Since(createdAt.Time).Hours() / 24)
		if ageDays < minAgeDays {
			return false
		}
	}

	// Reputation check via the wallet points balance.
	if h.points != nil && minReputation > 0 {
		points, err := h.points.GetPointsBalance(r.Context(), userID)
		if err != nil {
			slog.Error("visatimeline: reputation check failed", "error", err)
			errpkg.WriteError(w, http.StatusInternalServerError, "anti_gaming_check_failed", "Failed to verify reputation")

			return false
		}

		if points < int64(minReputation) {
			return false
		}
	}

	return true
}

// HandleUpdate handles PATCH /me/visa-timelines/:id — update own timeline
// (outcome / share toggle). Auth required (visa_timeline.manage_own permission
// checked at route registration).
func (h *Handlers) HandleUpdate(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	id, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	if h.service == nil || !h.service.Enabled(r.Context()) {
		errpkg.WriteError(w, http.StatusNotFound, "visa_timeline_disabled", "Visa Timeline Tracker is not available")
		return
	}

	var input UpdateTimelineInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	timeline, err := h.service.UpdateOwn(r.Context(), id, user.UserID, input)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "timeline_not_found", "Timeline not found")
			return
		}

		if errors.Is(err, ErrNotOwner) {
			errpkg.WriteError(w, http.StatusForbidden, "not_owner", "You can only update your own timelines")
			return
		}

		slog.Error("visatimeline: update failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "update_failed", "Failed to update timeline")

		return
	}

	h.resolveLiterals(r, &timeline)

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": timeline})
}

// HandleAddMilestone handles POST /me/visa-timelines/:id/milestones — add a
// milestone to the authenticated user's own timeline. Auth required
// (visa_timeline.manage_own permission checked at route registration).
func (h *Handlers) HandleAddMilestone(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	timelineID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	if h.service == nil || !h.service.Enabled(r.Context()) {
		errpkg.WriteError(w, http.StatusNotFound, "visa_timeline_disabled", "Visa Timeline Tracker is not available")
		return
	}

	var input AddMilestoneInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if input.MilestoneType == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_milestone_type", "Milestone type is required")
		return
	}

	milestone, err := h.service.AddMilestone(r.Context(), timelineID, user.UserID, input)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "timeline_not_found", "Timeline not found")
			return
		}

		if errors.Is(err, ErrNotOwner) {
			errpkg.WriteError(w, http.StatusForbidden, "not_owner", "You can only add milestones to your own timelines")
			return
		}

		if errors.Is(err, ErrImplausibleSequence) {
			errpkg.WriteError(w, http.StatusUnprocessableEntity, "implausible_sequence", "This milestone date is inconsistent with existing milestones")
			return
		}

		slog.Error("visatimeline: add milestone failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "add_milestone_failed", "Failed to add milestone")

		return
	}

	h.resolveLiterals(r, &milestone)

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": milestone})
}

// HandleUpdateMilestone handles PATCH /me/visa-timelines/:id/milestones/:mid —
// edit a milestone on the authenticated user's own timeline. Auth required
// (visa_timeline.manage_own permission checked at route registration).
func (h *Handlers) HandleUpdateMilestone(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	timelineID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	milestoneID, ok := scanUUIDParam(w, r, "mid")
	if !ok {
		return
	}

	if h.service == nil || !h.service.Enabled(r.Context()) {
		errpkg.WriteError(w, http.StatusNotFound, "visa_timeline_disabled", "Visa Timeline Tracker is not available")
		return
	}

	var input UpdateMilestoneInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if err := h.service.UpdateMilestone(r.Context(), milestoneID, timelineID, user.UserID, input); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "not_found", "Timeline or milestone not found")
			return
		}

		if errors.Is(err, ErrNotOwner) {
			errpkg.WriteError(w, http.StatusForbidden, "not_owner", "You can only edit milestones on your own timelines")
			return
		}

		slog.Error("visatimeline: update milestone failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "update_milestone_failed", "Failed to update milestone")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"updated": true}})
}

// ---------------------------------------------------------------------------
// Admin handlers (moderation queue + admin delete — visa_timeline.manage_any)
// ---------------------------------------------------------------------------

// HandleAdminList handles GET /admin/visa-timelines — moderation queue listing
// all timelines. Admin-only (visa_timeline.manage_any permission checked at
// route registration). Supports ?user_id=&dest=&visa=&outcome=&q=&page=&limit=.
func (h *Handlers) HandleAdminList(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}

	if h.service == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "visa_timeline_unavailable", "Visa Timeline service not configured")
		return
	}

	var filter AdminListFilter

	if v := r.URL.Query().Get("user_id"); v != "" {
		if err := filter.UserID.Scan(v); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid user_id")
			return
		}
	}

	if v := r.URL.Query().Get("dest"); v != "" {
		if err := filter.DestinationSpaceID.Scan(v); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid destination space ID")
			return
		}
	}

	filter.VisaCategory = r.URL.Query().Get("visa")
	filter.Outcome = r.URL.Query().Get("outcome")
	filter.Q = r.URL.Query().Get("q")

	// Sort support (geo-aware tool pattern: "all admin tables must have
	// filter + sort + paginate"). EntityPage sends sort as "column:dir"
	// (e.g. "created_at:desc"); a bare "dir" query param is also accepted
	// for compatibility with the DataTable urlSync convention.
	if v := r.URL.Query().Get("sort"); v != "" {
		if col, dir, ok := strings.Cut(v, ":"); ok {
			filter.Sort = col
			filter.Order = dir
		} else {
			filter.Sort = v
		}
	}

	if v := r.URL.Query().Get("dir"); v != "" {
		filter.Order = v
	}

	page, limit := parsePageParams(r)
	if limit > 200 {
		limit = 200
	}

	filter.Limit = limit
	filter.Offset = (page - 1) * limit

	timelines, total, err := h.service.AdminList(r.Context(), filter)
	if err != nil {
		slog.Error("visatimeline: admin list failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "admin_list_failed", "Failed to list timelines")

		return
	}

	h.resolveLiterals(r, &timelines)

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": timelines,
		"pagination": map[string]any{
			"page":  page,
			"limit": limit,
			"total": total,
		},
	})
}

// HandleAdminDelete handles DELETE /admin/visa-timelines/:id — admin
// soft-delete of any timeline. Admin-only (visa_timeline.manage_any permission
// checked at route registration). Soft-delete per Rule 11.
func (h *Handlers) HandleAdminDelete(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}

	id, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	if h.service == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "visa_timeline_unavailable", "Visa Timeline service not configured")
		return
	}

	if err := h.service.AdminDelete(r.Context(), id); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "timeline_not_found", "Timeline not found")
			return
		}

		slog.Error("visatimeline: admin delete failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "admin_delete_failed", "Failed to delete timeline")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"deleted": true}})
}
