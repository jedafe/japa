-- 001236_space_verification.down.sql

BEGIN;

-- Block the downgrade while space requests exist — dropping the column
-- would orphan them; an admin must resolve/delete those rows first.
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM verification_requests WHERE entity_type = 'space') THEN
    RAISE EXCEPTION 'verification_requests rows with entity_type=space exist — resolve them before downgrading';
  END IF;
END $$;

DROP INDEX IF EXISTS idx_vreq_space;
DROP INDEX IF EXISTS idx_vreq_pending_space;

ALTER TABLE verification_requests
  DROP CONSTRAINT vreq_org_entity;
ALTER TABLE verification_requests
  ADD CONSTRAINT vreq_org_entity CHECK (
    (entity_type = 'organization'    AND organization_id IS NOT NULL AND service_listing_id IS NULL) OR
    (entity_type = 'user'            AND organization_id IS NULL     AND service_listing_id IS NULL) OR
    (entity_type = 'service_listing' AND service_listing_id IS NOT NULL AND organization_id IS NULL)
  );

ALTER TABLE verification_requests
  DROP CONSTRAINT verification_requests_entity_type_check;
ALTER TABLE verification_requests
  ADD CONSTRAINT verification_requests_entity_type_check
  CHECK (entity_type IN ('organization','user','service_listing'));

ALTER TABLE verification_requests
  DROP COLUMN IF EXISTS space_id;

DELETE FROM verification_types WHERE scope = 'space';

ALTER TABLE verification_types
  DROP CONSTRAINT IF EXISTS verification_types_scope_check;
ALTER TABLE verification_types
  ADD CONSTRAINT verification_types_scope_check
  CHECK (scope IN ('organization','user','service_listing'));

ALTER TABLE spaces DROP COLUMN IF EXISTS verification_type;
ALTER TABLE spaces DROP COLUMN IF EXISTS verified;

COMMIT;
