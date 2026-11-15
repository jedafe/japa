-- name: GetAtprotoAccountByHandle :one
-- Handle resolution for /.well-known/atproto-did + did:web docs.
-- 'disabled' accounts keep resolving (upstream deactivated-repo
-- semantics: identity stays verifiable while the repo is frozen); only
-- tombstone rows and non-'active' users go dark.
-- The live u.username match outranks the stored handle: after a rename
-- the member's new subdomain resolves to their permanent DID (mirrors
-- the federated_actors precedence pattern). lower() on the username side
-- because handles are normalized lowercase but users.username preserves
-- case — 'Alice2' must still match a live username 'Alice2' → 'alice2'.
-- At most one account can match lower(u.username) at a time: two users
-- whose names collide under lower() compete for the same derived handle
-- and only one can hold an account.
SELECT a.*, u.username AS live_username
FROM atproto_accounts a
JOIN users u ON u.id = a.user_id
WHERE (lower(a.handle) = lower(@handle) OR lower(u.username) = @username)
  AND a.status IN ('active', 'disabled')
  AND u.status = 'active'
ORDER BY (lower(u.username) = @username) DESC
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
-- itself is immutable — did:web:...:u:{user_uuid} never changes).
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

-- name: GetAtprotoIdentityByDID :one
-- did:web document resolution. The :u:{user_uuid} path component IS
-- the DID encoding — looked up by DID (immutable) rather than handle so
-- the minted identifier keeps resolving after renames and handle syncs.
-- 'disabled' accounts keep their document (upstream deactivated-repo
-- semantics — prior signed content stays verifiable); tombstone rows
-- and non-'active' users resolve nothing.
-- previous_did matches keep serving the pre-migration did:web doc after
-- an A6-2 swap (§127): embedded at://{old_did} refs in records stay
-- resolvable. matched_did tells the caller WHICH did the document is
-- for — a previous_did hit must still declare the old did as its id.
SELECT a.*, u.username AS live_username,
       COALESCE(CASE WHEN a.did = @did THEN a.did ELSE a.previous_did END, '')::text AS matched_did
FROM atproto_accounts a
JOIN users u ON u.id = a.user_id
WHERE (a.did = @did OR a.previous_did = @did)
  AND a.status IN ('active', 'disabled')
  AND u.status = 'active'
ORDER BY (a.did = @did) DESC
LIMIT 1;

-- name: GetAtprotoRepoRefByDID :one
-- Sync-surface repo lookup — deliberately unfiltered by status so the
-- caller can name WHY a repo isn't servable (RepoDeactivated /
-- RepoSuspended / RepoTakendown per the lexicon) instead of a blanket
-- RepoNotFound.
SELECT a.id, a.did, a.status AS account_status, u.status AS user_status
FROM atproto_accounts a
JOIN users u ON u.id = a.user_id
WHERE a.did = @did OR a.previous_did = @did
ORDER BY (a.did = @did) DESC
LIMIT 1;

-- name: GetAtprotoRepoStatusByDID :one
-- getRepoStatus: account + user status + head rev in one read. The
-- endpoint must answer for inactive repos, so nothing is filtered.
SELECT a.status AS account_status, u.status AS user_status,
       COALESCE((SELECT c.rev FROM atproto_repo_commits c
        WHERE c.account_id = a.id
        ORDER BY c.seq DESC LIMIT 1), '')::text AS head_rev
FROM atproto_accounts a
JOIN users u ON u.id = a.user_id
WHERE a.did = @did OR a.previous_did = @did
ORDER BY (a.did = @did) DESC
LIMIT 1;

-- name: ListAtprotoRepos :many
-- listRepos: hosted-repo census. Tombstone rows are gone from the
-- network (excluded); disabled accounts list with active:false so
-- consumers see the freeze. Repos without a single commit have no
-- network presence (head/rev are required output fields) — the lateral
-- join excludes them. Cursor is opaque: the last returned did.
SELECT a.did, a.status AS account_status, u.status AS user_status,
       head.cid AS head_cid, head.rev AS head_rev
FROM atproto_accounts a
JOIN users u ON u.id = a.user_id
JOIN LATERAL (
    SELECT c.cid, c.rev FROM atproto_repo_commits c
    WHERE c.account_id = a.id
    ORDER BY c.seq DESC LIMIT 1
) head ON true
WHERE a.status <> 'tombstone'
  AND (@cursor::text = '' OR a.did > @cursor)
ORDER BY a.did
LIMIT @page_limit;

-- name: GetAtprotoAccountAnyUserStatus :one
-- Lifecycle sync + erasure: the account row regardless of user status —
-- the joined queries all filter u.status='active', which would hide
-- exactly the rows lifecycle transitions must reach.
SELECT * FROM atproto_accounts WHERE user_id = @user_id;

-- name: CreateAtprotoAccount :one
-- Opt-in insert; ON CONFLICT reactivates a disabled row (key custody is
-- permanent — re-enable keeps the same DID + keypair). The WHERE guard
-- refuses to resurrect a tombstoned row: a conflict against tombstone
-- yields no RETURNING row (ErrNoRows → caller maps tombstone-terminal).
INSERT INTO atproto_accounts (
    user_id, handle, did, signing_key_enc, public_key_multibase,
    plc_rotation_key_enc
) VALUES (
    @user_id, @handle, @did, @signing_key_enc, @public_key_multibase,
    @plc_rotation_key_enc
)
ON CONFLICT (user_id) DO UPDATE SET
    status = 'active',
    disabled_reason = NULL,
    disabled_at = NULL,
    updated_at = now()
WHERE atproto_accounts.status <> 'tombstone'
RETURNING *;

-- name: UpdateAtprotoAccountStatus :exec
-- disabled_reason records who froze the account ('member' | 'lifecycle'
-- — §127 lifecycle); NULL clears it (reactivation, tombstone).
UPDATE atproto_accounts
SET status = @status,
    disabled_reason = sqlc.narg(disabled_reason),
    disabled_at = CASE WHEN @status = 'active' THEN NULL ELSE now() END,
    updated_at = now()
WHERE id = @id;

-- name: GetAtprotoAccountByPreviousDID :one
-- A6-2 fallback: old-DID provenance resolution for late remote
-- replies/follows and old-did XRPC lookups. Tombstone rows resolve
-- nothing — their records are retracted and their identity is gone.
SELECT * FROM atproto_accounts
WHERE previous_did = @previous_did AND status <> 'tombstone';

-- name: GetAtprotoAccountPLCMaterial :one
-- The plc op inputs for an account: current DID/handle/status, the repo
-- signing pubkey (verificationMethods.atproto), rotation-key
-- ciphertext, and the member's live username (the genesis op's
-- alsoKnownAs carries the CURRENT desired handle, not a stale stored
-- one). Narrow read — no key material leaves the service layer.
SELECT a.id, a.user_id, a.did, a.handle, a.status,
       a.public_key_multibase, a.plc_rotation_key_enc,
       u.username AS live_username
FROM atproto_accounts a
JOIN users u ON u.id = a.user_id
WHERE a.id = @id;

-- name: SetAtprotoAccountRotationKey :execrows
-- First-write-wins rotation-key custody: the IS NULL guard stops a
-- racing provisioning attempt from clobbering a key a submitted op
-- already references. Zero rows = key already set (treat as success).
UPDATE atproto_accounts
SET plc_rotation_key_enc = @plc_rotation_key_enc, updated_at = now()
WHERE id = @id AND plc_rotation_key_enc IS NULL;

-- name: SwapAtprotoAccountDID :execrows
-- The A6-2 identity swap: CAS on the expected current DID so a double
-- run can't regress a newer swap. previous_did preserves the outgoing
-- identity for old-URI provenance; a did:web→did:web retry is the only
-- path that could clobber it and the caller early-returns before that.
UPDATE atproto_accounts
SET did = @new_did, previous_did = @old_did, updated_at = now()
WHERE id = @id AND did = @old_did;

-- name: RewriteAtprotoRecordDIDPrefix :execrows
-- The URI rewrite half of the A6-2 swap — runs in the same tx so the
-- index never straddles two identities. 'at://{did}' is the authority
-- prefix; the /collection/rkey tail is preserved verbatim.
UPDATE atproto_records
SET uri = 'at://' || @new_did || substr(uri, length('at://' || @old_did) + 1)
WHERE account_id = @account_id
  AND uri LIKE 'at://' || @old_did || '/%';

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
    (SELECT COUNT(*) FROM atproto_accounts)::bigint AS total_accounts,
    (SELECT COUNT(*) FROM atproto_records)::bigint AS total_records,
    (SELECT COUNT(*) FROM atproto_repo_commits)::bigint AS total_commits,
    (SELECT COUNT(*) FROM atproto_repo_events)::bigint AS total_repo_events;

-- name: InsertAtprotoConformanceRun :one
-- §127 admin: persist a conformance run result (checks as jsonb array).
INSERT INTO atproto_conformance_runs (actor, did, checks, passed, run_by)
VALUES (@actor, @did, @checks::jsonb, @passed, @run_by)
RETURNING id, actor, did, checks, passed, run_by, created_at;

-- name: ListAtprotoConformanceRuns :many
SELECT id, actor, did, checks, passed, run_by, created_at
FROM atproto_conformance_runs
ORDER BY created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: CountAtprotoConformanceRuns :one
SELECT COUNT(*)::bigint FROM atproto_conformance_runs;

-- name: PickAtprotoConformanceTarget :one
-- Auto-pick for the admin check + cmd/atproto-conformance: an active
-- account, preferring one with at least one projected feed.post so the
-- post-indexed check is meaningful.
SELECT a.did, EXISTS (
    SELECT 1 FROM atproto_records r
    WHERE r.account_id = a.id AND r.collection = 'app.bsky.feed.post'
) AS has_posts
FROM atproto_accounts a
WHERE a.status = 'active'
ORDER BY has_posts DESC, a.created_at ASC
LIMIT 1;

-- name: AtprotoDIDHasPosts :one
-- Post check needs "did we project any feed.post for this DID" — an
-- empty appview feed only fails when our own index holds posts.
SELECT EXISTS (
    SELECT 1 FROM atproto_records r
    JOIN atproto_accounts a ON a.id = r.account_id
    WHERE a.did = @did AND r.collection = 'app.bsky.feed.post'
);
