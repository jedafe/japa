# Phase 2 Implementation Patterns

**Date:** 2026-06-25
**Phase:** 2
**Purpose:** Implementation patterns learned during Phase 2, reusable in future phases.

## Pattern: sqlc Code Generation

sqlc generates type-safe Go code from SQL query files, eliminating hand-written
query boilerplate and preventing SQL injection by construction.

Configuration (`sqlc.yaml`):

```yaml
version: "2"
sql:
  - engine: "postgresql"
    queries: "internal/db/queries/"
    schema: "migrations/"
    gen:
      go:
        package: "gen"           # not "generated" — avoids import conflicts
        out: "internal/db/gen"
        sql_package: "pgx/v5"
        emit_pointers_for_null_types: true
        emit_json_tags: true
```

The generated `DBTX` interface is satisfied by `*pgxpool.Pool`, so the existing
reader/writer pool split works without modification:

```go
type Service struct {
    read  *gen.Queries  // backed by db.Reader
    write *gen.Queries  // backed by db.Writer
}

func NewService(database *db.DB) *Service {
    return &Service{
        read:  gen.New(database.Reader),
        write: gen.New(database.Writer),
    }
}
```

Query files live in `internal/db/queries/` with the `-- name: QueryName :many|:one|:exec`
annotation. Generated code lands in `internal/db/gen/` and should never be
hand-edited — regenerate with `sqlc generate`.

Nullable parameters use `sqlc.narg()` so the generated Go type is a pointer
(`*string`, `*pgtype.UUID`) rather than a bare value, allowing the caller to
pass `nil` for "no filter":

```sql
-- name: CreateAssignment :one
INSERT INTO custom_field_group_assignments (group_id, entity_class, entity_id, position)
VALUES (@group_id, @entity_class, sqlc.narg(entity_id), @position)
ON CONFLICT (group_id, entity_class, entity_id) DO NOTHING
RETURNING *;
```

## Pattern: Cursor Pagination with sqlc

Tuple comparison `(created_at, id) < (cursor_created_at, cursor_id)` causes a
sqlc type-inference bug: the cursor params are inferred as `interface{}` rather
than the correct `pgtype` types. The fix is to decompose the tuple into scalar
comparisons with an `IS NULL` guard for the first page (no cursor):

```sql
-- name: ListFormSubmissionsByForm :many
SELECT id, form_id, tenant_id, user_id, ip_address, field_values, status, created_at
FROM form_submissions
WHERE form_id = @form_id AND tenant_id = @tenant_id
  AND (sqlc.narg(cursor_created_at) IS NULL
       OR created_at < sqlc.narg(cursor_created_at)
       OR (created_at = sqlc.narg(cursor_created_at) AND id < sqlc.narg(cursor_id)))
ORDER BY created_at DESC, id DESC
LIMIT @page_limit;
```

The `sqlc.narg()` wrapper makes both cursor parameters nullable. When the caller
passes nil for both, the `IS NULL` branch is true and all rows are returned
(first page). On subsequent pages, the caller passes the last row's
`created_at` and `id` as the cursor.

The handler parses the cursor from a query string and builds the params:

```go
params := gen.ListFormSubmissionsByFormParams{
    FormID:    formID,
    TenantID:  tenantID,
    PageLimit: limit,
}
if cursor := r.URL.Query().Get("cursor"); cursor != "" {
    if t, err := time.Parse(time.RFC3339, cursor); err == nil {
        params.CursorCreatedAt = pgtype.Timestamptz{Time: t, Valid: true}
    }
}
```

Known limitation: sqlc may still infer the cursor param types imperfectly. pgx
handles encoding correctly at runtime by inspecting the concrete type, so this
is functionally correct but worth revisiting when sqlc releases a fix.

## Pattern: Valkey Client Graceful Degradation

Two constructors serve two needs. `New` fails fast when the server is
unreachable (use when the dependency is mandatory). `NewWithFallback` returns a
non-nil client even when the server is down, following the same
graceful-degradation model as the GeoIP reader in `internal/geoip`:

```go
// Fail-fast: caller decides whether to abort
client, err := valkey.New(ctx, cfg.RedisAddr)
if err != nil {
    log.Fatal(err)
}

// Graceful: server boots, individual operations fail until Valkey comes back
client := valkey.NewWithFallback(cfg.RedisAddr)
_ = client // retained for process lifetime, wired later
```

`NewWithFallback` sets a 3-second dial timeout so an absent server does not
stall startup. Subsequent commands fail fast and callers treat those failures
as cache misses. `Close()` is safe to call on a client whose server was never
reachable, and is idempotent.

All keys stored in Valkey must be namespaced with the `jc:` prefix (e.g.
`jc:cache:`, `jc:ratelimit:`, `jc:editlock:`). The package documents this
convention but does not enforce it — callers are responsible for key naming.

## Pattern: Platform Embed Resolver

The embed resolver identifies a platform from a URL's hostname (stripping the
`www.` prefix first), then dispatches to one of three strategies:

1. **Pure URL parsing** (no HTTP): GitHub, Figma, Google Maps
2. **oEmbed only** (hard-fail on error): Twitter/X, Vimeo, Spotify, CodePen
3. **Hybrid** (iframe + oEmbed enrichment, soft-fail): YouTube

```go
host := strings.TrimPrefix(parsed.Hostname(), "www.")
platform := identifyPlatform(host, parsed.Path)

switch platform {
case platformYouTube:
    return r.resolveYouTube(ctx, rawURL, parsed)      // hybrid
case platformGitHub:
    return resolveGitHub(rawURL, parsed), nil          // pure URL
case platformTwitter, platformVimeo, platformSpotify, platformCodePen:
    return r.resolveOEmbedPlatform(ctx, platform, rawURL) // oEmbed
case platformFigma:
    return resolveFigma(rawURL), nil                   // pure URL
case platformGoogleMaps:
    return resolveGoogleMaps(rawURL), nil              // pure URL
}
```

YouTube builds the iframe embed URL from the video ID even when the oEmbed call
fails — the video remains watchable, only metadata (title, author, thumbnail)
is absent. This is a deliberate soft-fail: log the oEmbed error and return the
partial embed.

For the other oEmbed platforms (Twitter, Vimeo, Spotify, CodePen), there is no
fallback representation. An oEmbed failure returns a hard error.

Testing uses `httptest.NewServer` with an unexported
`newEmbedResolverWithOverrides` constructor that replaces the real oEmbed
endpoints with the mock server URL. URL-parsed platforms (GitHub, Figma, Maps)
are verified to make zero HTTP calls by pointing their oEmbed endpoints at a
server that fails the test if contacted.

The resolver accepts an `allowedPlatforms` filter. When non-empty, only
platforms in the slice are accepted — any other recognized platform returns
`ErrUnsupportedPlatform`. This lets a field definition restrict which platforms
its `platform_embed` values may use.

## Pattern: Service Reader/Writer Pool Split

The `Service` struct holds two `*gen.Queries` instances: one reading from the
reader pool, one writing to the writer pool. This follows the existing
`db.DB` pattern used throughout the codebase:

```go
type Service struct {
    read  *gen.Queries
    write *gen.Queries
}

func NewService(database *db.DB) *Service {
    return &Service{
        read:  gen.New(database.Reader),
        write: gen.New(database.Writer),
    }
}
```

Read methods (List, Get) call `s.read`. Write methods (Create, Update, Delete,
Upsert) call `s.write`. All errors are wrapped with `fmt.Errorf("verb noun: %w", err)`
for context without obscuring the original error for `errors.Is` checks.

## Pattern: Permission Bootstrap at Startup

The `bootstrapPermissions` function registers permission keys and wires the
super_admin role at server startup. Every operation is idempotent so repeated
restarts are safe:

```go
func bootstrapPermissions(ctx context.Context, database *db.DB, rbacStore *rbac.Store) {
    // 1. Register each permission key (idempotent upsert)
    for _, perm := range permissions {
        rbacStore.RegisterPermission(ctx, perm.Key, perm.Description)
    }

    // 2. Create super_admin_role (ON CONFLICT DO NOTHING)
    database.Writer.Exec(ctx, `
        INSERT INTO user_roles (tenant_id, name, slug)
        VALUES ($1, 'Super Admin Role', 'super_admin_role')
        ON CONFLICT (tenant_id, slug) DO NOTHING
    `, defaultTenantID)

    // 3. Resolve role and group IDs (INSERT may have been a no-op)
    // 4. Assign every permission to the role
    // 5. Grant the role to the super_admin group
}
```

Failures are logged via `slog.Error` but never crash the server. If the database
is temporarily unhealthy at startup, individual permission checks will simply
fail (return false) until the next restart succeeds. The 10-second context
timeout prevents the bootstrap from hanging the server if the database is slow.

## Pattern: RBAC-Gated Route Registration

Three middleware composition patterns cover every route type:

```go
// Permission-gated route: auth required + specific permission
r.With(auth.RequireAuth(kr), rbac.RequirePermission(resolver, "custom_fields.group.create")).
    Post("/custom-field-groups", cfHandlers.CreateFieldGroup)

// Auth-only route: authenticated user, no specific permission
r.With(auth.RequireAuth(kr)).Get("/custom-field-groups", cfHandlers.ListFieldGroups)

// Public route with optional auth: anonymous accepted, user recorded if present
r.With(auth.OptionalAuth(kr)).Post("/forms/{id}/submissions", cfHandlers.SubmitForm)
```

`RequireAuth` returns 401 for missing/invalid tokens. `RequirePermission`
returns 403 when the authenticated user lacks the permission key (resolved
through the dynamic RBAC six-table JOIN). Public routes with `OptionalAuth`
populate the user context when a valid token is present but do not require one.

## Pattern: Space Resolver Middleware

Routes that operate on a specific space identified by slug (rather than UUID)
run `SpaceResolver` middleware before the handler. It resolves the `{slug}`
URL parameter to a fully loaded `gen.Space` and stores it in the request
context under `SpaceContextKey`. Handlers then retrieve it via
`spaces.FromContext` (or the package-internal `spaceFromContext` helper that
also writes a 404 when no space is present):

```go
r.With(auth.RequireAuth(kr), spaces.SpaceResolver(spaceService)).
    Get("/spaces/{slug}", spaceHandlers.GetSpace)
```

The middleware runs *after* tenant and auth middleware so it can resolve the
tenant from context and the authenticated user from the bearer token. On
failure it short-circuits with the JapaConnect error envelope: 404 on
`pgx.ErrNoRows` (`space_not_found`), 503 when no tenant is available
(`tenant_unavailable`), 500 on any other error. Passing a resolved space
through context — rather than re-resolving in every handler — keeps slug
routes to a single SQL lookup per request and gives handlers a guaranteed
non-nil pointer to lean on.

## Pattern: Space Permission Axes Validation

A space exposes five admin-configurable permission axes: `visibility`,
`posting`, `replying`, `commenting`, and `joining`. Each axis accepts a fixed
set of constant values defined once in `internal/spaces/spaces.go`. Validation
happens at the handler boundary via `ValidPermissionAxes`, which checks each
axis against its valid constant set (the posting/replying/commenting axes share
the same set):

```go
axes := SpacePermAxes{
    Visibility:  req.Visibility,
    Posting:     req.PostingPerm,
    Replying:    req.ReplyingPerm,
    Commenting:  req.CommentingPerm,
    Joining:     req.JoiningPerm,
}
if !ValidPermissionAxes(axes) {
    writeError(w, http.StatusBadRequest, "invalid_permission_axes",
        "One or more permission axes are invalid")
    return
}
```

`CreateSpace` resolves axes from a named permission preset when one is supplied,
falling back to `DefaultPermAxes()` (public / members / members / members /
open) otherwise. `UpdateSpace` follows fetch-then-overlay PATCH semantics:
each provided axis is validated individually and overlaid on the existing
space, leaving unspecified fields at their current values. Never hardcode axis
strings — always compare against the package-level constants
(`VisibilityPublic`, `PermMembers`, `JoiningOpen`, etc.).

## Pattern: Valkey Unread Counter with Graceful Degradation

The `UnreadCounter` tracks per-user per-space unread post counts in Valkey.
The key format is `jc:space_unread:{user_id}:{space_id}` → integer. The
counter accepts a `nil` Valkey client (which can happen during local dev when
no server is running) and degrades to safe no-op defaults for every method:

```go
func NewUnreadCounter(vk *valkey.Client) *UnreadCounter {
    if vk == nil {
        return &UnreadCounter{rdb: nil}
    }
    return &UnreadCounter{rdb: vk.Redis()}
}
```

- `Increment` → `INCR` (silently fails if Valkey is unreachable; unread
  counts are a nice-to-have, never critical path)
- `Reset` → `SET 0` (no TTL), called when a user visits a space
- `Get` → `GET`, returns 0 on error including `redis.Nil` (key absent means
  zero — no distinguishability between "unread 0" and "no counter yet")
- `GetBatch` → `MGET` for multiple spaces, returns `map[string]int` keyed by
  the stringified space ID; missing or errored entries default to 0

This pattern intentionally drops unread-count errors — the caller never sees
them. If a hard outage drops counter updates, the worst case is stale counts,
not a broken UX. The critical-path write (new post) succeeds independently.

## Pattern: Stubbed Asynq Task Interface

Space auto-population (fetching metadata from Wikipedia, flagcdn.com, Pexels)
is a background job that should be enqueued on space creation. Asynq is not
wired until Phase 6 (Section 11 — Job Queue), so the spaces package exposes a
`TaskEnqueuer` interface and ships a `noopEnqueuer` implementation that logs
the enqueue request via `slog.Info` and returns nil:

```go
type TaskEnqueuer interface {
    EnqueueSpaceAutoPopulate(ctx context.Context, spaceID pgtype.UUID) error
}

func NewNoopEnqueuer() TaskEnqueuer { return &noopEnqueuer{} }
```

The interface lets other packages call `EnqueueSpaceAutoPopulate` today
without knowing whether the producer is real or stubbed. Swapping to a real
Asynq producer in Phase 6 is a one-line change at the construction site — no
handler or service code needs to know. New task types follow the same shape:
define a `TaskType*` string constant, add a method to the interface, implement
it on the noop producer, and replace the producer when the queue is wired.
