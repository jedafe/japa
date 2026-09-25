-- 000461_drop_polymorphic_promoted_fk.up.sql
-- The intel_records.promoted_record_id column is polymorphic: it points to
-- scholarships(id) for scholarship records, intel_jobs(id) for job records,
-- visa_routes(id) for visa records, etc. A single FK constraint can only
-- reference one table, so the original constraint (added in 000429) that
-- forced promoted_record_id to reference scholarships(id) is incorrect for
-- non-scholarship entity types. Drop it — referential integrity for the
-- promoted projection is already enforced by the unique index on each
-- projection table's intel_record_id column.

BEGIN;
ALTER TABLE intel_records DROP CONSTRAINT IF EXISTS intel_records_promoted_record_id_fkey;
COMMIT;
