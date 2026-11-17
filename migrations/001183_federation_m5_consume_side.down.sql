-- 001183_federation_m5_consume_side.down.sql
-- Reverses the M5 consume-side schema. Order matters: outbound edges and
-- remote-actor follow mirrors are removed BEFORE the direction column /
-- entity_type CHECK tighten, or the restored constraints would collide.

BEGIN;

DELETE FROM system_settings
 WHERE key IN ('federation.move.cooldown_days', 'federation.timeline_page_size');

ALTER TABLE federated_actors
    DROP COLUMN moved_to_url,
    DROP COLUMN moved_at;

DROP INDEX idx_reports_remote_reporter_entity_unique;
DROP INDEX idx_reports_remote_reporter;
ALTER TABLE content_reports
    DROP COLUMN reporter_remote_actor_id;

DROP TABLE federation_remote_post_shares;
DROP TABLE federation_remote_posts;

DELETE FROM follows WHERE entity_type = 'remote_actor';
ALTER TABLE follows DROP CONSTRAINT follows_entity_type_check;
ALTER TABLE follows ADD CONSTRAINT follows_entity_type_check
    CHECK (entity_type = ANY (ARRAY[
        'user'::text, 'space'::text, 'tag'::text, 'organization'::text,
        'professional_profile'::text, 'source'::text]));

DELETE FROM federation_follows WHERE direction = 'outbound';
ALTER TABLE federation_follows DROP CONSTRAINT federation_follows_edge_unique;
ALTER TABLE federation_follows DROP CONSTRAINT federation_follows_direction_check;
ALTER TABLE federation_follows DROP COLUMN direction;
ALTER TABLE federation_follows
    ADD CONSTRAINT federation_follows_local_actor_id_remote_actor_id_key
        UNIQUE (local_actor_id, remote_actor_id);

COMMIT;
