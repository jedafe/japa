-- 000856_wiki_cost_of_living_rollup_levels.up.sql
-- Fix cost-of-living parent aggregation (§80 Hierarchical UX).
--
-- The country/state cost_of_living rollup configs (seeded by
-- 000750) omitted `levels`, so the descendant-type scope fell back
-- to the wiki_type_parents closure. That closure only connects
-- attraction/university to the geo types — country→state→city is
-- never declared — so the rollup resolved to {attraction,
-- university}, found no cost-of-living fields, and rendered nothing.
-- A parent's Cost of Living page therefore showed only its own
-- values, never an aggregate of its children.
--
-- Set explicit `levels` (the §80 override for the type-edge
-- closure): country pages aggregate their state+city descendants and
-- state pages aggregate their city descendants. Actual parentage is
-- already on spaces.parent_id / space_parent_spaces (the same edges
-- ListWikiDescendantSpaces walks), so the descendant spaces are
-- found; only the type-scope was missing.
--
-- City cost-of-living values live directly on the city space
-- (wiki_city_cost_of_living); the state category's group carries no
-- fields, so states pass through to their cities until they hold
-- values of their own.

BEGIN;

-- Country: aggregate descendant states and cities.
UPDATE wiki_type_categories
SET rollup = jsonb_set(rollup, '{levels}', '["state","city"]'::jsonb)
WHERE category = 'cost_of_living'
  AND space_type_id IN (SELECT id FROM space_types WHERE slug = 'country');

-- State: aggregate descendant cities.
UPDATE wiki_type_categories
SET rollup = jsonb_set(rollup, '{levels}', '["city"]'::jsonb)
WHERE category = 'cost_of_living'
  AND space_type_id IN (SELECT id FROM space_types WHERE slug = 'state');

COMMIT;
