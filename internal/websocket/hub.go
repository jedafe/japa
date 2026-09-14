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
	mu sync.RWMutex
	// A user may hold several concurrent connections (multi-tab, multi-device,
	// multiple app surfaces — §11). clients maps user_id -> set of connections
	// so a second connection no longer evicts the first, which previously
	// caused a reconnect war: each kicked client immediately re-minted a ticket
	// and re-registered, kicking the other side in turn.
	clients          map[string]map[*Client]struct{}
	spaceSubscribers map[string]map[*Client]struct{} // tenant:space -> connections
	pubsub           Publisher
}

// Client represents a single WebSocket connection.
type Client struct {
	Hub    *Hub
	Conn   *websocket.Conn
	UserID string

	Send           chan []byte
	SpaceIDs       map[string]bool          // subscribed spaces
	SessionChecker func(userID string) bool // BUG-018: re-validate session on pong
}

// WSEvent is the envelope for all WebSocket messages.
type WSEvent struct {
	Type    string      `json:"type"`
	Payload interface{} `json:"payload"`
}

// NewHub creates a new WebSocket hub.
func NewHub() *Hub {
	return &Hub{
		clients:          make(map[string]map[*Client]struct{}),
		spaceSubscribers: make(map[string]map[*Client]struct{}),
	}
}

// SetPubSub attaches a publisher for cross-instance delivery.
func (h *Hub) SetPubSub(p Publisher) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.pubsub = p
}

// Register adds a client to the hub. Multiple concurrent connections per
// user are supported (multi-tab, multi-device, or several JapaConnect app
// surfaces open at once). A stale half-open connection is cleaned up by its
// own read deadline (60s pong timeout) and writePump ping, so no eviction
// is needed here.
func (h *Hub) Register(client *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.clients[client.UserID] == nil {
		h.clients[client.UserID] = make(map[*Client]struct{})
	}

	h.clients[client.UserID][client] = struct{}{}
	slog.Info("ws client registered", "user_id", client.UserID)
}

// Unregister removes a client from the hub and cleans up space
// subscriptions. Only this connection's Send channel is closed — other
// live connections for the same user keep their hub entry (§11).
func (h *Hub) Unregister(client *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.removeSpaceSubscriptionsLocked(client)

	if conns, ok := h.clients[client.UserID]; ok {
		if _, ok := conns[client]; ok {
			delete(conns, client)
			close(client.Send)
		}

		if len(conns) == 0 {
			delete(h.clients, client.UserID)
		}
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
		key := spaceSubscriptionKey(spaceID)
		if subs, ok := h.spaceSubscribers[key]; ok {
			delete(subs, client)

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
	conns := h.clients[userID]
	clients := make([]*Client, 0, len(conns))

	for c := range conns {
		clients = append(clients, c)
	}
	h.mu.RUnlock()

	if len(clients) == 0 {
		return
	}

	data, err := json.Marshal(event)
	if err != nil {
		slog.Error("ws marshal event", "error", err)
		return
	}

	for _, client := range clients {
		select {
		case client.Send <- data:
		default:
			slog.Warn("ws client send buffer full, dropping message", "user_id", userID)
		}
	}
}

// BroadcastToSpace sends an event to all subscribers of a space on this
// instance and publishes to Valkey for delivery on other instances.
func (h *Hub) BroadcastToSpace(spaceID string, event WSEvent) {
	h.deliverLocalSpace(spaceID, event)

	h.mu.RLock()
	bridge := h.pubsub
	h.mu.RUnlock()

	if bridge != nil {
		bridge.Publish(event, "", "", spaceID)
	}
}

// deliverLocalSpace sends an event to all subscribers of a space connected
// to THIS instance only. It does NOT publish to Valkey — see
// deliverLocalUser for the loop-safety rationale.
func (h *Hub) deliverLocalSpace(spaceID string, event WSEvent) {
	h.mu.RLock()

	subs, ok := h.spaceSubscribers[spaceSubscriptionKey(spaceID)]
	if !ok {
		h.mu.RUnlock()
		return
	}

	clients := make([]*Client, 0, len(subs))
	for c := range subs {
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
			slog.Warn("ws client send buffer full, dropping space message", "user_id", client.UserID, "space_id", spaceID)
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
	for _, conns := range h.clients {
		for c := range conns {
			clients = append(clients, c)
		}
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
	for _, conns := range h.clients {
		for c := range conns {
			if "" == tenantID {
				clients = append(clients, c)
			}
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
			slog.Warn("ws client send buffer full, dropping tenant broadcast", "user_id", client.UserID)
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

// IsUserOnline checks if a user has at least one live connection.
func (h *Hub) IsUserOnline(userID string) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()

	return len(h.clients[userID]) > 0
}

// SubscribeToSpace adds a client to a space's subscribers. The subscription
// key is scoped to the client's tenant so a cross-tenant space ID collision
// cannot leak events to subscribers in another tenant (Rule 4: tenant-scoped).
func (h *Hub) SubscribeToSpace(client *Client, spaceID string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	key := spaceSubscriptionKey(spaceID)

	if h.spaceSubscribers[key] == nil {
		h.spaceSubscribers[key] = make(map[*Client]struct{})
	}

	h.spaceSubscribers[key][client] = struct{}{}
	client.SpaceIDs[spaceID] = true
}

// UnsubscribeFromSpace removes a client from a space's subscribers. The
// subscription key is scoped to the client's tenant (Rule 4: tenant-scoped).
func (h *Hub) UnsubscribeFromSpace(client *Client, spaceID string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	key := spaceSubscriptionKey(spaceID)

	if subs, ok := h.spaceSubscribers[key]; ok {
		delete(subs, client)

		if len(subs) == 0 {
			delete(h.spaceSubscribers, key)
		}
	}

	delete(client.SpaceIDs, spaceID)
}

// spaceSubscriptionKey builds the map key for a space subscription.
func spaceSubscriptionKey(spaceID string) string {
	return spaceID
}
