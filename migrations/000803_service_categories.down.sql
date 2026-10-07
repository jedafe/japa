-- 000803_service_categories.down.sql
BEGIN;
DELETE FROM role_permissions WHERE permission_id IN (SELECT id FROM permissions WHERE key = 'service.categories.manage');
DELETE FROM permissions WHERE key = 'service.categories.manage';
DROP TABLE IF EXISTS service_categories;
COMMIT;
