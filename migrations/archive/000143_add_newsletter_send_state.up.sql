-- Section 33 (Newsletter): Per-recipient send state for crash-safe newsletter delivery.

CREATE TABLE IF NOT EXISTS newsletter_send_state (
    campaign_id uuid NOT NULL REFERENCES newsletter_campaigns(id) ON DELETE CASCADE,
    recipient   text NOT NULL,
    status      text NOT NULL DEFAULT 'pending',  -- 'pending' | 'sent' | 'failed'
    sent_at     timestamptz,
    error       text,
    created_at  timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (campaign_id, recipient)
);
CREATE INDEX idx_newsletter_send_state_campaign ON newsletter_send_state(campaign_id, status);
