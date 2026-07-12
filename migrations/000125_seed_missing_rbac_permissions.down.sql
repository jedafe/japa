-- 000125_seed_missing_rbac_permissions.down.sql
-- Removes the 19 permissions seeded by the up migration.

BEGIN;

DELETE FROM role_permissions
WHERE role_id IN (
    SELECT id FROM user_roles
    WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
      AND slug = 'super_admin_role'
)
AND permission_id IN (
    SELECT id FROM permissions
    WHERE key IN (
        'admin.analytics.search', 'admin.users.list', 'admin.users.manage',
        'user.delete', 'affiliate.admin.view', 'affiliate.admin.manage',
        'affiliate.providers.manage', 'coupons.manage',
        'moderation.alerts.manage', 'spaces.moderator.assign',
        'ai.context.admin_delete_entry', 'control.panel.access',
        'custom_fields.group.view', 'forms.submissions.delete',
        'forms.submissions.edit', 'reports.review',
        'tags.create', 'tags.delete', 'tags.edit'
    )
);

DELETE FROM permissions
WHERE key IN (
    'admin.analytics.search', 'admin.users.list', 'admin.users.manage',
    'user.delete', 'affiliate.admin.view', 'affiliate.admin.manage',
    'affiliate.providers.manage', 'coupons.manage',
    'moderation.alerts.manage', 'spaces.moderator.assign',
    'ai.context.admin_delete_entry', 'control.panel.access',
    'custom_fields.group.view', 'forms.submissions.delete',
    'forms.submissions.edit', 'reports.review',
    'tags.create', 'tags.delete', 'tags.edit'
);

COMMIT;
