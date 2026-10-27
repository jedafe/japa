package websocket

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/literals"
	"github.com/jedafe/japa/internal/valkey"
	"github.com/jedafe/japa/internal/worker"
)

// DMStore provides low-level database access for DM operations.
type DMStore struct {
	read  *gen.Queries
	write *gen.Queries
}

// dmCursor is the decoded keyset position carried in the ?cursor= param of
// the DM list endpoints (Section 48). Which key field is populated depends
// on the active sort: T for timestamp sorts (recent/oldest), N for the
// unread_count sort, K for name sorts. ID is the row-id tiebreaker.
type dmCursor struct {
	S  string `json:"s"`
	T  int64  `json:"t,omitempty"`
	N  int64  `json:"n,omitempty"`
	K  string `json:"k,omitempty"`
	ID string `json:"id"`
}

func encodeDMCursor(c dmCursor) string {
	b, err := json.Marshal(c)
	if err != nil {
		return ""
	}

	return base64.RawURLEncoding.EncodeToString(b)
}

func decodeDMCursor(raw string) *dmCursor {
	if raw == "" {
		return nil
	}

	b, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return nil
	}

	var c dmCursor
	if err := json.Unmarshal(b, &c); err != nil || c.ID == "" {
		return nil
	}

	return &c
}

// dmCursorArgs converts a decoded cursor into the typed keyset params the
// queries expect. A cursor whose embedded sort doesn't match the requested
// sort is ignored — resuming across sorts would silently skip rows.
func dmCursorArgs(sort string, c *dmCursor) (has bool, ts pgtype.Timestamptz, i int64, txt string, id pgtype.UUID) {
	if c == nil || (c.S != "" && c.S != sort) {
		return false, ts, 0, "", id
	}

	if err := id.Scan(c.ID); err != nil {
		return false, ts, 0, "", id
	}

	if c.T != 0 {
		ts = pgtype.Timestamptz{Time: time.Unix(0, c.T).UTC(), Valid: true}
	}

	return true, ts, c.N, c.K, id
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

// GetUserConversations returns paginated conversations for a user. The
// optional q (ILIKE on username/display_name/last preview), filter
// ('unread'), and sort ('recent'|'oldest'|'unread'|'name') params mirror the
// query-string contract of GET /dm/conversations; empty values are no-ops.
// When a keyset cursor is present it takes precedence over page_offset.
func (s *DMStore) GetUserConversations(ctx context.Context, userID pgtype.UUID, q, filter, sort string, limit, offset int32, cursor *dmCursor) ([]gen.GetUserConversationsRow, error) {
	if limit <= 0 || limit > MaxPageSize {
		limit = DefaultPageSize
	}

	if offset < 0 {
		offset = 0
	}

	// An empty sort must resolve to the 'recent' arm explicitly — the
	// ORDER BY / keyset arms match on the literal 'recent' string.
	if sort == "" {
		sort = "recent"
	}

	has, ts, ci, ct, cid := dmCursorArgs(sort, cursor)
	if has {
		// Keyset and offset semantics never mix — the cursor wins.
		offset = 0
	}

	return s.read.GetUserConversations(ctx, gen.GetUserConversationsParams{
		UserID: userID,
		Q:      q,
		Filter: filter,
		Sort:   sort,

		HasCursor:  has,
		CursorTs:   ts,
		CursorInt:  ci,
		CursorText: ct,
		CursorID:   cid,

		PageLimit:  limit,
		PageOffset: offset,
	})
}

// InsertDMMessage inserts a new message into a conversation. mc carries
// the optional structured context (000941) — nil for plain DMs.
func (s *DMStore) InsertDMMessage(ctx context.Context, conversationID, senderID pgtype.UUID, body string, mc *DMMessageContext, asOrg bool) error {
	p := gen.InsertDMMessageParams{
		ConversationID: conversationID,
		SenderID:       senderID,
		Body:           body,
		SentAsOrg:      asOrg,
	}
	if mc != nil && mc.Type != "" {
		p.ContextType = &mc.Type
		p.ContextID = mc.ID
		p.ContextLabel = &mc.Label
		p.ContextSlug = &mc.Slug
	}
	return s.write.InsertDMMessage(ctx, p)
}

// GetServiceListingContext resolves a service_listing DM context to its
// published-listing snapshot (000941) — pgx.ErrNoRows for any other
// entity id (non-listing, unpublished, or deleted).
func (s *DMStore) GetServiceListingContext(ctx context.Context, id pgtype.UUID) (gen.GetServiceListingContextRow, error) {
	return s.read.GetServiceListingContext(ctx, id)
}

// GetOrCreateOrgConversation returns the single (user, org) conversation
// (000942) — the org side is every member, not a participant row.
func (s *DMStore) GetOrCreateOrgConversation(ctx context.Context, senderID, orgID pgtype.UUID) (gen.DmConversation, error) {
	return s.write.GetOrCreateOrgConversation(ctx, gen.GetOrCreateOrgConversationParams{
		Participant1ID: senderID,
		OrgID:          orgID,
	})
}

// IsOrgMember reports whether userID is a member of orgID (000942).
func (s *DMStore) IsOrgMember(ctx context.Context, orgID, userID pgtype.UUID) (bool, error) {
	members, err := s.read.ListOrgMembers(ctx, orgID)
	if err != nil {
		return false, err
	}
	for _, m := range members {
		if m == userID {
			return true, nil
		}
	}
	return false, nil
}

// ListOrgMembers returns the org's member user ids (broadcast fan-out).
func (s *DMStore) ListOrgMembers(ctx context.Context, orgID pgtype.UUID) ([]pgtype.UUID, error) {
	return s.read.ListOrgMembers(ctx, orgID)
}

// GetOrgIDForListingContext is the lenient variant of
// GetServiceListingContext (000942) — resolves a listing's org regardless
// of publish status so an existing org conversation survives staleness.
func (s *DMStore) GetOrgIDForListingContext(ctx context.Context, id pgtype.UUID) (pgtype.UUID, error) {
	return s.read.GetOrgIDForListingContext(ctx, id)
}

// GetOrgConversationByParticipants returns the (user, org) conversation
// if it exists — used for the stale-context degrade path (000942).
func (s *DMStore) GetOrgConversationByParticipants(ctx context.Context, senderID, orgID pgtype.UUID) (gen.DmConversation, error) {
	return s.read.GetOrgConversationByParticipants(ctx, gen.GetOrgConversationByParticipantsParams{
		Participant1ID: senderID,
		OrgID:          orgID,
	})
}

// GetSenderDisplayName resolves a sender's display name for live-event
// attribution (000942).
func (s *DMStore) GetSenderDisplayName(ctx context.Context, senderID pgtype.UUID) (string, error) {
	return s.read.GetSenderDisplayName(ctx, senderID)
}

// UpsertDMOrgReadState marks an org conversation read for one member.
func (s *DMStore) UpsertDMOrgReadState(ctx context.Context, conversationID, memberID pgtype.UUID) error {
	return s.write.UpsertDMOrgReadState(ctx, gen.UpsertDMOrgReadStateParams{
		ConversationID: conversationID,
		MemberUserID:   memberID,
	})
}

// GetConversationMessages returns paginated messages for a conversation.
// Keyset-paginated by (created_at, id) DESC — when a cursor is present it
// takes precedence over page_offset (Section 48).
func (s *DMStore) GetConversationMessages(ctx context.Context, conversationID pgtype.UUID, limit, offset int32, cursor *dmCursor) ([]gen.GetConversationMessagesRow, error) {
	if limit <= 0 || limit > MaxPageSize {
		limit = DefaultPageSize
	}

	if offset < 0 {
		offset = 0
	}

	has, ts, _, _, cid := dmCursorArgs("", cursor)
	if has {
		offset = 0
	}

	return s.read.GetConversationMessages(ctx, gen.GetConversationMessagesParams{
		ConversationID: conversationID,
		HasCursor:      has,
		CursorTs:       ts,
		CursorID:       cid,
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

// GetMessageRequest returns a message request by its dm_requests.id (the id
// carried by the /dm/requests/{id} routes and the list DTO).
func (s *DMStore) GetMessageRequest(ctx context.Context, requestID pgtype.UUID) (gen.DmRequest, error) {
	return s.read.GetMessageRequest(ctx,
		requestID,
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

// UpdateMessageRequestStatus updates the status of a message request by its
// dm_requests.id.
func (s *DMStore) UpdateMessageRequestStatus(ctx context.Context, requestID pgtype.UUID, status string) error {
	return s.write.UpdateMessageRequestStatus(ctx, gen.UpdateMessageRequestStatusParams{
		ID: requestID,

		Status: status,
	})
}

// GetPendingMessageRequests returns pending DM requests for a recipient.
// Optional q (ILIKE on requester username/display_name/preview) and sort
// ('recent'|'oldest'|'name') params mirror the GET /dm/requests contract.
func (s *DMStore) GetPendingMessageRequests(ctx context.Context, recipientID pgtype.UUID, q, sort string, limit, offset int32, cursor *dmCursor) ([]gen.GetPendingMessageRequestsRow, error) {
	if limit <= 0 || limit > MaxPageSize {
		limit = DefaultPageSize
	}

	if offset < 0 {
		offset = 0
	}

	if sort == "" {
		sort = "recent"
	}

	has, ts, _, ct, cid := dmCursorArgs(sort, cursor)
	if has {
		offset = 0
	}

	return s.read.GetPendingMessageRequests(ctx, gen.GetPendingMessageRequestsParams{
		RecipientID: recipientID,
		Q:           q,
		Sort:        sort,
		HasCursor:   has,
		CursorTs:    ts,
		CursorText:  ct,
		CursorID:    cid,
		PageLimit:   limit,
		PageOffset:  offset,
	})
}

// CreateUserBlock inserts a user_blocks row (blocker → blocked), used by the
// DM request "block" action per Section 48 ("decline + add sender to
// user_blocks"). ON CONFLICT DO NOTHING makes repeat calls idempotent.
func (s *DMStore) CreateUserBlock(ctx context.Context, blockerID, blockedID pgtype.UUID) error {
	return s.write.CreateUserBlock(ctx, gen.CreateUserBlockParams{
		BlockerID: blockerID,
		BlockedID: blockedID,
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
	// 000942 — org conversations show the org as the "other" party and
	// flag the type so the UI can badge it.
	ConversationType string  `json:"conversation_type,omitempty"`
	OrgID            *string `json:"org_id,omitempty"`
	OrgSlug          *string `json:"org_slug,omitempty"`
}

// DMMessageContext is the structured context attached to a dm_messages row
// (000941) — a link to the entity the conversation is about. Type is one
// of the migration CHECK values ("service_listing"); Label/Slug are the
// send-time display snapshot.
type DMMessageContext struct {
	Type  string      `json:"type"`
	ID    pgtype.UUID `json:"id"`
	Label string      `json:"label,omitempty"`
	Slug  string      `json:"slug,omitempty"`
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
	ContextType       *string `json:"context_type,omitempty"`
	ContextID         *string `json:"context_id,omitempty"`
	ContextLabel      *string `json:"context_label,omitempty"`
	ContextSlug       *string `json:"context_slug,omitempty"`
	// 000942 — sent by an org member on behalf of the org inbox.
	SentAsOrg bool `json:"sent_as_org,omitempty"`
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
	lit         literalResolver // nil disables literal resolution (§14)
}

// NewDMService creates a DMService with the given store and hub.
func NewDMService(store *DMStore, hub *Hub) *DMService {
	return &DMService{store: store, hub: hub}
}

// SetLiteralResolver injects the template-literal catalog so text emitted
// through channels with no later read-path resolution (the live WebSocket
// broadcast, push notification previews) renders {{...}} placeholders.
func (s *DMService) SetLiteralResolver(l literalResolver) { s.lit = l }

// resolveDMText resolves {{...}} placeholders in outbound text — best-effort,
// returns the input unchanged when no resolver is wired.
func (s *DMService) resolveDMText(ctx context.Context, text string) string {
	if s.lit == nil || !strings.Contains(text, "{{") {
		return text
	}

	return s.lit.ResolveText(ctx, text, literals.NewContext(ctx, ""))
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
	ErrDMBlocked                = errors.New("dm blocked by user_blocks")
	ErrDMPrivacyNobody          = errors.New("recipient does not accept new DM conversations")
	ErrDMNotMutualFollower      = errors.New("recipient only accepts DMs from mutual followers")
	ErrDMNotSender              = errors.New("user is not the sender of this message")
	ErrDMRequestNotFound        = errors.New("message request not found")
	ErrDMRequestNotRecipient    = errors.New("only the recipient can act on a message request")
	ErrDMRequestAlreadyResolved = errors.New("message request is no longer pending")
	ErrDMSelfConversation       = errors.New("cannot create conversation with yourself")
	// 000941 — a context that isn't a published listing authored by the
	// recipient (spoofed privacy-bypass attempt or stale entity id).
	ErrDMInvalidContext = errors.New("invalid message context")
	ErrDMNotParticipant = errors.New("user is not a participant in this conversation")
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
func (s *DMService) GetOrCreateConversation(ctx context.Context, senderID, recipientID pgtype.UUID, previewMessage string, mc *DMMessageContext) (ConversationResult, error) {
	// Ensure sender != recipient
	if senderID == recipientID {
		return ConversationResult{}, ErrDMSelfConversation
	}

	// 000941 — resolve a service_listing context before the thread lookup:
	// a valid context (published listing authored by the recipient) carries
	// the dm_privacy bypass and snapshots its label/slug server-side
	// (never client-trusted). A spoofed context (listing authored by
	// someone else) hard-fails everywhere; a stale one (unpublished or
	// deleted listing) only fails on the new-conversation path — an
	// existing thread stays usable and the message sends without context.
	listingCtx := false
	staleListing := false
	orgListing := pgtype.UUID{}
	listingAuthor := pgtype.UUID{}
	if mc != nil && mc.Type == "service_listing" {
		listing, err := s.store.GetServiceListingContext(ctx, mc.ID)
		switch {
		case err != nil:
			staleListing = true
		case listing.OrgID.Valid:
			// 000942 — org-owned listing: the commercial channel is the
			// org inbox, not whoever authored it. Authorship vs the
			// recipient_id is irrelevant here (any member may have
			// authored it).
			mc.Label = listing.Title
			mc.Slug = listing.Slug
			listingCtx = true
			orgListing = listing.OrgID
			listingAuthor = listing.AuthorID
		case listing.AuthorID != recipientID:
			return ConversationResult{}, ErrDMInvalidContext
		default:
			mc.Label = listing.Title
			mc.Slug = listing.Slug
			listingCtx = true
		}
	}

	// 000942 — org listings: one (sender, org) conversation, resolved BEFORE
	// the user-pair lookup so an org contact never lands in a personal
	// thread with the listing author. No dm_privacy (orgs have no such
	// setting) and no dm_requests round-trip — the org inbox IS the
	// channel. Block check still enforced against the listing's author.
	if listingCtx && orgListing.Valid {
		blocked, bErr := s.store.IsBlocked(ctx, senderID, listingAuthor)
		if bErr != nil {
			return ConversationResult{}, fmt.Errorf("checking block status: %w", bErr)
		}
		if blocked {
			return ConversationResult{}, ErrDMBlocked
		}

		// An org member contacting their own org's listing is a
		// self-inbox — reject it like a self-DM.
		if member, mErr := s.store.IsOrgMember(ctx, orgListing, senderID); mErr == nil && member {
			return ConversationResult{}, ErrDMSelfConversation
		}

		conv, err := s.store.GetOrCreateOrgConversation(ctx, senderID, orgListing)
		if err != nil {
			return ConversationResult{}, fmt.Errorf("getting or creating org conversation: %w", err)
		}

		return ConversationResult{Conversation: conv, IsNew: true}, nil
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

		// Stale context on an existing thread: degrade to a plain DM
		// rather than failing — the listing state can't kill an
		// already-open conversation.
		if staleListing {
			*mc = DMMessageContext{}
		}

		return ConversationResult{Conversation: existing, IsNew: false}, nil
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return ConversationResult{}, fmt.Errorf("checking existing conversation: %w", err)
	}

	// Stale context: before failing, check whether it was an org listing —
	// an existing org conversation survives its listing going stale (000942)
	// and degrades to a plain message, same as the DM path above.
	if staleListing {
		orgID, oErr := s.store.GetOrgIDForListingContext(ctx, mc.ID)
		if oErr == nil && orgID.Valid {
			conv, gErr := s.store.GetOrgConversationByParticipants(ctx, senderID, orgID)
			if gErr == nil {
				*mc = DMMessageContext{}
				return ConversationResult{Conversation: conv, IsNew: false}, nil
			}
		}
		return ConversationResult{}, ErrDMInvalidContext
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

	// Privacy check (Section 48) — only applies to NEW conversations, and
	// is bypassed when a valid service-listing context rides along (000941):
	// a listing's contact surface is a commercial channel, not a cold DM —
	// the provider's social DM setting can't kill it. Blocks still apply.
	isMutualFollow := false

	if listingCtx {
		mutualFollow, mErr := s.store.IsMutualFollow(ctx, senderID, recipientID)
		if mErr != nil {
			return ConversationResult{}, fmt.Errorf("checking mutual follow: %w", mErr)
		}
		isMutualFollow = mutualFollow
	} else {
		privacy, err := s.store.GetUserDMPrivacy(ctx, recipientID)
		if err != nil {
			return ConversationResult{}, fmt.Errorf("reading recipient dm_privacy: %w", err)
		}

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
	// The request preview carries the listing's reference too — the
	// provider's requests inbox shows what the inquiry is about before
	// they accept and see the structured card (000941).
	reqPreview := previewMessage
	if listingCtx && mc.Label != "" {
		reqPreview = "Re: " + mc.Label + "\n\n" + previewMessage
	}

	if !isMutualFollow {
		if err := s.store.CreateMessageRequest(ctx, senderID, recipientID, conv.ID, reqPreview); err != nil {
			slog.Warn("dm: failed to create message request", "conversation_id", conv.ID.String(), "error", err)
		}
		// Enqueue push notification for the recipient (fire-and-forget).
		s.enqueueDMNotifyRequest(ctx, recipientID, senderID, conv.ID)
	}

	return ConversationResult{Conversation: conv, IsNew: true}, nil
}

// SendMessage sends a message in a conversation.
func (s *DMService) SendMessage(ctx context.Context, conversationID, senderID pgtype.UUID, body string) error {
	return s.SendMessageWithContext(ctx, conversationID, senderID, body, nil)
}

// SendMessageWithContext stores an optional structured context on the
// message row (000941) — the first DM of a listing-initiated conversation
// carries the listing so the thread renders a context card.
func (s *DMService) SendMessageWithContext(ctx context.Context, conversationID, senderID pgtype.UUID, body string, mc *DMMessageContext) error {
	if body == "" {
		return errors.New("message body cannot be empty")
	}

	// Verify the sender is a participant in this conversation.
	conv, err := s.store.GetConversationByID(ctx, conversationID)
	if err != nil {
		return fmt.Errorf("verifying conversation access: %w", err)
	}

	// 000942 — org conversations admit any org member as the org side;
	// their sends carry sent_as_org for thread attribution. The external
	// user (p1) is always admitted without a membership lookup.
	asOrg := false
	if conv.ConversationType == "org" && conv.Participant1ID != senderID {
		member, mErr := s.store.IsOrgMember(ctx, conv.OrgID, senderID)
		if mErr != nil {
			return fmt.Errorf("checking org membership: %w", mErr)
		}
		if !member {
			return ErrDMNotParticipant
		}
		asOrg = true
	} else if conv.ConversationType != "org" && conv.Participant1ID != senderID && conv.Participant2ID != senderID {
		return ErrDMNotParticipant
	}

	// A later block must prevent further interaction even on existing
	// conversations (Section 48) — the initiate path rechecks, but direct
	// message sends bypassed it entirely. Org conversations skip this:
	// the block was evaluated against the listing author at initiation,
	// and a member's personal block can't gate the org's reply channel.
	if conv.ConversationType != "org" {
		peer := conv.Participant1ID
		if conv.Participant1ID == senderID {
			peer = conv.Participant2ID
		}
		blocked, bErr := s.store.IsBlocked(ctx, senderID, peer)
		if bErr != nil {
			return fmt.Errorf("checking block status: %w", bErr)
		}
		if blocked {
			return ErrDMBlocked
		}
	}

	// Insert the message.
	if err := s.store.InsertDMMessage(ctx, conversationID, senderID, body, mc, asOrg); err != nil {
		return fmt.Errorf("inserting dm message: %w", err)
	}

	// 000942 — notification/unread targets by conversation shape:
	//   DM: the other participant.
	//   org: inbound (external user) → every member (their unread is
	//   computed per member from dm_org_read_state); outbound (member)
	//   → the external user + fellow members for live thread sync.
	var notifyIDs []pgtype.UUID
	switch {
	case conv.ConversationType == "org" && asOrg:
		notifyIDs = append(notifyIDs, conv.Participant1ID)
		members, mErr := s.store.ListOrgMembers(ctx, conv.OrgID)
		if mErr == nil {
			notifyIDs = append(notifyIDs, members...)
		}
	case conv.ConversationType == "org":
		members, mErr := s.store.ListOrgMembers(ctx, conv.OrgID)
		if mErr != nil {
			return fmt.Errorf("listing org members: %w", mErr)
		}
		notifyIDs = append(notifyIDs, members...)
	default:
		if conv.Participant1ID == senderID {
			notifyIDs = append(notifyIDs, conv.Participant2ID)
		} else {
			notifyIDs = append(notifyIDs, conv.Participant1ID)
		}
	}

	// Increment the external user's DB counter when an org member sends —
	// org members read unread from dm_org_read_state instead.
	if conv.ConversationType == "org" && asOrg && conv.Participant1ID.Valid {
		if err := s.store.IncrementUnreadCount(ctx, conversationID, conv.Participant1ID); err != nil {
			return fmt.Errorf("incrementing unread count: %w", err)
		}
	} else if conv.ConversationType == "dm" && len(notifyIDs) > 0 {
		if err := s.store.IncrementUnreadCount(ctx, conversationID, notifyIDs[0]); err != nil {
			return fmt.Errorf("incrementing unread count: %w", err)
		}
	}

	// Maintain the Valkey badge counter: INCR jc:dm_total_unread:{recipient}
	// (Section 48). The DB column/read-state is the source of truth; the
	// cache avoids a DB query on every page load for the unread badge.
	if conv.ConversationType == "org" {
		if asOrg {
			s.incrTotalUnread(ctx, conv.Participant1ID)
		} else {
			for _, m := range notifyIDs {
				s.incrTotalUnread(ctx, m)
			}
		}
	} else if len(notifyIDs) > 0 {
		s.incrTotalUnread(ctx, notifyIDs[0])
	}

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

	// Broadcast DM message event via WebSocket hub. §14: resolve {{...}} —
	// the live event has no later read-path resolution.
	payload := map[string]interface{}{
		"conversation_id": conversationID.String(),
		"sender_id":       senderID.String(),
		"body":            s.resolveDMText(ctx, body),
	}
	if mc != nil {
		payload["context_type"] = mc.Type
		payload["context_id"] = mc.ID.String()
		payload["context_label"] = mc.Label
		payload["context_slug"] = mc.Slug
	}
	if asOrg {
		payload["sent_as_org"] = true
		if name, nErr := s.store.GetSenderDisplayName(ctx, senderID); nErr == nil {
			payload["sender_display_name"] = name
		}
	}
	for _, uid := range notifyIDs {
		if uid == senderID {
			continue
		}
		s.hub.BroadcastToUser(uid.String(), WSEvent{
			Type:    EventDMMessage,
			Payload: payload,
		})
	}

	// Enqueue push notification for recipients — fire-and-forget. For org
	// conversations the org side is every member except the sender.
	for _, uid := range notifyIDs {
		if uid == senderID || (conv.ConversationType == "org" && asOrg && uid != conv.Participant1ID) {
			continue
		}
		s.enqueueDMNotifyMessage(ctx, uid, senderID, conversationID, preview)
	}

	return nil
}

// GetConversations returns user's conversations with unread counts plus the
// opaque next_cursor for the following page (empty when the page was not
// full — no further rows exist).
func (s *DMService) GetConversations(ctx context.Context, userID pgtype.UUID, q, filter, sort string, limit, offset int32, cursor *dmCursor) ([]ConversationInfo, string, error) {
	if sort == "" {
		sort = "recent"
	}

	rows, err := s.store.GetUserConversations(ctx, userID, q, filter, sort, limit, offset, cursor)
	if err != nil {
		return nil, "", fmt.Errorf("getting user conversations: %w", err)
	}

	infos := make([]ConversationInfo, len(rows))

	for i, r := range rows {
		// Determine other party's ID — for org conversations viewed by a
		// member, the "other" is the external user (p1); for the external
		// user it's the org (no user id).
		var otherID pgtype.UUID
		if r.Participant1ID == userID {
			otherID = r.Participant2ID
		} else {
			otherID = r.Participant1ID
		}

		var otherUserID string
		if otherID.Valid {
			otherUserID = otherID.String()
		}

		infos[i] = ConversationInfo{
			ID:               r.ID.String(),
			OtherUserID:      otherUserID,
			OtherUsername:    r.OtherUsername,
			OtherDisplayName: r.OtherDisplayName,
			OtherAvatarURL:   r.OtherAvatarUrl,
			UnreadCount:      r.UnreadCount,
			CreatedAt:        r.CreatedAt.Time.Format(time.RFC3339),
			ConversationType: r.ConversationType,
		}

		if r.ConversationType == "org" {
			if r.OrgID.Valid {
				s := r.OrgID.String()
				infos[i].OrgID = &s
			}
			if r.OrgSlug != "" {
				slug := r.OrgSlug
				infos[i].OrgSlug = &slug
			}
		}

		if r.LastMessageAt.Valid {
			s := r.LastMessageAt.Time.Format(time.RFC3339)
			infos[i].LastMessageAt = &s
		}

		if r.LastMessagePreview != nil {
			infos[i].LastMessagePreview = r.LastMessagePreview
		}
	}

	return infos, nextConversationCursor(rows, sort, limit), nil
}

// nextConversationCursor encodes the keyset resume position for
// GET /dm/conversations from the last row of the page (Section 48). The key
// mirrors the ORDER BY arm of the active sort: COALESCE(last_message_at,
// created_at) for recent/oldest, unread_count for 'unread', the coalesced
// other-name for 'name' — all tiebroken by conversation id. An empty string
// means no further page.
func nextConversationCursor(rows []gen.GetUserConversationsRow, sort string, limit int32) string {
	if len(rows) == 0 || int32(len(rows)) < effectivePageLimit(limit) { // #nosec G115 -- slice length bounded by request-scoped data
		return ""
	}

	last := rows[len(rows)-1]
	c := dmCursor{S: sort, ID: last.ID.String()}

	switch sort {
	case "unread":
		c.N = int64(last.UnreadCount)
	case "name":
		c.K = last.OtherDisplayName
	default: // recent | oldest — the COALESCE(last_message_at, created_at) key
		ts := last.CreatedAt
		if last.LastMessageAt.Valid {
			ts = last.LastMessageAt
		}

		c.T = ts.Time.UnixNano()
	}

	return encodeDMCursor(c)
}

func effectivePageLimit(limit int32) int32 {
	if limit <= 0 || limit > MaxPageSize {
		return DefaultPageSize
	}

	return limit
}

// GetMessages returns messages in a conversation (newest-first) plus the
// opaque next_cursor to resume with — the (created_at, id) keyset of the
// last row, per Section 48. Empty next_cursor means no older messages.
func (s *DMService) GetMessages(ctx context.Context, conversationID, userID pgtype.UUID, limit, offset int32, cursor *dmCursor) ([]MessageInfo, string, error) {
	// Verify the user is a participant.
	conv, err := s.store.GetConversationByID(ctx, conversationID)
	if err != nil {
		return nil, "", fmt.Errorf("verifying conversation access: %w", err)
	}

	// 000942 — org conversations: any org member can read the thread.
	allowed := conv.Participant1ID == userID || conv.Participant2ID == userID
	if !allowed && conv.ConversationType == "org" {
		allowed, err = s.store.IsOrgMember(ctx, conv.OrgID, userID)
		if err != nil {
			return nil, "", fmt.Errorf("checking org membership: %w", err)
		}
	}
	if !allowed {
		return nil, "", ErrDMNotParticipant
	}

	rows, err := s.store.GetConversationMessages(ctx, conversationID, limit, offset, cursor)
	if err != nil {
		return nil, "", fmt.Errorf("getting conversation messages: %w", err)
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
			CreatedAt:         r.CreatedAt.Time.Format(time.RFC3339),
		}

		// 000941 — structured context snapshot on listing-initiated
		// first messages; the thread renders a linked card.
		if r.ContextType != nil {
			msgs[i].ContextType = r.ContextType
			id := r.ContextID.String()
			msgs[i].ContextID = &id
			msgs[i].ContextLabel = r.ContextLabel
			msgs[i].ContextSlug = r.ContextSlug
		}
		msgs[i].SentAsOrg = r.SentAsOrg

		if r.ReadAt.Valid {
			s := r.ReadAt.Time.Format(time.RFC3339)
			msgs[i].ReadAt = &s
		}
	}

	var nextCursor string

	if int32(len(rows)) == effectivePageLimit(limit) && len(rows) > 0 { // #nosec G115 -- slice length bounded by request-scoped data
		last := rows[len(rows)-1]
		nextCursor = encodeDMCursor(dmCursor{T: last.CreatedAt.Time.UnixNano(), ID: last.ID.String()})
	}

	return msgs, nextCursor, nil
}

// MarkAsRead marks messages as read and resets unread count. The ID
// scopes the update so a cross-platform conversation ID cannot be modified
// (BUG-009, Rule 4: platform-scoped). After resetting the DB column, the
// Valkey badge counter jc:dm_total_unread:{user_id} is recalculated from
// the sum of all conversation unread counts (Section 48).
func (s *DMService) MarkAsRead(ctx context.Context, conversationID, userID pgtype.UUID) error {
	// Participation check (IDOR): without it, anyone could flip read_at
	// on another conversation's messages — or write dm_org_read_state
	// rows for an org they don't belong to.
	conv, err := s.store.GetConversationByID(ctx, conversationID)
	if err != nil {
		return fmt.Errorf("verifying conversation access: %w", err)
	}

	allowed := conv.Participant1ID == userID || conv.Participant2ID == userID
	if !allowed && conv.ConversationType == "org" {
		allowed, err = s.store.IsOrgMember(ctx, conv.OrgID, userID)
		if err != nil {
			return fmt.Errorf("checking org membership: %w", err)
		}
	}
	if !allowed {
		return ErrDMNotParticipant
	}

	if err := s.store.MarkMessagesRead(ctx, conversationID, userID); err != nil {
		return fmt.Errorf("marking messages read: %w", err)
	}

	// 000942 — org members read-mark via dm_org_read_state (per-member
	// unread), not the shared participant counter.
	if conv.ConversationType == "org" && conv.Participant1ID != userID {
		if err := s.store.UpsertDMOrgReadState(ctx, conversationID, userID); err != nil {
			return fmt.Errorf("upserting org read state: %w", err)
		}
	} else if err := s.store.ResetUnreadCount(ctx, conversationID, userID); err != nil {
		return fmt.Errorf("resetting unread count: %w", err)
	}

	// Recalculate and re-set the Valkey badge counter (Section 48).
	s.recalcTotalUnread(ctx, userID)

	return nil
}

// HandleMessageRequest accepts, declines, or blocks a message request,
// looked up by its dm_requests.id (the id in the /dm/requests/{id} routes
// and the list DTO — BUG-025). Only the RECIPIENT may act — the requester
// cannot resolve their own outgoing request (BUG-005). Only pending
// requests are actionable. The "block" action additionally inserts a
// user_blocks row so the requester can no longer interact (Section 48).
// Returns the request's conversation_id so the recipient can be taken to
// the thread on accept (Section 48 response contract: {"conv_id": ...}).
func (s *DMService) HandleMessageRequest(ctx context.Context, requestID, userID pgtype.UUID, action string) (string, error) {
	dmReq, err := s.store.GetMessageRequest(ctx, requestID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", ErrDMRequestNotFound
		}

		return "", fmt.Errorf("looking up message request: %w", err)
	}

	// Only the recipient may accept/decline/block — not the requester.
	if dmReq.RecipientID != userID {
		return "", ErrDMRequestNotRecipient
	}

	if dmReq.Status != MessageRequestPending {
		return "", ErrDMRequestAlreadyResolved
	}

	switch action {
	case MessageRequestAccepted:
		if err := s.store.UpdateMessageRequestStatus(ctx, requestID, MessageRequestAccepted); err != nil {
			return "", err
		}

		return dmReq.ConversationID.String(), nil
	case MessageRequestDeclined:
		return "", s.store.UpdateMessageRequestStatus(ctx, requestID, MessageRequestDeclined)
	case MessageRequestBlocked:
		if err := s.store.UpdateMessageRequestStatus(ctx, requestID, MessageRequestBlocked); err != nil {
			return "", err
		}
		// Section 48: block = decline + add requester to user_blocks so they
		// can no longer DM or see the recipient's content (Section 41).
		if err := s.store.CreateUserBlock(ctx, userID, dmReq.RequesterID); err != nil {
			return "", fmt.Errorf("creating user block: %w", err)
		}

		return "", nil
	default:
		return "", fmt.Errorf("invalid action: %s", action)
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
func (s *DMService) GetPendingMessageRequests(ctx context.Context, recipientID pgtype.UUID, q, sort string, limit, offset int32, cursor *dmCursor) ([]MessageRequestInfo, string, error) {
	if sort == "" {
		sort = "recent"
	}

	rows, err := s.store.GetPendingMessageRequests(ctx, recipientID, q, sort, limit, offset, cursor)
	if err != nil {
		return nil, "", fmt.Errorf("getting pending message requests: %w", err)
	}

	infos := make([]MessageRequestInfo, len(rows))
	for i, r := range rows {
		infos[i] = MessageRequestInfo{
			ID:                r.ID.String(),
			RequesterID:       r.RequesterID.String(),
			RequesterUsername: r.RequesterUsername,
			PreviewMessage:    r.PreviewMessage,
			Status:            r.Status,
			CreatedAt:         r.CreatedAt.Time.Format(time.RFC3339),
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

	return infos, nextRequestCursor(rows, sort, limit), nil
}

// nextRequestCursor encodes the keyset resume position for GET /dm/requests
// from the last row — created_at for recent/oldest, the COALESCEd
// requester name for 'name' — tiebroken by request id. Empty = no more.
func nextRequestCursor(rows []gen.GetPendingMessageRequestsRow, sort string, limit int32) string {
	if len(rows) == 0 || int32(len(rows)) < effectivePageLimit(limit) { // #nosec G115 -- slice length bounded by request-scoped data
		return ""
	}

	last := rows[len(rows)-1]
	c := dmCursor{S: sort, ID: last.ID.String()}

	if sort == "name" {
		c.K = last.RequesterUsername
		if last.RequesterDisplayName != nil {
			c.K = *last.RequesterDisplayName
		}
	} else { // recent | oldest — created_at key
		c.T = last.CreatedAt.Time.UnixNano()
	}

	return encodeDMCursor(c)
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
		return ErrDMNotParticipant
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
		return ErrDMNotParticipant
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
	return s.GetOrCreateConversation(ctx, initiatorID, recipientID, previewMessage, nil)
}

// GetOrCreateConversationForInitiatorWithContext carries a structured
// context (000941). A valid service_listing context authored by the
// recipient bypasses the dm_privacy gate — the listing's contact surface
// is commercial, not a cold DM. Blocks still enforced; non-mutual senders
// still land as a message request.
func (s *DMService) GetOrCreateConversationForInitiatorWithContext(ctx context.Context, initiatorID, recipientID pgtype.UUID, previewMessage string, mc *DMMessageContext) (ConversationResult, error) {
	return s.GetOrCreateConversation(ctx, initiatorID, recipientID, previewMessage, mc)
}

// GetTypingTargets returns every user who should receive a typing event
// for a conversation (000942): the other participant for DMs; the external
// user + all org members minus the typer for org conversations. Non-
// participants get ErrDMNotParticipant (silently dropped by the caller).
func (s *DMService) GetTypingTargets(ctx context.Context, conversationID, userID pgtype.UUID) ([]pgtype.UUID, error) {
	conv, err := s.store.GetConversationByID(ctx, conversationID)
	if err != nil {
		return nil, fmt.Errorf("looking up conversation: %w", err)
	}

	if conv.ConversationType == "org" {
		member, mErr := s.store.IsOrgMember(ctx, conv.OrgID, userID)
		if mErr != nil {
			return nil, fmt.Errorf("checking org membership: %w", mErr)
		}
		if conv.Participant1ID != userID && !member {
			return nil, ErrDMNotParticipant
		}
		var targets []pgtype.UUID
		if conv.Participant1ID != userID {
			targets = append(targets, conv.Participant1ID)
		}
		members, lErr := s.store.ListOrgMembers(ctx, conv.OrgID)
		if lErr == nil {
			targets = append(targets, members...)
		}
		return targets, nil
	}

	if conv.Participant1ID != userID && conv.Participant2ID != userID {
		return nil, ErrDMNotParticipant
	}

	if conv.Participant1ID == userID {
		return []pgtype.UUID{conv.Participant2ID}, nil
	}
	return []pgtype.UUID{conv.Participant1ID}, nil
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

	convs, err := s.store.GetUserConversations(ctx, userID, "", "", "", MaxPageSize, 0, nil)
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
