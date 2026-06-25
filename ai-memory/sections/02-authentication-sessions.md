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
Short-lived JWT access tokens (15-minute expiry) + long-lived refresh tokens stored in SameSite=Strict, HttpOnly cookies. All session state lives in Redis (stateless app servers — no in-memory session state).

**JWT key rotation (zero-disruption):** A single JWT secret that forces a rotation to mass-invalidate all sessions was explicitly identified as a gap and fixed. Multiple active signing secrets identified by a `kid` (key ID) header in the JWT:
```json
{ "kid": "key-2024-07", "alg": "HS256" }
```
The validator checks `kid` against a small Redis set of active secrets (current + previous). Old secrets are retired after one access token TTL (15 minutes) — no user gets logged out on secret rotation.

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
Duplicate key within 24h → return cached response without re-processing. Prevents double-charged payments and duplicate content on network retry.

### User profile type
`user_profiles.profile_type` field (`personal` / `business` / `organisation`) set at registration. Business profiles carry extra fields: `business_name`, `registration_number`, `operating_hours JSONB`. This avoids a migration when the Service Directory (V2) activates.

### WebSocket auth
Short-lived WS ticket minted by an authenticated REST call (`POST /auth/ws-ticket`) — not a long-lived credential passed in the WebSocket handshake. See Section 10 for the full WS auth flow.

### Brute-force protection
Exponential backoff lockout on repeated failed login attempts, keyed by account and by IP independently — both can trigger a lockout, whichever fires first. See `ai-memory/skills/abuse-protection-patterns.md`.

## See Also
Section 10 (WS ticket flow), Section 22 (Admin Panel 2FA enforcement), ADR-022 (layered rate limiting, which includes login-endpoint limits).

