-- Auth tables: refresh tokens, TOTP credentials, OAuth accounts.
-- All tables carry tenant_id for multi-tenant scoping.

-- Refresh tokens: stored server-side for rotation and revocation.
-- Token hash is stored (not the raw token) so a DB leak doesn't
-- give an attacker valid refresh tokens.
CREATE TABLE refresh_tokens (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    user_id     uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash  text NOT NULL UNIQUE,
    expires_at  timestamptz NOT NULL,
    revoked_at  timestamptz,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_refresh_tokens_user ON refresh_tokens(tenant_id, user_id);
CREATE INDEX idx_refresh_tokens_hash ON refresh_tokens(token_hash);

-- TOTP credentials for 2FA. Mandatory for admin/panel users.
CREATE TABLE totp_credentials (
    id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id         uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    user_id           uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    secret_encrypted  text NOT NULL,
    enabled_at        timestamptz,
    backup_codes      jsonb NOT NULL DEFAULT '[]',
    created_at        timestamptz NOT NULL DEFAULT now(),
    updated_at        timestamptz NOT NULL DEFAULT now(),
    UNIQUE(tenant_id, user_id)
);

-- OAuth account links. Provider tokens are NEVER stored - only the
-- provider's sub identifier and the linkage to our user_id.
CREATE TABLE oauth_accounts (
    id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id         uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    user_id           uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    provider          text NOT NULL,
    provider_user_id  text NOT NULL,
    created_at        timestamptz NOT NULL DEFAULT now(),
    UNIQUE(tenant_id, provider, provider_user_id),
    UNIQUE(tenant_id, user_id, provider)
);

CREATE INDEX idx_oauth_accounts_provider ON oauth_accounts(tenant_id, provider, provider_user_id);
