-- 000702_content_reports_source_entity.down.sql
-- Renumbered from 000662 (duplicate version prefix). See the .up file.
-- 000662_content_reports_source_entity.down.sql
BEGIN;

ALTER TABLE content_reports
    DROP CONSTRAINT IF EXISTS content_reports_entity_type_check,
    ADD CONSTRAINT content_reports_entity_type_check
    CHECK (entity_type IN ('content', 'comment', 'user', 'space', 'message'));

COMMIT;
