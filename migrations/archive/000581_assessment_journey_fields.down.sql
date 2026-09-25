-- 000581_assessment_journey_fields.down.sql
-- Remove the journey context custom fields from the default assessment form.

BEGIN;

DELETE FROM custom_fields
WHERE id IN (
    '00000000-0000-0000-0000-00000000b001',
    '00000000-0000-0000-0000-00000000b002',
    '00000000-0000-0000-0000-00000000b003',
    '00000000-0000-0000-0000-00000000b004',
    '00000000-0000-0000-0000-00000000b005'
);

COMMIT;
