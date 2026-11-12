-- 001121 — email_providers table for runtime-managed email provider configs.
--
-- ADR-015 (multi-provider email abstraction with category routing). The
-- Control Panel manages which email backends are wired into the
-- EmailDispatcher at runtime; previously only the JC_SMTP_* env-var provider
-- existed and served both categories. Providers are assigned to categories
-- (transactional, marketing) and tried in priority order with
-- circuit-breaker fallback — mirroring the ai_providers gateway (ADR-027 /
-- migration 000269) exactly.
--
-- Secrets (API keys, SMTP passwords, AWS secret access keys, SendPulse
-- client secrets) are encrypted via internal/crypto (AES-256-GCM,
-- v1:nonce:ciphertext format) before being stored in api_key_encrypted —
-- the same treatment as payment provider secrets (ADR-015 Consequences).
-- Non-secret per-type settings live in config jsonb:
--   smtp:      {"host","port","username","from_address"}
--   resend:    {"from_address"}
--   postmark:  {"from_address","message_stream"}
--   ses:       {"region","access_key_id","from_address"}
--   sendgrid:  {"from_address"}
--   sendpulse: {"client_id","from_address"}
-- api_key_encrypted holds the single secret per type: SMTP password,
-- vendor API key / server token, AWS secret_access_key, SendPulse
-- client_secret.

CREATE TABLE IF NOT EXISTS email_providers (
    id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name                text NOT NULL,
    provider_type       text NOT NULL CHECK (provider_type IN ('smtp', 'resend', 'postmark', 'ses', 'sendgrid', 'sendpulse')),
    api_key_encrypted   text NOT NULL DEFAULT '',
    base_url            text NOT NULL DEFAULT '',
    config              jsonb NOT NULL DEFAULT '{}'::jsonb,
    categories          jsonb NOT NULL DEFAULT '[]'::jsonb,
    priority            integer NOT NULL DEFAULT 0,
    enabled             boolean NOT NULL DEFAULT true,
    created_at          timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_email_providers_enabled ON email_providers(enabled);
