-- 001162_atproto_oauth.up.sql
-- A6-5 full ATProto OAuth (§127): pushed authorization requests + opaque
-- DPoP-bound token grants. Tokens are random opaque strings; only sha256
-- hashes persist, same posture as atproto_sessions and oauth_tokens.
--
-- Ref: japaconnect-engineering/sections/127-atproto-native.md (A6-5)

BEGIN;

-- atproto_oauth_requests: PAR row → consent → code state machine.
-- request_uri is the PAR response handle; code_* columns gate the
-- single-use authorization-code exchange.
CREATE TABLE atproto_oauth_requests (
    request_uri    text PRIMARY KEY,
    client_id      text NOT NULL,
    client_jkt     text,           -- confidential-client key binding (kid|alg|jkt)
    redirect_uri   text NOT NULL,
    scope          text NOT NULL,
    state          text NOT NULL,
    code_challenge text NOT NULL UNIQUE,  -- S256; profile requires no reuse
    dpop_jkt       text NOT NULL,  -- DPoP key thumbprint bound at PAR
    login_hint     text,
    account_id     uuid REFERENCES atproto_accounts(id) ON DELETE CASCADE,
    code_hash      text UNIQUE,    -- sha256 hex of the issued code
    code_issued_at timestamptz,    -- CAS gate — one code per request_uri
    code_used_at   timestamptz,    -- CAS consume — single-use exchange
    expires_at     timestamptz NOT NULL,
    consumed_at    timestamptz,    -- request_uri spent (decision posted)
    created_at     timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX atproto_oauth_requests_code_hash_idx
    ON atproto_oauth_requests (code_hash) WHERE code_hash IS NOT NULL;

-- atproto_oauth_tokens: one row per live grant; rotation CASes
-- refresh_hash so a replayed old refresh token can be told apart from an
-- unknown one — the stale-hash path revokes the family (theft signal).
CREATE TABLE atproto_oauth_tokens (
    id                 uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id         uuid NOT NULL REFERENCES atproto_accounts(id) ON DELETE CASCADE,
    client_id          text NOT NULL,
    client_jkt         text,
    scope              text NOT NULL,
    dpop_jkt           text NOT NULL,
    access_hash        text NOT NULL UNIQUE,
    refresh_hash       text NOT NULL UNIQUE,
    prev_refresh_hash  text,             -- last rotated-out hash: replay → revoke family
    access_expires_at  timestamptz NOT NULL,
    refresh_expires_at timestamptz NOT NULL,
    revoked_at         timestamptz,
    created_at         timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX atproto_oauth_tokens_account_idx
    ON atproto_oauth_tokens (account_id) WHERE revoked_at IS NULL;

COMMIT;
