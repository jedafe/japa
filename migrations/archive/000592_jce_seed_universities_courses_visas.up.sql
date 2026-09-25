-- Demo universities, course catalog/offerings, and visa routes for the JCE geo-scoped picker.
-- Ref: japaconnect-engineering/sections/39-space-types-university.md
-- Ref: japaconnect-engineering/sections/90-visa-route-comparison.md

BEGIN;

-- University spaces. Use ON CONFLICT to reuse any demo spaces that already
-- exist under different ids and RETURNING to capture the real ids for the
-- downstream foreign keys.
WITH
    sys_tenant AS (
        SELECT '00000000-0000-0000-0000-000000000001'::uuid AS id
    ),
    stypes AS (
        SELECT id, slug
        FROM space_types
        WHERE tenant_id = (SELECT id FROM sys_tenant) AND slug = 'university'
    ),
    unilag_city AS (
        SELECT id
        FROM spaces
        WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'lagos' AND level = 'city'
        LIMIT 1
    ),
    unilag AS (
        INSERT INTO spaces (id, tenant_id, space_type_id, parent_id, name, slug, level, geo_code)
        SELECT '00000000-0000-0000-0000-000000000401',
               (SELECT id FROM sys_tenant),
               (SELECT id FROM stypes),
               (SELECT id FROM unilag_city),
               'University of Lagos', 'university-of-lagos', 'university', 'NG'
        ON CONFLICT (tenant_id, slug) DO UPDATE SET
            parent_id = EXCLUDED.parent_id,
            space_type_id = EXCLUDED.space_type_id,
            level = EXCLUDED.level,
            geo_code = EXCLUDED.geo_code
        RETURNING id, parent_id
    ),
    cape_coast_city AS (
        SELECT id
        FROM spaces
        WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'accra' AND level = 'city'
        LIMIT 1
    ),
    ucc AS (
        INSERT INTO spaces (id, tenant_id, space_type_id, parent_id, name, slug, level, geo_code)
        SELECT '00000000-0000-0000-0000-000000000402',
               (SELECT id FROM sys_tenant),
               (SELECT id FROM stypes),
               (SELECT id FROM cape_coast_city),
               'University of Cape Coast', 'university-of-cape-coast', 'university', 'GH'
        ON CONFLICT (tenant_id, slug) DO UPDATE SET
            parent_id = EXCLUDED.parent_id,
            space_type_id = EXCLUDED.space_type_id,
            level = EXCLUDED.level,
            geo_code = EXCLUDED.geo_code
        RETURNING id, parent_id
    )
SELECT 1;

-- Populate the many-to-many parent relationships for the demo universities.
INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, s.parent_id
FROM spaces s
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND s.level = 'university'
  AND s.slug IN ('university-of-lagos', 'university-of-cape-coast')
ON CONFLICT DO NOTHING;

-- Course catalog
INSERT INTO university_course_catalog (id, tenant_id, title, slug, description, degree_type, department, field_of_study, language, default_duration_years)
VALUES
    ('00000000-0000-0000-0000-000000000501', '00000000-0000-0000-0000-000000000001', 'BSc Computer Science', 'bsc-computer-science', 'Undergraduate computer science programme.', 'BSc', 'Computer Science', 'Technology', 'English', 4),
    ('00000000-0000-0000-0000-000000000502', '00000000-0000-0000-0000-000000000001', 'BSc Electrical Engineering', 'bsc-electrical-engineering', 'Undergraduate electrical engineering programme.', 'BSc', 'Engineering', 'Engineering', 'English', 5),
    ('00000000-0000-0000-0000-000000000503', '00000000-0000-0000-0000-000000000001', 'MBA', 'mba', 'Master of Business Administration.', 'MBA', 'Business School', 'Business', 'English', 2)
ON CONFLICT (tenant_id, slug) DO NOTHING;

-- Course offerings, using the real ids of the demo universities.
WITH
    unilag AS (
        SELECT id FROM spaces
        WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
          AND slug = 'university-of-lagos'
          AND level = 'university'
        LIMIT 1
    ),
    ucc AS (
        SELECT id FROM spaces
        WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
          AND slug = 'university-of-cape-coast'
          AND level = 'university'
        LIMIT 1
    )
INSERT INTO university_course_offerings (id, tenant_id, course_id, university_id, slug, tuition, currency, duration_years, study_mode, intake_months, entry_requirements, accreditation)
SELECT '00000000-0000-0000-0000-000000000601'::uuid,
       '00000000-0000-0000-0000-000000000001'::uuid,
       '00000000-0000-0000-0000-000000000501'::uuid,
       unilag.id,
       'unilag-bsc-computer-science', 2500, 'USD', 4, 'Full-time', ARRAY['September', 'January'], 'SSCE / equivalent', 'NUC accredited'
FROM unilag
UNION ALL
SELECT '00000000-0000-0000-0000-000000000602'::uuid,
       '00000000-0000-0000-0000-000000000001'::uuid,
       '00000000-0000-0000-0000-000000000502'::uuid,
       unilag.id,
       'unilag-bsc-electrical-engineering', 3000, 'USD', 5, 'Full-time', ARRAY['September'], 'SSCE / equivalent', 'NUC accredited'
FROM unilag
UNION ALL
SELECT '00000000-0000-0000-0000-000000000603'::uuid,
       '00000000-0000-0000-0000-000000000001'::uuid,
       '00000000-0000-0000-0000-000000000501'::uuid,
       ucc.id,
       'ucc-bsc-computer-science', 1800, 'USD', 4, 'Full-time', ARRAY['August'], 'WASSCE / equivalent', 'NAB accredited'
FROM ucc
ON CONFLICT (tenant_id, slug) DO NOTHING;

-- Visa routes for Nigeria, Ghana and Kenya as destination countries.
WITH
    nigeria AS (
        SELECT id FROM spaces
        WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
          AND slug = 'nigeria'
          AND level = 'country'
        LIMIT 1
    ),
    ghana AS (
        SELECT id FROM spaces
        WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
          AND slug = 'ghana'
          AND level = 'country'
        LIMIT 1
    ),
    kenya AS (
        SELECT id FROM spaces
        WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
          AND slug = 'kenya'
          AND level = 'country'
        LIMIT 1
    )
INSERT INTO visa_routes (id, tenant_id, destination_space_id, visa_category, name, slug, eligibility_summary, requirements_json, typical_fee_usd, gov_source_url, status, is_active)
SELECT '00000000-0000-0000-0000-000000000701'::uuid,
       '00000000-0000-0000-0000-000000000001'::uuid,
       nigeria.id,
       'work', 'Nigeria Talent Visa', 'nigeria-talent-visa',
       'Skilled workers and remote professionals can live and work in Nigeria.',
       '{"income": 24000, "work_allowed": true, "pr_eligible": true, "family_included": true}'::jsonb,
       250.00, 'https://example.com/ng-visa', 'published', true
FROM nigeria
UNION ALL
SELECT '00000000-0000-0000-0000-000000000702'::uuid,
       '00000000-0000-0000-0000-000000000001'::uuid,
       ghana.id,
       'work', 'Ghana Work Permit', 'ghana-work-permit',
       'Work permit for skilled professionals and investors.',
       '{"income": 18000, "work_allowed": true, "pr_eligible": true, "family_included": true}'::jsonb,
       200.00, 'https://example.com/gh-visa', 'published', true
FROM ghana
UNION ALL
SELECT '00000000-0000-0000-0000-000000000703'::uuid,
       '00000000-0000-0000-0000-000000000001'::uuid,
       kenya.id,
       'student', 'Kenya Student Visa', 'kenya-student-visa',
       'Student visa for full-time degree programmes.',
       '{"income": 0, "work_allowed": false, "pr_eligible": false, "family_included": false}'::jsonb,
       150.00, 'https://example.com/ke-visa', 'published', true
FROM kenya
ON CONFLICT (tenant_id, destination_space_id, slug) DO NOTHING;

COMMIT;
