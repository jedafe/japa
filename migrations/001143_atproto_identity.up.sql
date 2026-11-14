-- 001143_atproto_identity.up.sql
-- Native AT Protocol mini-PDS (§127, ADR-139) — A1 identity schema:
-- atproto_accounts (per-member opt-in accounts w/ encrypted K-256 signing
-- keys, did:web identifiers, {username}.{handle_domain} handles).
-- Repo storage tables (atproto_repo_*, atproto_records) arrive with A2.
-- RBAC nodes seeded complete (BE-005).
--
-- Ref: japaconnect-engineering/sections/127-atproto-native.md

BEGIN;

CREATE TABLE IF NOT EXISTS atproto_accounts (
    id                    uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id               uuid NOT NULL UNIQUE REFERENCES users(id) ON DELETE CASCADE,
    handle                text NOT NULL UNIQUE,  -- {username}.{JC_ATPROTO_HANDLE_DOMAIN}
    did                   text NOT NULL UNIQUE,  -- did:web:{pds_host}:atproto:u:{user_uuid}
    signing_key_enc       text NOT NULL,         -- K-256 private key, internal/crypto ciphertext
    public_key_multibase  text NOT NULL,         -- verificationMethod material for the DID doc
    status                text NOT NULL DEFAULT 'active',
    repo_seq              bigint NOT NULL DEFAULT 0,  -- commit watermark (A2)
    created_at            timestamptz NOT NULL DEFAULT now(),
    updated_at            timestamptz NOT NULL DEFAULT now(),
    disabled_at           timestamptz,
    CONSTRAINT atproto_accounts_status_check CHECK (status IN ('active','disabled','tombstone'))
);
-- handle lookups (atproto-did well-known, did:web docs) compare under
-- lower(); handles are case-insensitive in atproto, so uniqueness is
-- enforced case-insensitively at the DB level too — users 'Alice' and
-- 'alice' are distinct accounts but cannot both claim alice.japa.to
CREATE UNIQUE INDEX IF NOT EXISTS idx_atproto_accounts_handle ON atproto_accounts (lower(handle));
CREATE INDEX IF NOT EXISTS idx_atproto_accounts_user ON atproto_accounts (user_id);

-- RBAC nodes (BE-005 — seed completely):
--   atproto.use           member opt-in routes (/me/atproto)
--   atproto.accounts.view admin account list + stats
INSERT INTO permissions (key, description) VALUES
    ('atproto.use',           'Opt into AT Protocol / Bluesky federation (mint and manage own PDS account)'),
    ('atproto.accounts.view', 'View atproto accounts and stats (admin)')
ON CONFLICT (key) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM user_roles r
CROSS JOIN permissions p
WHERE (
    (r.slug IN ('member','pro_member','business_member','super_admin') AND p.key = 'atproto.use')
    OR
    (r.slug = 'super_admin' AND p.key = 'atproto.accounts.view')
)
ON CONFLICT DO NOTHING;

COMMIT;
