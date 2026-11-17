-- 001185_credits_usd_anchor_numeric_rate.down.sql
-- Restore NGN-anchored integer rates and positions.

BEGIN;

UPDATE currencies SET credits_per_unit = 1350, updated_at = now() WHERE code = 'USD';
UPDATE currencies SET credits_per_unit = 1,    updated_at = now() WHERE code = 'NGN';
UPDATE currencies SET credits_per_unit = 1750, updated_at = now() WHERE code = 'GBP';
UPDATE currencies SET credits_per_unit = 1500, updated_at = now() WHERE code = 'EUR';
UPDATE currencies SET credits_per_unit = 75,   updated_at = now() WHERE code = 'ZAR';
UPDATE currencies SET credits_per_unit = 1000, updated_at = now() WHERE code = 'CAD';
UPDATE currencies SET credits_per_unit = 380,  updated_at = now() WHERE code = 'AED';
UPDATE currencies SET credits_per_unit = 110,  updated_at = now() WHERE code = 'GHS';

UPDATE currencies SET position = 1, updated_at = now() WHERE code = 'NGN';
UPDATE currencies SET position = 2, updated_at = now() WHERE code = 'USD';

-- Withdrawal rate snapshots are arbitrary admin rates, not the fixed set —
-- rescale by the anchor ratio (1350/100 = 13.5) and clamp ≥1 so a raw cast
-- doesn't zero fractional snapshots (0.065 → 0 → 1350-scale ≈ 1).
UPDATE wallet_withdrawals
SET credits_per_unit = GREATEST(1, ROUND(credits_per_unit * 13.5));

ALTER TABLE wallet_withdrawals
    ALTER COLUMN credits_per_unit TYPE integer
    USING credits_per_unit::integer;

ALTER TABLE currencies
    ALTER COLUMN credits_per_unit TYPE integer
    USING credits_per_unit::integer;

COMMIT;
