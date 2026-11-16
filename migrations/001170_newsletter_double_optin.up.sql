-- 001170: Newsletter double opt-in (§62 Compliance hardening, audit NL-I1).
--
-- Adds a 'pending' subscriber status for unconfirmed signups and a purpose
-- column on newsletter_unsubscribe_tokens so confirmation links cannot be
-- replayed as unsubscribe tokens (and vice versa). Activated per-platform
-- by the newsletter.double_opt_in system setting (default off — preserves
-- the §62 single opt-in contract).

ALTER TABLE newsletter_subscribers DROP CONSTRAINT IF EXISTS newsletter_subscribers_status_check;
ALTER TABLE newsletter_subscribers ADD CONSTRAINT newsletter_subscribers_status_check
    CHECK (status IN ('pending','subscribed','unsubscribed','bounced','complained'));

ALTER TABLE newsletter_unsubscribe_tokens
    ADD COLUMN IF NOT EXISTS purpose text NOT NULL DEFAULT 'unsubscribe';
ALTER TABLE newsletter_unsubscribe_tokens DROP CONSTRAINT IF EXISTS newsletter_unsubscribe_tokens_purpose_check;
ALTER TABLE newsletter_unsubscribe_tokens
    ADD CONSTRAINT newsletter_unsubscribe_tokens_purpose_check
    CHECK (purpose IN ('unsubscribe','confirm'));

-- =====================================================================
-- system_settings — newsletter.double_opt_in (Section 62, ADR-080)
-- =====================================================================
INSERT INTO system_settings (key, value, is_public) VALUES
    ('newsletter.double_opt_in', to_jsonb(false), false)
ON CONFLICT (key) DO NOTHING;
