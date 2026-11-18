-- 001207_remove_checklist_domain_settings.down.sql
-- Re-seeds the two removed checklist domain settings with their original
-- defaults (000262 seed, carried through 000735 baseline).

BEGIN;

INSERT INTO system_settings (id, key, value, is_public)
VALUES
  (gen_random_uuid(), 'checklist.subdomain', '"visa-checklist"'::jsonb, false),
  (gen_random_uuid(), 'checklist.custom_domain', '""'::jsonb, false)
ON CONFLICT (key) DO NOTHING;

COMMIT;
