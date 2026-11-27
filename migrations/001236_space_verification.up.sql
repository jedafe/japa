-- 001236_space_verification.up.sql
-- §07 Verification Requests: 'space' as a fourth entity_type so a space owner
-- can file an evidence-backed badge request for their space. Ships the space
-- badge columns (spaces.verified / spaces.verification_type) and backfills
-- primary (geo/university/global — spaces.IsPrimaryLevel) platform-run spaces
-- as verified='official'; the spaces service stamps the same defaults at
-- create time. The badge grant remains the separate admin step
-- (POST /admin/spaces/{id}/verify).
--
-- Ref: sections/07-organization-profiles-claim-ownership.md (Verification Requests)

BEGIN;

-- 1. spaces: verification badge columns (mirrors users.verified /
-- users.verification_type from 000933).
ALTER TABLE spaces ADD COLUMN IF NOT EXISTS verified boolean NOT NULL DEFAULT false;
ALTER TABLE spaces ADD COLUMN IF NOT EXISTS verification_type text;
COMMENT ON COLUMN spaces.verification_type IS
  'official|community|organization — set via POST /admin/spaces/{id}/verify; primary (geo/university/global) spaces default to official';

-- Primary spaces are platform-run — verified by default.
UPDATE spaces SET verified = true, verification_type = 'official'
WHERE level IN ('country','state','city','university','global')
  AND deleted_at IS NULL;

-- 2. verification_types: 'space' scope + seeded vocabulary (protected rows).
ALTER TABLE verification_types
  DROP CONSTRAINT IF EXISTS verification_types_scope_check;
ALTER TABLE verification_types
  ADD CONSTRAINT verification_types_scope_check
  CHECK (scope IN ('organization','user','service_listing','space'));

INSERT INTO verification_types (scope, value, label, position, is_protected) VALUES
    ('space', 'official',     'Official',     10, true),
    ('space', 'community',    'Community',    20, true),
    ('space', 'organization', 'Organization', 30, true)
ON CONFLICT (scope, value) DO NOTHING;

-- 3. verification_requests: space entity target.
ALTER TABLE verification_requests
  ADD COLUMN IF NOT EXISTS space_id uuid REFERENCES spaces(id);

ALTER TABLE verification_requests
  DROP CONSTRAINT verification_requests_entity_type_check;
ALTER TABLE verification_requests
  ADD CONSTRAINT verification_requests_entity_type_check
  CHECK (entity_type IN ('organization','user','service_listing','space'));

ALTER TABLE verification_requests
  DROP CONSTRAINT vreq_org_entity;
ALTER TABLE verification_requests
  ADD CONSTRAINT vreq_org_entity CHECK (
    (entity_type = 'organization'    AND organization_id IS NOT NULL AND service_listing_id IS NULL AND space_id IS NULL) OR
    (entity_type = 'user'            AND organization_id IS NULL     AND service_listing_id IS NULL AND space_id IS NULL) OR
    (entity_type = 'service_listing' AND service_listing_id IS NOT NULL AND organization_id IS NULL AND space_id IS NULL) OR
    (entity_type = 'space'           AND space_id IS NOT NULL AND organization_id IS NULL AND service_listing_id IS NULL)
  );

-- One pending request per space — mirrors idx_vreq_pending_org/user/listing.
CREATE UNIQUE INDEX IF NOT EXISTS idx_vreq_pending_space
  ON verification_requests (space_id)
  WHERE status = 'pending' AND entity_type = 'space';

CREATE INDEX IF NOT EXISTS idx_vreq_space
  ON verification_requests (space_id)
  WHERE entity_type = 'space';

COMMIT;
