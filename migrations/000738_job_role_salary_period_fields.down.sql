-- 000738_job_role_salary_period_fields.down.sql
-- Reverse the job_role + job_function + salary_period job_listing_fields
-- seed and restore the original positions.
-- Ref: japaconnect-engineering/sections/63-jobs-board.md

BEGIN;

DELETE FROM custom_field_values
WHERE field_id IN (
    '00000000-0000-0000-0000-0000000000b3',
    '00000000-0000-0000-0000-0000000000b4',
    '00000000-0000-0000-0000-0000000000b5'
);

DELETE FROM custom_fields
WHERE id IN (
    '00000000-0000-0000-0000-0000000000b3',
    '00000000-0000-0000-0000-0000000000b4',
    '00000000-0000-0000-0000-0000000000b5'
);

-- Undo the second +1 shift (positions > 11), then the first +2 shift.
UPDATE custom_fields
SET position = position - 1
WHERE group_id = '00000000-0000-0000-0000-0000000000a0'
  AND position >= 12;

UPDATE custom_fields
SET position = position - 2
WHERE group_id = '00000000-0000-0000-0000-0000000000a0'
  AND position >= 5;

COMMIT;
