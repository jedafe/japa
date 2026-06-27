-- 000030_user_preferences.sql
-- User preferences for theme, language, and display settings.

CREATE TABLE user_preferences (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   uuid NOT NULL REFERENCES tenants(id),
    user_id     uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    dark_mode   text NOT NULL DEFAULT 'system',  -- 'light'|'dark'|'system'
    language    text NOT NULL DEFAULT 'en-NG',    -- en-NG, en-US, en-GB, en-CA
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),
    UNIQUE(tenant_id, user_id)
);

CREATE INDEX idx_user_preferences_tenant ON user_preferences(tenant_id);
