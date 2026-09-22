-- 000702_content_reports_source_entity.up.sql
-- Renumbered from 000662 — it collided with 000662_add_source_id_to_entities.
-- Tail-position body; databases that applied the 000662 file skip this via
-- the 000000 registry marker. Unchanged: DROP IF EXISTS + ADD is idempotent.
-- 000662_content_reports_source_entity.up.sql
-- Allow content_reports to record reports against intel_sources so users can
-- flag a publisher source from the source detail page.

BEGIN;

ALTER TABLE content_reports
    DROP CONSTRAINT IF EXISTS content_reports_entity_type_check,
    ADD CONSTRAINT content_reports_entity_type_check
    CHECK (entity_type IN ('content', 'comment', 'user', 'space', 'message', 'source'));

COMMIT;
