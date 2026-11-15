DROP INDEX IF EXISTS idx_tips_wallet_recipient;
ALTER TABLE tips DROP COLUMN IF EXISTS recipient_wallet_user_id;
