-- §128: record HOW a completed tip settled so refunds only reverse the
-- ledger the settle actually wrote. Fiat tips used to land exclusively in
-- the spend-only 'credits' bonus ledger; with the stablecoin rail live a
-- fiat tip settles to the recipient's backed onchain wallet instead.
-- Clawing credits back from a backed-settled tip would drive the bonus
-- ledger negative on funds that were never minted.
ALTER TABLE tips ADD COLUMN settled_via text NOT NULL DEFAULT 'credits';
ALTER TABLE tips ADD CONSTRAINT tips_settled_via_check
    CHECK (settled_via IN ('credits', 'backed'));

-- Onchain-settled tips already completed before this column existed.
UPDATE tips SET settled_via = 'backed' WHERE method = 'stablecoin' AND status = 'completed';
