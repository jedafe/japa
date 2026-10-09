DROP INDEX IF EXISTS idx_community_orgs_claimant;

ALTER TABLE community_organisations
    DROP COLUMN IF EXISTS claimed_by_user_id,
    DROP COLUMN IF EXISTS claim_rejected_at;
