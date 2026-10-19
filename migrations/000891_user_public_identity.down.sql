-- 000891_user_public_identity.down.sql — reverses the public identity columns.
BEGIN;

DROP INDEX IF EXISTS idx_users_public_username;
ALTER TABLE users DROP COLUMN IF EXISTS public_group_id;
ALTER TABLE users DROP COLUMN IF EXISTS public_username;

COMMIT;
