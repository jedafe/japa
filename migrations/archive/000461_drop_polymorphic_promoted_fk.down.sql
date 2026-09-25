-- 000461_drop_polymorphic_promoted_fk.down.sql
BEGIN;
ALTER TABLE intel_records
  ADD CONSTRAINT intel_records_promoted_record_id_fkey
  FOREIGN KEY (promoted_record_id) REFERENCES scholarships(id) ON DELETE SET NULL;
COMMIT;
