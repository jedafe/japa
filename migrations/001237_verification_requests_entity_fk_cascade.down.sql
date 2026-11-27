-- 001237_verification_requests_entity_fk_cascade.down.sql
-- Restore the original NO ACTION entity FKs.

BEGIN;

ALTER TABLE verification_requests
  DROP CONSTRAINT verification_requests_organization_id_fkey;
ALTER TABLE verification_requests
  ADD CONSTRAINT verification_requests_organization_id_fkey
  FOREIGN KEY (organization_id) REFERENCES organizations(id);

ALTER TABLE verification_requests
  DROP CONSTRAINT verification_requests_service_listing_id_fkey;
ALTER TABLE verification_requests
  ADD CONSTRAINT verification_requests_service_listing_id_fkey
  FOREIGN KEY (service_listing_id) REFERENCES contents(id);

ALTER TABLE verification_requests
  DROP CONSTRAINT verification_requests_space_id_fkey;
ALTER TABLE verification_requests
  ADD CONSTRAINT verification_requests_space_id_fkey
  FOREIGN KEY (space_id) REFERENCES spaces(id);

COMMIT;
