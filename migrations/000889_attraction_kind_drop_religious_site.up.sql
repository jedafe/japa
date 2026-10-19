-- Ref: .jeos/sections/80-destination-wiki.md — religious sites are not
-- attractions. Drops the `religious-site` option from the attraction
-- `kind` select seeded in 000795 and reclassifies existing values to
-- `other` (the classifier fallback — places of worship no longer map to
-- a dedicated kind). The seeder additionally excludes Q1370598
-- (structure of worship) so places of worship are never seeded.

BEGIN;

UPDATE custom_fields
SET options = '[{"value":"museum","label":"Museum"},{"value":"landmark","label":"Landmark"},{"value":"monument","label":"Monument"},{"value":"historic-site","label":"Historic Site"},{"value":"market","label":"Market"},{"value":"waterfall","label":"Waterfall"},{"value":"mountain","label":"Mountain"},{"value":"rock-formation","label":"Rock Formation"},{"value":"lake","label":"Lake"},{"value":"cave","label":"Cave"},{"value":"beach","label":"Beach"},{"value":"forest-reserve","label":"Forest Reserve"},{"value":"canyon","label":"Canyon"},{"value":"hot-spring","label":"Hot Spring"},{"value":"national-park","label":"National Park"},{"value":"garden","label":"Garden"},{"value":"zoo","label":"Zoo"},{"value":"theme-park","label":"Theme Park"},{"value":"other","label":"Other"}]'::jsonb
WHERE id = '00000000-0000-0000-0000-000000000111';

UPDATE custom_field_values
SET value_json = '{"value":"other"}'::jsonb
WHERE field_id = '00000000-0000-0000-0000-000000000111'
  AND value_json->>'value' = 'religious-site';

COMMIT;
