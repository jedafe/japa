-- 001001 — composite tail-read indexes for the seq-ordered ledger tails.
-- GetLatestBalance reads WHERE user_id=? ORDER BY seq DESC LIMIT 1; the
-- (user_id, created_at DESC) index served that approximately, but seq is
-- now the canonical ordering column so the index matches the query.
BEGIN;

CREATE INDEX idx_wallet_ledger_user_seq
    ON wallet_ledger (user_id, seq DESC);
CREATE INDEX idx_credits_ledger_user_seq
    ON credits_ledger (user_id, seq DESC);

COMMIT;
