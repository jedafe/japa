-- 001157_verification_request_service_listing.down.sql

BEGIN;

-- Block the downgrade while listing requests exist — dropping the column
-- would orphan them; an admin must resolve/delete those rows first.
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM verification_requests WHERE entity_type = 'service_listing') THEN
    RAISE EXCEPTION 'verification_requests rows with entity_type=service_listing exist — resolve them before downgrading';
  END IF;
END $$;

DROP INDEX IF EXISTS idx_vreq_listing;
DROP INDEX IF EXISTS idx_vreq_pending_listing;

ALTER TABLE verification_requests
  DROP CONSTRAINT vreq_org_entity;
ALTER TABLE verification_requests
  ADD CONSTRAINT vreq_org_entity CHECK (
    (entity_type = 'organization' AND organization_id IS NOT NULL) OR
    (entity_type = 'user' AND organization_id IS NULL)
  );

ALTER TABLE verification_requests
  DROP CONSTRAINT verification_requests_entity_type_check;
ALTER TABLE verification_requests
  ADD CONSTRAINT verification_requests_entity_type_check
  CHECK (entity_type IN ('organization','user'));

ALTER TABLE verification_requests
  DROP COLUMN IF EXISTS service_listing_id;

COMMIT;
