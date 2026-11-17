-- 001183_federation_m5_consume_side.up.sql
-- ActivityPub M5 (§126 consume-side federation):
--   * federation_follows.direction — outbound edges (member→remote follows)
--     share the edge table; the unique widens to (local_actor, remote_actor,
--     direction) so a mutual relationship can hold both directions.
--   * follows.entity_type += 'remote_actor' — the member's native following
--     graph covers remote accounts (entity_id → remote_actors.id).
--   * federation_remote_posts — remote statuses for the member timeline.
--   * content_reports.reporter_remote_actor_id — inbound Flag provenance.
--   * federated_actors.moved_to_url/moved_at — outbound Move advisory.
--   * settings seed: federation.move.cooldown_days, federation.timeline_page_size
--
-- Ref: .jeos/sections/126-activitypub-federation.md (M5 contract)

BEGIN;

-- 1. Bidirectional follow edges ------------------------------------------------
ALTER TABLE federation_follows
    ADD COLUMN direction text NOT NULL DEFAULT 'inbound';
ALTER TABLE federation_follows
    ADD CONSTRAINT federation_follows_direction_check
        CHECK (direction IN ('inbound', 'outbound'));
ALTER TABLE federation_follows
    DROP CONSTRAINT federation_follows_local_actor_id_remote_actor_id_key;
ALTER TABLE federation_follows
    ADD CONSTRAINT federation_follows_edge_unique
        UNIQUE (local_actor_id, remote_actor_id, direction);
-- Outbound-answer lookup (Accept/Reject matches ap_activity_id) and the
-- remote-actor side of Block/Delete unwinds.
CREATE INDEX idx_federation_follows_ap_id
    ON federation_follows (ap_activity_id) WHERE ap_activity_id IS NOT NULL;
CREATE INDEX idx_federation_follows_remote_dir
    ON federation_follows (remote_actor_id, direction, status);

-- 2. Native follows covers remote-actor targets --------------------------------
ALTER TABLE follows DROP CONSTRAINT follows_entity_type_check;
ALTER TABLE follows ADD CONSTRAINT follows_entity_type_check
    CHECK (entity_type = ANY (ARRAY[
        'user'::text, 'space'::text, 'tag'::text, 'organization'::text,
        'professional_profile'::text, 'source'::text, 'remote_actor'::text]));
-- Remote-target mirrors carry follower_id (local member) so the existing
-- (follower_id, entity_type, entity_id) unique already dedupes them.

-- 3. Remote timeline posts ------------------------------------------------------
CREATE TABLE federation_remote_posts (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    remote_actor_id uuid NOT NULL REFERENCES remote_actors(id) ON DELETE CASCADE,
    ap_id           text NOT NULL UNIQUE,
    in_reply_to_url text,
    url             text,
    name            text,
    summary         text,
    content_text    text NOT NULL,
    sensitive       boolean NOT NULL DEFAULT false,
    hashtags        text[] NOT NULL DEFAULT '{}',
    is_pinned       boolean NOT NULL DEFAULT false,
    published_at    timestamptz,
    edited_at       timestamptz,
    deleted_at      timestamptz,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_fed_remote_posts_actor
    ON federation_remote_posts (remote_actor_id, published_at DESC)
    WHERE deleted_at IS NULL;

-- 4. Flag provenance ------------------------------------------------------------
ALTER TABLE content_reports
    ADD COLUMN reporter_remote_actor_id uuid REFERENCES remote_actors(id) ON DELETE CASCADE;
CREATE UNIQUE INDEX idx_reports_remote_reporter_entity_unique
    ON content_reports (reporter_remote_actor_id, entity_type, entity_id)
    WHERE reporter_remote_actor_id IS NOT NULL;
CREATE INDEX idx_reports_remote_reporter
    ON content_reports (reporter_remote_actor_id)
    WHERE reporter_remote_actor_id IS NOT NULL;

-- 5. Outbound Move advisory ------------------------------------------------------
ALTER TABLE federated_actors
    ADD COLUMN moved_to_url text,
    ADD COLUMN moved_at timestamptz;

-- 6. Settings seed ----------------------------------------------------------------
INSERT INTO system_settings (key, value, is_public) VALUES
    ('federation.move.cooldown_days', to_jsonb(30), false),
    ('federation.timeline_page_size', to_jsonb(20), false)
ON CONFLICT (key) DO NOTHING;

COMMIT;
