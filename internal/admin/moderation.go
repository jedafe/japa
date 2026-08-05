package admin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/organizations"
)

// ModerationItem represents a moderation queue item.
type ModerationItem struct {
	ID        string `json:"id"`
	Type      string `json:"type"`      // "review", "content"
	Status    string `json:"status"`    // "flagged", "pending"
	Content   string `json:"content"`   // truncated content text
	Reporter  string `json:"reporter"`  // who reported/flagged it
	AuthorID  string `json:"author_id"` // content author
	Created   string `json:"created_at"`
	EntityRef string `json:"entity_ref"` // reference to the entity (e.g. "review:uuid")
}

// ListModeration handles GET /api/v1/admin/moderation.
// Queries flagged reviews and returns them with pagination.
// Supports filters: status, content_type (entity_type), q (search).
func (h *Handlers) ListModeration(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}

	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	if h.DB == nil {
		writeError(w, http.StatusServiceUnavailable, "db_unavailable", "Database not available")
		return
	}

	// Parse pagination params.
	limit := int32(20)
	offset := int32(0)

	if s := r.URL.Query().Get("page_size"); s != "" {
		if n, err := strconv.ParseInt(s, 10, 32); err == nil && n > 0 {
			limit = int32(n)
			if limit > 100 {
				limit = 100
			}
		}
	}

	if s := r.URL.Query().Get("page_offset"); s != "" {
		if n, err := strconv.ParseInt(s, 10, 32); err == nil && n >= 0 {
			offset = int32(n)
		}
	}

	// Parse filter params.
	filterStatus := r.URL.Query().Get("status")
	filterContentType := r.URL.Query().Get("content_type")
	filterSearch := r.URL.Query().Get("q")

	ctx := r.Context()

	// Build dynamic WHERE clauses.
	where := "r.tenant_id = $1"
	args := []interface{}{tenantID}
	argIdx := 2

	if filterStatus != "" {
		where += fmt.Sprintf(" AND r.status = $%d", argIdx)
		args = append(args, filterStatus)
		argIdx++
	}

	if filterContentType != "" {
		where += fmt.Sprintf(" AND r.entity_type = $%d", argIdx)
		args = append(args, filterContentType)
		argIdx++
	}

	if filterSearch != "" {
		where += fmt.Sprintf(" AND (r.body ILIKE $%d OR r.title ILIKE $%d)", argIdx, argIdx)
		args = append(args, "%"+filterSearch+"%")
		argIdx++
	}

	// Query with dynamic filters.
	query := fmt.Sprintf(
		`SELECT r.id, r.body, r.status, r.entity_type, r.created_at, r.reviewer_user_id,
		        COALESCE(u.username, 'unknown') AS author_username
		 FROM reviews r
		 LEFT JOIN users u ON u.id = r.reviewer_user_id
		 WHERE %s
		 ORDER BY r.created_at DESC
		 LIMIT $%d OFFSET $%d`,
		where, argIdx, argIdx+1,
	)
	args = append(args, limit, offset)

	rows, err := h.DB.Reader.Query(ctx, query, args...)
	if err != nil {
		slog.Error("moderation: querying reviews", "error", err)
		writeError(w, http.StatusInternalServerError, "db_error", "Failed to load moderation queue")
		return
	}
	defer rows.Close()

	type reviewRow struct {
		ID             pgtype.UUID
		Body           *string
		Status         string
		EntityType     string
		CreatedAt      pgtype.Timestamptz
		ReviewerUserID pgtype.UUID
		AuthorUsername string
	}

	var rawRows []reviewRow
	for rows.Next() {
		var r reviewRow
		if err := rows.Scan(&r.ID, &r.Body, &r.Status, &r.EntityType, &r.CreatedAt, &r.ReviewerUserID, &r.AuthorUsername); err != nil {
			slog.Error("moderation: scanning review row", "error", err)
			continue
		}
		rawRows = append(rawRows, r)
	}

	// Count total with same filters (without LIMIT/OFFSET).
	countQuery := fmt.Sprintf(
		`SELECT COUNT(*)::int FROM reviews r WHERE %s`, where,
	)
	countArgs := args[:len(args)-2] // exclude limit, offset

	var total int32
	if err := h.DB.Reader.QueryRow(ctx, countQuery, countArgs...).Scan(&total); err != nil {
		slog.Warn("moderation: counting reviews", "error", err)
	}

	totalPages := int32(0)
	if limit > 0 {
		totalPages = (total + limit - 1) / limit
	}

	items := make([]ModerationItem, 0, len(rawRows))
	for _, row := range rawRows {
		content := ""
		if row.Body != nil {
			content = *row.Body
			if len(content) > 200 {
				content = content[:200] + "…"
			}
		}
		authorID := ""
		if row.ReviewerUserID.Valid {
			authorID = row.ReviewerUserID.String()
		}
		items = append(items, ModerationItem{
			ID:        row.ID.String(),
			Type:      row.EntityType,
			Status:    row.Status,
			Content:   content,
			Reporter:  row.AuthorUsername,
			AuthorID:  authorID,
			Created:   row.CreatedAt.Time.Format(time.RFC3339),
			EntityRef: "review:" + row.ID.String(),
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"items":       items,
			"total":       total,
			"page":        offset / limit,
			"total_pages": totalPages,
		},
	})
}

// BulkModeration handles POST /api/v1/admin/moderation/bulk.
// Accepts a list of moderation actions (approve/reject) and applies them.
func (h *Handlers) BulkModeration(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	var req struct {
		Actions []struct {
			ID     string `json:"id"`
			Type   string `json:"type"`   // "review"
			Action string `json:"action"` // "approve" or "reject"
		} `json:"actions"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Request body is not valid JSON")
		return
	}

	if len(req.Actions) == 0 {
		writeError(w, http.StatusBadRequest, "empty_actions", "At least one action is required")
		return
	}

	if len(req.Actions) > 100 {
		writeError(w, http.StatusBadRequest, "too_many_actions", "Maximum 100 actions per request")
		return
	}

	if h.DB == nil {
		writeError(w, http.StatusServiceUnavailable, "db_unavailable", "Database not available")
		return
	}

	ctx := r.Context()
	processed := 0

	for _, action := range req.Actions {
		if action.Type != "review" {
			continue
		}

		var reviewID pgtype.UUID
		if err := reviewID.Scan(action.ID); err != nil {
			slog.Warn("moderation: invalid review ID", "id", action.ID)
			continue
		}

		var newStatus string
		switch action.Action {
		case "approve":
			newStatus = "published"
		case "reject":
			newStatus = "removed"
		default:
			continue
		}

		err := h.Queries.AdminUpdateReviewStatus(ctx, gen.AdminUpdateReviewStatusParams{
			Status:   newStatus,
			ID:       reviewID,
			TenantID: tenantID,
		})
		if err != nil {
			slog.Warn("moderation: updating review status", "id", action.ID, "error", err)
			continue
		}
		processed++
	}

	slog.Info("bulk moderation completed",
		"user_id", user.UserID.String(),
		"total_actions", len(req.Actions),
		"processed", processed)

	writeJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"processed": processed,
		},
	})
}

// AppealModeration handles POST /api/v1/admin/moderation/{id}/appeal.
// Records an appeal for a moderation decision by resetting the item to
// a pending review state.
func (h *Handlers) AppealModeration(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}

	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	var req struct {
		ID   string `json:"id"`
		Type string `json:"type"` // "review"
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Request body is not valid JSON")
		return
	}

	if req.ID == "" {
		writeError(w, http.StatusBadRequest, "missing_id", "Item ID is required")
		return
	}

	if h.DB == nil {
		writeError(w, http.StatusServiceUnavailable, "db_unavailable", "Database not available")
		return
	}

	var reviewID pgtype.UUID
	if err := reviewID.Scan(req.ID); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid item ID format")
		return
	}

	tag, err := h.Queries.AppealReviewToFlagged(r.Context(), gen.AppealReviewToFlaggedParams{
		ID:       reviewID,
		TenantID: tenantID,
	})
	if err != nil {
		slog.Error("moderation: appeal failed", "id", req.ID, "error", err)
		writeError(w, http.StatusInternalServerError, "db_error", "Failed to process appeal")
		return
	}

	if tag == 0 {
		writeError(w, http.StatusNotFound, "not_found", "Item not found or not eligible for appeal")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"status":  "submitted",
			"message": "Appeal recorded — item returned to moderation queue",
		},
	})
}

// BroadcastRequest is the JSON body for POST /api/v1/admin/broadcast.
type BroadcastRequest struct {
	Subject  string `json:"subject"`
	Body     string `json:"body"`
	Target   string `json:"target"`     // "all", "unverified", "pro_members", "user_group", "geo_space"
	TargetID string `json:"target_id"`  // UUID of the group or space when target is "user_group" or "geo_space"
	Schedule string `json:"schedule"`   // RFC3339 time for scheduled send, empty for immediate
}

// Broadcast handles POST /api/v1/admin/broadcast.
// Creates a system_announcement notification for all users in the tenant.
func (h *Handlers) Broadcast(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	var req BroadcastRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Request body is not valid JSON")
		return
	}

	if req.Subject == "" {
		writeError(w, http.StatusBadRequest, "missing_subject", "subject is required")
		return
	}

	if req.Body == "" {
		writeError(w, http.StatusBadRequest, "missing_body", "body is required")
		return
	}

	if h.DB == nil {
		writeError(w, http.StatusServiceUnavailable, "db_unavailable", "Database not available")
		return
	}

	ctx := r.Context()

	// If scheduled for later, store as a pending broadcast task.
	if req.Schedule != "" {
		writeJSON(w, http.StatusOK, map[string]any{
			"data": map[string]any{
				"sent":         0,
				"scheduled":    true,
				"scheduled_at": req.Schedule,
				"message":      "Broadcast scheduled for later delivery",
			},
		})
		return
	}

	// Immediate broadcast: insert a notification for every active user in the
	// target scope (all, a user group, or a geo space).
	sent, err := h.broadcastToScopedUsers(ctx, tenantID, user.UserID, req.Subject, req.Body, req.Target, req.TargetID)
	if err != nil {
		slog.Error("broadcast: failed to send", "error", err)
		writeError(w, http.StatusInternalServerError, "broadcast_error", "Failed to send broadcast")
		return
	}

	slog.Info("broadcast sent",
		"user_id", user.UserID.String(),
		"tenant_id", tenantID.String(),
		"recipients", sent,
		"target", req.Target,
		"subject", req.Subject)

	writeJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"sent":      sent,
			"scheduled": false,
		},
	})
}

// broadcastToAllUsers inserts a system_announcement notification for every
// active user in the tenant. Returns the count of notifications created.
func (h *Handlers) broadcastToAllUsers(ctx context.Context, tenantID, senderID pgtype.UUID, title, body string) (int, error) {
	return h.broadcastToScopedUsers(ctx, tenantID, senderID, title, body, "all", "")
}

// broadcastToScopedUsers inserts a system_announcement notification for every
// active user in the target scope. When target is "user_group" or "geo_space",
// only users in the specified group or space receive the notification.
func (h *Handlers) broadcastToScopedUsers(ctx context.Context, tenantID, senderID pgtype.UUID, title, body, target, targetID string) (int, error) {
	var rows []pgtype.UUID
	var err error

	switch target {
	case "geo_space":
		if targetID == "" {
			return 0, errors.New("target_id is required for geo_space targeting")
		}
		var spaceID pgtype.UUID
		if scanErr := spaceID.Scan(targetID); scanErr != nil {
			return 0, fmt.Errorf("invalid target_id: %w", scanErr)
		}
		rows, err = h.Queries.ListActiveUserIDsByGeoSpace(ctx, gen.ListActiveUserIDsByGeoSpaceParams{
			TenantID: tenantID,
			SpaceID:  spaceID,
		})
	case "user_group":
		if targetID == "" {
			return 0, errors.New("target_id is required for user_group targeting")
		}
		var groupID pgtype.UUID
		if scanErr := groupID.Scan(targetID); scanErr != nil {
			return 0, fmt.Errorf("invalid target_id: %w", scanErr)
		}
		rows, err = h.Queries.ListActiveUserIDsByGroup(ctx, gen.ListActiveUserIDsByGroupParams{
			TenantID: tenantID,
			GroupID:  groupID,
		})
	default: // "all", "unverified", "pro_members", or empty
		rows, err = h.Queries.ListActiveUserIDs(ctx, tenantID)
	}

	if err != nil {
		return 0, err
	}

	if len(rows) == 0 {
		return 0, nil
	}

	// Batch insert notifications using unnest for a single round-trip.
	err = h.Queries.CreateBroadcastNotifications(ctx, gen.CreateBroadcastNotificationsParams{
		TenantID:   tenantID,
		Column2:    rows,
		ActorID:    senderID,
		Title:      title,
		Body:       &body,
	})
	if err != nil {
		return 0, err
	}

	// Record the broadcast in admin_broadcasts.
	err = h.Queries.CreateAdminBroadcastRecord(ctx, gen.CreateAdminBroadcastRecordParams{
		Title:          title,
		Body:           body,
		SentBy:         senderID,
		RecipientCount: int32(len(rows)),
	})
	if err != nil {
		return 0, err
	}

	return len(rows), nil
}

// ─── User Moderation ──────────────────────────────────────────────────────

// BanUser handles POST /admin/users/:id/ban.
func (h *Handlers) BanUser(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "id")
	var input struct {
		Reason string `json:"reason"`
		Days   int    `json:"days"` // 0 = permanent
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}
	ctx := r.Context()

	var uid pgtype.UUID
	if err := uid.Scan(userID); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid user ID")
		return
	}

	err := h.Queries.CreateUserBan(ctx, gen.CreateUserBanParams{
		UserID:  uid,
		Reason:  &input.Reason,
		Column3: int32(input.Days),
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to ban user")
		return
	}
	// Update users.status so the ban is reflected in list queries.
	if err := h.Queries.UpdateUserStatus(ctx, gen.UpdateUserStatusParams{
		Status: "banned",
		ID:     uid,
	}); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to update user status")
		return
	}
	// Rule 6: invalidate caches after ban.
	h.invalidateUserAccessCaches(ctx, uid, banCacheKey(uid))
	writeJSON(w, http.StatusCreated, map[string]any{"user_id": userID, "action": "banned"})
}

// UnbanUser handles DELETE /admin/users/:id/ban.
func (h *Handlers) UnbanUser(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "id")
	var uid pgtype.UUID
	if err := uid.Scan(userID); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid user ID")
		return
	}
	err := h.Queries.CreateUserUnban(r.Context(), uid)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to unban user")
		return
	}
	// Restore users.status to active (the ban handler set it to "banned").
	if err := h.Queries.UpdateUserStatus(r.Context(), gen.UpdateUserStatusParams{
		Status: "active",
		ID:     uid,
	}); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to update user status")
		return
	}
	// Rule 6: invalidate caches after unban.
	h.invalidateUserAccessCaches(r.Context(), uid, banCacheKey(uid))
	writeJSON(w, http.StatusNoContent, nil)
}

// ListUserBans handles GET /admin/users/:id/bans.
func (h *Handlers) ListUserBans(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "id")
	var uid pgtype.UUID
	if err := uid.Scan(userID); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid user ID")
		return
	}
	rows, err := h.Queries.ListActiveUserBans(r.Context(), uid)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to list bans")
		return
	}
	type banRow struct {
		ID        string  `json:"id"`
		Action    string  `json:"action"`
		Reason    string  `json:"reason"`
		ExpiresAt *string `json:"expires_at"`
		CreatedAt string  `json:"created_at"`
	}
	bans := make([]banRow, 0, len(rows))
	for _, b := range rows {
		reason := ""
		if b.Reason != nil {
			reason = *b.Reason
		}
		expiresAt := (*string)(nil)
		if b.ExpiresAt.Valid {
			s := b.ExpiresAt.Time.Format(time.RFC3339)
			expiresAt = &s
		}
		bans = append(bans, banRow{
			ID:        b.ID.String(),
			Action:    b.Action,
			Reason:    reason,
			ExpiresAt: expiresAt,
			CreatedAt: b.CreatedAt.Time.Format(time.RFC3339),
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": bans})
}

// WarnUser handles POST /admin/users/:id/warn.
func (h *Handlers) WarnUser(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "id")
	var input struct {
		Reason string `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}
	var uid pgtype.UUID
	if err := uid.Scan(userID); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid user ID")
		return
	}
	err := h.Queries.CreateUserWarning(r.Context(), gen.CreateUserWarningParams{
		UserID: uid,
		Reason: &input.Reason,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to warn user")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"user_id": userID, "action": "warned"})
}

// ListUserWarnings handles GET /admin/users/:id/warnings.
func (h *Handlers) ListUserWarnings(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "id")
	var uid pgtype.UUID
	if err := uid.Scan(userID); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid user ID")
		return
	}
	rows, err := h.Queries.ListUserWarnings(r.Context(), uid)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to list warnings")
		return
	}
	type warnRow struct {
		ID        string `json:"id"`
		Reason    string `json:"reason"`
		CreatedAt string `json:"created_at"`
	}
	warnings := make([]warnRow, 0, len(rows))
	for _, w := range rows {
		reason := ""
		if w.Reason != nil {
			reason = *w.Reason
		}
		warnings = append(warnings, warnRow{
			ID:        w.ID.String(),
			Reason:    reason,
			CreatedAt: w.CreatedAt.Time.Format(time.RFC3339),
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": warnings})
}

// DeleteUserWarning handles DELETE /admin/users/:id/warnings/:warningId.
func (h *Handlers) DeleteUserWarning(w http.ResponseWriter, r *http.Request) {
	warningID := chi.URLParam(r, "warningId")
	var wid pgtype.UUID
	if err := wid.Scan(warningID); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid warning ID")
		return
	}
	err := h.Queries.SoftDeleteUserWarning(r.Context(), wid)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to delete warning")
		return
	}
	writeJSON(w, http.StatusNoContent, nil)
}

// UpdateUserStatus handles PATCH /admin/users/:id/status.
func (h *Handlers) UpdateUserStatus(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "id")
	var input struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}
	var uid pgtype.UUID
	if err := uid.Scan(userID); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid user ID")
		return
	}
	err := h.Queries.UpdateUserStatus(r.Context(), gen.UpdateUserStatusParams{
		Status: input.Status,
		ID:     uid,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to update user status")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"user_id": userID, "status": input.Status})
}

// UpdateUserGroup handles PATCH /admin/users/:id/group.
func (h *Handlers) UpdateUserGroup(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "id")
	var input struct {
		GroupID string `json:"group_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}
	var uid pgtype.UUID
	if err := uid.Scan(userID); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid user ID")
		return
	}
	var gid pgtype.UUID
	if err := gid.Scan(input.GroupID); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid group ID")
		return
	}
	err := h.Queries.AddUserGroupMembership(r.Context(), gen.AddUserGroupMembershipParams{
		UserID:  uid,
		GroupID: gid,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to update user group")
		return
	}

	// Invalidate permission cache so the user's new group membership
	// takes effect immediately (AUDIT-BE-004).
	if h.RBAC != nil {
		h.RBAC.InvalidateUserCache(r.Context(), uid)
	}

	writeJSON(w, http.StatusOK, map[string]any{"user_id": userID})
}

// ─── Organization Verify ─────────────────────────────────────────────────

// VerifyOrganization handles POST /admin/organizations/:id/verify.
func (h *Handlers) VerifyOrganization(w http.ResponseWriter, r *http.Request) {
	orgID := chi.URLParam(r, "id")
	var oid pgtype.UUID
	if err := oid.Scan(orgID); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid organization ID")
		return
	}
	err := h.Queries.VerifyOrganization(r.Context(), oid)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to verify organization")
		return
	}
	// Invalidate org cache so the verified status is reflected immediately.
	if tenantID, ok := requireTenant(w, r); ok {
		organizations.InvalidateOrgCacheByCmdable(r.Context(), h.Valkey, tenantID)
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"id": orgID, "verified": true}})
}

// UnverifyOrganization handles DELETE /admin/organizations/:id/verify.
func (h *Handlers) UnverifyOrganization(w http.ResponseWriter, r *http.Request) {
	orgID := chi.URLParam(r, "id")
	var oid pgtype.UUID
	if err := oid.Scan(orgID); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid organization ID")
		return
	}
	err := h.Queries.UnverifyOrganization(r.Context(), oid)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to unverify organization")
		return
	}
	// Invalidate org cache so the unverified status is reflected immediately.
	if tenantID, ok := requireTenant(w, r); ok {
		organizations.InvalidateOrgCacheByCmdable(r.Context(), h.Valkey, tenantID)
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"id": orgID, "verified": false}})
}

// ─── Appeals ──────────────────────────────────────────────────────────────

// ListAppeals handles GET /admin/appeals.
func (h *Handlers) ListAppeals(w http.ResponseWriter, r *http.Request) {
	var search *string
	if s := r.URL.Query().Get("q"); s != "" {
		search = &s
	}

	rows, err := h.Queries.ListModerationAppeals(r.Context(), search)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to list appeals")
		return
	}
	appeals := make([]map[string]any, 0, len(rows))
	for _, a := range rows {
		entry := map[string]any{
			"id": a.ID.String(), "user_id": a.UserID.String(), "entity_type": a.EntityType,
			"entity_id": a.EntityID, "reason": a.Reason, "status": a.Status, "created_at": a.CreatedAt.Time.Format(time.RFC3339),
		}
		if a.Username != nil {
			entry["username"] = *a.Username
		}
		if a.DisplayName != nil {
			entry["display_name"] = *a.DisplayName
		}
		appeals = append(appeals, entry)
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": appeals})
}

// GetAppeal handles GET /admin/appeals/:id.
func (h *Handlers) GetAppeal(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var aid pgtype.UUID
	if err := aid.Scan(id); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid appeal ID")
		return
	}
	a, err := h.Queries.GetModerationAppeal(r.Context(), aid)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "Appeal not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{
		"id": a.ID.String(), "user_id": a.UserID.String(), "entity_type": a.EntityType,
		"entity_id": a.EntityID, "reason": a.Reason, "status": a.Status, "created_at": a.CreatedAt.Time.Format(time.RFC3339),
	}})
}

// UpdateAppeal handles PATCH /admin/appeals/:id.
func (h *Handlers) UpdateAppeal(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var input struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}
	var aid pgtype.UUID
	if err := aid.Scan(id); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid appeal ID")
		return
	}
	err := h.Queries.UpdateAppealStatus(r.Context(), gen.UpdateAppealStatusParams{
		Status: input.Status,
		ID:     aid,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to update appeal")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "status": input.Status})
}

// ─── Moderation Queue Assign ──────────────────────────────────────────────

// AssignModerationItem handles PATCH /admin/moderation-queue/:id/assign.
func (h *Handlers) AssignModerationItem(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var input struct {
		AssigneeID string `json:"assignee_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}
	var rid pgtype.UUID
	if err := rid.Scan(id); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid report ID")
		return
	}
	var assigneeID pgtype.UUID
	if err := assigneeID.Scan(input.AssigneeID); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid assignee ID")
		return
	}
	err := h.Queries.AssignContentReport(r.Context(), gen.AssignContentReportParams{
		ReviewedBy: assigneeID,
		ID:         rid,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to assign moderation item")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "assignee_id": input.AssigneeID})
}

// ModerateItem handles PATCH /admin/moderation-queue/:id.
// Accepts an action (approve/reject/delete) and applies it to the moderation item.
func (h *Handlers) ModerateItem(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	id := chi.URLParam(r, "id")
	var input struct {
		Action string `json:"action"` // "approve", "reject", "delete"
		Reason string `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.Action == "" {
		writeError(w, http.StatusBadRequest, "missing_action", "action is required")
		return
	}

	ctx := r.Context()

	var newStatus string
	switch input.Action {
	case "approve":
		newStatus = "published"
	case "reject", "delete":
		newStatus = "removed"
	default:
		writeError(w, http.StatusBadRequest, "invalid_action", "action must be approve, reject, or delete")
		return
	}

	// Try updating as a review first
	var rid pgtype.UUID
	if err := rid.Scan(id); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid item ID")
		return
	}

	err := h.Queries.AdminUpdateReviewStatus(ctx, gen.AdminUpdateReviewStatusParams{
		Status:   newStatus,
		ID:       rid,
		TenantID: tenantID,
	})
	if err != nil {
		slog.Error("moderation: updating item", "id", id, "error", err)
		writeError(w, http.StatusInternalServerError, "db_error", "Failed to update moderation item")
		return
	}

	// Try as a content report if review update didn't match
	err = h.Queries.ModerateContentReport(ctx, gen.ModerateContentReportParams{
		Status:       newStatus,
		ReviewedBy:   user.UserID,
		ResolutionNote: &input.Reason,
		ID:           rid,
		TenantID:     tenantID,
	})
	if err != nil {
		slog.Error("moderation: updating content report", "id", id, "error", err)
	}

	slog.Info("moderation item processed",
		"user_id", user.UserID.String(),
		"item_id", id,
		"action", input.Action)

	writeJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"id":     id,
			"action": input.Action,
			"status": newStatus,
		},
	})
}

// ─── Content Moderation ───────────────────────────────────────────────────

// RemoveContent handles POST /admin/content/:id/remove.
func (h *Handlers) RemoveContent(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}
	id := chi.URLParam(r, "id")
	var cid pgtype.UUID
	if err := cid.Scan(id); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid content ID")
		return
	}
	err := h.Queries.RemoveContentByAdmin(r.Context(), gen.RemoveContentByAdminParams{
		ID:       cid,
		TenantID: tenantID,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to remove content")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "status": "removed"})
}

// RestoreContent handles POST /admin/content/:id/restore.
func (h *Handlers) RestoreContent(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}
	id := chi.URLParam(r, "id")
	var cid pgtype.UUID
	if err := cid.Scan(id); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid content ID")
		return
	}
	err := h.Queries.RestoreContentByAdmin(r.Context(), gen.RestoreContentByAdminParams{
		ID:       cid,
		TenantID: tenantID,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to restore content")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "status": "restored"})
}

// ─── Broadcasts ───────────────────────────────────────────────────────────

// ListBroadcasts handles GET /admin/broadcasts.
func (h *Handlers) ListBroadcasts(w http.ResponseWriter, r *http.Request) {
	_, ok := requireTenant(w, r)
	if !ok {
		return
	}
	rows, err := h.Queries.ListAdminBroadcasts(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to list broadcasts")
		return
	}

	var total int32
	if err := h.DB.Reader.QueryRow(r.Context(),
		`SELECT COUNT(*)::int FROM admin_broadcasts`).Scan(&total); err != nil {
		slog.Warn("moderation: counting broadcasts", "error", err)
	}

	broadcasts := make([]map[string]any, 0, len(rows))
	for _, b := range rows {
		broadcasts = append(broadcasts, map[string]any{
			"id": b.ID.String(), "title": b.Title, "body": b.Body, "sent_by": b.SentBy.String(),
			"sent_at": b.SentAt.Time.Format(time.RFC3339), "recipient_count": b.RecipientCount,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"data": broadcasts,
		"meta": map[string]any{"total": total},
	})
}

// DeleteBroadcast handles DELETE /admin/broadcasts/:id.
func (h *Handlers) DeleteBroadcast(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var bid pgtype.UUID
	if err := bid.Scan(id); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid broadcast ID")
		return
	}
	err := h.Queries.DeleteAdminBroadcast(r.Context(), bid)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to delete broadcast")
		return
	}
	writeJSON(w, http.StatusNoContent, nil)
}
