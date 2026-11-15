-- Tips wallet-owner snapshot (§125 amendment): recipient_wallet_user_id
-- freezes the user whose wallet a tip settles into at write time. Org tips
-- must keep recipient_user_id NULL (tips_recipient_check) yet settle on the
-- org owner's wallet — without a snapshot, an ownership transfer between
-- tip and clawback would debit the wrong wallet, and the org owner's
-- received ledger could never match the row.
ALTER TABLE tips ADD COLUMN IF NOT EXISTS recipient_wallet_user_id uuid REFERENCES users(id);

CREATE INDEX IF NOT EXISTS idx_tips_wallet_recipient ON tips (recipient_wallet_user_id, created_at DESC);

-- Backfill: space tips already snapshot the owner into recipient_user_id;
-- org tips get the current owner as best-effort (the wallet that actually
-- settled is unrecoverable for pre-existing rows after ownership changes).
UPDATE tips SET recipient_wallet_user_id = recipient_user_id
WHERE recipient_type IN ('user','space') AND recipient_user_id IS NOT NULL;

UPDATE tips t SET recipient_wallet_user_id = o.owner_user_id
FROM organizations o
WHERE t.recipient_organization_id = o.id AND t.recipient_wallet_user_id IS NULL;
