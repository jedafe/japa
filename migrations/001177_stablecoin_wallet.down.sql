-- 001177_stablecoin_wallet.down.sql
-- Ref: .jeos/sections/128-stablecoin-wallet-custody.md

BEGIN;

DROP TABLE IF EXISTS public.treasury_wallets;
DROP TABLE IF EXISTS public.service_escrows;
DROP TABLE IF EXISTS public.stablecoin_activity;
DROP TABLE IF EXISTS public.stablecoin_wallets;

COMMIT;
