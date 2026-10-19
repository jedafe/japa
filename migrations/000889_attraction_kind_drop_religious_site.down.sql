-- Restores the `religious-site` option on the attraction `kind` select.
-- Values reclassified to `other` by the up migration are not restored —
-- the original kind cannot be recovered.

BEGIN;

UPDATE custom_fields
SET options = '[{"value":"museum","label":"Museum"},{"value":"landmark","label":"Landmark"},{"value":"monument","label":"Monument"},{"value":"historic-site","label":"Historic Site"},{"value":"religious-site","label":"Religious Site"},{"value":"market","label":"Market"},{"value":"waterfall","label":"Waterfall"},{"value":"mountain","label":"Mountain"},{"value":"rock-formation","label":"Rock Formation"},{"value":"lake","label":"Lake"},{"value":"cave","label":"Cave"},{"value":"beach","label":"Beach"},{"value":"forest-reserve","label":"Forest Reserve"},{"value":"canyon","label":"Canyon"},{"value":"hot-spring","label":"Hot Spring"},{"value":"national-park","label":"National Park"},{"value":"garden","label":"Garden"},{"value":"zoo","label":"Zoo"},{"value":"theme-park","label":"Theme Park"},{"value":"other","label":"Other"}]'::jsonb
WHERE id = '00000000-0000-0000-0000-000000000111';

COMMIT;
