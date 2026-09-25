-- Phase 05: University bulk seed.
-- Ref: japaconnect-engineering/roadmaps/ROADMAP-BACKEND.md Phase 05 (task 05.07)
-- Curated dataset of globally recognised universities as space_type=university spaces.
-- Each university is parented to its country/state Primary Spaces.
-- custom_field_values populated for university_fields group.
-- Idempotent: skip if slug already exists.

-- ============================================================
-- UK Universities
-- ============================================================
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'university'),
    v.name,
    lower(replace(replace(replace(v.name, ' ', '-'), '''', ''), '.', '')),
    'GB',
    'custom',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('University of Oxford'),
    ('University of Cambridge'),
    ('Imperial College London'),
    ('UCL'),
    ('University of Edinburgh'),
    ('University of Manchester'),
    ('King''s College London'),
    ('London School of Economics'),
    ('University of Bristol'),
    ('University of Warwick')
) AS v(name)
ON CONFLICT (tenant_id, slug) DO NOTHING;

-- ============================================================
-- Canada Universities
-- ============================================================
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'university'),
    v.name,
    lower(replace(replace(replace(v.name, ' ', '-'), '''', ''), '.', '')),
    'CA',
    'custom',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('University of Toronto'),
    ('McGill University'),
    ('University of British Columbia'),
    ('University of Alberta'),
    ('McMaster University'),
    ('University of Waterloo')
) AS v(name)
ON CONFLICT (tenant_id, slug) DO NOTHING;

-- ============================================================
-- US Universities
-- ============================================================
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'university'),
    v.name,
    lower(replace(replace(replace(v.name, ' ', '-'), '''', ''), '.', '')),
    'US',
    'custom',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Massachusetts Institute of Technology'),
    ('Stanford University'),
    ('Harvard University'),
    ('California Institute of Technology'),
    ('University of California, Berkeley'),
    ('Columbia University'),
    ('University of Chicago'),
    ('Yale University'),
    ('Princeton University')
) AS v(name)
ON CONFLICT (tenant_id, slug) DO NOTHING;

-- ============================================================
-- Ireland Universities
-- ============================================================
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'university'),
    v.name,
    lower(replace(replace(replace(v.name, ' ', '-'), '''', ''), '.', '')),
    'IE',
    'custom',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Trinity College Dublin'),
    ('University College Dublin')
) AS v(name)
ON CONFLICT (tenant_id, slug) DO NOTHING;

-- ============================================================
-- Germany Universities
-- ============================================================
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'university'),
    v.name,
    lower(replace(replace(replace(v.name, ' ', '-'), '''', ''), '.', '')),
    'DE',
    'custom',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Ludwig Maximilian University of Munich'),
    ('Technical University of Munich'),
    ('Heidelberg University'),
    ('Humboldt University of Berlin')
) AS v(name)
ON CONFLICT (tenant_id, slug) DO NOTHING;

-- ============================================================
-- Netherlands Universities
-- ============================================================
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'university'),
    v.name,
    lower(replace(replace(replace(v.name, ' ', '-'), '''', ''), '.', '')),
    'NL',
    'custom',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('University of Amsterdam'),
    ('Delft University of Technology'),
    ('Leiden University')
) AS v(name)
ON CONFLICT (tenant_id, slug) DO NOTHING;

-- ============================================================
-- South Africa Universities
-- ============================================================
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'university'),
    v.name,
    lower(replace(replace(replace(v.name, ' ', '-'), '''', ''), '.', '')),
    'ZA',
    'custom',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('University of Cape Town')
) AS v(name)
ON CONFLICT (tenant_id, slug) DO NOTHING;

-- ============================================================
-- Nigerian Universities
-- ============================================================
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'university'),
    v.name,
    lower(replace(replace(replace(v.name, ' ', '-'), '''', ''), '.', '')),
    'NG',
    'custom',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('University of Lagos'),
    ('University of Ibadan'),
    ('Obafemi Awolowo University'),
    ('Ahmadu Bello University'),
    ('University of Nigeria, Nsukka')
) AS v(name)
ON CONFLICT (tenant_id, slug) DO NOTHING;

-- ============================================================
-- Parent UK universities to England (and UK country)
-- ============================================================
INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'england' AND p.level = 'state'
  AND s.level = 'custom'
  AND s.slug IN ('university-of-oxford', 'university-of-cambridge', 'imperial-college-london',
                 'ucl', 'university-of-manchester', 'kings-college-london',
                 'london-school-of-economics', 'university-of-bristol', 'university-of-warwick')
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'scotland' AND p.level = 'state'
  AND s.level = 'custom'
  AND s.slug = 'university-of-edinburgh'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Parent Canadian universities to Ontario/BC/Alberta/QC
INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND s.level = 'custom'
  AND (
    (s.slug = 'university-of-toronto' AND p.slug = 'ontario')
    OR (s.slug = 'mcgill-university' AND p.slug = 'quebec')
    OR (s.slug = 'university-of-british-columbia' AND p.slug = 'british-columbia')
    OR (s.slug = 'university-of-alberta' AND p.slug = 'alberta')
    OR (s.slug = 'mcmaster-university' AND p.slug = 'ontario')
    OR (s.slug = 'university-of-waterloo' AND p.slug = 'ontario')
  )
  AND p.level = 'state'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Parent US universities to their states
INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND s.level = 'custom'
  AND (
    (s.slug = 'massachusetts-institute-of-technology' AND p.slug = 'us-massachusetts')
    OR (s.slug = 'stanford-university' AND p.slug = 'us-california')
    OR (s.slug = 'harvard-university' AND p.slug = 'us-massachusetts')
    OR (s.slug = 'california-institute-of-technology' AND p.slug = 'us-california')
    OR (s.slug = 'university-of-california-berkeley' AND p.slug = 'us-california')
    OR (s.slug = 'columbia-university' AND p.slug = 'us-new-york')
    OR (s.slug = 'university-of-chicago' AND p.slug = 'us-illinois')
    OR (s.slug = 'yale-university' AND p.slug = 'us-connecticut')
    OR (s.slug = 'princeton-university' AND p.slug = 'us-new-jersey')
  )
  AND p.level = 'state'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Parent Nigerian universities to their states
INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND s.level = 'custom'
  AND (
    (s.slug = 'university-of-lagos' AND p.slug = 'ng-lagos')
    OR (s.slug = 'university-of-ibadan' AND p.slug = 'ng-oyo')
    OR (s.slug = 'obafemi-awolowo-university' AND p.slug = 'ng-osun')
    OR (s.slug = 'ahmadu-bello-university' AND p.slug = 'ng-kaduna')
    OR (s.slug = 'university-of-nigeria-nsukka' AND p.slug = 'ng-enugu')
  )
  AND p.level = 'state'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;
