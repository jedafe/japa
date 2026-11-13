-- Drop Toronto-branded fields from the shared city transportation
-- price group: PRESTO Card, Bike Share Toronto x2, Bird/Lime scooter.
-- One city's operator names baked into a template that renders for
-- every city; all four are also 'price_parts' fields, which the
-- comparison matrix cannot represent. 0 values, 0 dimension refs.

BEGIN;

DELETE FROM custom_fields
WHERE group_id = (SELECT id FROM custom_field_groups WHERE slug = 'wiki_city_prices_transportation')
  AND key IN ('ttc_presto_card', 'bike_single', 'bike_day_pass', 'escooter');

COMMIT;
