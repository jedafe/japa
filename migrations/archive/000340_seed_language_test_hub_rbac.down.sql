-- 000340_seed_language_test_hub_rbac.down.sql
BEGIN;
DELETE FROM role_permissions
WHERE permission_id IN (SELECT id FROM permissions WHERE key IN (
    'language_test_hub.view','language_test_hub.manage','language_test_hub.story'
));
DELETE FROM permissions WHERE key IN (
    'language_test_hub.view','language_test_hub.manage','language_test_hub.story'
);
COMMIT;
