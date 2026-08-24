-- 000548_seed_campaign_rbac.up.sql
-- Section 104 (AI Content Campaigns) — RBAC permission nodes.
--
-- Ref: .jeos/sections/104-content-campaigns.md

BEGIN;

INSERT INTO permissions (key, description) VALUES
    ('campaign.create', 'Create content campaigns'),
    ('campaign.view', 'View campaigns (read-only)'),
    ('campaign.manage', 'Full campaign management'),
    ('campaign.run', 'Run a planned campaign'),
    ('campaign.retry', 'Retry failed campaign items'),
    ('campaign.cancel', 'Cancel a pending campaign')
ON CONFLICT (key) DO UPDATE SET description = EXCLUDED.description;

-- Grant campaign.* to admin and super_admin on the default tenant.
INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug IN ('admin', 'super_admin')
  AND p.key IN ('campaign.create', 'campaign.view', 'campaign.manage', 'campaign.run', 'campaign.retry', 'campaign.cancel')
ON CONFLICT DO NOTHING;

COMMIT;
