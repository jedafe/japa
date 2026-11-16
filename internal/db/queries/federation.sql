-- Section 126 (ADR-138) — ActivityPub Federation.
-- Local actor management, remote actor cache, remote→local follows,
-- outbound delivery ledger, inbound dedup/audit, instance blocks.

-- name: GetFederatedActorByUsername :one
-- Actor lookup for WebFinger + actor-document fetches. acct localparts are
-- case-insensitive (usernames may be mixed-case), so preferred_username
-- compares under lower(); the live-username arm stays exact to keep using
-- its index. When a rename leaves a stale preferred_username, the live
-- u.username match wins (same precedence as GetUserByUsername) and the
-- superseded actor can never shadow a live handle. Suspended/deleted users
-- do not federate.
SELECT fa.*
FROM federated_actors fa
JOIN users u ON u.id = fa.user_id
WHERE (lower(fa.preferred_username) = lower(@username) OR u.username = @username)
  AND fa.status = 'active'
  AND u.status = 'active'
ORDER BY (u.username = @username) DESC, fa.created_at DESC
LIMIT 1;

-- name: GetFederatedActorByUserID :one
-- Member-facing + object-attribution lookup. Suspended/deleted users are
-- treated as un-federated so their objects stop serving.
SELECT fa.*
FROM federated_actors fa
JOIN users u ON u.id = fa.user_id
WHERE fa.user_id = @user_id
  AND u.status = 'active';

-- name: GetActorProfileByUsername :one
-- Actor-document source row: federated actor + public profile fields
-- (no PII) in one join. manually_approves_followers derives live from the
-- member's profile_visibility ('followers' → true) so privacy changes
-- apply without touching the actor row.
SELECT fa.id AS actor_id, fa.user_id, fa.actor_url, fa.public_key_pem, fa.public_key_id,
       fa.inbox_url, fa.outbox_url, fa.followers_url, fa.following_url,
       COALESCE(ups.profile_visibility = 'followers', false)::boolean AS manually_approves_followers,
       COALESCE(ups.search_indexable, true) AS discoverable,
       fa.status, fa.created_at AS actor_created_at,
       u.username, u.created_at AS user_created_at,
       up.display_name, up.bio, up.avatar_url, up.cover_url, up.website_url
FROM federated_actors fa
JOIN users u ON u.id = fa.user_id
LEFT JOIN user_profiles up ON up.user_id = u.id
LEFT JOIN user_privacy_settings ups ON ups.user_id = u.id
WHERE (lower(fa.preferred_username) = lower(@username) OR u.username = @username)
  AND fa.status = 'active'
  AND u.status = 'active'
ORDER BY (u.username = @username) DESC, fa.created_at DESC
LIMIT 1;

-- name: GetFederatedActorByUsernameAnyStatus :one
-- Tombstone/inbox lookup: the actor row must resolve regardless of
-- fa.status so disabled/tombstone actors answer 410 rather than the
-- status-gated lookup's blind 404. u.status stays gated — a suspended or
-- deleted member's actor is hidden, not gone.
SELECT fa.*
FROM federated_actors fa
JOIN users u ON u.id = fa.user_id
WHERE (lower(fa.preferred_username) = lower(@username) OR u.username = @username)
  AND u.status = 'active'
ORDER BY (u.username = @username) DESC, fa.created_at DESC
LIMIT 1;

-- name: GetFederatedActorByID :one
SELECT * FROM federated_actors WHERE id = @id;

-- name: GetFederatedActorHandlesByUserID :one
-- Cache-invalidation lookup: both handles an actor's docs may be cached
-- under (creation-time preferred_username + live u.username), resolved
-- WITHOUT status gates — lifecycle sync needs them precisely when the
-- member is no longer active, where the status-joined lookups go blind.
SELECT fa.preferred_username, u.username
FROM federated_actors fa
JOIN users u ON u.id = fa.user_id
WHERE fa.user_id = @user_id;

-- name: CreateFederatedActor :one
INSERT INTO federated_actors (
    user_id, actor_url, preferred_username, public_key_pem, private_key_enc,
    public_key_id, inbox_url, outbox_url, followers_url, following_url,
    manually_approves_followers
) VALUES (
    @user_id, @actor_url, @preferred_username, @public_key_pem, @private_key_enc,
    @public_key_id, @inbox_url, @outbox_url, @followers_url, @following_url,
    @manually_approves_followers
)
-- Only a disabled actor re-activates on conflict — a tombstone (M2 erasure)
-- row is terminal and must never be resurrected. The WHERE makes RETURNING
-- emit no row in that case (pgx.ErrNoRows → ErrNotFound in the service).
ON CONFLICT (user_id) DO UPDATE SET
    status = 'active',
    -- Reactivation refreshes the follow-approval snapshot — the member's
    -- profile_visibility may have changed while the actor was disabled.
    manually_approves_followers = EXCLUDED.manually_approves_followers,
    updated_at = now()
WHERE federated_actors.status = 'disabled'
RETURNING *;

-- name: UpdateFederatedActorStatus :exec
UPDATE federated_actors SET status = @status, updated_at = now() WHERE id = @id;

-- name: UpdateFederatedActorStatusFrom :execrows
-- Conditional transition — the status must still equal @expected at write
-- time. Rows-affected 0 means a concurrent transition won (e.g. an M2
-- erasure tombstoned between our read and this write): the service
-- re-reads and dispatches on the winner's status rather than clobbering it.
UPDATE federated_actors SET status = @status, updated_at = now()
WHERE id = @id AND status = @expected;

-- name: RotateFederatedActorKey :one
-- M3a: swap the actor's signing material in place. The status predicate
-- confines rotation to live actors — a disabled row's key churn would
-- serve nothing, and a tombstone must stay terminal even when a rotate
-- races an erasure write.
UPDATE federated_actors SET
    public_key_pem = @public_key_pem,
    private_key_enc = @private_key_enc,
    public_key_id = @public_key_id,
    key_rotated_at = now(),
    updated_at = now()
WHERE id = @id AND status = 'active'
RETURNING *;

-- name: ListFederatedActors :many
-- Admin DataTable: ?q= matches username/actor_url; page/limit/sort/dir contract
-- (sort whitelisted handler-side to created_at|username|status|actor_url).
-- u.email deliberately excluded — actor management needs the handle, not
-- PII (admins who need email use the users surface).
SELECT fa.*, u.username, up.display_name, up.avatar_url
FROM federated_actors fa
JOIN users u ON u.id = fa.user_id
LEFT JOIN user_profiles up ON up.user_id = u.id
WHERE (@q::text = '' OR u.username ILIKE '%' || @q || '%' OR fa.actor_url ILIKE '%' || @q || '%')
ORDER BY
  CASE WHEN @sort_dir = 'asc' THEN CASE @sort_key
    WHEN 'username' THEN u.username WHEN 'status' THEN fa.status WHEN 'actor_url' THEN fa.actor_url END
  END ASC NULLS LAST,
  CASE WHEN @sort_dir = 'desc' THEN CASE @sort_key
    WHEN 'username' THEN u.username WHEN 'status' THEN fa.status WHEN 'actor_url' THEN fa.actor_url END
  END DESC NULLS LAST,
  CASE WHEN @sort_key = 'created_at' AND @sort_dir = 'asc' THEN fa.created_at END ASC NULLS LAST,
  fa.created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: CountFederatedActors :one
SELECT COUNT(*)::bigint
FROM federated_actors fa
JOIN users u ON u.id = fa.user_id
WHERE (@q::text = '' OR u.username ILIKE '%' || @q || '%' OR fa.actor_url ILIKE '%' || @q || '%');

-- name: GetNodeInfoUsage :one
-- NodeInfo 2.1 usage counters (active users, published contents, comments).
SELECT
    (SELECT COUNT(*) FROM users WHERE status = 'active' AND is_synthetic = false)::bigint AS total_users,
    (SELECT COUNT(*) FROM contents WHERE status = 'published' AND visibility = 'public' AND deleted_at IS NULL)::bigint AS local_posts,
    (SELECT COUNT(*) FROM comments WHERE deleted_at IS NULL)::bigint AS local_comments;

-- name: GetFederationStats :one
SELECT
    (SELECT COUNT(*) FROM federated_actors WHERE status = 'active')::bigint AS local_actors,
    (SELECT COUNT(*) FROM remote_actors WHERE status = 'active')::bigint AS remote_actors,
    (SELECT COUNT(*) FROM federation_follows WHERE status = 'accepted')::bigint AS remote_follows,
    (SELECT COUNT(*) FROM federation_deliveries WHERE status = 'pending')::bigint AS deliveries_pending,
    (SELECT COUNT(*) FROM federation_deliveries WHERE status = 'delivered')::bigint AS deliveries_delivered,
    (SELECT COUNT(*) FROM federation_deliveries WHERE status = 'dead')::bigint AS deliveries_dead,
    (SELECT COUNT(*) FROM federation_instance_blocks)::bigint AS instance_blocks;

-- name: UpsertRemoteActor :one
INSERT INTO remote_actors (
    actor_url, acct, domain, inbox_url, shared_inbox_url, outbox_url,
    public_key_pem, public_key_id, preferred_username, display_name,
    avatar_url, fetched_at
) VALUES (
    @actor_url, @acct, @domain, @inbox_url, @shared_inbox_url, @outbox_url,
    @public_key_pem, @public_key_id, @preferred_username, @display_name,
    @avatar_url, now()
)
ON CONFLICT (actor_url) DO UPDATE SET
    acct = EXCLUDED.acct,
    domain = EXCLUDED.domain,
    inbox_url = EXCLUDED.inbox_url,
    shared_inbox_url = EXCLUDED.shared_inbox_url,
    outbox_url = EXCLUDED.outbox_url,
    public_key_pem = EXCLUDED.public_key_pem,
    public_key_id = EXCLUDED.public_key_id,
    preferred_username = EXCLUDED.preferred_username,
    display_name = EXCLUDED.display_name,
    avatar_url = EXCLUDED.avatar_url,
    fetched_at = now(),
    updated_at = now()
RETURNING *;

-- name: GetRemoteActorByURL :one
SELECT * FROM remote_actors WHERE actor_url = @actor_url;

-- name: GetRemoteActorByID :one
SELECT * FROM remote_actors WHERE id = @id;

-- name: GetRemoteActorByKeyID :one
-- Inbound signature verification resolves the signer by the Signature
-- header's keyId (typically {actor_url}#main-key but not guaranteed).
SELECT * FROM remote_actors WHERE public_key_id = @key_id AND status = 'active';

-- name: SetRemoteActorStatus :exec
-- Delete{actor} tombstones the cached identity — follows/comments/likes
-- unwind in the same dispatch.
UPDATE remote_actors SET status = @status, updated_at = now() WHERE id = @id;

-- name: UpsertFederationFollow :one
INSERT INTO federation_follows (local_actor_id, remote_actor_id, ap_activity_id, status)
VALUES (@local_actor_id, @remote_actor_id, @ap_activity_id, @status)
ON CONFLICT (local_actor_id, remote_actor_id) DO UPDATE SET
    ap_activity_id = COALESCE(EXCLUDED.ap_activity_id, federation_follows.ap_activity_id),
    -- Sticky states: 'accepted' survives any resend (the edge exists —
    -- resends are re-affirmations). 'undone' only survives a REPLAY of
    -- the same Follow (same or absent ap_activity_id): an Undo followed
    -- by a NEW Follow activity id is a genuine re-request and must
    -- resurrect — otherwise an unfollow permanently bans the actor.
    status = CASE WHEN federation_follows.status = 'accepted' THEN 'accepted'
                  WHEN federation_follows.status = 'undone'
                       AND (EXCLUDED.ap_activity_id IS NULL
                            OR EXCLUDED.ap_activity_id = federation_follows.ap_activity_id)
                       THEN 'undone'
                  ELSE EXCLUDED.status END,
    updated_at = now()
RETURNING *;

-- name: GetFederationFollowByActivityID :one
SELECT * FROM federation_follows WHERE ap_activity_id = @ap_activity_id;

-- name: GetFederationFollowByPair :one
-- Undo{Follow} may embed the Follow object without its @id — the
-- (local, remote) pair is the fallback match.
SELECT * FROM federation_follows
WHERE local_actor_id = @local_actor_id AND remote_actor_id = @remote_actor_id;

-- name: ListPendingFederationFollowsByUser :many
-- Member follow-request surface (§126): pending remote Follows for the
-- caller's actor, with the remote actor's display fields for rendering.
SELECT ff.id, ff.remote_actor_id, ff.ap_activity_id, ff.status, ff.created_at,
       ra.actor_url, ra.acct, ra.display_name, ra.avatar_url
FROM federation_follows ff
JOIN federated_actors fa ON fa.id = ff.local_actor_id
JOIN remote_actors ra ON ra.id = ff.remote_actor_id
WHERE fa.user_id = @user_id AND fa.status = 'active' AND ff.status = 'pending'
ORDER BY ff.created_at DESC;

-- name: ResolveFederationFollow :execrows
-- Member Accept/Reject of a pending request — the status='pending'
-- predicate makes the transition conditional: a resolved request can't
-- be re-answered (idempotent under double-clicks and replays).
UPDATE federation_follows SET status = @status, updated_at = now()
WHERE local_actor_id = @local_actor_id AND remote_actor_id = @remote_actor_id
  AND status = 'pending';

-- name: UpdateFederationFollowStatus :exec
UPDATE federation_follows SET status = @status, updated_at = now()
WHERE local_actor_id = @local_actor_id AND remote_actor_id = @remote_actor_id;

-- name: UndoFederationFollowsByRemoteActor :exec
-- Remote-actor tombstone propagation: every live/pending follow undoes
-- at once; 'undone' rows stay terminal so replays are idempotent.
UPDATE federation_follows SET status = 'undone', updated_at = now()
WHERE remote_actor_id = @remote_actor_id AND status <> 'undone';

-- name: RepointFederationFollowsToRemoteActor :execrows
-- §126 M4 Move{actor→target}: every edge that does not already exist on
-- the new actor rebinds to it. Colliding rows (the member was followed
-- from BOTH accounts pre-move) stay on the old id — the caller undoes
-- them via UndoFederationFollowsByRemoteActor so the new-actor edge wins.
UPDATE federation_follows ff SET remote_actor_id = @new_remote_actor_id, updated_at = now()
WHERE ff.remote_actor_id = @old_remote_actor_id
  AND NOT EXISTS (SELECT 1 FROM federation_follows dup
                  WHERE dup.local_actor_id = ff.local_actor_id
                    AND dup.remote_actor_id = @new_remote_actor_id);

-- name: RepointRemoteFollowsByActor :execrows
-- follows provenance mirror of the repoint above — same collision rule
-- on the partial-unique (follower_remote_actor_id, entity_type,
-- entity_id) index; stragglers delete via DeleteDuplicateRemoteFollowsByActor.
UPDATE follows f SET follower_remote_actor_id = @new_remote_actor_id
WHERE f.follower_remote_actor_id = @old_remote_actor_id
  AND NOT EXISTS (SELECT 1 FROM follows dup
                  WHERE dup.follower_remote_actor_id = @new_remote_actor_id
                    AND dup.entity_type = f.entity_type
                    AND dup.entity_id = f.entity_id);

-- name: DeleteDuplicateRemoteFollowsByActor :many
-- Mirror repoint cleanup: rows left after RepointRemoteFollowsByActor
-- are duplicate edges the new actor already holds — the accounts merged
-- into one follower, so each deleted row unwinds follower_count by one
-- (RETURNING entity_id feeds the AdjustUserFollowerCount loop, same as
-- DeleteRemoteFollowsByActor).
DELETE FROM follows WHERE follower_remote_actor_id = @old_remote_actor_id
RETURNING entity_id;

-- name: ListRemoteFollowerInboxesByActor :many
-- Fanout expansion (§126): one delivery row per DISTINCT inbox —
-- sharedInbox coalescing collapses same-instance followers into a
-- single POST. remote_actor_id is a representative for ledger
-- attribution (ON DELETE SET NULL, so the row survives actor churn).
SELECT COALESCE(ra.shared_inbox_url, ra.inbox_url) AS inbox_url,
       MIN(ra.id::text)::uuid AS remote_actor_id
FROM federation_follows ff
JOIN remote_actors ra ON ra.id = ff.remote_actor_id
WHERE ff.local_actor_id = @local_actor_id
  AND ff.status = 'accepted'
  AND ra.status = 'active'
  AND COALESCE(ra.shared_inbox_url, ra.inbox_url) IS NOT NULL
GROUP BY 1;

-- name: ListRemoteFollowerURLsByActor :many
-- Followers collection items: accepted remote follows for this local actor.
SELECT ra.actor_url
FROM federation_follows ff
JOIN remote_actors ra ON ra.id = ff.remote_actor_id
WHERE ff.local_actor_id = @local_actor_id
  AND ff.status = 'accepted'
  AND ra.status = 'active'
ORDER BY ff.created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: CountRemoteFollowersByActor :one
SELECT COUNT(*)::bigint
FROM federation_follows ff
JOIN remote_actors ra ON ra.id = ff.remote_actor_id
WHERE ff.local_actor_id = @local_actor_id
  AND ff.status = 'accepted'
  AND ra.status = 'active';

-- name: ListLocalFederatedFollowerURLs :many
-- Local followers of the actor's user who are themselves federated — their
-- actor URLs appear in the AP followers collection too. The users join keeps
-- suspended/deleted followers out of the collection (their docs 404 anyway).
SELECT fa.actor_url
FROM follows f
JOIN federated_actors fa ON fa.user_id = f.follower_id AND fa.status = 'active'
JOIN users fu ON fu.id = fa.user_id AND fu.status = 'active'
WHERE f.entity_type = 'user' AND f.entity_id = @user_id
ORDER BY f.created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: CountLocalFederatedFollowers :one
SELECT COUNT(*)::bigint
FROM follows f
JOIN federated_actors fa ON fa.user_id = f.follower_id AND fa.status = 'active'
JOIN users fu ON fu.id = fa.user_id AND fu.status = 'active'
WHERE f.entity_type = 'user' AND f.entity_id = @user_id;

-- name: ListLocalFederatedFollowingURLs :many
-- Following collection items: users the member follows who are federated.
SELECT fa.actor_url
FROM follows f
JOIN federated_actors fa ON fa.user_id = f.entity_id AND fa.status = 'active'
JOIN users fu ON fu.id = fa.user_id AND fu.status = 'active'
WHERE f.follower_id = @user_id AND f.entity_type = 'user'
ORDER BY f.created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: CountLocalFederatedFollowing :one
SELECT COUNT(*)::bigint
FROM follows f
JOIN federated_actors fa ON fa.user_id = f.entity_id AND fa.status = 'active'
JOIN users fu ON fu.id = fa.user_id AND fu.status = 'active'
WHERE f.follower_id = @user_id AND f.entity_type = 'user';

-- name: ListFederatableContentByAuthor :many
-- Outbox source rows (§126): published, public, non-deleted contents on the
-- federatable type allowlist. Media joins resolve the featured/header image
-- for the AP attachment (spec: featured/header image as Image attachment).
SELECT c.id, c.content_type, c.title, c.slug, c.body_html, c.canonical_url,
       c.featured_image_id, c.header_image_id, c.published_at, c.updated_at, c.created_at,
       mf.storage_bucket AS featured_bucket, mf.storage_path AS featured_path, mf.mime_type AS featured_mime,
       mh.storage_bucket AS header_bucket, mh.storage_path AS header_path, mh.mime_type AS header_mime
FROM contents c
LEFT JOIN media mf ON mf.id = c.featured_image_id AND mf.deleted_at IS NULL AND mf.confirmed = true
LEFT JOIN media mh ON mh.id = c.header_image_id AND mh.deleted_at IS NULL AND mh.confirmed = true
WHERE c.author_id = @author_id
  AND c.status = 'published'
  AND c.visibility = 'public'
  AND c.deleted_at IS NULL
  AND c.federate_opt_out = false
  AND c.content_type = ANY(@content_types::text[])
ORDER BY c.published_at DESC NULLS LAST, c.created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: CountFederatableContentByAuthor :one
SELECT COUNT(*)::bigint
FROM contents c
WHERE c.author_id = @author_id
  AND c.status = 'published'
  AND c.visibility = 'public'
  AND c.deleted_at IS NULL
  AND c.federate_opt_out = false
  AND c.content_type = ANY(@content_types::text[]);

-- name: GetFederatableContentByID :one
SELECT c.id, c.author_id, c.content_type, c.title, c.slug, c.body_html,
       c.canonical_url, c.featured_image_id, c.header_image_id, c.space_id,
       c.published_at, c.updated_at, c.created_at,
       mf.storage_bucket AS featured_bucket, mf.storage_path AS featured_path, mf.mime_type AS featured_mime,
       mh.storage_bucket AS header_bucket, mh.storage_path AS header_path, mh.mime_type AS header_mime
FROM contents c
LEFT JOIN media mf ON mf.id = c.featured_image_id AND mf.deleted_at IS NULL AND mf.confirmed = true
LEFT JOIN media mh ON mh.id = c.header_image_id AND mh.deleted_at IS NULL AND mh.confirmed = true
WHERE c.id = @id
  AND c.status = 'published'
  AND c.visibility = 'public'
  AND c.deleted_at IS NULL
  AND c.federate_opt_out = false
  AND c.content_type = ANY(@content_types::text[]);

-- name: GetContentLiteByID :one
-- Ungated contents read for delete propagation and inbound lock checks:
-- soft-deleted and unpublished rows still resolve — the questions the
-- callers answer are "could a remote copy exist" and "are comments
-- locked", not whether the object is currently live.
SELECT author_id, published_at, is_locked, deleted_at, visibility, updated_at FROM contents WHERE id = @id;

-- name: InsertFederationDelivery :one
INSERT INTO federation_deliveries (
    activity_ap_id, activity_type, activity_json, local_actor_id,
    inbox_url, remote_actor_id, next_retry_at
) VALUES (
    @activity_ap_id, @activity_type, @activity_json, @local_actor_id,
    @inbox_url, @remote_actor_id, now()
)
-- (activity_ap_id, inbox_url) unique — fanout replays dedup at the
-- constraint; ErrNoRows means the row (and its queue task) already exists.
ON CONFLICT (activity_ap_id, inbox_url) DO NOTHING
RETURNING *;

-- name: GetFederationDeliveryByID :one
SELECT * FROM federation_deliveries WHERE id = @id;

-- name: ListDueFederationDeliveries :many
-- Stranded-work sweep: 'failed' rows whose ledgered retry time arrived,
-- plus 'pending' rows old enough that their initial enqueue is presumed
-- lost (a fresh pending row's own task is the normal driver — the 5-min
-- floor keeps the sweep from double-arming in-flight work).
SELECT * FROM federation_deliveries
WHERE (status = 'failed' AND next_retry_at IS NOT NULL AND next_retry_at <= now())
   OR (status = 'pending' AND created_at < now() - make_interval(mins => 5))
ORDER BY created_at ASC
LIMIT @page_limit;

-- name: ListUndeliveredFederationDeliveriesByActor :many
-- Erasure flush (§126): every non-terminal delivery an actor owns. The
-- user-row cascade about to burn key custody is why EraseFederation gives
-- these one last synchronous attempt before tombstoning.
SELECT id FROM federation_deliveries
WHERE local_actor_id = @local_actor_id AND status IN ('pending','failed');

-- name: UpdateFederationDeliveryStatus :exec
UPDATE federation_deliveries
SET status = @status,
    attempts = @attempts,
    next_retry_at = @next_retry_at,
    last_error = @last_error,
    delivered_at = @delivered_at
WHERE id = @id;

-- name: InsertFederationInbound :one
-- The partial unique on activity_ap_id is the DB-level dedup backstop
-- behind the Valkey marker — a replay returning ErrNoRows is already
-- ledgered and must not re-dispatch.
INSERT INTO federation_inbound (
    activity_ap_id, activity_type, actor_url, key_id, signature_valid, payload, status
) VALUES (
    @activity_ap_id, @activity_type, @actor_url, @key_id, @signature_valid, @payload,
    COALESCE(sqlc.narg(status), 'received')
)
ON CONFLICT (activity_ap_id) WHERE activity_ap_id IS NOT NULL DO NOTHING
RETURNING *;

-- name: GetFederationInboundByActivityID :one
SELECT * FROM federation_inbound WHERE activity_ap_id = @activity_ap_id;

-- name: GetFederationInboundByID :one
SELECT * FROM federation_inbound WHERE id = @id;

-- name: UpdateFederationInboundStatus :exec
UPDATE federation_inbound SET status = @status, error = @error WHERE id = @id;

-- name: ListStaleFederationInbound :many
-- Same stranded-work class as deliveries: the handler's EnqueueInbound
-- is best-effort — a lost kick leaves 'received' rows with no driver.
SELECT id FROM federation_inbound
WHERE status = 'received' AND created_at < now() - make_interval(mins => 5)
LIMIT @page_limit;

-- name: PruneFederationDeliveries :exec
DELETE FROM federation_deliveries
WHERE status IN ('delivered','dead') AND created_at < now() - make_interval(days => @retention_days);

-- name: PruneFederationInbound :exec
-- Bounded retention for the audit ledger — terminal rows keep raw
-- payloads. 'received' rows past retention are poison payloads the
-- sweep would otherwise re-arm forever; deleting them is the bound.
DELETE FROM federation_inbound
WHERE created_at < now() - make_interval(days => @retention_days);

-- name: CreateInstanceBlock :one
INSERT INTO federation_instance_blocks (domain, reason, blocked_by)
VALUES (@domain, @reason, @blocked_by)
ON CONFLICT (domain) DO UPDATE SET reason = EXCLUDED.reason
RETURNING *;

-- name: DeleteInstanceBlock :exec
DELETE FROM federation_instance_blocks WHERE id = @id;

-- name: ListInstanceBlocks :many
SELECT * FROM federation_instance_blocks
WHERE (@q::text = '' OR domain ILIKE '%' || @q || '%')
ORDER BY created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: CountInstanceBlocks :one
SELECT COUNT(*)::bigint FROM federation_instance_blocks
WHERE (@q::text = '' OR domain ILIKE '%' || @q || '%');

-- name: GetInstanceBlockByDomain :one
-- lower() on the param: callers hand us URL-derived hosts, and Go's
-- url.Hostname() preserves case while CreateBlock stores lowercase.
SELECT * FROM federation_instance_blocks WHERE domain = lower(@domain);

-- =====================================================================
-- M2 — remote provenance on native social tables (migration 001167)
-- =====================================================================

-- name: InsertRemoteFollow :execrows
-- §126 inbound Follow→Accept: the remote follower lands as a native
-- follows row — follower_remote_actor_id carries the provenance
-- (follower_id stays NULL). Rows affected drives the follower-count +
-- notification side effects — a replay is a no-op.
INSERT INTO follows (follower_remote_actor_id, entity_type, entity_id)
VALUES (@remote_actor_id, 'user', @user_id)
ON CONFLICT (follower_remote_actor_id, entity_type, entity_id)
    WHERE follower_remote_actor_id IS NOT NULL DO NOTHING;

-- name: DeleteRemoteFollow :execrows
-- Undo{Follow} removes the native row — rows affected says whether one
-- existed to remove.
DELETE FROM follows
WHERE follower_remote_actor_id = @remote_actor_id
  AND entity_type = 'user' AND entity_id = @user_id;

-- name: DeleteRemoteFollowsByActor :many
-- Delete{actor} propagation: every native follow edge by the actor goes;
-- RETURNING feeds the per-user follower_count unwind.
DELETE FROM follows WHERE follower_remote_actor_id = @remote_actor_id
RETURNING entity_id;

-- name: AdjustUserFollowerCount :exec
-- Remote-provenance follow edges land/undo outside follows.Service —
-- keep the denormalized users.follower_count honest the same way
-- updateFollowerCount does for member edges.
UPDATE users SET follower_count = GREATEST(0, follower_count + @delta) WHERE id = @user_id;

-- name: AddRemoteReaction :execrows
-- §126 inbound Like: a real reactions row with remote provenance — it
-- counts in reaction_count and content_scores like a member's. One
-- reaction per remote actor per entity (partial unique); replays no-op.
INSERT INTO reactions (entity_type, entity_id, remote_actor_id, reaction_key)
VALUES (@entity_type, @entity_id, @remote_actor_id, @reaction_key)
ON CONFLICT (entity_type, entity_id, remote_actor_id)
    WHERE remote_actor_id IS NOT NULL DO NOTHING;

-- name: RemoveRemoteReaction :execrows
-- Undo{Like}: delete only if the row exists — rows affected keeps the
-- recount honest on replays.
DELETE FROM reactions
WHERE entity_type = @entity_type AND entity_id = @entity_id
  AND remote_actor_id = @remote_actor_id AND reaction_key = @reaction_key;

-- name: DeleteRemoteReactionsByActor :many
-- Delete{actor} propagation: all remote reactions go; RETURNING feeds
-- the per-content score refresh.
DELETE FROM reactions WHERE remote_actor_id = @remote_actor_id
RETURNING entity_type, entity_id;

-- name: InsertAPRemoteComment :one
-- Remote AP reply (§126 Create{inReplyTo}): author_user_id stays NULL —
-- provenance lives on remote_ap_actor_id + remote_uri (shared dedupe
-- column with the atproto bridge). Depth mirrors the local model.
INSERT INTO comments (content_id, author_user_id, parent_id, body, depth, is_anonymous,
                      remote_ap_actor_id, remote_uri, status)
VALUES (@content_id, NULL, sqlc.narg(parent_id), @body, @depth, false,
        @remote_ap_actor_id, @remote_uri, 'published')
ON CONFLICT (remote_uri) WHERE remote_uri IS NOT NULL DO NOTHING
RETURNING *;

-- name: SoftDeleteAPCommentsByActor :many
-- Delete{actor} propagation: tombstone every live remote comment by the
-- actor; RETURNING feeds the counter unwind (same shape as §127 A6-1).
UPDATE comments SET deleted_at = now(), is_pinned = false, updated_at = now()
WHERE remote_ap_actor_id = @remote_ap_actor_id AND deleted_at IS NULL
RETURNING id, content_id, parent_id;

-- name: IncrementContentShareCount :exec
-- §126 inbound Announce.
UPDATE contents SET share_count = share_count + 1 WHERE id = @id;

-- name: DecrementContentShareCount :exec
-- Undo{Announce} — clamped so a stray/replayed undo can't drive the
-- count negative.
UPDATE contents SET share_count = GREATEST(0, share_count - 1) WHERE id = @id;

-- name: InsertRemoteShare :execrows
-- Announce: row lands only once per (remote_actor, content) — rows
-- affected is the ++ gate so replays can't double-count.
INSERT INTO federation_remote_shares (remote_actor_id, content_id, ap_activity_id)
VALUES (@remote_actor_id, @content_id, @ap_activity_id)
ON CONFLICT (remote_actor_id, content_id) DO NOTHING;

-- name: DeleteRemoteShare :execrows
-- Undo{Announce}: decrement only when a live row actually went away.
DELETE FROM federation_remote_shares
WHERE remote_actor_id = @remote_actor_id AND content_id = @content_id;

-- name: DeleteRemoteSharesByActor :many
-- Delete{actor} propagation: every tracked share goes; RETURNING feeds
-- the per-content decrement.
DELETE FROM federation_remote_shares WHERE remote_actor_id = @remote_actor_id
RETURNING content_id;

-- name: GetLatestDeliveredObjectType :one
-- Fanout's Create-vs-Update decision, keyed on the latest DELIVERED
-- activity for this object: a row that never arrived proves nothing
-- (pending/failed/dead inboxes ignored an Update anyway), and a last-seen
-- 'Delete' means remotes hold a tombstone — only a fresh Create
-- resurrects it. 'Create'/'Update' last-seen → Update; no row → Create.
SELECT activity_type FROM federation_deliveries
WHERE local_actor_id = @local_actor_id
  AND activity_json->'object'->>'id' = @object_id::text
  AND status = 'delivered'
ORDER BY created_at DESC, id DESC
LIMIT 1;

-- name: DeadenUndeliveredNonDeleteDeliveriesByActor :exec
-- Actor tombstone fanout: once Delete{actor} is the story, every other
-- in-flight row for the actor is a hazard — a retried Update/Create
-- landing post-Delete resurrects state the remote just tore down, and a
-- member who disabled shouldn't emit further content traffic at all.
-- Delete rows stay live — a repeat tombstone converges harmlessly.
UPDATE federation_deliveries SET status = 'dead'
WHERE local_actor_id = @local_actor_id
  AND status IN ('pending','failed')
  AND activity_type <> 'Delete';

-- name: GetDeliveredObjectActor :one
-- Tombstone fanout on eligibility loss: which local actor delivered this
-- object before — the Delete must come from that actor's key.
SELECT local_actor_id FROM federation_deliveries
WHERE activity_type IN ('Create','Update')
  AND activity_json->'object'->>'id' = @object_id::text
LIMIT 1;

-- ---------------------------------------------------------------------------
-- M3 — discovery & reach: non-person actors, relays, groups, hashtags
-- ---------------------------------------------------------------------------

-- name: GetFederatedActorByKind :one
-- Singleton-style lookup for the instance Application actor (and any
-- future singleton kinds). Picks the oldest row defensively — the partial
-- unique index already guarantees at most one 'application' row.
SELECT * FROM federated_actors WHERE kind = @kind ORDER BY created_at ASC LIMIT 1;

-- name: GetFederatedActorBySpaceID :one
-- Group-fanout lookup: content.space_id → the space's Group actor row.
SELECT * FROM federated_actors WHERE kind = 'group' AND space_id = @space_id;

-- name: CreateFederatedActorOfKind :one
-- Non-person actor insert (kind application|group, NULL user_id, optional
-- space_id). ON CONFLICT DO NOTHING keeps concurrent minters honest —
-- a lost race returns ErrNoRows and the caller re-reads the winner.
INSERT INTO federated_actors (
    user_id, actor_url, preferred_username, public_key_pem, private_key_enc,
    public_key_id, inbox_url, outbox_url, followers_url, following_url,
    manually_approves_followers, kind, space_id
) VALUES (
    @user_id, @actor_url, @preferred_username, @public_key_pem, @private_key_enc,
    @public_key_id, @inbox_url, @outbox_url, @followers_url, @following_url,
    @manually_approves_followers, @kind, @space_id
)
ON CONFLICT DO NOTHING
RETURNING *;

-- name: GetGroupActorDocBySlug :one
-- Group-document source row: the actor joined to its space (with logo
-- media). Space gates apply in the service — this row resolves even when
-- the space flipped private so callers distinguish 404 from 410.
-- Keyed on fa.preferred_username (the immutable !{slug} handle baked
-- into actor_url at provision), NOT s.slug — a space slug rename must
-- never orphan the canonical actor URL, its WebFinger handle, or the
-- federation_follows edges keyed on it. idx_federated_actors_-
-- preferred_username (lower()) serves the lookup.
SELECT fa.*, s.slug AS space_slug, s.name AS space_name, s.description AS space_description,
       s.visibility AS space_visibility, s.status AS space_status, s.deleted_at AS space_deleted_at,
       s.level AS space_level, s.creator_user_id AS space_creator_user_id,
       ml.storage_bucket AS logo_bucket, ml.storage_path AS logo_path, ml.mime_type AS logo_mime
FROM federated_actors fa
JOIN spaces s ON s.id = fa.space_id
LEFT JOIN media ml ON ml.id = s.logo_media_id AND ml.deleted_at IS NULL AND ml.confirmed = true
WHERE fa.kind = 'group' AND lower(fa.preferred_username) = lower(@slug);

-- name: GetSpaceForGroupActor :one
-- Provisioning gate: only a live, public, MEMBER-CREATED space (§05
-- member_created predicate — custom level with a live creator) may mint
-- a Group actor. Platform/geo spaces never federate as Groups.
SELECT s.id, s.slug, s.name FROM spaces s
WHERE s.id = @id AND s.status = 'active' AND s.visibility = 'public' AND s.deleted_at IS NULL
  AND s.level = 'custom' AND s.creator_user_id IS NOT NULL;

-- name: ListGroupActors :many
SELECT fa.*, s.slug AS space_slug, s.name AS space_name
FROM federated_actors fa
JOIN spaces s ON s.id = fa.space_id
WHERE fa.kind = 'group'
  AND (@q::text = '' OR s.slug ILIKE '%' || @q || '%' OR s.name ILIKE '%' || @q || '%'
       OR fa.actor_url ILIKE '%' || @q || '%')
ORDER BY fa.created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: CountGroupActors :one
SELECT COUNT(*)::bigint
FROM federated_actors fa
JOIN spaces s ON s.id = fa.space_id
WHERE fa.kind = 'group'
  AND (@q::text = '' OR s.slug ILIKE '%' || @q || '%' OR s.name ILIKE '%' || @q || '%'
       OR fa.actor_url ILIKE '%' || @q || '%');

-- name: ListFederatableContentBySpace :many
-- Group outbox items: the space's live, public, federatable content —
-- same gate set as the member outbox, keyed on contents.space_id.
SELECT c.id, c.author_id, c.content_type, c.title, c.slug, c.body_html,
       c.canonical_url, c.published_at, c.created_at
FROM contents c
-- A listed item must resolve on fetch — the object doc 404s unless the
-- author's Person actor exists and is active, so non-federated authors
-- (and disabled actors) never appear in the collection.
JOIN federated_actors fa ON fa.user_id = c.author_id AND fa.status = 'active'
WHERE c.space_id = @space_id
  AND c.status = 'published'
  AND c.visibility = 'public'
  AND c.deleted_at IS NULL
  AND c.federate_opt_out = false
  AND c.content_type = ANY(@content_types::text[])
ORDER BY c.published_at DESC NULLS LAST, c.created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: CountFederatableContentBySpace :one
SELECT COUNT(*)::bigint
FROM contents c
JOIN federated_actors fa ON fa.user_id = c.author_id AND fa.status = 'active'
WHERE c.space_id = @space_id
  AND c.status = 'published'
  AND c.visibility = 'public'
  AND c.deleted_at IS NULL
  AND c.federate_opt_out = false
  AND c.content_type = ANY(@content_types::text[]);

-- name: ListAllFederatableContent :many
-- Instance outbox: every live public federatable object — the crawlable
-- instance-level collection.
SELECT c.id, c.author_id, c.content_type, c.title, c.slug, c.body_html,
       c.canonical_url, c.published_at, c.created_at
FROM contents c
JOIN federated_actors fa ON fa.user_id = c.author_id AND fa.status = 'active'
WHERE c.status = 'published'
  AND c.visibility = 'public'
  AND c.deleted_at IS NULL
  AND c.federate_opt_out = false
  AND c.content_type = ANY(@content_types::text[])
ORDER BY c.published_at DESC NULLS LAST, c.created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: CountAllFederatableContent :one
SELECT COUNT(*)::bigint
FROM contents c
JOIN federated_actors fa ON fa.user_id = c.author_id AND fa.status = 'active'
WHERE c.status = 'published'
  AND c.visibility = 'public'
  AND c.deleted_at IS NULL
  AND c.federate_opt_out = false
  AND c.content_type = ANY(@content_types::text[]);

-- name: ListContentHashtags :many
-- tag[] emission source for objectDocument — dedup keeps a body that
-- links the same tag twice from emitting duplicate entries.
SELECT DISTINCT t.name, t.slug
FROM content_tags ct
JOIN tags t ON t.id = ct.tag_id
WHERE ct.content_id = @content_id
ORDER BY t.name;

-- name: ListFeaturedFederatableContentByAuthor :many
-- featured collection source rows (§126 M4): the member's pinned
-- federatable contents — the outbox eligibility predicate plus is_pinned.
-- Column list mirrors ListFederatableContentByAuthor so objectDocument
-- renders both identically.
SELECT c.id, c.content_type, c.title, c.slug, c.body_html, c.canonical_url,
       c.featured_image_id, c.header_image_id, c.published_at, c.updated_at, c.created_at,
       mf.storage_bucket AS featured_bucket, mf.storage_path AS featured_path, mf.mime_type AS featured_mime,
       mh.storage_bucket AS header_bucket, mh.storage_path AS header_path, mh.mime_type AS header_mime
FROM contents c
LEFT JOIN media mf ON mf.id = c.featured_image_id AND mf.deleted_at IS NULL AND mf.confirmed = true
LEFT JOIN media mh ON mh.id = c.header_image_id AND mh.deleted_at IS NULL AND mh.confirmed = true
WHERE c.author_id = @author_id
  AND c.status = 'published'
  AND c.visibility = 'public'
  AND c.deleted_at IS NULL
  AND c.federate_opt_out = false
  AND c.is_pinned = true
  AND c.content_type = ANY(@content_types::text[])
ORDER BY c.published_at DESC NULLS LAST, c.created_at DESC
LIMIT @page_limit;

-- name: ListActorFederatedTags :many
-- featuredTags collection source (§126 M4): the hashtags the member
-- uses across their federatable contents — same eligibility predicate as
-- the outbox, deduped by tag row.
SELECT DISTINCT t.id, t.name, t.slug
FROM content_tags ct
JOIN tags t ON t.id = ct.tag_id
JOIN contents c ON c.id = ct.content_id
WHERE c.author_id = @author_id
  AND c.status = 'published'
  AND c.visibility = 'public'
  AND c.deleted_at IS NULL
  AND c.federate_opt_out = false
  AND c.content_type = ANY(@content_types::text[])
ORDER BY t.name;

-- name: UpsertFederationRelay :one
-- Relay subscribe ledger (FEP-ae0c client side). A fresh subscribe on a
-- dead-status row re-arms it (pending + new follow_ap_id supplied by the
-- caller); a live row keeps its status/follow identity so resubscribing
-- is a no-op.
INSERT INTO federation_relays (actor_url, inbox_url, domain, status, follow_ap_id, created_by)
VALUES (@actor_url, @inbox_url, @domain, 'pending', @follow_ap_id, @created_by)
ON CONFLICT (actor_url) DO UPDATE SET
    inbox_url = EXCLUDED.inbox_url,
    domain = EXCLUDED.domain,
    last_error = NULL,
    follow_ap_id = CASE WHEN federation_relays.status IN ('rejected','failed','disabled')
                        THEN EXCLUDED.follow_ap_id
                        ELSE federation_relays.follow_ap_id END,
    status = CASE WHEN federation_relays.status IN ('rejected','failed','disabled')
                  THEN 'pending' ELSE federation_relays.status END,
    updated_at = now()
RETURNING *;

-- name: GetFederationRelayByID :one
SELECT * FROM federation_relays WHERE id = @id;

-- name: GetFederationRelayByFollowApID :one
SELECT * FROM federation_relays WHERE follow_ap_id = @follow_ap_id;

-- name: GetFederationRelayByActorURL :one
SELECT * FROM federation_relays WHERE actor_url = @actor_url;

-- name: ListFederationRelays :many
SELECT * FROM federation_relays
WHERE (@status::text = '' OR status = @status)
  AND (@q::text = '' OR actor_url ILIKE '%' || @q || '%' OR domain ILIKE '%' || @q || '%')
ORDER BY created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: CountFederationRelays :one
SELECT COUNT(*)::bigint FROM federation_relays
WHERE (@status::text = '' OR status = @status)
  AND (@q::text = '' OR actor_url ILIKE '%' || @q || '%' OR domain ILIKE '%' || @q || '%');

-- name: SetFederationRelayStatus :exec
UPDATE federation_relays SET status = @status, last_error = @last_error, updated_at = now()
WHERE id = @id;

-- name: SetFederationRelayStatusByActorURL :exec
-- Delete{actor}/unreachable unwind — fanout must stop targeting the dead
-- relay until an admin resubscribes.
UPDATE federation_relays SET status = @status, last_error = @last_error, updated_at = now()
WHERE actor_url = @actor_url AND status = 'accepted';

-- name: SetFederationRelayStatusByInbox :exec
-- Unreachable unwind — a delivery to the relay's inbox dead-lettered.
-- Accepted rows unwind only on ENDPOINT-LEVEL causes (inbox 404/410,
-- unusable advertised URL, retry budget exhausted, destination
-- blocked): a remote that ANSWERED with a plain 4xx refused that
-- particular POST — activity/signature-specific, not unreachable — so
-- one malformed activity can't kill a live subscription. A pending row
-- unwinds whenever its own Follow dead-letters (any definitive refusal
-- is a failed handshake) but never on unrelated inbox traffic — a
-- sharedInbox refusal must not kill a handshake in flight.
UPDATE federation_relays SET status = @status, last_error = @last_error, updated_at = now()
WHERE inbox_url = @inbox_url
  AND ((status = 'accepted' AND @endpoint_level::boolean)
      OR (status = 'pending' AND follow_ap_id = @activity_ap_id));

-- name: DeleteFederationRelay :exec
DELETE FROM federation_relays WHERE id = @id;

-- name: ListActiveRelayInboxes :many
-- Fanout expansion: every accepted relay's delivery target.
SELECT inbox_url FROM federation_relays WHERE status = 'accepted';
