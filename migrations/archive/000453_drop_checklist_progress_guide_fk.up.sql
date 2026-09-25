-- 000453_drop_checklist_progress_guide_fk.up.sql
-- Drop the checklist_progress.guide_id → contents(id) foreign key so that
-- arrival checklist progress (§92, ADR-100) can use arrival_checklist_guides.id
-- as guide_id. The guide_id column is now polymorphic: it references
-- contents(id) when list_type = 'visa' (§70) or arrival_checklist_guides.id
-- when list_type = 'arrival' (§92). A single FK cannot reference two tables,
-- so the FK is dropped. Referential integrity is maintained by the
-- application layer (the store always verifies the guide exists before
-- inserting progress).
--
-- Ref: japaconnect-engineering/sections/92-arrival-checklist.md
-- Ref: japaconnect-engineering/sections/70-visa-checklist.md
-- Ref: japaconnect-engineering/adr/ADR-100-arrival-checklist.md

BEGIN;

ALTER TABLE checklist_progress
    DROP CONSTRAINT IF EXISTS checklist_progress_guide_id_fkey;

COMMIT;
