-- 000323_seed_service_directory_rbac.down.sql
BEGIN;
DELETE FROM role_permissions WHERE permission_id IN (
    SELECT id FROM permissions WHERE key IN
    ('service.create','service.edit_own','service.edit_any','service.verify','service.claim'));
DELETE FROM permissions WHERE key IN
    ('service.create','service.edit_own','service.edit_any','service.verify','service.claim');
COMMIT;
