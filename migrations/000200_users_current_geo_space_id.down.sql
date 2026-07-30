DROP INDEX IF EXISTS idx_users_geo_space;
ALTER TABLE users DROP COLUMN IF EXISTS current_geo_space_id;
