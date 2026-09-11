-- 000656_sources_post_count_trigger.down.sql

BEGIN;

DROP TRIGGER IF EXISTS trg_intel_records_post_count ON intel_records;
DROP FUNCTION IF EXISTS trg_intel_records_post_count_sync();

COMMIT;
