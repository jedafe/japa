-- Restore pre-001133 state: drop the city-cost-of-living pool row and
-- type definition. No comparisons referenced it yet (new type).

BEGIN;

DELETE FROM comparison_type_field_groups
WHERE type_id IN (SELECT id FROM comparison_type_definitions WHERE slug = 'city-cost-of-living');

DELETE FROM comparison_type_definitions WHERE slug = 'city-cost-of-living';

COMMIT;
