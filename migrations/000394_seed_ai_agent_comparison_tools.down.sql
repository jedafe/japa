-- 000394_seed_ai_agent_comparison_tools.down.sql
-- Reverses 000394_seed_ai_agent_comparison_tools.up.sql.
-- Removes the 5 Phase 7 comparison agent tools from the ai_tools table.
--
-- Ref: japaconnect-engineering/sections/103-ai-agent-executor.md (Comparison Engine)
-- Ref: japaconnect-engineering/roadmaps/ROADMAP-AI-AGENT.md (Phase 7, task 7.06)

BEGIN;

DELETE FROM ai_tools
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND name IN (
    'comparison.types.list',
    'comparison.create',
    'comparison.fill_values',
    'comparison.add_dimension',
    'comparison.read'
  );

COMMIT;
