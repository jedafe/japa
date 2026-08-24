-- 000548_seed_campaign_rbac.down.sql

BEGIN;

DELETE FROM role_permissions rp
USING permissions p
WHERE rp.permission_id = p.id
  AND p.key IN (
    'campaign.create',
    'campaign.view',
    'campaign.manage',
    'campaign.run',
    'campaign.retry',
    'campaign.cancel'
  );

DELETE FROM permissions
WHERE key IN (
    'campaign.create',
    'campaign.view',
    'campaign.manage',
    'campaign.run',
    'campaign.retry',
    'campaign.cancel'
);

COMMIT;
