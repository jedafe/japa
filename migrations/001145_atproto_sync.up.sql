-- 001145_atproto_sync.up.sql
-- Native AT Protocol mini-PDS (§127, ADR-139) — A3 sync surface:
-- atproto_repo_events is the subscribeRepos firehose sequencer. Every
-- #commit frame inserts inside the repo-write transaction, so seq order
-- == commit order across all accounts and a committed rev can never be
-- skipped by the stream. identity/account rows cover handle changes and
-- activation/deactivation (the modern lexicon's replacements for the
-- deprecated #handle frame).
--
-- payload holds the subscribeRepos frame body minus seq/time (stamped
-- from the row at emit): SyncSubscribeRepos_Commit (blocks CAR + ops +
-- rev/prevData) or _Identity (did+handle) or _Account (did+active+status).
--
-- Ref: japaconnect-engineering/sections/127-atproto-native.md

BEGIN;

CREATE TABLE IF NOT EXISTS atproto_repo_events (
    seq         bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    did         text NOT NULL,           -- repo DID the frame belongs to
    kind        text NOT NULL,
    payload     jsonb NOT NULL,          -- frame body (seq/time stamped at emit)
    created_at  timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT atproto_repo_events_kind_check CHECK (kind IN ('commit','identity','account'))
);

COMMIT;
