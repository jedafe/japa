# Section 02 — Authentication & Sessions

**Status:** ✅ Locked (reconstructed from session material and verified against security audit notes — previously the thinnest section in the set; now fully specified)

## Decided

### Auth methods
Email/password, OAuth social login (Google, Apple, Facebook), and TOTP-based 2FA. All three are built into core, not an extension.

### Password storage and policy
- bcrypt cost 12 (from the OWASP A02 row in the security audit table — cost 12 is the explicit stated value, not a default).
- Admin-configurable password policy via `system_settings` keys:
  - `auth.password.min_length` (default: 8)
  - `auth.password.require_uppercase` (default: true)
  - `auth.password.require_number` (default: true)
  - `auth.password.require_symbol` (default: false)
  - `auth.password.max_length` (default: 128)
  - `auth.password.breach_check` (default: true) — checks against HaveIBeenPwned API using k-anonymity (sends first 5 chars of SHA-1 hash, not the full password; zero privacy risk).

### Token model
Short-lived JWT access tokens (15-minute expiry) + long-lived refresh tokens stored in SameSite=Strict, HttpOnly cookies. All session state lives in Valkey (stateless app servers — no in-memory session state).

**JWT key rotation (zero-disruption):** A single JWT secret that forces a rotation to mass-invalidate all sessions was explicitly identified as a gap and fixed. Multiple active signing secrets identified by a `kid` (key ID) header in the JWT:
```json
{ "kid": "key-2024-07", "alg": "HS256" }
```
The validator checks `kid` against a small Valkey set of active secrets (current + previous). Old secrets are retired after one access token TTL (15 minutes) — no user gets logged out on secret rotation.

### 2FA (TOTP)
`totp_credentials` table (`user_id`, `secret_encrypted`, `enabled_at`, `backup_codes`). **Mandatory** for any user accessing the Admin Panel or Control Panel: enforced at middleware — if `totp_credentials.enabled_at IS NULL` for a user whose role includes admin capabilities, redirect to the 2FA setup page before any admin action is permitted. Optional (but encouraged) for regular users.

### CSRF posture
SameSite=Strict on refresh-token cookies + Bearer-header-only for access tokens on all mutation endpoints. This combination eliminates CSRF risk without a synchronizer token — SameSite=Strict means the refresh cookie isn't sent on cross-site requests, and Bearer tokens in Authorization headers aren't sent by browsers automatically. Documented as a RULES.md constraint: *"Never accept auth via cookie for mutation endpoints. Bearer token in Authorization header only."*

### OAuth social login
OAuth 2.0 flow, provider tokens never stored — only the resulting `user_id` linkage and the provider's `sub` identifier in an `oauth_accounts` table (`user_id`, `provider`, `provider_user_id`, `created_at`). On first OAuth login, create account if no matching email exists; if a matching email exists, merge the OAuth identity onto the existing account.

### Idempotency keys
Payment endpoints, content creation, and destructive form submissions accept an `Idempotency-Key` header. Stored in Valkey with 24h TTL:
- Key: `jc:idempotency:{user_id}:{key}`
- Value: `{status_code, response_body}`

**Atomicity is critical** — use `SET NX EX` (set-if-not-exists with expiry), not a GET-then-SET sequence. The GET-then-SET pattern has a race condition where two concurrent requests both find the key absent and both proceed to execute the mutation:

```go
func IdempotencyMiddleware(v *valkey.Client) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            key := r.Header.Get("Idempotency-Key")
            if key == "" { next.ServeHTTP(w, r); return }

            vKey := "jc:idempotency:" + MustUserID(r).String() + ":" + key

            // SET NX EX: atomic — only one request wins the lock
            ok, _ := v.SetNX(r.Context(), vKey, "in_flight", 24*time.Hour)
            if !ok {
                // Key already exists — return cached response or 409 if still in flight
                cached, _ := v.Get(r.Context(), vKey)
                if cached == "in_flight" {
                    renderError(w, http.StatusConflict, "duplicate_request", "Request already in flight")
                    return
                }
                // Return previously cached response
                var prev CachedResponse
                _ = json.Unmarshal([]byte(cached), &prev)
                w.WriteHeader(prev.StatusCode)
                w.Write(prev.Body)
                return
            }
            // Won the lock — process, then cache result
            rr := &responseRecorder{ResponseWriter: w}
            next.ServeHTTP(rr, r)
            cached, _ := json.Marshal(CachedResponse{StatusCode: rr.status, Body: rr.body})
            v.Set(r.Context(), vKey, string(cached), 24*time.Hour)
        })
    }
}
```

Duplicate key within 24h → return cached response without re-processing. Prevents double-charged payments and duplicate content on network retry.

### User profile type
`user_profiles.profile_type` field (`personal` / `business` / `organisation`) set at registration. Business profiles carry extra fields: `business_name`, `registration_number`, `operating_hours JSONB`. This avoids a migration when the Service Directory (V2) activates.

### WebSocket auth
Short-lived WS ticket minted by an authenticated REST call (`POST /auth/ws-ticket`) — not a long-lived credential passed in the WebSocket handshake. See Section 10 for the full WS auth flow.

### Brute-force protection
Exponential backoff lockout on repeated failed login attempts, keyed by account and by IP independently — both can trigger a lockout, whichever fires first. See `ai-memory/skills/abuse-protection-patterns.md`.

## See Also
Section 10 (WS ticket flow), Section 22 (Admin Panel 2FA enforcement), ADR-022 (layered rate limiting, which includes login-endpoint limits).


## Users & Auth Tables (audit pass — schemas were missing)

```sql
CREATE TABLE users (
  id                   uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id            uuid NOT NULL REFERENCES tenants(id),
  user_group_id        uuid NOT NULL REFERENCES user_groups(id),
  email                text NOT NULL,
  username             text NOT NULL,
  display_name         text NOT NULL,
  password_hash        text,                         -- NULL for OAuth-only accounts (bcrypt cost 12)
  profile_type         text NOT NULL DEFAULT 'personal'
                         CHECK (profile_type IN ('personal', 'business', 'organisation')),
  avatar_media_id      uuid REFERENCES media(id) ON DELETE SET NULL,
  bio                  text,
  current_geo_space_id uuid REFERENCES spaces(id) ON DELETE SET NULL,  -- primary space (Section 05)
  is_verified          boolean NOT NULL DEFAULT false,
  verification_type    text,   -- 'embassy'|'immigration_lawyer'|'business'|'notable_member'
  is_bot               boolean NOT NULL DEFAULT false,
  email_verified_at    timestamptz,
  last_seen_at         timestamptz,
  dm_privacy           text NOT NULL DEFAULT 'everyone'
                         CHECK (dm_privacy IN ('everyone', 'followers_only', 'nobody')),
  password_changed_at  timestamptz,
  status               text NOT NULL DEFAULT 'active'
                         CHECK (status IN ('active', 'suspended', 'banned', 'deactivated')),
  created_at           timestamptz NOT NULL DEFAULT now(),
  deleted_at           timestamptz,
  UNIQUE(tenant_id, email),
  UNIQUE(tenant_id, username)
);
CREATE INDEX idx_users_email    ON users (tenant_id, lower(email));
CREATE INDEX idx_users_username ON users (tenant_id, lower(username));
CREATE INDEX idx_users_geo      ON users (current_geo_space_id) WHERE current_geo_space_id IS NOT NULL;

CREATE TABLE user_profiles (
  user_id           uuid PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
  profession        text,
  tagline           text,
  date_of_birth     date,
  gender            text,
  social_links      jsonb NOT NULL DEFAULT '{}',  -- {twitter, linkedin, github, website, ...}
  field_values      jsonb NOT NULL DEFAULT '{}',  -- custom_field_group values (Section 06)
  interest_embedding vector(1536),               -- for Relevant sort in Section 32
  updated_at        timestamptz NOT NULL DEFAULT now()
);

-- JWT signing key store (multi-key rotation)
CREATE TABLE jwt_signing_keys (
  kid        text PRIMARY KEY,    -- e.g. 'key-2024-07'
  secret     text NOT NULL,       -- encrypted at rest via JC_ENCRYPTION_KEY
  active     boolean NOT NULL DEFAULT true,
  created_at timestamptz NOT NULL DEFAULT now(),
  retired_at timestamptz         -- set when rotated out; key still valid for 15-min access token TTL
);

-- OAuth social accounts
CREATE TABLE oauth_accounts (
  id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id          uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  provider         text NOT NULL,           -- 'google'|'apple'|'facebook'
  provider_user_id text NOT NULL,
  created_at       timestamptz NOT NULL DEFAULT now(),
  UNIQUE(provider, provider_user_id)
);

-- 2FA TOTP
CREATE TABLE totp_credentials (
  user_id       uuid PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
  secret_encrypted text NOT NULL,           -- encrypted with JC_ENCRYPTION_KEY
  backup_codes  text[] NOT NULL DEFAULT '{}',
  enabled_at    timestamptz NOT NULL DEFAULT now()
);
```

## Password Reset Flow

```sql
CREATE TABLE password_reset_tokens (
  id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id    uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  token_hash text NOT NULL UNIQUE,   -- SHA-256 of raw token; raw token sent by email only
  expires_at timestamptz NOT NULL DEFAULT now() + interval '1 hour',
  used_at    timestamptz,
  created_at timestamptz NOT NULL DEFAULT now()
);
```

```
POST /auth/forgot-password { email }
  → Rate-limited (3 requests per 10 min per email); always returns 200 (no email enumeration)
  → Creates password_reset_tokens row, emails link with raw token
  → Asynq task: email:send_password_reset (critical tier)

GET  /auth/reset-password?token=xxx
  → Validates SHA-256 hash, checks expiry and used_at; renders reset form (SvelteKit page)

POST /auth/reset-password { token, new_password }
  → Validates token, bcrypt hashes new password
  → Sets users.password_changed_at = now()
  → Marks token used_at = now()
  → Deletes all refresh tokens for this user from Valkey (force re-login everywhere)
  → Audit log: 'user.password_reset'
```

## Email Verification Flow

```sql
CREATE TABLE email_verifications (
  id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id     uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  email       text NOT NULL,           -- email being verified (may differ during address change)
  token_hash  text NOT NULL UNIQUE,
  expires_at  timestamptz NOT NULL DEFAULT now() + interval '24 hours',
  verified_at timestamptz,
  created_at  timestamptz NOT NULL DEFAULT now()
);
```

```
On registration:  enqueue email:send_verification (critical tier)
GET /auth/verify-email?token=xxx
  → Validates token, sets users.email_verified_at = now()
  → Redirects to onboarding wizard (Section 09)

POST /auth/resend-verification
  → Rate-limited (1 per 5 min); invalidates previous unverified token; sends new one
```

**Session invalidation on password change:** all refresh token keys `jc:refresh:{user_id}:*` are deleted from Valkey when password changes. This forces re-login on all devices. The pattern is: `SCAN` for keys matching `jc:refresh:{user_id}:*` then `DEL` in a pipeline.
