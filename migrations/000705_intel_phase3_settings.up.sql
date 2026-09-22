-- 000705_intel_phase3_settings.up.sql
-- Seed the remaining intel-channels Phase 3 setting not covered by the
-- 000685 seed: the inbound-email webhook signing key (spec §3 email
-- adapter). intel.search.provider / intel.search.api_key /
-- intel.email.inbound_domain / intel.email.allowed_senders were already
-- seeded (empty = disabled) by 000685_seed_intel_channel_settings.
--
--   intel.email.signing_key — Mailgun-compatible webhook signing key,
--                             stored as AES-256-GCM ciphertext via
--                             crypto.Encrypt (the payment-secret pattern).
--                             Empty = the /intel/inbound/email endpoint
--                             is disabled (503 inbound_disabled) — a
--                             deployment must never accept unsigned
--                             email deliveries.
--
-- Post-de-tenant convention: rows are keyed globally on `key`.

BEGIN;

INSERT INTO system_settings (key, value) VALUES
    ('intel.email.signing_key', to_jsonb(''::text))
ON CONFLICT (key) DO NOTHING;

COMMIT;
