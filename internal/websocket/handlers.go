package websocket

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	mw "github.com/jedafe/japa/internal/middleware"
	"github.com/jedafe/japa/internal/spam"
	"github.com/jedafe/japa/internal/valkey"
	"github.com/valkey-io/valkey-go/valkeycompat"
)

var wsUpgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	// Only upgrade connections from origins the CORS policy trusts.
	// Accepting any origin allows cross-site WebSocket hijacking, since the
	// browser attaches the caller's cookies to the upgrade request.
	CheckOrigin: func(r *http.Request) bool {
		origin := r.Header.Get("Origin")
		if origin == "" {
			// Non-browser clients (mobile apps, CLI tools) send no Origin.
			return true
		}

		return mw.IsOriginAllowed(origin)
	},
}

// Handlers holds dependencies for WebSocket and DM HTTP handlers.
type Handlers struct {
	service        *Service
	dmService      *DMService
	hub            *Hub
	vkClient       *valkey.Client
	keyRotation    *auth.KeyRotation
	spamPipeline   *spam.SpamPipeline
	sessionChecker func(userID string) bool // BUG-018
}

// NewHandlers creates Handlers with the given dependencies.
func NewHandlers(service *Service, dmService *DMService, hub *Hub, vkClient *valkey.Client, kr *auth.KeyRotation) *Handlers {
	return &Handlers{
		service:     service,
		dmService:   dmService,
		hub:         hub,
		vkClient:    vkClient,
		keyRotation: kr,
	}
}

// SetSpamPipeline injects the spam detection pipeline for DM safety checks.
func (h *Handlers) SetSpamPipeline(p *spam.SpamPipeline) {
	h.spamPipeline = p
}

// SetSessionChecker injects the session validity checker used to re-validate
// WebSocket connections on each pong (BUG-018). The checker returns false if
// the user's session has been revoked or the user has been banned.
func (h *Handlers) SetSessionChecker(checker func(userID string) bool) {
	h.sessionChecker = checker
}

// requireUser extracts the authenticated user from context.
func (h *Handlers) requireUser(r *http.Request) (*auth.AuthenticatedUser, bool) {
	user := auth.FromContext(r.Context())
	if user == nil {
		return nil, false
	}

	return user, true
}

// writeJSON serializes the given value as JSON and writes it to the response.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Warn("json encode error", "error", err)
	}
}

// writeError writes a JSON error response following the JapaConnect API format.
func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{
		"error": map[string]string{"code": code, "message": message},
	})
}

// GetWSTicket mints a short-lived WS ticket.
// POST /api/v1/auth/ws-ticket.
func (h *Handlers) GetWSTicket(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	ctx := r.Context()

	// Insert ticket into DB and get the generated ticket ID.
	ticket, err := h.service.store.CreateWSTicket(ctx, user.UserID, user.TenantID)
	if err != nil {
		slog.Error("creating ws ticket", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to create ticket")

		return
	}

	// Store in Valkey for fast lookup: jc:ws_ticket:{ticket_id} -> {user_id, tenant_id}, TTL 60s.
	// The upgrade handler resolves tickets from Valkey only, so a failed write
	// would hand the client a ticket that can never be redeemed. The tenant_id
	// is cached alongside the user_id so the upgrade handler does not fall back
	// to a hardcoded default tenant (Rule 4: Valkey keys must be tenant-scoped).
	if h.vkClient != nil {
		vctx, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()

		ticketFields := map[string]interface{}{
			"user_id":   user.UserID.String(),
			"tenant_id": user.TenantID.String(),
		}
		if err := h.vkClient.Redis().HSet(vctx, valkey.MustKey("ws_ticket:%s", ticket.ID.String()),
			ticketFields, 60*time.Second).Err(); err != nil {
			slog.Error("caching ws ticket", "error", err, "user_id", user.UserID.String())
			writeError(w, http.StatusInternalServerError, "internal_error", "Failed to create ticket")

			return
		}
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"ticket": ticket.ID.String(),
	})
}

// HandleWebSocket upgrades connection and authenticates via ticket.
// GET /api/v1/ws?ticket={ticket_id}.
func (h *Handlers) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	ticketStr := r.URL.Query().Get("ticket")
	if ticketStr == "" {
		writeError(w, http.StatusBadRequest, "missing_ticket", "Ticket query parameter is required")
		return
	}

	var ticketID pgtype.UUID
	if err := ticketID.Scan(ticketStr); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_ticket", "Invalid ticket format")
		return
	}

	ctx := r.Context()

	var userID pgtype.UUID

	var tenantID pgtype.UUID

	// Try Valkey first for fast lookup.
	if h.vkClient != nil {
		vctx, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()

		fields, err := h.vkClient.Redis().HGetAll(vctx, valkey.MustKey("ws_ticket:%s", ticketStr)).Result()

		switch {
		case errors.Is(err, valkeycompat.Nil), len(fields) == 0:
			// Unknown or expired ticket — handled by the !userID.Valid check below.
		case err != nil:
			slog.Error("looking up ws ticket", "error", err)
		default:
			if val, ok := fields["user_id"]; ok {
				if scanErr := userID.Scan(val); scanErr != nil {
					slog.Error("cached ws ticket holds an invalid user id", "error", scanErr, "ticket", ticketStr)
				}
			}
			if val, ok := fields["tenant_id"]; ok {
				if scanErr := tenantID.Scan(val); scanErr != nil {
					slog.Error("cached ws ticket holds an invalid tenant id", "error", scanErr, "ticket", ticketStr)
				}
			}
		}
	}

	// Fallback to DB if Valkey didn't have it. This resolves the user_id and
	// tenant_id from the persistent ticket record so a Valkey eviction does not
	// make a valid ticket unredeemable.
	if !userID.Valid {
		ticket, err := h.service.store.GetWSTicketForRedemption(ctx, ticketID)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "invalid_ticket", "Ticket not found or expired")
			return
		}
		userID = ticket.UserID
		tenantID = ticket.TenantID
	}

	// Mark ticket as used in DB, validating tenant_id alongside user_id so a
	// ticket minted in tenant A cannot be consumed in tenant B (BUG-019).
	if _, err := h.service.store.UseWSTicket(ctx, ticketID, userID, tenantID); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to consume ticket")
		return
	}

	// Immediately delete the Valkey key so a replayed request cannot pass the cache lookup.
	if h.vkClient != nil {
		if err := h.vkClient.Redis().Del(ctx, valkey.MustKey("ws_ticket:%s", ticketStr)).Err(); err != nil {
			slog.Error("deleting consumed ws ticket", "error", err, "ticket", ticketStr)
		}
	}

	// Upgrade to WebSocket.
	conn, err := wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Error("websocket upgrade failed", "error", err)
		return
	}

	// Create Client.
	client := &Client{
		Hub:            h.hub,
		Conn:           conn,
		UserID:         userID.String(),
		TenantID:       tenantID.String(),
		Send:           make(chan []byte, 256),
		SpaceIDs:       make(map[string]bool),
		SessionChecker: h.sessionChecker, // BUG-018
	}

	// Register with hub.
	h.hub.Register(client)

	// Send presence event to all.
	h.hub.BroadcastToAll(WSEvent{
		Type:    EventPresence,
		Payload: map[string]interface{}{"user_id": userID.String(), "status": "online"},
	})

	// Start read/write pumps.
	go client.writePump()
	go client.readPump(h.dmService)
}

// GetOnlineUsers returns currently online users.
// GET /api/v1/ws/online.
func (h *Handlers) GetOnlineUsers(w http.ResponseWriter, _ *http.Request) {
	users := h.hub.GetOnlineUsers()
	writeJSON(w, http.StatusOK, map[string]any{"data": users})
}

// readPump reads messages from the WebSocket connection.
func (c *Client) readPump(_ *DMService) {
	defer func() {
		c.Hub.Unregister(c)
		_ = c.Conn.Close()
	}()

	c.Conn.SetReadLimit(4096)
	_ = c.Conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	c.Conn.SetPongHandler(func(string) error {
		_ = c.Conn.SetReadDeadline(time.Now().Add(60 * time.Second))

		// BUG-018: re-validate the session on each pong (every 30-60s).
		// If the user has been banned or their sessions revoked mid-connection,
		// close the socket so it does not stay live with stale credentials.
		if c.SessionChecker != nil && !c.SessionChecker(c.UserID) {
			slog.Info("ws session invalidated, closing connection", "user_id", c.UserID)
			return websocket.ErrCloseSent
		}

		return nil
	})

	for {
		_, message, err := c.Conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				slog.Error("ws read error", "error", err)
			}

			break
		}

		var event WSEvent
		if err := json.Unmarshal(message, &event); err != nil {
			slog.Error("ws unmarshal event", "error", err)
			continue
		}

		// Handle client events.
		switch event.Type {
		case "subscribe_space":
			var payload struct {
				SpaceID string `json:"space_id"`
			}

			if data, ok := event.Payload.(map[string]interface{}); ok {
				if err := json.Unmarshal(mustMarshal(data), &payload); err == nil {
					c.Hub.SubscribeToSpace(c, payload.SpaceID)
				}
			}
		case "unsubscribe_space":
			var payload struct {
				SpaceID string `json:"space_id"`
			}

			if data, ok := event.Payload.(map[string]interface{}); ok {
				if err := json.Unmarshal(mustMarshal(data), &payload); err == nil {
					c.Hub.UnsubscribeFromSpace(c, payload.SpaceID)
				}
			}
		}
	}
}

// writePump writes messages from the send channel to the WebSocket connection.
func (c *Client) writePump() {
	ticker := time.NewTicker(30 * time.Second)
	defer func() {
		ticker.Stop()

		_ = c.Conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.Send:
			_ = c.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if !ok {
				_ = c.Conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			w, err := c.Conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}

			_, _ = w.Write(message)

			// Drain queued messages into current write.
			n := len(c.Send)
			for range n {
				_, _ = w.Write([]byte("\n"))
				_, _ = w.Write(<-c.Send)
			}

			if err := w.Close(); err != nil {
				return
			}

		case <-ticker.C:
			_ = c.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second))

			if err := c.Conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// generateUUID generates a random UUID string.
func generateUUID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40 // Version 4
	b[8] = (b[8] & 0x3f) | 0x80 // Variant 10

	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// mustMarshal marshals v to JSON, returning empty byte slice on error.
func mustMarshal(v interface{}) []byte {
	b, _ := json.Marshal(v)
	return b
}
