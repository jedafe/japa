-- 001017_persona_voices_system_flag.up.sql
-- Mark the seeded role x tone voice catalog as platform-managed system
-- voices (ADR-114). The catalog is seeded by the system user
-- (ffffffff-0000-0000-0000-000000000001) at deploy time and must not be
-- deletable via DELETE /admin/persona-voices/:id — personas referencing a
-- deleted voice would lose their tone anchor. Admin-created voices keep
-- is_system = false.
--
-- Ref: .jeos/adr/ADR-114-shared-persona-voices-and-content-type-model-defaults.md

BEGIN;

UPDATE persona_voices
SET is_system = true
WHERE created_by = 'ffffffff-0000-0000-0000-000000000001'::uuid;

COMMIT;
