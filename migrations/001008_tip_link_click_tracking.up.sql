-- §125 external tip link click tracking + affiliate monetization.
-- click_count: denormalized counter for cheap owner-dashboard display.
-- tip_external_link_clicks: §56-parity click events (hashed ip/ua only — no
-- raw PII), cascade-deleted with the link.
ALTER TABLE tip_external_links
    ADD COLUMN click_count bigint NOT NULL DEFAULT 0;

CREATE TABLE tip_external_link_clicks (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    link_id         uuid NOT NULL REFERENCES tip_external_links(id) ON DELETE CASCADE,
    ip_hash         text,
    user_agent_hash text,
    referrer_url    text,
    created_at      timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_tip_link_clicks_link ON tip_external_link_clicks (link_id, created_at DESC);
