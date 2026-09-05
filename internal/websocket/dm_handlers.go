package websocket

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/clientip"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/spam"
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

	// #nosec G109 // value bounded by MaxPageSize check
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

	// #nosec G109 // value bounded by MaxPageSize check
	// #nosec G115 // value bounded by caller
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
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return pgtype.UUID{}, false
	}

	return user.TenantID, true
}

// ListConversations handles GET /api/v1/dm/conversations.
func (h *Handlers) ListConversations(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUserFromContext(r)
	if !ok {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
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
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to get conversations")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": conversations})
}

// InitiateConversation handles POST /api/v1/dm/conversations.
//
// Creates or retrieves a conversation with the specified recipient and
// sends the initial message. Checks blocks and DM privacy settings per
// Section 48. Body: { recipient_id, message }.
func (h *Handlers) InitiateConversation(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUserFromContext(r)
	if !ok {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	tenantID, ok := requireTenantFromContext(w, r)
	if !ok {
		return
	}

	var req struct {
		RecipientID string `json:"recipient_id"`
		Message     string `json:"message"`
	}

	if err := parseJSONBody(r, &req); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if req.RecipientID == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_recipient_id", "recipient_id is required")
		return
	}

	if req.Message == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_message", "message is required")
		return
	}

	var recipientID pgtype.UUID
	if err := recipientID.Scan(req.RecipientID); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_recipient_id", "Invalid recipient ID format")
		return
	}

	result, err := h.dmService.GetOrCreateConversationForInitiator(r.Context(), tenantID, user.UserID, recipientID, req.Message)
	if err != nil {
		// Map DM privacy/block sentinel errors to 403 per Section 48.
		switch {
		case errors.Is(err, ErrDMBlocked):
			errpkg.WriteError(w, http.StatusForbidden, "dm_blocked", "Cannot send DMs to this user")
			return
		case errors.Is(err, ErrDMPrivacyNobody):
			errpkg.WriteError(w, http.StatusForbidden, "dm_privacy_blocked", "Recipient does not accept new DM conversations")
			return
		case errors.Is(err, ErrDMNotMutualFollower):
			errpkg.WriteError(w, http.StatusForbidden, "dm_not_mutual_follower", "Recipient only accepts DMs from mutual followers")
			return
		}

		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to initiate conversation")

		return
	}

	// Send the initial message (BUG-003: previously the handler created a
	// conversation but never sent the first message).
	if err := h.dmService.SendMessage(r.Context(), tenantID, result.Conversation.ID, user.UserID, req.Message); err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to send initial message")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
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
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	tenantID, ok := requireTenantFromContext(w, r)
	if !ok {
		return
	}

	conversationIDStr := chi.URLParam(r, "id")

	var conversationID pgtype.UUID

	if err := conversationID.Scan(conversationIDStr); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_conversation_id", "Invalid conversation ID format")
		return
	}

	limit := parsePageSize(r)
	offset := parsePageOffset(r)

	messages, err := h.dmService.GetMessages(r.Context(), tenantID, conversationID, user.UserID, limit, offset)
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to get messages")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": messages})
}

// SendMessage handles POST /api/v1/dm/conversations/{id}/messages.
func (h *Handlers) SendMessage(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUserFromContext(r)
	if !ok {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	tenantID, ok := requireTenantFromContext(w, r)
	if !ok {
		return
	}

	conversationIDStr := chi.URLParam(r, "id")

	var conversationID pgtype.UUID

	if err := conversationID.Scan(conversationIDStr); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_conversation_id", "Invalid conversation ID format")
		return
	}

	var req struct {
		Body string `json:"body"`
	}

	if err := parseJSONBody(r, &req); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if req.Body == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "empty_body", "Message body cannot be empty")
		return
	}

	// Spam pipeline check — multi-layer content safety (BE-034).
	if h.spamPipeline != nil {
		clientIP := clientip.FromRequest(r)

		spamResult, err := h.spamPipeline.Check(r.Context(), tenantID, spam.SpamCheckRequest{
			Content:    req.Body,
			UserID:     user.UserID,
			IP:         clientIP,
			UserType:   "existing",
			EntityType: "message",
		})
		if err != nil {
			slog.Warn("spam check failed", "error", err)
		} else if spamResult.Blocked {
			errpkg.WriteError(w, http.StatusForbidden, "message_blocked", "Message blocked by safety filters")
			return
		}
	}

	if err := h.dmService.SendMessage(r.Context(), tenantID, conversationID, user.UserID, req.Body); err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to send message")
		return
	}

	// Record the creation for the spam rate-limit layer (BUG-004). Without
	// this the jc:spam:rate:{tenant}:{user} sorted set is never populated and
	// the rate_limit scoring layer always returns 0.
	if h.spamPipeline != nil {
		h.spamPipeline.RecordContentCreation(r.Context(), tenantID, user.UserID)
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "ok"}})
}

// MarkAsRead handles POST /api/v1/dm/conversations/{id}/read.
func (h *Handlers) MarkAsRead(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUserFromContext(r)
	if !ok {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	tenantID, ok := requireTenantFromContext(w, r)
	if !ok {
		return
	}

	conversationIDStr := chi.URLParam(r, "id")

	var conversationID pgtype.UUID

	if err := conversationID.Scan(conversationIDStr); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_conversation_id", "Invalid conversation ID format")
		return
	}

	if err := h.dmService.MarkAsRead(r.Context(), tenantID, conversationID, user.UserID); err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to mark messages as read")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "ok"}})
}

// AcceptMessageRequest handles POST /api/v1/dm/requests/{id}/accept.
func (h *Handlers) AcceptMessageRequest(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUserFromContext(r)
	if !ok {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	tenantID, ok := requireTenantFromContext(w, r)
	if !ok {
		return
	}

	conversationIDStr := chi.URLParam(r, "id")

	var conversationID pgtype.UUID

	if err := conversationID.Scan(conversationIDStr); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_conversation_id", "Invalid conversation ID format")
		return
	}

	if err := h.dmService.HandleMessageRequest(r.Context(), tenantID, conversationID, user.UserID, MessageRequestAccepted); err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to accept message request")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "ok"}})
}

// DeclineMessageRequest handles POST /api/v1/dm/requests/{id}/decline.
func (h *Handlers) DeclineMessageRequest(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUserFromContext(r)
	if !ok {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	tenantID, ok := requireTenantFromContext(w, r)
	if !ok {
		return
	}

	conversationIDStr := chi.URLParam(r, "id")

	var conversationID pgtype.UUID

	if err := conversationID.Scan(conversationIDStr); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_conversation_id", "Invalid conversation ID format")
		return
	}

	if err := h.dmService.HandleMessageRequest(r.Context(), tenantID, conversationID, user.UserID, MessageRequestDeclined); err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to decline message request")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "ok"}})
}

// BlockMessageRequest handles POST /api/v1/dm/requests/{id}/block.
func (h *Handlers) BlockMessageRequest(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUserFromContext(r)
	if !ok {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	tenantID, ok := requireTenantFromContext(w, r)
	if !ok {
		return
	}

	conversationIDStr := chi.URLParam(r, "id")

	var conversationID pgtype.UUID

	if err := conversationID.Scan(conversationIDStr); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_conversation_id", "Invalid conversation ID format")
		return
	}

	if err := h.dmService.HandleMessageRequest(r.Context(), tenantID, conversationID, user.UserID, MessageRequestBlocked); err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to block message request")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "ok"}})
}

// GetDMPrivacy handles GET /api/v1/dm/privacy.
// Returns the current user's DM privacy setting.
func (h *Handlers) GetDMPrivacy(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUserFromContext(r)
	if !ok {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	tenantID, ok := requireTenantFromContext(w, r)
	if !ok {
		return
	}

	privacy, err := h.dmService.GetDMPrivacy(r.Context(), tenantID, user.UserID)
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to get DM privacy")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": DMPrivacyResult{Privacy: privacy}})
}

// UpdateDMPrivacy handles PUT /api/v1/dm/privacy.
func (h *Handlers) UpdateDMPrivacy(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUserFromContext(r)
	if !ok {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	tenantID, ok := requireTenantFromContext(w, r)
	if !ok {
		return
	}

	var req struct {
		Privacy string `json:"privacy"`
	}

	if err := parseJSONBody(r, &req); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if err := h.dmService.UpdateDMPrivacy(r.Context(), tenantID, user.UserID, req.Privacy); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_privacy", "Invalid privacy setting")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "ok"}})
}

// EditMessage handles PATCH /api/v1/dm/conversations/{id}/messages/{mid}.
//
// Edits a message within the 15-minute window. Only the sender can edit.
func (h *Handlers) EditMessage(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUserFromContext(r)
	if !ok {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	tenantID, ok := requireTenantFromContext(w, r)
	if !ok {
		return
	}

	conversationIDStr := chi.URLParam(r, "id")

	var conversationID pgtype.UUID

	if err := conversationID.Scan(conversationIDStr); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_conversation_id", "Invalid conversation ID format")
		return
	}

	messageIDStr := chi.URLParam(r, "mid")

	var messageID pgtype.UUID

	if err := messageID.Scan(messageIDStr); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_message_id", "Invalid message ID format")
		return
	}

	var req struct {
		Body string `json:"body"`
	}

	if err := parseJSONBody(r, &req); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if req.Body == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "empty_body", "Message body cannot be empty")
		return
	}

	if err := h.dmService.EditMessage(r.Context(), tenantID, conversationID, messageID, user.UserID, req.Body); err != nil {
		if errors.Is(err, ErrDMNotSender) {
			errpkg.WriteError(w, http.StatusForbidden, "not_sender", "Only the sender can edit this message")
			return
		}

		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to edit message")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "ok"}})
}

// DeleteMessage handles DELETE /api/v1/dm/conversations/{id}/messages/{mid}.
//
// Soft-deletes a message. Only the sender can delete.
func (h *Handlers) DeleteMessage(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUserFromContext(r)
	if !ok {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	tenantID, ok := requireTenantFromContext(w, r)
	if !ok {
		return
	}

	conversationIDStr := chi.URLParam(r, "id")

	var conversationID pgtype.UUID

	if err := conversationID.Scan(conversationIDStr); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_conversation_id", "Invalid conversation ID format")
		return
	}

	messageIDStr := chi.URLParam(r, "mid")

	var messageID pgtype.UUID

	if err := messageID.Scan(messageIDStr); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_message_id", "Invalid message ID format")
		return
	}

	if err := h.dmService.DeleteMessage(r.Context(), tenantID, conversationID, messageID, user.UserID); err != nil {
		if errors.Is(err, ErrDMNotSender) {
			errpkg.WriteError(w, http.StatusForbidden, "not_sender", "Only the sender can delete this message")
			return
		}

		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to delete message")

		return
	}

	errpkg.WriteJSON(w, http.StatusNoContent, nil)
}

// ─── P5.6 DM Requests List ───────────────────────────────────────────────

// ListMessageRequests handles GET /dm/requests (P5.6).
// Lists pending DM requests for the current user, filtering by recipient_id
// and status = 'pending' so only actionable requests are returned (BUG-003).
func (h *Handlers) ListMessageRequests(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUserFromContext(r)
	if !ok {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	tenantID, ok := requireTenantFromContext(w, r)
	if !ok {
		return
	}

	limit := parsePageSize(r)
	offset := parsePageOffset(r)

	requests, err := h.dmService.GetPendingMessageRequests(r.Context(), tenantID, user.UserID, limit, offset)
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to list message requests")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": requests})
}
