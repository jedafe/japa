-- 000344_seed_community_locator_rbac.down.sql
BEGIN;
DELETE FROM role_permissions
WHERE permission_id IN (SELECT id FROM permissions WHERE key IN (
    'community_locator.submit','community_locator.review',
    'community_locator.claim','community_locator.manage'
));
DELETE FROM permissions WHERE key IN (
    'community_locator.submit','community_locator.review',
    'community_locator.claim','community_locator.manage'
);
COMMIT;
