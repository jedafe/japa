-- 001188_wallet_credit_rbac_pilot.up.sql
-- Wallet/credit RBAC hardening for staged rollout (§128/ADR-140).
--
-- 1. Member-side stablecoin nodes so the feature can be piloted by a
--    selected user group before general release — the same pattern as
--    tips.send / wallet.credits.purchase:
--      stablecoin.wallet.use — provision + use a member stablecoin wallet
--      tips.stablecoin       — send a tip that settles onchain
--      tips.credits          — send a tip that moves bonus credits
--                              (transitional; §128 retires credits tips)
--      escrow.use            — open/confirm/release a service escrow
--    All are granted to admin-tier roles only; ops assigns them to a pilot
--    group (or member tiers at GA) via the RBAC admin UI.
--
-- 2. wallet.credits.purchase is withdrawn from member tiers — credit
--    purchases are permanently disabled for members under §128 (the
--    killswitch below enforces it platform-wide; the node is retained so
--    staff can still exercise the flow in staging).
--
-- 3. wallet.credits.spend is granted explicitly to member tiers — paying
--    with bonus credits stays available to all members.
--
-- 4. wallet.credit_purchase_enabled -> false: the baseline seed was true,
--    conflicting with §128's permanent-disable requirement.
--
-- 5. stablecoin_activity.counterparty_user_id -> ON DELETE SET NULL:
--    GDPR erasure must preserve counterparty-side history while removing
--    the erased member's wallet/activity rows (dynamic FK delete would
--    otherwise hard-delete the counterparty's mirror rows or fail).

BEGIN;

INSERT INTO permissions (key, description) VALUES
    ('stablecoin.wallet.use', 'Provision and use a member stablecoin wallet (deposit, withdraw, onchain tips)'),
    ('tips.stablecoin',       'Send tips that settle onchain via the member stablecoin wallet'),
    ('tips.credits',          'Send tips denominated in bonus credits (transitional credits-ledger method)'),
    ('escrow.use',            'Create, fund, confirm and release service booking escrows')
ON CONFLICT (key) DO NOTHING;

-- Pilot nodes: admin-tier only until GA assignment.
INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id FROM user_roles ur, permissions p
WHERE ur.slug IN ('admin-group-role', 'super_admin', 'super_admin_role')
  AND p.key IN ('stablecoin.wallet.use', 'tips.stablecoin', 'tips.credits', 'escrow.use')
ON CONFLICT DO NOTHING;

-- Spend node stays with all member tiers.
INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id FROM user_roles ur, permissions p
WHERE ur.slug IN ('member', 'pro_member', 'business_member')
  AND p.key = 'wallet.credits.spend'
ON CONFLICT DO NOTHING;

-- Purchase node: remove member-tier grants, retain admin-tier.
DELETE FROM role_permissions rp
USING user_roles ur, permissions p
WHERE rp.role_id = ur.id AND rp.permission_id = p.id
  AND p.key = 'wallet.credits.purchase'
  AND ur.slug IN ('member', 'pro_member', 'business_member');

INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id FROM user_roles ur, permissions p
WHERE ur.slug IN ('admin-group-role', 'super_admin', 'super_admin_role')
  AND p.key = 'wallet.credits.purchase'
ON CONFLICT DO NOTHING;

UPDATE system_settings SET value = 'false'::jsonb
WHERE key = 'wallet.credit_purchase_enabled';

-- tips.method CHECK: admit 'stablecoin' for the §128 onchain rail.
ALTER TABLE tips DROP CONSTRAINT tips_method_check;
ALTER TABLE tips ADD CONSTRAINT tips_method_check
    CHECK (method = ANY (ARRAY['points','credits','stablecoin','stripe','paystack','nowpayments','manual_transfer','opencollective','kofi','buymeacoffee']));

ALTER TABLE stablecoin_activity
    DROP CONSTRAINT IF EXISTS stablecoin_activity_counterparty_fk,
    ADD CONSTRAINT stablecoin_activity_counterparty_fk
    FOREIGN KEY (counterparty_user_id) REFERENCES users(id) ON DELETE SET NULL;

-- GDPR hard-delete path: mirror rows cascade with the erased user (the
-- provider/chain remains authoritative; mirrors are disposable caches).
-- Without CASCADE, HardDeleteUser's DELETE FROM users fails on the
-- NO ACTION user_fk.
ALTER TABLE stablecoin_wallets
    DROP CONSTRAINT stablecoin_wallets_user_fk,
    ADD CONSTRAINT stablecoin_wallets_user_fk
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;

ALTER TABLE stablecoin_activity
    DROP CONSTRAINT stablecoin_activity_wallet_fk,
    ADD CONSTRAINT stablecoin_activity_wallet_fk
    FOREIGN KEY (wallet_id) REFERENCES stablecoin_wallets(id) ON DELETE CASCADE;

-- Escrow rows are shared records between two members — SET NULL preserves
-- the counterparty's mirror when one side is erased (columns go nullable).
ALTER TABLE service_escrows
    ALTER COLUMN buyer_wallet_id DROP NOT NULL,
    ALTER COLUMN seller_wallet_id DROP NOT NULL,
    DROP CONSTRAINT service_escrows_buyer_wallet_fk,
    ADD CONSTRAINT service_escrows_buyer_wallet_fk
    FOREIGN KEY (buyer_wallet_id) REFERENCES stablecoin_wallets(id) ON DELETE SET NULL,
    DROP CONSTRAINT service_escrows_seller_wallet_fk,
    ADD CONSTRAINT service_escrows_seller_wallet_fk
    FOREIGN KEY (seller_wallet_id) REFERENCES stablecoin_wallets(id) ON DELETE SET NULL;

COMMIT;
