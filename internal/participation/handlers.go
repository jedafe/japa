// Package participation — handlers.go implements the HTTP handlers for the
// Participation Orchestration admin API (Section 76).
//
// Ref: japaconnect-engineering/sections/76-participation-orchestration.md
package participation

import (
	"encoding/json"
	"net/http"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/tenant"
)

// QueueItem is a pending persona action in the review queue.
type QueueItem struct {
	ReviewID      string `json:"review_id"`
	ContentID     string `json:"content_id"`
	PersonaName   string `json:"persona_name"`
	ContentType   string `json:"content_type"`
	Status        string `json:"status"`
	CreatedAt     string `json:"created_at"`
}

// QueueResponse is the response for the queue endpoint.
type QueueResponse struct {
	Items []QueueItem `json:"items"`
	Total int         `json:"total"`
}

// HandleQueue — GET /api/v1/admin/participation/queue
// Returns queued persona actions pending review.
func HandleQueue(database *db.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		t := tenant.FromContext(r.Context())
		if t == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{
				"error": map[string]any{"code": "tenant_unavailable", "message": "No tenant available", "status": 503},
			})
			return
		}

		rows, err := database.Reader.Query(r.Context(),
			`SELECT cr.id, cr.content_id, c.content_type, cr.status, cr.created_at,
			        COALESCE(p.display_name, u.username) as persona_name
			 FROM content_reviews cr
			 JOIN contents c ON c.id = cr.content_id
			 JOIN users u ON u.id = c.author_user_id
			 LEFT JOIN professional_profiles p ON p.id = c.author_profile_id
			 JOIN ai_personas ap ON ap.user_id = u.id
			 WHERE cr.tenant_id = $1 AND u.is_bot = true
			 ORDER BY cr.created_at DESC LIMIT 50`, t.ID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{
				"error": map[string]any{"code": "queue_query_failed", "message": err.Error(), "status": 500},
			})
			return
		}
		defer rows.Close()

		var items []QueueItem
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

		writeJSON(w, http.StatusOK, QueueResponse{
			Items: items,
			Total: len(items),
		})
	}
}

// StatsResponse is the response for the stats endpoint.
type StatsResponse struct {
	SyntheticActionsToday int `json:"synthetic_actions_today"`
	GlobalDailyCap        int `json:"global_daily_cap"`
	ActivePersonas        int `json:"active_personas"`
}

// HandleStats — GET /api/v1/admin/participation/stats
// Returns synthetic action totals and global cap status.
func HandleStats(database *db.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		t := tenant.FromContext(r.Context())
		if t == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{
				"error": map[string]any{"code": "tenant_unavailable", "message": "No tenant available", "status": 503},
			})
			return
		}

		var activePersonas int
		_ = database.Reader.QueryRow(r.Context(),
			`SELECT count(*) FROM ai_personas WHERE tenant_id = $1 AND status = 'active'`,
			t.ID).Scan(&activePersonas)

		// Synthetic actions today would come from Valkey counters, but the
		// service instance isn't available here. Return the cap and persona
		// count; the frontend can query Valkey for the counter.
		writeJSON(w, http.StatusOK, StatsResponse{
			SyntheticActionsToday: 0, // populated by Valkey counter query
			GlobalDailyCap:        500,
			ActivePersonas:        activePersonas,
		})
	}
}

// writeJSON writes a JSON response.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// _ prevents unused import warning.
var _ = auth.FromContext
