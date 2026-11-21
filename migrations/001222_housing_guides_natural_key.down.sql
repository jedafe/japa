-- 001222_housing_guides_natural_key.down.sql
-- Restores the NULLS DISTINCT index shape. Deduplicated rows and the
-- 'apartment' → 'private_rental' normalization are data changes and
-- are not reversible.

BEGIN;

DROP INDEX housing_guides_destination_space_id_city_space_id_key;
CREATE UNIQUE INDEX housing_guides_destination_space_id_city_space_id_key
    ON housing_guides (destination_space_id, city_space_id, accommodation_type);

COMMIT;
