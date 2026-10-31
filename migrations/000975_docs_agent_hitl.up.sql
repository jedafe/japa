-- 000975_docs_agent_hitl
-- §44 HITL: docs.approve and docs.publish were seeded with
-- requires_hitl=false (000862), letting an agent drive draft → approved →
-- published in one turn while stamping the human user as reviewer. Require
-- human confirmation for both reviewer transitions — the same gate
-- docs.create already has.
BEGIN;

UPDATE ai_tools
SET config = jsonb_set(config, '{requires_hitl}', 'true'::jsonb)
WHERE name IN ('docs.approve', 'docs.publish')
  AND config->>'requires_hitl' <> 'true';

COMMIT;
