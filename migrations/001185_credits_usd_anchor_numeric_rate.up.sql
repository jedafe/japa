-- 001185_credits_usd_anchor_numeric_rate.up.sql
-- §58: re-anchor the credit to USD — 1 credit = $0.01 (1 USD = 100
-- credits), USD is the platform default currency. credits_per_unit widens
-- to numeric(20,8) so weak currencies can express sub-1 rates (NGN ≈ 0.065
-- credits/₦ was unrepresentable as a positive integer).
--
-- Rates re-anchored from the NGN-anchored table to approximate USD FX
-- (ops should refresh periodically — see §58). Position puts USD first.
--
-- Deploy order: roll app binaries with float64 consumers FIRST, then run
-- this migration — the numeric column is unreadable by pre-change code
-- (int32 scans fail on numeric even for integral values like 100.00000000).
--
-- Ref: .jeos/sections/58-credit-currency-system.md

BEGIN;

ALTER TABLE currencies
    ALTER COLUMN credits_per_unit TYPE numeric(20,8);

-- The withdrawal-time rate snapshot widens the same way — a weak-currency
-- withdrawal can't snapshot a sub-1 rate into an integer.
ALTER TABLE wallet_withdrawals
    ALTER COLUMN credits_per_unit TYPE numeric(20,8);

-- Rescale the active set to the $1 = 100-credit anchor.
UPDATE currencies SET credits_per_unit = 100,    updated_at = now() WHERE code = 'USD';
UPDATE currencies SET credits_per_unit = 0.065,  updated_at = now() WHERE code = 'NGN';
UPDATE currencies SET credits_per_unit = 127,    updated_at = now() WHERE code = 'GBP';
UPDATE currencies SET credits_per_unit = 109,    updated_at = now() WHERE code = 'EUR';
UPDATE currencies SET credits_per_unit = 5.5,    updated_at = now() WHERE code = 'ZAR';
UPDATE currencies SET credits_per_unit = 72,     updated_at = now() WHERE code = 'CAD';
UPDATE currencies SET credits_per_unit = 27.23,  updated_at = now() WHERE code = 'AED';
UPDATE currencies SET credits_per_unit = 6.5,    updated_at = now() WHERE code = 'GHS';

-- USD first in display order.
UPDATE currencies SET position = 1, updated_at = now() WHERE code = 'USD';
UPDATE currencies SET position = 2, updated_at = now() WHERE code = 'NGN';

COMMIT;
