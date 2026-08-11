-- 000388_seed_ai_agent_tools_v1.down.sql
-- Reverses 000388_seed_ai_agent_tools_v1.up.sql.
-- Removes the 10 Phase 4 agent tools from the ai_tools table.
--
-- Ref: japaconnect-engineering/sections/103-ai-agent-executor.md
-- Ref: japaconnect-engineering/roadmaps/ROADMAP-AI-AGENT.md (Phase 4, task 4.11)

BEGIN;

DELETE FROM ai_tools
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND name IN (
    'content.search',
    'content.read',
    'content.create',
    'content.update',
    'custom_fields.list',
    'custom_field_values.read',
    'custom_field_values.set',
    'custom_field_values.bulk_set',
    'rag.search',
    'external.search'
  );

COMMIT;
