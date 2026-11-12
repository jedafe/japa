BEGIN;

UPDATE comparison_type_definitions SET config = jsonb_set(config, '{family}', '"finance"')      WHERE slug = 'remittance';
UPDATE comparison_type_definitions SET config = jsonb_set(config, '{family}', '"geo"')          WHERE slug = 'neighborhoods';
UPDATE comparison_type_definitions SET config = jsonb_set(config, '{family}', '"guides"')       WHERE slug = 'housing';
UPDATE comparison_type_definitions SET config = jsonb_set(config, '{family}', '"marketplace"')  WHERE slug = 'job-listings';
UPDATE comparison_type_definitions SET config = config - 'family'                               WHERE slug = 'residents-permit-requirements';
UPDATE comparison_type_definitions SET config = config - 'family'
WHERE slug IN ('ai-agent', 'ai-model', 'cloud', 'hosting', 'mobile-app', 'saas', 'web-app', 'website');

COMMIT;
