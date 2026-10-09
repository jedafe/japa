-- 000817_service_claim_approve_links_org.up.sql — §98 (Service Directory)
-- + §07 (Organization claim pattern) data repair.
--
-- ApproveServiceClaimIfPending previously set claim_status='approved' and
-- claimed_by_org_id without linking service_listing_meta.org_id. An
-- approved claim is the ownership transfer: without org_id the claiming
-- org cannot edit the listing (loadListingForEdit → ErrNotOwner), cannot
-- receive inquiries/bookings (service_bookings.org_id is NOT NULL), and
-- the listing never resolves org_name/org_slug on public payloads.
-- The query now writes org_id; this migration repairs rows approved
-- before that fix.
UPDATE service_listing_meta
SET org_id = claimed_by_org_id, updated_at = now()
WHERE claim_status = 'approved'
  AND claimed_by_org_id IS NOT NULL
  AND org_id IS NULL;
