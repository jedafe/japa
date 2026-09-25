-- 000638_seed_video_claim_rbac_nodes.down.sql

BEGIN;

DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT id FROM permissions WHERE key IN ('admin.video_claims.read', 'admin.video_claims.manage')
);

DELETE FROM permissions
WHERE key IN ('admin.video_claims.read', 'admin.video_claims.manage');

COMMIT;
