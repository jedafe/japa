-- 000688_ownership_claims_unique_pending.up.sql
-- Renumbered from 000412 — it collided with 000412_widget_configurations.
-- Tail-position body; databases that applied the 000412 file skip this via
-- the 000000 registry marker. Unchanged: already idempotent (IF NOT EXISTS).
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
