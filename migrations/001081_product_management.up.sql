-- 001081_product_management.up.sql
-- Product management (§80): product↔deal links, product-type field-group
-- assignments (entity_class = type slug), 'product' cfv entity type, and
-- dedicated products.* / product_types.* RBAC nodes.
BEGIN;

CREATE TABLE product_deals (
    product_id uuid NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    coupon_id  uuid NOT NULL REFERENCES coupons(id) ON DELETE CASCADE,
    position   integer NOT NULL DEFAULT 0,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (product_id, coupon_id)
);
CREATE INDEX idx_product_deals_coupon ON product_deals (coupon_id);

-- Type → group association: every wiki_{type}_{category} group bound to its
-- product type slug so GET /custom-field-groups?entity_class={slug} resolves
-- the type's field schema (§80: "entity_class = product type slug"). The
-- (group_id, entity_class, entity_id) unique key does not dedupe NULL
-- entity_id rows, so the NOT EXISTS guard carries idempotency.
INSERT INTO custom_field_group_assignments (group_id, entity_class, entity_id, position)
SELECT g.id, pt.slug, NULL, 0
FROM product_types pt
JOIN custom_field_groups g
  ON g.slug LIKE 'wiki\_' || replace(pt.slug, '-', '_') || '\_%' ESCAPE '\'
WHERE NOT EXISTS (
    SELECT 1 FROM custom_field_group_assignments a
    WHERE a.group_id = g.id AND a.entity_class = pt.slug AND a.entity_id IS NULL
);

-- Register 'product' as a cfv entity type.
INSERT INTO custom_field_entity_types (value, label, is_system, sort_order)
VALUES ('product', 'Product', true, 8)
ON CONFLICT (value) DO NOTHING;

-- RBAC nodes (BE-005: full seed).
INSERT INTO permissions (key, description) VALUES
    ('products.list',    'List products (admin)'),
    ('products.view',    'View product detail incl. providers/deals/custom fields'),
    ('products.create',  'Create products'),
    ('products.update',  'Update products incl. provider/deal links'),
    ('products.delete',  'Delete/restore products'),
    ('product_types.list',   'List product types'),
    ('product_types.view',   'View product type incl. bound field groups'),
    ('product_types.create', 'Create product types (provisions field groups + comparison type)'),
    ('product_types.update', 'Update product types (label/icon/enabled/categories)'),
    ('product_types.delete', 'Delete product types with no products')
ON CONFLICT (key) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.slug IN ('super_admin', 'super_admin_role')
  AND p.key IN (
    'products.list','products.view','products.create','products.update','products.delete',
    'product_types.list','product_types.view','product_types.create',
    'product_types.update','product_types.delete'
  )
ON CONFLICT DO NOTHING;

COMMIT;
