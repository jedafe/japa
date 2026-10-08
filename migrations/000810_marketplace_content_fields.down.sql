-- 000810_marketplace_content_fields.down.sql
-- Reverse of 000810 up, in reverse order.
BEGIN;

DELETE FROM system_settings WHERE key IN (
    'service_marketplace.content.max_faqs',
    'service_marketplace.content.max_steps',
    'service_marketplace.badges.min_reviews',
    'service_marketplace.badges.min_rating'
);

ALTER TABLE service_listing_meta
    DROP COLUMN IF EXISTS steps,
    DROP COLUMN IF EXISTS faqs;

COMMIT;
