-- 000310_create_digest_editions.up.sql
-- Community Digest Widget (§84, ADR-092).
-- Ref: japaconnect-engineering/sections/84-community-digest-widget.md

BEGIN;

CREATE TABLE IF NOT EXISTS digest_editions (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id     uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    space_id      uuid REFERENCES spaces(id) ON DELETE SET NULL, -- nullable for tenant-wide digest
    cadence       text NOT NULL DEFAULT 'weekly', -- weekly|monthly
    status        text NOT NULL DEFAULT 'draft', -- draft|review|published|cancelled (§84)
    scheduled_for timestamptz,
    published_at  timestamptz,
    content       jsonb NOT NULL DEFAULT '{}', -- sections JSON (top_stories, events, etc.)
    email_sent    boolean NOT NULL DEFAULT false,
    email_sent_at timestamptz,
    generated_by  text NOT NULL DEFAULT 'system', -- 'system'|'manual'
    reviewed_by   uuid REFERENCES users(id) ON DELETE SET NULL,
    created_at    timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_digest_editions_tenant_status
    ON digest_editions(tenant_id, status, published_at DESC);
CREATE INDEX IF NOT EXISTS idx_digest_editions_tenant_space
    ON digest_editions(tenant_id, space_id, published_at DESC)
    WHERE status = 'published';

COMMIT;
