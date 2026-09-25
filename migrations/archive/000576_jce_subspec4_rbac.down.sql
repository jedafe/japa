-- 000576_jce_subspec4_rbac.down.sql
-- Reverts 000576: removes role grants, permission nodes, and feature flags.

BEGIN;

DELETE FROM role_permissions rp
USING permissions p
WHERE rp.permission_id = p.id
  AND p.key IN ('jce.data_source.manage','jce.data_source.view','jce.refresh.trigger','jce.ai_advisor.use','jce.ingestion_job.view');

DELETE FROM permissions
WHERE key IN ('jce.data_source.manage','jce.data_source.view','jce.refresh.trigger','jce.ai_advisor.use','jce.ingestion_job.view');

DELETE FROM feature_flags
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND key IN ('feature.jce.data_sources','feature.jce.refresh','feature.jce.ai_advisor');

COMMIT;
