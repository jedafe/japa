-- 000270_university_track_categories.up.sql
-- Creates the university_track_categories table for admin-managed track categories
-- and seeds the university.category.manage RBAC permission node.
-- Ref: japaconnect-engineering/sections/36-learning-platform.md (Track categories)
--      japaconnect-engineering/conventions/permission-key-naming.md (Japa University)

BEGIN;

-- 1. Create the university_track_categories table.
CREATE TABLE IF NOT EXISTS university_track_categories (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    slug        text NOT NULL,
    name        text NOT NULL,
    sort_order  int  NOT NULL DEFAULT 0,
    is_active   boolean NOT NULL DEFAULT true,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),
    UNIQUE(tenant_id, slug)
);

CREATE INDEX idx_university_track_categories_tenant
    ON university_track_categories(tenant_id);

-- 2. Seed the university.category.manage permission node.
INSERT INTO permissions (key, description) VALUES
    ('university.category.manage', 'Manage university track categories (create, edit, delete, reorder)')
ON CONFLICT (key) DO NOTHING;

-- 3. Grant to super_admin_role (idempotent).
INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug = 'super_admin_role'
  AND p.key = 'university.category.manage'
ON CONFLICT DO NOTHING;

-- 4. Seed a few default categories for the default tenant.
INSERT INTO university_track_categories (tenant_id, slug, name, sort_order) VALUES
    ('00000000-0000-0000-0000-000000000001', 'software-engineering', 'Software Engineering', 10),
    ('00000000-0000-0000-0000-000000000001', 'data-science',         'Data Science',         20),
    ('00000000-0000-0000-0000-000000000001', 'product-design',       'Product Design',       30),
    ('00000000-0000-0000-0000-000000000001', 'business',             'Business',             40),
    ('00000000-0000-0000-0000-000000000001', 'marketing',            'Marketing',            50)
ON CONFLICT (tenant_id, slug) DO NOTHING;

COMMIT;
