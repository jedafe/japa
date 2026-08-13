-- 000455_seed_attestation_hague_members_setting.up.sql
-- Fix BUG-002: §97 specifies apostille_required is "a static config derived
-- from a seeded hague_convention_members lookup table (stored in
-- system_settings as a JSONB list)". No such setting existed. This seeds an
-- empty JSON array; admin populates it with destination country space IDs
-- that are Hague Convention members. The attestationguide service reads this
-- list and sets apostille_required=true when the destination country is a
-- member.
--
-- Ref: japaconnect-engineering/sections/97-attestation-guide.md (Hague Convention)

BEGIN;

INSERT INTO system_settings (tenant_id, key, value) VALUES
    ('00000000-0000-0000-0000-000000000001', 'attestation_guide.hague_members', '[]')
ON CONFLICT (tenant_id, key) DO NOTHING;

COMMIT;
