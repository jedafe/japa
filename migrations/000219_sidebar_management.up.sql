-- 000219_sidebar_management.up.sql
-- Sidebar Management System — dedicated tables modeled on ADR-054 (Navigation Menus).
-- Replaces the deprecated page_type='sidebar' approach from migration 000217.
-- Ref: Section 59 (Sidebar Management System), ADR-066

BEGIN;

-- ── sidebars ──────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS sidebars (
    id                 uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id          uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    app                text NOT NULL CHECK (app IN ('main', 'docs', 'learn')),
    position           text NOT NULL CHECK (position IN ('left', 'right')),
    slug               text NOT NULL,
    name               text NOT NULL,
    page_pattern       text NOT NULL DEFAULT '*' CHECK (page_pattern <> ''),
    visibility         text NOT NULL DEFAULT 'all' CHECK (visibility IN ('all', 'authenticated', 'guest')),
    required_group_ids jsonb NOT NULL DEFAULT '[]'::jsonb,
    status             text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'inactive')),
    created_at         timestamptz NOT NULL DEFAULT now(),
    updated_at         timestamptz NOT NULL DEFAULT now(),
    UNIQUE(tenant_id, app, position, slug)
);

CREATE INDEX IF NOT EXISTS idx_sidebars_resolve
    ON sidebars(tenant_id, app, position, status);

-- ── sidebar_blocks ────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS sidebar_blocks (
    id                 uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id          uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    sidebar_id         uuid NOT NULL REFERENCES sidebars(id) ON DELETE CASCADE,
    block_type         text NOT NULL,
    label              text NOT NULL DEFAULT '',
    props              jsonb NOT NULL DEFAULT '{}'::jsonb,
    sort_order         int NOT NULL DEFAULT 0,
    visibility         text NOT NULL DEFAULT 'all' CHECK (visibility IN ('all', 'authenticated', 'guest')),
    required_group_ids jsonb NOT NULL DEFAULT '[]'::jsonb,
    permission_node    text,
    status             text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'inactive')),
    created_at         timestamptz NOT NULL DEFAULT now(),
    updated_at         timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_sidebar_blocks_sidebar
    ON sidebar_blocks(tenant_id, sidebar_id, sort_order);

-- ── RBAC: sidebars.manage permission node ─────────────────────────────
INSERT INTO permissions (key, description) VALUES
    ('sidebars.manage', 'Manage sidebars and sidebar blocks across all apps')
ON CONFLICT (key) DO NOTHING;

-- Grant to super_admin_role and admin_role (idempotent).
INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug IN ('super_admin_role', 'admin_role')
  AND p.key = 'sidebars.manage'
ON CONFLICT DO NOTHING;

-- ── Deprecation cleanup: drop orphaned _sidebar_left / _sidebar_right pages ─
-- These were seeded by migration 000217. The pages.page_type column is kept
-- (removing a column from a hot table violates the zero-downtime rule); only
-- the orphaned sidebar page rows are removed.
DELETE FROM pages
WHERE page_type = 'sidebar'
  AND slug IN ('_sidebar_left', '_sidebar_right');

COMMIT;
