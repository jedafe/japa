-- 000199_seed_system_comparison_types.up.sql
-- Seed the pre-seeded system comparison types (Section 30, BUG-005).
-- Linked to existing custom_field_groups for the default tenant.
-- IDs are fixed UUIDs so the down migration can remove them deterministically.

INSERT INTO comparison_type_definitions (id, tenant_id, name, slug, description, field_group_id, item_label, is_system)
VALUES
    ('00000000-0000-0000-0000-0000000000a1', '00000000-0000-0000-0000-000000000001', 'Universities', 'universities', 'Compare universities and study programs', '00000000-0000-0000-0000-000000000044', 'University', true),
    ('00000000-0000-0000-0000-0000000000a2', '00000000-0000-0000-0000-000000000001', 'Visa Types', 'visa-types', 'Compare visa categories and requirements', '00000000-0000-0000-0000-000000000040', 'Visa', true),
    ('00000000-0000-0000-0000-0000000000a3', '00000000-0000-0000-0000-000000000001', 'Banks', 'banks', 'Compare banking services and fees', '00000000-0000-0000-0000-000000000045', 'Bank', true),
    ('00000000-0000-0000-0000-0000000000a4', '00000000-0000-0000-0000-000000000001', 'Courses', 'courses', 'Compare courses and training programs', '00000000-0000-0000-0000-000000000044', 'Course', true)
ON CONFLICT (tenant_id, slug) DO NOTHING;
