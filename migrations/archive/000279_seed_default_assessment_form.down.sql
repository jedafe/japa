-- 000279_seed_default_assessment_form.down.sql
-- Reverse of 000279: remove the seeded assessment form, its field group, and
-- the group-to-form assignment. custom_fields cascade from the group via
-- ON DELETE CASCADE; the assignment cascades from the group too.
BEGIN;

DELETE FROM forms
WHERE id = '00000000-0000-0000-0000-00000000a002';

DELETE FROM custom_field_groups
WHERE id = '00000000-0000-0000-0000-00000000a001';

COMMIT;
