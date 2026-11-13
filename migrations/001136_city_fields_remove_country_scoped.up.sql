-- Remove country/state-scoped fields from wiki_city_* groups. These
-- describe national/provincial attributes, not city attributes, and
-- already exist in the wiki_country_* / wiki_state_* counterparts:
--   qf_currency        x3 (currency is a country attribute)
--   emergency_number   (national emergency numbers)
--   minimum_wage       (set by national/state law)
--   official_language  (country/state attribute)
--   mobile_operators   (national carriers)
-- All 7 had 0 field values and 0 comparison dimensions.

BEGIN;

DELETE FROM custom_fields
WHERE group_id IN (SELECT id FROM custom_field_groups WHERE slug LIKE 'wiki_city_%')
  AND key IN ('qf_currency', 'emergency_number', 'minimum_wage', 'official_language', 'mobile_operators');

COMMIT;
