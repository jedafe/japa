package websocket

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/valkey"
	"github.com/jedafe/japa/internal/worker"
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

// CreateWSTicket inserts a new WebSocket ticket with 60-second TTL and
// returns the generated ticket row so the caller can return the ID to the
// client and cache it in Valkey under jc:ws_ticket:{id}.
func (s *Store) CreateWSTicket(ctx context.Context, userID pgtype.UUID) (gen.CreateWSTicketRow, error) {
	return s.write.CreateWSTicket(ctx,
		userID,
	)
}

// UseWSTicket marks a ticket as used and returns it if valid. The
// is validated alongside the user_id so a ticket minted in platform A cannot be
// consumed in platform B context (Rule 4: platform-scoped validation).
func (s *Store) UseWSTicket(ctx context.Context, id, userID pgtype.UUID) (gen.WsTicket, error) {
	return s.write.UseWSTicket(ctx, gen.UseWSTicketParams{
		ID:     id,
		UserID: userID,
	})
}

// GetWSTicketForRedemption resolves a ticket by ID when the Valkey cache
// misses, returning the user_id and needed to upgrade the WebSocket
// connection.
func (s *Store) GetWSTicketForRedemption(ctx context.Context, id pgtype.UUID) (gen.WsTicket, error) {
	return s.read.GetWSTicketForRedemption(ctx, id)
}

// GetOrCreateConversation gets existing or creates new DM conversation.
func (s *DMStore) GetOrCreateConversation(ctx context.Context, participant1ID, participant2ID pgtype.UUID) (gen.DmConversation, error) {
	return s.write.GetOrCreateConversation(ctx, gen.GetOrCreateConversationParams{

		Participant1ID: participant1ID,
		Participant2ID: participant2ID,
	})
}

// GetConversationByParticipants returns an existing conversation between two
// users without creating one. Returns pgx.ErrNoRows when no conversation
// exists. Used by the DM privacy check (Section 48) to skip privacy
// enforcement for already-active threads.
func (s *DMStore) GetConversationByParticipants(ctx context.Context, userA, userB pgtype.UUID) (gen.DmConversation, error) {
	return s.read.GetConversationByParticipants(ctx, gen.GetConversationByParticipantsParams{

		Participant1ID: userA,
		Participant2ID: userB,
	})
}

// GetConversationByID returns a conversation by ID and platform.
func (s *DMStore) GetConversationByID(ctx context.Context, id pgtype.UUID) (gen.DmConversation, error) {
	return s.read.GetConversationByID(ctx,
		id,
	)
}

// GetUserConversations returns paginated conversations for a user.
func (s *DMStore) GetUserConversations(ctx context.Context, userID pgtype.UUID, limit, offset int32) ([]gen.GetUserConversationsRow, error) {
	if limit <= 0 || limit > MaxPageSize {
		limit = DefaultPageSize
	}

	if offset < 0 {
		offset = 0
	}

	return s.read.GetUserConversations(ctx, gen.GetUserConversationsParams{
		UserID: userID,

		PageLimit:  limit,
		PageOffset: offset,
	})
}

// InsertDMMessage inserts a new message into a conversation.
func (s *DMStore) InsertDMMessage(ctx context.Context, conversationID, senderID pgtype.UUID, body string) error {
	return s.write.InsertDMMessage(ctx, gen.InsertDMMessageParams{
		ConversationID: conversationID,
		SenderID:       senderID,

		Body: body,
	})
}

// GetConversationMessages returns paginated messages for a conversation.
func (s *DMStore) GetConversationMessages(ctx context.Context, conversationID pgtype.UUID, limit, offset int32) ([]gen.GetConversationMessagesRow, error) {
	if limit <= 0 || limit > MaxPageSize {
		limit = DefaultPageSize
	}

	if offset < 0 {
		offset = 0
	}

	return s.read.GetConversationMessages(ctx, gen.GetConversationMessagesParams{

		ConversationID: conversationID,
		PageLimit:      limit,
		PageOffset:     offset,
	})
}

// MarkMessagesRead marks all unread messages from other participants as read.
func (s *DMStore) MarkMessagesRead(ctx context.Context, conversationID, userID pgtype.UUID) error {
	return s.write.MarkMessagesRead(ctx, gen.MarkMessagesReadParams{
		ConversationID: conversationID,

		UserID: userID,
	})
}

// UpdateConversationLastMessage updates the last message metadata. The
// timestamp is set to now() in SQL (not passed from Go) so it is always
// the actual current time. The scopes the update (Rule 4).
func (s *DMStore) UpdateConversationLastMessage(ctx context.Context, conversationID pgtype.UUID, preview *string) error {
	return s.write.UpdateConversationLastMessage(ctx, gen.UpdateConversationLastMessageParams{
		ConversationID: conversationID,

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

// GetMessageRequest returns a message request for a conversation, scoped
// to the platform (Rule 4).
func (s *DMStore) GetMessageRequest(ctx context.Context, conversationID pgtype.UUID) (gen.DmRequest, error) {
	return s.read.GetMessageRequest(ctx,
		conversationID,
	)
}

// CreateMessageRequest creates a pending message request with the actual
// requester and recipient IDs (not derived from conversation participant
// ordering). Platform-scoped (Rule 4).
func (s *DMStore) CreateMessageRequest(ctx context.Context, requesterID, recipientID, conversationID pgtype.UUID, previewMessage string) error {
	return s.write.CreateMessageRequest(ctx, gen.CreateMessageRequestParams{

		RequesterID:    requesterID,
		RecipientID:    recipientID,
		ConversationID: conversationID,
		PreviewMessage: previewMessage,
	})
}

// UpdateMessageRequestStatus updates the status of a message request,
// scoped to the platform (Rule 4).
func (s *DMStore) UpdateMessageRequestStatus(ctx context.Context, conversationID pgtype.UUID, status string) error {
	return s.write.UpdateMessageRequestStatus(ctx, gen.UpdateMessageRequestStatusParams{
		ConversationID: conversationID,

		Status: status,
	})
}

// GetPendingMessageRequests returns pending DM requests for a recipient.
func (s *DMStore) GetPendingMessageRequests(ctx context.Context, recipientID pgtype.UUID, limit, offset int32) ([]gen.GetPendingMessageRequestsRow, error) {
	if limit <= 0 || limit > MaxPageSize {
		limit = DefaultPageSize
	}

	if offset < 0 {
		offset = 0
	}

	return s.read.GetPendingMessageRequests(ctx, gen.GetPendingMessageRequestsParams{

		RecipientID: recipientID,
		PageLimit:   limit,
		PageOffset:  offset,
	})
}

// GetUserDMPrivacy returns the DM privacy setting for a user.
func (s *DMStore) GetUserDMPrivacy(ctx context.Context, userID pgtype.UUID) (string, error) {
	return s.read.GetUserDMPrivacy(ctx,
		userID,
	)
}

// IsBlocked returns true if either user has blocked the other (bidirectional
// check per Section 48). Wraps the sqlc-generated IsBlocked query.
// Platform-scoped per JEOS §9 (BUG-006).
func (s *DMStore) IsBlocked(ctx context.Context, userA, userB pgtype.UUID) (bool, error) {
	return s.read.IsBlocked(ctx, gen.IsBlockedParams{
		BlockerID: userA,
		BlockedID: userB,
	})
}

// IsMutualFollow returns true if initiator follows recipient AND recipient
// follows initiator. The sqlc IsMutualFollow query checks a single direction,
// so we call it twice and AND the results.
func (s *DMStore) IsMutualFollow(ctx context.Context, initiatorID, recipientID pgtype.UUID) (bool, error) {
	initiatorFollowsRecipient, err := s.read.IsMutualFollow(ctx, gen.IsMutualFollowParams{

		EntityID:   recipientID,
		FollowerID: initiatorID,
	})
	if err != nil {
		return false, fmt.Errorf("checking initiator->recipient follow: %w", err)
	}

	if !initiatorFollowsRecipient {
		return false, nil
	}

	recipientFollowsInitiator, err := s.read.IsMutualFollow(ctx, gen.IsMutualFollowParams{

		EntityID:   initiatorID,
		FollowerID: recipientID,
	})
	if err != nil {
		return false, fmt.Errorf("checking recipient->initiator follow: %w", err)
	}

	return recipientFollowsInitiator, nil
}

// UpsertUserDMPrivacy creates or updates the DM privacy setting.
func (s *DMStore) UpsertUserDMPrivacy(ctx context.Context, userID pgtype.UUID, privacy string) error {
	return s.write.UpsertUserDMPrivacy(ctx, gen.UpsertUserDMPrivacyParams{
		UserID: userID,

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

// GetDMMessageByID returns a single DM message by ID and conversation.
// Returns pgx.ErrNoRows when the message does not exist. Used by the
// edit/delete service paths to verify sender ownership before mutating
// (Rule 5: return 403 not 500 for a non-sender).
func (s *DMStore) GetDMMessageByID(ctx context.Context, messageID, conversationID pgtype.UUID) (gen.DmMessage, error) {
	return s.read.GetDMMessageByID(ctx, gen.GetDMMessageByIDParams{
		ID:             messageID,
		ConversationID: conversationID,
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
	store       *DMStore
	hub         *Hub
	AsynqClient *asynq.Client
	vkClient    *valkey.Client
}

// NewDMService creates a DMService with the given store and hub.
func NewDMService(store *DMStore, hub *Hub) *DMService {
	return &DMService{store: store, hub: hub}
}

// SetValkeyClient injects the Valkey client used to maintain the
// jc:dm_total_unread:{user_id} badge counter (Section 48). Without this,
// the DB unread columns are still correct (source of truth) but the
// Valkey-cached badge total is never updated.
func (s *DMService) SetValkeyClient(vk *valkey.Client) {
	s.vkClient = vk
}

// DM privacy / block sentinel errors per Section 48.
var (
	ErrDMBlocked           = errors.New("dm blocked by user_blocks")
	ErrDMPrivacyNobody     = errors.New("recipient does not accept new DM conversations")
	ErrDMNotMutualFollower = errors.New("recipient only accepts DMs from mutual followers")
	ErrDMNotSender         = errors.New("user is not the sender of this message")
)

// GetOrCreateConversation gets existing or creates new DM conversation.
// Enforces the recipient's dm_privacy setting and the bidirectional block
// check per Section 48 before creating a new conversation. Existing
// conversations are returned unchanged (privacy settings do not retroactively
// close already-active threads). Non-mutual-follow senders land as a message
// request (pending state) instead of a direct conversation (Section 48).
// The previewMessage is stored as dm_requests.preview_message for
// non-mutual-follow senders so the recipient can see what the requester
// wants to say (Section 48: "first message text; shown in the request
// preview").
func (s *DMService) GetOrCreateConversation(ctx context.Context, senderID, recipientID pgtype.UUID, previewMessage string) (ConversationResult, error) {
	// Ensure sender != recipient
	if senderID == recipientID {
		return ConversationResult{}, errors.New("cannot create conversation with yourself")
	}

	// If a conversation already exists, return it — privacy settings do not
	// retroactively close already-active threads (Section 48: "existing ones
	// still active"). The block check still applies: a later block must
	// prevent further interaction even on existing conversations.
	if existing, err := s.store.GetConversationByParticipants(ctx, senderID, recipientID); err == nil {
		// Block check (Section 48): either direction blocks → 403, even
		// for existing conversations.
		blocked, bErr := s.store.IsBlocked(ctx, senderID, recipientID)
		if bErr != nil {
			return ConversationResult{}, fmt.Errorf("checking block status: %w", bErr)
		}

		if blocked {
			return ConversationResult{}, ErrDMBlocked
		}

		return ConversationResult{Conversation: existing, IsNew: false}, nil
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return ConversationResult{}, fmt.Errorf("checking existing conversation: %w", err)
	}

	// No existing conversation — apply privacy + block checks for new ones.
	// Block check (Section 48): either direction blocks → 403.
	blocked, err := s.store.IsBlocked(ctx, senderID, recipientID)
	if err != nil {
		return ConversationResult{}, fmt.Errorf("checking block status: %w", err)
	}

	if blocked {
		return ConversationResult{}, ErrDMBlocked
	}

	// Privacy check (Section 48) — only applies to NEW conversations.
	privacy, err := s.store.GetUserDMPrivacy(ctx, recipientID)
	if err != nil {
		return ConversationResult{}, fmt.Errorf("reading recipient dm_privacy: %w", err)
	}

	isMutualFollow := false

	switch privacy {
	case "nobody":
		return ConversationResult{}, ErrDMPrivacyNobody
	case "followers_only":
		mutualFollow, mErr := s.store.IsMutualFollow(ctx, senderID, recipientID)
		if mErr != nil {
			return ConversationResult{}, fmt.Errorf("checking mutual follow: %w", mErr)
		}

		if !mutualFollow {
			return ConversationResult{}, ErrDMNotMutualFollower
		}

		isMutualFollow = true
	case "everyone", "":
		// Allowed; check mutual follow to decide direct vs request.
		mutualFollow, mErr := s.store.IsMutualFollow(ctx, senderID, recipientID)
		if mErr != nil {
			return ConversationResult{}, fmt.Errorf("checking mutual follow: %w", mErr)
		}

		isMutualFollow = mutualFollow
	default:
		// Unknown privacy value — fail closed (treat as nobody).
		slog.Warn("dm: unknown dm_privacy value, failing closed", "recipient_id", recipientID.String(), "dm_privacy", privacy)
		return ConversationResult{}, ErrDMPrivacyNobody
	}

	conv, err := s.store.GetOrCreateConversation(ctx, senderID, recipientID)
	if err != nil {
		return ConversationResult{}, fmt.Errorf("getting or creating conversation: %w", err)
	}

	// Non-mutual-follow → create a message request (Section 48). The
	// conversation exists but the recipient must accept the request before
	// it joins their main inbox. The requester's first message is the
	// preview. (BUG-002: previously skipped, creating direct conversations
	// for all non-mutual-follow senders.)
	if !isMutualFollow {
		if err := s.store.CreateMessageRequest(ctx, senderID, recipientID, conv.ID, previewMessage); err != nil {
			slog.Warn("dm: failed to create message request", "conversation_id", conv.ID.String(), "error", err)
		}
		// Enqueue push notification for the recipient (fire-and-forget).
		s.enqueueDMNotifyRequest(ctx, recipientID, senderID, conv.ID)
	}

	return ConversationResult{Conversation: conv, IsNew: true}, nil
}

// SendMessage sends a message in a conversation.
func (s *DMService) SendMessage(ctx context.Context, conversationID, senderID pgtype.UUID, body string) error {
	if body == "" {
		return errors.New("message body cannot be empty")
	}

	// Verify the sender is a participant in this conversation.
	conv, err := s.store.GetConversationByID(ctx, conversationID)
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

	// Maintain the Valkey badge counter: INCR jc:dm_total_unread:{recipient}
	// (Section 48). The DB column is the source of truth; this cache
	// avoids a DB query on every page load for the unread badge.
	s.incrTotalUnread(ctx, recipientID)

	// Truncate preview to 100 characters (runes, not bytes — slicing by
	// bytes splits multi-byte UTF-8 and corrupts non-ASCII previews on a
	// multi-language platform, §6.2).
	preview := truncateRunes(body, 100)

	previewPtr := &preview

	// Update conversation last message. The timestamp is set to now() in
	// SQL (BUG-001: previously passed a zero-time pgtype.Timestamptz).
	if err := s.store.UpdateConversationLastMessage(ctx, conversationID, previewPtr); err != nil {
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

	// Enqueue push notification for recipient — fire-and-forget.
	s.enqueueDMNotifyMessage(ctx, recipientID, senderID, conversationID, preview)

	return nil
}

// GetConversations returns user's conversations with unread counts.
func (s *DMService) GetConversations(ctx context.Context, userID pgtype.UUID, limit, offset int32) ([]ConversationInfo, error) {
	rows, err := s.store.GetUserConversations(ctx, userID, limit, offset)
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
func (s *DMService) GetMessages(ctx context.Context, conversationID, userID pgtype.UUID, limit, offset int32) ([]MessageInfo, error) {
	// Verify the user is a participant.
	conv, err := s.store.GetConversationByID(ctx, conversationID)
	if err != nil {
		return nil, fmt.Errorf("verifying conversation access: %w", err)
	}

	if conv.Participant1ID != userID && conv.Participant2ID != userID {
		return nil, errors.New("user is not a participant in this conversation")
	}

	rows, err := s.store.GetConversationMessages(ctx, conversationID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("getting conversation messages: %w", err)
	}

	msgs := make([]MessageInfo, len(rows))

	for i, r := range rows {
		// SenderDisplayName is a *string from sqlc. Dereference safely so
		// the API returns the actual name, not a pointer address (the
		// previous fmt.Sprintf("%v", *string) printed "0xc0000123a0").
		var displayName string
		if r.SenderDisplayName != nil {
			displayName = *r.SenderDisplayName
		}

		msgs[i] = MessageInfo{
			ID:                r.ID.String(),
			SenderID:          r.SenderID.String(),
			SenderUsername:    r.SenderUsername,
			SenderDisplayName: displayName,
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

// MarkAsRead marks messages as read and resets unread count. The ID
// scopes the update so a cross-platform conversation ID cannot be modified
// (BUG-009, Rule 4: platform-scoped). After resetting the DB column, the
// Valkey badge counter jc:dm_total_unread:{user_id} is recalculated from
// the sum of all conversation unread counts (Section 48).
func (s *DMService) MarkAsRead(ctx context.Context, conversationID, userID pgtype.UUID) error {
	if err := s.store.MarkMessagesRead(ctx, conversationID, userID); err != nil {
		return fmt.Errorf("marking messages read: %w", err)
	}

	if err := s.store.ResetUnreadCount(ctx, conversationID, userID); err != nil {
		return fmt.Errorf("resetting unread count: %w", err)
	}

	// Recalculate and re-set the Valkey badge counter (Section 48).
	s.recalcTotalUnread(ctx, userID)

	return nil
}

// HandleMessageRequest accepts, declines, or blocks a message request.
// Only the RECIPIENT of the request may take action — the requester cannot
// accept their own outgoing request (BUG-005).
func (s *DMService) HandleMessageRequest(ctx context.Context, conversationID, userID pgtype.UUID, action string) error {
	// Look up the message request, scoped to the platform (Rule 4).
	dmReq, err := s.store.GetMessageRequest(ctx, conversationID)
	if err != nil {
		return fmt.Errorf("looking up message request: %w", err)
	}

	// Only the recipient may accept/decline/block — not the requester.
	if dmReq.RecipientID != userID {
		return errors.New("only the recipient can act on a message request")
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

// MessageRequestInfo is the API response for a pending DM request item.
type MessageRequestInfo struct {
	ID                   string  `json:"id"`
	ConversationID       *string `json:"conversation_id,omitempty"`
	RequesterID          string  `json:"requester_id"`
	RequesterUsername    string  `json:"requester_username"`
	RequesterDisplayName string  `json:"requester_display_name"`
	RequesterAvatarURL   *string `json:"requester_avatar_url,omitempty"`
	PreviewMessage       string  `json:"preview_message"`
	Status               string  `json:"status"`
	CreatedAt            string  `json:"created_at"`
}

// GetPendingMessageRequests returns pending DM requests for a recipient
// (BUG-003). Filters by recipient_id and status = 'pending' so only
// actionable requests are returned. Wraps the raw sqlc rows in an API
// response struct (BUG-014: previously leaked internal gen struct names).
func (s *DMService) GetPendingMessageRequests(ctx context.Context, recipientID pgtype.UUID, limit, offset int32) ([]MessageRequestInfo, error) {
	rows, err := s.store.GetPendingMessageRequests(ctx, recipientID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("getting pending message requests: %w", err)
	}

	infos := make([]MessageRequestInfo, len(rows))
	for i, r := range rows {
		infos[i] = MessageRequestInfo{
			ID:                r.ID.String(),
			RequesterID:       r.RequesterID.String(),
			RequesterUsername: r.RequesterUsername,
			PreviewMessage:    r.PreviewMessage,
			Status:            r.Status,
			CreatedAt:         r.CreatedAt.Time.String(),
		}
		if r.RequesterDisplayName != nil {
			infos[i].RequesterDisplayName = *r.RequesterDisplayName
		}

		if r.ConversationID.Valid {
			s := r.ConversationID.String()
			infos[i].ConversationID = &s
		}

		if r.RequesterAvatarUrl != nil {
			infos[i].RequesterAvatarURL = r.RequesterAvatarUrl
		}
	}

	return infos, nil
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

// EditMessage edits a message within the 15-minute window. Only the
// sender of the message may edit it; a participant who is not the sender
// gets ErrDMNotSender so the handler can return 403 instead of 500
// (Rule 5: structured error envelope with correct status code).
func (s *DMService) EditMessage(ctx context.Context, conversationID, messageID, senderID pgtype.UUID, body string) error {
	if body == "" {
		return errors.New("message body cannot be empty")
	}

	conv, err := s.store.GetConversationByID(ctx, conversationID)
	if err != nil {
		return fmt.Errorf("verifying conversation access: %w", err)
	}

	if conv.Participant1ID != senderID && conv.Participant2ID != senderID {
		return errors.New("user is not a participant in this conversation")
	}

	// Verify the user is the sender of this specific message. The
	// EditDMMessage :exec query does not return ErrNoRows on a sender
	// mismatch (it silently affects 0 rows), so we must check ownership
	// explicitly to return a 403 instead of a 500 (Rule 5).
	msg, err := s.store.GetDMMessageByID(ctx, messageID, conversationID)
	if err != nil {
		return fmt.Errorf("looking up message: %w", err)
	}

	if msg.SenderID != senderID {
		return ErrDMNotSender
	}

	if err := s.store.EditDMMessage(ctx, messageID, conversationID, senderID, body); err != nil {
		return fmt.Errorf("editing dm message: %w", err)
	}

	return nil
}

// DeleteMessage soft-deletes a message owned by the sender. Only the
// sender may delete; a non-sender participant gets ErrDMNotSender so the
// handler returns 403, not 500 (Rule 5).
func (s *DMService) DeleteMessage(ctx context.Context, conversationID, messageID, senderID pgtype.UUID) error {
	conv, err := s.store.GetConversationByID(ctx, conversationID)
	if err != nil {
		return fmt.Errorf("verifying conversation access: %w", err)
	}

	if conv.Participant1ID != senderID && conv.Participant2ID != senderID {
		return errors.New("user is not a participant in this conversation")
	}

	// Verify sender ownership explicitly — the SoftDeleteDMMessage :exec
	// query silently affects 0 rows on a sender mismatch (Rule 5).
	msg, err := s.store.GetDMMessageByID(ctx, messageID, conversationID)
	if err != nil {
		return fmt.Errorf("looking up message: %w", err)
	}

	if msg.SenderID != senderID {
		return ErrDMNotSender
	}

	if err := s.store.SoftDeleteDMMessage(ctx, messageID, conversationID, senderID); err != nil {
		return fmt.Errorf("deleting dm message: %w", err)
	}

	return nil
}

// GetOrCreateConversationForInitiator gets or creates a conversation,
// checking blocks and privacy settings per Section 48. The previewMessage
// is stored as the dm_requests.preview_message when a message request is
// created for a non-mutual-follow sender (Section 48 spec).
func (s *DMService) GetOrCreateConversationForInitiator(ctx context.Context, initiatorID, recipientID pgtype.UUID, previewMessage string) (ConversationResult, error) {
	return s.GetOrCreateConversation(ctx, initiatorID, recipientID, previewMessage)
}

// GetOtherParticipant returns the ID of the other participant in a
// conversation, scoped to the platform. Returns an error if the conversation
// doesn't exist or the given user is not a participant. Used by the typing
// indicator path (Section 10) to route typing events to the correct
// recipient without exposing conversation membership to non-participants.
func (s *DMService) GetOtherParticipant(ctx context.Context, conversationID, userID pgtype.UUID) (pgtype.UUID, error) {
	conv, err := s.store.GetConversationByID(ctx, conversationID)
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("looking up conversation: %w", err)
	}

	if conv.Participant1ID != userID && conv.Participant2ID != userID {
		return pgtype.UUID{}, errors.New("user is not a participant in this conversation")
	}

	if conv.Participant1ID == userID {
		return conv.Participant2ID, nil
	}

	return conv.Participant1ID, nil
}

// enqueueDMNotifyMessage enqueues a push notification for a new DM message.
func (s *DMService) enqueueDMNotifyMessage(ctx context.Context, recipientID, senderID, conversationID pgtype.UUID, bodyPreview string) {
	if s.AsynqClient == nil {
		return
	}

	// Look up sender username for notification title.
	var senderUsername string
	if row, err := s.store.read.GetUserByID(ctx,

		senderID,
	); err == nil {
		senderUsername = row.Username
	}

	payload, _ := json.Marshal(map[string]string{
		"recipient_id": recipientID.String(),

		"sender_username": senderUsername,
		"conversation_id": conversationID.String(),
		"body_preview":    bodyPreview,
	})

	if _, err := s.AsynqClient.Enqueue(
		asynq.NewTask(worker.TypeDMNotifyMessage, payload),
		asynq.Queue(worker.QueueForTask(worker.TypeDMNotifyMessage)),
	); err != nil {
		slog.Warn("dm: failed to enqueue notify_message", "conversation_id", conversationID.String(), "error", err)
	}
}

// enqueueDMNotifyRequest enqueues a push notification for a new DM request.
func (s *DMService) enqueueDMNotifyRequest(ctx context.Context, recipientID, senderID, conversationID pgtype.UUID) {
	if s.AsynqClient == nil {
		return
	}

	// Look up sender username for notification title.
	var senderUsername string
	if row, err := s.store.read.GetUserByID(ctx,

		senderID,
	); err == nil {
		senderUsername = row.Username
	}

	payload, _ := json.Marshal(map[string]string{
		"recipient_id": recipientID.String(),

		"sender_username": senderUsername,
		"conversation_id": conversationID.String(),
	})

	if _, err := s.AsynqClient.Enqueue(
		asynq.NewTask(worker.TypeDMNotifyRequest, payload),
		asynq.Queue(worker.QueueForTask(worker.TypeDMNotifyRequest)),
	); err != nil {
		slog.Warn("dm: failed to enqueue notify_request", "conversation_id", conversationID.String(), "error", err)
	}
}

// truncateRunes shortens s to at most n runes, preserving valid UTF-8.
// Unlike byte-slicing (s[:n]), this never splits a multi-byte character,
// which is required on a multi-language platform (en, fr, non-ASCII).
func truncateRunes(s string, n int) string {
	if n <= 0 {
		return ""
	}

	r := []rune(s)
	if len(r) <= n {
		return s
	}

	return string(r[:n])
}

// incrTotalUnread increments the Valkey badge counter
// jc:dm_total_unread:{user_id} by 1 (Section 48). Failures are logged and
// swallowed — the DB column is the source of truth and the next
// recalculation (on MarkAsRead) will correct any drift.
func (s *DMService) incrTotalUnread(ctx context.Context, userID pgtype.UUID) {
	if s.vkClient == nil {
		return
	}

	vctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	key := valkey.MustKey("dm_total_unread:%s", userID.String())
	if err := s.vkClient.Redis().Incr(vctx, key).Err(); err != nil {
		slog.Warn("dm: failed to incr total unread", "user_id", userID.String(), "error", err)
	}
}

// recalcTotalUnread recalculates the total unread DM count for a user by
// summing all per-conversation unread counts from the DB, then sets the
// Valkey badge counter jc:dm_total_unread:{user_id} with a 24h TTL
// (Section 48). Fetches up to MaxPageSize conversations; for users with
// more, the badge may be slightly off until the next recalculation —
// acceptable for a cache backed by the DB source of truth.
func (s *DMService) recalcTotalUnread(ctx context.Context, userID pgtype.UUID) {
	if s.vkClient == nil {
		return
	}

	convs, err := s.store.GetUserConversations(ctx, userID, MaxPageSize, 0)
	if err != nil {
		slog.Warn("dm: failed to fetch conversations for unread recalc", "error", err)
		return
	}

	var total int64
	for _, c := range convs {
		total += int64(c.UnreadCount)
	}

	vctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	key := valkey.MustKey("dm_total_unread:%s", userID.String())
	if err := s.vkClient.Redis().Set(vctx, key, total, 24*time.Hour).Err(); err != nil {
		slog.Warn("dm: failed to set total unread", "user_id", userID.String(), "error", err)
	}
}
