-- Remove the "Cost of Living" field from the shared geo_fields group.
-- Geo entities' COL data lives in the dedicated wiki_*_cost_of_living
-- rollup groups and the cost-of-living comparison types; this lone
-- text field was unpopulated (0 values, 0 dimension refs, 0 code refs)
-- and doesn't belong in the Geo Fields picker group.

BEGIN;

DELETE FROM custom_fields
WHERE id = '45390ad3-7bee-4ad1-8e5a-d92bdb9fe1ab'
  AND group_id = (SELECT id FROM custom_field_groups WHERE slug = 'geo_fields')
  AND key = 'cost_of_living';

COMMIT;
