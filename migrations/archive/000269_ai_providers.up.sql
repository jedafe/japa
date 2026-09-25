-- 000269 — ai_providers table for runtime-managed AI provider configs.
--
-- The Control Panel AI Providers page (super-admin) manages which AI
-- backends are wired into the dispatcher at runtime. Previously providers
-- were hardcoded at startup from env vars only (server.go RegisterProvider).
-- This table stores encrypted provider configs so super-admins can add,
-- edit, enable/disable, and remove providers without a redeploy.
--
-- API keys are encrypted via internal/crypto (AES-256-GCM, v1:nonce:ciphertext
-- format) before being stored in api_key_encrypted.
--
-- Ref: ADR-027, Section 20 (AI Core), Control Panel skill (jc-control-develop).

CREATE TABLE IF NOT EXISTS ai_providers (
    id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id           uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name                text NOT NULL,
    provider_type       text NOT NULL CHECK (provider_type IN ('xiaomi', 'openai', 'ollama', 'anthropic', 'custom')),
    api_key_encrypted   text NOT NULL DEFAULT '',
    base_url            text NOT NULL DEFAULT '',
    model               text NOT NULL DEFAULT '',
    categories          jsonb NOT NULL DEFAULT '[]'::jsonb,
    priority            integer NOT NULL DEFAULT 0,
    enabled             boolean NOT NULL DEFAULT true,
    created_at          timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_ai_providers_tenant_id ON ai_providers(tenant_id);
CREATE INDEX idx_ai_providers_tenant_enabled ON ai_providers(tenant_id, enabled);

-- Seed a row from the existing env-var-configured Xiaomi provider so the
-- management UI shows the currently-active provider on first load. The API
-- key is left empty here (it is still read from JC_XIAOMI_API_KEY at startup
-- as the fallback); the super-admin can set it via the UI to persist it.
-- Only seed if a tenants row exists (avoids FK violation on fresh installs
-- where the first tenant has not been created yet).
INSERT INTO ai_providers (tenant_id, name, provider_type, base_url, model, categories, priority, enabled)
SELECT t.id, 'Xiaomi MiMo (env)', 'xiaomi', '', 'mimo-v2-flash',
       '["content_generation", "chat"]'::jsonb, 0, true
FROM tenants t
WHERE NOT EXISTS (SELECT 1 FROM ai_providers ap WHERE ap.tenant_id = t.id)
LIMIT 1;
