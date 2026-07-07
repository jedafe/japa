package websocket

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
)

const defaultPageSize int32 = 20

// parseJSONBody reads the request body and decodes it into v.
func parseJSONBody(r *http.Request, v any) error {
	return json.NewDecoder(r.Body).Decode(v)
}

// parsePageSize extracts and clamps the page_size query parameter.
func parsePageSize(r *http.Request) int32 {
	s := r.URL.Query().Get("page_size")
	if s == "" {
		return defaultPageSize
	}

	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 {
		return defaultPageSize
	}

	if n > MaxPageSize {
		return MaxPageSize
	}

	return int32(n) //nolint:gosec // G109: bounded by MaxPageSize check above
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

	return int32(n) //nolint:gosec // G109: bounded by MaxPageSize check above
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

// ListConversations handles GET /api/v1/dm/conversations.
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

// InitiateConversation handles POST /api/v1/dm/conversations.
//
// Creates or retrieves a conversation with the specified recipient.
// Checks blocks and DM privacy settings per Section 48.
func (h *Handlers) InitiateConversation(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUserFromContext(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	tenantID, ok := requireTenantFromContext(w, r)
	if !ok {
		return
	}

	var req struct {
		RecipientID string `json:"recipient_id"`
	}

	if err := parseJSONBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if req.RecipientID == "" {
		writeError(w, http.StatusBadRequest, "missing_recipient_id", "recipient_id is required")
		return
	}

	var recipientID pgtype.UUID
	if err := recipientID.Scan(req.RecipientID); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_recipient_id", "Invalid recipient ID format")
		return
	}

	result, err := h.dmService.GetOrCreateConversationForInitiator(r.Context(), tenantID, user.UserID, recipientID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to initiate conversation")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"conversation_id": result.Conversation.ID.String(),
			"is_new":          result.IsNew,
		},
	})
}

// ListMessages handles GET /api/v1/dm/conversations/{id}/messages.
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

// SendMessage handles POST /api/v1/dm/conversations/{id}/messages.
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

// MarkAsRead handles POST /api/v1/dm/conversations/{id}/read.
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

// AcceptMessageRequest handles POST /api/v1/dm/message-requests/{id}/accept.
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

// DeclineMessageRequest handles POST /api/v1/dm/message-requests/{id}/decline.
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

// UpdateDMPrivacy handles PUT /api/v1/dm/privacy.
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

// EditMessage handles PATCH /api/v1/dm/conversations/{id}/messages/{mid}.
//
// Edits a message within the 15-minute window. Only the sender can edit.
func (h *Handlers) EditMessage(w http.ResponseWriter, r *http.Request) {
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

	messageIDStr := chi.URLParam(r, "mid")
	var messageID pgtype.UUID
	if err := messageID.Scan(messageIDStr); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_message_id", "Invalid message ID format")
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

	if err := h.dmService.EditMessage(r.Context(), tenantID, conversationID, messageID, user.UserID, req.Body); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to edit message")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

// DeleteMessage handles DELETE /api/v1/dm/conversations/{id}/messages/{mid}.
//
// Soft-deletes a message. Only the sender can delete.
func (h *Handlers) DeleteMessage(w http.ResponseWriter, r *http.Request) {
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

	messageIDStr := chi.URLParam(r, "mid")
	var messageID pgtype.UUID
	if err := messageID.Scan(messageIDStr); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_message_id", "Invalid message ID format")
		return
	}

	if err := h.dmService.DeleteMessage(r.Context(), tenantID, conversationID, messageID, user.UserID); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to delete message")
		return
	}

	writeJSON(w, http.StatusNoContent, nil)
}
