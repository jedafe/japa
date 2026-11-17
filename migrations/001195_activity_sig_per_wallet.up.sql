-- §128: one onchain tx fires a provider event per participating wallet —
-- a member→member send is tip_out for the sender AND tip_in for the
-- recipient under the SAME tx_signature. Dedupe must scope to
-- (wallet_id, tx_signature) or the second wallet's mirror row is dropped —
-- and the sender's missing out-row also undercounts the delegated-signer
-- daily cap (SumOutActivityByAssetSince).
ALTER TABLE stablecoin_activity DROP CONSTRAINT stablecoin_activity_tx_signature_unique;
ALTER TABLE stablecoin_activity ADD CONSTRAINT stablecoin_activity_wallet_tx_unique
    UNIQUE (wallet_id, tx_signature);
