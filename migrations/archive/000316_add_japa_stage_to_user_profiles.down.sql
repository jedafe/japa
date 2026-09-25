-- 000316_add_japa_stage_to_user_profiles.down.sql
BEGIN;
DROP INDEX IF EXISTS idx_user_profiles_japa_stage;
ALTER TABLE user_profiles
    DROP COLUMN IF EXISTS japa_stage_history,
    DROP COLUMN IF EXISTS japa_stage_is_public,
    DROP COLUMN IF EXISTS japa_stage_updated_at,
    DROP COLUMN IF EXISTS japa_stage_dest_space_id,
    DROP COLUMN IF EXISTS japa_stage;
COMMIT;
