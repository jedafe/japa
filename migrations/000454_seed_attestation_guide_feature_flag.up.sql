-- 000454_seed_attestation_guide_feature_flag.up.sql
-- Fix BUG-001: The attestation_guide feature flag was never seeded into the
-- feature_flags table. Migration 000404 renamed feature.attestation_guide →
-- attestation_guide.enabled, but no prior migration ever inserted the
-- feature.attestation_guide row. The features.Gate middleware reads
-- feature_flags (not system_settings), so the tool was permanently 404-
-- disabled regardless of the Control Panel toggle.
--
-- This seeds attestation_guide.enabled=false for the default tenant so the
-- Gate middleware finds the row. The Control Panel's UpdateExtension handler
-- also writes system_settings (attestation_guide.enabled) — both stores now
-- agree on the key name.
--
-- Ref: japaconnect-engineering/sections/97-attestation-guide.md
--      japaconnect-engineering/conventions/feature-flag-naming.md

BEGIN;

INSERT INTO feature_flags (key, tenant_id, enabled) VALUES
    ('attestation_guide.enabled', '00000000-0000-0000-0000-000000000001', false)
ON CONFLICT (key, tenant_id) DO NOTHING;

COMMIT;
