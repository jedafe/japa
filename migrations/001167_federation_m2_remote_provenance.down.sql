-- 001167_federation_m2_remote_provenance.down.sql

BEGIN;

-- Remote-provenance rows must go before the constraints come back —
-- NOT NULL and the composite PK can't be restored over them.
DELETE FROM comments WHERE remote_ap_actor_id IS NOT NULL;
DELETE FROM follows WHERE follower_remote_actor_id IS NOT NULL;
DELETE FROM reactions WHERE remote_actor_id IS NOT NULL;

DROP INDEX IF EXISTS idx_reactions_remote_actor;
DROP INDEX IF EXISTS idx_reactions_remote_entity;
DROP INDEX IF EXISTS idx_reactions_user_entity;
ALTER TABLE reactions DROP CONSTRAINT reactions_actor_xor_check;
ALTER TABLE reactions DROP COLUMN remote_actor_id;
ALTER TABLE reactions ALTER COLUMN user_id SET NOT NULL;
ALTER TABLE reactions DROP CONSTRAINT reactions_pkey;
ALTER TABLE reactions DROP COLUMN id;
ALTER TABLE reactions ADD CONSTRAINT reactions_pkey
    PRIMARY KEY (entity_type, entity_id, user_id);

DROP INDEX IF EXISTS idx_follows_remote_actor;
DROP INDEX IF EXISTS idx_follows_remote_actor_unique;
ALTER TABLE follows DROP CONSTRAINT follows_follower_xor_check;
ALTER TABLE follows DROP COLUMN follower_remote_actor_id;
ALTER TABLE follows ALTER COLUMN follower_id SET NOT NULL;

DROP INDEX IF EXISTS idx_comments_remote_ap_actor;
ALTER TABLE comments DROP COLUMN remote_ap_actor_id;

COMMIT;
