-- 000417_seed_reports_rbac_nodes.up.sql
-- Seeds the reports.submit and reports.view_own permission nodes defined in
-- JEOS Section 51 (Content Reports & Flagging) but absent from the permissions
-- table. Grants them to the member role (default: all verified members / all
-- users per the section) so submit routes can enforce reports.submit instead
-- of the generic moderation.report node.
-- Ref: japaconnect-engineering/sections/51-content-reports-flagging.md

BEGIN;

-- 1. Ensure the permission nodes exist.
INSERT INTO permissions (key, description) VALUES
    ('reports.submit', 'Submit content reports (default: all verified members)'),
    ('reports.view_own', 'View own submitted reports (default: all)')
ON CONFLICT (key) DO NOTHING;

-- 2. Grant them to the super_admin role (super_admin gets every node).
INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug = 'super_admin'
  AND p.key IN ('reports.submit', 'reports.view_own')
ON CONFLICT DO NOTHING;

-- 3. Grant them to the member role (standard member capabilities).
INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug = 'member'
  AND p.key IN ('reports.submit', 'reports.view_own')
ON CONFLICT DO NOTHING;

-- 4. Seed the report.new notification type (Section 51 report:notify_admin).
--    Notifies moderators (users with reports.review) that a new report was
--    submitted. In-app + push so the moderation queue gets prompt attention.
INSERT INTO notification_types (key, label, description, default_in_app, default_email, default_push) VALUES
    ('report.new', 'New Report', 'A new content report was submitted', true, false, true)
ON CONFLICT (key) DO NOTHING;

COMMIT;
