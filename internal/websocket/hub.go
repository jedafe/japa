package websocket

import (
	"encoding/json"
	"log/slog"
	"sync"

	"github.com/gorilla/websocket"
)

// Publisher is the interface for cross-instance message delivery.
// PubSubBridge implements this using Valkey pub/sub. The targetTenantID
// scopes space delivery so cross-tenant space ID collisions cannot leak
// events on other instances (Rule 4: tenant-scoped).
type Publisher interface {
	Publish(event WSEvent, targetUserID, targetTenantID, targetSpaceID string)
	// PublishTenant publishes a tenant-scoped broadcast so only instances
	// with clients in the same tenant deliver it locally. Used for presence
	// and other tenant-wide events that must not cross tenant boundaries
	// (Rule 4: tenant-scoped, §5.3 cross-tenant leak prevention).
	PublishTenant(event WSEvent, targetTenantID string)
}

// Hub manages WebSocket connections and broadcasts.
type Hub struct {
	mu               sync.RWMutex
	clients          map[string]*Client            // user_id -> Client
	spaceSubscribers map[string]map[string]*Client // space_id -> user_id -> Client
	pubsub           Publisher
}

// Client represents a single WebSocket connection.
type Client struct {
	Hub            *Hub
	Conn           *websocket.Conn
	UserID         string
	TenantID       string
	Send           chan []byte
	SpaceIDs       map[string]bool                    // subscribed spaces
	SessionChecker func(userID, tenantID string) bool // BUG-018: re-validate session on pong
}

// WSEvent is the envelope for all WebSocket messages.
type WSEvent struct {
	Type    string      `json:"type"`
	Payload interface{} `json:"payload"`
}

// NewHub creates a new WebSocket hub.
func NewHub() *Hub {
	return &Hub{
		clients:          make(map[string]*Client),
		spaceSubscribers: make(map[string]map[string]*Client),
	}
}

// SetPubSub attaches a publisher for cross-instance delivery.
func (h *Hub) SetPubSub(p Publisher) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.pubsub = p
}

// Register adds a client to the hub. If an existing client for the same
// user is already registered (e.g. a reconnect or multi-tab close race),
// its Send channel is closed so its writePump exits and its readPump
// eventually breaks. Without this, the old connection's goroutines leak
// and its later Unregister would delete the new client from the hub
// (§4.3 memory/connection leak, §11 connection not cleaned on close).
func (h *Hub) Register(client *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if existing, ok := h.clients[client.UserID]; ok && existing != client {
		h.removeSpaceSubscriptionsLocked(existing)
		close(existing.Send)
		delete(h.clients, client.UserID)
	}

	h.clients[client.UserID] = client
	slog.Info("ws client registered", "user_id", client.UserID)
}

// Unregister removes a client from the hub and cleans up space
// subscriptions. Only deletes the hub map entry and closes the Send
// channel if this client is still the one registered for the user — a
// newer connection may have replaced it via Register, in which case
// evicting would drop the live connection (§11 connection leak).
func (h *Hub) Unregister(client *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.removeSpaceSubscriptionsLocked(client)

	if current, ok := h.clients[client.UserID]; ok && current == client {
		delete(h.clients, client.UserID)
		close(client.Send)
	}

	slog.Info("ws client unregistered", "user_id", client.UserID)
}

// removeSpaceSubscriptionsLocked removes the client from every space
// subscription map it belongs to. The key is tenant-scoped
// (spaceSubscriptionKey) so it matches what SubscribeToSpace stores —
// the previous Unregister used the bare spaceID and never matched,
// leaking every space subscription on disconnect (§4.3 memory leak).
// Caller must hold h.mu.
func (h *Hub) removeSpaceSubscriptionsLocked(client *Client) {
	for spaceID := range client.SpaceIDs {
		key := spaceSubscriptionKey(client.TenantID, spaceID)
		if subs, ok := h.spaceSubscribers[key]; ok {
			delete(subs, client.UserID)

			if len(subs) == 0 {
				delete(h.spaceSubscribers, key)
			}
		}
	}
}

// BroadcastToUser sends an event to a specific user on this instance and
// publishes to Valkey for delivery on other instances.
func (h *Hub) BroadcastToUser(userID string, event WSEvent) {
	h.deliverLocalUser(userID, event)

	h.mu.RLock()
	bridge := h.pubsub
	h.mu.RUnlock()

	if bridge != nil {
		bridge.Publish(event, userID, "", "")
	}
}

// deliverLocalUser sends an event to a specific user connected to THIS
// instance only. It does NOT publish to Valkey, so it is safe to call from
// the pubsub subscription goroutine (which must not re-publish, or every
// received message would fan out again and create an infinite loop).
func (h *Hub) deliverLocalUser(userID string, event WSEvent) {
	h.mu.RLock()
	client, ok := h.clients[userID]
	h.mu.RUnlock()

	if !ok {
		return
	}

	data, err := json.Marshal(event)
	if err != nil {
		slog.Error("ws marshal event", "error", err)
		return
	}
	select {
	case client.Send <- data:
	default:
		slog.Warn("ws client send buffer full, dropping message", "user_id", userID)
	}
}

// BroadcastToSpace sends an event to all subscribers of a space on this
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
}

// deliverLocalSpace sends an event to all subscribers of a space connected
// to THIS instance only. It does NOT publish to Valkey — see
// deliverLocalUser for the loop-safety rationale. The subscription key is
// scoped to tenantID (Rule 4: tenant-scoped).
func (h *Hub) deliverLocalSpace(tenantID, spaceID string, event WSEvent) {
	h.mu.RLock()

	subs, ok := h.spaceSubscribers[spaceSubscriptionKey(tenantID, spaceID)]
	if !ok {
		h.mu.RUnlock()
		return
	}

	clients := make([]*Client, 0, len(subs))
	for _, c := range subs {
		clients = append(clients, c)
	}
	h.mu.RUnlock()

	data, err := json.Marshal(event)
	if err != nil {
		slog.Error("ws marshal space event", "error", err)
		return
	}

	for _, client := range clients {
		select {
		case client.Send <- data:
		default:
			slog.Warn("ws client send buffer full, dropping space message",
				"user_id", client.UserID, "space_id", spaceID)
		}
	}
}

// BroadcastToAll sends an event to all connected clients on this instance
// and publishes to Valkey for delivery on other instances.
func (h *Hub) BroadcastToAll(event WSEvent) {
	h.deliverLocalAll(event)

	h.mu.RLock()
	bridge := h.pubsub
	h.mu.RUnlock()

	if bridge != nil {
		bridge.Publish(event, "", "", "")
	}
}

// BroadcastToTenant sends an event to every connected client in a specific
// tenant on this instance and publishes to Valkey for delivery on other
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
}

// deliverLocalAll sends an event to every client connected to THIS instance
// only. It does NOT publish to Valkey — see deliverLocalUser for the
// loop-safety rationale.
func (h *Hub) deliverLocalAll(event WSEvent) {
	h.mu.RLock()

	clients := make([]*Client, 0, len(h.clients))
	for _, c := range h.clients {
		clients = append(clients, c)
	}
	h.mu.RUnlock()

	data, err := json.Marshal(event)
	if err != nil {
		slog.Error("ws marshal broadcast event", "error", err)
		return
	}

	for _, client := range clients {
		select {
		case client.Send <- data:
		default:
			slog.Warn("ws client send buffer full, dropping broadcast", "user_id", client.UserID)
		}
	}
}

// deliverLocalTenant sends an event to every client connected to THIS instance
// whose TenantID matches, only. It does NOT publish to Valkey — see
// deliverLocalUser for the loop-safety rationale. This is the tenant-scoped
// counterpart of deliverLocalAll, used for presence so a user in tenant A
// never receives presence events from tenant B (Rule 4: tenant-scoped).
func (h *Hub) deliverLocalTenant(tenantID string, event WSEvent) {
	h.mu.RLock()

	clients := make([]*Client, 0, len(h.clients))
	for _, c := range h.clients {
		if c.TenantID == tenantID {
			clients = append(clients, c)
		}
	}
	h.mu.RUnlock()

	data, err := json.Marshal(event)
	if err != nil {
		slog.Error("ws marshal tenant broadcast event", "error", err)
		return
	}

	for _, client := range clients {
		select {
		case client.Send <- data:
		default:
			slog.Warn("ws client send buffer full, dropping tenant broadcast",
				"user_id", client.UserID, "tenant_id", tenantID)
		}
	}
}

// GetOnlineUsers returns list of online user IDs.
func (h *Hub) GetOnlineUsers() []string {
	h.mu.RLock()
	defer h.mu.RUnlock()

	users := make([]string, 0, len(h.clients))
	for userID := range h.clients {
		users = append(users, userID)
	}

	return users
}

// IsUserOnline checks if a user is connected.
func (h *Hub) IsUserOnline(userID string) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	_, ok := h.clients[userID]

	return ok
}

// SubscribeToSpace adds a client to a space's subscribers. The subscription
// key is scoped to the client's tenant so a cross-tenant space ID collision
// cannot leak events to subscribers in another tenant (Rule 4: tenant-scoped).
func (h *Hub) SubscribeToSpace(client *Client, spaceID string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	key := spaceSubscriptionKey(client.TenantID, spaceID)

	if h.spaceSubscribers[key] == nil {
		h.spaceSubscribers[key] = make(map[string]*Client)
	}

	h.spaceSubscribers[key][client.UserID] = client
	client.SpaceIDs[spaceID] = true
}

// UnsubscribeFromSpace removes a client from a space's subscribers. The
// subscription key is scoped to the client's tenant (Rule 4: tenant-scoped).
func (h *Hub) UnsubscribeFromSpace(client *Client, spaceID string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	key := spaceSubscriptionKey(client.TenantID, spaceID)

	if subs, ok := h.spaceSubscribers[key]; ok {
		delete(subs, client.UserID)

		if len(subs) == 0 {
			delete(h.spaceSubscribers, key)
		}
	}

	delete(client.SpaceIDs, spaceID)
}

// spaceSubscriptionKey builds the map key for a space subscription, scoped by
// tenant_id so cross-tenant space ID collisions cannot leak events.
func spaceSubscriptionKey(tenantID, spaceID string) string {
	return tenantID + ":" + spaceID
}
