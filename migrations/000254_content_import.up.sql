-- 000254_content_import.up.sql
-- Content Import & Migration Tools (Section 65, ADR-083).
-- Creates the content_imports tracking table and seeds import.*
-- system_settings + RBAC nodes.
--
-- Ref: japaconnect-engineering/sections/65-content-import.md
--      japaconnect-engineering/adr/ADR-083-content-import.md

BEGIN;

-- =====================================================================
-- content_imports — tracking table for user content imports from
-- Medium, Substack, RSS, and ZIP exports.
-- =====================================================================
CREATE TABLE IF NOT EXISTS content_imports (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id      uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    user_id        uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    source_type    text NOT NULL CHECK (source_type IN ('medium','substack','rss','zip')),
    source_url     text,
    status         text NOT NULL DEFAULT 'pending'
                     CHECK (status IN ('pending','processing','completed','failed','partial')),
    total_items    int NOT NULL DEFAULT 0,
    imported_count int NOT NULL DEFAULT 0,
    failed_count   int NOT NULL DEFAULT 0,
    error_log      jsonb NOT NULL DEFAULT '[]',
    created_at     timestamptz NOT NULL DEFAULT now(),
    completed_at   timestamptz
);

CREATE INDEX IF NOT EXISTS idx_content_imports_user
    ON content_imports (tenant_id, user_id, status);

CREATE INDEX IF NOT EXISTS idx_content_imports_tenant
    ON content_imports (tenant_id, status, created_at DESC);

-- =====================================================================
-- system_settings — 1 import.* key (Section 65, ADR-083)
-- =====================================================================
INSERT INTO system_settings (tenant_id, key, value) VALUES
    ('00000000-0000-0000-0000-000000000001', 'import.max_items_per_run', '"50"')
ON CONFLICT (tenant_id, key) DO NOTHING;

-- =====================================================================
-- RBAC permission nodes — 2 import.* nodes (Section 65, ADR-083)
-- =====================================================================
INSERT INTO permissions (key, description) VALUES
    ('import.use',    'Start content imports (all authenticated users)'),
    ('import.manage', 'Admin: view all imports, retry failed')
ON CONFLICT (key) DO NOTHING;

-- Grant import.manage to super_admin.
INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug = 'super_admin'
  AND p.key = 'import.manage'
ON CONFLICT DO NOTHING;

-- Grant import.use to the default 'member' role.
INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug = 'member'
  AND p.key = 'import.use'
ON CONFLICT DO NOTHING;

COMMIT;
