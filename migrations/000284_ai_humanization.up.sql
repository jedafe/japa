-- 000284_ai_humanization.up.sql
-- AI text humanization feature (Section 20 extension).
--
-- Adds two system_settings keys that gate the feature globally and set the
-- default intensity, plus an RBAC permission node (ai.humanize.manage) that
-- gates which admins can toggle the feature in the Control/Admin panel.
--
-- The feature injects natural-language style instructions into the LLM system
-- context at the provider layer, so every generation surface (generate, chat,
-- assist/improve, assist/summarize, assist/translate, persona/studio drafts)
-- inherits it. Users can also opt in/out per request via the humanize payload
-- field, which the handler merges with this global setting.
--
-- Idempotent: ON CONFLICT DO NOTHING preserves any tenant-specific override.
-- Seeded for the default tenant only; migration 000240's backfill pattern
-- copies rows to other tenants on subsequent runs.

BEGIN;

-- 1. Seed the two ai.humanize.* system_settings keys (default tenant).
INSERT INTO system_settings (tenant_id, key, value) VALUES
    ('00000000-0000-0000-0000-000000000001', 'ai.humanize.enabled', '"false"'),
    ('00000000-0000-0000-0000-000000000001', 'ai.humanize.level',   '"medium"')
ON CONFLICT (tenant_id, key) DO NOTHING;

-- 2. Seed the ai.humanize.manage RBAC permission node (admin capability).
INSERT INTO permissions (key, description) VALUES
    ('ai.humanize.manage', 'Toggle the ai.humanize.* system settings (enable/disable humanization and set intensity)')
ON CONFLICT (key) DO NOTHING;

-- 3. Grant ai.humanize.manage to super_admin (admin capability, not a
--    context-feed node, so it is not subject to the §19 default-off rule).
INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug = 'super_admin'
  AND p.key = 'ai.humanize.manage'
ON CONFLICT DO NOTHING;

COMMIT;
