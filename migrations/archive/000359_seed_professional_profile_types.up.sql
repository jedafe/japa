-- 000359_seed_professional_profile_types.up.sql
-- Section 33: Seed the five system professional profile types for the default
-- tenant. These are the canonical types defined in the section spec:
-- Organization, Agency, Consultancy, Brand, Practice.
--
-- NOTE: 'Student' is intentionally NOT seeded — students are individuals who
-- should use personal profile type + student custom field group (Section 06).

INSERT INTO professional_profile_types (tenant_id, name, slug, icon, is_system)
VALUES
    ('00000000-0000-0000-0000-000000000001', 'Organization', 'organization', 'building', true),
    ('00000000-0000-0000-0000-000000000001', 'Agency', 'agency', 'briefcase', true),
    ('00000000-0000-0000-0000-000000000001', 'Consultancy', 'consultancy', 'lightbulb', true),
    ('00000000-0000-0000-0000-000000000001', 'Brand', 'brand', 'tag', true),
    ('00000000-0000-0000-0000-000000000001', 'Practice', 'practice', 'stethoscope', true)
ON CONFLICT (tenant_id, slug) DO NOTHING;
