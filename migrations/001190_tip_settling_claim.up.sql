-- §128: settlement claims transition pending/failed → 'settling' before
-- the onchain leg, so a racing refund/sweep can't flip the row mid-send.
-- The settle write then moves settling → completed/refunded normally.
ALTER TABLE tips DROP CONSTRAINT tips_status_check;
ALTER TABLE tips ADD CONSTRAINT tips_status_check
    CHECK ((status = ANY (ARRAY['pending'::text, 'settling'::text, 'completed'::text, 'failed'::text, 'refunded'::text])));
