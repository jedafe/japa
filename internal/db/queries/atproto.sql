-- name: GetAtprotoAccountByHandle :one
-- Handle resolution for /.well-known/atproto-did + did:web docs. Only
-- active accounts of active users resolve — suspended/deleted members
-- and disabled accounts don't exist on the atproto network.
-- The live u.username match outranks the stored handle: after a rename
-- the member's new subdomain resolves to their permanent DID (mirrors
-- the federated_actors precedence pattern).
SELECT a.*, u.username AS live_username
FROM atproto_accounts a
JOIN users u ON u.id = a.user_id
WHERE (lower(a.handle) = lower(@handle) OR u.username = @username)
  AND a.status = 'active'
  AND u.status = 'active'
ORDER BY (u.username = @username) DESC
LIMIT 1;

-- name: GetAtprotoHandleClaim :one
-- Any-status claimant of a handle, joined to its live username — lets
-- EnableAccount distinguish a live-aligned holder (409) from a stale
-- row left behind by a rename (reclaim via lazy sync, then insert).
SELECT a.*, u.username AS live_username
FROM atproto_accounts a
JOIN users u ON u.id = a.user_id
WHERE lower(a.handle) = lower(@handle)
LIMIT 1;

-- name: UpdateAtprotoAccountHandle :exec
-- Lazy rename sync: the stored handle tracks the live username (the DID
-- itself is immutable — did:web:...:u:{mint_username} never changes).
UPDATE atproto_accounts
SET handle = @handle, updated_at = now()
WHERE id = @id
  AND lower(handle) <> lower(@handle);

-- name: GetAtprotoAccountByUserID :one
-- Member status reads + opt-in upsert path. Joins users so suspended
-- members can't re-enable through the ON CONFLICT arm.
SELECT a.*
FROM atproto_accounts a
JOIN users u ON u.id = a.user_id
WHERE a.user_id = @user_id
  AND u.status = 'active';

-- name: GetAtprotoAccountByDID :one
-- did:web document resolution. The :u:{mint_username} path component IS
-- the DID encoding — looked up by DID (immutable) rather than handle so
-- the minted identifier keeps resolving after renames and handle syncs.
SELECT a.*, u.username AS live_username
FROM atproto_accounts a
JOIN users u ON u.id = a.user_id
WHERE a.did = @did
  AND a.status = 'active'
  AND u.status = 'active';

-- name: CreateAtprotoAccount :one
-- Opt-in insert; ON CONFLICT reactivates a disabled row (key custody is
-- permanent — re-enable keeps the same DID + keypair).
INSERT INTO atproto_accounts (
    user_id, handle, did, signing_key_enc, public_key_multibase
) VALUES (
    @user_id, @handle, @did, @signing_key_enc, @public_key_multibase
)
ON CONFLICT (user_id) DO UPDATE SET
    status = 'active',
    disabled_at = NULL,
    updated_at = now()
RETURNING *;

-- name: UpdateAtprotoAccountStatus :exec
UPDATE atproto_accounts
SET status = @status,
    disabled_at = CASE WHEN @status = 'active' THEN NULL ELSE now() END,
    updated_at = now()
WHERE id = @id;

-- name: ListAtprotoAccounts :many
-- Admin list — signing_key_enc selected but sanitized in the service
-- layer before serialization (never leaves the API).
SELECT a.id, a.user_id, a.handle, a.did, a.public_key_multibase,
       a.status, a.repo_seq, a.signing_key_enc,
       a.created_at, a.updated_at, a.disabled_at,
       u.username
FROM atproto_accounts a
JOIN users u ON u.id = a.user_id
WHERE (@q::text = '' OR a.handle ILIKE '%' || @q || '%' OR u.username ILIKE '%' || @q || '%')
ORDER BY a.created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: CountAtprotoAccounts :one
SELECT COUNT(*)::bigint
FROM atproto_accounts a
JOIN users u ON u.id = a.user_id
WHERE (@q::text = '' OR a.handle ILIKE '%' || @q || '%' OR u.username ILIKE '%' || @q || '%');

-- name: GetAtprotoStats :one
SELECT
    (SELECT COUNT(*) FROM atproto_accounts WHERE status = 'active')::bigint AS active_accounts,
    (SELECT COUNT(*) FROM atproto_accounts WHERE status = 'disabled')::bigint AS disabled_accounts,
    (SELECT COUNT(*) FROM atproto_accounts)::bigint AS total_accounts;
