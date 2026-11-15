ALTER TABLE comments DROP COLUMN IF EXISTS remote_uri;
ALTER TABLE comments DROP COLUMN IF EXISTS remote_atproto_actor_id;
DROP TABLE IF EXISTS atproto_remote_likes;
DROP TABLE IF EXISTS atproto_remote_follows;
DROP TABLE IF EXISTS atproto_inbound;
DROP TABLE IF EXISTS atproto_remote_actors;
