-- 000369_add_checklist_progress_list_type.down.sql
-- Reverses 000369_add_checklist_progress_list_type.up.sql.

BEGIN;

-- Restore the original unique constraint (without list_type).
ALTER TABLE checklist_progress
    DROP CONSTRAINT IF EXISTS checklist_progress_tenant_id_user_id_guide_id_item_key_list_type_key;

ALTER TABLE checklist_progress
    ADD CONSTRAINT checklist_progress_tenant_id_user_id_guide_id_item_key_key
    UNIQUE (tenant_id, user_id, guide_id, item_key);

DROP INDEX IF EXISTS idx_checklist_progress_list_type;

-- Drop the list_type column. Existing arrival checklist progress rows will
-- be lost on downgrade — this is expected since the feature is being removed.
ALTER TABLE checklist_progress
    DROP COLUMN IF EXISTS list_type;

COMMIT;
