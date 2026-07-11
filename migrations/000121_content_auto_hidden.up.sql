-- Add auto-hide columns to contents for moderation auto-hide feature.
-- When pending report count reaches the configured threshold, content is
-- auto-hidden from public views without changing its status.

ALTER TABLE contents
    ADD COLUMN auto_hidden    boolean NOT NULL DEFAULT false,
    ADD COLUMN auto_hidden_at timestamptz;

CREATE INDEX idx_contents_auto_hidden ON contents(tenant_id, auto_hidden) WHERE auto_hidden = true;
