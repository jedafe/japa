-- Push notification tokens for FCM (Android/web) and APNS (iOS).
-- Ref: sections/50-user-dashboard-settings.md, maintenance:prune_push_tokens task.

CREATE TABLE push_tokens (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    tenant_id   uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    token       text NOT NULL,
    platform    text NOT NULL CHECK (platform IN ('fcm', 'apns', 'web')),
    device_name text,
    created_at  timestamptz NOT NULL DEFAULT now(),
    last_used_at timestamptz,
    UNIQUE(user_id, token)
);

CREATE INDEX idx_push_tokens_user ON push_tokens (user_id);
CREATE INDEX idx_push_tokens_tenant ON push_tokens (tenant_id);
CREATE INDEX idx_push_tokens_stale ON push_tokens (last_used_at, created_at);
