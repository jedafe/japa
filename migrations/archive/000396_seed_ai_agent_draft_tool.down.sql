-- 000396_seed_ai_agent_draft_tool.down.sql
-- Reverses 000396: removes the content.create_draft tool seed.

BEGIN;

DELETE FROM ai_tools
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND name = 'content.create_draft';

COMMIT;
