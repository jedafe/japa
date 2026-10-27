-- 000942_org_dm_conversations.up.sql
-- Org-scoped DM conversations: "Message provider" on an org-owned service
-- listing opens a (user ↔ org) conversation instead of DMing whichever
-- member authored the listing. participant_2_id goes nullable — org
-- conversations set conversation_type='org' + org_id and leave p2 NULL.
-- Org-side visibility is every org member (they reply on behalf of the
-- org via sent_as_org), with per-member read state in dm_org_read_state.

BEGIN;

ALTER TABLE dm_conversations
    ADD COLUMN conversation_type text NOT NULL DEFAULT 'dm',
    ADD COLUMN org_id uuid REFERENCES organizations(id);

ALTER TABLE dm_conversations
    ALTER COLUMN participant_2_id DROP NOT NULL;

ALTER TABLE dm_conversations
    ADD CONSTRAINT dm_conversations_type_check
        CHECK (conversation_type IN ('dm', 'org')),
    ADD CONSTRAINT dm_conversations_org_shape_check
        CHECK (
            (conversation_type = 'dm' AND participant_2_id IS NOT NULL AND org_id IS NULL)
            OR (conversation_type = 'org' AND org_id IS NOT NULL AND participant_2_id IS NULL)
        );

-- One org conversation per (external user, org).
CREATE UNIQUE INDEX dm_conversations_org_pair
    ON dm_conversations (participant_1_id, org_id)
    WHERE conversation_type = 'org';

CREATE INDEX dm_conversations_org_inbox
    ON dm_conversations (org_id, last_message_at DESC NULLS LAST)
    WHERE conversation_type = 'org';

ALTER TABLE dm_messages
    ADD COLUMN sent_as_org boolean NOT NULL DEFAULT false;

-- Per-member unread on the org side: unread = messages created after
-- last_read_at for that member.
CREATE TABLE dm_org_read_state (
    conversation_id uuid NOT NULL REFERENCES dm_conversations(id) ON DELETE CASCADE,
    member_user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    last_read_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (conversation_id, member_user_id)
);

CREATE INDEX dm_org_read_state_member ON dm_org_read_state (member_user_id);

COMMIT;
