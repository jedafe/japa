package websocket

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
)

// parseJSONBody reads the request body and decodes it into v.
func parseJSONBody(r *http.Request, v any) error {
	return json.NewDecoder(r.Body).Decode(v)
}

// parsePageSize extracts and clamps the page_size query parameter.
func parsePageSize(r *http.Request) int32 {
	s := r.URL.Query().Get("page_size")
	if s == "" {
		return DefaultPageSize
	}
	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 {
		return DefaultPageSize
	}
	if n > MaxPageSize {
		return MaxPageSize
	}
	return int32(n)
}

// parsePageOffset extracts the page_offset query parameter.
func parsePageOffset(r *http.Request) int32 {
	s := r.URL.Query().Get("page_offset")
	if s == "" {
		return 0
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 0 {
		return 0
	}
	return int32(n)
}

// requireUserFromContext extracts the authenticated user from the request context.
func requireUserFromContext(r *http.Request) (*auth.AuthenticatedUser, bool) {
	user := auth.FromContext(r.Context())
	if user == nil {
		return nil, false
	}
	return user, true
}

// requireTenantFromContext extracts the tenant ID from the request context.
// For DM handlers, we need the tenant from the auth token or context.
func requireTenantFromContext(w http.ResponseWriter, r *http.Request) (pgtype.UUID, bool) {
	user, ok := requireUserFromContext(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return pgtype.UUID{}, false
	}
	return user.TenantID, true
}

// ListConversations handles GET /api/v1/dm/conversations
func (h *Handlers) ListConversations(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUserFromContext(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	tenantID, ok := requireTenantFromContext(w, r)
	if !ok {
		return
	}

	limit := parsePageSize(r)
	offset := parsePageOffset(r)

	conversations, err := h.dmService.GetConversations(r.Context(), tenantID, user.UserID, limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to get conversations")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": conversations})
}

// ListMessages handles GET /api/v1/dm/conversations/{id}/messages
func (h *Handlers) ListMessages(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUserFromContext(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	tenantID, ok := requireTenantFromContext(w, r)
	if !ok {
		return
	}

	conversationIDStr := chi.URLParam(r, "id")
	var conversationID pgtype.UUID
	if err := conversationID.Scan(conversationIDStr); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_conversation_id", "Invalid conversation ID format")
		return
	}

	limit := parsePageSize(r)
	offset := parsePageOffset(r)

	messages, err := h.dmService.GetMessages(r.Context(), tenantID, conversationID, user.UserID, limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to get messages")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": messages})
}

// SendMessage handles POST /api/v1/dm/conversations/{id}/messages
func (h *Handlers) SendMessage(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUserFromContext(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	tenantID, ok := requireTenantFromContext(w, r)
	if !ok {
		return
	}

	conversationIDStr := chi.URLParam(r, "id")
	var conversationID pgtype.UUID
	if err := conversationID.Scan(conversationIDStr); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_conversation_id", "Invalid conversation ID format")
		return
	}

	var req struct {
		Body string `json:"body"`
	}
	if err := parseJSONBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if req.Body == "" {
		writeError(w, http.StatusBadRequest, "empty_body", "Message body cannot be empty")
		return
	}

	if err := h.dmService.SendMessage(r.Context(), tenantID, conversationID, user.UserID, req.Body); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to send message")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

// MarkAsRead handles POST /api/v1/dm/conversations/{id}/read
func (h *Handlers) MarkAsRead(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUserFromContext(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	conversationIDStr := chi.URLParam(r, "id")
	var conversationID pgtype.UUID
	if err := conversationID.Scan(conversationIDStr); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_conversation_id", "Invalid conversation ID format")
		return
	}

	if err := h.dmService.MarkAsRead(r.Context(), conversationID, user.UserID); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to mark messages as read")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

// AcceptMessageRequest handles POST /api/v1/dm/message-requests/{id}/accept
func (h *Handlers) AcceptMessageRequest(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUserFromContext(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	tenantID, ok := requireTenantFromContext(w, r)
	if !ok {
		return
	}

	conversationIDStr := chi.URLParam(r, "id")
	var conversationID pgtype.UUID
	if err := conversationID.Scan(conversationIDStr); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_conversation_id", "Invalid conversation ID format")
		return
	}

	if err := h.dmService.HandleMessageRequest(r.Context(), tenantID, conversationID, user.UserID, MessageRequestAccepted); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to accept message request")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

// DeclineMessageRequest handles POST /api/v1/dm/message-requests/{id}/decline
func (h *Handlers) DeclineMessageRequest(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUserFromContext(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	tenantID, ok := requireTenantFromContext(w, r)
	if !ok {
		return
	}

	conversationIDStr := chi.URLParam(r, "id")
	var conversationID pgtype.UUID
	if err := conversationID.Scan(conversationIDStr); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_conversation_id", "Invalid conversation ID format")
		return
	}

	if err := h.dmService.HandleMessageRequest(r.Context(), tenantID, conversationID, user.UserID, MessageRequestDeclined); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to decline message request")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

// UpdateDMPrivacy handles PUT /api/v1/dm/privacy
func (h *Handlers) UpdateDMPrivacy(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUserFromContext(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	var req struct {
		Privacy string `json:"privacy"`
	}
	if err := parseJSONBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if err := h.dmService.UpdateDMPrivacy(r.Context(), user.UserID, req.Privacy); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_privacy", "Invalid privacy setting")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}
