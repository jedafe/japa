-- Ref: japaconnect-engineering/sections/127-atproto-native.md (A5 — Jetstream inbound)

-- name: UpsertAtprotoRemoteActor :one
-- Remote actors are did-keyed, never users rows. Profile fields refresh on
-- every touch so a later event sees the actor's latest advertised handle/name.
INSERT INTO atproto_remote_actors (did, handle, display_name, avatar_url, fetched_at)
VALUES (@did, @handle, @display_name, @avatar_url, now())
ON CONFLICT (did) DO UPDATE SET
    handle       = COALESCE(EXCLUDED.handle, atproto_remote_actors.handle),
    display_name = COALESCE(EXCLUDED.display_name, atproto_remote_actors.display_name),
    avatar_url   = COALESCE(EXCLUDED.avatar_url, atproto_remote_actors.avatar_url),
    fetched_at   = now(),
    updated_at   = now()
RETURNING *;

-- name: GetAtprotoRemoteActorByDID :one
SELECT * FROM atproto_remote_actors WHERE did = @did;

-- name: InsertAtprotoInbound :one
-- Audit ledger insert doubles as the DB-level dedupe: the (event_uri,
-- operation, event_seq) unique key rejects replays (replays carry the same
-- upstream time_us) while still letting a legit create→delete→create
-- sequence through, so an ErrNoRows return means the event was already
-- recorded. status starts at 'received' and transitions to
-- processed/ignored/rejected/error after dispatch.
INSERT INTO atproto_inbound (event_uri, event_seq, did, collection, operation, payload)
VALUES (@event_uri, @event_seq, @did, @collection, @operation, @payload::jsonb)
ON CONFLICT (event_uri, operation, event_seq) DO NOTHING
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

-- name: UndoAtprotoRemoteFollow :execrows
UPDATE atproto_remote_follows SET undone_at = now() WHERE follow_uri = @follow_uri AND undone_at IS NULL;

-- name: UpsertAtprotoRemoteLike :one
INSERT INTO atproto_remote_likes (account_id, remote_actor_id, like_uri, content_id)
VALUES (@account_id, @remote_actor_id, @like_uri, @content_id)
ON CONFLICT (like_uri) DO UPDATE SET undone_at = NULL
RETURNING *;

-- name: UndoAtprotoRemoteLike :execrows
UPDATE atproto_remote_likes SET undone_at = now() WHERE like_uri = @like_uri AND undone_at IS NULL;

-- name: InsertAtprotoRemoteComment :one
-- Remote reply: author_user_id stays NULL; provenance lives on
-- remote_atproto_actor_id + remote_uri. depth mirrors the local reply model —
-- a reply to a member's top-level post is depth 0 under the content.
INSERT INTO comments (content_id, author_user_id, parent_id, body, depth, is_anonymous,
                      remote_atproto_actor_id, remote_uri, status)
VALUES (@content_id, NULL, sqlc.narg(parent_id), @body, 0, false,
        @remote_atproto_actor_id, @remote_uri, 'published')
ON CONFLICT (remote_uri) WHERE remote_uri IS NOT NULL DO NOTHING
RETURNING *;

-- name: GetAtprotoCommentByRemoteURI :one
SELECT id, content_id, deleted_at FROM comments WHERE remote_uri = @remote_uri;

-- name: SoftDeleteAtprotoCommentByRemoteURI :execrows
-- Remote delete ops tombstone the local row the same way member deletes do.
UPDATE comments SET deleted_at = now()
WHERE remote_uri = @remote_uri AND deleted_at IS NULL;

-- name: GetAtprotoAccountByDID :one
-- Follow subjects resolve by DID string (at:// URIs embed the member's did).
SELECT id, user_id, did, handle, status FROM atproto_accounts WHERE did = @did AND status = 'active';
