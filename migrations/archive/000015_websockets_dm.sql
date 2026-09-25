-- Section 10: Real-Time WebSockets + Direct Messaging
-- WebSocket ticket storage, DM conversations and messages

-- 1. WebSocket tickets (short-lived, single-use)
CREATE TABLE ws_tickets (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    tenant_id   uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    used        boolean NOT NULL DEFAULT false,
    created_at  timestamptz NOT NULL DEFAULT now(),
    expires_at  timestamptz NOT NULL
);
CREATE INDEX idx_ws_tickets_user ON ws_tickets (user_id, used, expires_at);

-- 2. DM Conversations
CREATE TABLE dm_conversations (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    participant_1_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    participant_2_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    last_message_at timestamptz,
    last_message_preview text,
    -- Unread counts per participant
    p1_unread_count  int NOT NULL DEFAULT 0,
    p2_unread_count  int NOT NULL DEFAULT 0,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    UNIQUE(tenant_id, participant_1_id, participant_2_id),
    CHECK (participant_1_id < participant_2_id) -- enforce ordering to prevent duplicates
);

-- 3. DM Messages
CREATE TABLE dm_messages (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    conversation_id uuid NOT NULL REFERENCES dm_conversations(id) ON DELETE CASCADE,
    sender_id       uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    body            text NOT NULL,
    created_at      timestamptz NOT NULL DEFAULT now(),
    read_at         timestamptz
);
CREATE INDEX idx_dm_messages_conversation ON dm_messages (conversation_id, created_at DESC);
CREATE INDEX idx_dm_messages_sender ON dm_messages (sender_id);

-- 4. DM Message Requests (for non-mutual-follow senders)
CREATE TABLE dm_message_requests (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    conversation_id uuid NOT NULL REFERENCES dm_conversations(id) ON DELETE CASCADE,
    status          text NOT NULL DEFAULT 'pending', -- pending, accepted, declined
    created_at      timestamptz NOT NULL DEFAULT now(),
    UNIQUE(conversation_id)
);

-- 5. User DM privacy settings (stored as separate table)
CREATE TABLE user_dm_settings (
    user_id     uuid PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    dm_privacy  text NOT NULL DEFAULT 'everyone', -- everyone, followers_only, nobody
    updated_at  timestamptz NOT NULL DEFAULT now()
);
