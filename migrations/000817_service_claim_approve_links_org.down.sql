-- 000817_service_claim_approve_links_org.down.sql
-- Restores the pre-backfill state: clears org_id on rows this migration
-- linked (still marked approved with matching claimed_by_org_id). Safe to
-- run only when org_id == claimed_by_org_id — a later re-attribution via
-- PATCH /services/{id} is untouched. Restricted to listing_kind='service':
-- packages are created pre-claimed with org_id = claimed_by_org_id, so a
-- blanket revert would corrupt legit org-authored package ownership.
UPDATE service_listing_meta
SET org_id = NULL, updated_at = now()
WHERE claim_status = 'approved'
  AND listing_kind = 'service'
  AND claimed_by_org_id IS NOT NULL
  AND org_id = claimed_by_org_id;
