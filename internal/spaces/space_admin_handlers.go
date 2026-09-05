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
)

// ─── Admin Space Extras (P5.6 — Task 12) ─────────────────────────────────

// ListJoinRequests handles GET /admin/spaces/:id/join-requests.
func (h *Handlers) ListJoinRequests(w http.ResponseWriter, r *http.Request) {
	spaceID, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", err.Error())
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	rows, err := h.Service.read.ListSpaceJoinRequests(r.Context(), gen.ListSpaceJoinRequestsParams{
		SpaceID:  spaceID,
		TenantID: tenantID,
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

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	err = h.Service.write.UpdateSpaceJoinRequest(r.Context(), gen.UpdateSpaceJoinRequestParams{
		SpaceID:  spaceID,
		UserID:   requestID,
		Status:   input.Status,
		TenantID: tenantID,
	})
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to update join request")
		return
	}
	// SPACES-004: increment member_count only on approval. The pending
	// join path did not increment, so the approval must.
	if input.Status == "approved" {
		if err := h.Service.IncrementSpaceCount(r.Context(), spaceID, tenantID, 1); err != nil {
			slog.Error("spaces: failed to increment member_count on join approval",
				"space_id", spaceID.String(), "user_id", requestID.String(), "error", err)
		}
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"id": requestID.String(), "status": input.Status})
}

// UpdateSpaceStatus handles PATCH /admin/spaces/:id/status.
func (h *Handlers) UpdateSpaceStatus(w http.ResponseWriter, r *http.Request) {
	spaceID, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", err.Error())
		return
	}

	var input struct {
		Status string `json:"status"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	err = h.Service.write.UpdateSpaceStatus(r.Context(), gen.UpdateSpaceStatusParams{
		ID:       spaceID,
		TenantID: tenantID,
		Status:   input.Status,
	})
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to update space status")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"id": spaceID.String(), "status": input.Status})
}

// GetSpaceAnalytics handles GET /admin/spaces/:id/analytics.
func (h *Handlers) GetSpaceAnalytics(w http.ResponseWriter, r *http.Request) {
	user := auth.FromContext(r.Context())
	if user == nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	// BUG-005: accept both UUID and slug. The catalog route is
	// /spaces/{id}/analytics, but the SSR space page passes the slug.
	// Try UUID first; if parsing fails, fall back to slug lookup.
	param := chi.URLParam(r, "id")

	var spaceID pgtype.UUID

	if err := spaceID.Scan(param); err != nil {
		// Not a UUID — try slug lookup.
		space, err := h.Service.GetSpaceBySlug(r.Context(), param, tenantID)
		if err != nil {
			errpkg.WriteError(w, http.StatusNotFound, "space_not_found", "Space not found")
			return
		}

		spaceID = space.ID
	}

	// [auth, space-admin] per JEOS §215 — only the space owner or an assigned
	// moderator may view the analytics dashboard. Guests are blocked by
	// RequireAuth on the route; non-admin members are blocked here.
	canManage, err := h.Service.CanManageSpace(r.Context(), spaceID, user.UserID, tenantID)
	if err != nil {
		slog.Error("space stats: checking manage permission", "error", err, "space_id", spaceID.String())
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to authorize analytics")

		return
	}

	if !canManage {
		errpkg.WriteError(w, http.StatusForbidden, "analytics_forbidden", "Only space admins can view analytics")
		return
	}

	// BUG-020: single round-trip aggregate instead of 3 sequential queries.
	summary, err := h.Service.GetSpaceAnalyticsSummary(r.Context(), spaceID, tenantID)
	if err != nil {
		slog.Error("space stats: loading analytics summary", "error", err, "space_id", spaceID.String())
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to load space stats")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{
		"member_count":  summary.MemberCount,
		"content_count": summary.ContentCount,
		"view_count":    summary.ViewCount,
	}})
}

// AdminListJoinRequests handles GET /api/v1/admin/join-requests — all
// pending join requests across every space in the admin's tenant. Each row is
// given a composite id (space_id::user_id) so the panel can target a specific
// request for approval/rejection.
func (h *Handlers) AdminListJoinRequests(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	rows, err := h.Service.read.ListAllSpaceJoinRequests(r.Context(), tenantID)
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
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

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

	if err := h.Service.write.UpdateSpaceJoinRequest(r.Context(), gen.UpdateSpaceJoinRequestParams{
		SpaceID:  spaceID,
		UserID:   userID,
		Status:   status,
		TenantID: tenantID,
	}); err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to update join request")
		return
	}

	// SPACES-004: increment member_count only on approval.
	if status == "approved" {
		if err := h.Service.IncrementSpaceCount(r.Context(), spaceID, tenantID, 1); err != nil {
			slog.Error("spaces: failed to increment member_count on join approval",
				"space_id", spaceID.String(), "user_id", userID.String(), "error", err)
		}
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"id": composite, "status": status})
}

// AdminApproveJoinRequest handles POST /api/v1/admin/join-requests/{id}/approve.
func (h *Handlers) AdminApproveJoinRequest(w http.ResponseWriter, r *http.Request) {
	h.AdminResolveJoinRequest(w, r, "approved")
}

// AdminRejectJoinRequest handles POST /api/v1/admin/join-requests/{id}/reject.
func (h *Handlers) AdminRejectJoinRequest(w http.ResponseWriter, r *http.Request) {
	h.AdminResolveJoinRequest(w, r, "rejected")
}
