-- 001170 down: revert double opt-in.
--
-- Pending rows must be resolved before the CHECK narrows again — treat
-- pending signups as unsubscribed so the constraint can shrink safely.

UPDATE newsletter_subscribers SET status = 'unsubscribed' WHERE status = 'pending';

ALTER TABLE newsletter_subscribers DROP CONSTRAINT IF EXISTS newsletter_subscribers_status_check;
ALTER TABLE newsletter_subscribers ADD CONSTRAINT newsletter_subscribers_status_check
    CHECK (status IN ('subscribed','unsubscribed','bounced','complained'));

ALTER TABLE newsletter_unsubscribe_tokens DROP CONSTRAINT IF EXISTS newsletter_unsubscribe_tokens_purpose_check;
ALTER TABLE newsletter_unsubscribe_tokens DROP COLUMN IF EXISTS purpose;

DELETE FROM system_settings WHERE key = 'newsletter.double_opt_in';
