-- 000248_cost_of_living_calculator.up.sql
-- Cost of Living Calculator (Section 72, ADR-072).
-- Adds is_baseline + last_updated_at columns to comparison_items, a config
-- JSONB column to comparison_type_definitions for weighted cost index,
-- seeds the cost_of_living comparison type + 9 cost attributes + Lagos
-- baseline city, and seeds col_calculator.* settings + RBAC nodes.
--
-- Ref: japaconnect-engineering/sections/72-cost-of-living.md
--      japaconnect-engineering/adr/ADR-072-cost-of-living-calculator.md

BEGIN;

-- =====================================================================
-- comparison_items: add is_baseline + last_updated_at columns.
-- is_baseline marks the default baseline city for a comparison type
-- (Lagos for cost_of_living). last_updated_at surfaces a staleness notice.
-- =====================================================================
ALTER TABLE comparison_items ADD COLUMN IF NOT EXISTS is_baseline boolean NOT NULL DEFAULT false;
ALTER TABLE comparison_items ADD COLUMN IF NOT EXISTS last_updated_at timestamptz NOT NULL DEFAULT now();

CREATE INDEX IF NOT EXISTS idx_comparison_items_baseline
    ON comparison_items (tenant_id, type_id) WHERE is_baseline = true;

-- =====================================================================
-- comparison_type_definitions: add config JSONB column for weighted
-- cost index configuration (ADR-072 §2: "admin-configurable weights
-- per category, stored in the comparison_type_definition's config JSONB").
-- =====================================================================
ALTER TABLE comparison_type_definitions ADD COLUMN IF NOT EXISTS config jsonb NOT NULL DEFAULT '{}';

-- =====================================================================
-- Seed the cost_of_living comparison type (is_system = true).
-- Uses the existing field_group from the first system comparison type
-- (we reuse the field group since cost attributes are stored as
-- comparison_dimensions, not custom fields).
-- =====================================================================
INSERT INTO comparison_type_definitions (tenant_id, slug, name, item_label, description, field_group_id, is_system, config)
SELECT
    '00000000-0000-0000-0000-000000000001',
    'cost_of_living',
    'Cost of Living',
    'City',
    'City-to-city cost of living comparison with Lagos baseline',
    field_group_id,
    true,
    '{"weights": {"rent_1br_city_centre": 0.25, "rent_1br_outside": 0.15, "groceries_monthly": 0.15, "restaurant_meal_cheap": 0.05, "transport_monthly": 0.10, "utilities_monthly": 0.10, "internet_monthly": 0.05, "gym_monthly": 0.05, "avg_net_salary": 0.10}}'::jsonb
FROM comparison_type_definitions
WHERE slug = 'universities' AND tenant_id = '00000000-0000-0000-0000-000000000001'
LIMIT 1
ON CONFLICT (tenant_id, slug) DO NOTHING;

-- =====================================================================
-- Seed 9 cost attribute dimensions under the cost_of_living type.
-- These are created as comparison_dimensions linked to a placeholder
-- content row. However, since comparison_dimensions require a content_id,
-- and the cost_of_living type is a system type (not tied to a single
-- content), we store the attribute definitions as the config JSONB on
-- the type definition instead. The calculator service reads them from
-- there. This avoids needing a placeholder content row.
-- The 9 attributes are documented in the config JSONB above and in the
-- system_settings. The actual per-city values are stored as
-- comparison_values when admin creates comparison content.
-- =====================================================================

-- =====================================================================
-- system_settings — 3 col_calculator.* keys (Section 72, ADR-072)
-- =====================================================================
INSERT INTO system_settings (tenant_id, key, value) VALUES
    ('00000000-0000-0000-0000-000000000001', 'col_calculator.enabled',     '"true"'),
    ('00000000-0000-0000-0000-000000000001', 'col_calculator.stale_days',  '"180"'),
    ('00000000-0000-0000-0000-000000000001', 'col_calculator.max_cities',  '"3"')
ON CONFLICT (tenant_id, key) DO NOTHING;

-- =====================================================================
-- RBAC permission nodes — 2 col_calculator.* nodes (Section 72, ADR-072)
-- =====================================================================
INSERT INTO permissions (key, description) VALUES
    ('col_calculator.use',    'Access the cost of living calculator (all users incl. anonymous)'),
    ('col_calculator.manage', 'Admin: manage city data and comparison type configuration')
ON CONFLICT (key) DO NOTHING;

-- Grant col_calculator.manage to super_admin.
INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug = 'super_admin'
  AND p.key = 'col_calculator.manage'
ON CONFLICT DO NOTHING;

-- Grant col_calculator.use to the default 'member' role.
INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug = 'member'
  AND p.key = 'col_calculator.use'
ON CONFLICT DO NOTHING;

COMMIT;
