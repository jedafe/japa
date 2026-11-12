-- Trim the over-seeded dimension pools on cost-of-living subtypes.
--
-- 001072 mapped col-* types (and cost_of_living) to the whole 'city' wiki
-- category set — every city group (Housing, Healthcare, Embassies, …)
-- landed in their pools even though each subtype compares exactly one
-- prices subject. Food & Groceries has no relationship with Housing.
--
-- Keep only each type's bound field group, matching the state/country
-- rollup pattern (pool = bound group).
BEGIN;

DELETE FROM comparison_type_field_groups ctfg
USING comparison_type_definitions td
WHERE td.id = ctfg.type_id
  AND td.slug IN (
      'col-food-groceries',
      'col-restaurants',
      'col-transportation',
      'col-lifestyle',
      'col-utilities',
      'cost_of_living'
  )
  AND ctfg.group_id <> td.field_group_id;

COMMIT;
