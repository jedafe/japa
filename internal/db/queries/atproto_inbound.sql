-- Ref: japaconnect-engineering/sections/127-atproto-native.md (A5 — Jetstream inbound)

-- name: UpsertAtprotoRemoteActor :one
-- Remote actors are did-keyed, never users rows. Profile fields refresh on
-- every touch so a later event sees the actor's latest advertised handle/name.
INSERT INTO atproto_remote_actors (did, handle, display_name, avatar_url, fetched_at)
VALUES (@did, @handle, @display_name, @avatar_url, @fetched_at)
ON CONFLICT (did) DO UPDATE SET
    handle       = COALESCE(EXCLUDED.handle, atproto_remote_actors.handle),
    display_name = COALESCE(EXCLUDED.display_name, atproto_remote_actors.display_name),
    avatar_url   = COALESCE(EXCLUDED.avatar_url, atproto_remote_actors.avatar_url),
    -- fetched_at only advances when a profile fetch succeeded — a failed
    -- fetch leaves the row stale so the next event retries the AppView.
    fetched_at   = COALESCE(EXCLUDED.fetched_at, atproto_remote_actors.fetched_at),
    updated_at   = now()
RETURNING *;

-- name: GetAtprotoRemoteActorByDID :one
SELECT * FROM atproto_remote_actors WHERE did = @did;

-- name: SetAtprotoRemoteActorHandle :execrows
-- Jetstream `identity` events carry handle changes before any commit traffic
-- arrives — refresh the provenance row's display handle when we track the
-- DID (untracked DIDs are a no-op: identity events never mint actor rows).
UPDATE atproto_remote_actors SET handle = @handle, updated_at = now()
WHERE did = @did;

-- name: SetAtprotoRemoteActorDeactivated :execrows
-- Jetstream `account` events flip active:false on deactivation/takedown and
-- back to true on reactivation — mirror that on the tracked actor row.
UPDATE atproto_remote_actors
SET deactivated_at = CASE WHEN @active::bool THEN NULL ELSE now() END,
    updated_at = now()
WHERE did = @did;

-- name: DeleteStaleAtprotoInbound :execrows
-- Retention sweep for the audit ledger — the table is pure audit (nothing
-- reads it back for correctness), so rows past the configured window go.
DELETE FROM atproto_inbound WHERE created_at < now() - make_interval(days => @days);

-- name: InsertAtprotoInbound :one
-- Audit ledger insert doubles as the DB-level dedupe: the (event_uri,
-- operation, event_seq) unique key rejects replays (replays carry the same
-- upstream time_us) while still letting a legit create→delete→create
-- sequence through, so an ErrNoRows return means the event was already
-- recorded. status starts at 'received' and transitions to
-- processed/ignored/rejected/error after dispatch.
INSERT INTO atproto_inbound (event_uri, event_seq, did, collection, operation, payload)
VALUES (@event_uri, @event_seq, @did, @collection, @operation, @payload::jsonb)
ON CONFLICT (event_uri, operation, event_seq)
-- Errored rows are retryable: replay resets them to 'received' so dispatch
-- runs again (all normalizers are idempotent). processed/ignored/rejected
-- rows stay terminal.
DO UPDATE SET status = 'received', error = NULL
WHERE atproto_inbound.status = 'error'
RETURNING id;

-- name: UpdateAtprotoInboundStatus :exec
UPDATE atproto_inbound SET status = @status, error = @error WHERE id = @id;

-- name: UpsertAtprotoRemoteFollow :one
-- follow_uri is the record's ATURI — unique per remote repo, so a replayed
-- create is a no-op and a later create after an undo revives the row.
INSERT INTO atproto_remote_follows (account_id, remote_actor_id, follow_uri)
VALUES (@account_id, @remote_actor_id, @follow_uri)
ON CONFLICT (follow_uri) DO UPDATE SET undone_at = NULL
RETURNING *;

-- name: GetAtprotoRemoteFollowByURI :one
-- Replay guard: a create replayed with a fresh upstream seq still carries the
-- same record URI — if the row is already active the event is a no-op and no
-- second notification fires.
SELECT id, undone_at FROM atproto_remote_follows WHERE follow_uri = @follow_uri;

-- name: UndoAtprotoRemoteFollow :execrows
UPDATE atproto_remote_follows SET undone_at = now() WHERE follow_uri = @follow_uri AND undone_at IS NULL;

-- name: UpsertAtprotoRemoteLike :one
INSERT INTO atproto_remote_likes (account_id, remote_actor_id, like_uri, content_id)
VALUES (@account_id, @remote_actor_id, @like_uri, @content_id)
ON CONFLICT (like_uri) DO UPDATE SET undone_at = NULL
RETURNING *;

-- name: GetAtprotoRemoteLikeByURI :one
SELECT id, undone_at FROM atproto_remote_likes WHERE like_uri = @like_uri;

-- name: UndoAtprotoRemoteLike :execrows
UPDATE atproto_remote_likes SET undone_at = now() WHERE like_uri = @like_uri AND undone_at IS NULL;

-- name: InsertAtprotoRemoteComment :one
-- Remote reply: author_user_id stays NULL; provenance lives on
-- remote_atproto_actor_id + remote_uri. Depth mirrors the local model —
-- 0 top-level under the content, parent.depth + 1 when nested.
INSERT INTO comments (content_id, author_user_id, parent_id, body, depth, is_anonymous,
                      remote_atproto_actor_id, remote_uri, status)
VALUES (@content_id, NULL, sqlc.narg(parent_id), @body, @depth, false,
        @remote_atproto_actor_id, @remote_uri, 'published')
ON CONFLICT (remote_uri) WHERE remote_uri IS NOT NULL DO NOTHING
RETURNING *;

-- name: GetAtprotoCommentByRemoteURI :one
-- Parent resolution carries content_id + parent_id + depth so nested remote
-- replies land under the right comment on the right content with
-- depth = parent.depth + 1, and the delete path can unwind the counters the
-- local create path maintains.
SELECT id, content_id, parent_id, depth, deleted_at FROM comments WHERE remote_uri = @remote_uri;

-- name: UpdateAtprotoRemoteCommentBody :execrows
-- Remote edit ops carry the full updated record — refresh the local copy
-- (is_edited mirrors the local edit-path marker).
UPDATE comments SET body = @body, is_edited = true, edited_at = now(), updated_at = now()
WHERE remote_uri = @remote_uri AND deleted_at IS NULL;

-- name: SoftDeleteAtprotoCommentByRemoteURI :execrows
-- Remote delete ops tombstone the local row the same way member deletes do —
-- is_pinned clears too, mirroring SoftDeleteComment.
UPDATE comments SET deleted_at = now(), is_pinned = false, updated_at = now()
WHERE remote_uri = @remote_uri AND deleted_at IS NULL;

-- name: GetAtprotoAccountByDID :one
-- Follow subjects resolve by DID string (at:// URIs embed the member's did).
SELECT id, user_id, did, handle, status FROM atproto_accounts WHERE did = @did AND status = 'active';
