-- 000652_link_entity_profiles_to_sources.down.sql

BEGIN;

DROP INDEX IF EXISTS idx_users_source_id;
DROP INDEX IF EXISTS idx_organizations_source_id;
DROP INDEX IF EXISTS idx_spaces_source_id;

ALTER TABLE users DROP COLUMN IF EXISTS source_id;
ALTER TABLE organizations DROP COLUMN IF EXISTS source_id;
ALTER TABLE spaces DROP COLUMN IF EXISTS source_id;

COMMIT;
