-- DM message requests for non-mutual-follow senders.
-- Ref: sections/48-direct-messages.md, dm:cleanup_declined_requests task.

CREATE TABLE dm_requests (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    requester_id    uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    recipient_id    uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    conversation_id uuid REFERENCES dm_conversations(id),
    preview_message text NOT NULL,
    status          text NOT NULL DEFAULT 'pending'
                      CHECK (status IN ('pending', 'accepted', 'declined', 'blocked')),
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    resolved_at     timestamptz,
    UNIQUE(requester_id, recipient_id)
);

CREATE INDEX idx_dm_requests_recipient ON dm_requests (recipient_id, status, created_at DESC);
CREATE INDEX idx_dm_requests_tenant ON dm_requests (tenant_id);
CREATE INDEX idx_dm_requests_stale ON dm_requests (status, updated_at);
