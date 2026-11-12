-- Normalize config.family to the catalogued families in
-- compareFamilies.ts. Stray keys (finance, geo, guides, marketplace) were
-- seeded ad hoc; the i18n descriptions make the intent clear — banking
-- covers "remittance services", jobs covers "job listings and business
-- registration", location covers places. Also groups the product types
-- under a new 'products' family and folds residents-permit into
-- visa_routes so they stop rendering as ungrouped tiles.

BEGIN;

UPDATE comparison_type_definitions SET config = jsonb_set(config, '{family}', '"banking"')      WHERE slug = 'remittance';
UPDATE comparison_type_definitions SET config = jsonb_set(config, '{family}', '"location"')     WHERE slug = 'neighborhoods';
UPDATE comparison_type_definitions SET config = jsonb_set(config, '{family}', '"location"')     WHERE slug = 'housing';
UPDATE comparison_type_definitions SET config = jsonb_set(config, '{family}', '"jobs"')         WHERE slug = 'job-listings';
UPDATE comparison_type_definitions SET config = jsonb_set(config, '{family}', '"visa_routes"')  WHERE slug = 'residents-permit-requirements';
UPDATE comparison_type_definitions SET config = jsonb_set(config, '{family}', '"products"')
WHERE slug IN ('ai-agent', 'ai-model', 'cloud', 'hosting', 'mobile-app', 'saas', 'web-app', 'website');

COMMIT;
