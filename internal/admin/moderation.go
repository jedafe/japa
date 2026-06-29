package admin

import (
	"encoding/json"
	"net/http"
)

// ModerationItem represents a moderation queue item.
type ModerationItem struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Status   string `json:"status"`
	Content  string `json:"content"`
	Reporter string `json:"reporter"`
	Created  string `json:"created_at"`
}

// ListModeration handles GET /api/v1/admin/moderation.
// Returns stub results for now.
func (h *Handlers) ListModeration(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}
	if _, ok := requireTenant(w, r); !ok {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"items":      []ModerationItem{},
			"page":       0,
			"total_pages": 0,
		},
	})
}

// BulkModeration handles POST /api/v1/admin/moderation/bulk.
// Returns stub results for now.
func (h *Handlers) BulkModeration(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}
	if _, ok := requireTenant(w, r); !ok {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"processed": 0,
		},
	})
}

// AppealModeration handles POST /api/v1/admin/moderation/{id}/appeal.
// Returns stub results for now.
func (h *Handlers) AppealModeration(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}
	if _, ok := requireTenant(w, r); !ok {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"status": "submitted",
		},
	})
}

// BroadcastRequest is the JSON body for POST /api/v1/admin/broadcast.
type BroadcastRequest struct {
	Subject  string `json:"subject"`
	Body     string `json:"body"`
	Target   string `json:"target"`
	Schedule string `json:"schedule"`
}

// Broadcast handles POST /api/v1/admin/broadcast.
// Returns stub results for now.
func (h *Handlers) Broadcast(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}
	if _, ok := requireTenant(w, r); !ok {
		return
	}
	var req BroadcastRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Request body is not valid JSON")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"sent":      0,
			"scheduled": false,
		},
	})
}
