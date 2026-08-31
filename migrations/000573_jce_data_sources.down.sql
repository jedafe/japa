-- 000573_jce_data_sources.down.sql
-- Reverts 000573: drops the comparison_data_sources table and all dependent objects.

BEGIN;

DROP TABLE IF EXISTS comparison_data_sources CASCADE;

COMMIT;
