-- 000249_scholarship_finder.up.sql
-- Scholarship Finder (Section 73, ADR-073).
-- Creates the saved_scholarships tracker table, seeds scholarship_finder.*
-- system_settings, RBAC nodes, and registers the scholarship:check_deadlines
-- Asynq task in the task catalog.
--
-- Ref: japaconnect-engineering/sections/73-scholarship-finder.md
--      japaconnect-engineering/adr/ADR-073-scholarship-finder.md

BEGIN;

-- =====================================================================
-- saved_scholarships — per-user scholarship tracker with application
-- status and deadline reminder state.
-- =====================================================================
CREATE TABLE IF NOT EXISTS saved_scholarships (
    id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id           uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    user_id             uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    guide_id            uuid NOT NULL REFERENCES contents(id) ON DELETE CASCADE,
    status              text NOT NULL DEFAULT 'saved'
                          CHECK (status IN ('saved','researching','applied','accepted','rejected','withdrawn')),
    notes               text,
    remind_before_days  int NOT NULL DEFAULT 14,
    last_reminded_at    timestamptz,
    created_at          timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now(),
    UNIQUE(tenant_id, user_id, guide_id)
);

CREATE INDEX IF NOT EXISTS idx_saved_scholarships_user
    ON saved_scholarships (tenant_id, user_id, status);

CREATE INDEX IF NOT EXISTS idx_saved_scholarships_deadline
    ON saved_scholarships (tenant_id, guide_id);

CREATE INDEX IF NOT EXISTS idx_saved_scholarships_remind
    ON saved_scholarships (tenant_id, last_reminded_at)
    WHERE last_reminded_at IS NULL OR last_reminded_at < now() - interval '1 day';

-- =====================================================================
-- system_settings — 3 scholarship_finder.* keys (Section 73, ADR-073)
-- =====================================================================
INSERT INTO system_settings (tenant_id, key, value) VALUES
    ('00000000-0000-0000-0000-000000000001', 'scholarship_finder.enabled',              '"true"'),
    ('00000000-0000-0000-0000-000000000001', 'scholarship_finder.default_reminder_days', '"14"'),
    ('00000000-0000-0000-0000-000000000001', 'scholarship_finder.completion_points',     '"10"')
ON CONFLICT (tenant_id, key) DO NOTHING;

-- =====================================================================
-- RBAC permission nodes — 3 scholarship.* nodes (Section 73, ADR-073)
-- =====================================================================
INSERT INTO permissions (key, description) VALUES
    ('scholarship.browse', 'Browse the scholarship finder (all users incl. anonymous)'),
    ('scholarship.save',   'Save scholarships to tracker (authenticated users)'),
    ('scholarship.manage', 'Admin: view all saves, export data, manage content')
ON CONFLICT (key) DO NOTHING;

-- Grant scholarship.manage to super_admin.
INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug = 'super_admin'
  AND p.key = 'scholarship.manage'
ON CONFLICT DO NOTHING;

-- Grant scholarship.browse + scholarship.save to the default 'member' role.
INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug = 'member'
  AND p.key IN ('scholarship.browse', 'scholarship.save')
ON CONFLICT DO NOTHING;

COMMIT;
