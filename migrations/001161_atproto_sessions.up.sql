-- 001161_atproto_sessions.up.sql
-- A6-4 external-client sessions (§127): app passwords + refresh-tracked
-- sessions for the com.atproto.server.* XRPC surface. App passwords
-- authenticate external logins only — they never sign repo writes.
--
-- Ref: japaconnect-engineering/sections/127-atproto-native.md (A6-4)

BEGIN;

CREATE TABLE atproto_app_passwords (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id    uuid NOT NULL REFERENCES atproto_accounts(id) ON DELETE CASCADE,
    name          text NOT NULL,
    password_hash text NOT NULL,
    created_at    timestamptz NOT NULL DEFAULT now(),
    last_used_at  timestamptz,
    revoked_at    timestamptz
);

-- Real-PDS parity: names are unique per account while live. Partial
-- index keeps revoked rows from colliding with a reused name.
CREATE UNIQUE INDEX atproto_app_passwords_live_name_key
    ON atproto_app_passwords (account_id, name) WHERE revoked_at IS NULL;

CREATE INDEX atproto_app_passwords_account_idx
    ON atproto_app_passwords (account_id) WHERE revoked_at IS NULL;

CREATE TABLE atproto_sessions (
    id                 uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id         uuid NOT NULL REFERENCES atproto_accounts(id) ON DELETE CASCADE,
    app_password_id    uuid REFERENCES atproto_app_passwords(id) ON DELETE SET NULL,
    refresh_hash       text NOT NULL UNIQUE,
    user_agent         text,
    last_ip            text,
    created_at         timestamptz NOT NULL DEFAULT now(),
    refresh_expires_at timestamptz NOT NULL,
    revoked_at         timestamptz
);

CREATE INDEX atproto_sessions_account_idx
    ON atproto_sessions (account_id) WHERE revoked_at IS NULL;

COMMIT;
