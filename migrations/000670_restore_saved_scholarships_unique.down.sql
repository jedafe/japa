-- 000670_restore_saved_scholarships_unique.down.sql
-- Reverts 000670: drops the de-tenant unique constraint and indexes on
-- saved_scholarships. (Does NOT restore tenant_id — that is 000668's down.)

BEGIN;

DROP INDEX IF EXISTS idx_saved_scholarships_remind;
DROP INDEX IF EXISTS idx_saved_scholarships_deadline;
DROP INDEX IF EXISTS idx_saved_scholarships_user;

ALTER TABLE saved_scholarships
    DROP CONSTRAINT IF EXISTS saved_scholarships_user_id_guide_id_key;

COMMIT;
