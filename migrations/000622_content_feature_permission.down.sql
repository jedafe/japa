-- 000622_content_feature_permission.down.sql
-- Remove the generic content featuring node and its grants.

BEGIN;

DELETE FROM role_permissions rp
USING permissions p
WHERE rp.permission_id = p.id AND p.key = 'content.feature';

DELETE FROM permissions WHERE key = 'content.feature';

COMMIT;
