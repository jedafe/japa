-- 000582_assessment_travel_date.down.sql
-- Remove the optional travel date field from the default assessment form.

BEGIN;

DELETE FROM custom_fields
WHERE id = '00000000-0000-0000-0000-00000000b006';

COMMIT;
