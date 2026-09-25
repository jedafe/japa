-- 000575_jce_ingestion_jobs.down.sql
-- Reverts 000575: drops the comparison_data_ingestion_jobs table.

BEGIN;

DROP TABLE IF EXISTS comparison_data_ingestion_jobs CASCADE;

COMMIT;
