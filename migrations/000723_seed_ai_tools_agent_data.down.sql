-- 000723_seed_ai_tools_agent_data.down.sql
-- Reverses 000723_seed_ai_tools_agent_data.up.sql.
-- Removes the 5 agent data tools from the ai_tools table.
--
-- Ref: docs/superpowers/specs/2026-09-23-agent-data-api-design.md

BEGIN;

DELETE FROM ai_tools
WHERE name IN (
    'visa.search',
    'visa.get',
    'visa.compare',
    'university.get',
    'destination.brief'
  );

COMMIT;
