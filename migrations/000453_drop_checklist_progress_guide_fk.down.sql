-- 000453_drop_checklist_progress_guide_fk.down.sql
-- Reverses 000453_drop_checklist_progress_guide_fk.up.sql.
-- Restores the checklist_progress.guide_id → contents(id) FK.
-- WARNING: arrival checklist progress rows (list_type = 'arrival') will
-- fail the FK check and must be deleted before running this downgrade.

BEGIN;

ALTER TABLE checklist_progress
    ADD CONSTRAINT checklist_progress_guide_id_fkey
    FOREIGN KEY (guide_id) REFERENCES contents(id) ON DELETE CASCADE;

COMMIT;
