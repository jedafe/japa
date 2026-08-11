-- 000381_add_checklist_progress_list_type.up.sql
-- Add list_type column to checklist_progress so arrival checklist progress
-- (§92) can be distinguished from visa checklist progress (§70).
--
-- §92 line 13: "Reuses checklist_progress from §70 with list_type = 'arrival'"
-- The column was referenced in migration 000329's comment but never added.
--
-- Ref: japaconnect-engineering/sections/92-arrival-checklist.md
-- Ref: japaconnect-engineering/sections/70-visa-checklist.md

BEGIN;

-- Add list_type column with default 'visa' so existing rows are visa checklist progress.
ALTER TABLE checklist_progress
    ADD COLUMN IF NOT EXISTS list_type text NOT NULL DEFAULT 'visa';

-- Update the unique constraint to include list_type so a user can have both
-- a visa checklist and an arrival checklist for the same guide_id+item_key
-- without colliding. (guide_id for arrival checklists references
-- arrival_checklist_guides.id, which is a different UUID namespace from
-- contents.id, so collisions are already impossible in practice — but
-- including list_type in the constraint makes the intent explicit and
-- supports future checklist types.)
ALTER TABLE checklist_progress
    DROP CONSTRAINT IF EXISTS checklist_progress_tenant_id_user_id_guide_id_item_key_key;

ALTER TABLE checklist_progress
    ADD CONSTRAINT checklist_progress_tenant_id_user_id_guide_id_item_key_list_type_key
    UNIQUE (tenant_id, user_id, guide_id, item_key, list_type);

-- Index for filtering by list_type (used by /me/arrival-checklists routes).
CREATE INDEX IF NOT EXISTS idx_checklist_progress_list_type
    ON checklist_progress (tenant_id, user_id, list_type);

COMMIT;
