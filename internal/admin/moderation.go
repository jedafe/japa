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
	errpkg "github.com/jedafe/japa/internal/errors"
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
// When content_type=ad_campaign, queries ad_campaigns with pending_review
// status instead of the reviews table (§61 Review Gate, ADR-079 §3).
func (h *Handlers) ListModeration(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}

	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	if h.DB == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "db_unavailable", "Database not available")
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

	// §61: When content_type=ad_campaign, query ad_campaigns table for
	// pending-review self-serve campaigns instead of the reviews table.
	// The content_reviews table has a hard FK to contents(id) so ad
	// campaigns cannot live there; the pending_review status IS the
	// review-queue entry.
	if filterContentType == "ad_campaign" {
		h.listAdCampaignModeration(w, r, ctx, tenantID, limit, offset, filterStatus, filterSearch)
		return
	}

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
		errpkg.WriteError(w, http.StatusInternalServerError, "db_error", "Failed to load moderation queue")
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

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"items":       items,
			"total":       total,
			"page":        offset / limit,
			"total_pages": totalPages,
		},
	})
}

// listAdCampaignModeration queries ad_campaigns for the moderation queue
// when content_type=ad_campaign. Returns pending-review campaigns with
// pagination. Ref: §61 Review Gate, ADR-079 §3.
func (h *Handlers) listAdCampaignModeration(
	w http.ResponseWriter, r *http.Request,
	ctx context.Context, tenantID pgtype.UUID,
	limit, offset int32, filterStatus, filterSearch string,
) {
	where := "ac.tenant_id = $1 AND ac.organization_id IS NOT NULL"
	args := []interface{}{tenantID}
	argIdx := 2

	// Default to pending_review when no status filter is provided;
	// for ad campaigns, pending_review is the "needs review" status.
	statusFilter := filterStatus
	if statusFilter == "" {
		statusFilter = "pending_review"
	}
	where += fmt.Sprintf(" AND ac.status = $%d", argIdx)
	args = append(args, statusFilter)
	argIdx++

	if filterSearch != "" {
		where += fmt.Sprintf(" AND ac.name ILIKE $%d", argIdx)
		args = append(args, "%"+filterSearch+"%")
		argIdx++
	}

	query := fmt.Sprintf(
		`SELECT ac.id, ac.name, ac.status, ac.content, ac.created_at,
		        ac.created_by, COALESCE(u.username, 'unknown') AS creator_username,
		        ac.organization_id
		 FROM ad_campaigns ac
		 LEFT JOIN users u ON u.id = ac.created_by
		 WHERE %s
		 ORDER BY ac.created_at DESC
		 LIMIT $%d OFFSET $%d`,
		where, argIdx, argIdx+1,
	)
	args = append(args, limit, offset)

	rows, err := h.DB.Reader.Query(ctx, query, args...)
	if err != nil {
		slog.Error("moderation: querying ad_campaigns", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "db_error", "Failed to load ad campaign moderation queue")
		return
	}
	defer rows.Close()

	type adCampaignRow struct {
		ID              pgtype.UUID
		Name            string
		Status          string
		Content         []byte
		CreatedAt       pgtype.Timestamptz
		CreatedBy       pgtype.UUID
		CreatorUsername string
		OrganizationID  pgtype.UUID
	}

	var rawRows []adCampaignRow
	for rows.Next() {
		var r adCampaignRow
		if err := rows.Scan(&r.ID, &r.Name, &r.Status, &r.Content, &r.CreatedAt,
			&r.CreatedBy, &r.CreatorUsername, &r.OrganizationID); err != nil {
			slog.Error("moderation: scanning ad_campaign row", "error", err)
			continue
		}
		rawRows = append(rawRows, r)
	}

	// Count total with same filters (without LIMIT/OFFSET).
	countQuery := fmt.Sprintf(
		`SELECT COUNT(*)::int FROM ad_campaigns ac WHERE %s`, where,
	)
	countArgs := args[:len(args)-2]

	var total int32
	if err := h.DB.Reader.QueryRow(ctx, countQuery, countArgs...).Scan(&total); err != nil {
		slog.Warn("moderation: counting ad_campaigns", "error", err)
	}

	totalPages := int32(0)
	if limit > 0 {
		totalPages = (total + limit - 1) / limit
	}

	items := make([]ModerationItem, 0, len(rawRows))
	for _, row := range rawRows {
		content := row.Name
		if len(row.Content) > 0 {
			// Content is JSONB; append a truncated text preview.
			content += " — " + string(row.Content)
		}
		if len(content) > 200 {
			content = content[:200] + "…"
		}
		authorID := ""
		if row.CreatedBy.Valid {
			authorID = row.CreatedBy.String()
		}
		items = append(items, ModerationItem{
			ID:        row.ID.String(),
			Type:      "ad_campaign",
			Status:    row.Status,
			Content:   content,
			Reporter:  row.CreatorUsername,
			AuthorID:  authorID,
			Created:   row.CreatedAt.Time.Format(time.RFC3339),
			EntityRef: "ad_campaign:" + row.ID.String(),
		})
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
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
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Request body is not valid JSON")
		return
	}

	if len(req.Actions) == 0 {
		errpkg.WriteError(w, http.StatusBadRequest, "empty_actions", "At least one action is required")
		return
	}

	if len(req.Actions) > 100 {
		errpkg.WriteError(w, http.StatusBadRequest, "too_many_actions", "Maximum 100 actions per request")
		return
	}

	if h.DB == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "db_unavailable", "Database not available")
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

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
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
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Request body is not valid JSON")
		return
	}

	if req.ID == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_id", "Item ID is required")
		return
	}

	if h.DB == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "db_unavailable", "Database not available")
		return
	}

	var reviewID pgtype.UUID
	if err := reviewID.Scan(req.ID); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid item ID format")
		return
	}

	tag, err := h.Queries.AppealReviewToFlagged(r.Context(), gen.AppealReviewToFlaggedParams{
		ID:       reviewID,
		TenantID: tenantID,
	})
	if err != nil {
		slog.Error("moderation: appeal failed", "id", req.ID, "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "db_error", "Failed to process appeal")
		return
	}

	if tag == 0 {
		errpkg.WriteError(w, http.StatusNotFound, "not_found", "Item not found or not eligible for appeal")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
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
	Target   string `json:"target"`    // "all", "unverified", "pro_members", "user_group", "geo_space"
	TargetID string `json:"target_id"` // UUID of the group or space when target is "user_group" or "geo_space"
	Schedule string `json:"schedule"`  // RFC3339 time for scheduled send, empty for immediate
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
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Request body is not valid JSON")
		return
	}

	if req.Subject == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_subject", "subject is required")
		return
	}

	if req.Body == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_body", "body is required")
		return
	}

	if h.DB == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "db_unavailable", "Database not available")
		return
	}

	ctx := r.Context()

	// If scheduled for later, validate the schedule time and return an
	// error instead of silently dropping the broadcast content (BUG-008).
	// The admin_broadcasts table has no status/scheduled_at column, so
	// scheduled delivery is not yet supported via this endpoint. The
	// separate POST /admin/broadcasts/{id}/send endpoint handles sending
	// an already-created broadcast. Ref: §35 L128.
	if req.Schedule != "" {
		scheduledAt, parseErr := time.Parse(time.RFC3339, req.Schedule)
		if parseErr != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_schedule", "schedule must be a valid RFC3339 timestamp")
			return
		}
		if scheduledAt.Before(time.Now()) {
			errpkg.WriteError(w, http.StatusBadRequest, "past_schedule", "schedule time must be in the future")
			return
		}
		errpkg.WriteError(w, http.StatusNotImplemented, "scheduled_broadcast_unsupported",
			"Scheduled broadcasts are not yet supported. Use POST /admin/broadcasts/{id}/send to send an existing broadcast.")
		return
	}

	// Immediate broadcast: insert a notification for every active user in the
	// target scope (all, a user group, or a geo space).
	sent, err := h.broadcastToScopedUsers(ctx, tenantID, user.UserID, req.Subject, req.Body, req.Target, req.TargetID)
	if err != nil {
		slog.Error("broadcast: failed to send", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "broadcast_error", "Failed to send broadcast")
		return
	}

	slog.Info("broadcast sent",
		"user_id", user.UserID.String(),
		"tenant_id", tenantID.String(),
		"recipients", sent,
		"target", req.Target,
		"subject", req.Subject)

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
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
		TenantID: tenantID,
		Column2:  rows,
		ActorID:  senderID,
		Title:    title,
		Body:     &body,
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
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}
	ctx := r.Context()

	var uid pgtype.UUID
	if err := uid.Scan(userID); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid user ID")
		return
	}

	err := h.Queries.CreateUserBan(ctx, gen.CreateUserBanParams{
		UserID:  uid,
		Reason:  &input.Reason,
		Column3: int32(input.Days),
	})
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to ban user")
		return
	}
	// Update users.status so the ban is reflected in list queries.
	if err := h.Queries.UpdateUserStatus(ctx, gen.UpdateUserStatusParams{
		Status: "banned",
		ID:     uid,
	}); err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to update user status")
		return
	}
	// Rule 6: invalidate caches after ban.
	h.invalidateUserAccessCaches(ctx, uid, banCacheKey(uid))
	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"user_id": userID, "action": "banned"})
}

// UnbanUser handles DELETE /admin/users/:id/ban.
func (h *Handlers) UnbanUser(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "id")
	var uid pgtype.UUID
	if err := uid.Scan(userID); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid user ID")
		return
	}
	err := h.Queries.CreateUserUnban(r.Context(), uid)
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to unban user")
		return
	}
	// Restore users.status to active (the ban handler set it to "banned").
	if err := h.Queries.UpdateUserStatus(r.Context(), gen.UpdateUserStatusParams{
		Status: "active",
		ID:     uid,
	}); err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to update user status")
		return
	}
	// Rule 6: invalidate caches after unban.
	h.invalidateUserAccessCaches(r.Context(), uid, banCacheKey(uid))
	errpkg.WriteJSON(w, http.StatusNoContent, nil)
}

// ListUserBans handles GET /admin/users/:id/bans.
func (h *Handlers) ListUserBans(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "id")
	var uid pgtype.UUID
	if err := uid.Scan(userID); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid user ID")
		return
	}
	rows, err := h.Queries.ListActiveUserBans(r.Context(), uid)
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to list bans")
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
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": bans})
}

// WarnUser handles POST /admin/users/:id/warn.
func (h *Handlers) WarnUser(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "id")
	var input struct {
		Reason string `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}
	var uid pgtype.UUID
	if err := uid.Scan(userID); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid user ID")
		return
	}
	err := h.Queries.CreateUserWarning(r.Context(), gen.CreateUserWarningParams{
		UserID: uid,
		Reason: &input.Reason,
	})
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to warn user")
		return
	}
	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"user_id": userID, "action": "warned"})
}

// ListUserWarnings handles GET /admin/users/:id/warnings.
func (h *Handlers) ListUserWarnings(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "id")
	var uid pgtype.UUID
	if err := uid.Scan(userID); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid user ID")
		return
	}
	rows, err := h.Queries.ListUserWarnings(r.Context(), uid)
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to list warnings")
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
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": warnings})
}

// DeleteUserWarning handles DELETE /admin/users/:id/warnings/:warningId.
func (h *Handlers) DeleteUserWarning(w http.ResponseWriter, r *http.Request) {
	warningID := chi.URLParam(r, "warningId")
	var wid pgtype.UUID
	if err := wid.Scan(warningID); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid warning ID")
		return
	}
	err := h.Queries.SoftDeleteUserWarning(r.Context(), wid)
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to delete warning")
		return
	}
	errpkg.WriteJSON(w, http.StatusNoContent, nil)
}

// UpdateUserStatus handles PATCH /admin/users/:id/status.
func (h *Handlers) UpdateUserStatus(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "id")
	var input struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}
	var uid pgtype.UUID
	if err := uid.Scan(userID); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid user ID")
		return
	}
	err := h.Queries.UpdateUserStatus(r.Context(), gen.UpdateUserStatusParams{
		Status: input.Status,
		ID:     uid,
	})
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to update user status")
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"user_id": userID, "status": input.Status})
}

// UpdateUserGroup handles PATCH /admin/users/:id/group.
func (h *Handlers) UpdateUserGroup(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "id")
	var input struct {
		GroupID string `json:"group_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}
	var uid pgtype.UUID
	if err := uid.Scan(userID); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid user ID")
		return
	}
	var gid pgtype.UUID
	if err := gid.Scan(input.GroupID); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid group ID")
		return
	}
	err := h.Queries.AddUserGroupMembership(r.Context(), gen.AddUserGroupMembershipParams{
		UserID:  uid,
		GroupID: gid,
	})
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to update user group")
		return
	}

	// Invalidate permission cache so the user's new group membership
	// takes effect immediately (AUDIT-BE-004).
	if h.RBAC != nil {
		h.RBAC.InvalidateUserCache(r.Context(), uid)
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"user_id": userID})
}

// ─── Organization Verify ─────────────────────────────────────────────────

// VerifyOrganization handles POST /admin/organizations/:id/verify.
func (h *Handlers) VerifyOrganization(w http.ResponseWriter, r *http.Request) {
	orgID := chi.URLParam(r, "id")
	var oid pgtype.UUID
	if err := oid.Scan(orgID); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid organization ID")
		return
	}
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}
	user, ok := requireUser(w, r)
	if !ok {
		return
	}
	// Route through the organizations service so the organization.verified
	// webhook event is emitted (Section 12) and the org cache is invalidated.
	if h.Orgs != nil {
		if err := h.Orgs.VerifyOrganization(r.Context(), oid, tenantID, user.UserID); err != nil {
			errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to verify organization")
			return
		}
	} else {
		err := h.Queries.VerifyOrganization(r.Context(), gen.VerifyOrganizationParams{ID: oid, TenantID: tenantID})
		if err != nil {
			errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to verify organization")
			return
		}
		// Invalidate org cache so the verified status is reflected immediately.
		organizations.InvalidateOrgCacheByCmdable(r.Context(), h.Valkey, tenantID)
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"id": orgID, "verified": true}})
}

// UnverifyOrganization handles DELETE /admin/organizations/:id/verify.
// Ref: japaconnect-engineering/sections/07-organization-profiles-claim-ownership.md
func (h *Handlers) UnverifyOrganization(w http.ResponseWriter, r *http.Request) {
	orgID := chi.URLParam(r, "id")
	var oid pgtype.UUID
	if err := oid.Scan(orgID); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid organization ID")
		return
	}
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}
	user, ok := requireUser(w, r)
	if !ok {
		return
	}
	// Route through the organizations service so the organization.unverified
	// webhook event is emitted (Section 12) and the org cache is invalidated.
	if h.Orgs != nil {
		if err := h.Orgs.UnverifyOrganization(r.Context(), oid, tenantID, user.UserID); err != nil {
			errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to unverify organization")
			return
		}
	} else {
		err := h.Queries.UnverifyOrganization(r.Context(), gen.UnverifyOrganizationParams{ID: oid, TenantID: tenantID})
		if err != nil {
			errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to unverify organization")
			return
		}
		organizations.InvalidateOrgCacheByCmdable(r.Context(), h.Valkey, tenantID)
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"id": orgID, "verified": false}})
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
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to list appeals")
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
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": appeals})
}

// GetAppeal handles GET /admin/appeals/:id.
func (h *Handlers) GetAppeal(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var aid pgtype.UUID
	if err := aid.Scan(id); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid appeal ID")
		return
	}
	a, err := h.Queries.GetModerationAppeal(r.Context(), aid)
	if err != nil {
		errpkg.WriteError(w, http.StatusNotFound, "not_found", "Appeal not found")
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{
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
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}
	var aid pgtype.UUID
	if err := aid.Scan(id); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid appeal ID")
		return
	}
	err := h.Queries.UpdateAppealStatus(r.Context(), gen.UpdateAppealStatusParams{
		Status: input.Status,
		ID:     aid,
	})
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to update appeal")
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"id": id, "status": input.Status})
}

// ─── Moderation Queue Assign ──────────────────────────────────────────────

// AssignModerationItem handles PATCH /admin/moderation-queue/:id/assign.
func (h *Handlers) AssignModerationItem(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var input struct {
		AssigneeID string `json:"assignee_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}
	var rid pgtype.UUID
	if err := rid.Scan(id); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid report ID")
		return
	}
	var assigneeID pgtype.UUID
	if err := assigneeID.Scan(input.AssigneeID); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid assignee ID")
		return
	}
	err := h.Queries.AssignContentReport(r.Context(), gen.AssignContentReportParams{
		ReviewedBy: assigneeID,
		ID:         rid,
	})
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to assign moderation item")
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"id": id, "assignee_id": input.AssigneeID})
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
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.Action == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_action", "action is required")
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
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_action", "action must be approve, reject, or delete")
		return
	}

	// Try updating as a review first
	var rid pgtype.UUID
	if err := rid.Scan(id); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid item ID")
		return
	}

	err := h.Queries.AdminUpdateReviewStatus(ctx, gen.AdminUpdateReviewStatusParams{
		Status:   newStatus,
		ID:       rid,
		TenantID: tenantID,
	})
	if err != nil {
		slog.Error("moderation: updating item", "id", id, "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "db_error", "Failed to update moderation item")
		return
	}

	// Try as a content report if review update didn't match
	err = h.Queries.ModerateContentReport(ctx, gen.ModerateContentReportParams{
		Status:         newStatus,
		ReviewedBy:     user.UserID,
		ResolutionNote: &input.Reason,
		ID:             rid,
		TenantID:       tenantID,
	})
	if err != nil {
		slog.Error("moderation: updating content report", "id", id, "error", err)
	}

	// Participation Orchestration (§76, ADR-077 §4, BUG-001): if the ID
	// matches a content_reviews row with source='persona_action', update
	// its status and — on approve — enqueue persona:dispatch_action so the
	// approved action is actually published. Previously ModerateItem only
	// touched the `reviews` table (§29 Ratings & Reviews) and the
	// content_reviews persona-action row stayed 'pending' forever.
	if h.handlePersonaActionReview(ctx, rid, tenantID, user.UserID, input.Action, input.Reason) {
		slog.Info("moderation: persona action review processed",
			"item_id", id, "action", input.Action, "reviewer", user.UserID.String())
	}

	// AI Content Studio (§75, ADR-076 §3): if the ID matches a
	// content_reviews row with source='studio', transition the linked
	// studio_jobs row to 'published' (approve) or 'rejected' (reject) and
	// — on approve — publish the draft content and transition the linked
	// intel_records row to 'published'. Without this, studio jobs stay
	// 'in_review' forever after admin review (same defect class as the
	// persona_action BUG-001 fix above).
	if h.handleStudioReview(ctx, rid, tenantID, user.UserID, input.Action, input.Reason) {
		slog.Info("moderation: studio review processed",
			"item_id", id, "action", input.Action, "reviewer", user.UserID.String())
	}

	slog.Info("moderation item processed",
		"user_id", user.UserID.String(),
		"item_id", id,
		"action", input.Action)

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"id":     id,
			"action": input.Action,
			"status": newStatus,
		},
	})
}

// handlePersonaActionReview handles a content_reviews row with
// source='persona_action' (§76, ADR-077 §4, BUG-001). It returns true if the
// ID matched a persona-action review (so the caller can log it). On approve,
// it sets the review status to 'approved', records the reviewer, and enqueues
// the persona:dispatch_action task so the action is published. On reject, it
// sets the status to 'rejected'. The content_reviews status CHECK constraint
// (000018) allows 'approved' and 'rejected'.
func (h *Handlers) handlePersonaActionReview(ctx context.Context, id, tenantID, reviewerID pgtype.UUID, action, reason string) bool {
	// Check whether this ID is a persona-action content_reviews row.
	var source, status string
	err := h.DB.Reader.QueryRow(ctx,
		`SELECT source, status FROM content_reviews
		 WHERE id = $1 AND tenant_id = $2 AND deleted_at IS NULL`,
		id, tenantID,
	).Scan(&source, &status)
	if err != nil {
		return false // not a content_reviews row (or not found) — not a persona action
	}
	if source != "persona_action" {
		return false // an editorial review, not a persona action — leave to other flows
	}

	// Map the moderation action to a content_reviews status. content_reviews
	// allows 'pending','approved','changes_requested','rejected' (000018).
	var newCRStatus string
	switch action {
	case "approve":
		newCRStatus = "approved"
	case "reject", "delete":
		newCRStatus = "rejected"
	default:
		return true
	}

	// Update only status and reviewer_user_id. The persona_payload column
	// stores the actionPayload JSON that DispatchAction reads to route the
	// approved action to the correct existing endpoint (ADR-077 §5,
	// migration 000482). Overwriting it with the admin's reason string would
	// destroy the payload and the approved action would never dispatch
	// (BUG-001). The reason is logged for audit purposes instead.
	_, err = h.DB.Writer.Exec(ctx,
		`UPDATE content_reviews
		 SET status = $1, reviewer_user_id = $2, updated_at = now()
		 WHERE id = $3 AND tenant_id = $4 AND source = 'persona_action'`,
		newCRStatus, reviewerID, id, tenantID,
	)
	if err != nil {
		slog.Error("moderation: persona action review update failed", "id", id, "error", err)
		return true
	}
	if reason != "" {
		slog.Info("moderation: persona action review reason",
			"id", id.String(), "action", action, "reason", reason)
	}

	// On approve, enqueue persona:dispatch_action so the action is published
	// (ADR-077 §4/§5). The dispatch task loads the review by ID, verifies
	// status='approved', and publishes the content.
	if action == "approve" && h.Tasks != nil {
		payload, _ := json.Marshal(map[string]string{"action_id": id.String()})
		if err := h.Tasks.Enqueue("persona:dispatch_action", payload); err != nil {
			slog.Error("moderation: enqueue persona:dispatch_action failed", "id", id, "error", err)
		}
	}
	return true
}

// handleStudioReview handles a content_reviews row with source='studio'
// (§75, ADR-076 §3). It returns true if the ID matched a studio HITL review
// (so the caller can log it). On approve, it delegates to the injected
// StudioReviewHook which publishes the draft content, transitions
// studio_jobs to 'published', and transitions the linked intel_records row
// to 'published'. On reject, it transitions studio_jobs to 'rejected'.
// The content_reviews status is updated first so the admin UI reflects the
// decision immediately.
func (h *Handlers) handleStudioReview(ctx context.Context, id, tenantID, reviewerID pgtype.UUID, action, reason string) bool {
	// Check whether this ID is a studio content_reviews row.
	var source, status string
	err := h.DB.Reader.QueryRow(ctx,
		`SELECT source, status FROM content_reviews
		 WHERE id = $1 AND tenant_id = $2 AND deleted_at IS NULL`,
		id, tenantID,
	).Scan(&source, &status)
	if err != nil {
		return false // not a content_reviews row (or not found)
	}
	if source != "studio" {
		return false // not a studio review — leave to other flows
	}

	// Map the moderation action to a content_reviews status.
	var newCRStatus string
	switch action {
	case "approve":
		newCRStatus = "approved"
	case "reject", "delete":
		newCRStatus = "rejected"
	default:
		return true
	}

	// Update the content_reviews row status + reviewer.
	_, err = h.DB.Writer.Exec(ctx,
		`UPDATE content_reviews
		 SET status = $1, reviewer_user_id = $2, updated_at = now()
		 WHERE id = $3 AND tenant_id = $4 AND source = 'studio'`,
		newCRStatus, reviewerID, id, tenantID,
	)
	if err != nil {
		slog.Error("moderation: studio review update failed", "id", id, "error", err)
		return true
	}
	if reason != "" {
		slog.Info("moderation: studio review reason",
			"id", id.String(), "action", action, "reason", reason)
	}

	// Delegate studio_jobs + intel_records transitions to the studio hook.
	// The hook looks up the studio_jobs row by review_id and handles the
	// full transition (publish content, update job status, transition
	// intel_record). When no hook is wired, the content_reviews status is
	// still updated above so the admin UI is consistent — but studio_jobs
	// would remain 'in_review' (logged for ops visibility).
	if h.StudioHook == nil {
		slog.Warn("moderation: studio review hook not wired, studio_jobs status not transitioned",
			"review_id", id, "action", action)
		return true
	}
	switch action {
	case "approve":
		if err := h.StudioHook.OnStudioReviewApproved(ctx, tenantID, id); err != nil {
			slog.Error("moderation: studio review approve hook failed",
				"review_id", id, "error", err)
		}
	case "reject", "delete":
		if err := h.StudioHook.OnStudioReviewRejected(ctx, tenantID, id); err != nil {
			slog.Error("moderation: studio review reject hook failed",
				"review_id", id, "error", err)
		}
	}
	return true
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
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid content ID")
		return
	}
	err := h.Queries.RemoveContentByAdmin(r.Context(), gen.RemoveContentByAdminParams{
		ID:       cid,
		TenantID: tenantID,
	})
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to remove content")
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"id": id, "status": "removed"})
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
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid content ID")
		return
	}
	err := h.Queries.RestoreContentByAdmin(r.Context(), gen.RestoreContentByAdminParams{
		ID:       cid,
		TenantID: tenantID,
	})
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to restore content")
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"id": id, "status": "restored"})
}

// ListDeletedContents handles GET /admin/deletions.
// Returns soft-deleted and removed content items for the admin deletions audit view.
// Ref: sections/19-gdpr-ndpr-compliance.md, Rule 11 (soft deletes only for user content)
func (h *Handlers) ListDeletedContents(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	limit := int32(100) //nolint:gosec // G115: admin listing, bounded
	if v := r.URL.Query().Get("page_size"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 && parsed <= 500 {
			limit = int32(parsed) //nolint:gosec // G115: bounded by 500 check
		}
	}

	rows, err := h.Queries.ListDeletedContents(r.Context(), gen.ListDeletedContentsParams{
		TenantID: tenantID,
		Limit:    limit,
	})
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to list deleted content")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": rows})
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
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to list broadcasts")
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
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": broadcasts,
		"meta": map[string]any{"total": total},
	})
}

// DeleteBroadcast handles DELETE /admin/broadcasts/:id.
func (h *Handlers) DeleteBroadcast(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var bid pgtype.UUID
	if err := bid.Scan(id); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid broadcast ID")
		return
	}
	err := h.Queries.DeleteAdminBroadcast(r.Context(), bid)
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to delete broadcast")
		return
	}
	errpkg.WriteJSON(w, http.StatusNoContent, nil)
}
