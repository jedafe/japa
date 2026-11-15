-- A6-5 full ATProto OAuth queries (§127).
-- PAR requests are one-shot rows: authorize lookups by request_uri,
-- consent-approve CASes code_issued_at, exchange CASes code_used_at.

-- name: CreateOAuthRequest :one
INSERT INTO atproto_oauth_requests (
    request_uri, client_id, client_jkt, redirect_uri, scope, state,
    code_challenge, dpop_jkt, login_hint, expires_at
) VALUES (
    @request_uri, @client_id, @client_jkt, @redirect_uri, @scope, @state,
    @code_challenge, @dpop_jkt, @login_hint, @expires_at
)
RETURNING *;

-- name: GetOAuthRequest :one
SELECT * FROM atproto_oauth_requests WHERE request_uri = @request_uri;

-- name: IssueOAuthCode :one
-- Consent approve — CAS on code_issued_at so a repeated decision POST
-- (or a racing request_uri reuse) can never mint a second code. Returns
-- the row when this call minted; nothing when the row is spent/dead.
UPDATE atproto_oauth_requests
SET account_id = @account_id,
    code_hash = @code_hash,
    code_issued_at = now(),
    consumed_at = now()
WHERE request_uri = @request_uri
  AND code_issued_at IS NULL
  AND consumed_at IS NULL
  AND expires_at > now()
RETURNING *;

-- name: DenyOAuthRequest :one
UPDATE atproto_oauth_requests
SET consumed_at = now()
WHERE request_uri = @request_uri
  AND consumed_at IS NULL
RETURNING *;

-- name: ConsumeOAuthCode :one
-- Token exchange — CAS on code_used_at; the returned row feeds PKCE +
-- redirect_uri verification. Second presentation misses (used_at set).
UPDATE atproto_oauth_requests
SET code_used_at = now()
WHERE code_hash = @code_hash
  AND code_issued_at IS NOT NULL
  AND code_used_at IS NULL
  AND code_issued_at > now() - INTERVAL '5 minutes'
RETURNING *;

-- name: RevokeOAuthGrantsForRequest :execrows
-- Code reuse detection: when a code presented at the token endpoint
-- maps to an already-consumed request, the grant it minted dies —
-- exactly that request's grant, never a sibling the same client holds
-- (profile: replay of a redeemed code revokes the session it created).
-- The NULL branch catches grants minted before request_uri existed.
UPDATE atproto_oauth_tokens
SET revoked_at = now()
WHERE (request_uri = @request_uri
       OR (request_uri IS NULL AND account_id = @account_id AND client_id = @client_id))
  AND revoked_at IS NULL;

-- name: GetOAuthRequestByCodeHash :one
-- Code-replay probe after a ConsumeOAuthCode miss: the presented code
-- hashes to a row that was already consumed — the grant it minted dies.
SELECT * FROM atproto_oauth_requests WHERE code_hash = @code_hash;

-- name: CreateOAuthGrant :one
INSERT INTO atproto_oauth_tokens (
    account_id, request_uri, client_id, client_jkt, scope, dpop_jkt,
    access_hash, refresh_hash, access_expires_at, refresh_expires_at
) VALUES (
    @account_id, @request_uri, @client_id, @client_jkt, @scope, @dpop_jkt,
    @access_hash, @refresh_hash, @access_expires_at, @refresh_expires_at
)
RETURNING *;

-- name: GetOAuthGrantByAccessHash :one
SELECT * FROM atproto_oauth_tokens WHERE access_hash = @access_hash;

-- name: RotateOAuthGrant :one
-- Refresh rotation — CAS on refresh_hash so a replayed old refresh
-- token resolves as a miss (handled by GetOAuthGrantByAnyHash below)
-- while the live row advances to the new token pair atomically.
UPDATE atproto_oauth_tokens
SET access_hash = @new_access_hash,
    prev_refresh_hash = refresh_hash,
    refresh_hash = @new_refresh_hash,
    refresh_rotated_at = now(),
    access_expires_at = @access_expires_at
WHERE refresh_hash = @refresh_hash
  AND revoked_at IS NULL
  AND refresh_expires_at > now()
RETURNING *;

-- name: GetOAuthGrantByAnyHash :one
-- Stale-hash probe after a rotation miss: distinguishes "token doesn't
-- exist" from "replayed old token on a live grant" (revoke the family).
SELECT * FROM atproto_oauth_tokens
WHERE refresh_hash = @hash OR access_hash = @hash OR prev_refresh_hash = @hash
LIMIT 1;

-- name: RevokeOAuthGrant :one
UPDATE atproto_oauth_tokens
SET revoked_at = now()
WHERE id = @id AND revoked_at IS NULL
RETURNING *;

-- name: RevokeOAuthGrantsForAccount :execrows
-- Tombstone/erasure path — every live grant on the account dies with it.
UPDATE atproto_oauth_tokens
SET revoked_at = now()
WHERE account_id = @account_id AND revoked_at IS NULL;

-- name: ListOAuthGrantsForAccount :many
-- Member grant management: distinct live client grants, newest first.
SELECT client_id, scope, client_jkt, min(created_at) AS first_granted_at,
       max(created_at) AS last_granted_at
FROM atproto_oauth_tokens
WHERE account_id = @account_id AND revoked_at IS NULL
GROUP BY client_id, scope, client_jkt
ORDER BY last_granted_at DESC;

-- name: RevokeOAuthGrantsForClient :execrows
-- Member revokes one client's family (account_id is always the caller's).
UPDATE atproto_oauth_tokens
SET revoked_at = now()
WHERE account_id = @account_id AND client_id = @client_id AND revoked_at IS NULL;

-- name: DeleteStaleOAuthRequests :execrows
-- Daily sweep on the session-retention knob — PARs are 10-minute rows
-- with no credential material; dead tokens age out likewise.
DELETE FROM atproto_oauth_requests
WHERE created_at < now() - make_interval(days => @days::int);

-- name: DeleteStaleOAuthTokens :execrows
DELETE FROM atproto_oauth_tokens
WHERE (revoked_at IS NOT NULL AND revoked_at < now() - make_interval(days => @days::int))
   OR (revoked_at IS NULL AND refresh_expires_at < now() - make_interval(days => @days::int));
