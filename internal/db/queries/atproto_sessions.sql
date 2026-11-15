-- name: GetAtprotoSessionAccount :one
-- createSession identifier resolution: handle, live username, current
-- DID, or a pre-migration previous_did all name the account (bare
-- usernames resolve through the users join — bsky.app login accepts
-- 'alice' as readily as 'alice.japa.to'). Any account status returns:
-- disabled rows mint sessions (the client needs them to reactivate);
-- only tombstone hides. user_id + email ride along for session output.
SELECT a.id, a.user_id, a.did, a.handle, a.status, a.disabled_reason,
       u.username AS live_username, u.email AS user_email, u.status AS user_status
FROM atproto_accounts a
JOIN users u ON u.id = a.user_id
WHERE lower(a.handle) = lower(@identifier)
   OR lower(u.username) = lower(@identifier)
   OR a.did = @identifier
   OR a.previous_did = @identifier
ORDER BY (a.did = @identifier) DESC,
         (lower(a.handle) = lower(@identifier)) DESC,
         (lower(u.username) = lower(@identifier)) DESC
LIMIT 1;

-- name: ListAtprotoLiveAppPasswords :many
-- createSession verifies the submitted secret against every live hash
-- on the account (the wire carries no name — any live password admits).
SELECT * FROM atproto_app_passwords
WHERE account_id = @account_id AND revoked_at IS NULL
ORDER BY created_at;

-- name: CreateAtprotoAppPassword :one
INSERT INTO atproto_app_passwords (account_id, name, password_hash)
VALUES (@account_id, @name, @password_hash)
RETURNING *;

-- name: ListAtprotoAppPasswords :many
SELECT * FROM atproto_app_passwords
WHERE account_id = @account_id AND revoked_at IS NULL
ORDER BY created_at DESC;

-- name: RevokeAtprotoAppPassword :execrows
-- Member-API revocation — the id+account pair stops cross-account hits.
UPDATE atproto_app_passwords SET revoked_at = now()
WHERE id = @id AND account_id = @account_id AND revoked_at IS NULL;

-- name: RevokeAtprotoAppPasswordByName :execrows
-- XRPC revokeAppPassword names the credential (lexicon contract).
UPDATE atproto_app_passwords SET revoked_at = now()
WHERE account_id = @account_id AND name = @name AND revoked_at IS NULL;

-- name: TouchAtprotoAppPassword :exec
UPDATE atproto_app_passwords SET last_used_at = now()
WHERE id = @id;

-- name: CreateAtprotoSession :one
INSERT INTO atproto_sessions (account_id, app_password_id, refresh_hash,
                              user_agent, last_ip, refresh_expires_at)
VALUES (@account_id, @app_password_id, @refresh_hash,
        @user_agent, @last_ip, @refresh_expires_at)
RETURNING *;

-- name: GetAtprotoSessionByID :one
-- refreshSession/deleteSession resolve the refresh JWT's jti → row.
-- previous_did rides along: a refresh JWT minted pre-migration still
-- names the old did in sub — it's valid, and rotation re-mints on the
-- current one.
SELECT s.*, a.did, a.handle, a.user_id, a.status AS account_status,
       a.disabled_reason, a.previous_did, u.email AS user_email
FROM atproto_sessions s
JOIN atproto_accounts a ON a.id = s.account_id
JOIN users u ON u.id = a.user_id
WHERE s.id = @id;

-- name: RotateAtprotoSession :execrows
-- Single-use refresh: the CAS on the presented token's hash means a
-- replayed old refresh loses to the rotation that already landed —
-- the caller distinguishes 'dead session' from 'stale hash on a live
-- one' (theft signal → revoke) via GetAtprotoLiveSessionByID.
UPDATE atproto_sessions SET refresh_hash = @new_hash
WHERE id = @id AND refresh_hash = @old_hash
  AND revoked_at IS NULL AND refresh_expires_at > now();

-- name: GetAtprotoLiveSessionByID :one
SELECT id FROM atproto_sessions
WHERE id = @id AND revoked_at IS NULL AND refresh_expires_at > now();

-- name: RevokeAtprotoSession :execrows
UPDATE atproto_sessions SET revoked_at = now()
WHERE id = @id AND revoked_at IS NULL;

-- name: RevokeAtprotoSessionsForAccount :exec
-- Kill every live session — app-password revoke cascade + tombstone
-- sweep + a member's "log out everywhere" lever.
UPDATE atproto_sessions SET revoked_at = now()
WHERE account_id = @account_id AND revoked_at IS NULL;

-- name: RevokeAtprotoSessionsForAppPassword :exec
-- Kill every session a single app password minted — revoking the
-- credential must log out the clients holding it.
UPDATE atproto_sessions SET revoked_at = now()
WHERE app_password_id = @app_password_id AND revoked_at IS NULL;

-- name: GetAtprotoAccountStatusStats :one
-- checkAccountStatus: the repo/material counts migration tooling diffs
-- between source and destination. expectedBlobs == importedBlobs — a
-- blob we index is always stored (no partial-import state exists).
SELECT a.status AS account_status,
       COALESCE((SELECT c.cid FROM atproto_repo_commits c
         WHERE c.account_id = a.id ORDER BY c.seq DESC LIMIT 1), '')::text AS repo_commit,
       COALESCE((SELECT c.rev FROM atproto_repo_commits c
         WHERE c.account_id = a.id ORDER BY c.seq DESC LIMIT 1), '')::text AS repo_rev,
       (SELECT count(*) FROM atproto_repo_blocks b WHERE b.account_id = a.id) AS repo_blocks,
       (SELECT count(*) FROM atproto_records r WHERE r.account_id = a.id) AS indexed_records,
       (SELECT count(*) FROM atproto_blobs bl WHERE bl.account_id = a.id) AS imported_blobs
FROM atproto_accounts a
WHERE a.id = @id;
