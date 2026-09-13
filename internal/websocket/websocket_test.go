package websocket

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

func TestEventConstants(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		got  string
		want string
	}{
		{"EventNotification", EventNotification, "notification"},
		{"EventPresence", EventPresence, "presence"},
		{"EventTyping", EventTyping, "typing"},
		{"EventSpaceActivity", EventSpaceActivity, "space_activity"},
		{"EventFeedUpdate", EventFeedUpdate, "feed_update"},
		{"EventDMMessage", EventDMMessage, "dm_message"},
		{"EventModerationAlert", EventModerationAlert, "moderation_alert"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if tt.got != tt.want {
				t.Errorf("%s = %q, want %q", tt.name, tt.got, tt.want)
			}
		})
	}
}

func TestDMPrivacyConstants(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		got  string
		want string
	}{
		{"DMPrivacyEveryone", DMPrivacyEveryone, "everyone"},
		{"DMPrivacyFollowersOnly", DMPrivacyFollowersOnly, "followers_only"},
		{"DMPrivacyNobody", DMPrivacyNobody, "nobody"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if tt.got != tt.want {
				t.Errorf("%s = %q, want %q", tt.name, tt.got, tt.want)
			}
		})
	}
}

func TestMessageRequestConstants(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		got  string
		want string
	}{
		{"MessageRequestPending", MessageRequestPending, "pending"},
		{"MessageRequestAccepted", MessageRequestAccepted, "accepted"},
		{"MessageRequestDeclined", MessageRequestDeclined, "declined"},
		{"MessageRequestBlocked", MessageRequestBlocked, "blocked"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if tt.got != tt.want {
				t.Errorf("%s = %q, want %q", tt.name, tt.got, tt.want)
			}
		})
	}
}

func TestPageSizeConstants(t *testing.T) {
	t.Parallel()

	if DefaultPageSize != 20 {
		t.Errorf("DefaultPageSize = %d, want 20", DefaultPageSize)
	}

	if MaxPageSize != 100 {
		t.Errorf("MaxPageSize = %d, want 100", MaxPageSize)
	}
}

// ---------------------------------------------------------------------------
// Hub
// ---------------------------------------------------------------------------

func TestNewHub(t *testing.T) {
	t.Parallel()

	hub := NewHub()
	if hub == nil {
		t.Fatal("NewHub returned nil")
	}

	if hub.clients == nil {
		t.Error("clients map not initialized")
	}

	if hub.spaceSubscribers == nil {
		t.Error("spaceSubscribers map not initialized")
	}
}

func TestHubRegisterAndUnregister(t *testing.T) {
	t.Parallel()

	hub := NewHub()
	client := &Client{
		Hub:    hub,
		UserID: "user-1",

		Send:     make(chan []byte, 256),
		SpaceIDs: make(map[string]bool),
	}

	hub.Register(client)

	if !hub.IsUserOnline("user-1") {
		t.Error("user should be online after register")
	}

	users := hub.GetOnlineUsers()
	if len(users) != 1 {
		t.Errorf("expected 1 online user, got %d", len(users))
	}

	hub.Unregister(client)

	if hub.IsUserOnline("user-1") {
		t.Error("user should be offline after unregister")
	}

	users = hub.GetOnlineUsers()
	if len(users) != 0 {
		t.Errorf("expected 0 online users, got %d", len(users))
	}
}

func TestHubIsUserOnline(t *testing.T) {
	t.Parallel()

	hub := NewHub()

	if hub.IsUserOnline("nonexistent") {
		t.Error("nonexistent user should not be online")
	}

	client := &Client{
		Hub:    hub,
		UserID: "user-1",

		Send:     make(chan []byte, 256),
		SpaceIDs: make(map[string]bool),
	}
	hub.Register(client)

	if !hub.IsUserOnline("user-1") {
		t.Error("user should be online")
	}
}

func TestHubBroadcastToUser(t *testing.T) {
	t.Parallel()

	hub := NewHub()
	client := &Client{
		Hub:    hub,
		UserID: "user-1",

		Send:     make(chan []byte, 256),
		SpaceIDs: make(map[string]bool),
	}
	hub.Register(client)

	event := WSEvent{
		Type:    EventNotification,
		Payload: map[string]interface{}{"message": "test"},
	}
	hub.BroadcastToUser("user-1", event)

	select {
	case msg := <-client.Send:
		var received WSEvent
		if err := json.Unmarshal(msg, &received); err != nil {
			t.Fatalf("failed to unmarshal message: %v", err)
		}

		if received.Type != EventNotification {
			t.Errorf("expected event type %q, got %q", EventNotification, received.Type)
		}
	default:
		t.Error("no message received on client channel")
	}
}

func TestHubBroadcastToUserNonexistent(t *testing.T) {
	t.Parallel()

	hub := NewHub()
	event := WSEvent{Type: "test"}
	// Should not panic.
	hub.BroadcastToUser("nonexistent", event)
}

func TestHubSubscribeToSpace(t *testing.T) {
	t.Parallel()

	hub := NewHub()
	client := &Client{
		Hub:    hub,
		UserID: "user-1",

		Send:     make(chan []byte, 256),
		SpaceIDs: make(map[string]bool),
	}
	hub.Register(client)

	hub.SubscribeToSpace(client, "space-1")

	if !client.SpaceIDs["space-1"] {
		t.Error("client should be subscribed to space-1")
	}

	subs, ok := hub.spaceSubscribers["space-1"]
	if !ok || len(subs) != 1 {
		t.Error("space-1 should have 1 subscriber")
	}

	hub.UnsubscribeFromSpace(client, "space-1")

	if client.SpaceIDs["space-1"] {
		t.Error("client should not be subscribed to space-1 after unsubscribe")
	}

	subs, ok = hub.spaceSubscribers["space-1"]
	if ok && len(subs) > 0 {
		t.Error("space-1 should have 0 subscribers after unsubscribe")
	}
}

func TestHubBroadcastToSpace(t *testing.T) {
	t.Parallel()

	hub := NewHub()
	client1 := &Client{
		Hub:    hub,
		UserID: "user-1",

		Send:     make(chan []byte, 256),
		SpaceIDs: make(map[string]bool),
	}
	client2 := &Client{
		Hub:    hub,
		UserID: "user-2",

		Send:     make(chan []byte, 256),
		SpaceIDs: make(map[string]bool),
	}

	hub.Register(client1)
	hub.Register(client2)

	hub.SubscribeToSpace(client1, "space-1")
	hub.SubscribeToSpace(client2, "space-1")

	event := WSEvent{
		Type:    EventSpaceActivity,
		Payload: map[string]interface{}{"space_id": "space-1"},
	}
	hub.BroadcastToSpace("space-1", event)

	// Both clients should receive the message.
	for _, client := range []*Client{client1, client2} {
		select {
		case msg := <-client.Send:
			var received WSEvent
			if err := json.Unmarshal(msg, &received); err != nil {
				t.Fatalf("failed to unmarshal message for %s: %v", client.UserID, err)
			}

			if received.Type != EventSpaceActivity {
				t.Errorf("expected event type %q, got %q for %s", EventSpaceActivity, received.Type, client.UserID)
			}
		default:
			t.Errorf("no message received for %s", client.UserID)
		}
	}
}

func TestHubUnregisterCleansUpSpaceSubscriptions(t *testing.T) {
	t.Parallel()

	hub := NewHub()
	client := &Client{
		Hub:    hub,
		UserID: "user-1",

		Send:     make(chan []byte, 256),
		SpaceIDs: make(map[string]bool),
	}
	hub.Register(client)
	hub.SubscribeToSpace(client, "space-1")

	hub.Unregister(client)

	subs, ok := hub.spaceSubscribers["space-1"]
	if ok && len(subs) > 0 {
		t.Error("space subscriptions should be cleaned up after unregister")
	}
}

// ---------------------------------------------------------------------------
// WSEvent struct
// ---------------------------------------------------------------------------

func TestWSEventJSONTags(t *testing.T) {
	t.Parallel()

	event := WSEvent{
		Type:    "notification",
		Payload: map[string]interface{}{"key": "value"},
	}

	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("failed to marshal WSEvent: %v", err)
	}

	var got map[string]interface{}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if got["type"] != "notification" {
		t.Errorf("type = %v, want notification", got["type"])
	}
}

// ---------------------------------------------------------------------------
// ConversationInfo struct
// ---------------------------------------------------------------------------

func TestConversationInfoStructFields(t *testing.T) {
	t.Parallel()

	preview := "Hello world"
	info := ConversationInfo{
		ID:                 "conv-1",
		OtherUserID:        "user-2",
		OtherUsername:      "alice",
		OtherDisplayName:   "Alice Smith",
		OtherAvatarURL:     "https://example.com/avatar.jpg",
		LastMessageAt:      nil,
		LastMessagePreview: &preview,
		UnreadCount:        3,
		CreatedAt:          "2026-01-01",
	}

	if info.ID != "conv-1" {
		t.Errorf("ID = %q, want conv-1", info.ID)
	}

	if info.OtherUsername != "alice" {
		t.Errorf("OtherUsername = %q, want alice", info.OtherUsername)
	}

	if info.UnreadCount != 3 {
		t.Errorf("UnreadCount = %d, want 3", info.UnreadCount)
	}

	if info.LastMessagePreview == nil || *info.LastMessagePreview != "Hello world" {
		t.Errorf("LastMessagePreview = %v, want Hello world", info.LastMessagePreview)
	}

	if info.LastMessageAt != nil {
		t.Errorf("LastMessageAt should be nil, got %v", info.LastMessageAt)
	}
}

func TestConversationInfoJSONTags(t *testing.T) {
	t.Parallel()

	info := ConversationInfo{
		ID:            "conv-1",
		OtherUsername: "alice",
		UnreadCount:   3,
	}

	data, err := json.Marshal(info)
	if err != nil {
		t.Fatalf("failed to marshal ConversationInfo: %v", err)
	}

	var got map[string]interface{}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if got["id"] != "conv-1" {
		t.Errorf("id = %v, want conv-1", got["id"])
	}

	if got["other_username"] != "alice" {
		t.Errorf("other_username = %v, want alice", got["other_username"])
	}
}

// ---------------------------------------------------------------------------
// MessageInfo struct
// ---------------------------------------------------------------------------

func TestMessageInfoStructFields(t *testing.T) {
	t.Parallel()

	msg := MessageInfo{
		ID:                "msg-1",
		SenderID:          "user-1",
		SenderUsername:    "bob",
		SenderDisplayName: "Bob Jones",
		Body:              "Hey there",
		CreatedAt:         "2026-01-01",
	}

	if msg.ID != "msg-1" {
		t.Errorf("ID = %q, want msg-1", msg.ID)
	}

	if msg.Body != "Hey there" {
		t.Errorf("Body = %q, want Hey there", msg.Body)
	}
}

func TestMessageInfoNilReadAt(t *testing.T) {
	t.Parallel()

	msg := MessageInfo{ID: "msg-1"}
	if msg.ReadAt != nil {
		t.Errorf("ReadAt should be nil, got %v", msg.ReadAt)
	}
}

// ---------------------------------------------------------------------------
// DMPrivacy validation
// ---------------------------------------------------------------------------

func TestDMPrivacyValidation(t *testing.T) {
	t.Parallel()

	valid := map[string]bool{
		DMPrivacyEveryone:      true,
		DMPrivacyFollowersOnly: true,
		DMPrivacyNobody:        true,
		"invalid":              false,
		"":                     false,
		"EVERYONE":             false,
	}

	for privacy, wantValid := range valid {
		t.Run(privacy, func(t *testing.T) {
			t.Parallel()
			// Simulate the validation logic from DMService.UpdateDMPrivacy.
			gotValid := privacy == DMPrivacyEveryone ||
				privacy == DMPrivacyFollowersOnly ||
				privacy == DMPrivacyNobody
			if gotValid != wantValid {
				t.Errorf("privacy %q: valid = %v, want %v", privacy, gotValid, wantValid)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// MessageRequestStatus validation
// ---------------------------------------------------------------------------

func TestMessageRequestStatusValidation(t *testing.T) {
	t.Parallel()

	valid := map[string]bool{
		MessageRequestPending:  true,
		MessageRequestAccepted: true,
		MessageRequestDeclined: true,
		"invalid":              false,
		"":                     false,
	}

	for status, wantValid := range valid {
		t.Run(status, func(t *testing.T) {
			t.Parallel()

			gotValid := status == MessageRequestPending ||
				status == MessageRequestAccepted ||
				status == MessageRequestDeclined
			if gotValid != wantValid {
				t.Errorf("status %q: valid = %v, want %v", status, gotValid, wantValid)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// DMPrivacyResult struct
// ---------------------------------------------------------------------------

func TestDMPrivacyResultStructFields(t *testing.T) {
	t.Parallel()

	result := DMPrivacyResult{Privacy: DMPrivacyFollowersOnly}
	if result.Privacy != "followers_only" {
		t.Errorf("Privacy = %q, want followers_only", result.Privacy)
	}
}

// ---------------------------------------------------------------------------
// ConversationResult struct
// ---------------------------------------------------------------------------

func TestConversationResultStructFields(t *testing.T) {
	t.Parallel()

	result := ConversationResult{
		IsNew: true,
	}
	if !result.IsNew {
		t.Error("IsNew should be true")
	}
}

// ---------------------------------------------------------------------------
// WS-005: handleTypingEvent — typing indicator routing (Section 10)
// ---------------------------------------------------------------------------
//
// handleTypingEvent parses a typing event from the client, looks up the other
// participant via DMService.GetOtherParticipant, and broadcasts a typing event
// to that participant. Non-participants get a silent no-op. These tests use a
// mock DMService to avoid a DB dependency.

// mockDMServiceForTyping implements the subset of DMService needed by
// handleTypingEvent: just GetOtherParticipant.
type mockDMServiceForTyping struct {
	// otherParticipantID is returned for valid participant lookups.
	otherParticipantID string
	// err controls the return value. If non-nil, the lookup "fails" (simulating
	// a non-participant or non-existent conversation).
	err error
	// calls records the arguments passed to GetOtherParticipant.
	calls []mockTypingCall
}

type mockTypingCall struct {
	tenantID       string
	conversationID string
	userID         string
}

func (m *mockDMServiceForTyping) GetOtherParticipant(_ context.Context, tenantID, conversationID, userID pgtype.UUID) (pgtype.UUID, error) {
	m.calls = append(m.calls, mockTypingCall{
		tenantID:       tenantID.String(),
		conversationID: conversationID.String(),
		userID:         userID.String(),
	})
	if m.err != nil {
		return pgtype.UUID{}, m.err
	}

	var id pgtype.UUID
	_ = id.Scan(m.otherParticipantID)

	return id, nil
}

func TestHandleTypingEventBroadcastsToOtherParticipant(t *testing.T) {
	t.Parallel()

	const (
		senderID    = "11111111-1111-1111-1111-111111111111"
		recipientID = "22222222-2222-2222-2222-222222222222"
		tenantID    = "33333333-3333-3333-3333-333333333333"
		convID      = "44444444-4444-4444-4444-444444444444"
	)

	hub := NewHub()
	otherClient := &Client{
		Hub:    hub,
		UserID: recipientID,

		Send:     make(chan []byte, 256),
		SpaceIDs: make(map[string]bool),
	}
	hub.Register(otherClient)

	sender := &Client{
		Hub:    hub,
		UserID: senderID,

		Send:     make(chan []byte, 256),
		SpaceIDs: make(map[string]bool),
	}

	mockSvc := &mockDMServiceForTyping{otherParticipantID: recipientID}
	sender.handleTypingEvent(mockSvc, WSEvent{
		Type: EventTyping,
		Payload: map[string]interface{}{
			"conversation_id": convID,
			"is_typing":       true,
		},
	})

	if len(mockSvc.calls) != 1 {
		t.Fatalf("expected 1 GetOtherParticipant call, got %d", len(mockSvc.calls))
	}

	// The other participant should receive a typing event.
	select {
	case msg := <-otherClient.Send:
		var ev WSEvent
		if err := json.Unmarshal(msg, &ev); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}

		if ev.Type != EventTyping {
			t.Errorf("type = %q, want %q", ev.Type, EventTyping)
		}

		payload, ok := ev.Payload.(map[string]interface{})
		if !ok {
			t.Fatal("payload is not a map")
		}

		if payload["user_id"] != senderID {
			t.Errorf("user_id = %v, want %s", payload["user_id"], senderID)
		}

		if payload["is_typing"] != true {
			t.Errorf("is_typing = %v, want true", payload["is_typing"])
		}
	default:
		t.Error("other participant did not receive typing event")
	}
}

func TestHandleTypingEventNonParticipantSilentlyDrops(t *testing.T) {
	t.Parallel()

	const (
		recipientID = "22222222-2222-2222-2222-222222222222"
		tenantID    = "33333333-3333-3333-3333-333333333333"
		convID      = "44444444-4444-4444-4444-444444444444"
	)

	hub := NewHub()
	otherClient := &Client{
		Hub:    hub,
		UserID: recipientID,

		Send:     make(chan []byte, 256),
		SpaceIDs: make(map[string]bool),
	}
	hub.Register(otherClient)

	sender := &Client{
		Hub:    hub,
		UserID: "55555555-5555-5555-5555-555555555555",

		Send:     make(chan []byte, 256),
		SpaceIDs: make(map[string]bool),
	}

	// Simulate a non-participant: GetOtherParticipant returns an error.
	mockSvc := &mockDMServiceForTyping{err: errors.New("not a participant")}
	sender.handleTypingEvent(mockSvc, WSEvent{
		Type: EventTyping,
		Payload: map[string]interface{}{
			"conversation_id": convID,
			"is_typing":       true,
		},
	})

	// The other participant should NOT receive anything.
	select {
	case <-otherClient.Send:
		t.Error("recipient received typing event from non-participant — membership leak")
	default:
		// expected
	}
}

func TestHandleTypingEventNilDMServiceIsNoOp(t *testing.T) {
	t.Parallel()

	hub := NewHub()
	sender := &Client{
		Hub:    hub,
		UserID: "11111111-1111-1111-1111-111111111111",

		Send:     make(chan []byte, 256),
		SpaceIDs: make(map[string]bool),
	}

	// Should not panic with nil DMService.
	sender.handleTypingEvent(nil, WSEvent{
		Type:    EventTyping,
		Payload: map[string]interface{}{"conversation_id": "44444444-4444-4444-4444-444444444444", "is_typing": true},
	})
}

func TestHandleTypingEventMissingConversationIDIgnores(t *testing.T) {
	t.Parallel()

	hub := NewHub()
	sender := &Client{
		Hub:    hub,
		UserID: "11111111-1111-1111-1111-111111111111",

		Send:     make(chan []byte, 256),
		SpaceIDs: make(map[string]bool),
	}

	mockSvc := &mockDMServiceForTyping{otherParticipantID: "22222222-2222-2222-2222-222222222222"}
	// Missing conversation_id — should be a no-op, no DB lookup.
	sender.handleTypingEvent(mockSvc, WSEvent{
		Type: EventTyping,
		Payload: map[string]interface{}{
			"is_typing": true,
		},
	})

	if len(mockSvc.calls) != 0 {
		t.Errorf("expected 0 GetOtherParticipant calls, got %d", len(mockSvc.calls))
	}
}

// ---------------------------------------------------------------------------
// WS-006: SessionChecker signature (userID + tenantID)
// ---------------------------------------------------------------------------

