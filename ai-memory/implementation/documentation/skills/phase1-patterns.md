# Phase 1 Implementation Skills

**Date:** 2026-06-25
**Phase:** 1
**Purpose:** Implementation patterns learned during Phase 1, reusable in future phases.

## Pattern: Tenant Middleware

All API routes under `/api/v1` go through `tenant.Middleware` which resolves the
tenant from the Host header and falls back to the default tenant. Health and
readiness routes bypass this middleware entirely.

```go
// Tenant is injected into request context by the middleware
tenantID := tenant.IDFromContext(ctx) // returns pgtype.UUID

// All scoped queries must filter by tenant_id
// sqlc generated params always include TenantID
```

## Pattern: JWT Multi-Kid Rotation

`KeyRotation` holds multiple signing secrets identified by a `kid` header.
Rotate keys with zero disruption:

1. Add the new key with `AddKey(kid, secret)`
2. Set it as current with `SetCurrent(kid)`
3. Wait one TTL (15 minutes) so all outstanding tokens expire
4. Remove the old key with `RemoveKey(kid)`

No mass logout required. The `ValidateAccessToken` function checks the `kid`
header to select the correct key, so tokens signed by the old key remain valid
until they expire.

## Pattern: Bearer Token Auth

`RequireAuth` middleware extracts a Bearer token from the Authorization header.
Never accept auth via cookie for mutation endpoints. `OptionalAuth` allows
anonymous access but populates user context if a valid token is present.

```go
// Protected route - Bearer token required
r.With(auth.RequireAuth(kr)).Get("/api/v1/me", meHandler)

// Public route - token optional, user populated if present
r.With(auth.OptionalAuth(kr)).Get("/api/v1/feed", feedHandler)
```

## Pattern: Refresh Token Security

Refresh tokens are stored as SHA-256 hashes (never raw). Cookies are HttpOnly,
SameSite=Strict, Secure, and path-scoped to `/api/v1/auth`. TTL is 30 days.
Tokens are revoked on logout.

```go
// Token is hashed before storage
hash := sha256.Sum256([]byte(rawToken))
store.StoreRefreshToken(ctx, hex.EncodeToString(hash[:]))

// Cookie is never accessible to JavaScript
http.SetCookie(w, &http.Cookie{
    Name:     "jc_refresh",
    Value:    rawToken,
    HttpOnly: true,
    SameSite: http.SameSiteStrictMode,
    Secure:   true,
    Path:     "/api/v1/auth",
    MaxAge:   30 * 24 * 60 * 60,
})
```

## Pattern: Brute-Force Protection

Exponential backoff keyed by account AND IP independently. Either dimension
can trigger lockout. The store fails open on infrastructure errors (do not
lock users out because Valkey is down). Parameters:

- 5 attempts threshold
- `2^(n-4)` seconds backoff after threshold
- 30-minute cap on lockout duration

## Pattern: Dynamic RBAC Resolution

`HasPermission` resolves through a 6-table JOIN:
`user_group_memberships -> user_groups -> group_roles -> user_roles -> role_permissions -> permissions`.
A per-request cache avoids repeated lookups within the same request.

Never hardcode a role name. Always resolve through the permission system:

```go
// WRONG - hardcoded role check
if user.Role == "editor" { allowPublish() }

// RIGHT - permission node resolved at runtime
if rbac.HasPermission(ctx, userID, tenantID, "blog.publish") { allowPublish() }
```

Permission node naming convention: `{domain}.{action}`
(e.g. `content.page_builder.create`, `ai.editor.assist`).

## Pattern: Feature Flag Gate

`Gate` middleware returns **404** (not 403) for disabled features. This is a
deliberate privacy choice: third parties should not discover which features a
tenant has disabled. Flags are stored in the database (not environment
variables) so they can be toggled without a server restart.

```go
r.With(features.Gate(checker, "blog.comments")).Post("/api/v1/comments", postComment)
```

## Pattern: GeoIP Graceful Degradation

The server starts and works fine without a `.mmdb` file. Lookups return
`IsValid=false`. Downstream handlers must check `IsValid` before using geo
data.

```go
geo := geoip.FromContext(ctx)
if geo.IsValid {
    // use geo.CountryCode, geo.City, etc.
}
```

## Pattern: Atomic Pointer Swap

GeoIP database updates use `atomic.Pointer` to swap the reader without a
server restart. The old reader is closed after the swap completes.

```go
// New database loaded, swap atomically
old := r.db.Swap(newDB)
if old != nil {
    old.Close()
}
```

## Pattern: SvelteKit App Setup

- SSR apps use `adapter-node`; SPA apps use `adapter-static` with
  `fallback: 'index.html'`
- Tailwind v4 is integrated via the `@tailwindcss/vite` plugin
- `resolve.conditions: ['browser']` is needed in `vite.config.ts` for
  Svelte 5 + vitest to work correctly
- Test files must be named `page.test.ts` (not `+page.test.ts`) because
  SvelteKit reserves the `+` prefix for route files -- using it in test
  filenames causes sync errors
- Svelte 5 runes: `$state`, `$derived`, `$effect`, `$props` (not Svelte 4
  patterns like stores or `on:event`)

## Pattern: Test Runner Selection

Go tests use the standard `go test ./...`. SvelteKit component tests use
`bun run test` which executes `vitest run` in a jsdom environment. Do not use
`bun test` for component tests because it lacks DOM support.

## Pattern: Custom Recovery Middleware

Chi v5.3.0's built-in `Recoverer` writes panic stack traces to a package-level
unexported variable that captures `os.Stderr` at init time, which means
redirects of `os.Stderr` in tests have no effect. The fix: a custom
`recoveryMiddleware` using `slog` for structured logging, returning JSON error
responses following the JapaConnect API error format. In tests, silence slog
output with `slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))`.

## Pattern: pgvector Extension Naming

In the `pgvector/pgvector:pg18` Docker image, the PostgreSQL extension is
registered as `vector`, not `pgvector`. Always use:

```sql
CREATE EXTENSION IF NOT EXISTS "vector";
```

Not `"pgvector"` — that will fail with "extension not found".

## Pattern: PostgreSQL Data Directory Mount

The `pgvector/pgvector:pg18` image expects the data directory at
`/var/lib/postgresql`, not `/var/lib/postgresql/data`. Use:

```yaml
volumes:
  - postgres-data:/var/lib/postgresql
```

If you mount to the wrong path, PostgreSQL exits immediately with code 1 and
the logs show `LOG: could not open file "/var/lib/postgresql/data/PG_VERSION"`.

## Pattern: Container Port Mapping

Docker Compose port mapping is `HOST:CONTAINER`. PostgreSQL always listens on
port 5432 inside its container regardless of what you map externally:

```yaml
ports:
  - "5433:5432"  # host:5433 -> container:5432
```

Common mistake: `5433:5433` — this tries to connect to port 5433 inside the
container, which nothing listens on.

## Pattern: .env Files and Go

Go's `os.Getenv()` does **not** read `.env` files. For local development:

- Docker Compose reads `.env` from the project root automatically
- Go requires `source .env` or `export` before `go run`
- Alternative: use a library like `joho/godotenv` in `main.go` for local dev

The `.env` file must use the same variable names as `os.Getenv()` calls in
the Go code (e.g., `JC_DATABASE_URL`, `JC_REDIS_ADDR`).

## Pattern: imgproxy Local Development

imgproxy requires `IMGPROXY_KEY` and `IMGPROXY_SALT` to start. For local
development, hardcode defaults in `docker-compose.yml`:

```yaml
environment:
  IMGPROXY_KEY: development-key
  IMGPROXY_SALT: development-salt
```

Do not reference shell variables like `${JC_IMGPROXY_KEY:-default}` — Docker
Compose resolves these from the host environment, not from Go's config. The
`.env` file is read by Docker Compose, but only if the variable names match
and the file exists in the project root.
