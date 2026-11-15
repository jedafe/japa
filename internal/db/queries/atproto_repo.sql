-- Native AT Protocol mini-PDS (§127, ADR-139) — A2 repo engine queries.
-- Repo state lives in three tables: signed commit chain
-- (atproto_repo_commits), content-addressed blocks (atproto_repo_blocks),
-- and the at:// URI projection index (atproto_records). atproto_blobs is
-- the media blob store A4's uploadBlob/getBlob surface serves.
--
-- Write path is transactional: LockAtprotoAccount serializes same-repo
-- writers (FOR UPDATE), blocks+commit+records-index+repo_seq persist in
-- one commit.

-- name: LockAtprotoAccountForRepo :one
-- user_id comes along so the write path can gate on the member's live
-- platform status (suspended users can't commit) without a second lock.
SELECT id, user_id, did, repo_seq, signing_key_enc, status
FROM atproto_accounts
WHERE id = @id
FOR UPDATE;

-- name: GetAtprotoRepoHead :one
SELECT * FROM atproto_repo_commits
WHERE account_id = @account_id
ORDER BY seq DESC
LIMIT 1;

-- name: ListAtprotoRepoBlocks :many
SELECT * FROM atproto_repo_blocks
WHERE account_id = @account_id;

-- name: InsertAtprotoRepoBlock :exec
-- blocks are content-addressed and immutable; a conflicting row holds the
-- same bytes, so retries/idempotent writes are safe.
INSERT INTO atproto_repo_blocks (account_id, cid, data)
VALUES (@account_id, @cid, @data)
ON CONFLICT (account_id, cid) DO NOTHING;

-- name: InsertAtprotoRepoCommit :one
INSERT INTO atproto_repo_commits (account_id, seq, cid, rev, prev_cid, data)
VALUES (@account_id, @seq, @cid, @rev, @prev_cid, @data)
RETURNING *;

-- name: BumpAtprotoRepoSeq :exec
UPDATE atproto_accounts
SET repo_seq = @repo_seq, updated_at = now()
WHERE id = @id;

-- name: UpsertAtprotoRecord :one
INSERT INTO atproto_records (uri, account_id, collection, rkey, cid, content_id)
VALUES (@uri, @account_id, @collection, @rkey, @cid, @content_id)
ON CONFLICT (uri) DO UPDATE
SET cid = @cid, content_id = @content_id, updated_at = now()
RETURNING *;

-- name: DeleteAtprotoRecord :exec
DELETE FROM atproto_records
WHERE account_id = @account_id AND collection = @collection AND rkey = @rkey;

-- name: GetAtprotoRecord :one
SELECT * FROM atproto_records
WHERE uri = @uri;

-- name: ListAtprotoRecords :many
SELECT * FROM atproto_records
WHERE account_id = @account_id
ORDER BY collection, rkey;

-- name: CountAtprotoRecords :one
SELECT COUNT(*)::bigint FROM atproto_records
WHERE account_id = @account_id;

-- name: GetAtprotoAccountByID :one
SELECT * FROM atproto_accounts WHERE id = @id;

-- name: GetAtprotoAccountStatusByID :one
-- Lightweight status read for write-paths that must refuse inactive
-- accounts without taking the FOR UPDATE repo lock (blob puts — the
-- blobs aren't signed repo data, so the repo lock is unnecessary).
SELECT status FROM atproto_accounts WHERE id = @id;

-- name: InsertAtprotoBlob :one
INSERT INTO atproto_blobs (account_id, cid, mime, size, data)
VALUES (@account_id, @cid, @mime, @size, @data)
ON CONFLICT (account_id, cid) DO UPDATE SET mime = @mime
RETURNING *;

-- name: GetAtprotoBlob :one
SELECT * FROM atproto_blobs
WHERE account_id = @account_id AND cid = @cid;

-- name: GetAtprotoAccountIDByContent :one
-- Content-keyed account resolution: the single source of truth for which
-- repo a contents row projects into — callers pass only content_id so a
-- (account, content) pairing bug is impossible by construction. The
-- users join mirrors GetAtprotoAccountByUserID / AP actor resolution —
-- suspended members don't project (§127: same eligibility as AP).
SELECT a.id
FROM atproto_accounts a
JOIN contents c ON c.author_id = a.user_id
JOIN users u ON u.id = a.user_id
WHERE c.id = @content_id
  AND a.status = 'active'
  AND u.status = 'active';

-- name: GetAtprotoProjectableContent :one
-- Eligibility mirrors federation's GetFederatableContentByID: published +
-- public + not deleted + federatable type (§127 Security: "same
-- eligibility as AP"). Also returns body (markdown) — feed.post text is
-- projected from the plain-text strip, not body_html.
SELECT c.id, c.author_id, c.content_type, c.title, c.slug, c.body,
       c.canonical_url, c.published_at, c.updated_at, c.created_at
FROM contents c
WHERE c.id = @id
  AND c.status = 'published'
  AND c.visibility = 'public'
  AND c.deleted_at IS NULL
  AND c.federate_opt_out = false
  AND c.content_type = ANY(@content_types::text[]);

-- name: GetAtprotoRecordsByContent :many
-- Content-keyed delete resolution: index rows are the durable proof a
-- record was projected, so they resolve the (account_id, uri) delete
-- task even after the contents row is soft-deleted/unreachable. Many,
-- not one — an author reassignment can briefly index the same content
-- under two repos until the old record's tombstone lands. The account
-- join keeps disabled-account repos frozen (deliberate §127 lifecycle —
-- emit sites skip, and the task itself no-ops on inactive accounts).
SELECT r.account_id, r.uri
FROM atproto_records r
JOIN atproto_accounts a ON a.id = r.account_id AND a.status = 'active'
WHERE r.content_id = @content_id
  AND r.collection = 'app.bsky.feed.post';

-- name: GetAtprotoUserProfile :one
SELECT display_name, bio, avatar_media_id, banner_media_id
FROM user_profiles
WHERE user_id = @user_id;

-- name: GetAtprotoUserProfileByAccount :one
-- Account-keyed variant: the publish_record task path holds only the
-- account_id — profile blob enrichment (A4 avatar/banner) resolves the
-- member's media ids through it, and the task-time refresh re-derives
-- display_name/bio (plus the username fallback) from committed state.
SELECT p.display_name, p.bio, p.avatar_media_id, p.banner_media_id, u.username
FROM user_profiles p
JOIN atproto_accounts a ON a.user_id = p.user_id
JOIN users u ON u.id = a.user_id
WHERE a.id = @account_id;

-- name: ListAtprotoContentImages :many
-- A4 feed.post embed source: a content row's image media in embed order —
-- featured/header first, then item/body blocks by position, deduped on
-- media_id (DISTINCT ON keeps the highest-priority field_key), capped at
-- the lexicon's 4-image embed limit. Confirmed, non-deleted raster images
-- only: unconfirmed media is unscanned/unreviewed, and SVG is neither a
-- legal embed type nor safe to serve through the network.
SELECT * FROM (
  SELECT DISTINCT ON (m.id)
    m.id, m.mime_type, m.storage_bucket, m.storage_path,
    m.alt_text, m.width_px, m.height_px, m.file_size_bytes,
    CASE cmf.field_key WHEN 'featured_image' THEN 0
                       WHEN 'header_image' THEN 1
                       WHEN 'item_media' THEN 2
                       WHEN 'body_block' THEN 3
                       ELSE 4 END AS field_rank,
    cmf.position
  FROM content_media_fields cmf
  JOIN media m ON m.id = cmf.media_id
  WHERE cmf.content_id = @content_id
    AND m.deleted_at IS NULL
    AND m.confirmed
    AND m.mime_type LIKE 'image/%'
    AND m.mime_type <> 'image/svg+xml'
  ORDER BY m.id, field_rank, cmf.position
) ranked
ORDER BY field_rank, position
LIMIT 4;

-- name: ListAtprotoReconcileMissingPosts :many
-- A4 reconcile: the member's canonically-eligible contents with no
-- index row under this account — content published inside a disabled
-- window never emitted (emitAtprotoPublish skips inactive accounts),
-- so the index walk alone can't see it. Eligibility mirrors
-- GetAtprotoProjectableContent's predicate.
SELECT c.id
FROM contents c
WHERE c.author_id = @user_id
  AND c.status = 'published'
  AND c.visibility = 'public'
  AND c.deleted_at IS NULL
  AND c.federate_opt_out = false
  AND c.content_type = ANY(@content_types::text[])
  AND NOT EXISTS (
    SELECT 1 FROM atproto_records r
    WHERE r.content_id = c.id AND r.account_id = @account_id
  );

-- name: ListAtprotoBlobsByAccount :many
-- CAR traversal sees embedded blob CIDs as DAG links — the blockstore
-- must resolve them or car.WriteCar aborts on the missing leaf (A4).
SELECT cid, data FROM atproto_blobs WHERE account_id = @account_id;

-- name: DeleteAtprotoBlobsForAccount :execrows
-- A4 erasure: blob bytes are member media — they must not outlive the
-- tombstone (§127 Content pipeline). Erasure is the only caller; blobs
-- are content-addressed and immutable, never updated.
DELETE FROM atproto_blobs WHERE account_id = @account_id;
