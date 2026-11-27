-- 001237_verification_requests_entity_fk_cascade.up.sql
-- verification_requests entity-target FKs are NO ACTION (default): an admin
-- hard-delete of an organization/contents row/space that still has requests
-- fails with an FK violation instead of cleanly removing the dependent rows.
-- A request is meaningless without its target — cascade on entity delete.
-- requester_user_id/reviewed_by stay untouched (GDPR erasure already deletes
-- by requester and nullifies the reviewer).
--
-- Ref: sections/07-organization-profiles-claim-ownership.md (Verification Requests)

BEGIN;

ALTER TABLE verification_requests
  DROP CONSTRAINT verification_requests_organization_id_fkey;
ALTER TABLE verification_requests
  ADD CONSTRAINT verification_requests_organization_id_fkey
  FOREIGN KEY (organization_id) REFERENCES organizations(id) ON DELETE CASCADE;

ALTER TABLE verification_requests
  DROP CONSTRAINT verification_requests_service_listing_id_fkey;
ALTER TABLE verification_requests
  ADD CONSTRAINT verification_requests_service_listing_id_fkey
  FOREIGN KEY (service_listing_id) REFERENCES contents(id) ON DELETE CASCADE;

ALTER TABLE verification_requests
  DROP CONSTRAINT verification_requests_space_id_fkey;
ALTER TABLE verification_requests
  ADD CONSTRAINT verification_requests_space_id_fkey
  FOREIGN KEY (space_id) REFERENCES spaces(id) ON DELETE CASCADE;

COMMIT;
