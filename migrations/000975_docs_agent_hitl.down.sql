-- Reverts: docs.approve/docs.publish requires_hitl=true (000975).
BEGIN;

UPDATE ai_tools
SET config = jsonb_set(config, '{requires_hitl}', 'false'::jsonb)
WHERE name IN ('docs.approve', 'docs.publish');

COMMIT;
