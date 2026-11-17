-- Revert 001188: restore member purchase grants + killswitch default,
-- drop the pilot nodes, restore counterparty FK to NO ACTION.

BEGIN;

ALTER TABLE service_escrows
    DROP CONSTRAINT service_escrows_buyer_wallet_fk,
    ADD CONSTRAINT service_escrows_buyer_wallet_fk
    FOREIGN KEY (buyer_wallet_id) REFERENCES stablecoin_wallets(id),
    DROP CONSTRAINT service_escrows_seller_wallet_fk,
    ADD CONSTRAINT service_escrows_seller_wallet_fk
    FOREIGN KEY (seller_wallet_id) REFERENCES stablecoin_wallets(id);
-- NOTE: NOT NULL restoration is omitted — rows SET NULL by an erasure
-- can't regain their wallet refs, so forcing it would fail the rollback.

ALTER TABLE stablecoin_activity
    DROP CONSTRAINT stablecoin_activity_wallet_fk,
    ADD CONSTRAINT stablecoin_activity_wallet_fk
    FOREIGN KEY (wallet_id) REFERENCES stablecoin_wallets(id);

ALTER TABLE stablecoin_wallets
    DROP CONSTRAINT stablecoin_wallets_user_fk,
    ADD CONSTRAINT stablecoin_wallets_user_fk
    FOREIGN KEY (user_id) REFERENCES users(id);

ALTER TABLE stablecoin_activity
    DROP CONSTRAINT IF EXISTS stablecoin_activity_counterparty_fk,
    ADD CONSTRAINT stablecoin_activity_counterparty_fk
    FOREIGN KEY (counterparty_user_id) REFERENCES users(id);

UPDATE system_settings SET value = 'true'::jsonb
WHERE key = 'wallet.credit_purchase_enabled';

ALTER TABLE tips DROP CONSTRAINT tips_method_check;
ALTER TABLE tips ADD CONSTRAINT tips_method_check
    CHECK (method = ANY (ARRAY['points','credits','stripe','paystack','nowpayments','manual_transfer','opencollective','kofi','buymeacoffee']));

INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id FROM user_roles ur, permissions p
WHERE ur.slug IN ('member', 'pro_member', 'business_member')
  AND p.key = 'wallet.credits.purchase'
ON CONFLICT DO NOTHING;

DELETE FROM role_permissions rp
USING permissions p
WHERE rp.permission_id = p.id
  AND p.key IN ('stablecoin.wallet.use', 'tips.stablecoin', 'tips.credits', 'escrow.use');

DELETE FROM permissions
WHERE key IN ('stablecoin.wallet.use', 'tips.stablecoin', 'tips.credits', 'escrow.use');

COMMIT;
