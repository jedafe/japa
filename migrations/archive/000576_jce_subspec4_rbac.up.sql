-- 000576_jce_subspec4_rbac.up.sql
-- JCE Sub-spec 4: feature flags, permission nodes, and default role grants.
-- The actual schema uses permissions + role_permissions (not rbac_nodes + permission_grants).
-- Ref: docs/superpowers/specs/2026-08-30-jce-subspec-4-design.md

BEGIN;

INSERT INTO feature_flags (key, tenant_id, enabled, config)
VALUES
    ('feature.jce.data_sources', '00000000-0000-0000-0000-000000000001', false, '{}'),
    ('feature.jce.refresh',      '00000000-0000-0000-0000-000000000001', false, '{}'),
    ('feature.jce.ai_advisor',   '00000000-0000-0000-0000-000000000001', false, '{}')
ON CONFLICT (key, tenant_id) DO UPDATE SET
    enabled = EXCLUDED.enabled,
    config = EXCLUDED.config,
    updated_at = now();

INSERT INTO permissions (key, description) VALUES
    ('jce.data_source.manage', 'Manage comparison data sources - Create, update, and delete data source definitions'),
    ('jce.data_source.view',   'View comparison data sources - List and view data source definitions'),
    ('jce.refresh.trigger',    'Trigger comparison refresh - Enqueue a refresh of comparison values'),
    ('jce.ai_advisor.use',     'Use JCE AI advisor - Ask the AI advisor questions about a comparison'),
    ('jce.ingestion_job.view', 'View ingestion jobs - View refresh job history')
ON CONFLICT (key) DO UPDATE SET description = EXCLUDED.description;

INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug IN ('super_admin', 'admin')
  AND p.key IN ('jce.data_source.manage','jce.data_source.view','jce.refresh.trigger','jce.ai_advisor.use','jce.ingestion_job.view')
ON CONFLICT DO NOTHING;

COMMIT;
