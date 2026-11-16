-- 001175_federation_m3_discovery.down.sql
BEGIN;

DELETE FROM system_settings WHERE key = 'federation.authorized_fetch';
DROP TABLE IF EXISTS federation_relays;
DROP INDEX IF EXISTS federated_actors_application_uidx;
DROP INDEX IF EXISTS federated_actors_space_uidx;
DROP INDEX IF EXISTS idx_federated_actors_kind;
ALTER TABLE federated_actors DROP CONSTRAINT IF EXISTS federated_actors_kind_user_match;
ALTER TABLE federated_actors DROP CONSTRAINT IF EXISTS federated_actors_kind_check;
ALTER TABLE federated_actors DROP COLUMN IF EXISTS space_id;
ALTER TABLE federated_actors DROP COLUMN IF EXISTS kind;
-- Only re-tighten user_id when no non-person rows survive.
ALTER TABLE federated_actors ALTER COLUMN user_id SET NOT NULL;

COMMIT;
