-- Native AT Protocol mini-PDS (§127, ADR-139) — A3 sync surface queries.
-- atproto_repo_events is the subscribeRepos sequencer: commit rows land
-- inside the repo-write tx (seq order == commit order), identity/account
-- rows land best-effort on lifecycle transitions. Consumers replay from
-- a cursor (last-seen seq) then live-tail.

-- name: InsertAtprotoRepoEvent :one
INSERT INTO atproto_repo_events (did, kind, payload)
VALUES (@did, @kind, @payload::jsonb)
RETURNING seq;

-- name: ListAtprotoRepoEventsAfter :many
-- Cursor replay + live tail for subscribeRepos — strict seq ordering.
SELECT * FROM atproto_repo_events
WHERE seq > @after
ORDER BY seq
LIMIT @page_limit;

-- name: GetAtprotoRepoEventMaxSeq :one
-- Cursor validation: a subscribeRepos cursor beyond the current max is a
-- FutureCursor error frame.
SELECT COALESCE(MAX(seq), 0)::bigint AS max_seq FROM atproto_repo_events;

-- name: GetAtprotoRecordByPath :one
-- Path-keyed record read: commit-event op building (prev CID for
-- update/delete) and getRecord's existence check.
SELECT * FROM atproto_records
WHERE account_id = @account_id AND collection = @collection AND rkey = @rkey;

-- name: ListAtprotoRepoBlocksByCids :many
-- getBlocks: fetch a caller-named CID slice from one account's block set.
SELECT * FROM atproto_repo_blocks
WHERE account_id = @account_id AND cid = ANY(@cids::text[]);

-- name: InsertAtprotoCommitBlock :exec
-- One row per block a commit introduced — the getRepo?since index.
INSERT INTO atproto_commit_blocks (account_id, seq, cid)
VALUES (@account_id, @seq, @cid);

-- name: ListAtprotoCommitsAfterRev :many
-- getRepo?since: commits newer than the client's rev. Revs are TIDs —
-- lexicographic order == chronological.
SELECT * FROM atproto_repo_commits
WHERE account_id = @account_id AND rev > @rev
ORDER BY seq;

-- name: ListAtprotoCommitBlocks :many
-- The introduced-block set for a seq range — joined to repo_blocks for
-- data by the caller (blocks are content-addressed, no dup data needed).
SELECT seq, cid FROM atproto_commit_blocks
WHERE account_id = @account_id AND seq = ANY(@seqs::bigint[])
ORDER BY seq;

-- name: AtprotoCommitRevExists :one
-- getRepo?since validation: a since naming a rev we never minted tells
-- us nothing about the client's state — the full CAR is the only
-- provably-sufficient answer.
SELECT EXISTS(
    SELECT 1 FROM atproto_repo_commits
    WHERE account_id = @account_id AND rev = @rev
);
