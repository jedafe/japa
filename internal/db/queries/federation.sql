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
       COALESCE(ups.profile_visibility = 'followers', false) AS manually_approves_followers,
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

-- name: GetFederatedActorByID :one
SELECT * FROM federated_actors WHERE id = @id;

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
ON CONFLICT (user_id) DO UPDATE SET
    status = 'active',
    updated_at = now()
RETURNING *;

-- name: UpdateFederatedActorStatus :exec
UPDATE federated_actors SET status = @status, updated_at = now() WHERE id = @id;

-- name: ListFederatedActors :many
-- Admin DataTable: ?q= matches username/actor_url; sorted newest first.
-- u.email deliberately excluded — actor management needs the handle, not
-- PII (admins who need email use the users surface).
SELECT fa.*, u.username, up.display_name, up.avatar_url
FROM federated_actors fa
JOIN users u ON u.id = fa.user_id
LEFT JOIN user_profiles up ON up.user_id = u.id
WHERE (@q::text = '' OR u.username ILIKE '%' || @q || '%' OR fa.actor_url ILIKE '%' || @q || '%')
ORDER BY fa.created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: CountFederatedActors :one
SELECT COUNT(*)::bigint
FROM federated_actors fa
JOIN users u ON u.id = fa.user_id
WHERE (@q::text = '' OR u.username ILIKE '%' || @q || '%' OR fa.actor_url ILIKE '%' || @q || '%');

-- name: GetNodeInfoUsage :one
-- NodeInfo 2.1 usage counters (active users, published contents, comments).
SELECT
    (SELECT COUNT(*) FROM users WHERE status = 'active')::bigint AS total_users,
    (SELECT COUNT(*) FROM contents WHERE status = 'published' AND deleted_at IS NULL)::bigint AS local_posts,
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

-- name: UpsertFederationFollow :one
INSERT INTO federation_follows (local_actor_id, remote_actor_id, ap_activity_id, status)
VALUES (@local_actor_id, @remote_actor_id, @ap_activity_id, @status)
ON CONFLICT (local_actor_id, remote_actor_id) DO UPDATE SET
    ap_activity_id = COALESCE(EXCLUDED.ap_activity_id, federation_follows.ap_activity_id),
    status = EXCLUDED.status,
    updated_at = now()
RETURNING *;

-- name: GetFederationFollowByActivityID :one
SELECT * FROM federation_follows WHERE ap_activity_id = @ap_activity_id;

-- name: UpdateFederationFollowStatus :exec
UPDATE federation_follows SET status = @status, updated_at = now()
WHERE local_actor_id = @local_actor_id AND remote_actor_id = @remote_actor_id;

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
-- actor URLs appear in the AP followers collection too.
SELECT fa.actor_url
FROM follows f
JOIN federated_actors fa ON fa.user_id = f.follower_id AND fa.status = 'active'
WHERE f.entity_type = 'user' AND f.entity_id = @user_id
ORDER BY f.created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: CountLocalFederatedFollowers :one
SELECT COUNT(*)::bigint
FROM follows f
JOIN federated_actors fa ON fa.user_id = f.follower_id AND fa.status = 'active'
WHERE f.entity_type = 'user' AND f.entity_id = @user_id;

-- name: ListLocalFederatedFollowingURLs :many
-- Following collection items: users the member follows who are federated.
SELECT fa.actor_url
FROM follows f
JOIN federated_actors fa ON fa.user_id = f.entity_id AND fa.status = 'active'
WHERE f.follower_id = @user_id AND f.entity_type = 'user'
ORDER BY f.created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: CountLocalFederatedFollowing :one
SELECT COUNT(*)::bigint
FROM follows f
JOIN federated_actors fa ON fa.user_id = f.entity_id AND fa.status = 'active'
WHERE f.follower_id = @user_id AND f.entity_type = 'user';

-- name: ListFederatableContentByAuthor :many
-- Outbox source rows (§126): published, public, non-deleted contents on the
-- federatable type allowlist.
SELECT c.id, c.content_type, c.title, c.slug, c.body_html, c.canonical_url,
       c.featured_image_id, c.header_image_id, c.published_at, c.updated_at, c.created_at
FROM contents c
WHERE c.author_id = @author_id
  AND c.status = 'published'
  AND c.visibility = 'public'
  AND c.deleted_at IS NULL
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
  AND c.content_type = ANY(@content_types::text[]);

-- name: GetFederatableContentByID :one
SELECT c.id, c.author_id, c.content_type, c.title, c.slug, c.body_html,
       c.canonical_url, c.featured_image_id, c.header_image_id,
       c.published_at, c.updated_at, c.created_at
FROM contents c
WHERE c.id = @id
  AND c.status = 'published'
  AND c.visibility = 'public'
  AND c.deleted_at IS NULL
  AND c.content_type = ANY(@content_types::text[]);

-- name: InsertFederationDelivery :one
INSERT INTO federation_deliveries (
    activity_ap_id, activity_type, activity_json, local_actor_id,
    inbox_url, remote_actor_id, next_retry_at
) VALUES (
    @activity_ap_id, @activity_type, @activity_json, @local_actor_id,
    @inbox_url, @remote_actor_id, now()
)
RETURNING *;

-- name: ListDueFederationDeliveries :many
SELECT * FROM federation_deliveries
WHERE status IN ('pending','failed')
  AND (next_retry_at IS NULL OR next_retry_at <= now())
ORDER BY created_at ASC
LIMIT @page_limit;

-- name: UpdateFederationDeliveryStatus :exec
UPDATE federation_deliveries
SET status = @status,
    attempts = @attempts,
    next_retry_at = @next_retry_at,
    last_error = @last_error,
    delivered_at = @delivered_at
WHERE id = @id;

-- name: InsertFederationInbound :one
INSERT INTO federation_inbound (
    activity_ap_id, activity_type, actor_url, key_id, signature_valid, payload
) VALUES (
    @activity_ap_id, @activity_type, @actor_url, @key_id, @signature_valid, @payload
)
RETURNING *;

-- name: GetFederationInboundByActivityID :one
SELECT * FROM federation_inbound WHERE activity_ap_id = @activity_ap_id;

-- name: UpdateFederationInboundStatus :exec
UPDATE federation_inbound SET status = @status, error = @error WHERE id = @id;

-- name: PruneFederationDeliveries :exec
DELETE FROM federation_deliveries
WHERE status IN ('delivered','dead') AND created_at < now() - make_interval(days => @retention_days);

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
SELECT * FROM federation_instance_blocks WHERE domain = @domain;
