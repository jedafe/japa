BEGIN;
DROP INDEX IF EXISTS idx_user_profiles_japa_intent;
ALTER TABLE user_profiles
    DROP CONSTRAINT IF EXISTS user_profiles_intent_stage_chk,
    DROP CONSTRAINT IF EXISTS user_profiles_japa_intent_chk,
    DROP COLUMN IF EXISTS japa_intent;
COMMIT;
