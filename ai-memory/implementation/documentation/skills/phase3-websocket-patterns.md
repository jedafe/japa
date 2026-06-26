# Phase 3 WebSocket Patterns

**Date:** 2026-06-26
**Phase:** 3
**Purpose:** Implementation patterns learned during Section 10 (Real-Time WebSockets + Direct Messaging), reusable in future sections.

## Pattern: WebSocket Ticket Auth (Short-Lived, Single-Use, Valkey+DB)

WebSocket connections cannot carry custom HTTP headers (the `Upgrade` request is
limited by browsers), so a short-lived ticket passed as a query parameter is used
instead. The ticket is minted via a standard REST endpoint, stored in both
PostgreSQL and Valkey, and consumed once during the WebSocket upgrade:

```go
// Mint ticket — stores in DB with 60s TTL, caches in Valkey for fast lookup.
func (h *Handlers) GetWSTicket(w http.ResponseWriter, r *http.Request) {
    // ... authenticate user ...

    // Insert ticket into DB.
    if err := h.service.store.CreateWSTicket(ctx, user.UserID, user.TenantID); err != nil {
        // ... handle error ...
    }

    // Store in Valkey for fast lookup: jc:ws_ticket:{ticket_id} -> user_id, TTL 60s
    if h.vkClient != nil {
        vctx, cancel := context.WithTimeout(ctx, 2*time.Second)
        defer cancel()
        _ = h.vkClient.Redis().Set(vctx, "jc:ws_ticket:"+ticketID.String(), user.UserID.String(), 60*time.Second).Err()
    }

    writeJSON(w, http.StatusOK, map[string]string{"ticket": ticketID.String()})
}
```

The SQL for creating the ticket uses `now() + interval '60 seconds'` for the TTL:

```sql
-- name: CreateWSTicket :exec
INSERT INTO ws_tickets (user_id, tenant_id, expires_at)
VALUES (@user_id, @tenant_id, now() + interval '60 seconds');
```

The `UseWSTicket` query validates the ticket is unused and not expired:

```sql
-- name: UseWSTicket :one
UPDATE ws_tickets SET used = true
WHERE id = @id AND user_id = @user_id AND used = false AND expires_at > now()
RETURNING *;
```

**Pattern for adding ticket auth to new endpoints:** Mint the ticket in a REST
handler, store in both DB and Valkey, pass as query parameter during upgrade.
The Valkey key format `jc:{feature}:{id}` is consistent across the codebase.

## Pattern: In-Memory Hub (Client Registry, Space Subscriptions, Broadcast)

The Hub is the central registry for all active WebSocket connections. It uses
two maps protected by a single `sync.RWMutex`: one for direct user lookups,
one for space-scoped subscriptions:

```go
type Hub struct {
    mu               sync.RWMutex
    clients          map[string]*Client       // user_id -> Client
    spaceSubscribers map[string]map[string]*Client // space_id -> user_id -> Client
}
```

**Registration and cleanup:** `Register` adds a client; `Unregister` removes it
and cleans up all space subscriptions atomically:

```go
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
}
```

**Copy-then-send for broadcasts:** `BroadcastToSpace` and `BroadcastToAll` copy
the client map under read lock before sending, avoiding holding the lock during
potentially slow WebSocket writes:

```go
func (h *Hub) BroadcastToSpace(spaceID string, event WSEvent) {
    h.mu.RLock()
    subs, ok := h.spaceSubscribers[spaceID]
    if !ok {
        h.mu.RUnlock()
        return
    }
    // Copy the map under lock to avoid holding it during sends.
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
```

**Non-blocking sends:** All broadcast methods use `select/default` to avoid
blocking on slow clients. If a client's send buffer (256 messages) is full,
the message is dropped with a warning log:

```go
select {
case client.Send <- data:
default:
    // Client send buffer full, drop message.
    slog.Warn("ws client send buffer full, dropping message", "user_id", userID)
}
```

**Pattern for adding new broadcast scopes:** Add a new map to the Hub (e.g.,
`dmSubscribers map[string]map[string]*Client` for DM threads), add
`Subscribe`/`Unsubscribe` methods, and follow the copy-then-send pattern.

## Pattern: DM Conversation Ordering (LEAST/GREATEST)

DM conversations must be unique between any two users. The canonical ordering
enforces `participant_1_id < participant_2_id` using PostgreSQL's `LEAST`/`GREATEST`
functions in the upsert query:

```sql
-- name: GetOrCreateConversation :one
INSERT INTO dm_conversations (tenant_id, participant_1_id, participant_2_id)
VALUES (@tenant_id,
    LEAST(@participant_1_id, @participant_2_id),
    GREATEST(@participant_1_id, @participant_2_id))
ON CONFLICT (tenant_id, participant_1_id, participant_2_id) DO UPDATE
SET updated_at = now()
RETURNING *;
```

The CHECK constraint in the migration enforces this at the database level:

```sql
CHECK (participant_1_id < participant_2_id) -- enforce ordering to prevent duplicates
```

**Pattern for bidirectional relationships:** Any table representing a relationship
between two entities of the same type should use this `LEAST`/`GREATEST` pattern
with a CHECK constraint. This prevents duplicate rows regardless of which user
initiates the relationship.

## Pattern: Message Request Gating (Non-Mutual-Follow Check)

When a user with `followers_only` privacy receives a DM from someone who doesn't
follow them, a message request is created with `pending` status. The recipient must
accept/decline before messages flow:

```sql
-- name: CreateMessageRequest :exec
INSERT INTO dm_message_requests (conversation_id, status)
VALUES (@conversation_id, 'pending')
ON CONFLICT (conversation_id) DO NOTHING;
```

The accept/decline handlers update the status:

```go
func (s *DMService) HandleMessageRequest(ctx context.Context, tenantID, conversationID, userID pgtype.UUID, action string) error {
    // Verify the conversation exists and user is a participant.
    conv, err := s.store.GetConversationByID(ctx, conversationID, tenantID)
    if err != nil {
        return fmt.Errorf("verifying conversation: %w", err)
    }
    if conv.Participant1ID != userID && conv.Participant2ID != userID {
        return fmt.Errorf("user is not a participant in this conversation")
    }

    switch action {
    case MessageRequestAccepted:
        return s.store.UpdateMessageRequestStatus(ctx, conversationID, MessageRequestAccepted)
    case MessageRequestDeclined:
        return s.store.UpdateMessageRequestStatus(ctx, conversationID, MessageRequestDeclined)
    default:
        return fmt.Errorf("invalid action: %s", action)
    }
}
```

**Pattern for gating access based on relationships:** Check the relationship
(follow, friend, membership) before allowing the operation. If the relationship
doesn't exist, create a pending request that the target can accept/decline.
Store the request status in a separate table with a unique constraint on the
conversation/relationship ID.

## Pattern: DM Privacy Settings (Everyone/Followers-Only/Nobody)

Per-user DM privacy is stored in a separate table with three possible values:

```sql
CREATE TABLE user_dm_settings (
    user_id     uuid PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    dm_privacy  text NOT NULL DEFAULT 'everyone',
    updated_at  timestamptz NOT NULL DEFAULT now()
);
```

The privacy setting is checked before allowing DM operations. `GetUserDMPrivacy`
uses `COALESCE` to return `everyone` if no row exists:

```sql
-- name: GetUserDMPrivacy :one
SELECT COALESCE(dm_privacy, 'everyone') AS dm_privacy FROM user_dm_settings WHERE user_id = @user_id;
```

The upsert ensures idempotent updates:

```sql
-- name: UpsertUserDMPrivacy :exec
INSERT INTO user_dm_settings (user_id, dm_privacy, updated_at)
VALUES (@user_id, @dm_privacy, now())
ON CONFLICT (user_id) DO UPDATE SET dm_privacy = EXCLUDED.dm_privacy, updated_at = now();
```

**Pattern for per-user settings:** Use a separate settings table with the user
ID as primary key. Use `COALESCE` in the getter to provide defaults without
requiring explicit inserts. Use `ON CONFLICT DO UPDATE` for idempotent upserts.

## Pattern: Unread Count Management (Per-Participant Increment/Reset)

Unread counts are stored directly on the conversation row, with separate columns
per participant. Increment and reset use `CASE` expressions to target the correct
column:

```sql
-- name: IncrementUnreadCount :exec
UPDATE dm_conversations
SET p1_unread_count = CASE WHEN participant_1_id = @user_id THEN p1_unread_count + 1 ELSE p1_unread_count END,
    p2_unread_count = CASE WHEN participant_2_id = @user_id THEN p2_unread_count + 1 ELSE p2_unread_count END
WHERE id = @conversation_id;

-- name: ResetUnreadCount :exec
UPDATE dm_conversations
SET p1_unread_count = CASE WHEN participant_1_id = @user_id THEN 0 ELSE p1_unread_count END,
    p2_unread_count = CASE WHEN participant_2_id = @user_id THEN 0 ELSE p2_unread_count END
WHERE id = @conversation_id;
```

The conversation list query resolves the correct unread count based on which
participant the current user is:

```sql
(CASE WHEN dc.participant_1_id = @user_id THEN dc.p1_unread_count ELSE dc.p2_unread_count END)::int AS unread_count
```

**Pattern for per-entity counters:** Store counters directly on the parent row
when there are a fixed number of counters (2 participants). Use `CASE` expressions
to increment/reset the correct column. Resolve the correct value at read time
based on the current user's role.

## Pattern: Read/Write Pump Pattern (Ping/Pong Keepalive)

Each WebSocket connection runs two goroutines: `readPump` (client → server) and
`writePump` (server → client). The read pump sets a read deadline and pong handler;
the write pump sends periodic pings:

```go
func (c *Client) readPump(dmSvc *DMService) {
    defer func() {
        c.Hub.Unregister(c)
        c.Conn.Close()
    }()

    c.Conn.SetReadLimit(4096)
    _ = c.Conn.SetReadDeadline(time.Now().Add(60 * time.Second))
    c.Conn.SetPongHandler(func(string) error {
        _ = c.Conn.SetReadDeadline(time.Now().Add(60 * time.Second))
        return nil
    })

    for {
        _, message, err := c.Conn.ReadMessage()
        if err != nil {
            break
        }
        // ... handle message ...
    }
}

func (c *Client) writePump() {
    ticker := time.NewTicker(30 * time.Second)
    defer func() {
        ticker.Stop()
        c.Conn.Close()
    }()

    for {
        select {
        case message, ok := <-c.Send:
            _ = c.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
            if !ok {
                _ = c.Conn.WriteMessage(websocket.CloseMessage, []byte{})
                return
            }
            // ... write message, drain queue ...
        case <-ticker.C:
            _ = c.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
            if err := c.Conn.WriteMessage(websocket.PingMessage, nil); err != nil {
                return
            }
        }
    }
}
```

Key details:
- **Read limit:** 4096 bytes prevents memory exhaustion from large messages
- **Read deadline:** 60 seconds; reset on every pong received
- **Write deadline:** 10 seconds per write operation
- **Ping interval:** 30 seconds (half the read deadline)
- **Queue draining:** Multiple queued messages are batched into a single write
  with newline separators

**Pattern for new WebSocket features:** Add new event types to the `WSEvent.Type`
field, handle them in `readPump` for client-to-server events, and produce them
from service methods for server-to-client events. The pump pattern is
infrastructure — don't modify it unless the keepalive timing needs adjustment.

## Lesson: Non-Blocking Sends Prevent Slow Client Degradation

The `select/default` pattern in all broadcast methods is critical. Without it,
a single slow client (e.g., a mobile device on a poor connection) could block
the entire hub, preventing messages from reaching all other clients:

```go
// CORRECT — non-blocking, drops message if buffer full
select {
case client.Send <- data:
default:
    slog.Warn("ws client send buffer full, dropping message", "user_id", userID)
}

// WRONG — blocks if buffer full, degrading all clients
client.Send <- data
```

The 256-message buffer provides headroom for burst traffic. Messages are dropped
only when the client consistently cannot keep up — this is the correct behavior
for a real-time system where stale messages are worse than dropped messages.

## Lesson: Copy-Then-Send Avoids Lock Contention

Holding the read lock during WebSocket sends creates contention: a slow send
blocks all other hub operations (register, unregister, broadcast). The
copy-then-send pattern reads the client list under lock, releases the lock,
then sends without holding any lock:

```go
// CORRECT — copy under lock, send without lock
h.mu.RLock()
clients := make([]*Client, 0, len(subs))
for _, c := range subs {
    clients = append(clients, c)
}
h.mu.RUnlock()

for _, client := range clients {
    select {
    case client.Send <- data:
    default:
        // ...
    }
}

// WRONG — holds read lock during sends, blocking writes
h.mu.RLock()
for _, client := range h.spaceSubscribers[spaceID] {
    client.Send <- data // blocks!
}
h.mu.RUnlock()
```

The copy has minimal allocation overhead (a slice of pointers) and the
performance benefit of reduced lock contention far outweighs it.

## Lesson: Ticket Expiry Should Be Short

The 60-second TTL on WS tickets is intentional. Tickets are single-use
authentication credentials — they should expire quickly to minimize the window
where a stolen ticket could be used. The flow is:

1. Client calls `POST /auth/ws-ticket` to get a ticket
2. Client immediately uses the ticket to connect: `GET /ws?ticket={id}`
3. Server marks the ticket as used

If the ticket expires before step 2, the client simply requests a new one.
The cost of requesting a new ticket is one HTTP round-trip — acceptable for
a connection that will last minutes or hours.
