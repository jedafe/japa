-- 000890_ai_byok.up.sql — BYOK (bring-your-own-key): admin-curated provider
-- allowlist + encrypted per-user provider keys.
-- Spec: docs/superpowers/specs/2026-10-19-ai-byok-design.md
BEGIN;

CREATE TABLE ai_byok_providers (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    provider_type text NOT NULL UNIQUE
        CHECK (provider_type = ANY (ARRAY['openai','anthropic','groq','perplexity'])),
    display_name text NOT NULL,
    models jsonb NOT NULL DEFAULT '[]',
    signup_url text NOT NULL DEFAULT '',
    enabled boolean NOT NULL DEFAULT true,
    referral_clicks integer NOT NULL DEFAULT 0,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE user_ai_keys (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    byok_provider_id uuid NOT NULL REFERENCES ai_byok_providers(id) ON DELETE CASCADE,
    label text NOT NULL DEFAULT '',
    api_key_encrypted text NOT NULL,
    status text NOT NULL DEFAULT 'active' CHECK (status IN ('active','invalid')),
    last_error text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_user_ai_keys_user ON user_ai_keys(user_id);

-- RBAC node: BYOK is available to all member tiers; admins can revoke per
-- role later via the RBAC UI.
INSERT INTO permissions (key, description)
VALUES ('ai.byok', 'Attach personal AI provider API keys (BYOK) for chat/agent turns')
ON CONFLICT (key) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE p.key = 'ai.byok'
  AND ur.slug IN ('member', 'pro_member', 'business_member', 'admin-group-role', 'super_admin_role')
ON CONFLICT DO NOTHING;

COMMIT;
