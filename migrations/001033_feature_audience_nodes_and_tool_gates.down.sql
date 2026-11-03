-- 001033_feature_audience_nodes_and_tool_gates.down.sql

BEGIN;

UPDATE ai_tools
SET required_permission = NULL, updated_at = now()
WHERE name IN ('comparison.promote', 'user.update_profile')
  AND required_permission = 'agent.use';

DELETE FROM permissions
WHERE key IN ('feature.ads', 'feature.ai_agent', 'feature.japabot');

COMMIT;
