-- 001150_feedback_form_field_group.up.sql
-- Fixes /feedback rendering an empty form: the 'feedback' form carries TWO
-- custom_field_group_assignments rows — the canonical 'Feedback Form Fields'
-- group (0000...0050, seeded by 000399: Category + Details, guest-viewable)
-- and a stale 'Form: Feedback' group (6a65521f-...) auto-provisioned by
-- EnsureFormGroup before 000399 landed, whose Full Name/Email Address fields
-- are all show_in_frontend=false. GetAssignmentByEntity is a :one lookup with
-- no ordering, so Postgres returns the stale group and the CF13 guest filter
-- strips every field → GET /forms/feedback/embed returns fields: [].
-- §06 models one field group per form, so the stale assignment + orphan
-- group are removed. No submissions or field values reference it.
--
-- Ref: .jeos/sections/06-custom-field-builder-form-builder.md (one group per form)

BEGIN;

DELETE FROM custom_field_group_assignments
WHERE group_id = '6a65521f-971a-46f0-b29d-be139f3e890f'
  AND entity_class = 'form'
  AND entity_id = '00000000-0000-0000-0000-000000000047';

DELETE FROM custom_field_permissions
WHERE field_id IN (
	'cc240431-c4eb-4fa4-9a26-f4ea3f66a89e',
	'99e4a4d5-64b3-4c62-92f2-98f8f7ee55b2'
);

DELETE FROM custom_fields WHERE group_id = '6a65521f-971a-46f0-b29d-be139f3e890f';

DELETE FROM custom_field_groups WHERE id = '6a65521f-971a-46f0-b29d-be139f3e890f';

COMMIT;
