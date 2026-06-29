-- Password reset tokens and email verification tables.
-- All tables carry tenant_id for multi-tenant scoping.

-- Password reset tokens: short-lived, single-use tokens for password recovery.
-- Token hash is stored (not the raw token) so a DB leak doesn't give an attacker valid tokens.
CREATE TABLE IF NOT EXISTS password_reset_tokens (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    user_id     uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash  text NOT NULL,
    expires_at  timestamptz NOT NULL,
    used_at     timestamptz,
    created_at  timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_password_reset_tokens_hash ON password_reset_tokens(token_hash);
CREATE INDEX IF NOT EXISTS idx_password_reset_tokens_user ON password_reset_tokens(tenant_id, user_id);

-- Email verification tokens: used to confirm a user's email address.
-- Verified_at is set when the user clicks the verification link.
CREATE TABLE IF NOT EXISTS email_verifications (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    user_id     uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    email       text NOT NULL,
    token_hash  text NOT NULL,
    expires_at  timestamptz NOT NULL,
    verified_at timestamptz,
    created_at  timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_email_verifications_hash ON email_verifications(token_hash);
