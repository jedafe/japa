-- 000734_job_company_fields.down.sql
-- Reverse the company_logo + company_website job_listing_fields seed.
-- Ref: japaconnect-engineering/sections/63-jobs-board.md

BEGIN;

DELETE FROM custom_field_values
WHERE field_id IN (
    '00000000-0000-0000-0000-0000000000b1',
    '00000000-0000-0000-0000-0000000000b2'
);

DELETE FROM custom_fields
WHERE id IN (
    '00000000-0000-0000-0000-0000000000b1',
    '00000000-0000-0000-0000-0000000000b2'
);

COMMIT;
