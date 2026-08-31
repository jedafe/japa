-- 000574_jce_value_freshness.down.sql
-- Reverts 000574: removes provenance columns and index from comparison_values.

BEGIN;

DROP INDEX IF EXISTS idx_comparison_values_data_source;

ALTER TABLE comparison_values
    DROP COLUMN IF EXISTS data_source_id,
    DROP COLUMN IF EXISTS updated_at;

COMMIT;
