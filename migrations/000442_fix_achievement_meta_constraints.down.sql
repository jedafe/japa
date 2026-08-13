-- 000442_fix_achievement_meta_constraints.down.sql
BEGIN;
-- Revert destination_space_id to nullable (original state in 000320).
ALTER TABLE achievement_meta
    ALTER COLUMN destination_space_id DROP NOT NULL;

-- Revert the milestone_type comment to the original (incorrect) value.
COMMENT ON COLUMN achievement_meta.milestone_type IS
    'visa_approved|landed|first_job|graduation|citizenship|other';
COMMIT;
