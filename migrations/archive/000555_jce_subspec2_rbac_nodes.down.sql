-- 000555_jce_subspec2_rbac_nodes.down.sql

BEGIN;

DELETE FROM role_permissions rp
USING permissions p
WHERE rp.permission_id = p.id
  AND p.key IN (
    'jce.profile.list',
    'jce.profile.view',
    'jce.profile.create',
    'jce.profile.update',
    'jce.profile.delete',
    'jce.score.view',
    'jce.score.recalculate',
    'jce.recommendation.view',
    'jce.factor.manage'
  );

DELETE FROM permissions
WHERE key IN (
    'jce.profile.list',
    'jce.profile.view',
    'jce.profile.create',
    'jce.profile.update',
    'jce.profile.delete',
    'jce.score.view',
    'jce.score.recalculate',
    'jce.recommendation.view',
    'jce.factor.manage'
);

DELETE FROM feature_flags
WHERE key IN ('feature.jce.profiles', 'feature.jce.smart_compare', 'feature.jce.scoring')
  AND tenant_id = '00000000-0000-0000-0000-000000000001';

COMMIT;
