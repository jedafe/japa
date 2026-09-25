-- 000554_jce_recommendations.down.sql

BEGIN;

DROP TABLE IF EXISTS comparison_recommendations CASCADE;

UPDATE comparison_type_definitions
SET config = config - 'recommendation_templates'
WHERE is_system = true;

COMMIT;
