-- 000856_wiki_cost_of_living_rollup_levels.down.sql
-- Revert cost-of-living rollup `levels` to the 000750 shape
-- (descendant types derived from the wiki_type_parents closure).

BEGIN;

UPDATE wiki_type_categories
SET rollup = rollup - 'levels'
WHERE category = 'cost_of_living'
  AND space_type_id IN (SELECT id FROM space_types WHERE slug IN ('country', 'state'));

COMMIT;
