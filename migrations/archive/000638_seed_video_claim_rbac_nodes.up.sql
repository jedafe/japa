-- 000638_seed_video_claim_rbac_nodes.up.sql
-- Add RBAC nodes for the video channel claim admin queue.

BEGIN;

INSERT INTO permissions (key, description) VALUES
    ('admin.video_claims.read', 'View video channel claim admin queue'),
    ('admin.video_claims.manage', 'Review and manage video channel claims')
ON CONFLICT (key) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.slug = 'super_admin'
  AND p.key IN ('admin.video_claims.read', 'admin.video_claims.manage')
ON CONFLICT DO NOTHING;

COMMIT;
