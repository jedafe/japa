-- 000555_jce_subspec2_rbac_nodes.up.sql
-- JCE Sub-spec 2/3: RBAC permission nodes for profiles, scores, factors and recommendations.

BEGIN;

INSERT INTO permissions (key, description) VALUES
    ('jce.profile.list', 'List comparison profiles'),
    ('jce.profile.view', 'View a comparison profile'),
    ('jce.profile.create', 'Create a comparison profile'),
    ('jce.profile.update', 'Update a comparison profile'),
    ('jce.profile.delete', 'Delete a comparison profile'),
    ('jce.score.view', 'View JapaScore for a comparison'),
    ('jce.score.recalculate', 'Recalculate JapaScore for a comparison'),
    ('jce.recommendation.view', 'View comparison recommendations'),
    ('jce.factor.manage', 'Manage score factors and scoring rules')
ON CONFLICT (key) DO UPDATE SET description = EXCLUDED.description;

-- Grant profile and read access to member/admin/super_admin; factor management to admin/super_admin.
INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND (
      (p.key IN ('jce.profile.list', 'jce.profile.view', 'jce.profile.create', 'jce.profile.update', 'jce.profile.delete',
                 'jce.score.view', 'jce.score.recalculate', 'jce.recommendation.view')
       AND ur.slug IN ('member', 'admin', 'super_admin'))
      OR (p.key = 'jce.factor.manage' AND ur.slug IN ('admin', 'super_admin'))
  )
ON CONFLICT DO NOTHING;

-- Feature flags for JCE Sub-spec 2 surfaces (gated off by default).
INSERT INTO feature_flags (key, tenant_id, enabled, config)
VALUES
    ('feature.jce.profiles',       '00000000-0000-0000-0000-000000000001', false, '{}'),
    ('feature.jce.smart_compare',  '00000000-0000-0000-0000-000000000001', false, '{}'),
    ('feature.jce.scoring',        '00000000-0000-0000-0000-000000000001', false, '{}')
ON CONFLICT (key, tenant_id) DO UPDATE SET
    enabled = EXCLUDED.enabled,
    config = EXCLUDED.config,
    updated_at = now();

COMMIT;
