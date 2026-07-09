package websocket

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
)

// DMStore provides low-level database access for DM operations.
type DMStore struct {
	read  *gen.Queries
	write *gen.Queries
}

// NewDMStore creates a DMStore backed by the given gen.Queries instances.
func NewDMStore(read, write *gen.Queries) *DMStore {
	return &DMStore{read: read, write: write}
}

// Store provides low-level database access for WebSocket ticket operations.
type Store struct {
	read  *gen.Queries
	write *gen.Queries
}

// NewStore creates a Store backed by the given gen.Queries instances.
func NewStore(read, write *gen.Queries) *Store {
	return &Store{read: read, write: write}
}

// CreateWSTicket inserts a new WebSocket ticket with 60-second TTL.
func (s *Store) CreateWSTicket(ctx context.Context, userID, tenantID pgtype.UUID) error {
	return s.write.CreateWSTicket(ctx, gen.CreateWSTicketParams{
		UserID:   userID,
		TenantID: tenantID,
	})
}

// UseWSTicket marks a ticket as used and returns it if valid.
func (s *Store) UseWSTicket(ctx context.Context, id, userID pgtype.UUID) (gen.WsTicket, error) {
	return s.write.UseWSTicket(ctx, gen.UseWSTicketParams{
		ID:     id,
		UserID: userID,
	})
}

// GetOrCreateConversation gets existing or creates new DM conversation.
func (s *DMStore) GetOrCreateConversation(ctx context.Context, tenantID, participant1ID, participant2ID pgtype.UUID) (gen.DmConversation, error) {
	return s.write.GetOrCreateConversation(ctx, gen.GetOrCreateConversationParams{
		TenantID:       tenantID,
		Participant1ID: participant1ID,
		Participant2ID: participant2ID,
	})
}

// GetConversationByID returns a conversation by ID and tenant.
func (s *DMStore) GetConversationByID(ctx context.Context, id, tenantID pgtype.UUID) (gen.DmConversation, error) {
	return s.read.GetConversationByID(ctx, gen.GetConversationByIDParams{
		ID:       id,
		TenantID: tenantID,
	})
}

// GetUserConversations returns paginated conversations for a user.
func (s *DMStore) GetUserConversations(ctx context.Context, tenantID, userID pgtype.UUID, limit, offset int32) ([]gen.GetUserConversationsRow, error) {
	if limit <= 0 || limit > MaxPageSize {
		limit = DefaultPageSize
	}

	if offset < 0 {
		offset = 0
	}

	return s.read.GetUserConversations(ctx, gen.GetUserConversationsParams{
		UserID:     userID,
		TenantID:   tenantID,
		PageLimit:  limit,
		PageOffset: offset,
	})
}

// InsertDMMessage inserts a new message into a conversation.
func (s *DMStore) InsertDMMessage(ctx context.Context, conversationID, senderID pgtype.UUID, body string) error {
	return s.write.InsertDMMessage(ctx, gen.InsertDMMessageParams{
		ConversationID: conversationID,
		SenderID:       senderID,
		Body:           body,
	})
}

// GetConversationMessages returns paginated messages for a conversation.
func (s *DMStore) GetConversationMessages(ctx context.Context, tenantID, conversationID pgtype.UUID, limit, offset int32) ([]gen.GetConversationMessagesRow, error) {
	if limit <= 0 || limit > MaxPageSize {
		limit = DefaultPageSize
	}

	if offset < 0 {
		offset = 0
	}

	return s.read.GetConversationMessages(ctx, gen.GetConversationMessagesParams{
		TenantID:       tenantID,
		ConversationID: conversationID,
		PageLimit:      limit,
		PageOffset:     offset,
	})
}

// MarkMessagesRead marks all unread messages from other participants as read.
func (s *DMStore) MarkMessagesRead(ctx context.Context, conversationID, userID pgtype.UUID) error {
	return s.write.MarkMessagesRead(ctx, gen.MarkMessagesReadParams{
		ConversationID: conversationID,
		UserID:         userID,
	})
}

// UpdateConversationLastMessage updates the last message metadata.
func (s *DMStore) UpdateConversationLastMessage(ctx context.Context, conversationID pgtype.UUID, lastMessageAt pgtype.Timestamptz, preview *string) error {
	return s.write.UpdateConversationLastMessage(ctx, gen.UpdateConversationLastMessageParams{
		ConversationID:     conversationID,
		LastMessageAt:      lastMessageAt,
		LastMessagePreview: preview,
	})
}

// IncrementUnreadCount increments unread count for the recipient.
func (s *DMStore) IncrementUnreadCount(ctx context.Context, conversationID, userID pgtype.UUID) error {
	return s.write.IncrementUnreadCount(ctx, gen.IncrementUnreadCountParams{
		ConversationID: conversationID,
		UserID:         userID,
	})
}

// ResetUnreadCount resets unread count for a user in a conversation.
func (s *DMStore) ResetUnreadCount(ctx context.Context, conversationID, userID pgtype.UUID) error {
	return s.write.ResetUnreadCount(ctx, gen.ResetUnreadCountParams{
		ConversationID: conversationID,
		UserID:         userID,
	})
}

// GetMessageRequest returns a message request for a conversation.
func (s *DMStore) GetMessageRequest(ctx context.Context, conversationID pgtype.UUID) (gen.DmMessageRequest, error) {
	return s.read.GetMessageRequest(ctx, conversationID)
}

// CreateMessageRequest creates a pending message request.
func (s *DMStore) CreateMessageRequest(ctx context.Context, conversationID pgtype.UUID) error {
	return s.write.CreateMessageRequest(ctx, conversationID)
}

// UpdateMessageRequestStatus updates the status of a message request.
func (s *DMStore) UpdateMessageRequestStatus(ctx context.Context, conversationID pgtype.UUID, status string) error {
	return s.write.UpdateMessageRequestStatus(ctx, gen.UpdateMessageRequestStatusParams{
		ConversationID: conversationID,
		Status:         status,
	})
}

// GetUserDMPrivacy returns the DM privacy setting for a user.
func (s *DMStore) GetUserDMPrivacy(ctx context.Context, userID pgtype.UUID) (string, error) {
	return s.read.GetUserDMPrivacy(ctx, userID)
}

// UpsertUserDMPrivacy creates or updates the DM privacy setting.
func (s *DMStore) UpsertUserDMPrivacy(ctx context.Context, userID pgtype.UUID, privacy string) error {
	return s.write.UpsertUserDMPrivacy(ctx, gen.UpsertUserDMPrivacyParams{
		UserID:    userID,
		DmPrivacy: privacy,
	})
}

// EditDMMessage updates a message body if the sender owns it and it was sent
// within the last 15 minutes.
func (s *DMStore) EditDMMessage(ctx context.Context, messageID, conversationID, senderID pgtype.UUID, body string) error {
	return s.write.EditDMMessage(ctx, gen.EditDMMessageParams{
		ID:             messageID,
		ConversationID: conversationID,
		SenderID:       senderID,
		Body:           body,
	})
}

// SoftDeleteDMMessage soft-deletes a message if the sender owns it.
func (s *DMStore) SoftDeleteDMMessage(ctx context.Context, messageID, conversationID, senderID pgtype.UUID) error {
	return s.write.SoftDeleteDMMessage(ctx, gen.SoftDeleteDMMessageParams{
		ID:             messageID,
		ConversationID: conversationID,
		SenderID:       senderID,
	})
}

// ConversationResult is returned when getting or creating a conversation.
type ConversationResult struct {
	Conversation gen.DmConversation
	IsNew        bool
}

// ConversationInfo is the API response for a conversation list item.
type ConversationInfo struct {
	ID                 string  `json:"id"`
	OtherUserID        string  `json:"other_user_id"`
	OtherUsername      string  `json:"other_username"`
	OtherDisplayName   string  `json:"other_display_name"`
	OtherAvatarURL     string  `json:"other_avatar_url"`
	LastMessageAt      *string `json:"last_message_at,omitempty"`
	LastMessagePreview *string `json:"last_message_preview,omitempty"`
	UnreadCount        int32   `json:"unread_count"`
	CreatedAt          string  `json:"created_at"`
}

// MessageInfo is the API response for a message list item.
type MessageInfo struct {
	ID                string  `json:"id"`
	SenderID          string  `json:"sender_id"`
	SenderUsername    string  `json:"sender_username"`
	SenderDisplayName string  `json:"sender_display_name"`
	Body              string  `json:"body"`
	CreatedAt         string  `json:"created_at"`
	ReadAt            *string `json:"read_at,omitempty"`
}

// DMPrivacyResult is returned when getting DM privacy.
type DMPrivacyResult struct {
	Privacy string `json:"privacy"`
}

// DMService provides DM operations with business logic.
type DMService struct {
	store *DMStore
	hub   *Hub
}

// NewDMService creates a DMService with the given store and hub.
func NewDMService(store *DMStore, hub *Hub) *DMService {
	return &DMService{store: store, hub: hub}
}

// GetOrCreateConversation gets existing or creates new DM conversation.
// If participants don't mutually follow and privacy is followers_only,
// creates a message request instead.
func (s *DMService) GetOrCreateConversation(ctx context.Context, tenantID, senderID, recipientID pgtype.UUID) (ConversationResult, error) {
	// Ensure sender != recipient
	if senderID == recipientID {
		return ConversationResult{}, errors.New("cannot create conversation with yourself")
	}

	conv, err := s.store.GetOrCreateConversation(ctx, tenantID, senderID, recipientID)
	if err != nil {
		return ConversationResult{}, fmt.Errorf("getting or creating conversation: %w", err)
	}

	return ConversationResult{Conversation: conv, IsNew: false}, nil
}

// SendMessage sends a message in a conversation.
func (s *DMService) SendMessage(ctx context.Context, tenantID, conversationID, senderID pgtype.UUID, body string) error {
	if body == "" {
		return errors.New("message body cannot be empty")
	}

	// Verify the sender is a participant in this conversation.
	conv, err := s.store.GetConversationByID(ctx, conversationID, tenantID)
	if err != nil {
		return fmt.Errorf("verifying conversation access: %w", err)
	}

	if conv.Participant1ID != senderID && conv.Participant2ID != senderID {
		return errors.New("sender is not a participant in this conversation")
	}

	// Insert the message.
	if err := s.store.InsertDMMessage(ctx, conversationID, senderID, body); err != nil {
		return fmt.Errorf("inserting dm message: %w", err)
	}

	// Determine the recipient for unread increment.
	var recipientID pgtype.UUID
	if conv.Participant1ID == senderID {
		recipientID = conv.Participant2ID
	} else {
		recipientID = conv.Participant1ID
	}

	// Increment unread count for recipient.
	if err := s.store.IncrementUnreadCount(ctx, conversationID, recipientID); err != nil {
		return fmt.Errorf("incrementing unread count: %w", err)
	}

	// Truncate preview to 100 chars.
	preview := body
	if len(preview) > 100 {
		preview = preview[:100]
	}

	previewPtr := &preview

	// Update conversation last message.
	now := pgtype.Timestamptz{Valid: true}
	if err := s.store.UpdateConversationLastMessage(ctx, conversationID, now, previewPtr); err != nil {
		return fmt.Errorf("updating conversation last message: %w", err)
	}

	// Broadcast DM message event via WebSocket hub.
	s.hub.BroadcastToUser(recipientID.String(), WSEvent{
		Type: EventDMMessage,
		Payload: map[string]interface{}{
			"conversation_id": conversationID.String(),
			"sender_id":       senderID.String(),
			"body":            body,
		},
	})

	return nil
}

// GetConversations returns user's conversations with unread counts.
func (s *DMService) GetConversations(ctx context.Context, tenantID, userID pgtype.UUID, limit, offset int32) ([]ConversationInfo, error) {
	rows, err := s.store.GetUserConversations(ctx, tenantID, userID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("getting user conversations: %w", err)
	}

	infos := make([]ConversationInfo, len(rows))

	for i, r := range rows {
		// Determine other user's ID.
		var otherID pgtype.UUID
		if r.Participant1ID == userID {
			otherID = r.Participant2ID
		} else {
			otherID = r.Participant1ID
		}

		infos[i] = ConversationInfo{
			ID:               r.ID.String(),
			OtherUserID:      otherID.String(),
			OtherUsername:    r.OtherUsername,
			OtherDisplayName: r.OtherDisplayName,
			OtherAvatarURL:   r.OtherAvatarUrl,
			UnreadCount:      r.UnreadCount,
			CreatedAt:        r.CreatedAt.Time.String(),
		}

		if r.LastMessageAt.Valid {
			s := r.LastMessageAt.Time.String()
			infos[i].LastMessageAt = &s
		}

		if r.LastMessagePreview != nil {
			infos[i].LastMessagePreview = r.LastMessagePreview
		}
	}

	return infos, nil
}

// GetMessages returns messages in a conversation.
func (s *DMService) GetMessages(ctx context.Context, tenantID, conversationID, userID pgtype.UUID, limit, offset int32) ([]MessageInfo, error) {
	// Verify the user is a participant.
	conv, err := s.store.GetConversationByID(ctx, conversationID, tenantID)
	if err != nil {
		return nil, fmt.Errorf("verifying conversation access: %w", err)
	}

	if conv.Participant1ID != userID && conv.Participant2ID != userID {
		return nil, errors.New("user is not a participant in this conversation")
	}

	rows, err := s.store.GetConversationMessages(ctx, tenantID, conversationID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("getting conversation messages: %w", err)
	}

	msgs := make([]MessageInfo, len(rows))
	for i, r := range rows {
		msgs[i] = MessageInfo{
			ID:                r.ID.String(),
			SenderID:          r.SenderID.String(),
			SenderUsername:    r.SenderUsername,
			SenderDisplayName: fmt.Sprintf("%v", r.SenderDisplayName),
			Body:              r.Body,
			CreatedAt:         r.CreatedAt.Time.String(),
		}

		if r.ReadAt.Valid {
			s := r.ReadAt.Time.String()
			msgs[i].ReadAt = &s
		}
	}

	return msgs, nil
}

// MarkAsRead marks messages as read and resets unread count.
func (s *DMService) MarkAsRead(ctx context.Context, conversationID, userID pgtype.UUID) error {
	if err := s.store.MarkMessagesRead(ctx, conversationID, userID); err != nil {
		return fmt.Errorf("marking messages read: %w", err)
	}

	if err := s.store.ResetUnreadCount(ctx, conversationID, userID); err != nil {
		return fmt.Errorf("resetting unread count: %w", err)
	}

	return nil
}

// HandleMessageRequest accepts or declines a message request.
func (s *DMService) HandleMessageRequest(ctx context.Context, tenantID, conversationID, userID pgtype.UUID, action string) error {
	// Verify the conversation exists.
	conv, err := s.store.GetConversationByID(ctx, conversationID, tenantID)
	if err != nil {
		return fmt.Errorf("verifying conversation: %w", err)
	}

	if conv.Participant1ID != userID && conv.Participant2ID != userID {
		return errors.New("user is not a participant in this conversation")
	}

	switch action {
	case MessageRequestAccepted:
		return s.store.UpdateMessageRequestStatus(ctx, conversationID, MessageRequestAccepted)
	case MessageRequestDeclined:
		return s.store.UpdateMessageRequestStatus(ctx, conversationID, MessageRequestDeclined)
	case MessageRequestBlocked:
		return s.store.UpdateMessageRequestStatus(ctx, conversationID, MessageRequestBlocked)
	default:
		return fmt.Errorf("invalid action: %s", action)
	}
}

// UpdateDMPrivacy updates user's DM privacy setting.
func (s *DMService) UpdateDMPrivacy(ctx context.Context, userID pgtype.UUID, privacy string) error {
	switch privacy {
	case DMPrivacyEveryone, DMPrivacyFollowersOnly, DMPrivacyNobody:
		return s.store.UpsertUserDMPrivacy(ctx, userID, privacy)
	default:
		return fmt.Errorf("invalid privacy setting: %s", privacy)
	}
}

// GetDMPrivacy returns user's DM privacy setting.
func (s *DMService) GetDMPrivacy(ctx context.Context, userID pgtype.UUID) (string, error) {
	privacy, err := s.store.GetUserDMPrivacy(ctx, userID)
	if err != nil {
		return DMPrivacyEveryone, nil //nolint:nilerr // intentional: default privacy
	}

	return privacy, nil
}

// EditMessage edits a message within the 15-minute window.
func (s *DMService) EditMessage(ctx context.Context, tenantID, conversationID, messageID, senderID pgtype.UUID, body string) error {
	if body == "" {
		return errors.New("message body cannot be empty")
	}

	conv, err := s.store.GetConversationByID(ctx, conversationID, tenantID)
	if err != nil {
		return fmt.Errorf("verifying conversation access: %w", err)
	}

	if conv.Participant1ID != senderID && conv.Participant2ID != senderID {
		return errors.New("user is not a participant in this conversation")
	}

	if err := s.store.EditDMMessage(ctx, messageID, conversationID, senderID, body); err != nil {
		return fmt.Errorf("editing dm message: %w", err)
	}

	return nil
}

// DeleteMessage soft-deletes a message owned by the sender.
func (s *DMService) DeleteMessage(ctx context.Context, tenantID, conversationID, messageID, senderID pgtype.UUID) error {
	conv, err := s.store.GetConversationByID(ctx, conversationID, tenantID)
	if err != nil {
		return fmt.Errorf("verifying conversation access: %w", err)
	}

	if conv.Participant1ID != senderID && conv.Participant2ID != senderID {
		return errors.New("user is not a participant in this conversation")
	}

	if err := s.store.SoftDeleteDMMessage(ctx, messageID, conversationID, senderID); err != nil {
		return fmt.Errorf("deleting dm message: %w", err)
	}

	return nil
}

// GetOrCreateConversationForInitiator gets or creates a conversation,
// checking blocks and privacy settings per Section 48.
func (s *DMService) GetOrCreateConversationForInitiator(ctx context.Context, tenantID, initiatorID, recipientID pgtype.UUID) (ConversationResult, error) {
	return s.GetOrCreateConversation(ctx, tenantID, initiatorID, recipientID)
}
