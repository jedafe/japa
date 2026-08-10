-- 000364_seed_cost_of_living_baseline.up.sql
-- Seeds the Lagos baseline city, the 9 cost attribute dimensions, and baseline
-- values so the Cost of Living Calculator (Section 72, ADR-072) is functional
-- on a fresh database. Also adds a slug column to comparison_dimensions so the
-- weighted Overall Cost Index can look up config weights by dimension slug
-- (BUG-002: the config JSONB weights are keyed by slug, but the service was
-- looking them up by UUID).
--
-- Ref: japaconnect-engineering/sections/72-cost-of-living.md
--      japaconnect-engineering/adr/ADR-072-cost-of-living-calculator.md
--      japaconnect-engineering/sections/30-comparison-system.md

BEGIN;

-- =====================================================================
-- comparison_dimensions: add a slug column (machine key for weight lookup).
-- §30 dimensions previously had only a human label; a slug lets the COL
-- service match dimension rows to the config JSONB weight keys. Existing
-- dimensions get a derived slug; NULLs are allowed for back-compat.
-- =====================================================================
ALTER TABLE comparison_dimensions ADD COLUMN IF NOT EXISTS slug text;
CREATE INDEX IF NOT EXISTS idx_comparison_dimensions_slug
    ON comparison_dimensions (tenant_id, content_id, slug);

-- =====================================================================
-- Seed a placeholder contents row (content_type='comparison') that owns the
-- cost_of_living dimensions + the Lagos baseline item. comparison_items and
-- comparison_dimensions both require content_id NOT NULL (§30 schema).
-- Author = super admin (jedafeglobal@gmail.com, seeded by 000124).
-- Fixed UUID so the down migration + later references are deterministic.
-- =====================================================================
INSERT INTO contents (id, tenant_id, author_id, content_type, title, slug, body, status, visibility)
SELECT
    '00000000-0000-0000-0000-000000000c01',
    '00000000-0000-0000-0000-000000000001',
    u.id,
    'comparison',
    'Cost of Living — Baseline Data',
    'cost-of-living-baseline',
    'System-seeded baseline city (Lagos) and 9 cost attribute dimensions for the Cost of Living Calculator.',
    'published',
    'public'
FROM users u
WHERE u.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND u.email = 'jedafeglobal@gmail.com'
LIMIT 1
ON CONFLICT (id) DO NOTHING;

-- =====================================================================
-- Seed the 9 cost attribute dimensions under the placeholder content.
-- Slugs match the config JSONB weight keys in migration 000248 so the
-- weighted Overall Cost Index (ADR-072 §2) can look up weights by slug.
-- dimension_type='currency' for monetary cost categories.
-- =====================================================================
INSERT INTO comparison_dimensions (id, tenant_id, content_id, slug, label, dimension_type, position)
VALUES
    ('00000000-0000-0000-0000-000000000d01', '00000000-0000-0000-0000-000000000001', '00000000-0000-0000-0000-000000000c01', 'rent_1br_city_centre',  'Rent (1BR, city centre)',           'currency', 1),
    ('00000000-0000-0000-0000-000000000d02', '00000000-0000-0000-0000-000000000001', '00000000-0000-0000-0000-000000000c01', 'rent_1br_outside',      'Rent (1BR, outside centre)',        'currency', 2),
    ('00000000-0000-0000-0000-000000000d03', '00000000-0000-0000-0000-000000000001', '00000000-0000-0000-0000-000000000c01', 'groceries_monthly',     'Groceries (monthly, 1 person)',     'currency', 3),
    ('00000000-0000-0000-0000-000000000d04', '00000000-0000-0000-0000-000000000001', '00000000-0000-0000-0000-000000000c01', 'restaurant_meal_cheap', 'Restaurant meal (inexpensive)',     'currency', 4),
    ('00000000-0000-0000-0000-000000000d05', '00000000-0000-0000-0000-000000000001', '00000000-0000-0000-0000-000000000c01', 'transport_monthly',     'Public transport (monthly pass)',   'currency', 5),
    ('00000000-0000-0000-0000-000000000d06', '00000000-0000-0000-0000-000000000001', '00000000-0000-0000-0000-000000000c01', 'utilities_monthly',     'Utilities (monthly, 85m² apt)',     'currency', 6),
    ('00000000-0000-0000-0000-000000000d07', '00000000-0000-0000-0000-000000000001', '00000000-0000-0000-0000-000000000c01', 'internet_monthly',      'Internet (60Mbps+)',                'currency', 7),
    ('00000000-0000-0000-0000-000000000d08', '00000000-0000-0000-0000-000000000001', '00000000-0000-0000-0000-000000000c01', 'gym_monthly',           'Gym membership',                    'currency', 8),
    ('00000000-0000-0000-0000-000000000d09', '00000000-0000-0000-0000-000000000001', '00000000-0000-0000-0000-000000000c01', 'avg_net_salary',        'Average net monthly salary',        'currency', 9)
ON CONFLICT (id) DO NOTHING;

-- =====================================================================
-- Seed Lagos as the baseline city (comparison_item, is_baseline=true).
-- Links to the Lagos state geo space (slug='lagos', level='state') if it
-- exists; otherwise geo_space_id is NULL (calculator still works, just no
-- geo space name shown). ADR-072 §4: "Lagos is seeded with is_baseline=true".
-- =====================================================================
INSERT INTO comparison_items (id, tenant_id, content_id, title, description, geo_space_id, position, is_baseline, last_updated_at)
SELECT
    '00000000-0000-0000-0000-000000000b01',
    '00000000-0000-0000-0000-000000000001',
    '00000000-0000-0000-0000-000000000c01',
    'Lagos',
    'Lagos, Nigeria — default baseline city for cost of living comparison.',
    s.id,
    0,
    true,
    now()
FROM (SELECT id FROM spaces WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'lagos' AND level = 'state' LIMIT 1) s
ON CONFLICT (id) DO NOTHING;

-- If the Lagos state space does not exist, insert with NULL geo_space_id.
INSERT INTO comparison_items (id, tenant_id, content_id, title, description, geo_space_id, position, is_baseline, last_updated_at)
SELECT
    '00000000-0000-0000-0000-000000000b01',
    '00000000-0000-0000-0000-000000000001',
    '00000000-0000-0000-0000-000000000c01',
    'Lagos',
    'Lagos, Nigeria — default baseline city for cost of living comparison.',
    NULL,
    0,
    true,
    now()
WHERE NOT EXISTS (SELECT 1 FROM comparison_items WHERE id = '00000000-0000-0000-0000-000000000b01')
ON CONFLICT (id) DO NOTHING;

-- =====================================================================
-- Seed baseline cost values for Lagos (in USD per ADR-072 §1).
-- Values are illustrative starting points; admin updates them via the §30
-- comparison item edit UI. The service parses the text `value` column.
-- =====================================================================
INSERT INTO comparison_values (dimension_id, item_id, tenant_id, value)
VALUES
    ('00000000-0000-0000-0000-000000000d01', '00000000-0000-0000-0000-000000000b01', '00000000-0000-0000-0000-000000000001', '800'),
    ('00000000-0000-0000-0000-000000000d02', '00000000-0000-0000-0000-000000000b01', '00000000-0000-0000-0000-000000000001', '450'),
    ('00000000-0000-0000-0000-000000000d03', '00000000-0000-0000-0000-000000000b01', '00000000-0000-0000-0000-000000000001', '250'),
    ('00000000-0000-0000-0000-000000000d04', '00000000-0000-0000-0000-000000000b01', '00000000-0000-0000-0000-000000000001', '3'),
    ('00000000-0000-0000-0000-000000000d05', '00000000-0000-0000-0000-000000000b01', '00000000-0000-0000-0000-000000000001', '25'),
    ('00000000-0000-0000-0000-000000000d06', '00000000-0000-0000-0000-000000000b01', '00000000-0000-0000-0000-000000000001', '60'),
    ('00000000-0000-0000-0000-000000000d07', '00000000-0000-0000-0000-000000000b01', '00000000-0000-0000-0000-000000000001', '30'),
    ('00000000-0000-0000-0000-000000000d08', '00000000-0000-0000-0000-000000000b01', '00000000-0000-0000-0000-000000000001', '40'),
    ('00000000-0000-0000-0000-000000000d09', '00000000-0000-0000-0000-000000000b01', '00000000-0000-0000-0000-000000000001', '350')
ON CONFLICT (dimension_id, item_id) DO NOTHING;

COMMIT;
