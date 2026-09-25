-- 000573_template_literals_permission.down.sql
-- Reverses 000573_template_literals_permission.up.sql

BEGIN;

DELETE FROM role_permissions
WHERE permission_id IN (SELECT id FROM permissions WHERE key = 'content.literals.manage');

DELETE FROM permissions WHERE key = 'content.literals.manage';

COMMIT;
