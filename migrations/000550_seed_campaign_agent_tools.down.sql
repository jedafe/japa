-- 000550_seed_campaign_agent_tools.down.sql

BEGIN;

DELETE FROM ai_tools
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND name IN ('personas.create', 'content.campaign.create');

COMMIT;
