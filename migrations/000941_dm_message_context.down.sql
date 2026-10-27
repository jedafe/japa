BEGIN;

ALTER TABLE dm_messages
    DROP CONSTRAINT IF EXISTS dm_messages_context_type_check;

ALTER TABLE dm_messages
    DROP COLUMN IF EXISTS context_slug,
    DROP COLUMN IF EXISTS context_label,
    DROP COLUMN IF EXISTS context_id,
    DROP COLUMN IF EXISTS context_type;

COMMIT;
