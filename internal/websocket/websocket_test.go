package websocket

import (
	"encoding/json"
	"testing"
)

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

func TestEventConstants(t *testing.T) {
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
			if tt.got != tt.want {
				t.Errorf("%s = %q, want %q", tt.name, tt.got, tt.want)
			}
		})
	}
}

func TestDMPrivacyConstants(t *testing.T) {
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
			if tt.got != tt.want {
				t.Errorf("%s = %q, want %q", tt.name, tt.got, tt.want)
			}
		})
	}
}

func TestMessageRequestConstants(t *testing.T) {
	tests := []struct {
		name string
		got  string
		want string
	}{
		{"MessageRequestPending", MessageRequestPending, "pending"},
		{"MessageRequestAccepted", MessageRequestAccepted, "accepted"},
		{"MessageRequestDeclined", MessageRequestDeclined, "declined"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("%s = %q, want %q", tt.name, tt.got, tt.want)
			}
		})
	}
}

func TestPageSizeConstants(t *testing.T) {
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
	hub := NewHub()
	client := &Client{
		Hub:      hub,
		UserID:   "user-1",
		TenantID: "tenant-1",
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
	hub := NewHub()

	if hub.IsUserOnline("nonexistent") {
		t.Error("nonexistent user should not be online")
	}

	client := &Client{
		Hub:      hub,
		UserID:   "user-1",
		TenantID: "tenant-1",
		Send:     make(chan []byte, 256),
		SpaceIDs: make(map[string]bool),
	}
	hub.Register(client)

	if !hub.IsUserOnline("user-1") {
		t.Error("user should be online")
	}
}

func TestHubBroadcastToUser(t *testing.T) {
	hub := NewHub()
	client := &Client{
		Hub:      hub,
		UserID:   "user-1",
		TenantID: "tenant-1",
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
	hub := NewHub()
	event := WSEvent{Type: "test"}
	// Should not panic.
	hub.BroadcastToUser("nonexistent", event)
}

func TestHubSubscribeToSpace(t *testing.T) {
	hub := NewHub()
	client := &Client{
		Hub:      hub,
		UserID:   "user-1",
		TenantID: "tenant-1",
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
	hub := NewHub()
	client1 := &Client{
		Hub:      hub,
		UserID:   "user-1",
		TenantID: "tenant-1",
		Send:     make(chan []byte, 256),
		SpaceIDs: make(map[string]bool),
	}
	client2 := &Client{
		Hub:      hub,
		UserID:   "user-2",
		TenantID: "tenant-1",
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
	hub := NewHub()
	client := &Client{
		Hub:      hub,
		UserID:   "user-1",
		TenantID: "tenant-1",
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
	preview := "Hello world"
	info := ConversationInfo{
		ID:                "conv-1",
		OtherUserID:       "user-2",
		OtherUsername:     "alice",
		OtherDisplayName:  "Alice Smith",
		OtherAvatarURL:    "https://example.com/avatar.jpg",
		LastMessageAt:     nil,
		LastMessagePreview: &preview,
		UnreadCount:       3,
		CreatedAt:         "2026-01-01",
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
	msg := MessageInfo{ID: "msg-1"}
	if msg.ReadAt != nil {
		t.Errorf("ReadAt should be nil, got %v", msg.ReadAt)
	}
}

// ---------------------------------------------------------------------------
// Task payloads
// ---------------------------------------------------------------------------

func TestDMSendNotificationPayloadJSONTags(t *testing.T) {
	p := DMSendNotificationPayload{
		ConversationID: "conv-1",
		MessageID:      "msg-1",
		SenderID:       "user-1",
	}
	data, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("failed to marshal DMSendNotificationPayload: %v", err)
	}
	var got map[string]interface{}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if got["conversation_id"] != "conv-1" {
		t.Errorf("conversation_id = %v, want conv-1", got["conversation_id"])
	}
	if got["message_id"] != "msg-1" {
		t.Errorf("message_id = %v, want msg-1", got["message_id"])
	}
	if got["sender_id"] != "user-1" {
		t.Errorf("sender_id = %v, want user-1", got["sender_id"])
	}
}

// ---------------------------------------------------------------------------
// DMPrivacy validation
// ---------------------------------------------------------------------------

func TestDMPrivacyValidation(t *testing.T) {
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
	valid := map[string]bool{
		MessageRequestPending:  true,
		MessageRequestAccepted: true,
		MessageRequestDeclined: true,
		"invalid":              false,
		"":                     false,
	}

	for status, wantValid := range valid {
		t.Run(status, func(t *testing.T) {
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
	result := DMPrivacyResult{Privacy: DMPrivacyFollowersOnly}
	if result.Privacy != "followers_only" {
		t.Errorf("Privacy = %q, want followers_only", result.Privacy)
	}
}

// ---------------------------------------------------------------------------
// ConversationResult struct
// ---------------------------------------------------------------------------

func TestConversationResultStructFields(t *testing.T) {
	result := ConversationResult{
		IsNew: true,
	}
	if !result.IsNew {
		t.Error("IsNew should be true")
	}
}
