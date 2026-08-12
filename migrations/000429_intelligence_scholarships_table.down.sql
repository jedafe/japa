-- 000429_intelligence_scholarships_table.down.sql
BEGIN;
ALTER TABLE intel_records DROP CONSTRAINT IF EXISTS intel_records_promoted_record_id_fkey;
DROP TABLE IF EXISTS scholarships;
COMMIT;
