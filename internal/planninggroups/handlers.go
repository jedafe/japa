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
//
//	japaconnect-engineering/adr/ADR-090-planning-groups.md
package planninggroups

import (
	"context"
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
	"github.com/jedafe/japa/internal/literals"
)

// literalResolver resolves {{...}} template literals in user-facing text
// fields (§14). Implemented by *literals.Catalog; nil disables resolution.
type literalResolver interface {
	ResolveFields(ctx context.Context, v any, rc literals.RenderContext)
}

// Handlers exposes the planning-groups endpoints.
type Handlers struct {
	service  *Service
	settings *config.Settings
	lit      literalResolver // nil disables literal resolution (§14)
}

// NewHandlers creates planning-groups handlers bound to the provided service.
func NewHandlers(service *Service, settings *config.Settings) *Handlers {
	return &Handlers{service: service, settings: settings}
}

// SetLiteralResolver injects the template-literal catalog so group
// names/descriptions, milestone titles, and check-in notes render {{...}}
// placeholders (§14).
func (h *Handlers) SetLiteralResolver(l literalResolver) { h.lit = l }

// resolveLiterals resolves {{...}} placeholders on a DTO or slice.
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

// ---------------------------------------------------------------------------
// Shared helpers (same pattern as story/handlers.go, wiki/handlers.go)
// ---------------------------------------------------------------------------

func requireUser(w http.ResponseWriter, r *http.Request) (*auth.AuthenticatedUser, bool) {
	u := auth.FromContext(r.Context())
	if u == nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return nil, false
	}

	return u, true
}

// requireMembership checks that the requesting user is a member of the given
// planning group. Returns true if the user is a member; otherwise writes a
// 403 error and returns false. §82: "Group check-ins and milestone details
// are visible only to group members.".
func (h *Handlers) requireMembership(w http.ResponseWriter, r *http.Request, groupID pgtype.UUID) (*auth.AuthenticatedUser, bool) {
	user, ok := requireUser(w, r)
	if !ok {
		return nil, false
	}

	isMember, err := h.service.IsMember(r.Context(), groupID, user.UserID)
	if err != nil {
		slog.Error("planninggroups: membership check failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "membership_check_failed", "Failed to verify membership")

		return nil, false
	}

	if !isMember {
		errpkg.WriteError(w, http.StatusForbidden, "not_member", "You must be a member of this group to perform this action")
		return nil, false
	}

	return user, true
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
	// Accept both "limit" (legacy) and "page_size" (convention standard).
	pageSize := r.URL.Query().Get("page_size")
	if pageSize == "" {
		pageSize = r.URL.Query().Get("limit")
	}

	if pageSize != "" {
		if n, err := strconv.Atoi(pageSize); err == nil && n > 0 && n <= 100 {
			limit = int32(n) // #nosec
		}
	}

	return page, limit
}

// ---------------------------------------------------------------------------
// Public handlers (list + single — anonymous browse per §82)
// ---------------------------------------------------------------------------

// HandleList handles GET /planning-groups — list public planning groups.
// Query params: ?dest=&visa=&page=&limit= (also accepts ?destination_space_id= as legacy alias).
func (h *Handlers) HandleList(w http.ResponseWriter, r *http.Request) {

	if h.service == nil || !h.service.Enabled(r.Context()) {
		errpkg.WriteError(w, http.StatusNotFound, "planning_groups_disabled", "Planning groups are not available")
		return
	}

	var destSpaceID pgtype.UUID
	// Accept ?dest= (geo-aware standard) or ?destination_space_id= (legacy alias).
	destParam := r.URL.Query().Get("dest")
	if destParam == "" {
		destParam = r.URL.Query().Get("destination_space_id")
	}

	if destParam != "" {
		if err := destSpaceID.Scan(destParam); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid destination space ID")
			return
		}
	}

	visaCategory := r.URL.Query().Get("visa")
	page, limit := parsePageParams(r)

	groups, total, err := h.service.List(r.Context(), destSpaceID, visaCategory, page, limit)
	if err != nil {
		slog.Error("planninggroups: list failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "list_failed", "Failed to list planning groups")

		return
	}

	h.resolveLiterals(r, &groups)

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
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

	if h.service == nil || !h.service.Enabled(r.Context()) {
		errpkg.WriteError(w, http.StatusNotFound, "planning_groups_disabled", "Planning groups are not available")
		return
	}

	groupID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	group, err := h.service.Get(r.Context(), groupID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "group_not_found", "Planning group not found")
			return
		}

		slog.Error("planninggroups: get failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "get_failed", "Failed to get planning group")

		return
	}

	h.resolveLiterals(r, &group)

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": group})
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

	if h.service == nil || !h.service.Enabled(r.Context()) {
		errpkg.WriteError(w, http.StatusNotFound, "planning_groups_disabled", "Planning groups are not available")
		return
	}

	var input CreateGroupInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if input.Name == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_name", "Name is required")
		return
	}

	if input.DestinationSpaceID == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_destination", "Destination space ID is required")
		return
	}

	group, err := h.service.Create(r.Context(), user.UserID, input)
	if err != nil {
		slog.Error("planninggroups: create failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "create_failed", "Failed to create planning group")

		return
	}

	h.resolveLiterals(r, &group)

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": group})
}

// HandleJoin handles POST /planning-groups/:id/join — join a planning group.
// Auth required (planning_group.create permission checked at route registration).
func (h *Handlers) HandleJoin(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	groupID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	if h.service == nil || !h.service.Enabled(r.Context()) {
		errpkg.WriteError(w, http.StatusNotFound, "planning_groups_disabled", "Planning groups are not available")
		return
	}

	var body struct {
		DepartureDate string `json:"departure_date"`
	}

	_ = json.NewDecoder(r.Body).Decode(&body)

	if err := h.service.Join(r.Context(), groupID, user.UserID, body.DepartureDate); err != nil {
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			errpkg.WriteError(w, http.StatusNotFound, "group_not_found", "Planning group not found")
		case errors.Is(err, ErrGroupNotOpen):
			errpkg.WriteError(w, http.StatusForbidden, "group_not_open", "This planning group is invite-only")
		case errors.Is(err, ErrGroupFull):
			errpkg.WriteError(w, http.StatusConflict, "group_full", "This planning group is at capacity")
		case errors.Is(err, ErrAlreadyMember):
			errpkg.WriteError(w, http.StatusConflict, "already_member", "You are already a member of this group")
		default:
			slog.Error("planninggroups: join failed", "error", err)
			errpkg.WriteError(w, http.StatusInternalServerError, "join_failed", "Failed to join planning group")
		}

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"joined": true}})
}

// HandleInvite handles POST /planning-groups/:id/invite — invite a user to
// join a planning group (owner only). Auth required (planning_group.manage_own
// permission checked at route registration).
func (h *Handlers) HandleInvite(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	groupID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	if h.service == nil || !h.service.Enabled(r.Context()) {
		errpkg.WriteError(w, http.StatusNotFound, "planning_groups_disabled", "Planning groups are not available")
		return
	}

	var body struct {
		UserID string `json:"user_id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if body.UserID == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_user_id", "user_id is required")
		return
	}

	var inviteeID pgtype.UUID
	if err := inviteeID.Scan(body.UserID); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_user_id", "Invalid user_id")
		return
	}

	// Verify the requesting user is the group owner.
	group, err := h.service.Get(r.Context(), groupID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "group_not_found", "Planning group not found")
			return
		}

		slog.Error("planninggroups: invite: get failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "invite_failed", "Failed to invite user")

		return
	}

	if group.OwnerID != user.UserID.String() {
		errpkg.WriteError(w, http.StatusForbidden, "not_owner", "Only the group owner can invite users")
		return
	}

	if err := h.service.InviteMember(r.Context(), groupID, inviteeID); err != nil {
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			errpkg.WriteError(w, http.StatusNotFound, "group_not_found", "Planning group not found")
		case errors.Is(err, ErrGroupFull):
			errpkg.WriteError(w, http.StatusConflict, "group_full", "This planning group is at capacity")
		default:
			slog.Error("planninggroups: invite failed", "error", err)
			errpkg.WriteError(w, http.StatusInternalServerError, "invite_failed", "Failed to invite user")
		}

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"invited": true}})
}

// HandleLeave handles DELETE /planning-groups/:id/leave — leave a group.
// Auth required.
func (h *Handlers) HandleLeave(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	groupID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	if h.service == nil || !h.service.Enabled(r.Context()) {
		errpkg.WriteError(w, http.StatusNotFound, "planning_groups_disabled", "Planning groups are not available")
		return
	}

	if err := h.service.Leave(r.Context(), groupID, user.UserID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "membership_not_found", "You are not a member of this group")
			return
		}

		slog.Error("planninggroups: leave failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "leave_failed", "Failed to leave planning group")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"left": true}})
}

// HandleGetMembers handles GET /planning-groups/:id/members — list members.
// Auth + membership required (§82: group content is members-only).
func (h *Handlers) HandleGetMembers(w http.ResponseWriter, r *http.Request) {

	groupID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	if h.service == nil || !h.service.Enabled(r.Context()) {
		errpkg.WriteError(w, http.StatusNotFound, "planning_groups_disabled", "Planning groups are not available")
		return
	}

	if _, ok := h.requireMembership(w, r, groupID); !ok {
		return
	}

	members, err := h.service.GetMembers(r.Context(), groupID)
	if err != nil {
		slog.Error("planninggroups: get members failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "members_failed", "Failed to get group members")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": members})
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

	groupID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	if h.service == nil || !h.service.Enabled(r.Context()) {
		errpkg.WriteError(w, http.StatusNotFound, "planning_groups_disabled", "Planning groups are not available")
		return
	}

	var input UpdateGroupInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	group, err := h.service.UpdateOwn(r.Context(), groupID, user.UserID, input)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "group_not_found", "Planning group not found")
			return
		}

		if errors.Is(err, ErrNotOwner) {
			errpkg.WriteError(w, http.StatusForbidden, "not_owner", "You can only update your own planning groups")
			return
		}

		slog.Error("planninggroups: update failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "update_failed", "Failed to update planning group")

		return
	}

	h.resolveLiterals(r, &group)

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": group})
}

// HandleDelete handles DELETE /planning-groups/:id — delete own group. Auth
// required (planning_group.manage_own permission checked at route registration).
// Soft-delete per Rule 11.
func (h *Handlers) HandleDelete(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	groupID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	if h.service == nil || !h.service.Enabled(r.Context()) {
		errpkg.WriteError(w, http.StatusNotFound, "planning_groups_disabled", "Planning groups are not available")
		return
	}

	if err := h.service.DeleteOwn(r.Context(), groupID, user.UserID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "group_not_found", "Planning group not found")
			return
		}

		if errors.Is(err, ErrNotOwner) {
			errpkg.WriteError(w, http.StatusForbidden, "not_owner", "You can only delete your own planning groups")
			return
		}

		slog.Error("planninggroups: delete failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "delete_failed", "Failed to delete planning group")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"deleted": true}})
}

// ---------------------------------------------------------------------------
// Authenticated handlers (milestones — auth required)
// ---------------------------------------------------------------------------

// HandleListMilestones handles GET /planning-groups/:id/milestones — list the
// requesting user's milestones in a group. Auth + membership required
// (§82: milestone details are visible only to group members).
func (h *Handlers) HandleListMilestones(w http.ResponseWriter, r *http.Request) {

	groupID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	if h.service == nil || !h.service.Enabled(r.Context()) {
		errpkg.WriteError(w, http.StatusNotFound, "planning_groups_disabled", "Planning groups are not available")
		return
	}

	user, ok := h.requireMembership(w, r, groupID)
	if !ok {
		return
	}

	milestones, err := h.service.ListMilestones(r.Context(), groupID, user.UserID)
	if err != nil {
		slog.Error("planninggroups: list milestones failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "list_milestones_failed", "Failed to list milestones")

		return
	}

	h.resolveLiterals(r, &milestones)

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": milestones})
}

// HandleCreateMilestone handles POST /planning-groups/:id/milestones — create
// a milestone for the requesting user. Auth + membership required.
func (h *Handlers) HandleCreateMilestone(w http.ResponseWriter, r *http.Request) {

	groupID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	if h.service == nil || !h.service.Enabled(r.Context()) {
		errpkg.WriteError(w, http.StatusNotFound, "planning_groups_disabled", "Planning groups are not available")
		return
	}

	user, ok := h.requireMembership(w, r, groupID)
	if !ok {
		return
	}

	var body struct {
		Title string `json:"title"`
	}

	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if body.Title == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_title", "Title is required")
		return
	}

	milestone, err := h.service.CreateMilestone(r.Context(), groupID, user.UserID, body.Title)
	if err != nil {
		slog.Error("planninggroups: create milestone failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "create_milestone_failed", "Failed to create milestone")

		return
	}

	h.resolveLiterals(r, &milestone)

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": milestone})
}

// HandleUpdateMilestone handles PATCH /planning-groups/:id/milestones/:mid —
// update the completed state of a milestone. Auth + ownership required
// (§82: milestones are personal; only the owner can complete their own).
func (h *Handlers) HandleUpdateMilestone(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	groupID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	milestoneID, ok := scanUUIDParam(w, r, "mid")
	if !ok {
		return
	}

	if h.service == nil || !h.service.Enabled(r.Context()) {
		errpkg.WriteError(w, http.StatusNotFound, "planning_groups_disabled", "Planning groups are not available")
		return
	}

	var body struct {
		Completed bool `json:"completed"`
	}

	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if err := h.service.UpdateMilestone(r.Context(), milestoneID, groupID, user.UserID, body.Completed); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "milestone_not_found", "Milestone not found")
			return
		}

		slog.Error("planninggroups: update milestone failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "update_milestone_failed", "Failed to update milestone")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"updated": true}})
}

// ---------------------------------------------------------------------------
// Authenticated handlers (check-ins — auth required)
// ---------------------------------------------------------------------------

// HandleListCheckins handles GET /planning-groups/:id/checkins — list this
// week's check-ins for a group. Auth + membership required (§82: group
// check-ins are visible only to group members).
func (h *Handlers) HandleListCheckins(w http.ResponseWriter, r *http.Request) {

	groupID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	if h.service == nil || !h.service.Enabled(r.Context()) {
		errpkg.WriteError(w, http.StatusNotFound, "planning_groups_disabled", "Planning groups are not available")
		return
	}

	if _, ok := h.requireMembership(w, r, groupID); !ok {
		return
	}

	checkins, err := h.service.ListCheckins(r.Context(), groupID)
	if err != nil {
		slog.Error("planninggroups: list checkins failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "list_checkins_failed", "Failed to list check-ins")

		return
	}

	h.resolveLiterals(r, &checkins)

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": checkins})
}

// HandleCreateCheckin handles POST /planning-groups/:id/checkins — create a
// weekly check-in for the requesting user. Auth + membership required.
func (h *Handlers) HandleCreateCheckin(w http.ResponseWriter, r *http.Request) {

	groupID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	if h.service == nil || !h.service.Enabled(r.Context()) {
		errpkg.WriteError(w, http.StatusNotFound, "planning_groups_disabled", "Planning groups are not available")
		return
	}

	user, ok := h.requireMembership(w, r, groupID)
	if !ok {
		return
	}

	var body struct {
		Note string `json:"note"`
	}

	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if body.Note == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_note", "Note is required")
		return
	}

	checkin, err := h.service.CreateCheckin(r.Context(), groupID, user.UserID, body.Note)
	if err != nil {
		switch {
		case errors.Is(err, ErrAlreadyCheckedIn):
			errpkg.WriteError(w, http.StatusConflict, "already_checked_in", "You have already checked in this week")
		case errors.Is(err, ErrCheckinTooLong):
			errpkg.WriteError(w, http.StatusBadRequest, "checkin_too_long", "Check-in note must be 200 characters or fewer")
		default:
			slog.Error("planninggroups: create checkin failed", "error", err)
			errpkg.WriteError(w, http.StatusInternalServerError, "create_checkin_failed", "Failed to create check-in")
		}

		return
	}

	h.resolveLiterals(r, &checkin)

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": checkin})
}

// ---------------------------------------------------------------------------
// Admin handler (list + extend expiry — planning_group.manage_any)
// ---------------------------------------------------------------------------

// HandleAdminList handles GET /admin/planning-groups — list all planning
// groups for the admin review queue. Admin-only (planning_group.manage_any
// permission checked at route registration). Supports ?page=&limit=&q=&dest=.
func (h *Handlers) HandleAdminList(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}

	if h.service == nil || !h.service.Enabled(r.Context()) {
		errpkg.WriteError(w, http.StatusNotFound, "planning_groups_disabled", "Planning groups are not available")
		return
	}

	page, limit := parsePageParams(r)
	offset := (page - 1) * limit
	search := r.URL.Query().Get("q")

	var destSpaceID pgtype.UUID
	if destParam := r.URL.Query().Get("dest"); destParam != "" {
		if err := destSpaceID.Scan(destParam); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid destination space ID")
			return
		}
	}

	groups, total, err := h.service.AdminListGroups(r.Context(), AdminListGroupsFilter{

		DestinationSpaceID: destSpaceID,
		Search:             search,
		Limit:              limit,
		Offset:             offset,
	})
	if err != nil {
		slog.Error("planninggroups: admin list failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "admin_list_failed", "Failed to list planning groups")

		return
	}

	h.resolveLiterals(r, &groups)

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": groups,
		"pagination": map[string]any{
			"page":  page,
			"limit": limit,
			"total": total,
		},
	})
}

// HandleAdminExtend handles POST /admin/planning-groups/:id/extend — extend
// the expiry of a planning group. Admin-only (planning_group.manage_any
// permission checked at route registration). Accepts { days: int } (extends
// from the later of current expires_at or now) or { expires_at: RFC3339 }
// for an explicit new expiry.
func (h *Handlers) HandleAdminExtend(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}

	groupID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	if h.service == nil || !h.service.Enabled(r.Context()) {
		errpkg.WriteError(w, http.StatusNotFound, "planning_groups_disabled", "Planning groups are not available")
		return
	}

	var body struct {
		Days      *int   `json:"days"`
		ExpiresAt string `json:"expires_at"`
	}

	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if body.Days != nil && *body.Days > 0 {
		if err := h.service.AdminExtendExpiryByDays(r.Context(), groupID, *body.Days); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				errpkg.WriteError(w, http.StatusNotFound, "group_not_found", "Planning group not found")
				return
			}

			slog.Error("planninggroups: admin extend failed", "error", err)
			errpkg.WriteError(w, http.StatusInternalServerError, "extend_failed", "Failed to extend planning group expiry")

			return
		}

		errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"extended": true}})

		return
	}

	if body.ExpiresAt == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_days_or_expires_at", "Either days or expires_at is required")
		return
	}

	if err := h.service.AdminExtendExpiry(r.Context(), groupID, body.ExpiresAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "group_not_found", "Planning group not found")
			return
		}

		slog.Error("planninggroups: admin extend failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "extend_failed", "Failed to extend planning group expiry")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"extended": true}})
}
