-- §22 appeal subject community_org_claim: community_organisations claim
-- rows carry no claimant identity today — InitiateClaim flips
-- claim_status='pending' with no record of who claimed, and ResolveClaim
-- rejects back to 'unclaimed' losing it entirely. Without a claimant there
-- is nobody to authorize for an appeal, so track them.
--
-- claimed_by_user_id    — most recent claimant (kept across rejection so a
--                         rejected claimant can appeal; overwritten by the
--                         next InitiateClaim)
-- claim_rejected_at     — rejection marker: claim_status resets to
--                         'unclaimed' so a *new* claimant can still claim,
--                         but this timestamp makes the previous rejection
--                         appealable and distinguishable from never-claimed.

ALTER TABLE community_organisations
    ADD COLUMN IF NOT EXISTS claimed_by_user_id uuid REFERENCES users(id),
    ADD COLUMN IF NOT EXISTS claim_rejected_at timestamptz;

CREATE INDEX IF NOT EXISTS idx_community_orgs_claimant
    ON community_organisations (claimed_by_user_id)
    WHERE claim_rejected_at IS NOT NULL;
