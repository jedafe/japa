-- 000806_marketplace_followups.up.sql
-- Services Marketplace follow-ups:
--  a) GDPR consistency — coupon_redemptions.user_id moves from
--     NOT NULL … ON DELETE CASCADE to nullable … ON DELETE SET NULL,
--     matching service_bookings.booker_user_id and coupon_events.user_id.
--     A GDPR-erased user's redemption row then survives so the org's
--     amount_saved audit trail stays intact.
--  b) Category label correction — DBs that applied 000803 before the
--     health/insurance split carry label 'Health & Insurance' on slug
--     'health'; the corrected seed reads 'Health'.
BEGIN;

-- a) GDPR-safe user reference on coupon_redemptions
ALTER TABLE coupon_redemptions ALTER COLUMN user_id DROP NOT NULL;
ALTER TABLE coupon_redemptions
    DROP CONSTRAINT coupon_redemptions_user_id_fkey,
    ADD CONSTRAINT coupon_redemptions_user_id_fkey
        FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE SET NULL;

-- b) label fix for DBs seeded before the 000803 correction
--    (fresh databases no-op — the seed already inserts 'Health')
UPDATE service_categories SET label = 'Health'
WHERE slug = 'health' AND label = 'Health & Insurance';

COMMIT;
