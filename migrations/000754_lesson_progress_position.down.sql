BEGIN;

ALTER TABLE lesson_progress
    DROP COLUMN IF EXISTS position_secs;

COMMIT;
