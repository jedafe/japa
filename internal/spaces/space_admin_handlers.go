package spaces

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/wallet"
)

// ─── Admin Space Extras (P5.6 — Task 12) ─────────────────────────────────

// ListJoinRequests handles GET /admin/spaces/:id/join-requests.
func (h *Handlers) ListJoinRequests(w http.ResponseWriter, r *http.Request) {
	spaceID, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", err.Error())
		return
	}

	limit := parsePageSize(r)
	offset := parsePageSizeOffset(r)
	rows, err := h.Service.read.ListSpaceJoinRequests(r.Context(), gen.ListSpaceJoinRequestsParams{
		SpaceID: spaceID, PageLimit: limit, PageOffset: offset,
	})
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to list join requests")
		return
	}

	requests := make([]map[string]any, 0, len(rows))
	for _, r := range rows {
		requests = append(requests, map[string]any{
			"user_id": r.UserID, "username": r.Username, "display_name": r.DisplayName, "avatar_url": r.AvatarUrl, "joined_at": r.JoinedAt, "status": r.Status,
		})
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": requests})
}

// UpdateJoinRequest handles PATCH /admin/spaces/:id/join-requests/:requestId.
// SPACES-004: when the request is approved, increment the space's
// member_count so the counter reflects the newly active member.
func (h *Handlers) UpdateJoinRequest(w http.ResponseWriter, r *http.Request) {
	spaceID, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", err.Error())
		return
	}

	requestID, err := parseUUIDParam(r, "uid")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_request_id", "Invalid request ID")
		return
	}

	var input struct {
		Status string `json:"status"` // "approved" or "rejected"
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	// Validate the transition — space_members.status is a constrained column;
	// an arbitrary value would corrupt membership state silently.
	if input.Status != "approved" && input.Status != "rejected" {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_status", "Status must be 'approved' or 'rejected'")
		return
	}

	// BUG-008/020: the update only affects status='pending' rows — a
	// missing or already-resolved request is a no-op, not a member_count
	// increment or a phantom approval.
	affected, err := h.Service.write.UpdateSpaceJoinRequest(r.Context(), gen.UpdateSpaceJoinRequestParams{
		SpaceID: spaceID,
		UserID:  requestID,
		Status:  input.Status,
	})
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to update join request")
		return
	}
	if affected == 0 {
		errpkg.WriteError(w, http.StatusConflict, "request_not_pending",
			"No pending join request for this user in this space")
		return
	}
	// SPACES-004: approval also flips the member row to 'approved' —
	// trg_space_member_count bumps member_count in the same transaction.
	if input.Status == "approved" {

		// §64: approval is the moment a pending requester becomes a member —
		// record space.joined so badge/challenge triggers fire for this
		// path too (parity with AdminResolveJoinRequest).
		h.Service.emitActivity(r.Context(), requestID, wallet.ActivityKeySpaceJoined, "space:"+spaceID.String()+":"+requestID.String(), nil)

		// BUG-009: join approval auto-follows the space (§05/§31) — the
		// POST approve path already did; the PATCH path must match.
		if h.follows != nil {
			if _, ferr := h.follows.EnsureFollow(r.Context(), requestID, "space", spaceID); ferr != nil {
				slog.Warn("join-approval: auto-follow failed",
					"space_id", spaceID.String(), "user_id", requestID.String(), "error", ferr)
			}
		}
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"id": requestID.String(), "status": input.Status}})
}

// UpdateSpaceStatus handles PATCH /admin/spaces/:id/status. The optional
// reason is recorded in audit_logs so a suspension decision is traceable;
// spaces.status is a constrained column, so arbitrary values are rejected.
func (h *Handlers) UpdateSpaceStatus(w http.ResponseWriter, r *http.Request) {
	spaceID, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", err.Error())
		return
	}

	var input struct {
		Status string `json:"status"`
		Reason string `json:"reason"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	switch input.Status {
	case StatusActive, StatusArchived, StatusDraft, "suspended":
	default:
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_status", "Status must be 'active', 'suspended', 'archived', or 'draft'")
		return
	}

	err = h.Service.write.UpdateSpaceStatus(r.Context(), gen.UpdateSpaceStatusParams{
		ID: spaceID,

		Status: input.Status,
	})
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to update space status")
		return
	}

	// Record the moderation decision — the reason is otherwise dropped.
	if user := auth.FromContext(r.Context()); user != nil && user.UserID.Valid {
		targetType := "space"
		metadata, _ := json.Marshal(map[string]any{
			"status": input.Status,
			"reason": strings.TrimSpace(input.Reason),
		})

		if _, aerr := h.Service.write.CreateAuditLog(r.Context(), gen.CreateAuditLogParams{
			ActorUserID: user.UserID,
			ActionType:  "space.status_update",
			TargetType:  &targetType,
			TargetID:    spaceID,
			Metadata:    metadata,
		}); aerr != nil {
			slog.Error("spaces: failed to write status-update audit log",
				"space_id", spaceID.String(), "error", aerr)
		}
	}

	// Suspend/archive/draft removes the space from discovery; re-activation
	// re-indexes it — otherwise a suspended space stayed searchable and
	// linked to a 404.
	if input.Status != StatusActive {
		h.Service.EnqueueSearchIndex(r.Context(), spaceID, "delete")
	} else {
		h.Service.EnqueueSearchIndex(r.Context(), spaceID, "index")
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"id": spaceID.String(), "status": input.Status}})
}

// GetSpaceAnalytics handles GET /admin/spaces/:id/analytics.
func (h *Handlers) GetSpaceAnalytics(w http.ResponseWriter, r *http.Request) {
	user := auth.FromContext(r.Context())
	if user == nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	// BUG-005: accept both UUID and slug. The catalog route is
	// /spaces/{id}/analytics, but the SSR space page passes the slug.
	// Try UUID first; if parsing fails, fall back to slug lookup.
	param := chi.URLParam(r, "id")

	var spaceID pgtype.UUID

	if err := spaceID.Scan(param); err != nil {
		// Not a UUID — try slug lookup.
		space, err := h.Service.GetSpaceBySlug(r.Context(), param)
		if err != nil {
			errpkg.WriteError(w, http.StatusNotFound, "space_not_found", "Space not found")
			return
		}

		spaceID = space.ID
	}

	// [auth, space-admin] per JEOS §215 — the space owner, an assigned
	// moderator, or a platform moderator holding spaces.moderate.any
	// (super_admin wildcard included) may view the analytics dashboard.
	// Guests are blocked by RequireAuth on the route; non-admin members
	// are blocked here.
	canManage, err := h.Service.CanManageSpace(r.Context(), spaceID, user.UserID)
	if err != nil {
		slog.Error("space stats: checking manage permission", "error", err, "space_id", spaceID.String())
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to authorize analytics")

		return
	}

	if !canManage && h.permChecker != nil &&
		h.permChecker.HasPermission(r.Context(), user.UserID, "spaces.moderate.any") {
		canManage = true
	}

	if !canManage {
		errpkg.WriteError(w, http.StatusForbidden, "analytics_forbidden", "Only space admins can view analytics")
		return
	}

	// BUG-020: single round-trip aggregate instead of 3 sequential queries.
	summary, err := h.Service.GetSpaceAnalyticsSummary(r.Context(), spaceID)
	if err != nil {
		slog.Error("space stats: loading analytics summary", "error", err, "space_id", spaceID.String())
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to load space stats")

		return
	}

	// Response keys match the Analytics tab contract on the space hub:
	// total_views / total_posts / total_members / total_followers /
	// posts_last_30d (Ref: sections/40-platform-analytics-reports.md).
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{
		"total_views":     summary.ViewCount,
		"total_posts":     summary.ContentCount,
		"total_members":   summary.MemberCount,
		"total_followers": summary.FollowerCount,
		"posts_last_30d":  summary.PostsLast30d,
	}})
}

// AdminListJoinRequests handles GET /api/v1/admin/join-requests — all
// pending join requests across every space in the admin's platform. Each row is
// given a composite id (space_id::user_id) so the panel can target a specific
// request for approval/rejection.
func (h *Handlers) AdminListJoinRequests(w http.ResponseWriter, r *http.Request) {
	limit := parsePageSize(r)
	offset := parsePageSizeOffset(r)
	rows, err := h.Service.read.ListAllSpaceJoinRequests(r.Context(), gen.ListAllSpaceJoinRequestsParams{
		PageLimit: limit, PageOffset: offset,
	})
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to list join requests")
		return
	}

	requests := make([]map[string]any, 0, len(rows))

	for _, row := range rows {
		compositeID := row.SpaceID.String() + "::" + row.UserID.String()

		entry := map[string]any{
			"id":           compositeID,
			"space_id":     row.SpaceID,
			"user_id":      row.UserID,
			"username":     row.Username,
			"display_name": row.DisplayName,
			"avatar_url":   row.AvatarUrl,
			"joined_at":    row.JoinedAt,
			"status":       row.Status,
		}
		if row.SpaceName != nil {
			entry["space_name"] = *row.SpaceName
		}

		requests = append(requests, entry)
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": requests})
}

// AdminResolveJoinRequest is the shared approve/reject implementation behind
// POST /api/v1/admin/join-requests/{id}/approve and /reject. The composite id
// encodes space_id::user_id, which is split and applied via the existing
// UpdateSpaceJoinRequest store call.
func (h *Handlers) AdminResolveJoinRequest(w http.ResponseWriter, r *http.Request, status string) {
	composite := chi.URLParam(r, "id")

	parts := strings.Split(composite, "::")
	if len(parts) != 2 {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid composite request id")
		return
	}

	var spaceID, userID pgtype.UUID
	if err := spaceID.Scan(parts[0]); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid space id")
		return
	}

	if err := userID.Scan(parts[1]); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid user id")
		return
	}

	affected, err := h.Service.write.UpdateSpaceJoinRequest(r.Context(), gen.UpdateSpaceJoinRequestParams{
		SpaceID: spaceID,
		UserID:  userID,
		Status:  status,
	})
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to update join request")
		return
	}
	if affected == 0 {
		errpkg.WriteError(w, http.StatusConflict, "request_not_pending",
			"No pending join request for this user in this space")
		return
	}

	// SPACES-004: approval flips the member row to 'approved' —
	// trg_space_member_count bumps member_count in the same transaction.
	if status == "approved" {

		// §64: approval is the moment a pending requester becomes a member —
		// record space.joined so badge/challenge triggers (e.g.
		// space_explorer) fire for the request-approval join path too.
		h.Service.emitActivity(r.Context(), userID, wallet.ActivityKeySpaceJoined, "space:"+spaceID.String()+":"+userID.String(), nil)

		// BUG-031 (follows audit): approval completes the join — the
		// join→follow contract applies on this path too, not just the
		// immediate open-join flow. Ensure semantics make it idempotent.
		if h.follows != nil {
			if _, ferr := h.follows.EnsureFollow(r.Context(), userID, "space", spaceID); ferr != nil {
				slog.Warn("join-approval: auto-follow failed",
					"space_id", spaceID.String(), "user_id", userID.String(), "error", ferr)
			}
		}
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"id": composite, "status": status}})
}

// AdminApproveJoinRequest handles POST /api/v1/admin/join-requests/{id}/approve.
func (h *Handlers) AdminApproveJoinRequest(w http.ResponseWriter, r *http.Request) {
	h.AdminResolveJoinRequest(w, r, "approved")
}

// AdminRejectJoinRequest handles POST /api/v1/admin/join-requests/{id}/reject.
func (h *Handlers) AdminRejectJoinRequest(w http.ResponseWriter, r *http.Request) {
	h.AdminResolveJoinRequest(w, r, "rejected")
}
