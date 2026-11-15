-- 001157_verification_request_service_listing.up.sql
-- §07 Verification Requests: add 'service_listing' as a third entity_type so
-- agents/providers can file evidence-backed badge requests for their own
-- listings (§98 service_listing-scope verification types already exist but
-- the request flow only reached organization|user). The badge grant remains
-- the separate admin step (PATCH /admin/services/{id}/verify).

BEGIN;

ALTER TABLE verification_requests
  ADD COLUMN IF NOT EXISTS service_listing_id uuid REFERENCES contents(id);

ALTER TABLE verification_requests
  DROP CONSTRAINT verification_requests_entity_type_check;
ALTER TABLE verification_requests
  ADD CONSTRAINT verification_requests_entity_type_check
  CHECK (entity_type IN ('organization','user','service_listing'));

ALTER TABLE verification_requests
  DROP CONSTRAINT vreq_org_entity;
ALTER TABLE verification_requests
  ADD CONSTRAINT vreq_org_entity CHECK (
    (entity_type = 'organization'    AND organization_id IS NOT NULL AND service_listing_id IS NULL) OR
    (entity_type = 'user'            AND organization_id IS NULL     AND service_listing_id IS NULL) OR
    (entity_type = 'service_listing' AND service_listing_id IS NOT NULL AND organization_id IS NULL)
  );

-- One pending request per listing — mirrors idx_vreq_pending_org/user.
CREATE UNIQUE INDEX IF NOT EXISTS idx_vreq_pending_listing
  ON verification_requests (service_listing_id)
  WHERE status = 'pending' AND entity_type = 'service_listing';

CREATE INDEX IF NOT EXISTS idx_vreq_listing
  ON verification_requests (service_listing_id)
  WHERE entity_type = 'service_listing';

COMMIT;
