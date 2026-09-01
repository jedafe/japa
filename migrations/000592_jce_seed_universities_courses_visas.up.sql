-- Demo universities, course catalog/offerings, and visa routes for the JCE geo-scoped picker.
-- Ref: japaconnect-engineering/sections/39-space-types-university.md
-- Ref: japaconnect-engineering/sections/90-visa-route-comparison.md

BEGIN;

-- University spaces
WITH sys_tenant AS (
    SELECT '00000000-0000-0000-0000-000000000001'::uuid AS id
),
stypes AS (
    SELECT id, slug FROM space_types
    WHERE tenant_id = (SELECT id FROM sys_tenant) AND slug = 'university'
),
unilag_city AS (
    SELECT id FROM spaces
    WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'lagos' AND level = 'city'
    LIMIT 1
),
unilag AS (
    INSERT INTO spaces (id, tenant_id, space_type_id, parent_id, name, slug, level, geo_code)
    SELECT '00000000-0000-0000-0000-000000000401',
           '00000000-0000-0000-0000-000000000001',
           (SELECT id FROM stypes),
           (SELECT id FROM unilag_city),
           'University of Lagos', 'university-of-lagos', 'university', 'NG'
    ON CONFLICT (tenant_id, slug) DO UPDATE SET parent_id = EXCLUDED.parent_id
    RETURNING id
),
cape_coast_city AS (
    SELECT id FROM spaces
    WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'accra' AND level = 'city'
    LIMIT 1
),
ucc AS (
    INSERT INTO spaces (id, tenant_id, space_type_id, parent_id, name, slug, level, geo_code)
    SELECT '00000000-0000-0000-0000-000000000402',
           '00000000-0000-0000-0000-000000000001',
           (SELECT id FROM stypes),
           (SELECT id FROM cape_coast_city),
           'University of Cape Coast', 'university-of-cape-coast', 'university', 'GH'
    ON CONFLICT (tenant_id, slug) DO UPDATE SET parent_id = EXCLUDED.parent_id
    RETURNING id
)
SELECT 1;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, s.parent_id
FROM spaces s
WHERE s.id IN ('00000000-0000-0000-0000-000000000401', '00000000-0000-0000-0000-000000000402')
ON CONFLICT DO NOTHING;

-- Course catalog
INSERT INTO university_course_catalog (id, tenant_id, title, slug, description, degree_type, department, field_of_study, language, default_duration_years)
VALUES
    ('00000000-0000-0000-0000-000000000501', '00000000-0000-0000-0000-000000000001', 'BSc Computer Science', 'bsc-computer-science', 'Undergraduate computer science programme.', 'BSc', 'Computer Science', 'Technology', 'English', 4),
    ('00000000-0000-0000-0000-000000000502', '00000000-0000-0000-0000-000000000001', 'BSc Electrical Engineering', 'bsc-electrical-engineering', 'Undergraduate electrical engineering programme.', 'BSc', 'Engineering', 'Engineering', 'English', 5),
    ('00000000-0000-0000-0000-000000000503', '00000000-0000-0000-0000-000000000001', 'MBA', 'mba', 'Master of Business Administration.', 'MBA', 'Business School', 'Business', 'English', 2)
ON CONFLICT (tenant_id, slug) DO NOTHING;

-- Course offerings
INSERT INTO university_course_offerings (id, tenant_id, course_id, university_id, slug, tuition, currency, duration_years, study_mode, intake_months, entry_requirements, accreditation)
VALUES
    ('00000000-0000-0000-0000-000000000601', '00000000-0000-0000-0000-000000000001',
     '00000000-0000-0000-0000-000000000501', '00000000-0000-0000-0000-000000000401',
     'unilag-bsc-computer-science', 2500, 'USD', 4, 'Full-time', ARRAY['September', 'January'], 'SSCE / equivalent', 'NUC accredited'),
    ('00000000-0000-0000-0000-000000000602', '00000000-0000-0000-0000-000000000001',
     '00000000-0000-0000-0000-000000000502', '00000000-0000-0000-0000-000000000401',
     'unilag-bsc-electrical-engineering', 3000, 'USD', 5, 'Full-time', ARRAY['September'], 'SSCE / equivalent', 'NUC accredited'),
    ('00000000-0000-0000-0000-000000000603', '00000000-0000-0000-0000-000000000001',
     '00000000-0000-0000-0000-000000000501', '00000000-0000-0000-0000-000000000402',
     'ucc-bsc-computer-science', 1800, 'USD', 4, 'Full-time', ARRAY['August'], 'WASSCE / equivalent', 'NAB accredited')
ON CONFLICT (tenant_id, slug) DO NOTHING;

-- Visa routes for Nigeria, Ghana and Kenya as destination countries.
INSERT INTO visa_routes (id, tenant_id, destination_space_id, visa_category, name, slug, eligibility_summary, requirements_json, typical_fee_usd, gov_source_url, status, is_active)
VALUES
    ('00000000-0000-0000-0000-000000000701', '00000000-0000-0000-0000-000000000001',
     '00000000-0000-0000-0000-000000000101', 'work', 'Nigeria Talent Visa', 'nigeria-talent-visa',
     'Skilled workers and remote professionals can live and work in Nigeria.',
     '{"income": 24000, "work_allowed": true, "pr_eligible": true, "family_included": true}'::jsonb,
     250.00, 'https://example.com/ng-visa', 'published', true),
    ('00000000-0000-0000-0000-000000000702', '00000000-0000-0000-0000-000000000001',
     '00000000-0000-0000-0000-000000000102', 'work', 'Ghana Work Permit', 'ghana-work-permit',
     'Work permit for skilled professionals and investors.',
     '{"income": 18000, "work_allowed": true, "pr_eligible": true, "family_included": true}'::jsonb,
     200.00, 'https://example.com/gh-visa', 'published', true),
    ('00000000-0000-0000-0000-000000000703', '00000000-0000-0000-0000-000000000001',
     '00000000-0000-0000-0000-000000000103', 'student', 'Kenya Student Visa', 'kenya-student-visa',
     'Student visa for full-time degree programmes.',
     '{"income": 0, "work_allowed": false, "pr_eligible": false, "family_included": false}'::jsonb,
     150.00, 'https://example.com/ke-visa', 'published', true)
ON CONFLICT (tenant_id, destination_space_id, slug) DO NOTHING;

COMMIT;
