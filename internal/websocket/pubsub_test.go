package websocket

import (
	"encoding/json"
	"testing"
)

// ---------------------------------------------------------------------------
// PubSubMessage
// ---------------------------------------------------------------------------

func TestPubSubMessageJSONRoundTrip(t *testing.T) {
	t.Parallel()

	msg := PubSubMessage{
		Type:          EventNotification,
		TargetUserID:  "user-1",
		TargetSpaceID: "",
		Payload:       map[string]string{"hello": "world"},
	}

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got PubSubMessage
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if got.Type != EventNotification {
		t.Errorf("type = %q, want %q", got.Type, EventNotification)
	}

	if got.TargetUserID != "user-1" {
		t.Errorf("target_user_id = %q, want user-1", got.TargetUserID)
	}

	if got.TargetSpaceID != "" {
		t.Errorf("target_space_id = %q, want empty", got.TargetSpaceID)
	}
}

func TestPubSubMessageOmitsEmptyFields(t *testing.T) {
	t.Parallel()

	msg := PubSubMessage{
		Type:    EventFeedUpdate,
		Payload: "data",
	}

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal raw: %v", err)
	}

	if _, ok := raw["target_user_id"]; ok {
		t.Error("target_user_id should be omitted when empty")
	}

	if _, ok := raw["target_space_id"]; ok {
		t.Error("target_space_id should be omitted when empty")
	}
}

func TestPubSubMessageWithSpaceTarget(t *testing.T) {
	t.Parallel()

	msg := PubSubMessage{
		Type:          EventSpaceActivity,
		TargetSpaceID: "space-42",
		Payload:       map[string]string{"action": "post"},
	}

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got PubSubMessage
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if got.TargetSpaceID != "space-42" {
		t.Errorf("target_space_id = %q, want space-42", got.TargetSpaceID)
	}

	if got.TargetUserID != "" {
		t.Errorf("target_user_id = %q, want empty", got.TargetUserID)
	}
}

// ---------------------------------------------------------------------------
// PubSubBridge type existence
// ---------------------------------------------------------------------------

func TestPubSubBridgeTypeExists(t *testing.T) {
	t.Parallel()
	// Verify PubSubBridge is accessible from test code.
	var b *PubSubBridge
	if b != nil {
		t.Error("expected nil PubSubBridge")
	}
}

// ---------------------------------------------------------------------------
// dispatch
// ---------------------------------------------------------------------------

func TestDispatchToUser(t *testing.T) {
	t.Parallel()

	hub := NewHub()
	bridge := &PubSubBridge{hub: hub}

	client := &Client{
		Hub:      hub,
		UserID:   "user-dispatch",
		TenantID: "t1",
		Send:     make(chan []byte, 256),
		SpaceIDs: make(map[string]bool),
	}
	hub.Register(client)

	msg := PubSubMessage{
		Type:         EventNotification,
		TargetUserID: "user-dispatch",
		Payload:      map[string]string{"key": "val"},
	}
	bridge.dispatch(msg)

	select {
	case raw := <-client.Send:
		var ev WSEvent
		if err := json.Unmarshal(raw, &ev); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}

		if ev.Type != EventNotification {
			t.Errorf("type = %q, want %q", ev.Type, EventNotification)
		}
	default:
		t.Error("no message dispatched to user")
	}
}

func TestDispatchToSpace(t *testing.T) {
	t.Parallel()

	hub := NewHub()
	bridge := &PubSubBridge{hub: hub}

	c1 := &Client{Hub: hub, UserID: "u1", TenantID: "t1", Send: make(chan []byte, 256), SpaceIDs: make(map[string]bool)}
	c2 := &Client{Hub: hub, UserID: "u2", TenantID: "t1", Send: make(chan []byte, 256), SpaceIDs: make(map[string]bool)}
	hub.Register(c1)
	hub.Register(c2)
	hub.SubscribeToSpace(c1, "s-dispatch")
	hub.SubscribeToSpace(c2, "s-dispatch")

	msg := PubSubMessage{
		Type:           EventSpaceActivity,
		TargetTenantID: "t1",
		TargetSpaceID:  "s-dispatch",
		Payload:        "space-data",
	}
	bridge.dispatch(msg)

	for _, c := range []*Client{c1, c2} {
		select {
		case raw := <-c.Send:
			var ev WSEvent
			if err := json.Unmarshal(raw, &ev); err != nil {
				t.Fatalf("unmarshal for %s: %v", c.UserID, err)
			}

			if ev.Type != EventSpaceActivity {
				t.Errorf("%s: type = %q, want %q", c.UserID, ev.Type, EventSpaceActivity)
			}
		default:
			t.Errorf("no message dispatched to %s", c.UserID)
		}
	}
}

func TestDispatchToAll(t *testing.T) {
	t.Parallel()

	hub := NewHub()
	bridge := &PubSubBridge{hub: hub}

	c1 := &Client{Hub: hub, UserID: "u1", TenantID: "t1", Send: make(chan []byte, 256), SpaceIDs: make(map[string]bool)}
	c2 := &Client{Hub: hub, UserID: "u2", TenantID: "t1", Send: make(chan []byte, 256), SpaceIDs: make(map[string]bool)}
	hub.Register(c1)
	hub.Register(c2)

	msg := PubSubMessage{
		Type:    EventFeedUpdate,
		Payload: "feed-data",
	}
	bridge.dispatch(msg)

	for _, c := range []*Client{c1, c2} {
		select {
		case raw := <-c.Send:
			var ev WSEvent
			if err := json.Unmarshal(raw, &ev); err != nil {
				t.Fatalf("unmarshal for %s: %v", c.UserID, err)
			}

			if ev.Type != EventFeedUpdate {
				t.Errorf("%s: type = %q, want %q", c.UserID, ev.Type, EventFeedUpdate)
			}
		default:
			t.Errorf("no message dispatched to %s", c.UserID)
		}
	}
}

func TestDispatchEmptyTargetBroadcastsToAll(t *testing.T) {
	t.Parallel()

	hub := NewHub()
	bridge := &PubSubBridge{hub: hub}

	client := &Client{Hub: hub, UserID: "u-broadcast", TenantID: "t1", Send: make(chan []byte, 256), SpaceIDs: make(map[string]bool)}
	hub.Register(client)

	msg := PubSubMessage{
		Type:    EventModerationAlert,
		Payload: "alert",
	}
	bridge.dispatch(msg)

	select {
	case <-client.Send:
		// message received — correct path
	default:
		t.Error("no message dispatched to all clients")
	}
}

// ---------------------------------------------------------------------------
// dispatch must NOT re-publish (infinite-loop prevention)
// ---------------------------------------------------------------------------
//
// When a PubSubMessage arrives over Valkey, dispatch must deliver it locally
// only. If it called the public Broadcast* methods, those would re-publish to
// Valkey, and every instance (including the originator) would receive and
// re-publish the same message forever — an infinite fan-out loop. These
// regression tests wire a mockBridge that records Publish calls and assert
// that dispatch triggers zero Publish calls.

func TestDispatchToUserDoesNotRepublish(t *testing.T) {
	t.Parallel()

	hub := NewHub()
	mock := &mockBridge{hub: hub}
	hub.SetPubSub(mock) // wired as in production
	bridge := &PubSubBridge{hub: hub}

	client := &Client{Hub: hub, UserID: "u-loop", TenantID: "t1", Send: make(chan []byte, 256), SpaceIDs: make(map[string]bool)}
	hub.Register(client)

	bridge.dispatch(PubSubMessage{
		Type:         EventNotification,
		TargetUserID: "u-loop",
		Payload:      "x",
	})

	// Local delivery happened.
	select {
	case <-client.Send:
	default:
		t.Fatal("local delivery failed")
	}

	// And critically, dispatch did NOT re-publish.
	if mock.lastEvent.Type != "" {
		t.Errorf("dispatch re-published event %q; dispatch must be local-only to avoid an infinite loop", mock.lastEvent.Type)
	}
}

func TestDispatchToSpaceDoesNotRepublish(t *testing.T) {
	t.Parallel()

	hub := NewHub()
	mock := &mockBridge{hub: hub}
	hub.SetPubSub(mock)
	bridge := &PubSubBridge{hub: hub}

	c := &Client{Hub: hub, UserID: "u-sp-loop", TenantID: "t1", Send: make(chan []byte, 256), SpaceIDs: make(map[string]bool)}
	hub.Register(c)
	hub.SubscribeToSpace(c, "s-loop")

	bridge.dispatch(PubSubMessage{
		Type:           EventSpaceActivity,
		TargetTenantID: "t1",
		TargetSpaceID:  "s-loop",
		Payload:        "x",
	})

	select {
	case <-c.Send:
	default:
		t.Fatal("local delivery failed")
	}

	if mock.lastEvent.Type != "" {
		t.Errorf("dispatch re-published event %q; dispatch must be local-only to avoid an infinite loop", mock.lastEvent.Type)
	}
}

func TestDispatchToAllDoesNotRepublish(t *testing.T) {
	t.Parallel()

	hub := NewHub()
	mock := &mockBridge{hub: hub}
	hub.SetPubSub(mock)
	bridge := &PubSubBridge{hub: hub}

	c := &Client{Hub: hub, UserID: "u-all-loop", TenantID: "t1", Send: make(chan []byte, 256), SpaceIDs: make(map[string]bool)}
	hub.Register(c)

	bridge.dispatch(PubSubMessage{
		Type:    EventFeedUpdate,
		Payload: "x",
	})

	select {
	case <-c.Send:
	default:
		t.Fatal("local delivery failed")
	}

	if mock.lastEvent.Type != "" {
		t.Errorf("dispatch re-published event %q; dispatch must be local-only to avoid an infinite loop", mock.lastEvent.Type)
	}
}

// ---------------------------------------------------------------------------
// Hub.SetPubSub
// ---------------------------------------------------------------------------

func TestHubSetPubSub(t *testing.T) {
	t.Parallel()

	hub := NewHub()
	bridge := &PubSubBridge{hub: hub}

	hub.SetPubSub(bridge)

	hub.mu.RLock()
	got := hub.pubsub
	hub.mu.RUnlock()

	if got != bridge {
		t.Error("SetPubSub did not store the bridge")
	}
}

func TestHubBroadcastWithoutBridge(t *testing.T) {
	t.Parallel()

	hub := NewHub()
	client := &Client{Hub: hub, UserID: "u-nobs", TenantID: "t1", Send: make(chan []byte, 256), SpaceIDs: make(map[string]bool)}
	hub.Register(client)

	event := WSEvent{Type: EventNotification, Payload: "no-bridge"}
	hub.BroadcastToUser("u-nobs", event)

	select {
	case <-client.Send:
		// local delivery works without bridge
	default:
		t.Error("local delivery failed without bridge")
	}
}

// ---------------------------------------------------------------------------
// Hub broadcast triggers Publish
// ---------------------------------------------------------------------------

func TestHubBroadcastTriggersPublish(t *testing.T) {
	t.Parallel()

	hub := NewHub()
	client := &Client{Hub: hub, UserID: "u-pub", TenantID: "t1", Send: make(chan []byte, 256), SpaceIDs: make(map[string]bool)}
	hub.Register(client)

	bridge := &mockBridge{hub: hub}
	hub.SetPubSub(bridge)

	event := WSEvent{Type: EventDMMessage, Payload: "hello"}
	hub.BroadcastToUser("u-pub", event)

	// Verify local delivery.
	select {
	case <-client.Send:
	default:
		t.Error("local delivery failed")
	}

	// Verify publish was called.
	if bridge.lastEvent.Type != EventDMMessage {
		t.Errorf("publish type = %q, want %q", bridge.lastEvent.Type, EventDMMessage)
	}

	if bridge.lastUserID != "u-pub" {
		t.Errorf("publish user = %q, want u-pub", bridge.lastUserID)
	}
}

func TestHubSpaceBroadcastTriggersPublish(t *testing.T) {
	t.Parallel()

	hub := NewHub()
	client := &Client{Hub: hub, UserID: "u-sp", TenantID: "t1", Send: make(chan []byte, 256), SpaceIDs: make(map[string]bool)}
	hub.Register(client)
	hub.SubscribeToSpace(client, "s-pub")

	bridge := &mockBridge{hub: hub}
	hub.SetPubSub(bridge)

	event := WSEvent{Type: EventSpaceActivity, Payload: "space-data"}
	hub.BroadcastToSpace("t1", "s-pub", event)

	select {
	case <-client.Send:
	default:
		t.Error("local delivery failed")
	}

	if bridge.lastSpaceID != "s-pub" {
		t.Errorf("publish space = %q, want s-pub", bridge.lastSpaceID)
	}
}

func TestHubAllBroadcastTriggersPublish(t *testing.T) {
	t.Parallel()

	hub := NewHub()
	client := &Client{Hub: hub, UserID: "u-all", TenantID: "t1", Send: make(chan []byte, 256), SpaceIDs: make(map[string]bool)}
	hub.Register(client)

	bridge := &mockBridge{hub: hub}
	hub.SetPubSub(bridge)

	event := WSEvent{Type: EventFeedUpdate, Payload: "all-data"}
	hub.BroadcastToAll(event)

	select {
	case <-client.Send:
	default:
		t.Error("local delivery failed")
	}

	if bridge.lastEvent.Type != EventFeedUpdate {
		t.Errorf("publish type = %q, want %q", bridge.lastEvent.Type, EventFeedUpdate)
	}
	// BroadcastToAll publishes with no target.
	if bridge.lastUserID != "" || bridge.lastSpaceID != "" {
		t.Error("BroadcastToAll should publish with no target")
	}
}

// mockBridge is a lightweight stand-in for PubSubBridge that records the
// last Publish call without requiring a real Valkey connection.
type mockBridge struct {
	hub           *Hub
	lastEvent     WSEvent
	lastUserID    string
	lastTenantID  string
	lastSpaceID   string
}

func (m *mockBridge) Publish(event WSEvent, userID, tenantID, spaceID string) {
	m.lastEvent = event
	m.lastUserID = userID
	m.lastTenantID = tenantID
	m.lastSpaceID = spaceID
}

// PublishTenant satisfies the extended Publisher interface (tenant-scoped
// broadcast for presence). Records the call so tests can assert it fired.
func (m *mockBridge) PublishTenant(event WSEvent, tenantID string) {
	m.lastEvent = event
	m.lastTenantID = tenantID
	m.lastUserID = ""
	m.lastSpaceID = ""
}

// Ensure mockBridge satisfies the implicit interface at compile time.
var _ = (*mockBridge)(nil)

// ---------------------------------------------------------------------------
// BroadcastToTenant — tenant-scoped presence (WS-002 regression)
// ---------------------------------------------------------------------------
//
// Presence must be delivered only to clients in the same tenant. A user in
// tenant A connecting must NOT cause a presence event to reach a user in
// tenant B. These tests verify both local delivery isolation and that the
// bridge PublishTenant path is used (not the global Publish path).

func TestBroadcastToTenantDeliversOnlyToSameTenant(t *testing.T) {
	t.Parallel()

	hub := NewHub()
	sameTenant := &Client{Hub: hub, UserID: "u-same", TenantID: "t1", Send: make(chan []byte, 256), SpaceIDs: make(map[string]bool)}
	otherTenant := &Client{Hub: hub, UserID: "u-other", TenantID: "t2", Send: make(chan []byte, 256), SpaceIDs: make(map[string]bool)}
	hub.Register(sameTenant)
	hub.Register(otherTenant)

	hub.BroadcastToTenant("t1", WSEvent{Type: EventPresence, Payload: "online"})

	// Same-tenant client receives the event.
	select {
	case <-sameTenant.Send:
	default:
		t.Error("same-tenant client did not receive tenant broadcast")
	}

	// Other-tenant client must NOT receive the event.
	select {
	case <-otherTenant.Send:
		t.Error("other-tenant client received tenant broadcast — cross-tenant leak")
	default:
		// expected
	}
}

func TestBroadcastToTenantTriggersPublishTenant(t *testing.T) {
	t.Parallel()

	hub := NewHub()
	client := &Client{Hub: hub, UserID: "u-pres", TenantID: "t1", Send: make(chan []byte, 256), SpaceIDs: make(map[string]bool)}
	hub.Register(client)

	mock := &mockBridge{hub: hub}
	hub.SetPubSub(mock)

	hub.BroadcastToTenant("t1", WSEvent{Type: EventPresence, Payload: "online"})

	if mock.lastEvent.Type != EventPresence {
		t.Errorf("publish type = %q, want %q", mock.lastEvent.Type, EventPresence)
	}
	if mock.lastTenantID != "t1" {
		t.Errorf("publish tenant = %q, want t1", mock.lastTenantID)
	}
}

func TestDispatchTenantBroadcastDoesNotLeakAcrossTenants(t *testing.T) {
	t.Parallel()

	hub := NewHub()
	bridge := &PubSubBridge{hub: hub}

	t1Client := &Client{Hub: hub, UserID: "u-t1", TenantID: "t1", Send: make(chan []byte, 256), SpaceIDs: make(map[string]bool)}
	t2Client := &Client{Hub: hub, UserID: "u-t2", TenantID: "t2", Send: make(chan []byte, 256), SpaceIDs: make(map[string]bool)}
	hub.Register(t1Client)
	hub.Register(t2Client)

	bridge.dispatch(PubSubMessage{
		Type:            EventPresence,
		TargetTenantID:  "t1",
		TenantBroadcast: true,
		Payload:         "online",
	})

	select {
	case <-t1Client.Send:
	default:
		t.Error("t1 client did not receive tenant broadcast")
	}

	select {
	case <-t2Client.Send:
		t.Error("t2 client received t1 tenant broadcast — cross-tenant leak via pubsub")
	default:
		// expected
	}
}

func TestDispatchTenantBroadcastDoesNotRepublish(t *testing.T) {
	t.Parallel()

	hub := NewHub()
	mock := &mockBridge{hub: hub}
	hub.SetPubSub(mock)
	bridge := &PubSubBridge{hub: hub}

	client := &Client{Hub: hub, UserID: "u-tnb", TenantID: "t1", Send: make(chan []byte, 256), SpaceIDs: make(map[string]bool)}
	hub.Register(client)

	bridge.dispatch(PubSubMessage{
		Type:            EventPresence,
		TargetTenantID:  "t1",
		TenantBroadcast: true,
		Payload:         "x",
	})

	select {
	case <-client.Send:
	default:
		t.Fatal("local delivery failed")
	}

	if mock.lastEvent.Type != "" {
		t.Errorf("dispatch re-published event %q; dispatch must be local-only to avoid an infinite loop", mock.lastEvent.Type)
	}
}

// ---------------------------------------------------------------------------
// PubSubChannel constant
// ---------------------------------------------------------------------------

func TestPubSubChannelConstant(t *testing.T) {
	t.Parallel()

	if PubSubChannel != "jc:ws:broadcast" {
		t.Errorf("PubSubChannel = %q, want jc:ws:broadcast", PubSubChannel)
	}
}
