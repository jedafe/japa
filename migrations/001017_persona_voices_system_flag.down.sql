-- 001017_persona_voices_system_flag.down.sql
-- Reverts the seeded catalog to non-system (best-effort — cannot distinguish
-- voices an admin later marked system after this migration ran).

BEGIN;

UPDATE persona_voices
SET is_system = false
WHERE created_by = 'ffffffff-0000-0000-0000-000000000001'::uuid;

COMMIT;
