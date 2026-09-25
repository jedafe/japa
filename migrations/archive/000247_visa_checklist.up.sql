-- 000247_visa_checklist.up.sql
-- Visa Document Checklist Generator (Section 70, ADR-070).
-- Creates the checklist_progress table for per-user, per-item progress
-- tracking, and seeds the checklist.* system_settings and RBAC nodes.
--
-- Ref: japaconnect-engineering/sections/70-visa-checklist.md
--      japaconnect-engineering/adr/ADR-070-visa-document-checklist-generator.md

BEGIN;

-- =====================================================================
-- checklist_progress — per-user, per-checklist-item progress state.
-- item_key is a stable slug derived from the document label so that
-- editing a guide's label wording does not orphan saved progress.
-- =====================================================================
CREATE TABLE IF NOT EXISTS checklist_progress (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id    uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    user_id      uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    guide_id     uuid NOT NULL REFERENCES contents(id) ON DELETE CASCADE,
    item_key     text NOT NULL,
    completed    boolean NOT NULL DEFAULT false,
    completed_at timestamptz,
    notes        text,
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now(),
    UNIQUE(tenant_id, user_id, guide_id, item_key)
);

CREATE INDEX IF NOT EXISTS idx_checklist_progress_user
    ON checklist_progress (tenant_id, user_id, guide_id);

CREATE INDEX IF NOT EXISTS idx_checklist_progress_guide
    ON checklist_progress (tenant_id, guide_id);

-- =====================================================================
-- system_settings — 2 checklist.* keys (Section 70, ADR-070)
-- =====================================================================
INSERT INTO system_settings (tenant_id, key, value) VALUES
    ('00000000-0000-0000-0000-000000000001', 'checklist.enabled',           '"true"'),
    ('00000000-0000-0000-0000-000000000001', 'checklist.completion_points', '"0"')
ON CONFLICT (tenant_id, key) DO NOTHING;

-- =====================================================================
-- RBAC permission nodes — 2 checklist.* nodes (Section 70, ADR-070)
-- =====================================================================
INSERT INTO permissions (key, description) VALUES
    ('checklist.use',    'Access the visa checklist tool (all authenticated users)'),
    ('checklist.manage', 'Admin: view all users progress, export data')
ON CONFLICT (key) DO NOTHING;

-- Grant checklist.manage to super_admin; checklist.use is granted to all
-- authenticated users via the default user role (done in a separate seed).
INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug = 'super_admin'
  AND p.key = 'checklist.manage'
ON CONFLICT DO NOTHING;

-- Grant checklist.use to the default 'member' role so all authenticated
-- users can access the tool.
INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug = 'member'
  AND p.key = 'checklist.use'
ON CONFLICT DO NOTHING;

COMMIT;
