-- 000442_fix_achievement_meta_constraints.up.sql
-- Fixes two issues identified in the achievement audit (§89, ADR-097):
--
--   1. destination_space_id was nullable in the original migration (000320),
--      but the spec requires it to be NOT NULL (destination_country_id INT
--      NOT NULL in §89 Schema). The backend already enforces this at the
--      handler level (HandleSubmit rejects missing destination), but the
--      DB column should also be NOT NULL for defense-in-depth.
--
--   2. The milestone_type column comment listed incorrect values
--      (visa_approved|landed|first_job|graduation|citizenship|other) instead
--      of the spec-defined enum values
--      (visa_approved|job_offer|university_accepted|arrived|one_year|
--       citizenship|other).
--
-- Ref: japaconnect-engineering/sections/89-achievement-feed.md

BEGIN;

-- (1) Clean up any rows with NULL destination_space_id before adding the
--     NOT NULL constraint. In practice there should be none (the backend
--     always requires destination), but this guards against manual inserts.
DELETE FROM achievement_meta WHERE destination_space_id IS NULL;

-- Add NOT NULL constraint on destination_space_id.
ALTER TABLE achievement_meta
    ALTER COLUMN destination_space_id SET NOT NULL;

-- (2) Correct the milestone_type column comment to match the spec-defined
--     enum values (§89).
COMMENT ON COLUMN achievement_meta.milestone_type IS
    'visa_approved|job_offer|university_accepted|arrived|one_year|citizenship|other';

COMMIT;
