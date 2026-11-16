-- 001167_federation_m2_remote_provenance.up.sql
-- ActivityPub M2 (§126 two-way federation) — remote-actor provenance on the
-- native social tables so inbound activities land as first-class rows:
--   * comments.remote_ap_actor_id — remote reply authorship (mirrors
--     §127's remote_atproto_actor_id; comments.remote_uri stays the shared
--     dedupe key across both bridges)
--   * follows.follower_remote_actor_id — remote followers listed as real
--     followers (follower_id goes nullable; exactly one side set)
--   * reactions.remote_actor_id — remote Likes counted as real reactions
--     (user_id goes nullable; composite PK can't hold NULL so it moves to
--     a surrogate id PK + partial uniques preserving "one reaction per
--     actor per entity" on both sides)
--
-- Ref: japaconnect-engineering/sections/126-activitypub-federation.md

BEGIN;

ALTER TABLE comments ADD COLUMN remote_ap_actor_id uuid REFERENCES remote_actors(id);
CREATE INDEX idx_comments_remote_ap_actor ON comments(remote_ap_actor_id)
    WHERE remote_ap_actor_id IS NOT NULL;

ALTER TABLE follows ALTER COLUMN follower_id DROP NOT NULL;
ALTER TABLE follows ADD COLUMN follower_remote_actor_id uuid REFERENCES remote_actors(id) ON DELETE CASCADE;
ALTER TABLE follows ADD CONSTRAINT follows_follower_xor_check
    CHECK ((follower_id IS NOT NULL) <> (follower_remote_actor_id IS NOT NULL));
-- The table unique (follower_id, entity_type, entity_id) treats NULLs as
-- distinct, so remote provenance needs its own partial unique.
CREATE UNIQUE INDEX idx_follows_remote_actor_unique
    ON follows (follower_remote_actor_id, entity_type, entity_id)
    WHERE follower_remote_actor_id IS NOT NULL;
CREATE INDEX idx_follows_remote_actor ON follows (follower_remote_actor_id)
    WHERE follower_remote_actor_id IS NOT NULL;

ALTER TABLE reactions ADD COLUMN id uuid DEFAULT gen_random_uuid();
ALTER TABLE reactions DROP CONSTRAINT reactions_pkey;
ALTER TABLE reactions ADD PRIMARY KEY (id);
ALTER TABLE reactions ALTER COLUMN user_id DROP NOT NULL;
ALTER TABLE reactions ADD COLUMN remote_actor_id uuid REFERENCES remote_actors(id) ON DELETE CASCADE;
ALTER TABLE reactions ADD CONSTRAINT reactions_actor_xor_check
    CHECK ((user_id IS NOT NULL) <> (remote_actor_id IS NOT NULL));
-- Partial uniques replace the composite PK's dedupe: "one reaction per
-- actor per entity" holds for users and remote actors independently.
CREATE UNIQUE INDEX idx_reactions_user_entity
    ON reactions (entity_type, entity_id, user_id)
    WHERE user_id IS NOT NULL;
CREATE UNIQUE INDEX idx_reactions_remote_entity
    ON reactions (entity_type, entity_id, remote_actor_id)
    WHERE remote_actor_id IS NOT NULL;
CREATE INDEX idx_reactions_remote_actor ON reactions (remote_actor_id)
    WHERE remote_actor_id IS NOT NULL;

COMMIT;
