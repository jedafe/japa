// Package participation — handlers.go implements the HTTP handlers for the
// Participation Orchestration admin API (Section 76).
//
// Ref: japaconnect-engineering/sections/76-participation-orchestration.md
package participation

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/valkey-io/valkey-go/valkeycompat"

	"github.com/jedafe/japa/internal/config"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/tenant"
	"github.com/jedafe/japa/internal/valkey"
)

// QueueItem is a pending persona action in the review queue.
type QueueItem struct {
	ReviewID    string `json:"review_id"`
	ContentID   string `json:"content_id"`
	PersonaName string `json:"persona_name"`
	ContentType string `json:"content_type"`
	Status      string `json:"status"`
	CreatedAt   string `json:"created_at"`
}

// QueueResponse is the data payload for the queue endpoint.
type QueueResponse struct {
	Items      []QueueItem `json:"items"`
	Total      int         `json:"total"`
	Page       int         `json:"page"`
	PageSize   int         `json:"page_size"`
	TotalPages int         `json:"total_pages"`
}

// StatsResponse is the data payload for the stats endpoint.
type StatsResponse struct {
	SyntheticActionsToday int `json:"synthetic_actions_today"`
	GlobalDailyCap        int `json:"global_daily_cap"`
	ActivePersonas        int `json:"active_personas"`
}

// Handlers holds the dependencies for the participation admin HTTP handlers.
// The service is injected so the handlers can read Valkey counters and
// system_settings (BUG-005/013 — previously the stats endpoint returned a
// hardcoded 0 and a hardcoded cap, and the comment suggested the frontend
// query Valkey directly, which a SPA cannot do per Rule 16).
type Handlers struct {
	database *db.DB
	service  *Service
	settings *config.Settings
	vk       valkeycompat.Cmdable
}

// NewHandlers creates a Handlers backed by the given database, service,
// settings, and Valkey client.
func NewHandlers(database *db.DB, service *Service, settings *config.Settings, vk valkeycompat.Cmdable) *Handlers {
	return &Handlers{database: database, service: service, settings: settings, vk: vk}
}

// HandleQueue — GET /api/v1/admin/participation/queue
// Returns queued persona actions pending review, filtered by
// source='persona_action' AND status='pending' (§76, migration 000411).
// Supports server-side pagination (page, page_size), search (q), and
// sort (sort + dir) per the admin list-endpoint spec.
func (h *Handlers) HandleQueue(w http.ResponseWriter, r *http.Request) {
	t := tenant.FromContext(r.Context())
	if t == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"error": map[string]any{"code": "tenant_unavailable", "message": "No tenant available", "status": 503},
		})
		return
	}

	page, pageSize := parsePagination(r)
	sortCol, sortDir := parseSort(r, map[string]bool{"created_at": true, "persona_name": true, "content_type": true, "status": true}, "created_at", "desc")
	search := strings.TrimSpace(r.URL.Query().Get("q"))

	// Count total matching rows for pagination meta.
	var total int
	countArgs := []interface{}{t.ID}
	countQuery := `SELECT count(*) FROM content_reviews cr
		 JOIN contents c ON c.id = cr.content_id
		 JOIN users u ON u.id = c.author_user_id
		 LEFT JOIN professional_profiles p ON p.id = c.author_profile_id
		 WHERE cr.tenant_id = $1 AND cr.source = 'persona_action' AND cr.status = 'pending' AND cr.deleted_at IS NULL`
	if search != "" {
		countQuery += ` AND (COALESCE(p.display_name, u.username) ILIKE '%' || $2 || '%' OR c.content_type ILIKE '%' || $2 || '%')`
		countArgs = append(countArgs, search)
	}
	if err := h.database.Reader.QueryRow(r.Context(), countQuery, countArgs...).Scan(&total); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{
			"error": map[string]any{"code": "queue_count_failed", "message": err.Error(), "status": 500},
		})
		return
	}

	// Build the order-by clause from the parsed sort. Whitelisted columns
	// prevent SQL injection (the sort value is validated against the
	// allowed map in parseSort).
	orderCol := sortCol
	if orderCol == "persona_name" {
		orderCol = "COALESCE(p.display_name, u.username)"
	}
	offset := (page - 1) * pageSize
	listArgs := []interface{}{t.ID, pageSize, offset}
	listQuery := `SELECT cr.id, cr.content_id, c.content_type, cr.status, cr.created_at,
			        COALESCE(p.display_name, u.username) as persona_name
			 FROM content_reviews cr
			 JOIN contents c ON c.id = cr.content_id
			 JOIN users u ON u.id = c.author_user_id
			 LEFT JOIN professional_profiles p ON p.id = c.author_profile_id
			 WHERE cr.tenant_id = $1 AND cr.source = 'persona_action' AND cr.status = 'pending' AND cr.deleted_at IS NULL`
	argIdx := 2
	if search != "" {
		listQuery += ` AND (COALESCE(p.display_name, u.username) ILIKE '%' || $` + strconv.Itoa(argIdx) + ` || '%' OR c.content_type ILIKE '%' || $` + strconv.Itoa(argIdx) + ` || '%')`
		listArgs = append(listArgs, search)
		argIdx++
	}
	// Re-index page/pageSize/offset after the optional search param.
	listQuery += ` ORDER BY ` + orderCol + ` ` + sortDir + ` LIMIT $` + strconv.Itoa(argIdx) + ` OFFSET $` + strconv.Itoa(argIdx+1)
	listArgs = append(listArgs, pageSize, offset)

	rows, err := h.database.Reader.Query(r.Context(), listQuery, listArgs...)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{
			"error": map[string]any{"code": "queue_query_failed", "message": err.Error(), "status": 500},
		})
		return
	}
	defer rows.Close()

	items := make([]QueueItem, 0, pageSize)
	for rows.Next() {
		var item QueueItem
		var reviewID, contentID pgtype.UUID
		if err := rows.Scan(&reviewID, &contentID, &item.ContentType, &item.Status, &item.CreatedAt, &item.PersonaName); err != nil {
			continue
		}
		item.ReviewID = reviewID.String()
		item.ContentID = contentID.String()
		items = append(items, item)
	}

	totalPages := 0
	if total > 0 {
		totalPages = (total + pageSize - 1) / pageSize
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": QueueResponse{
			Items:      items,
			Total:      total,
			Page:       page,
			PageSize:   pageSize,
			TotalPages: totalPages,
		},
	})
}

// HandleStats — GET /api/v1/admin/participation/stats
// Returns synthetic action totals (read from the Valkey global daily counter,
// BUG-005) and the global cap (read from system_settings
// participation.global_daily_cap, BUG-013). The tenant for the counter is
// taken from the request context.
func (h *Handlers) HandleStats(w http.ResponseWriter, r *http.Request) {
	t := tenant.FromContext(r.Context())
	if t == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"error": map[string]any{"code": "tenant_unavailable", "message": "No tenant available", "status": 503},
		})
		return
	}

	var activePersonas int
	_ = h.database.Reader.QueryRow(r.Context(),
		`SELECT count(*) FROM ai_personas WHERE tenant_id = $1 AND status = 'active'`,
		t.ID).Scan(&activePersonas)

	// Synthetic actions today from the Valkey global daily counter
	// (jc:persona:global:{tenant_id}:{date}, §76). A SPA cannot query
	// Valkey directly (Rule 16), so the backend must return this value.
	syntheticToday := 0
	today := time.Now().UTC().Format("2006-01-02")
	if h.vk != nil {
		key := valkey.MustKey("persona:global:%s:%s", t.ID.String(), today)
		if v, err := h.vk.Get(r.Context(), key).Int(); err == nil {
			syntheticToday = v
		}
	}

	// Global daily cap from system_settings (BUG-013 — was hardcoded 500).
	globalCap := 500
	if h.settings != nil {
		globalCap = h.settings.GetIntOr(r.Context(), t.ID, "participation.global_daily_cap", 500)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": StatsResponse{
			SyntheticActionsToday: syntheticToday,
			GlobalDailyCap:        globalCap,
			ActivePersonas:        activePersonas,
		},
	})
}

// parsePagination extracts page and page_size from the query string with
// sane defaults and bounds. page is 1-based; page_size defaults to 25 and is
// clamped to [1, 100].
func parsePagination(r *http.Request) (int, int) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	pageSize, _ := strconv.Atoi(r.URL.Query().Get("page_size"))
	if pageSize < 1 {
		pageSize = 25
	}
	if pageSize > 100 {
		pageSize = 100
	}
	return page, pageSize
}

// parseSort extracts the sort column and direction from the query string,
// validating the column against an allow-list and the direction against
// asc/desc. Returns the defaults on any invalid input.
func parseSort(r *http.Request, allowed map[string]bool, defaultCol, defaultDir string) (string, string) {
	col := r.URL.Query().Get("sort")
	if !allowed[col] {
		col = defaultCol
	}
	dir := strings.ToLower(r.URL.Query().Get("dir"))
	if dir != "asc" && dir != "desc" {
		dir = defaultDir
	}
	return col, dir
}

// writeJSON writes a JSON response.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
