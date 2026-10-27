-- 000941_dm_message_context.up.sql
-- Structured context on dm_messages: a service-listing contact ("Message
-- provider / Contact mentor") can attach the listing it references so the
-- thread renders a linked context card instead of context baked into the
-- message body. context_label/context_slug are send-time snapshots so the
-- card renders without a join (Rule 16: the API owns the data, the label
-- is display-only metadata).
--
-- Used by the POST /dm/conversations context payload; the websocket.dm
-- service validates that a service_listing context points at a published
-- listing whose author is the recipient — which also powers the
-- listing-scoped dm_privacy bypass (a commercial contact surface can't be
-- killed by the provider's social DM setting).

BEGIN;

ALTER TABLE dm_messages
    ADD COLUMN context_type text,
    ADD COLUMN context_id uuid,
    ADD COLUMN context_label text,
    ADD COLUMN context_slug text;

ALTER TABLE dm_messages
    ADD CONSTRAINT dm_messages_context_type_check
    CHECK (context_type IS NULL OR context_type IN ('service_listing'));

-- Thread reads scan by conversation — context columns ride the same index;
-- no additional index needed (point lookups by conversation_id already exist).

COMMIT;
