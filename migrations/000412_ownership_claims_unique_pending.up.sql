-- 000412: Add unique index on ownership_claims to prevent duplicate pending
-- claims per user per org (BUG-011).
-- Ref: japaconnect-engineering/sections/07-organization-profiles-claim-ownership.md
--
-- The spec schema defines UNIQUE(claimant_user_id, organization_id) but the
-- original migration (000105) omitted it. We use a partial unique index that
-- only applies to pending claims, allowing multiple historical (approved/
-- rejected/withdrawn) claims per user per org while preventing duplicate
-- pending claims.
CREATE UNIQUE INDEX IF NOT EXISTS idx_ownership_claims_pending_unique
    ON ownership_claims (claimant_user_id, organization_id)
    WHERE status = 'pending';
