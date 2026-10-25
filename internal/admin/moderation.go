package admin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/appeals"
	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/organizations"
	"github.com/jedafe/japa/internal/rbac"
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
//
// and three source tables; breaking it up would not reduce the distinct checks.
//
//nolint:gocyclo,funlen // admin queue endpoint combines pagination, filtering,
func (h *Handlers) ListModeration(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
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
		h.listAdCampaignModeration(w, r, ctx, limit, offset, filterStatus, filterSearch)
		return
	}

	// Build a unified moderation queue from:
	// - reviews (§29 user-generated reviews)
	// - content_reviews with source='persona_action' (§76)
	// - ai_hitl_reviews with source='studio' (§75, ADR-076 §3)
	// Filters are passed through for each sub-query; an empty filter string
	// means "no filter" for that sub-query. Placeholders are shared across
	// all three sub-queries so we can use a single arg list.
	listArgs := []interface{}{filterStatus, filterContentType, "%" + filterSearch + "%", limit, offset}

	query := `
SELECT id, body, status, entity_type, created_at, reviewer_user_id, author_username, queue_source
FROM (
    SELECT r.id,
           r.body,
           r.status,
           r.entity_type,
           r.created_at,
           r.reviewer_user_id,
           COALESCE(u.username, 'unknown') AS author_username,
           'review' AS queue_source
    FROM reviews r
    LEFT JOIN users u ON u.id = r.reviewer_user_id
    WHERE ($1::text = '' OR r.status = $1)
      AND ($2::text = '' OR r.entity_type = $2)
      AND ($3::text = '%' OR (r.body ILIKE $3 OR r.title ILIKE $3))
    UNION ALL
    SELECT cr.id,
           cr.feedback AS body,
           cr.status,
           cr.source AS entity_type,
           cr.created_at,
           cr.reviewer_user_id,
           COALESCE(u.username, 'unknown') AS author_username,
           'content_review' AS queue_source
    FROM content_reviews cr
    LEFT JOIN users u ON u.id = cr.reviewer_user_id
    WHERE cr.source = 'persona_action'
      AND ($1::text = '' OR cr.status = $1)
      AND ($2::text = '' OR cr.source = $2)
      AND ($3::text = '%' OR cr.feedback ILIKE $3)
    UNION ALL
    SELECT ah.id,
           COALESCE(ah.human_feedback::text, ah.ai_output::text) AS body,
           ah.status,
           ah.content_type AS entity_type,
           ah.created_at,
           ah.reviewer_id AS reviewer_user_id,
           COALESCE(u.username, 'unknown') AS author_username,
           'ai_hitl' AS queue_source
    FROM ai_hitl_reviews ah
    LEFT JOIN users u ON u.id = ah.reviewer_id
    WHERE ah.source = 'studio'
      AND ($1::text = '' OR ah.status = $1)
      AND ($2::text = '' OR ah.content_type = $2)
      AND ($3::text = '%' OR (ah.human_feedback::text ILIKE $3 OR ah.ai_output::text ILIKE $3))
    UNION ALL
    -- §§107–110 tool entities awaiting HITL review (queue_source='tool')
    SELECT cr.id, cr.version_label, cr.status, 'crs_ruleset', cr.created_at,
           NULL::uuid, 'admin', 'tool'
    FROM crs_rulesets cr
    WHERE cr.status IN ('draft','ai_draft','pending')
      AND ($1::text = '' OR cr.status = $1)
      AND ($2::text = '' OR 'crs_ruleset' = $2)
      AND ($3::text = '%' OR cr.version_label ILIKE $3)
    UNION ALL
    SELECT p.id, p.program_label || ' (' || p.visa_type || ')', p.status, 'pof_requirement',
           p.created_at, NULL::uuid, 'admin', 'tool'
    FROM pof_requirements p
    WHERE p.status IN ('draft','ai_draft','pending')
      AND ($1::text = '' OR p.status = $1)
      AND ($2::text = '' OR 'pof_requirement' = $2)
      AND ($3::text = '%' OR (p.program_label ILIKE $3 OR p.visa_type ILIKE $3))
    UNION ALL
    SELECT g.id, g.purpose || ' guide — ' || COALESCE(ep.name,''), g.status, 'eca_guide',
           g.created_at, NULL::uuid, 'admin', 'tool'
    FROM eca_guides g
    LEFT JOIN eca_providers ep ON ep.id = g.provider_id
    WHERE g.status IN ('draft','ai_draft','pending')
      AND ($1::text = '' OR g.status = $1)
      AND ($2::text = '' OR 'eca_guide' = $2)
      AND ($3::text = '%' OR g.purpose ILIKE $3)
    UNION ALL
    SELECT cu.id, COALESCE(cu.note, cu.fee_estimate, ''), cu.status, 'eca_update',
           cu.created_at, cu.user_id, COALESCE(up.display_name, 'member'), 'tool'
    FROM eca_community_updates cu
    LEFT JOIN user_profiles up ON up.user_id = cu.user_id
    WHERE cu.status = 'pending' AND cu.deleted_at IS NULL
      AND ($1::text = '' OR cu.status = $1)
      AND ($2::text = '' OR 'eca_update' = $2)
      AND ($3::text = '%' OR COALESCE(cu.note,'') ILIKE $3)
    UNION ALL
    SELECT st.id, st.title, st.status, 'sop_template', st.created_at,
           NULL::uuid, 'admin', 'tool'
    FROM sop_templates st
    WHERE st.status IN ('draft','ai_draft','pending')
      AND ($1::text = '' OR st.status = $1)
      AND ($2::text = '' OR 'sop_template' = $2)
      AND ($3::text = '%' OR st.title ILIKE $3)
    UNION ALL
    SELECT ss.id, ss.title, ss.status, 'sop_sample', ss.created_at,
           NULL::uuid, 'admin', 'tool'
    FROM sop_samples ss
    WHERE ss.status IN ('draft','ai_draft','pending')
      AND ($1::text = '' OR ss.status = $1)
      AND ($2::text = '' OR 'sop_sample' = $2)
      AND ($3::text = '%' OR ss.title ILIKE $3)
) combined
ORDER BY created_at DESC
LIMIT $4 OFFSET $5`

	rows, err := h.DB.Reader.Query(ctx, query, listArgs...)
	if err != nil {
		slog.Error("moderation: querying unified queue", "error", err)
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
		QueueSource    string
	}

	var rawRows []reviewRow

	for rows.Next() {
		var r reviewRow
		if err := rows.Scan(&r.ID, &r.Body, &r.Status, &r.EntityType, &r.CreatedAt, &r.ReviewerUserID, &r.AuthorUsername, &r.QueueSource); err != nil {
			slog.Error("moderation: scanning review row", "error", err)
			continue
		}

		rawRows = append(rawRows, r)
	}

	// Count total with same filters (without LIMIT/OFFSET).
	countArgs := []interface{}{filterStatus, filterContentType, "%" + filterSearch + "%"}
	countQuery := `
SELECT COALESCE(SUM(cnt), 0)::int FROM (
    SELECT COUNT(*) AS cnt
    FROM reviews r
    WHERE ($1::text = '' OR r.status = $1)
      AND ($2::text = '' OR r.entity_type = $2)
      AND ($3::text = '%' OR (r.body ILIKE $3 OR r.title ILIKE $3))
    UNION ALL
    SELECT COUNT(*)
    FROM content_reviews cr
    WHERE cr.source = 'persona_action'
      AND ($1::text = '' OR cr.status = $1)
      AND ($2::text = '' OR cr.source = $2)
      AND ($3::text = '%' OR cr.feedback ILIKE $3)
    UNION ALL
    SELECT COUNT(*)
    FROM ai_hitl_reviews ah
    WHERE ah.source = 'studio'
      AND ($1::text = '' OR ah.status = $1)
      AND ($2::text = '' OR ah.content_type = $2)
      AND ($3::text = '%' OR (ah.human_feedback::text ILIKE $3 OR ah.ai_output::text ILIKE $3))
    UNION ALL
    -- §§107–110 tool entities (mirrors the list query)
    SELECT COUNT(*) FROM crs_rulesets cr
    WHERE cr.status IN ('draft','ai_draft','pending')
      AND ($1::text = '' OR cr.status = $1)
      AND ($2::text = '' OR 'crs_ruleset' = $2)
      AND ($3::text = '%' OR cr.version_label ILIKE $3)
    UNION ALL
    SELECT COUNT(*) FROM pof_requirements p
    WHERE p.status IN ('draft','ai_draft','pending')
      AND ($1::text = '' OR p.status = $1)
      AND ($2::text = '' OR 'pof_requirement' = $2)
      AND ($3::text = '%' OR (p.program_label ILIKE $3 OR p.visa_type ILIKE $3))
    UNION ALL
    SELECT COUNT(*) FROM eca_guides g
    WHERE g.status IN ('draft','ai_draft','pending')
      AND ($1::text = '' OR g.status = $1)
      AND ($2::text = '' OR 'eca_guide' = $2)
      AND ($3::text = '%' OR g.purpose ILIKE $3)
    UNION ALL
    SELECT COUNT(*) FROM eca_community_updates cu
    WHERE cu.status = 'pending' AND cu.deleted_at IS NULL
      AND ($1::text = '' OR cu.status = $1)
      AND ($2::text = '' OR 'eca_update' = $2)
      AND ($3::text = '%' OR COALESCE(cu.note,'') ILIKE $3)
    UNION ALL
    SELECT COUNT(*) FROM sop_templates st
    WHERE st.status IN ('draft','ai_draft','pending')
      AND ($1::text = '' OR st.status = $1)
      AND ($2::text = '' OR 'sop_template' = $2)
      AND ($3::text = '%' OR st.title ILIKE $3)
    UNION ALL
    SELECT COUNT(*) FROM sop_samples ss
    WHERE ss.status IN ('draft','ai_draft','pending')
      AND ($1::text = '' OR ss.status = $1)
      AND ($2::text = '' OR 'sop_sample' = $2)
      AND ($3::text = '%' OR ss.title ILIKE $3)
) t`

	var total int32
	if err := h.DB.Reader.QueryRow(ctx, countQuery, countArgs...).Scan(&total); err != nil {
		slog.Warn("moderation: counting unified queue", "error", err)
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
			EntityRef: row.QueueSource + ":" + row.ID.String(),
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

// ModerationOption is a single value/label pair used in filter dropdowns.
type ModerationOption struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// labelForModerationType produces a human-readable label from a raw entity_type
// value (e.g. "ad_campaign" -> "Ad Campaign").
func labelForModerationType(v string) string {
	if v == "" {
		return v
	}

	parts := strings.Split(v, "_")
	for i, p := range parts {
		if len(p) > 0 {
			parts[i] = strings.ToUpper(p[:1]) + p[1:]
		}
	}

	return strings.Join(parts, " ")
}

// GetModerationOptions handles GET /api/v1/admin/moderation/options.
// Returns the canonical status and content_type (entity_type) options for the
// moderation queue, including platform-specific entity types from the reviews
// table (Rule 16).
func (h *Handlers) GetModerationOptions(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}

	statuses := []ModerationOption{
		{Value: "pending", Label: "Pending"},
		{Value: "flagged", Label: "Flagged"},
		{Value: "published", Label: "Published"},
		{Value: "approved", Label: "Approved"},
		{Value: "rejected", Label: "Rejected"},
		{Value: "changes_requested", Label: "Changes Requested"},
		{Value: "removed", Label: "Removed"},
	}

	contentTypes := []ModerationOption{
		{Value: "organization", Label: "Organization"},
		{Value: "service", Label: "Service"},
		{Value: "mentor", Label: "Mentor"},
		{Value: "university", Label: "University"},
		{Value: "ad_campaign", Label: "Ad Campaign"},
		{Value: "persona_action", Label: "Persona Action"},
	}

	seen := make(map[string]bool)
	for i := range contentTypes {
		seen[contentTypes[i].Value] = true
	}

	if h.DB != nil {
		// Reviews entity types.
		rows, err := h.DB.Reader.Query(r.Context(), `SELECT DISTINCT entity_type FROM reviews WHERE entity_type <> ''`)
		if err == nil {
			defer rows.Close()

			for rows.Next() {
				var et string
				if err := rows.Scan(&et); err != nil {
					continue
				}

				if !seen[et] {
					seen[et] = true

					contentTypes = append(contentTypes, ModerationOption{Value: et, Label: labelForModerationType(et)})
				}
			}
		}

		// Studio HITL content types.
		rows2, err := h.DB.Reader.Query(r.Context(), `SELECT DISTINCT content_type FROM ai_hitl_reviews WHERE content_type <> '' AND source = 'studio'`)
		if err == nil {
			defer rows2.Close()

			for rows2.Next() {
				var ct string
				if err := rows2.Scan(&ct); err != nil {
					continue
				}

				if !seen[ct] {
					seen[ct] = true

					contentTypes = append(contentTypes, ModerationOption{Value: ct, Label: labelForModerationType(ct)})
				}
			}
		}
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"statuses":      statuses,
			"content_types": contentTypes,
		},
	})
}

// listAdCampaignModeration queries ad_campaigns for the moderation queue
// when content_type=ad_campaign. Returns pending-review campaigns with
// pagination. Ref: §61 Review Gate, ADR-079 §3.
func (h *Handlers) listAdCampaignModeration(
	w http.ResponseWriter, r *http.Request,
	ctx context.Context,
	limit, offset int32, filterStatus, filterSearch string,
) {
	where := "ac.organization_id IS NOT NULL"
	args := []interface{}{}
	argIdx := 1

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
	countQuery := "SELECT COUNT(*)::int FROM ad_campaigns ac WHERE " + where
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
			Status: newStatus,
			ID:     reviewID,
		})
		if err != nil {
			slog.Warn("moderation: updating review status", "id", action.ID, "error", err)
			continue
		}

		processed++
	}

	slog.Info("bulk moderation completed", "user_id", user.UserID.String(), "total_actions", len(req.Actions), "processed", processed)

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

	tag, err := h.Queries.AppealReviewToFlagged(r.Context(),
		reviewID,
	)
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
// Creates a system_announcement notification for all users in the platform.
func (h *Handlers) Broadcast(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
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

	// Scheduled broadcast (§35): persist a pending row; the
	// broadcast:dispatch_scheduled Asynq sweep (every minute) delivers it
	// when scheduled_at is due and stamps sent_at + recipient_count.
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

		target := req.Target
		if target == "" {
			target = "all"
		}

		var targetID pgtype.UUID
		if req.TargetID != "" {
			if scanErr := targetID.Scan(req.TargetID); scanErr != nil {
				errpkg.WriteError(w, http.StatusBadRequest, "invalid_target_id", "target_id must be a UUID")
				return
			}
		}

		row, schedErr := h.Queries.CreateScheduledBroadcast(ctx, gen.CreateScheduledBroadcastParams{
			Title:       req.Subject,
			Body:        req.Body,
			SentBy:      user.UserID,
			ScheduledAt: pgtype.Timestamptz{Time: scheduledAt, Valid: true},
			Target:      target,
			TargetID:    targetID,
		})
		if schedErr != nil {
			slog.Error("broadcast: failed to schedule", "error", schedErr)
			errpkg.WriteError(w, http.StatusInternalServerError, "broadcast_error", "Failed to schedule broadcast")

			return
		}

		errpkg.WriteJSON(w, http.StatusAccepted, map[string]any{
			"data": map[string]any{
				"id":           row.ID.String(),
				"scheduled":    true,
				"scheduled_at": scheduledAt.Format(time.RFC3339),
			},
		})

		return
	}

	// Immediate broadcast: insert a notification for every active user in the
	// target scope (all, a user group, or a geo space).
	sent, err := h.broadcastToScopedUsers(ctx, user.UserID, req.Subject, req.Body, req.Target, req.TargetID)
	if err != nil {
		slog.Error("broadcast: failed to send", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "broadcast_error", "Failed to send broadcast")

		return
	}

	slog.Info("broadcast sent", "user_id", user.UserID.String(), "recipients", sent, "target", req.Target, "subject", req.Subject)

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"sent":      sent,
			"scheduled": false,
		},
	})
}

// broadcastToAllUsers inserts a system_announcement notification for every
// active user in the platform. Returns the count of notifications created.
func (h *Handlers) broadcastToAllUsers(ctx context.Context, senderID pgtype.UUID, title, body string) (int, error) {
	return h.broadcastToScopedUsers(ctx, senderID, title, body, "all", "")
}

// broadcastToScopedUsers inserts a system_announcement notification for every
// active user in the target scope. When target is "user_group" or "geo_space",
// only users in the specified group or space receive the notification.
func (h *Handlers) broadcastToScopedUsers(ctx context.Context, senderID pgtype.UUID, title, body, target, targetID string) (int, error) {
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

		rows, err = h.Queries.ListActiveUserIDsByGeoSpace(ctx,

			spaceID,
		)
	case "user_group":
		if targetID == "" {
			return 0, errors.New("target_id is required for user_group targeting")
		}

		var groupID pgtype.UUID
		if scanErr := groupID.Scan(targetID); scanErr != nil {
			return 0, fmt.Errorf("invalid target_id: %w", scanErr)
		}

		rows, err = h.Queries.ListActiveUserIDsByGroup(ctx,

			groupID,
		)
	default: // "all", "unverified", "pro_members", or empty
		rows, err = h.Queries.ListActiveUserIDs(ctx)
	}

	if err != nil {
		return 0, err
	}

	if len(rows) == 0 {
		return 0, nil
	}

	// Batch insert notifications using unnest for a single round-trip.
	err = h.Queries.CreateBroadcastNotifications(ctx, gen.CreateBroadcastNotificationsParams{
		RecipientID: senderID,
		Column2:     rows,
		Title:       title,
		Body:        &body,
		CreatedAt:   pgtype.Timestamptz{Time: time.Now(), Valid: true},
	})
	if err != nil {
		return 0, err
	}

	// Record the broadcast in admin_broadcasts.
	err = h.Queries.CreateAdminBroadcastRecord(ctx, gen.CreateAdminBroadcastRecordParams{
		Title:  title,
		Body:   body,
		SentBy: senderID,
		// #nosec G115 // value bounded by caller
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

	if input.Days < 0 || input.Days > math.MaxInt32 {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_days", "days out of range")
		return
	}

	// 404 (not a FK-violation 500) when the target user does not exist; a
	// deleted account must not be flipped back to 'banned'.
	curStatus, err := h.Queries.GetUserStatus(ctx, uid)
	if err != nil {
		errpkg.HandleServiceError(w, err)
		return
	}

	if curStatus == "deleted" {
		errpkg.WriteError(w, http.StatusConflict, "invalid_status_transition",
			"Cannot ban a deleted user")
		return
	}

	// §03 owner amendment: owner immutable; banning a super_admin is owner-only.
	if !h.guardSuperAdminTarget(w, r, uid) {
		return
	}

	err = h.Queries.CreateUserBan(ctx, gen.CreateUserBanParams{
		UserID:  uid,
		Reason:  &input.Reason,
		Column3: int32(input.Days), // #nosec G115 -- input.Days is range-checked against math.MaxInt32 above
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

	// Soft-delete the active ban row(s) first — enforcement checks
	// user_moderation_actions for a live 'ban' row, so inserting an 'unban'
	// audit record alone would leave the user blocked.
	if err := h.Queries.SoftDeleteUserBan(r.Context(), uid); err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to unban user")
		return
	}

	err := h.Queries.CreateUserUnban(r.Context(), uid)
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to unban user")
		return
	}
	// Restore users.status — 'restricted' when operation bans from a prior
	// restriction are still active, otherwise 'active'.
	newStatus := "active"

	if remaining, cerr := h.Queries.CountActiveOperationBans(r.Context(), uid); cerr != nil {
		slog.Error("unban: failed to count active operation bans", "error", cerr, "user_id", uid.String())
	} else if remaining > 0 {
		newStatus = "restricted"
	}

	if err := h.Queries.UpdateUserStatus(r.Context(), gen.UpdateUserStatusParams{
		Status: newStatus,
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

	// 404 (not a FK-violation 500) when the target user does not exist.
	if _, err := h.Queries.GetUserStatus(r.Context(), uid); err != nil {
		errpkg.HandleServiceError(w, err)
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
// Uses the typed query so the admin warnings tab can display the
// violation_type recorded by warn-typed alongside the reason.
func (h *Handlers) ListUserWarnings(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "id")

	var uid pgtype.UUID

	if err := uid.Scan(userID); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid user ID")
		return
	}

	rows, err := h.Queries.ListUserWarningsTyped(r.Context(), uid)
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to list warnings")
		return
	}

	type warnRow struct {
		ID            string `json:"id"`
		Reason        string `json:"reason"`
		ViolationType string `json:"violation_type"`
		CreatedAt     string `json:"created_at"`
	}

	warnings := make([]warnRow, 0, len(rows))

	for _, w := range rows {
		reason := ""
		if w.Reason != nil {
			reason = *w.Reason
		}

		violationType := ""
		if w.ViolationType != nil {
			violationType = *w.ViolationType
		}

		warnings = append(warnings, warnRow{
			ID:            w.ID.String(),
			Reason:        reason,
			ViolationType: violationType,
			CreatedAt:     w.CreatedAt.Time.Format(time.RFC3339),
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

	// users.status is a free-text column — whitelist the canonical values
	// (mirrors the statuses served by GET /admin/users/config) so a typo'd
	// or arbitrary string cannot corrupt the account state. Validated
	// before the DB read so malformed input cannot mask a 404.
	switch input.Status {
	case "active", "pending", "restricted", "suspended", "banned", "deleted":
	default:
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_status", "Invalid status value")
		return
	}

	// The UPDATE is a no-op for a missing user — return a real 404 instead
	// of a misleading 200.
	if _, err := h.Queries.GetUserStatus(r.Context(), uid); err != nil {
		errpkg.HandleServiceError(w, err)
		return
	}

	// §03 owner amendment: owner immutable; status changes on a super_admin
	// are owner-only.
	if !h.guardSuperAdminTarget(w, r, uid) {
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

	// Status gates session validity — drop cached access state so the new
	// status takes effect immediately.
	h.invalidateUserAccessCaches(r.Context(), uid, banCacheKey(uid), restrictionsCacheKey(uid))

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

	// The membership insert relies on the FK for a missing group — surface
	// a proper 404 instead of a raw constraint-violation 500.
	targetGroup, err := h.Queries.GetUserGroupByID(r.Context(), gid)
	if err != nil {
		errpkg.HandleServiceError(w, err)
		return
	}

	// §03 owner amendment: granting super_admin membership is owner-only.
	if targetGroup.Slug == rbac.SystemGroupSuperAdmin && !h.callerIsOwner(r) {
		errpkg.WriteError(w, http.StatusForbidden, "owner_required", "Only the owner can grant super_admin membership")
		return
	}

	err = h.Queries.AddUserGroupMembership(r.Context(), gen.AddUserGroupMembershipParams{
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

// validOrgVerificationTypes is the §7 verification badge enum for
// organizations, extended with professional_body for the §111
// package-publishing trust gate.
var validOrgVerificationTypes = map[string]struct{}{
	"business":           {},
	"embassy":            {},
	"notable":            {},
	"immigration_lawyer": {},
	"professional_body":  {},
}

func isValidOrgVerificationType(s string) bool {
	_, ok := validOrgVerificationTypes[s]
	return ok
}

// VerifyOrganization handles POST /admin/organizations/:id/verify.
func (h *Handlers) VerifyOrganization(w http.ResponseWriter, r *http.Request) {
	orgID := chi.URLParam(r, "id")

	var oid pgtype.UUID

	if err := oid.Scan(orgID); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid organization ID")
		return
	}

	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	// Optional §7 verification badge type — §111 package publishing gates
	// on business|professional_body, so the type must be settable here.
	var body struct {
		VerificationType *string `json:"verification_type"`
	}

	if r.Body != nil {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil && !errors.Is(err, io.EOF) {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
			return
		}
	}

	if body.VerificationType != nil && !isValidOrgVerificationType(*body.VerificationType) {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_verification_type",
			"verification_type must be one of business|embassy|notable|immigration_lawyer|professional_body")
		return
	}

	// Route through the organizations service so the organization.verified
	// webhook event is emitted (Section 12) and the org cache is invalidated.
	if h.Orgs != nil {
		if err := h.Orgs.VerifyOrganization(r.Context(), oid, user.UserID, body.VerificationType); err != nil {
			errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to verify organization")
			return
		}
	} else {
		err := h.Queries.VerifyOrganization(r.Context(), gen.VerifyOrganizationParams{
			ID:               oid,
			VerificationType: body.VerificationType,
		})
		if err != nil {
			errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to verify organization")
			return
		}
		// Invalidate org cache so the verified status is reflected immediately.
		organizations.InvalidateOrgCacheByCmdable(r.Context(), h.Valkey)
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"id": orgID, "verified": true}})
}

// UnverifyOrganization handles DELETE /admin/organizations/:id/verify.
// Ref: japaconnect-engineering/sections/07-organization-profiles-claim-ownership.md.
func (h *Handlers) UnverifyOrganization(w http.ResponseWriter, r *http.Request) {
	orgID := chi.URLParam(r, "id")

	var oid pgtype.UUID

	if err := oid.Scan(orgID); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid organization ID")
		return
	}

	user, ok := requireUser(w, r)
	if !ok {
		return
	}
	// Route through the organizations service so the organization.unverified
	// webhook event is emitted (Section 12) and the org cache is invalidated.
	if h.Orgs != nil {
		if err := h.Orgs.UnverifyOrganization(r.Context(), oid, user.UserID); err != nil {
			errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to unverify organization")
			return
		}
	} else {
		err := h.Queries.UnverifyOrganization(r.Context(), oid)
		if err != nil {
			errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to unverify organization")
			return
		}

		organizations.InvalidateOrgCacheByCmdable(r.Context(), h.Valkey)
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"id": orgID, "verified": false}})
}

// VerifyUser handles POST /admin/users/:id/verify — grants the §07 profile
// verification badge. Routed through the users service so the user.verified
// webhook event is emitted (Section 12) and the grant email is enqueued.
// Ref: sections/07-organization-profiles-claim-ownership.md (Verification Requests).
func (h *Handlers) VerifyUser(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "id")

	var uid pgtype.UUID

	if err := uid.Scan(userID); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid user ID")
		return
	}

	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	var body struct {
		VerificationType *string `json:"verification_type"`
	}

	if r.Body != nil {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil && !errors.Is(err, io.EOF) {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
			return
		}
	}

	if body.VerificationType != nil && !isValidOrgVerificationType(*body.VerificationType) {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_verification_type",
			"verification_type must be one of business|embassy|notable|immigration_lawyer|professional_body")
		return
	}

	if h.Users == nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Verification service unavailable")
		return
	}

	if err := h.Users.VerifyUser(r.Context(), uid, user.UserID, body.VerificationType); err != nil {
		errpkg.HandleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"id": userID, "verified": true}})
}

// UnverifyUser handles DELETE /admin/users/:id/verify — revokes the profile
// verification badge and emits user.unverified (Section 12).
func (h *Handlers) UnverifyUser(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "id")

	var uid pgtype.UUID

	if err := uid.Scan(userID); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid user ID")
		return
	}

	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	if h.Users == nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Verification service unavailable")
		return
	}

	if err := h.Users.UnverifyUser(r.Context(), uid, user.UserID); err != nil {
		errpkg.HandleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"id": userID, "verified": false}})
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
		if a.SubjectLabel != nil {
			entry["subject_label"] = *a.SubjectLabel
		}

		if a.TicketID.Valid {
			entry["ticket_id"] = a.TicketID.String()
		}

		if a.TicketNumber != nil {
			entry["ticket_number"] = *a.TicketNumber
		}

		if a.ResolutionNote != nil {
			entry["resolution_note"] = *a.ResolutionNote
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

	out := map[string]any{
		"id": a.ID.String(), "user_id": a.UserID.String(), "entity_type": a.EntityType,
		"entity_id": a.EntityID, "reason": a.Reason, "status": a.Status, "created_at": a.CreatedAt.Time.Format(time.RFC3339),
	}
	if a.SubjectLabel != nil {
		out["subject_label"] = *a.SubjectLabel
	}

	if a.TicketID.Valid {
		out["ticket_id"] = a.TicketID.String()
	}

	if a.TicketNumber != nil {
		out["ticket_number"] = *a.TicketNumber
	}

	if a.ResolutionNote != nil {
		out["resolution_note"] = *a.ResolutionNote
	}

	if a.ReviewedBy.Valid {
		out["reviewed_by"] = a.ReviewedBy.String()
	}

	if a.ResolvedAt.Valid {
		out["resolved_at"] = a.ResolvedAt.Time.Format(time.RFC3339)
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": out})
}

// UpdateAppeal handles PATCH /admin/appeals/:id. Delegates to the §22
// appeals service so 'overturn' applies the subject-type reversal
// (lift ban / restore content / approve claim) and resolves the linked
// ticket; 'upheld' and 'dismissed' close the appeal without subject
// action.
func (h *Handlers) UpdateAppeal(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var input struct {
		Status         string `json:"status"`
		ResolutionNote string `json:"resolution_note"`
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

	if h.AppealResolver == nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Appeal resolution unavailable")
		return
	}

	user := auth.FromContext(r.Context())
	if user == nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	if err := h.AppealResolver.Resolve(r.Context(), aid, user.UserID, input.Status, input.ResolutionNote); err != nil {
		switch {
		case errors.Is(err, appeals.ErrInvalidStatus):
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_status", "Status must be upheld, overturn, or dismissed")
		case errors.Is(err, appeals.ErrAlreadyResolved):
			errpkg.WriteError(w, http.StatusConflict, "already_resolved", "This appeal has already been resolved")
		case errors.Is(err, appeals.ErrReversalBlocked):
			// e.g. the appellant already spent the refunded credits — the
			// reversal can't safely complete yet; the appeal stays pending.
			errpkg.WriteError(w, http.StatusConflict, "resolution_blocked", err.Error())
		case errors.Is(err, pgx.ErrNoRows):
			errpkg.WriteError(w, http.StatusNotFound, "not_found", "Appeal not found")
		default:
			slog.Error("appeal resolution failed", "id", id, "error", err)
			errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to update appeal")
		}

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

	// AI Content Studio (§75, ADR-076 §3): try the studio HITL queues first.
	// Studio drafts live either in content_reviews (legacy path, source='studio')
	// or in ai_hitl_reviews (current path, source='studio'). If this ID matches
	// either, the hook updates the review row, transitions the studio_job, and
	// — on approve — publishes the draft. In that case we skip the generic
	// content_reviews / content_reports updates below because they have no row
	// for an ai_hitl_reviews ID and would fail the request.
	handled, hookErr := h.handleStudioReview(ctx, rid, user.UserID, input.Action, input.Reason)
	if hookErr != nil {
		// Fail closed (BUG-004): the studio hook ran before the review status
		// was committed, so the review stays pending and the reviewer sees
		// why the publish/reject step failed.
		slog.Error("moderation: studio hook failed", "item_id", id, "action", input.Action, "error", hookErr)
		errpkg.WriteError(w, http.StatusBadGateway, "studio_hook_failed",
			fmt.Sprintf("The studio %s step failed: %v. The review status has not been changed", input.Action, hookErr))

		return
	}

	if handled {
		slog.Info("moderation: studio review processed", "item_id", id, "action", input.Action, "reviewer", user.UserID.String())
	} else {
		err := h.Queries.AdminUpdateReviewStatus(ctx, gen.AdminUpdateReviewStatusParams{
			Status: newStatus,
			ID:     rid,
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
		})
		if err != nil {
			slog.Error("moderation: updating content report", "id", id, "error", err)
		}
	}

	// Participation Orchestration (§76, ADR-077 §4, BUG-001): if the ID
	// matches a content_reviews row with source='persona_action', update
	// its status and — on approve — enqueue persona:dispatch_action so the
	// approved action is actually published. Previously ModerateItem only
	// touched the `reviews` table (§29 Ratings & Reviews) and the
	// content_reviews persona-action row stayed 'pending' forever.
	if h.handlePersonaActionReview(ctx, rid, user.UserID, input.Action, input.Reason) {
		slog.Info("moderation: persona action review processed", "item_id", id, "action", input.Action, "reviewer", user.UserID.String())
	}

	// §§107–110 tool entities (crs_rulesets, pof_requirements, eca_guides,
	// eca_community_updates, sop_samples, sop_templates) surface in the
	// queue with queue_source='tool'. The hook applies the entity-level
	// transition — e.g. CRS approve activates + supersedes the prior
	// ruleset atomically — which the generic 'published'/'removed' status
	// cannot express. Fail closed: a failed transition leaves the item
	// pending rather than reporting a phantom success.
	if h.ToolReviewHook != nil {
		toolHandled, appliedStatus, toolErr := h.ToolReviewHook.HandleToolReview(ctx, rid, user.UserID, input.Action, input.Reason)
		if toolErr != nil {
			slog.Error("moderation: tool review failed", "item_id", id, "action", input.Action, "error", toolErr)
			errpkg.WriteError(w, http.StatusBadGateway, "tool_review_failed",
				fmt.Sprintf("The tool %s step failed: %v. The item status has not been changed", input.Action, toolErr))

			return
		}

		if toolHandled {
			newStatus = appliedStatus

			slog.Info("moderation: tool review processed", "item_id", id, "action", input.Action, "reviewer", user.UserID.String())
		}
	}

	slog.Info("moderation item processed", "user_id", user.UserID.String(), "item_id", id, "action", input.Action)

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
func (h *Handlers) handlePersonaActionReview(ctx context.Context, id, reviewerID pgtype.UUID, action, reason string) bool {
	// Check whether this ID is a persona-action content_reviews row.
	var source, status string

	err := h.DB.Reader.QueryRow(ctx, `SELECT source, status FROM content_reviews
		 WHERE id = $1 AND deleted_at IS NULL`, id).Scan(&source, &status)
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
	_, err = h.DB.Writer.Exec(ctx, `UPDATE content_reviews
		 SET status = $1, reviewer_user_id = $2, updated_at = now()
		 WHERE id = $3 AND source = 'persona_action'`, newCRStatus, reviewerID, id)
	if err != nil {
		slog.Error("moderation: persona action review update failed", "id", id, "error", err)
		return true
	}

	if reason != "" {
		slog.Info("moderation: persona action review reason", "id", id.String(), "action", action, "reason", reason)
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

// handleStudioReview handles an ai_hitl_reviews row (current path,
// ADR-088) or a content_reviews row with source='studio' (legacy, §75,
// ADR-076 §3). It returns (true, nil) if the ID matched (so the caller can
// log it).
//   - source='studio': the StudioReviewHook publishes the draft, transitions
//     studio_jobs to 'published'/'rejected', and transitions the linked
//     intel_records row.
//   - other sources (agent_draft_request, agent_tool, manual): the review
//     status is committed directly; on approve the DraftPublishHook takes the
//     review's content_id target live first (§103 "Approve → publish").
//
// Hooks run BEFORE the review status is committed — a hook failure is
// returned as (true, err) so the caller can fail closed and leave the
// review pending (BUG-004, matching the ai HITL approve/reject handlers).
func (h *Handlers) handleStudioReview(ctx context.Context, id, reviewerID pgtype.UUID, action, reason string) (bool, error) {
	// Try the current ai_hitl_reviews queue first (ADR-088 HITL queue).
	var hitlSource, hitlStatus, hitlTargetTable string

	var hitlContentID pgtype.UUID

	err := h.DB.Reader.QueryRow(ctx, `SELECT source, status, content_id, target_table FROM ai_hitl_reviews
		 WHERE id = $1`, id).Scan(&hitlSource, &hitlStatus, &hitlContentID, &hitlTargetTable)
	if err == nil {
		if hitlStatus != "pending" {
			slog.Warn("moderation: ai_hitl_review is not pending, skipping", "review_id", id, "status", hitlStatus)
			return true, nil
		}

		if hitlSource == "studio" {
			return h.updateAIHITLStudioReview(ctx, id, reviewerID, action, reason)
		}

		// Non-studio ai_hitl_reviews row — generic approve/reject.
		return h.updateAIHITLReview(ctx, id, reviewerID, action, reason, hitlTargetTable, hitlContentID)
	}

	// Fallback to the legacy content_reviews path.
	var source, status string

	err = h.DB.Reader.QueryRow(ctx, `SELECT source, status FROM content_reviews
		 WHERE id = $1 AND deleted_at IS NULL`, id).Scan(&source, &status)
	if err != nil {
		return false, nil // not a content_reviews row (or not found) — not a studio review
	}

	if source != "studio" {
		return false, nil // not a studio review — leave to other flows
	}

	// Map the moderation action to a content_reviews status.
	var newCRStatus string

	switch action {
	case "approve":
		newCRStatus = "approved"
	case "reject", "delete":
		newCRStatus = "rejected"
	default:
		return true, nil
	}

	// Fail closed (BUG-004): run the studio hook BEFORE committing the
	// review status so a failed publish/reject never leaves an "approved"
	// review paired with an unpublished studio job.
	if hookErr := h.callStudioHook(ctx, id, reviewerID, action); hookErr != nil {
		return true, hookErr
	}

	// Update the content_reviews row status + reviewer.
	_, err = h.DB.Writer.Exec(ctx, `UPDATE content_reviews
		 SET status = $1, reviewer_user_id = $2, updated_at = now()
		 WHERE id = $3 AND source = 'studio'`, newCRStatus, reviewerID, id)
	if err != nil {
		slog.Error("moderation: studio review update failed", "id", id, "error", err)
		return true, nil
	}

	if reason != "" {
		slog.Info("moderation: studio review reason", "id", id.String(), "action", action, "reason", reason)
	}

	return true, nil
}

// updateAIHITLStudioReview updates an ai_hitl_reviews row with source='studio'
// and invokes the studio hook. The hook runs BEFORE the status update so a
// failed publish/reject leaves the review pending (BUG-004, same behaviour as
// the ai HITL approve/reject handlers). Returns (handled, hookError).
func (h *Handlers) updateAIHITLStudioReview(ctx context.Context, id, reviewerID pgtype.UUID, action, reason string) (bool, error) {
	var newHITLStatus string

	switch action {
	case "approve":
		newHITLStatus = "approved"
	case "reject", "delete":
		newHITLStatus = "rejected"
	default:
		return true, nil
	}

	if hookErr := h.callStudioHook(ctx, id, reviewerID, action); hookErr != nil {
		return true, hookErr
	}

	var reasonPtr *string
	if reason != "" {
		reasonPtr = &reason
	}

	_, err := h.DB.Writer.Exec(ctx, `UPDATE ai_hitl_reviews
		 SET status = $1, reviewer_id = $2, reviewed_at = now(), reason = $3
		 WHERE id = $4 AND source = 'studio' AND status = 'pending'`, newHITLStatus, reviewerID, reasonPtr, id)
	if err != nil {
		slog.Error("moderation: studio ai_hitl_reviews update failed", "id", id, "error", err)
		return true, nil
	}

	if reason != "" {
		slog.Info("moderation: studio ai_hitl review reason", "id", id.String(), "action", action, "reason", reason)
	}

	return true, nil
}

// updateAIHITLReview updates a non-studio ai_hitl_reviews row
// (agent_draft_request, agent_tool, manual). On approve the DraftPublishHook
// publishes the review's content_id target BEFORE the status commits — a
// publish failure fails closed and leaves the review pending (BUG-004,
// §103 "Approve → publish"). On reject the draft simply stays unpublished.
// Returns (handled, hookError).
func (h *Handlers) updateAIHITLReview(ctx context.Context, id, reviewerID pgtype.UUID, action, reason, targetTable string, contentID pgtype.UUID) (bool, error) {
	var newHITLStatus string

	switch action {
	case "approve":
		newHITLStatus = "approved"
	case "reject", "delete":
		newHITLStatus = "rejected"
	default:
		return true, nil
	}

	if action == "approve" && contentID.Valid && h.DraftPublishHook != nil {
		if hookErr := h.DraftPublishHook.PublishHITLTarget(ctx, reviewerID, targetTable, contentID); hookErr != nil {
			slog.Error("moderation: ai_hitl publish hook failed", "review_id", id, "content_id", contentID, "target_table", targetTable, "error", hookErr)

			return true, hookErr
		}
	}

	var reasonPtr *string
	if reason != "" {
		reasonPtr = &reason
	}

	_, err := h.DB.Writer.Exec(ctx, `UPDATE ai_hitl_reviews
		 SET status = $1, reviewer_id = $2, reviewed_at = now(), reason = $3
		 WHERE id = $4 AND status = 'pending'`, newHITLStatus, reviewerID, reasonPtr, id)
	if err != nil {
		slog.Error("moderation: ai_hitl_reviews update failed", "id", id, "error", err)
		return true, nil
	}

	if reason != "" {
		slog.Info("moderation: ai_hitl review reason", "id", id.String(), "action", action, "reason", reason)
	}

	return true, nil
}

// callStudioHook invokes the injected StudioReviewHook for approve/reject.
// A hook failure is returned so callers can fail closed — the review status
// must not be committed when the linked studio_job could not transition
// (BUG-004).
func (h *Handlers) callStudioHook(ctx context.Context, id, reviewerID pgtype.UUID, action string) error {
	if h.StudioHook == nil {
		slog.Warn("moderation: studio review hook not wired, studio_jobs status not transitioned", "review_id", id, "action", action)
		return nil
	}

	switch action {
	case "approve":
		if err := h.StudioHook.OnStudioReviewApproved(ctx, id, reviewerID); err != nil {
			slog.Error("moderation: studio review approve hook failed", "review_id", id, "error", err)
			return err
		}
	case "reject", "delete":
		if err := h.StudioHook.OnStudioReviewRejected(ctx, id, reviewerID); err != nil {
			slog.Error("moderation: studio review reject hook failed", "review_id", id, "error", err)
			return err
		}
	}

	return nil
}

// ─── Content Moderation ───────────────────────────────────────────────────

// ListContent handles GET /admin/content — the admin moderation list.
// Unlike the public /content handler, this supports a soft-delete view via
// ?deleted=only|all ("" = live only) plus type/status/q filters. Kept
// separate so deleted rows can never leak to the public route.
func (h *Handlers) ListContent(w http.ResponseWriter, r *http.Request) {
	limit := int32(50)

	if v := r.URL.Query().Get("page_size"); v != "" {
		// ParseInt with bitSize 32 rejects out-of-range values — the int32
		// conversion below cannot overflow (gosec G109/G115).
		if parsed, err := strconv.ParseInt(v, 10, 32); err == nil && parsed > 0 && parsed <= 200 {
			limit = int32(parsed)
		}
	}

	var offset int32

	if v := r.URL.Query().Get("page_offset"); v != "" {
		if parsed, err := strconv.ParseInt(v, 10, 32); err == nil && parsed >= 0 {
			offset = int32(parsed)
		}
	}

	deleted := r.URL.Query().Get("deleted")
	if deleted != "" && deleted != "only" && deleted != "all" {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_filter", "deleted must be 'only' or 'all'")
		return
	}

	params := gen.AdminListContentsParams{
		Deleted:     deleted,
		ContentType: r.URL.Query().Get("type"),
		Status:      r.URL.Query().Get("status"),
		Q:           r.URL.Query().Get("q"),
		PageLimit:   limit,
		PageOffset:  offset,
	}

	rows, err := h.Queries.AdminListContents(r.Context(), params)
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to list content")
		return
	}

	total, err := h.Queries.AdminCountContents(r.Context(), gen.AdminCountContentsParams{
		Deleted:     params.Deleted,
		ContentType: params.ContentType,
		Status:      params.Status,
		Q:           params.Q,
	})
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to count content")
		return
	}

	totalPages := int32(1)
	if limit > 0 && total > 0 {
		totalPages = (total + limit - 1) / limit
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data":        rows,
		"total":       total,
		"total_pages": totalPages,
	})
}

// RemoveContent handles POST /admin/content/:id/remove.
// ?force=true permanently removes an already-soft-deleted row, gated by
// admin.hard_delete via rbac.RequireForceDelete.
func (h *Handlers) RemoveContent(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var cid pgtype.UUID

	if err := cid.Scan(id); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid content ID")
		return
	}

	if r.URL.Query().Get("force") == "true" {
		affected, err := h.Queries.HardDeleteContent(r.Context(), cid)
		if err != nil {
			if errpkg.IsFKViolation(err) {
				errpkg.WriteError(w, http.StatusConflict, "in_use", "Content is referenced by other records and cannot be permanently deleted")
				return
			}

			errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to delete content")

			return
		}

		if affected == 0 {
			exists, err := h.Queries.ContentExistsAny(r.Context(), cid)
			if err != nil {
				errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to delete content")
				return
			}

			if !exists {
				errpkg.WriteError(w, http.StatusNotFound, "not_found", "Content not found")
				return
			}

			errpkg.WriteError(w, http.StatusConflict, "not_deleted", "Content must be removed (soft-deleted) before it can be permanently deleted")

			return
		}

		w.WriteHeader(http.StatusNoContent)

		return
	}

	err := h.Queries.RemoveContentByAdmin(r.Context(),
		cid,
	)
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to remove content")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"id": id, "status": "removed"})
}

// RestoreContent handles POST /admin/content/:id/restore.
func (h *Handlers) RestoreContent(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var cid pgtype.UUID

	if err := cid.Scan(id); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid content ID")
		return
	}

	err := h.Queries.RestoreContentByAdmin(r.Context(),
		cid,
	)
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to restore content")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"id": id, "status": "restored"})
}

// ListDeletedContents handles GET /admin/deletions.
// Returns soft-deleted and removed content items for the admin deletions audit view.
// Ref: sections/19-gdpr-ndpr-compliance.md, Rule 11 (soft deletes only for user content).
func (h *Handlers) ListDeletedContents(w http.ResponseWriter, r *http.Request) {
	limit := int32(100)

	if v := r.URL.Query().Get("page_size"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 && parsed <= 500 {
			// #nosec G109 // value bounded by MaxPageSize check
			limit = int32(parsed) //nolint:gosec // G115: bounded by 500 check
		}
	}

	rows, err := h.Queries.ListDeletedContents(r.Context(),

		limit,
	)
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to list deleted content")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": rows})
}

// ─── Broadcasts ───────────────────────────────────────────────────────────

// ListBroadcasts handles GET /admin/broadcasts.
func (h *Handlers) ListBroadcasts(w http.ResponseWriter, r *http.Request) {
	rows, err := h.Queries.ListAdminBroadcasts(r.Context())
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to list broadcasts")
		return
	}

	var total int32
	if err := h.DB.Reader.QueryRow(r.Context(), `SELECT COUNT(*)::int FROM admin_broadcasts`).Scan(&total); err != nil {
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
