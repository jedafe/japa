-- 001144_atproto_repo.up.sql
-- Native AT Protocol mini-PDS (§127, ADR-139) — A2 repo engine schema:
-- atproto_repo_commits (signed commit chain), atproto_repo_blocks (MST +
-- record + commit blocks keyed by CID), atproto_records (at:// URI index
-- back to contents), atproto_blobs (media blob store for A4 uploadBlob/
-- getBlob). Blocks are content-addressed + immutable — ON CONFLICT
-- re-inserts are writes of the same bytes.
--
-- Ref: japaconnect-engineering/sections/127-atproto-native.md

BEGIN;

CREATE TABLE IF NOT EXISTS atproto_repo_commits (
    account_id  uuid NOT NULL REFERENCES atproto_accounts(id) ON DELETE CASCADE,
    seq         bigint NOT NULL,
    cid         text NOT NULL,           -- commit block CID
    rev         text NOT NULL,           -- TID (time-sortable revision)
    prev_cid    text,                    -- repo v3 commits carry no prev chain; kept for v2 compat
    data        bytea NOT NULL,          -- signed commit, dag-cbor encoded
    created_at  timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (account_id, seq)
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_atproto_repo_commits_cid ON atproto_repo_commits (cid);
-- (no head index: PK (account_id, seq) already serves "latest seq per
-- account" via backward scan)

CREATE TABLE IF NOT EXISTS atproto_repo_blocks (
    account_id  uuid NOT NULL REFERENCES atproto_accounts(id) ON DELETE CASCADE,
    cid         text NOT NULL,
    data        bytea NOT NULL,          -- dag-cbor block (MST node, record, or commit)
    created_at  timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (account_id, cid)
);

CREATE TABLE IF NOT EXISTS atproto_records (
    uri         text PRIMARY KEY,        -- at://{did}/{collection}/{rkey}
    account_id  uuid NOT NULL REFERENCES atproto_accounts(id) ON DELETE CASCADE,
    collection  text NOT NULL,           -- app.bsky.actor.profile | app.bsky.feed.post
    rkey        text NOT NULL,
    cid         text NOT NULL,           -- record block CID
    -- ON DELETE SET NULL: deleting a contents row detaches the projection
    -- (the repo record is deleted by the same flow first); a dangling
    -- content_id would cascade the record away and orphan the MST entry.
    content_id  uuid REFERENCES contents(id) ON DELETE SET NULL,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_atproto_records_coll_rkey ON atproto_records (account_id, collection, rkey);
CREATE INDEX IF NOT EXISTS idx_atproto_records_content ON atproto_records (content_id) WHERE content_id IS NOT NULL;

CREATE TABLE IF NOT EXISTS atproto_blobs (
    account_id  uuid NOT NULL REFERENCES atproto_accounts(id) ON DELETE CASCADE,
    cid         text NOT NULL,           -- raw-codec sha256 CID (atproto blob ref)
    mime        text NOT NULL,
    size        bigint NOT NULL,
    data        bytea NOT NULL,
    created_at  timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (account_id, cid)
);

COMMIT;
