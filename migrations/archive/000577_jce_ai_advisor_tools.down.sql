-- 000577_jce_ai_advisor_tools.down.sql
-- Reverts 000577: removes the JCE AI advisor tool rows.

BEGIN;

DELETE FROM ai_tools
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND name IN ('comparison.analyze','comparison.rank','comparison.explain','comparison.recommend_next_step','comparison.refresh');

COMMIT;
