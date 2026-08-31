-- 000580_jce_gdpr_owners.down.sql

BEGIN;

DROP INDEX IF EXISTS idx_comparison_data_sources_created_by;

ALTER TABLE comparison_data_sources
    DROP COLUMN IF EXISTS created_by_user_id;

COMMIT;
