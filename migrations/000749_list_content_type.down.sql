-- 000749_list_content_type.down.sql
BEGIN;
DELETE FROM role_permissions
WHERE permission_id IN (SELECT id FROM permissions WHERE key IN ('list.create','agent.content_type.list'));
DELETE FROM permissions WHERE key IN ('list.create','agent.content_type.list');
DELETE FROM content_type_jsonld_templates WHERE content_type = 'list';
COMMIT;
