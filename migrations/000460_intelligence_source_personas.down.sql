-- 000460_intelligence_source_personas.down.sql
-- Reverses the persona attachment: removes persona_id from intel_sources,
-- drops the 16 seeded personas + their profiles + bot users, removes the
-- intel.default_persona_id setting, and removes the contributor profile type.

BEGIN;

-- Remove persona_id from intel_sources.
ALTER TABLE intel_sources DROP CONSTRAINT IF EXISTS intel_sources_persona_id_fkey;
ALTER TABLE intel_sources DROP COLUMN IF EXISTS persona_id;

-- Remove the intel.default_persona_id setting.
DELETE FROM system_settings
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND key = 'intel.default_persona_id';

-- Delete the 16 ai_personas rows. CASCADE from users will handle the rest
-- (ai_personas.user_id FK → users(id) ON DELETE CASCADE, and
-- professional_profiles.owner_user_id FK → users(id)).
-- But we need to delete profiles explicitly since they don't CASCADE on user
-- delete (owner_user_id has no ON DELETE CASCADE).
DELETE FROM professional_profiles
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND slug LIKE 'intel-%';

DELETE FROM ai_personas
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND user_id IN (
    SELECT id FROM users
    WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
      AND email LIKE 'intel-%@bot.internal'
  );

DELETE FROM users
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND email LIKE 'intel-%@bot.internal';

-- Remove the contributor profile type (only if it was seeded by this migration).
DELETE FROM professional_profile_types
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND slug = 'contributor'
  AND is_system = true;

COMMIT;
