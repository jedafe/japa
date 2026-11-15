-- Ref: japaconnect-engineering/sections/127-atproto-native.md (A5 — Jetstream inbound)
-- Remote Bluesky actors and the inbound normalization ledgers. Remote actors are
-- never users rows; they carry their own provenance (DID + handle) so comments,
-- likes, and follows can attribute them without a local account.

CREATE TABLE atproto_remote_actors (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    did           text NOT NULL UNIQUE,
    handle        text,
    display_name  text,
    avatar_url    text,
    fetched_at    timestamptz,
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_atproto_remote_actors_handle ON atproto_remote_actors(handle);

-- Audit ledger for every Jetstream event addressed at a member repo. The
-- (event_uri, operation) pair is the natural dedupe key: a create→delete pair
-- shares the URI but not the operation.
CREATE TABLE atproto_inbound (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    event_uri   text NOT NULL,
    event_seq   bigint NOT NULL DEFAULT 0,
    did         text NOT NULL,
    collection  text NOT NULL,
    operation   text NOT NULL,
    payload     jsonb,
    status      text NOT NULL DEFAULT 'received'
                CHECK (status IN ('received','processed','ignored','rejected','error')),
    error       text,
    created_at  timestamptz NOT NULL DEFAULT now(),
    UNIQUE (event_uri, operation, event_seq)
);
CREATE INDEX idx_atproto_inbound_did ON atproto_inbound(did);
CREATE INDEX idx_atproto_inbound_status ON atproto_inbound(status) WHERE status <> 'processed';

-- Remote Bluesky follows on member repos (app.bsky.graph.follow whose subject
-- is a member DID). undone_at marks a matched delete operation.
CREATE TABLE atproto_remote_follows (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id      uuid NOT NULL REFERENCES atproto_accounts(id) ON DELETE CASCADE,
    remote_actor_id uuid NOT NULL REFERENCES atproto_remote_actors(id) ON DELETE CASCADE,
    follow_uri      text NOT NULL UNIQUE,
    undone_at       timestamptz,
    created_at      timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_atproto_remote_follows_account ON atproto_remote_follows(account_id) WHERE undone_at IS NULL;

-- Remote Bluesky likes on member feed.post records (app.bsky.feed.like whose
-- subject resolves to a projected record).
CREATE TABLE atproto_remote_likes (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id      uuid NOT NULL REFERENCES atproto_accounts(id) ON DELETE CASCADE,
    remote_actor_id uuid NOT NULL REFERENCES atproto_remote_actors(id) ON DELETE CASCADE,
    like_uri        text NOT NULL UNIQUE,
    content_id      uuid REFERENCES contents(id) ON DELETE CASCADE,
    undone_at       timestamptz,
    created_at      timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_atproto_remote_likes_content ON atproto_remote_likes(content_id) WHERE undone_at IS NULL;

-- Remote-authored replies land in the normal comments tree: author_user_id
-- stays NULL and remote_atproto_actor_id + remote_uri carry the provenance.
ALTER TABLE comments ADD COLUMN remote_atproto_actor_id uuid REFERENCES atproto_remote_actors(id);
ALTER TABLE comments ADD COLUMN remote_uri text;
CREATE UNIQUE INDEX idx_comments_remote_uri ON comments(remote_uri) WHERE remote_uri IS NOT NULL;
CREATE INDEX idx_comments_remote_actor ON comments(remote_atproto_actor_id) WHERE remote_atproto_actor_id IS NOT NULL;
