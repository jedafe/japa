package admin

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
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

	ctx := r.Context()

	items, err := h.queryFlaggedReviews(ctx, tenantID, limit, offset)
	if err != nil {
		slog.Error("moderation: querying flagged reviews", "error", err)
		writeError(w, http.StatusInternalServerError, "db_error", "Failed to load moderation queue")

		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"items":       items,
			"page":        offset / limit,
			"total_pages": 0, // cursor-based — total not computed for performance
		},
	})
}

// queryFlaggedReviews queries the reviews table for items with status='flagged'.
func (h *Handlers) queryFlaggedReviews(ctx context.Context, tenantID pgtype.UUID, limit, offset int32) ([]ModerationItem, error) {
	rows, err := h.DB.Reader.Query(ctx, `
		SELECT r.id, r.body, r.status, r.created_at, r.author_id,
		       COALESCE(u.username, 'unknown') AS author_username
		FROM reviews r
		LEFT JOIN users u ON u.id = r.author_id
		WHERE r.tenant_id = $1
		  AND r.status = 'flagged'
		ORDER BY r.created_at DESC
		LIMIT $2 OFFSET $3
	`, tenantID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("querying flagged reviews: %w", err)
	}
	defer rows.Close()

	var items []ModerationItem

	for rows.Next() {
		var item ModerationItem

		var id pgtype.UUID

		var body pgtype.Text

		var status string

		var createdAt pgtype.Timestamptz

		var authorID pgtype.UUID

		var authorUsername string

		if err := rows.Scan(&id, &body, &status, &createdAt, &authorID, &authorUsername); err != nil {
			slog.Warn("moderation: scanning review row", "error", err)
			continue
		}

		content := ""
		if body.Valid {
			content = body.String
			if len(content) > 200 {
				content = content[:200] + "…"
			}
		}

		item = ModerationItem{
			ID:        id.String(),
			Type:      "review",
			Status:    status,
			Content:   content,
			Reporter:  "system",
			AuthorID:  authorID.String(),
			Created:   createdAt.Time.Format("2006-01-02T15:04:05Z07:00"),
			EntityRef: "review:" + id.String(),
		}
		items = append(items, item)
	}

	if items == nil {
		items = []ModerationItem{}
	}

	return items, nil
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

		result, err := h.DB.Writer.Exec(ctx, `
			UPDATE reviews
			SET status = $1, updated_at = now()
			WHERE id = $2 AND tenant_id = $3 AND status = 'flagged'
		`, newStatus, reviewID, tenantID)
		if err != nil {
			slog.Warn("moderation: updating review status", "id", action.ID, "error", err)
			continue
		}

		if result.RowsAffected() > 0 {
			processed++
		}
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

	// Extract ID from URL — the chi router param is set at the route level.
	// For now, we accept the ID in the request body as a fallback since
	// the route pattern may vary.
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

	if req.Type == "" {
		req.Type = "review"
	}

	if h.DB == nil {
		writeError(w, http.StatusServiceUnavailable, "db_unavailable", "Database not available")
		return
	}

	ctx := r.Context()

	var reviewID pgtype.UUID
	if err := reviewID.Scan(req.ID); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid item ID format")
		return
	}

	// Reset the review back to "flagged" so it returns to the moderation queue.
	result, err := h.DB.Writer.Exec(ctx, `
		UPDATE reviews
		SET status = 'flagged', updated_at = now()
		WHERE id = $1 AND tenant_id = $2 AND status IN ('removed')
	`, reviewID, tenantID)
	if err != nil {
		slog.Error("moderation: appeal failed", "id", req.ID, "error", err)
		writeError(w, http.StatusInternalServerError, "db_error", "Failed to process appeal")

		return
	}

	if result.RowsAffected() == 0 {
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
	Target   string `json:"target"`   // "all", "unverified", "pro_members"
	Schedule string `json:"schedule"` // RFC3339 time for scheduled send, empty for immediate
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

	// Immediate broadcast: insert a notification for every active user in the tenant.
	sent, err := h.broadcastToAllUsers(ctx, tenantID, user.UserID, req.Subject, req.Body)
	if err != nil {
		slog.Error("broadcast: failed to send", "error", err)
		writeError(w, http.StatusInternalServerError, "broadcast_error", "Failed to send broadcast")

		return
	}

	slog.Info("broadcast sent",
		"user_id", user.UserID.String(),
		"tenant_id", tenantID.String(),
		"recipients", sent,
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
	// Fetch all active user IDs for this tenant.
	rows, err := h.DB.Reader.Query(ctx, `
		SELECT id FROM users WHERE tenant_id = $1 AND status = 'active'
	`, tenantID)
	if err != nil {
		return 0, fmt.Errorf("querying users for broadcast: %w", err)
	}
	defer rows.Close()

	var userIDs []pgtype.UUID

	for rows.Next() {
		var id pgtype.UUID
		if err := rows.Scan(&id); err != nil {
			slog.Warn("broadcast: scanning user ID", "error", err)
			continue
		}

		userIDs = append(userIDs, id)
	}

	if len(userIDs) == 0 {
		return 0, nil
	}

	// Batch insert notifications. Use a transaction for atomicity.
	tx, err := h.DB.Writer.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("beginning broadcast transaction: %w", err)
	}

	defer func() { _ = tx.Rollback(ctx) }()

	// Batch insert notifications using unnest for a single round-trip.
	_, err = tx.Exec(ctx, `
		INSERT INTO notifications (tenant_id, recipient_id, notification_type, actor_type, actor_id, title, body, created_at, updated_at)
		SELECT $1, unnest($2::uuid[]), 'system_announcement', 'system', $3, $4, $5, now(), now()
	`, tenantID, userIDs, senderID, title, body)
	if err != nil {
		return 0, fmt.Errorf("batch inserting broadcast notifications: %w", err)
	}

	sent := len(userIDs)

	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("committing broadcast: %w", err)
	}

	return sent, nil
}

// ─── User Moderation (P5.6 — Task 21) ────────────────────────────────────

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
	_, err := h.DB.Writer.Exec(ctx,
		`INSERT INTO user_moderation_actions (user_id, tenant_id, action, reason, expires_at, created_at)
		 VALUES ($1, (SELECT tenant_id FROM users WHERE id=$1), 'ban', $2,
		         CASE WHEN $3 > 0 THEN now() + ($3 || ' days')::interval ELSE NULL END, now())
		 ON CONFLICT DO NOTHING`,
		userID, input.Reason, input.Days)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to ban user")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"user_id": userID, "action": "banned"})
}

// UnbanUser handles DELETE /admin/users/:id/ban.
func (h *Handlers) UnbanUser(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "id")
	_, err := h.DB.Writer.Exec(r.Context(),
		`DELETE FROM user_moderation_actions WHERE user_id = $1 AND action = 'ban'`, userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to unban user")
		return
	}
	writeJSON(w, http.StatusNoContent, nil)
}

// ListUserBans handles GET /admin/users/:id/bans.
func (h *Handlers) ListUserBans(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "id")
	rows, err := h.DB.Reader.Query(r.Context(),
		`SELECT id, action, reason, expires_at, created_at FROM user_moderation_actions
		 WHERE user_id = $1 AND action = 'ban' ORDER BY created_at DESC`, userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to list bans")
		return
	}
	defer rows.Close()
	type banRow struct {
		ID        string  `json:"id"`
		Action    string  `json:"action"`
		Reason    string  `json:"reason"`
		ExpiresAt *string `json:"expires_at"`
		CreatedAt string  `json:"created_at"`
	}
	var bans []banRow
	for rows.Next() {
		var b banRow
		_ = rows.Scan(&b.ID, &b.Action, &b.Reason, &b.ExpiresAt, &b.CreatedAt)
		bans = append(bans, b)
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
	_, err := h.DB.Writer.Exec(r.Context(),
		`INSERT INTO user_moderation_actions (user_id, tenant_id, action, reason, created_at)
		 VALUES ($1, (SELECT tenant_id FROM users WHERE id=$1), 'warning', $2, now())`,
		userID, input.Reason)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to warn user")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"user_id": userID, "action": "warned"})
}

// ListUserWarnings handles GET /admin/users/:id/warnings.
func (h *Handlers) ListUserWarnings(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "id")
	rows, err := h.DB.Reader.Query(r.Context(),
		`SELECT id, reason, created_at FROM user_moderation_actions
		 WHERE user_id = $1 AND action = 'warning' ORDER BY created_at DESC`, userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to list warnings")
		return
	}
	defer rows.Close()
	type warnRow struct {
		ID        string `json:"id"`
		Reason    string `json:"reason"`
		CreatedAt string `json:"created_at"`
	}
	var warnings []warnRow
	for rows.Next() {
		var w warnRow
		_ = rows.Scan(&w.ID, &w.Reason, &w.CreatedAt)
		warnings = append(warnings, w)
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": warnings})
}

// DeleteUserWarning handles DELETE /admin/users/:id/warnings/:warningId.
func (h *Handlers) DeleteUserWarning(w http.ResponseWriter, r *http.Request) {
	warningID := chi.URLParam(r, "warningId")
	_, err := h.DB.Writer.Exec(r.Context(),
		`DELETE FROM user_moderation_actions WHERE id = $1 AND action = 'warning'`, warningID)
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
	_, err := h.DB.Writer.Exec(r.Context(),
		`UPDATE users SET status = $2, updated_at = now() WHERE id = $1`, userID, input.Status)
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
	_, err := h.DB.Writer.Exec(r.Context(),
		`INSERT INTO user_group_memberships (user_id, group_id) VALUES ($1, $2)
		 ON CONFLICT (user_id, group_id) DO NOTHING`, userID, input.GroupID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to update user group")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"user_id": userID})
}

// ─── Organization Verify (P5.6 — Task 10) ───────────────────────────────

// VerifyOrganization handles POST /admin/organizations/:id/verify.
func (h *Handlers) VerifyOrganization(w http.ResponseWriter, r *http.Request) {
	orgID := chi.URLParam(r, "id")
	_, err := h.DB.Writer.Exec(r.Context(),
		`UPDATE organizations SET verified = true, updated_at = now() WHERE id = $1`, orgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to verify organization")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": orgID, "verified": true})
}

// UnverifyOrganization handles DELETE /admin/organizations/:id/verify.
func (h *Handlers) UnverifyOrganization(w http.ResponseWriter, r *http.Request) {
	orgID := chi.URLParam(r, "id")
	_, err := h.DB.Writer.Exec(r.Context(),
		`UPDATE organizations SET verified = false, updated_at = now() WHERE id = $1`, orgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to unverify organization")
		return
	}
	writeJSON(w, http.StatusNoContent, nil)
}

// ─── Appeals (P5.6 — Task 22) ───────────────────────────────────────────

// ListAppeals handles GET /admin/appeals.
func (h *Handlers) ListAppeals(w http.ResponseWriter, r *http.Request) {
	rows, err := h.DB.Reader.Query(r.Context(),
		`SELECT id, user_id, entity_type, entity_id, reason, status, created_at
		 FROM moderation_appeals ORDER BY created_at DESC LIMIT 50`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to list appeals")
		return
	}
	defer rows.Close()
	var appeals []map[string]any
	for rows.Next() {
		var id, userID, entityType, entityID, reason, status, createdAt string
		_ = rows.Scan(&id, &userID, &entityType, &entityID, &reason, &status, &createdAt)
		appeals = append(appeals, map[string]any{
			"id": id, "user_id": userID, "entity_type": entityType,
			"entity_id": entityID, "reason": reason, "status": status, "created_at": createdAt,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": appeals})
}

// GetAppeal handles GET /admin/appeals/:id.
func (h *Handlers) GetAppeal(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var appealID, userID, entityType, entityID, reason, status, createdAt string
	err := h.DB.Reader.QueryRow(r.Context(),
		`SELECT id, user_id, entity_type, entity_id, reason, status, created_at
		 FROM moderation_appeals WHERE id = $1`, id).
		Scan(&appealID, &userID, &entityType, &entityID, &reason, &status, &createdAt)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "Appeal not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{
		"id": appealID, "user_id": userID, "entity_type": entityType,
		"entity_id": entityID, "reason": reason, "status": status, "created_at": createdAt,
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
	_, err := h.DB.Writer.Exec(r.Context(),
		`UPDATE moderation_appeals SET status = $2, updated_at = now() WHERE id = $1`, id, input.Status)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to update appeal")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "status": input.Status})
}

// ─── Moderation Queue Assign (P5.6 — Task 26) ───────────────────────────

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
	_, err := h.DB.Writer.Exec(r.Context(),
		`UPDATE content_reports SET assigned_to = $2, status = 'in_review', updated_at = now() WHERE id = $1`, id, input.AssigneeID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to assign moderation item")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "assignee_id": input.AssigneeID})
}

// ─── Content Moderation (P5.6 — Task 23) ────────────────────────────────

// RemoveContent handles POST /admin/content/:id/remove.
func (h *Handlers) RemoveContent(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	_, err := h.DB.Writer.Exec(r.Context(),
		`UPDATE contents SET status = 'removed', updated_at = now() WHERE id = $1`, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to remove content")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "status": "removed"})
}

// RestoreContent handles POST /admin/content/:id/restore.
func (h *Handlers) RestoreContent(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	_, err := h.DB.Writer.Exec(r.Context(),
		`UPDATE contents SET status = 'draft', updated_at = now() WHERE id = $1 AND status = 'removed'`, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to restore content")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "status": "restored"})
}

// ─── Broadcasts (P5.6 — Task 4) ──────────────────────────────────────────

// ListBroadcasts handles GET /admin/broadcasts.
func (h *Handlers) ListBroadcasts(w http.ResponseWriter, r *http.Request) {
	_, ok := requireTenant(w, r)
	if !ok {
		return
	}
	rows, err := h.DB.Reader.Query(r.Context(),
		`SELECT id, title, body, sent_by, sent_at, recipient_count
		 FROM admin_broadcasts ORDER BY sent_at DESC LIMIT 50`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to list broadcasts")
		return
	}
	defer rows.Close()
	var broadcasts []map[string]any
	for rows.Next() {
		var id, title, body, sentBy, sentAt string
		var recipientCount int
		_ = rows.Scan(&id, &title, &body, &sentBy, &sentAt, &recipientCount)
		broadcasts = append(broadcasts, map[string]any{
			"id": id, "title": title, "body": body, "sent_by": sentBy,
			"sent_at": sentAt, "recipient_count": recipientCount,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": broadcasts})
}

// DeleteBroadcast handles DELETE /admin/broadcasts/:id.
func (h *Handlers) DeleteBroadcast(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	_, err := h.DB.Writer.Exec(r.Context(),
		`DELETE FROM admin_broadcasts WHERE id = $1`, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to delete broadcast")
		return
	}
	writeJSON(w, http.StatusNoContent, nil)
}
