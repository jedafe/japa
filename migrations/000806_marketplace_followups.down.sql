-- 000806_marketplace_followups.down.sql
-- Reverse of 000806 up. The 'health' label update is intentionally not
-- reverted — 'Health & Insurance' was the pre-split seed label and
-- restoring it would reintroduce the duplicate-coverage label (the
-- forward fix is lossless).
BEGIN;

-- Rows whose user_id was nulled by a GDPR erase cannot satisfy the
-- restored NOT NULL/CASCADE pair, so they are removed — equivalent to
-- what the old ON DELETE CASCADE would have done at erase time.
DELETE FROM coupon_redemptions WHERE user_id IS NULL;

ALTER TABLE coupon_redemptions
    DROP CONSTRAINT coupon_redemptions_user_id_fkey,
    ADD CONSTRAINT coupon_redemptions_user_id_fkey
        FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;
ALTER TABLE coupon_redemptions ALTER COLUMN user_id SET NOT NULL;

COMMIT;
