package spaces

// Ref: .jeos/sections/05-geo-first-space-system.md
// (Amendment addendum 2026-11-27: Space Hub Redesign — member invites are a
// custom-space feature; primary spaces are platform-owned and follow-only.)

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
)

// Notifier is the subset of notifications.Service the invite handler needs —
// CreateNotification writes the in-app row and fans out push/email per the
// recipient's preferences. nil-safe: a missing notifier skips the alert.
type Notifier interface {
	CreateNotification(ctx context.Context, recipientID pgtype.UUID, nType, actorType string, actorID pgtype.UUID, subjectType string, subjectID pgtype.UUID, title, body, actionURL string) error
}

// ---------------------------------------------------------------------------
// Invites (custom spaces only)
// ---------------------------------------------------------------------------

// CreateInvite handles POST /api/v1/spaces/{slug}/invites —
// { user_id | username } → pending space_invites row + notification.
func (h *Handlers) CreateInvite(w http.ResponseWriter, r *http.Request) {
	space, ok := spaceFromContext(w, r)
	if !ok {
		return
	}

	if h.rejectWikiOnly(w, r, space) {
		return
	}

	user := auth.FromContext(r.Context())
	if user == nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	// Invites are a member-created custom-space feature — primary spaces
	// (geo/university) are follow-only platform entities (§05).
	if isPrimarySpace(space) {
		errpkg.WriteError(w, http.StatusForbidden, "invites_not_allowed",
			"Primary spaces do not accept member invites — users follow them")
		return
	}

	// A private space is invisible to non-members; gate on membership first
	// so this endpoint cannot confirm the space exists to outsiders.
	isMember, err := h.Service.IsMember(r.Context(), space.ID, user.UserID)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	if !isMember {
		errpkg.WriteError(w, http.StatusForbidden, "not_a_member",
			"Only members can invite others to this space")
		return
	}

	var req struct {
		UserID   string `json:"user_id"`
		Username string `json:"username"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Request body is not valid JSON")
		return
	}

	var inviteeID pgtype.UUID
	switch {
	case strings.TrimSpace(req.UserID) != "":
		if err := inviteeID.Scan(strings.TrimSpace(req.UserID)); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid user_id format")
			return
		}
	case strings.TrimSpace(req.Username) != "":
		var uerr error
		inviteeID, uerr = h.Service.GetUserIDByUsername(r.Context(), strings.TrimSpace(req.Username))
		if uerr != nil {
			errpkg.WriteError(w, http.StatusNotFound, "user_not_found", "No user with that username was found")
			return
		}
	default:
		errpkg.WriteError(w, http.StatusBadRequest, "missing_fields", "user_id or username is required")
		return
	}

	if inviteeID == user.UserID {
		errpkg.WriteError(w, http.StatusBadRequest, "cannot_invite_self", "You cannot invite yourself")
		return
	}

	if inviteeIsMember, _ := h.Service.IsMember(r.Context(), space.ID, inviteeID); inviteeIsMember {
		errpkg.WriteError(w, http.StatusConflict, "already_member", "That user is already a member of this space")
		return
	}

	if _, found, err := h.Service.GetPendingSpaceInvite(r.Context(), space.ID, inviteeID); err != nil {
		handleServiceError(w, err)
		return
	} else if found {
		errpkg.WriteError(w, http.StatusConflict, "invite_pending", "That user already has a pending invite to this space")
		return
	}

	invite, err := h.Service.CreateSpaceInvite(r.Context(), space.ID, user.UserID, inviteeID)
	if err != nil {
		// A concurrent insert that passed our pre-check trips the pending
		// unique index — surface it as the same 409 rather than a 500.
		if strings.Contains(err.Error(), "space_invites_pending_unique") {
			errpkg.WriteError(w, http.StatusConflict, "invite_pending", "That user already has a pending invite to this space")
			return
		}
		handleServiceError(w, err)
		return
	}

	// Notify the invitee — the row carries actor_id (inviter) + subject
	// (space) so the notification centre renders "{inviter} invited you to
	// {space}". Best-effort: a notification failure must not fail the
	// invite itself (Rule 8 — dispatch errors are logged, never returned).
	if h.notifier != nil {
		if nerr := h.notifier.CreateNotification(r.Context(), inviteeID,
			"space_invite", "user", user.UserID,
			"space", space.ID,
			"Space invitation", "You have been invited to join "+space.Name,
			"/spaces/"+space.Slug); nerr != nil {
			slog.Warn("spaces: invite notification failed",
				"space_id", space.ID, "invitee_id", inviteeID, "error", nerr)
		}
	}

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": invite})
}

// ListInvites handles GET /api/v1/spaces/{slug}/invites — pending invite
// list for space admins (the router gates this on spaces.moderate).
func (h *Handlers) ListInvites(w http.ResponseWriter, r *http.Request) {
	space, ok := spaceFromContext(w, r)
	if !ok {
		return
	}

	limit := parsePageSize(r)
	offset := parsePageSizeOffset(r)

	rows, err := h.Service.ListSpaceInvites(r.Context(), space.ID, limit, offset)
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to list space invites")
		return
	}
	if rows == nil {
		rows = []gen.ListSpaceInvitesRow{}
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": rows})
}

// ListMySpaceInvites handles GET /api/v1/me/space-invites — the caller's
// pending invites across all spaces (notification action surface).
func (h *Handlers) ListMySpaceInvites(w http.ResponseWriter, r *http.Request) {
	user := auth.FromContext(r.Context())
	if user == nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	limit := parsePageSize(r)
	offset := parsePageSizeOffset(r)

	rows, err := h.Service.ListSpaceInvitesByInvitee(r.Context(), user.UserID, limit, offset)
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to list your space invites")
		return
	}
	if rows == nil {
		rows = []gen.ListSpaceInvitesByInviteeRow{}
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": rows})
}

// SetNotifier injects the notifications service used to alert invitees.
// nil disables notification dispatch (invite row still records).
func (h *Handlers) SetNotifier(n Notifier) { h.notifier = n }
