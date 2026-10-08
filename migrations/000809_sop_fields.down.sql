-- 000809_sop_fields.down.sql
-- Reverse of 000809: drop the curated field-of-study dictionary.
-- sop_samples.field_of_study (text) is untouched and survives the rollback.

BEGIN;

DELETE FROM ai_tools WHERE name IN ('sop_field.create', 'sop_field.update', 'sop_field.delete');

DROP TABLE IF EXISTS sop_fields;

COMMIT;
