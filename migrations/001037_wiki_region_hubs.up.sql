-- 001037_wiki_region_hubs.up.sql
-- §wiki-seo — regional wiki hubs: a 'region' wiki-only space type sitting
-- ABOVE country in the declared type hierarchy, with one hub space per UN
-- M49 region (matching the spaces.region values seeded by 000529).
--
-- Effect:
--   * /wiki landing root becomes region (wikiDrillChain walks declared
--     edges): landing shows ~6 region cards instead of the flat A–Z
--     country list — the hub-and-spoke layout the SEO audit asked for.
--   * Every country gains a region ancestor (space_parent_spaces link),
--     so breadcrumbs + BreadcrumbList JSON-LD render
--     Wiki → Africa → Nigeria → … instead of Wiki → Nigeria → ….
--   * Each region gets a 'countries' directory category whose stub page
--     lists its countries — /wiki/europe/countries is the regional hub
--     page the audit recommended.
--
-- The region column on country spaces (UN M49 top-level region) drives
-- the parent links — names match spaces.region exactly.

BEGIN;

-- ---------------------------------------------------------------------------
-- Type + config. Next free wiki-type id after …007e (program, 000971).
-- ---------------------------------------------------------------------------
INSERT INTO space_types (id, name, slug, description, icon, field_group_id, is_system, category, wiki_only) VALUES
    ('00000000-0000-0000-0000-00000000007f', 'Region', 'region',
     'Continental region hub — wiki-only space grouping countries (UN M49)', 'globe',
     NULL, true, 'geo', true)
ON CONFLICT (id) DO NOTHING;

INSERT INTO wiki_type_configs (space_type_id, enabled, label, icon, seed_stubs, position) VALUES
    ('00000000-0000-0000-0000-00000000007f', true, 'Regions', 'globe', true, 0)
ON CONFLICT (space_type_id) DO NOTHING;

-- Declared edge: country may nest under region — required for the
-- ancestor arbitration (getAncestors picks only declared-type parents
-- over junction links) and for wikiDrillChain to root at region.
INSERT INTO wiki_type_parents (space_type_id, parent_space_type_id) VALUES
    ('00000000-0000-0000-0000-000000000060', '00000000-0000-0000-0000-00000000007f') -- country → region
ON CONFLICT DO NOTHING;

-- ---------------------------------------------------------------------------
-- Category seeds for the region type: an 'overview' prose page and a
-- 'countries' directory-mode category whose stub lists descendant
-- country spaces (mirrors the attraction directory pattern, 000795).
-- ---------------------------------------------------------------------------
INSERT INTO custom_field_groups (name, slug, is_system) VALUES
    ('Overview (region)',  'wiki_region_overview',  true),
    ('Countries (region)', 'wiki_region_countries', true)
ON CONFLICT (slug) DO NOTHING;

INSERT INTO wiki_type_categories (id, space_type_id, slug, category, label, field_group_id, position, is_system, rollup, icon)
SELECT gen_random_uuid(), '00000000-0000-0000-0000-00000000007f',
       v.slug, v.category, v.label, g.id, v.position, true, v.rollup, v.icon
FROM (VALUES
    ('overview',  'overview',  'Overview',  1, NULL::jsonb,                                          'info'),
    ('countries', 'countries', 'Countries', 2, '{"enabled":true,"mode":"directory","levels":["country"]}'::jsonb, 'flag')
) AS v(slug, category, label, position, rollup, icon)
JOIN custom_field_groups g ON g.slug = 'wiki_region_' || v.category
ON CONFLICT (space_type_id, slug) DO NOTHING;

INSERT INTO custom_field_group_assignments (group_id, entity_class, entity_id, position)
SELECT DISTINCT wtc.field_group_id, 'space_type:' || st.slug, NULL::uuid, 0
FROM wiki_type_categories wtc
JOIN space_types st ON st.id = wtc.space_type_id
WHERE st.slug = 'region'
  AND NOT EXISTS (
      SELECT 1 FROM custom_field_group_assignments a
      WHERE a.group_id = wtc.field_group_id
        AND a.entity_class = 'space_type:' || st.slug
        AND a.entity_id IS NULL
  );

-- ---------------------------------------------------------------------------
-- The six hub spaces — one per UN M49 top-level region value used on
-- country spaces (000529). level='region' keeps them out of every
-- level IN ('country','state','city') filter; the type is wiki_only so
-- they never surface in the member-facing /spaces directory.
-- ---------------------------------------------------------------------------
INSERT INTO spaces (space_type_id, name, slug, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-00000000007f',
    v.name, v.slug, 'region',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Africa',   'africa'),
    ('Americas', 'americas'),
    ('Asia',     'asia'),
    ('Europe',   'europe'),
    ('Global',   'global'),
    ('Oceania',  'oceania'),
    ('Polar',    'polar')
) AS v(name, slug)
ON CONFLICT DO NOTHING;

-- ---------------------------------------------------------------------------
-- Parent links: each country hangs under the region matching its
-- spaces.region value (junction edge — parent_id stays NULL so the
-- declared-edge arbitration resolves region as the breadcrumb parent).
-- ---------------------------------------------------------------------------
INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT c.id, r.id
FROM spaces c
JOIN spaces r ON r.level = 'region' AND r.name = c.region AND r.deleted_at IS NULL
  AND r.space_type_id = '00000000-0000-0000-0000-00000000007f'
WHERE c.level = 'country' AND c.deleted_at IS NULL AND c.region IS NOT NULL
ON CONFLICT DO NOTHING;

COMMIT;
