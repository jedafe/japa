BEGIN;

-- Org conversations are incompatible with the restored p1<p2/NOT NULL
-- shape — drop them before reverting (their messages cascade).
DELETE FROM dm_conversations WHERE conversation_type = 'org';

DROP TABLE IF EXISTS dm_org_read_state;
ALTER TABLE dm_messages DROP COLUMN IF EXISTS sent_as_org;
DROP INDEX IF EXISTS dm_conversations_org_inbox;
DROP INDEX IF EXISTS dm_conversations_org_pair;
ALTER TABLE dm_conversations
    DROP CONSTRAINT IF EXISTS dm_conversations_org_shape_check,
    DROP CONSTRAINT IF EXISTS dm_conversations_type_check;
ALTER TABLE dm_conversations
    DROP COLUMN IF EXISTS org_id,
    DROP COLUMN IF EXISTS conversation_type;
ALTER TABLE dm_conversations
    ALTER COLUMN participant_2_id SET NOT NULL;

COMMIT;
