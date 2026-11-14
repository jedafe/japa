-- 001146_atproto_commit_blocks.up.sql
-- §127 A3: per-commit block index powering getRepo?since. Each row names
-- one block a commit introduced (its dirty set — commit block, new MST
-- nodes, record blocks), letting the sync surface serve a true diff CAR
-- instead of always the full repo. Same data the #commit firehose frame
-- emits as its diff; rows write inside the repo-write tx.
--
-- Ref: japaconnect-engineering/sections/127-atproto-native.md

BEGIN;

CREATE TABLE IF NOT EXISTS atproto_commit_blocks (
    account_id  uuid NOT NULL,
    seq         bigint NOT NULL,
    cid         text NOT NULL,
    PRIMARY KEY (account_id, seq, cid),
    FOREIGN KEY (account_id, seq)
        REFERENCES atproto_repo_commits (account_id, seq) ON DELETE CASCADE
);

COMMIT;
