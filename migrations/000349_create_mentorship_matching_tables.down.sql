-- 000349_create_mentorship_matching_tables.down.sql
-- Rollback: remove mentorship_connections table, trigger, function,
-- and user_profiles mentorship columns.

BEGIN;

DROP TRIGGER IF EXISTS trg_mentee_count ON mentorship_connections;
DROP FUNCTION IF EXISTS trg_update_active_mentee_count();

DROP TABLE IF EXISTS mentorship_connections;

ALTER TABLE user_profiles DROP COLUMN IF EXISTS mentor_embedding;
ALTER TABLE user_profiles DROP COLUMN IF EXISTS mentor_since;
ALTER TABLE user_profiles DROP COLUMN IF EXISTS mentor_bio;
ALTER TABLE user_profiles DROP COLUMN IF EXISTS mentor_areas;
ALTER TABLE user_profiles DROP COLUMN IF EXISTS active_mentee_count;
ALTER TABLE user_profiles DROP COLUMN IF EXISTS mentor_capacity;
ALTER TABLE user_profiles DROP COLUMN IF EXISTS is_mentor;

COMMIT;
