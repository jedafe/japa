# Section 48 — Direct Messages (DM)

**Status:** ✅ Locked (extracted from Section 10 stub + fully specified in audit pass)

Feature-flagged: `feature.dm` (Default: true). Turning this off hides all DM UI and returns 404 on DM routes.

## Overview

1:1 private messaging. New conversations from a non-mutual-follow sender land as a **message request** — a pending state the recipient must accept before it joins their main inbox. A per-user privacy setting gates who can initiate new conversations.

## Schema

```sql
CREATE TABLE dm_conversations (
  id                   uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id            uuid NOT NULL REFERENCES tenants(id),
  participant_a        uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  participant_b        uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  last_message_at      timestamptz,
  last_message_preview text,   -- first 100 chars of last message body (plain text)
  unread_a             int NOT NULL DEFAULT 0,   -- unread count for participant_a
  unread_b             int NOT NULL DEFAULT 0,   -- unread count for participant_b
  created_at           timestamptz NOT NULL DEFAULT now(),
  CHECK (participant_a < participant_b),          -- canonical ordering prevents duplicate rows
  UNIQUE(tenant_id, participant_a, participant_b)
);
CREATE INDEX idx_dm_conv_a ON dm_conversations (participant_a, last_message_at DESC NULLS LAST);
CREATE INDEX idx_dm_conv_b ON dm_conversations (participant_b, last_message_at DESC NULLS LAST);

CREATE TABLE dm_messages (
  id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id       uuid NOT NULL REFERENCES tenants(id),
  conversation_id uuid NOT NULL REFERENCES dm_conversations(id) ON DELETE CASCADE,
  sender_id       uuid NOT NULL REFERENCES users(id),
  body            jsonb NOT NULL,      -- TipTap JSON (Section 15); plain text for now, rich text opt-in
  media_id        uuid REFERENCES media(id) ON DELETE SET NULL,  -- optional single attachment
  edited_at       timestamptz,
  deleted_at      timestamptz,         -- soft delete; body replaced with null on delete
  created_at      timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_dm_messages_conv ON dm_messages (conversation_id, created_at DESC);

-- Message request: DM from someone who isn't yet allowed direct access
CREATE TABLE dm_requests (
  id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id       uuid NOT NULL REFERENCES tenants(id),
  requester_id    uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  recipient_id    uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  conversation_id uuid REFERENCES dm_conversations(id),  -- created on accept
  preview_message text NOT NULL,   -- first message text; shown in the request preview
  status          text NOT NULL DEFAULT 'pending'
                    CHECK (status IN ('pending', 'accepted', 'declined', 'blocked')),
  created_at      timestamptz NOT NULL DEFAULT now(),
  resolved_at     timestamptz,
  UNIQUE(requester_id, recipient_id)
);
CREATE INDEX idx_dm_requests_recipient ON dm_requests (recipient_id, status, created_at DESC);
```

## DM Privacy Settings

`users.dm_privacy` controls who can open a **new** conversation with you:

| Setting | Behaviour |
|---|---|
| `everyone` | Anyone can send a DM request; mutual followers get direct access |
| `followers_only` | Only mutual followers can open new conversations |
| `nobody` | No new DM conversations accepted; existing ones still active |

Mentions and mention notifications (`@user`) fire regardless of `dm_privacy` setting — a mention is not a new conversation.

## Conversation Routing Logic

```go
func InitiateDM(ctx context.Context, senderID, recipientID uuid.UUID, message string) error {
    recipient := getUser(ctx, recipientID)

    // Check block: sender blocked by recipient or vice versa → 403
    if isBlocked(ctx, senderID, recipientID) {
        return ErrBlocked
    }

    isMutualFollow := areMutualFollowers(ctx, senderID, recipientID)

    switch recipient.DmPrivacy {
    case "nobody":
        return ErrDMPrivacyNobody
    case "followers_only":
        if !isMutualFollow { return ErrNotMutualFollower }
    }

    // If mutual follow or dm_privacy='everyone' with mutual follow → direct conversation
    if isMutualFollow {
        conv := getOrCreateConversation(ctx, senderID, recipientID)
        return sendMessage(ctx, conv.ID, senderID, message)
    }

    // Non-mutual-follow → create a message request
    return createDMRequest(ctx, senderID, recipientID, message)
}
```

## Valkey Unread Counters

DB `unread_a` / `unread_b` columns are the source of truth. Valkey caches the total for badge display:

```
jc:dm_total_unread:{user_id}  →  INT  (TTL 24h, refreshed on read events)
```

On new message: `INCR jc:dm_total_unread:{user_id}  # user_id = the recipient` + `UPDATE dm_conversations SET unread_{a|b} = unread_{a|b} + 1`.
On conversation read (`POST /dm/conversations/:id/read`): set DB column to 0, recalculate and re-set Valkey key.

## API Routes

```
GET  /dm/conversations                     — list conversations (last_message_at DESC, includes unread count)
POST /dm/conversations                     — initiate DM or get existing; body: { recipient_id, message }
GET  /dm/conversations/:id/messages        — cursor-paginated (keyset by created_at), newest first
POST /dm/conversations/:id/messages        — send message; body: { body: TipTapJSON, media_id?: uuid }
PATCH /dm/conversations/:id/messages/:mid  — edit own message (15-min window); body: { body }
DELETE /dm/conversations/:id/messages/:mid — soft delete own message
POST /dm/conversations/:id/read            — mark conversation read; clears unread count

GET  /dm/requests                          — list incoming pending requests
POST /dm/requests/:id/accept               — accept → creates/activates conversation → returns conv_id
POST /dm/requests/:id/decline              — decline; sender gets no notification
POST /dm/requests/:id/block                — decline + add sender to user_blocks (Section 41)
```

## WebSocket Integration

```go
// Channel per conversation: dm:{conversation_id}
// Both participants subscribe when the conversation page is open

// Events emitted:
// dm_message   — new message { conversation_id, message: DmMessage }
// typing       — typing indicator { conversation_id, user_id, is_typing: bool } (existing event type)

func BroadcastDMMessage(ctx context.Context, conv *DmConversation, msg *DmMessage) {
    channel := "dm:" + conv.ID.String()
    payload := WSEvent{Type: "dm_message", Payload: map[string]any{
        "conversation_id": conv.ID,
        "message":         msg,
    }}
    valkey.Publish(ctx, channel, mustJSON(payload))

    // Also push a notification event to the recipient's presence channel
    // so the unread badge updates even if the conversation isn't open
    DispatchNotification(ctx, NotificationInput{
        Type:      "dm_message",
        Recipient: otherParticipant(conv, msg.SenderID),
        Actor:     msg.SenderID,
        Subject:   conv.ID,
    })
}
```

## Asynq Tasks

- `notification:dm_request` (critical tier) — notifies recipient of a new DM request
- `dm:cleanup_declined_requests` (low tier, weekly) — removes declined requests older than 90 days

## Frontend Routes (Main Site)

```
/messages                      — conversation list; unread badge in nav
/messages/:conversation_id     — conversation thread; infinite scroll up, new messages at bottom
/messages/requests             — pending message requests tab
```

On mobile: `/messages` is a full-page view. On desktop: left-sidebar conversation list + right-pane thread (SvelteKit layout).

## Asynq Tasks

- `notification:dm_message` (critical tier) — push notification + email if user has email DMs enabled
- `notification:dm_request` (critical tier) — notify recipient of incoming request
- `dm:cleanup_declined_requests` (low tier, weekly) — prune declined requests older than 90 days

## See Also

Section 10 (WebSocket — `dm_message` event, `typing` event), Section 15 (TipTap JSON body format), Section 35 (notification_type: `dm_message` | `dm_request`), Section 41 (user blocking — blocks prevent DMs), Section 42 (feature.dm flag). ADR-029.
