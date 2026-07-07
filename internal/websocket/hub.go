package websocket

import (
	"encoding/json"
	"log/slog"
	"sync"

	"github.com/gorilla/websocket"
)

// Publisher is the interface for cross-instance message delivery.
// PubSubBridge implements this using Valkey pub/sub.
type Publisher interface {
	Publish(event WSEvent, targetUserID, targetSpaceID string)
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
	Hub      *Hub
	Conn     *websocket.Conn
	UserID   string
	TenantID string
	Send     chan []byte
	SpaceIDs map[string]bool // subscribed spaces
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

// Register adds a client to the hub.
func (h *Hub) Register(client *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.clients[client.UserID] = client
	slog.Info("ws client registered", "user_id", client.UserID)
}

// Unregister removes a client from the hub and cleans up space subscriptions.
func (h *Hub) Unregister(client *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()

	// Remove from all space subscriptions.
	for spaceID := range client.SpaceIDs {
		if subs, ok := h.spaceSubscribers[spaceID]; ok {
			delete(subs, client.UserID)

			if len(subs) == 0 {
				delete(h.spaceSubscribers, spaceID)
			}
		}
	}

	delete(h.clients, client.UserID)
	close(client.Send)
	slog.Info("ws client unregistered", "user_id", client.UserID)
}

// BroadcastToUser sends an event to a specific user on this instance and
// publishes to Valkey for delivery on other instances.
func (h *Hub) BroadcastToUser(userID string, event WSEvent) {
	h.mu.RLock()
	client, ok := h.clients[userID]
	bridge := h.pubsub
	h.mu.RUnlock()

	if ok {
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

	if bridge != nil {
		bridge.Publish(event, userID, "")
	}
}

// BroadcastToSpace sends an event to all subscribers of a space on this
// instance and publishes to Valkey for delivery on other instances.
func (h *Hub) BroadcastToSpace(spaceID string, event WSEvent) {
	h.mu.RLock()
	subs, ok := h.spaceSubscribers[spaceID]
	bridge := h.pubsub

	if ok {
		clients := make([]*Client, 0, len(subs))
		for _, c := range subs {
			clients = append(clients, c)
		}
		h.mu.RUnlock()

		data, err := json.Marshal(event)
		if err != nil {
			slog.Error("ws marshal space event", "error", err)
			// Still publish to bridge for cross-instance delivery.
			if bridge != nil {
				bridge.Publish(event, "", spaceID)
			}
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
	} else {
		h.mu.RUnlock()
	}

	if bridge != nil {
		bridge.Publish(event, "", spaceID)
	}
}

// BroadcastToAll sends an event to all connected clients on this instance
// and publishes to Valkey for delivery on other instances.
func (h *Hub) BroadcastToAll(event WSEvent) {
	h.mu.RLock()

	clients := make([]*Client, 0, len(h.clients))
	for _, c := range h.clients {
		clients = append(clients, c)
	}

	bridge := h.pubsub
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

	if bridge != nil {
		bridge.Publish(event, "", "")
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

// SubscribeToSpace adds a client to a space's subscribers.
func (h *Hub) SubscribeToSpace(client *Client, spaceID string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.spaceSubscribers[spaceID] == nil {
		h.spaceSubscribers[spaceID] = make(map[string]*Client)
	}

	h.spaceSubscribers[spaceID][client.UserID] = client
	client.SpaceIDs[spaceID] = true
}

// UnsubscribeFromSpace removes a client from a space's subscribers.
func (h *Hub) UnsubscribeFromSpace(client *Client, spaceID string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if subs, ok := h.spaceSubscribers[spaceID]; ok {
		delete(subs, client.UserID)

		if len(subs) == 0 {
			delete(h.spaceSubscribers, spaceID)
		}
	}

	delete(client.SpaceIDs, spaceID)
}
