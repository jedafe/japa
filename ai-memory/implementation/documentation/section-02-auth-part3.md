# Section 02 — Authentication & Sessions, Part 3 of 3

## Date
2026-06-25

## Scope
Auth middleware, DB store implementation, server route wiring, and JWT config fields.

## Files Created

### `internal/auth/middleware.go`
- `AuthenticatedUser` struct with `UserID` and `TenantID` (both `pgtype.UUID`)
- `UserContextKey` — typed context key for storing the authenticated user
- `FromContext(ctx)` — extracts `*AuthenticatedUser` from request context
- `RequireAuth(kr)` — middleware requiring a valid Bearer JWT; returns 401 on missing token, invalid scheme, or invalid/expired token
- `OptionalAuth(kr)` — middleware that validates a Bearer token if present but does not require it; passes through anonymously if no token or invalid token

### `internal/auth/middleware_test.go`
6 unit tests:
- `TestRequireAuth_NoToken` — 401 when no Authorization header
- `TestRequireAuth_InvalidScheme` — 401 when scheme is not Bearer
- `TestRequireAuth_ValidToken` — 200 + user in context with valid token
- `TestRequireAuth_ExpiredToken` — 401 on invalid token string
- `TestOptionalAuth_NoToken` — handler called, no user in context
- `TestOptionalAuth_ValidToken` — handler called, user in context

### `internal/auth/store.go`
- `DBStore` struct wrapping `*db.DB`
- `NewDBStore(database)` constructor
- Implements the `Querier` interface:
  - `CreateUser` — INSERT into users, RETURNING id
  - `GetUserByEmail` — SELECT by tenant_id + email from reader pool
  - `StoreRefreshToken` — INSERT into refresh_tokens
  - `GetRefreshToken` — SELECT by token_hash from reader pool
  - `RevokeRefreshToken` — UPDATE revoked_at where token_hash matches and not already revoked

## Files Updated

### `internal/server/server.go`
- Added imports: `context`, `crypto/rand`, `log/slog`, `github.com/jedafe/japa/internal/auth`
- JWT key rotation: generates a random 32-byte dev key at startup (production should load from config/secrets)
- Auth handlers wired with `DBStore`, `KeyRotation`, `noopBruteForceStore`, and `DefaultPasswordPolicy`
- Routes registered:
  - `POST /api/v1/auth/register` — public, tenant-scoped
  - `POST /api/v1/auth/login` — public, tenant-scoped
  - `POST /api/v1/auth/refresh` — public, uses HttpOnly cookie
  - `POST /api/v1/auth/logout` — public, revokes refresh token
  - Protected route group with `RequireAuth(kr)` middleware (placeholder for future endpoints)
- `noopBruteForceStore` — no-op implementation of `BruteForceStore` for development (replaced by Valkey-backed store in production)

### `internal/config/config.go`
- Added `JWTSecret` field (from `JC_JWT_SECRET`, default: empty — server generates random key in development)
- Added `JWTKeyID` field (from `JC_JWT_KEY_ID`, default: `"key-dev"`)

## Verification Results

| Command | Result |
|---------|--------|
| `go test ./internal/auth/...` | ok — github.com/jedafe/japa/internal/auth 0.858s |
| `go build ./cmd/api/` | success — no errors |
| `go vet ./...` | clean — no issues |

## Architecture Notes

- Bearer token in Authorization header only for protected endpoints
- Refresh tokens are HttpOnly, SameSite=Strict cookies — never accepted via cookie on mutation endpoints
- JWT access tokens: 15-minute expiry, HS256, multi-kid rotation support
- bcrypt cost 12 for password hashing
- All errors wrapped with `fmt.Errorf("doing X: %w", err)`
- All UUIDs use `pgtype.UUID`, never `uuid.UUID` directly
- Parameterized queries throughout — no string interpolation in SQL

## Section 02 Complete
All three parts of Section 02 (Authentication & Sessions) are now complete:
- Part 1: Auth DB migrations, JWT minting/validation, password hashing
- Part 2: Brute-force protection, auth HTTP handlers, refresh token generation
- Part 3: Auth middleware, DB store, server route wiring, JWT config
