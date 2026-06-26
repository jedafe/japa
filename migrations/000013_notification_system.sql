-- Section 35: Notification System
-- Notification types, notifications, preferences, and batch dedup windows

-- 1. Notification types (admin-configurable registry)
CREATE TABLE notification_types (
    key              text PRIMARY KEY,
    label            text NOT NULL,
    description      text,
    enabled          boolean NOT NULL DEFAULT true,
    default_in_app   boolean NOT NULL DEFAULT true,
    default_email    boolean NOT NULL DEFAULT false,
    default_push     boolean NOT NULL DEFAULT false
);

-- Seed default notification types
INSERT INTO notification_types (key, label, description, default_in_app, default_email, default_push) VALUES
    ('new_follower', 'New Follower', 'Someone followed you', true, false, false),
    ('post_reply', 'Post Reply', 'Someone replied to your post', true, true, false),
    ('comment', 'Comment', 'Someone commented on your post', true, true, false),
    ('mention', 'Mention', 'Someone mentioned you', true, true, false),
    ('reaction', 'Reaction', 'Someone reacted to your post', true, false, false),
    ('dm_message', 'Direct Message', 'You received a new message', true, true, true),
    ('dm_request', 'Message Request', 'Someone you don''t mutually follow wants to message you', true, false, false),
    ('system_announcement', 'System Announcement', 'Platform-wide announcement', true, true, false)
ON CONFLICT (key) DO NOTHING;

-- 2. Notifications table
CREATE TABLE notifications (
    id                 uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id          uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    recipient_id       uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    notification_type  text NOT NULL REFERENCES notification_types(key),
    actor_type         text,        -- 'user' or 'system'
    actor_id           uuid,
    subject_type       text,        -- 'content', 'space', 'comment', 'dm_conversation', 'review'
    subject_id         uuid,
    title              text NOT NULL,
    body               text,
    action_url         text,
    read_at            timestamptz,
    created_at         timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_notifications_recipient ON notifications (tenant_id, recipient_id, read_at, created_at DESC);
CREATE INDEX idx_notifications_type ON notifications (tenant_id, notification_type);

-- 3. Per-user notification preferences
CREATE TABLE notification_preferences (
    user_id            uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    notification_type  text NOT NULL REFERENCES notification_types(key),
    in_app             boolean NOT NULL DEFAULT true,
    email              boolean NOT NULL DEFAULT false,
    push               boolean NOT NULL DEFAULT false,
    PRIMARY KEY (user_id, notification_type)
);

-- 4. Batch dedup windows
CREATE TABLE notification_batch_windows (
    id                 uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    recipient_id       uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    notification_type  text NOT NULL,
    subject_type       text NOT NULL,
    subject_id         uuid NOT NULL,
    actor_ids          uuid[] NOT NULL DEFAULT '{}',
    actor_count        int NOT NULL DEFAULT 1,
    last_actor_id      uuid,
    window_open_at     timestamptz NOT NULL DEFAULT now(),
    window_close_at    timestamptz NOT NULL,
    dispatched         boolean NOT NULL DEFAULT false,
    UNIQUE(recipient_id, notification_type, subject_type, subject_id)
);
