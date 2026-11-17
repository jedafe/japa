-- Restore global sig uniqueness: collapse cross-wallet duplicates first
-- (keep the earliest row per signature).
DELETE FROM stablecoin_activity a
USING stablecoin_activity b
WHERE a.tx_signature IS NOT NULL
  AND a.tx_signature = b.tx_signature
  AND a.id > b.id;
ALTER TABLE stablecoin_activity DROP CONSTRAINT stablecoin_activity_wallet_tx_unique;
ALTER TABLE stablecoin_activity ADD CONSTRAINT stablecoin_activity_tx_signature_unique
    UNIQUE (tx_signature);
