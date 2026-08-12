-- Down migration for widget_configurations (Section 16).
DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT id FROM permissions WHERE key = 'widgets.manage'
);
DELETE FROM permissions WHERE key = 'widgets.manage';
DROP TABLE IF EXISTS widget_configurations;
