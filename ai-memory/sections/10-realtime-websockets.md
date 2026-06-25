# Section 10 — Real-Time (WebSockets)

**Status:** ✅ Locked

## Decided
WebSocket hub with Redis pub/sub bridge for multi-instance scaling. Seven event types: `notification`, `presence`, `typing`, `space_activity`, `feed_update`, `dm_message`, `moderation_alert`. Layered presence: platform-wide via a heartbeat-TTL Redis key, space-level derived from pub/sub channel subscription (no separate storage), thread-level pure ephemeral broadcast (never persisted). Auth via a short-lived WS ticket minted by an authenticated REST call, not a long-lived credential in the handshake. Client-side exponential backoff reconnection; missed-event catch-up happens via REST fetch, not WS replay — the real-time layer is a live-update enhancement, never the source of truth, since Redis pub/sub has no durability/replay by design.

## Direct Messaging (restored post-audit)

1:1 conversations, consuming the `dm_message` event type that was already in the catalog above but never had an actual feature built around it. `dm_conversations` (two participants, last-message preview, unread count per participant) and `dm_messages` (conversation_id, sender_id, body, created_at, read_at nullable). New conversations from a non-mutual-follow sender land as a **message request** — a separate pending state the recipient must accept before it joins their main inbox, the standard pattern against unsolicited DMs. A per-user DM privacy setting (`everyone` / `followers only` / `nobody`) gates who can open a new conversation at all; mentions bypass this setting (see Section 15's mention-notification edge cases — a mention notification still fires even if the mentioned user has DMs set to `nobody`, since a mention isn't a new conversation). Typing indicators reuse the existing `typing` event type, scoped to the conversation channel rather than a new mechanism.

## See Also
ADR-029.
