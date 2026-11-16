-- §126 M2: Announce provenance. share_count is a denormalized counter —
-- ++/-- must be idempotent under replays and double-boosts, so each
-- remote boost lands a row here first; only a fresh insert bumps the
-- counter, and only a real delete decrements it.
CREATE TABLE federation_remote_shares (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    remote_actor_id uuid NOT NULL REFERENCES remote_actors(id) ON DELETE CASCADE,
    content_id      uuid NOT NULL REFERENCES contents(id) ON DELETE CASCADE,
    ap_activity_id  text,
    created_at      timestamptz NOT NULL DEFAULT now(),
    UNIQUE (remote_actor_id, content_id)
);
CREATE INDEX idx_fed_remote_shares_content ON federation_remote_shares (content_id);

-- Delivery dedup: a fanout retry (or double-enqueue) must not re-ledger
-- the same activity for the same inbox. (activity_ap_id, inbox_url) is
-- exactly the idempotency key — different activities to the same inbox
-- remain distinct rows.
ALTER TABLE federation_deliveries
    ADD CONSTRAINT federation_deliveries_activity_inbox_uniq
    UNIQUE (activity_ap_id, inbox_url);
