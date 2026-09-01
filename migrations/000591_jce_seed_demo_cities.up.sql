-- Demo geo spaces and cost_of_living items for the JCE geo-scoped picker.
-- Ref: japaconnect-engineering/sections/72-cost-of-living.md
-- Ref: japaconnect-engineering/sections/30-generic-comparison-system.md

BEGIN;

-- Fixed demo UUIDs. Keep deterministic so the down migration can clean up cleanly.
-- Country codes use ISO-3166 alpha-2 for the spaces that carry them.

WITH
    sys_tenant AS (
        SELECT '00000000-0000-0000-0000-000000000001'::uuid AS id
    ),
    stypes AS (
        SELECT id, slug
        FROM space_types
        WHERE tenant_id = (SELECT id FROM sys_tenant)
          AND slug IN ('country', 'state', 'city', 'university')
    ),
    nigeria AS (
        INSERT INTO spaces (id, tenant_id, space_type_id, name, slug, level, geo_code, region, subregion)
        SELECT '00000000-0000-0000-0000-000000000101',
               (SELECT id FROM sys_tenant),
               (SELECT id FROM stypes WHERE slug = 'country'),
               'Nigeria', 'nigeria', 'country', 'NG', 'Africa', 'West Africa'
        ON CONFLICT (tenant_id, slug) DO UPDATE SET
            region = EXCLUDED.region, subregion = EXCLUDED.subregion, geo_code = EXCLUDED.geo_code
        RETURNING id
    ),
    ghana AS (
        INSERT INTO spaces (id, tenant_id, space_type_id, name, slug, level, geo_code, region, subregion)
        SELECT '00000000-0000-0000-0000-000000000102',
               (SELECT id FROM sys_tenant),
               (SELECT id FROM stypes WHERE slug = 'country'),
               'Ghana', 'ghana', 'country', 'GH', 'Africa', 'West Africa'
        ON CONFLICT (tenant_id, slug) DO UPDATE SET
            region = EXCLUDED.region, subregion = EXCLUDED.subregion, geo_code = EXCLUDED.geo_code
        RETURNING id
    ),
    kenya AS (
        INSERT INTO spaces (id, tenant_id, space_type_id, name, slug, level, geo_code, region, subregion)
        SELECT '00000000-0000-0000-0000-000000000103',
               (SELECT id FROM sys_tenant),
               (SELECT id FROM stypes WHERE slug = 'country'),
               'Kenya', 'kenya', 'country', 'KE', 'Africa', 'East Africa'
        ON CONFLICT (tenant_id, slug) DO UPDATE SET
            region = EXCLUDED.region, subregion = EXCLUDED.subregion, geo_code = EXCLUDED.geo_code
        RETURNING id
    ),
    lagos_state AS (
        INSERT INTO spaces (id, tenant_id, space_type_id, parent_id, name, slug, level, geo_code)
        SELECT '00000000-0000-0000-0000-000000000201',
               (SELECT id FROM sys_tenant),
               (SELECT id FROM stypes WHERE slug = 'state'),
               (SELECT id FROM nigeria), 'Lagos State', 'lagos-state', 'state', 'NG-LA'
        ON CONFLICT (tenant_id, slug) DO UPDATE SET parent_id = EXCLUDED.parent_id
        RETURNING id
    ),
    fct AS (
        INSERT INTO spaces (id, tenant_id, space_type_id, parent_id, name, slug, level, geo_code)
        SELECT '00000000-0000-0000-0000-000000000202',
               (SELECT id FROM sys_tenant),
               (SELECT id FROM stypes WHERE slug = 'state'),
               (SELECT id FROM nigeria), 'Federal Capital Territory', 'federal-capital-territory', 'state', 'NG-FC'
        ON CONFLICT (tenant_id, slug) DO UPDATE SET parent_id = EXCLUDED.parent_id
        RETURNING id
    ),
    accra_region AS (
        INSERT INTO spaces (id, tenant_id, space_type_id, parent_id, name, slug, level, geo_code)
        SELECT '00000000-0000-0000-0000-000000000203',
               (SELECT id FROM sys_tenant),
               (SELECT id FROM stypes WHERE slug = 'state'),
               (SELECT id FROM ghana), 'Greater Accra', 'greater-accra', 'state', 'GH-AA'
        ON CONFLICT (tenant_id, slug) DO UPDATE SET parent_id = EXCLUDED.parent_id
        RETURNING id
    ),
    nairobi_county AS (
        INSERT INTO spaces (id, tenant_id, space_type_id, parent_id, name, slug, level, geo_code)
        SELECT '00000000-0000-0000-0000-000000000204',
               (SELECT id FROM sys_tenant),
               (SELECT id FROM stypes WHERE slug = 'state'),
               (SELECT id FROM kenya), 'Nairobi County', 'nairobi-county', 'state', 'KE-30'
        ON CONFLICT (tenant_id, slug) DO UPDATE SET parent_id = EXCLUDED.parent_id
        RETURNING id
    ),
    lagos_city AS (
        INSERT INTO spaces (id, tenant_id, space_type_id, parent_id, name, slug, level, geo_code)
        SELECT '00000000-0000-0000-0000-000000000301',
               (SELECT id FROM sys_tenant),
               (SELECT id FROM stypes WHERE slug = 'city'),
               (SELECT id FROM lagos_state), 'Lagos', 'lagos', 'city', 'NG'
        ON CONFLICT (tenant_id, slug) DO UPDATE SET parent_id = EXCLUDED.parent_id, level = EXCLUDED.level
        RETURNING id
    ),
    abuja_city AS (
        INSERT INTO spaces (id, tenant_id, space_type_id, parent_id, name, slug, level, geo_code)
        SELECT '00000000-0000-0000-0000-000000000302',
               (SELECT id FROM sys_tenant),
               (SELECT id FROM stypes WHERE slug = 'city'),
               (SELECT id FROM fct), 'Abuja', 'abuja', 'city', 'NG'
        ON CONFLICT (tenant_id, slug) DO UPDATE SET parent_id = EXCLUDED.parent_id
        RETURNING id
    ),
    accra_city AS (
        INSERT INTO spaces (id, tenant_id, space_type_id, parent_id, name, slug, level, geo_code)
        SELECT '00000000-0000-0000-0000-000000000303',
               (SELECT id FROM sys_tenant),
               (SELECT id FROM stypes WHERE slug = 'city'),
               (SELECT id FROM accra_region), 'Accra', 'accra', 'city', 'GH'
        ON CONFLICT (tenant_id, slug) DO UPDATE SET parent_id = EXCLUDED.parent_id
        RETURNING id
    ),
    nairobi_city AS (
        INSERT INTO spaces (id, tenant_id, space_type_id, parent_id, name, slug, level, geo_code)
        SELECT '00000000-0000-0000-0000-000000000304',
               (SELECT id FROM sys_tenant),
               (SELECT id FROM stypes WHERE slug = 'city'),
               (SELECT id FROM nairobi_county), 'Nairobi', 'nairobi', 'city', 'KE'
        ON CONFLICT (tenant_id, slug) DO UPDATE SET parent_id = EXCLUDED.parent_id
        RETURNING id
    )
SELECT 1;

-- Populate the many-to-many parent relationships for all inserted demo cities.
INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT c.id, c.parent_id
FROM spaces c
WHERE c.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND c.level = 'city'
  AND c.slug IN ('lagos', 'abuja', 'accra', 'nairobi')
ON CONFLICT DO NOTHING;

-- The existing baseline Lagos item is linked to a state-level space. Repoint it to the city space.
UPDATE comparison_items
SET geo_space_id = (SELECT id FROM spaces WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'lagos' AND level = 'city' LIMIT 1)
WHERE id = '00000000-0000-0000-0000-000000000b01';

-- Ensure the baseline Lagos item carries the correct cost_of_living type.
UPDATE comparison_items
SET type_id = (SELECT id FROM comparison_type_definitions WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'cost_of_living')
WHERE id = '00000000-0000-0000-0000-000000000b01';

-- Insert cost_of_living items for the other demo cities.
INSERT INTO comparison_items (id, tenant_id, content_id, type_id, title, description, geo_space_id, position, is_baseline, last_updated_at)
VALUES
    ('00000000-0000-0000-0000-000000000b02', '00000000-0000-0000-0000-000000000001', '00000000-0000-0000-0000-000000000c01',
     (SELECT id FROM comparison_type_definitions WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'cost_of_living'),
     'Abuja', 'Abuja, Nigeria — cost of living baseline.', (SELECT id FROM spaces WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'abuja' AND level = 'city' LIMIT 1), 1, false, now()),
    ('00000000-0000-0000-0000-000000000b03', '00000000-0000-0000-0000-000000000001', '00000000-0000-0000-0000-000000000c01',
     (SELECT id FROM comparison_type_definitions WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'cost_of_living'),
     'Accra', 'Accra, Ghana — cost of living baseline.', (SELECT id FROM spaces WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'accra' AND level = 'city' LIMIT 1), 2, false, now()),
    ('00000000-0000-0000-0000-000000000b04', '00000000-0000-0000-0000-000000000001', '00000000-0000-0000-0000-000000000c01',
     (SELECT id FROM comparison_type_definitions WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'cost_of_living'),
     'Nairobi', 'Nairobi, Kenya — cost of living baseline.', (SELECT id FROM spaces WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'nairobi' AND level = 'city' LIMIT 1), 3, false, now())
ON CONFLICT (id) DO NOTHING;

-- Seed cost values for the new cities using the 9 baseline dimensions.
INSERT INTO comparison_values (dimension_id, item_id, tenant_id, value)
SELECT d.id, i.id, i.tenant_id, v.value
FROM comparison_items i
CROSS JOIN comparison_dimensions d
CROSS JOIN LATERAL (
    VALUES
        ('rent_1br_city_centre',  CASE WHEN i.title = 'Lagos' THEN '800' WHEN i.title = 'Abuja' THEN '700' WHEN i.title = 'Accra' THEN '650' WHEN i.title = 'Nairobi' THEN '600' END),
        ('rent_1br_outside',      CASE WHEN i.title = 'Lagos' THEN '450' WHEN i.title = 'Abuja' THEN '400' WHEN i.title = 'Accra' THEN '350' WHEN i.title = 'Nairobi' THEN '300' END),
        ('groceries_monthly',     CASE WHEN i.title = 'Lagos' THEN '250' WHEN i.title = 'Abuja' THEN '240' WHEN i.title = 'Accra' THEN '230' WHEN i.title = 'Nairobi' THEN '220' END),
        ('restaurant_meal_cheap', CASE WHEN i.title = 'Lagos' THEN '3'   WHEN i.title = 'Abuja' THEN '4'   WHEN i.title = 'Accra' THEN '3.5' WHEN i.title = 'Nairobi' THEN '3' END),
        ('transport_monthly',     CASE WHEN i.title = 'Lagos' THEN '25'  WHEN i.title = 'Abuja' THEN '30'  WHEN i.title = 'Accra' THEN '20'  WHEN i.title = 'Nairobi' THEN '25' END),
        ('utilities_monthly',     CASE WHEN i.title = 'Lagos' THEN '60'  WHEN i.title = 'Abuja' THEN '70'  WHEN i.title = 'Accra' THEN '55'  WHEN i.title = 'Nairobi' THEN '50' END),
        ('internet_monthly',      CASE WHEN i.title = 'Lagos' THEN '30'  WHEN i.title = 'Abuja' THEN '35'  WHEN i.title = 'Accra' THEN '30'  WHEN i.title = 'Nairobi' THEN '25' END),
        ('gym_monthly',           CASE WHEN i.title = 'Lagos' THEN '40'  WHEN i.title = 'Abuja' THEN '45'  WHEN i.title = 'Accra' THEN '35'  WHEN i.title = 'Nairobi' THEN '30' END),
        ('avg_net_salary',        CASE WHEN i.title = 'Lagos' THEN '350' WHEN i.title = 'Abuja' THEN '450' WHEN i.title = 'Accra' THEN '400' WHEN i.title = 'Nairobi' THEN '350' END)
) AS v(slug, value)
WHERE i.id IN ('00000000-0000-0000-0000-000000000b02', '00000000-0000-0000-0000-000000000b03', '00000000-0000-0000-0000-000000000b04')
  AND d.slug = v.slug
  AND d.tenant_id = i.tenant_id
ON CONFLICT (dimension_id, item_id) DO NOTHING;

COMMIT;
