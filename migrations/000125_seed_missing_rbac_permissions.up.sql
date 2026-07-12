-- 000125_seed_missing_rbac_permissions.up.sql
-- Seeds 19 permission nodes enforced in routes but absent from all prior
-- migrations, and grants them to the super_admin_role so that the super_admin
-- group has sitewide access to every feature and setting.

BEGIN;

-- 1. Ensure the permission nodes exist.
INSERT INTO permissions (key, description) VALUES
    ('admin.analytics.search', 'Search analytics data'),
    ('admin.users.list', 'List platform users'),
    ('admin.users.manage', 'View user sessions and activity'),
    ('user.delete', 'Delete user accounts'),
    ('affiliate.admin.view', 'View affiliate admin dashboard'),
    ('affiliate.admin.manage', 'Manage affiliate admin settings'),
    ('affiliate.providers.manage', 'Manage affiliate providers'),
    ('coupons.manage', 'Manage discount coupons'),
    ('moderation.alerts.manage', 'View and acknowledge moderation alerts'),
    ('spaces.moderator.assign', 'Assign moderators to spaces'),
    ('ai.context.admin_delete_entry', 'Delete AI context entries'),
    ('control.panel.access', 'Access super-admin control panel'),
    ('custom_fields.group.view', 'View custom field groups'),
    ('forms.submissions.delete', 'Delete form submissions'),
    ('forms.submissions.edit', 'Edit form submission status'),
    ('reports.review', 'Review and action content reports'),
    ('tags.create', 'Create general tags'),
    ('tags.delete', 'Delete general tags'),
    ('tags.edit', 'Edit general tags')
ON CONFLICT (key) DO NOTHING;

-- 2. Grant them to the super_admin_role (idempotent).
INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug = 'super_admin_role'
  AND p.key IN (
    'admin.analytics.search', 'admin.users.list', 'admin.users.manage',
    'user.delete', 'affiliate.admin.view', 'affiliate.admin.manage',
    'affiliate.providers.manage', 'coupons.manage',
    'moderation.alerts.manage', 'spaces.moderator.assign',
    'ai.context.admin_delete_entry', 'control.panel.access',
    'custom_fields.group.view', 'forms.submissions.delete',
    'forms.submissions.edit', 'reports.review',
    'tags.create', 'tags.delete', 'tags.edit'
  )
ON CONFLICT DO NOTHING;

COMMIT;
