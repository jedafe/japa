-- 000727_seed_ai_tools_phase2.down.sql
-- Reverses 000727_seed_ai_tools_phase2.up.sql.
-- Removes the 5 phase-2 agent data tools from the ai_tools table.
--
-- Ref: docs/superpowers/specs/2026-09-24-agent-data-api-phase2-design.md

BEGIN;

DELETE FROM ai_tools
WHERE name IN (
    'feeds.explore',
    'feeds.for_you',
    'scam.list',
    'scam.get',
    'search.semantic'
  );

COMMIT;
