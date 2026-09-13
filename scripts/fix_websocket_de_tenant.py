#!/usr/bin/env python3
"""De-tenant the websocket package enough for single-tenant operation and tests."""
from pathlib import Path
import re

ws = Path("internal/websocket")

hub = ws / "hub.go"
text = hub.read_text()

# Client: keep TenantID field for tests, but remove from SessionChecker and space keys.
# Change Client.SessionChecker to func(userID string) bool
text = re.sub(
    r"SessionChecker func\(userID, tenantID string\) bool",
    "SessionChecker func(userID string) bool",
    text,
)

# spaceSubscriptionKey single-tenant
old = """// spaceSubscriptionKey builds the map key for a space subscription, scoped by
// tenantID so cross-tenant space ID collisions cannot leak events (Rule 4).
func spaceSubscriptionKey(tenantID, spaceID string) string {
	return tenantID + ":" + spaceID
}"""
new = """// spaceSubscriptionKey builds the map key for a space subscription.
func spaceSubscriptionKey(spaceID string) string {
	return spaceID
}"""
text = text.replace(old, new)

# Update space key calls
for fn in ["removeSpaceSubscriptionsLocked", "SubscribeToSpace", "UnsubscribeFromSpace", "deliverLocalSpace"]:
    text = text.replace(f"spaceSubscriptionKey(client.TenantID, spaceID)", "spaceSubscriptionKey(spaceID)")
    text = text.replace(f"spaceSubscriptionKey(tenantID, spaceID)", "spaceSubscriptionKey(spaceID)")

# Update SubscribeToSpace signature and body
old = """// SubscribeToSpace adds a client to the space's subscriber set. The key is
// scoped to tenantID so cross-tenant space ID collisions cannot leak events
// (Rule 4: tenant-scoped).
func (h *Hub) SubscribeToSpace(client *Client, tenantID, spaceID string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	client.SpaceIDs[spaceID] = true

	key := spaceSubscriptionKey(client.TenantID, spaceID)"""
new = """// SubscribeToSpace adds a client to the space's subscriber set.
func (h *Hub) SubscribeToSpace(client *Client, spaceID string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	client.SpaceIDs[spaceID] = true

	key := spaceSubscriptionKey(spaceID)"""
text = text.replace(old, new)

old = """// UnsubscribeFromSpace removes a client from a space's subscriber set. The
// key is scoped to tenantID (Rule 4: tenant-scoped).
func (h *Hub) UnsubscribeFromSpace(client *Client, tenantID, spaceID string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	delete(client.SpaceIDs, spaceID)

	key := spaceSubscriptionKey(client.TenantID, spaceID)"""
new = """// UnsubscribeFromSpace removes a client from a space's subscriber set.
func (h *Hub) UnsubscribeFromSpace(client *Client, spaceID string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	delete(client.SpaceIDs, spaceID)

	key := spaceSubscriptionKey(spaceID)"""
text = text.replace(old, new)

# Update BroadcastToSpace signature and calls
old = """// BroadcastToSpace sends an event to all subscribers of a space on this
// instance and publishes to Valkey for delivery on other instances. The
// subscription is scoped to tenantID so cross-tenant space ID collisions
// cannot leak events (Rule 4: tenant-scoped).
func (h *Hub) BroadcastToSpace(tenantID, spaceID string, event WSEvent) {
	h.deliverLocalSpace(tenantID, spaceID, event)

	h.mu.RLock()
	bridge := h.pubsub
	h.mu.RUnlock()

	if bridge != nil {
		bridge.Publish(event, "", tenantID, spaceID)
	}
}"""
new = """// BroadcastToSpace sends an event to all subscribers of a space on this
// instance and publishes to Valkey for delivery on other instances.
func (h *Hub) BroadcastToSpace(spaceID string, event WSEvent) {
	h.deliverLocalSpace(spaceID, event)

	h.mu.RLock()
	bridge := h.pubsub
	h.mu.RUnlock()

	if bridge != nil {
		bridge.Publish(event, "", "", spaceID)
	}
}"""
text = text.replace(old, new)

old = """// deliverLocalSpace sends an event to all subscribers of a space connected
// to THIS instance only. It does NOT publish to Valkey — see
// deliverLocalUser for the loop-safety rationale. The subscription key is
// scoped to tenantID (Rule 4: tenant-scoped).
func (h *Hub) deliverLocalSpace(tenantID, spaceID string, event WSEvent) {
	h.mu.RLock()

	subs, ok := h.spaceSubscribers[spaceSubscriptionKey(tenantID, spaceID)]
	if !ok {
		h.mu.RUnlock()
		return
	}"""
new = """// deliverLocalSpace sends an event to all subscribers of a space connected
// to THIS instance only. It does NOT publish to Valkey — see
// deliverLocalUser for the loop-safety rationale.
func (h *Hub) deliverLocalSpace(spaceID string, event WSEvent) {
	h.mu.RLock()

	subs, ok := h.spaceSubscribers[spaceSubscriptionKey(spaceID)]
	if !ok {
		h.mu.RUnlock()
		return
	}"""
text = text.replace(old, new)

# BroadcastToTenant: make it global broadcast (single tenant)
old = """// BroadcastToTenant sends an event to every connected client in a specific
// tenant on this instance and publishes to Valkey for delivery to other
// instances. Use this for presence and other tenant-scoped broadcasts so a
// user in tenant A never receives events from tenant B (Rule 4: tenant-scoped,
// §5.3 cross-tenant leak prevention).
func (h *Hub) BroadcastToTenant(tenantID string, event WSEvent) {
	h.deliverLocalTenant(tenantID, event)

	h.mu.RLock()
	bridge := h.pubsub
	h.mu.RUnlock()

	if bridge != nil {
		bridge.PublishTenant(event, tenantID)
	}
}"""
new = """// BroadcastToTenant sends an event to every connected client on this
// instance and publishes to Valkey for delivery to other instances. In the
// single-tenant system this is equivalent to BroadcastToAll.
func (h *Hub) BroadcastToTenant(_ string, event WSEvent) {
	h.deliverLocalAll(event)

	h.mu.RLock()
	bridge := h.pubsub
	h.mu.RUnlock()

	if bridge != nil {
		bridge.PublishTenant(event, "")
	}
}"""
text = text.replace(old, new)

hub.write_text(text)
print("patched hub.go")

# pubsub.go
pubsub = ws / "pubsub.go"
text = pubsub.read_text()

# Remove tenant fields from PubSubMessage; keep TargetUserID/SpaceID.
old = """type PubSubMessage struct {
	Type           string `json:"type"`
	TargetUserID   string `json:"target_user_id,omitempty"`
	TargetTenantID string `json:"target_tenant_id,omitempty"`
	TargetSpaceID  string `json:"target_space_id,omitempty"`
	// TenantBroadcast is true for a tenant-scoped broadcast (e.g. presence).
	// It distinguishes a tenant broadcast (deliver to every client in the
	// tenant) from a space-scoped delivery (TargetSpaceID set) which happens
	// to also carry TargetTenantID for tenant scoping. Without this flag a
	// tenant broadcast with no space/user target would fall through to the
	// global BroadcastToAll path and leak across tenants (Rule 4).
	TenantBroadcast bool        `json:"tenant_broadcast,omitempty"`
	Payload         interface{} `json:"payload"`
}"""
new = """type PubSubMessage struct {
	Type          string      `json:"type"`
	TargetUserID  string      `json:"target_user_id,omitempty"`
	TargetSpaceID string      `json:"target_space_id,omitempty"`
	Payload       interface{} `json:"payload"`
}"""
text = text.replace(old, new)

# Update Publish to not include TargetTenantID
text = re.sub(
    r"TargetUserID:\s+targetUserID,\n\t\tTargetTenantID:\s+targetTenantID,\n\t\tTargetSpaceID:\s+targetSpaceID,",
    "TargetUserID:  targetUserID,\n\t\tTargetSpaceID: targetSpaceID,",
    text,
)

# Update PublishTenant to not include TargetTenantID/TenantBroadcast
old = """// PublishTenant serialises a tenant-scoped broadcast event as a PubSubMessage
// and publishes it to Valkey for delivery to other instances. Receiving
// instances deliver it only to clients in the same tenant. The local hub is
// NOT called here — callers invoke BroadcastToTenant for local delivery and
// PublishTenant separately for cross-instance.
func (b *PubSubBridge) PublishTenant(event WSEvent, targetTenantID string) {
	msg := PubSubMessage{
		Type:            event.Type,
		TargetTenantID:  targetTenantID,
		TenantBroadcast: true,
		Payload:         event.Payload,
	}"""
new = """// PublishTenant serialises a broadcast event as a PubSubMessage
// and publishes it to Valkey for delivery to other instances. The local hub is
// NOT called here — callers invoke BroadcastToTenant for local delivery and
// PublishTenant separately for cross-instance.
func (b *PubSubBridge) PublishTenant(event WSEvent, _ string) {
	msg := PubSubMessage{
		Type:    event.Type,
		Payload: event.Payload,
	}"""
text = text.replace(old, new)

# Update dispatch to treat TenantBroadcast as all (fields gone, so just default)
old = """func (b *PubSubBridge) dispatch(msg PubSubMessage) {
	event := WSEvent{Type: msg.Type, Payload: msg.Payload}

	switch {
	case msg.TargetUserID != "":
		b.hub.deliverLocalUser(msg.TargetUserID, event)
	case msg.TargetSpaceID != "":
		b.hub.deliverLocalSpace(msg.TargetTenantID, msg.TargetSpaceID, event)
	case msg.TenantBroadcast:
		// Tenant-scoped broadcast (e.g. presence). Deliver only to clients
		// in the same tenant — never fall through to deliverLocalAll, which
		// would leak the event across tenants (Rule 4: tenant-scoped).
		b.hub.deliverLocalTenant(msg.TargetTenantID, event)
	default:
		b.hub.deliverLocalAll(event)
	}
}"""
new = """func (b *PubSubBridge) dispatch(msg PubSubMessage) {
	event := WSEvent{Type: msg.Type, Payload: msg.Payload}

	switch {
	case msg.TargetUserID != "":
		b.hub.deliverLocalUser(msg.TargetUserID, event)
	case msg.TargetSpaceID != "":
		b.hub.deliverLocalSpace(msg.TargetSpaceID, event)
	default:
		b.hub.deliverLocalAll(event)
	}
}"""
text = text.replace(old, new)

pubsub.write_text(text)
print("patched pubsub.go")

# handlers.go: fix BroadcastToSpace and BroadcastToTenant calls + SessionChecker + ticket TenantID
handlers = ws / "handlers.go"
text = handlers.read_text()

text = re.sub(r"h\.hub\.BroadcastToSpace\(tenantID\.String\(\),\s+", "h.hub.BroadcastToSpace(", text)
text = re.sub(r"h\.hub\.BroadcastToTenant\(tenantID\.String\(\),\s+", "h.hub.BroadcastToTenant("", ", text)
text = re.sub(r"c\.Hub\.BroadcastToTenant\(c\.TenantID,\s+", "c.Hub.BroadcastToTenant("", ", text)
text = re.sub(r"SessionChecker\(c\.UserID,\s+c\.TenantID\)", "SessionChecker(c.UserID)", text)
text = re.sub(r"tenantID\.Scan\(c\.TenantID\)", "tenantID.Scan(c.UserID)", text)
handlers.write_text(text)
print("patched handlers.go")
