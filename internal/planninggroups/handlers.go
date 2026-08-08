// Package planninggroups — handlers.go provides the HTTP handlers for the
// Japa Planning Groups routes (§82, ADR-090).
//
// Routes:
//
//	GET    /api/v1/planning-groups                       — list public groups
//	GET    /api/v1/planning-groups/:id                   — single group (public)
//	POST   /api/v1/planning-groups                       — create (auth, planning_group.create)
//	POST   /api/v1/planning-groups/:id/join              — join (auth, planning_group.create)
//	DELETE /api/v1/planning-groups/:id/leave             — leave (auth)
//	PATCH  /api/v1/planning-groups/:id                   — update own (auth, planning_group.manage_own)
//	DELETE /api/v1/planning-groups/:id                   — delete own (auth, planning_group.manage_own)
//	GET    /api/v1/planning-groups/:id/milestones        — list own milestones (auth)
//	POST   /api/v1/planning-groups/:id/milestones        — create milestone (auth)
//	PATCH  /api/v1/planning-groups/:id/milestones/:mid   — update milestone (auth)
//	GET    /api/v1/planning-groups/:id/checkins          — list this week's checkins (auth)
//	POST   /api/v1/planning-groups/:id/checkins          — create checkin (auth)
//	POST   /api/v1/admin/planning-groups/:id/extend      — extend expiry (auth, planning_group.manage_any)
//
// Ref: japaconnect-engineering/sections/82-planning-groups.md
//      japaconnect-engineering/adr/ADR-090-planning-groups.md
package planninggroups

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

// Handlers exposes the planning-groups endpoints.
type Handlers struct {
	service  *Service
	settings *config.Settings
}

// NewHandlers creates planning-groups handlers bound to the provided service.
func NewHandlers(service *Service, settings *config.Settings) *Handlers {
	return &Handlers{service: service, settings: settings}
}

// ---------------------------------------------------------------------------
// Shared helpers (same pattern as story/handlers.go, wiki/handlers.go)
// ---------------------------------------------------------------------------

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("planninggroups: encode response", "error", err)
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
// Public handlers (list + single — anonymous browse per §82)
// ---------------------------------------------------------------------------

// HandleList handles GET /planning-groups — list public planning groups.
// Query params: ?destination_space_id=&visa=&page=&limit=
func (h *Handlers) HandleList(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	if h.service == nil || !h.service.Enabled(r.Context(), tenantID) {
		writeError(w, http.StatusNotFound, "planning_groups_disabled", "Planning groups are not available")
		return
	}

	var destSpaceID pgtype.UUID
	if v := r.URL.Query().Get("destination_space_id"); v != "" {
		if err := destSpaceID.Scan(v); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_id", "Invalid destination_space_id")
			return
		}
	}

	visaCategory := r.URL.Query().Get("visa")
	page, limit := parsePageParams(r)

	groups, total, err := h.service.List(r.Context(), tenantID, destSpaceID, visaCategory, page, limit)
	if err != nil {
		slog.Error("planninggroups: list failed", "error", err)
		writeError(w, http.StatusInternalServerError, "list_failed", "Failed to list planning groups")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": groups,
		"pagination": map[string]any{
			"page":  page,
			"limit": limit,
			"total": total,
		},
	})
}

// HandleGet handles GET /planning-groups/:id — single planning group (public).
func (h *Handlers) HandleGet(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	if h.service == nil || !h.service.Enabled(r.Context(), tenantID) {
		writeError(w, http.StatusNotFound, "planning_groups_disabled", "Planning groups are not available")
		return
	}

	groupID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	group, err := h.service.Get(r.Context(), groupID, tenantID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "group_not_found", "Planning group not found")
			return
		}
		slog.Error("planninggroups: get failed", "error", err)
		writeError(w, http.StatusInternalServerError, "get_failed", "Failed to get planning group")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": group})
}

// ---------------------------------------------------------------------------
// Authenticated handlers (create, join, leave — auth required)
// ---------------------------------------------------------------------------

// HandleCreate handles POST /planning-groups — create a new planning group.
// Auth required (planning_group.create permission checked at route registration).
func (h *Handlers) HandleCreate(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	if h.service == nil || !h.service.Enabled(r.Context(), tenantID) {
		writeError(w, http.StatusNotFound, "planning_groups_disabled", "Planning groups are not available")
		return
	}

	var input CreateGroupInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if input.Name == "" {
		writeError(w, http.StatusBadRequest, "missing_name", "Name is required")
		return
	}
	if input.DestinationSpaceID == "" {
		writeError(w, http.StatusBadRequest, "missing_destination", "Destination space ID is required")
		return
	}

	group, err := h.service.Create(r.Context(), tenantID, user.UserID, input)
	if err != nil {
		slog.Error("planninggroups: create failed", "error", err)
		writeError(w, http.StatusInternalServerError, "create_failed", "Failed to create planning group")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{"data": group})
}

// HandleJoin handles POST /planning-groups/:id/join — join a planning group.
// Auth required (planning_group.create permission checked at route registration).
func (h *Handlers) HandleJoin(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}
	groupID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	if h.service == nil || !h.service.Enabled(r.Context(), tenantID) {
		writeError(w, http.StatusNotFound, "planning_groups_disabled", "Planning groups are not available")
		return
	}

	var body struct {
		DepartureDate string `json:"departure_date"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)

	if err := h.service.Join(r.Context(), groupID, user.UserID, tenantID, body.DepartureDate); err != nil {
		slog.Error("planninggroups: join failed", "error", err)
		writeError(w, http.StatusInternalServerError, "join_failed", "Failed to join planning group")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"joined": true}})
}

// HandleLeave handles DELETE /planning-groups/:id/leave — leave a group.
// Auth required.
func (h *Handlers) HandleLeave(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}
	groupID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	if h.service == nil || !h.service.Enabled(r.Context(), tenantID) {
		writeError(w, http.StatusNotFound, "planning_groups_disabled", "Planning groups are not available")
		return
	}

	if err := h.service.Leave(r.Context(), groupID, user.UserID, tenantID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "membership_not_found", "You are not a member of this group")
			return
		}
		slog.Error("planninggroups: leave failed", "error", err)
		writeError(w, http.StatusInternalServerError, "leave_failed", "Failed to leave planning group")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"left": true}})
}

// HandleGetMembers handles GET /planning-groups/:id/members — list members.
// Auth required (route registration enforces auth; the list is visible to
// authenticated members of the community).
func (h *Handlers) HandleGetMembers(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}
	groupID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	if h.service == nil || !h.service.Enabled(r.Context(), tenantID) {
		writeError(w, http.StatusNotFound, "planning_groups_disabled", "Planning groups are not available")
		return
	}

	members, err := h.service.GetMembers(r.Context(), groupID, tenantID)
	if err != nil {
		slog.Error("planninggroups: get members failed", "error", err)
		writeError(w, http.StatusInternalServerError, "members_failed", "Failed to get group members")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": members})
}

// ---------------------------------------------------------------------------
// Authenticated handlers (update, delete — planning_group.manage_own)
// ---------------------------------------------------------------------------

// HandleUpdate handles PATCH /planning-groups/:id — update own group. Auth
// required (planning_group.manage_own permission checked at route registration).
func (h *Handlers) HandleUpdate(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}
	groupID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	if h.service == nil || !h.service.Enabled(r.Context(), tenantID) {
		writeError(w, http.StatusNotFound, "planning_groups_disabled", "Planning groups are not available")
		return
	}

	var input UpdateGroupInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	group, err := h.service.UpdateOwn(r.Context(), groupID, tenantID, user.UserID, input)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "group_not_found", "Planning group not found")
			return
		}
		if errors.Is(err, ErrNotOwner) {
			writeError(w, http.StatusForbidden, "not_owner", "You can only update your own planning groups")
			return
		}
		slog.Error("planninggroups: update failed", "error", err)
		writeError(w, http.StatusInternalServerError, "update_failed", "Failed to update planning group")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": group})
}

// HandleDelete handles DELETE /planning-groups/:id — delete own group. Auth
// required (planning_group.manage_own permission checked at route registration).
// Soft-delete per Rule 11.
func (h *Handlers) HandleDelete(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}
	groupID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	if h.service == nil || !h.service.Enabled(r.Context(), tenantID) {
		writeError(w, http.StatusNotFound, "planning_groups_disabled", "Planning groups are not available")
		return
	}

	if err := h.service.DeleteOwn(r.Context(), groupID, tenantID, user.UserID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "group_not_found", "Planning group not found")
			return
		}
		if errors.Is(err, ErrNotOwner) {
			writeError(w, http.StatusForbidden, "not_owner", "You can only delete your own planning groups")
			return
		}
		slog.Error("planninggroups: delete failed", "error", err)
		writeError(w, http.StatusInternalServerError, "delete_failed", "Failed to delete planning group")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"deleted": true}})
}

// ---------------------------------------------------------------------------
// Authenticated handlers (milestones — auth required)
// ---------------------------------------------------------------------------

// HandleListMilestones handles GET /planning-groups/:id/milestones — list the
// requesting user's milestones in a group. Auth required.
func (h *Handlers) HandleListMilestones(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}
	groupID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	if h.service == nil || !h.service.Enabled(r.Context(), tenantID) {
		writeError(w, http.StatusNotFound, "planning_groups_disabled", "Planning groups are not available")
		return
	}

	milestones, err := h.service.ListMilestones(r.Context(), groupID, user.UserID, tenantID)
	if err != nil {
		slog.Error("planninggroups: list milestones failed", "error", err)
		writeError(w, http.StatusInternalServerError, "list_milestones_failed", "Failed to list milestones")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": milestones})
}

// HandleCreateMilestone handles POST /planning-groups/:id/milestones — create
// a milestone for the requesting user. Auth required.
func (h *Handlers) HandleCreateMilestone(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}
	groupID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	if h.service == nil || !h.service.Enabled(r.Context(), tenantID) {
		writeError(w, http.StatusNotFound, "planning_groups_disabled", "Planning groups are not available")
		return
	}

	var body struct {
		Title string `json:"title"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}
	if body.Title == "" {
		writeError(w, http.StatusBadRequest, "missing_title", "Title is required")
		return
	}

	milestone, err := h.service.CreateMilestone(r.Context(), groupID, user.UserID, tenantID, body.Title)
	if err != nil {
		slog.Error("planninggroups: create milestone failed", "error", err)
		writeError(w, http.StatusInternalServerError, "create_milestone_failed", "Failed to create milestone")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{"data": milestone})
}

// HandleUpdateMilestone handles PATCH /planning-groups/:id/milestones/:mid —
// update the completed state of a milestone. Auth required.
func (h *Handlers) HandleUpdateMilestone(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}
	milestoneID, ok := scanUUIDParam(w, r, "mid")
	if !ok {
		return
	}

	if h.service == nil || !h.service.Enabled(r.Context(), tenantID) {
		writeError(w, http.StatusNotFound, "planning_groups_disabled", "Planning groups are not available")
		return
	}

	var body struct {
		Completed bool `json:"completed"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if err := h.service.UpdateMilestone(r.Context(), milestoneID, tenantID, body.Completed); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "milestone_not_found", "Milestone not found")
			return
		}
		slog.Error("planninggroups: update milestone failed", "error", err)
		writeError(w, http.StatusInternalServerError, "update_milestone_failed", "Failed to update milestone")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"updated": true}})
}

// ---------------------------------------------------------------------------
// Authenticated handlers (check-ins — auth required)
// ---------------------------------------------------------------------------

// HandleListCheckins handles GET /planning-groups/:id/checkins — list this
// week's check-ins for a group. Auth required.
func (h *Handlers) HandleListCheckins(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}
	groupID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	if h.service == nil || !h.service.Enabled(r.Context(), tenantID) {
		writeError(w, http.StatusNotFound, "planning_groups_disabled", "Planning groups are not available")
		return
	}

	checkins, err := h.service.ListCheckins(r.Context(), groupID, tenantID)
	if err != nil {
		slog.Error("planninggroups: list checkins failed", "error", err)
		writeError(w, http.StatusInternalServerError, "list_checkins_failed", "Failed to list check-ins")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": checkins})
}

// HandleCreateCheckin handles POST /planning-groups/:id/checkins — create a
// weekly check-in for the requesting user. Auth required.
func (h *Handlers) HandleCreateCheckin(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}
	groupID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	if h.service == nil || !h.service.Enabled(r.Context(), tenantID) {
		writeError(w, http.StatusNotFound, "planning_groups_disabled", "Planning groups are not available")
		return
	}

	var body struct {
		Note string `json:"note"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}
	if body.Note == "" {
		writeError(w, http.StatusBadRequest, "missing_note", "Note is required")
		return
	}

	checkin, err := h.service.CreateCheckin(r.Context(), groupID, user.UserID, tenantID, body.Note)
	if err != nil {
		slog.Error("planninggroups: create checkin failed", "error", err)
		writeError(w, http.StatusInternalServerError, "create_checkin_failed", "Failed to create check-in")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{"data": checkin})
}

// ---------------------------------------------------------------------------
// Admin handler (extend expiry — planning_group.manage_any)
// ---------------------------------------------------------------------------

// HandleAdminExtend handles POST /admin/planning-groups/:id/extend — extend
// the expiry of a planning group. Admin-only (planning_group.manage_any
// permission checked at route registration).
func (h *Handlers) HandleAdminExtend(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}
	groupID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	if h.service == nil {
		writeError(w, http.StatusServiceUnavailable, "planning_groups_unavailable", "Planning groups service not configured")
		return
	}

	var body struct {
		ExpiresAt string `json:"expires_at"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}
	if body.ExpiresAt == "" {
		writeError(w, http.StatusBadRequest, "missing_expires_at", "expires_at is required")
		return
	}

	if err := h.service.AdminExtendExpiry(r.Context(), groupID, tenantID, body.ExpiresAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "group_not_found", "Planning group not found")
			return
		}
		slog.Error("planninggroups: admin extend failed", "error", err)
		writeError(w, http.StatusInternalServerError, "extend_failed", "Failed to extend planning group expiry")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"extended": true}})
}
