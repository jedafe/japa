-- Remove four comparison types requested out of the catalog: regions,
-- attractions, neighborhoods, housing. None have items/dimensions; pool
-- rows go first (RESTRICT on group), then the type definitions. The
-- region/attraction/neighborhood resolvers stay registered — they also
-- serve wiki field ops on those entity types.

BEGIN;

DELETE FROM comparison_type_field_groups
WHERE type_id IN (
    SELECT id FROM comparison_type_definitions
    WHERE slug IN ('regions', 'attractions', 'neighborhoods', 'housing')
);

DELETE FROM comparison_type_definitions
WHERE slug IN ('regions', 'attractions', 'neighborhoods', 'housing');

COMMIT;
