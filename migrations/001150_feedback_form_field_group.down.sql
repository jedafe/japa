-- 001150_feedback_form_field_group.down.sql
-- Restores the stale 'Form: Feedback' field group, its two fields, their
-- field permissions, and the duplicate form assignment removed by the up
-- migration. Restoring re-introduces the duplicate-assignment ambiguity the
-- up migration removes — this exists only for rollback symmetry.

BEGIN;

INSERT INTO custom_field_groups (id, name, slug, description, is_system, created_at)
VALUES (
	'6a65521f-971a-46f0-b29d-be139f3e890f',
	'Form: Feedback',
	'form-feedback',
	'',
	false,
	'2026-07-24 16:29:12.593253+00'
)
ON CONFLICT (id) DO NOTHING;

INSERT INTO custom_fields (id, group_id, label, key, field_type, is_required, is_system, show_in_frontend, position, config, created_at)
VALUES
	('cc240431-c4eb-4fa4-9a26-f4ea3f66a89e', '6a65521f-971a-46f0-b29d-be139f3e890f', 'Full Name', 'full_name', 'text', false, false, false, 0, '{}'::jsonb, '2026-07-24 16:29:12.60779+00'),
	('99e4a4d5-64b3-4c62-92f2-98f8f7ee55b2', '6a65521f-971a-46f0-b29d-be139f3e890f', 'Email Address', 'email_address', 'text', false, false, false, 1, '{}'::jsonb, '2026-07-24 16:31:35.976012+00')
ON CONFLICT (group_id, key) DO NOTHING;

INSERT INTO custom_field_permissions (field_id, user_group_id, can_view, can_edit)
SELECT f.id, ug.id, true, ug.slug <> 'guest'
FROM custom_fields f
CROSS JOIN user_groups ug
WHERE f.id IN (
	'cc240431-c4eb-4fa4-9a26-f4ea3f66a89e',
	'99e4a4d5-64b3-4c62-92f2-98f8f7ee55b2'
)
ON CONFLICT (field_id, user_group_id) DO NOTHING;

INSERT INTO custom_field_group_assignments (id, group_id, entity_class, entity_id, position, created_at)
VALUES (
	'e865db8f-09c9-4156-b521-4b4f86f4697f',
	'6a65521f-971a-46f0-b29d-be139f3e890f',
	'form',
	'00000000-0000-0000-0000-000000000047',
	0,
	'2026-07-24 16:29:12.597747+00'
)
ON CONFLICT (group_id, entity_class, entity_id) DO NOTHING;

COMMIT;
