# User Public Profile Backend Implementation Plan (Phase A)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. This is a **JapaConnect Go backend** plan — follow `jc-backend-develop` conventions (layered Handler→Service→Store, sqlc-only SQL, RenderJSON contract). Run `jc-backend-migrate` for the one schema-touching item (none in Phase A — all queries target existing tables).

**Goal:** Make the public user profile surface (`/u/[username]`) fully serveable by the backend — a lean, PII-safe profile header plus the five profile tabs (Posts · Replies · Media · Likes · Spaces) and public certificate badges — so the frontend `jc-main-develop` API-Readiness Gate passes.

**Architecture:** All routes are **public reads on the existing `users` entity** (which already has full `/admin/users/*` CRUD + RBAC nodes), so **no new RBAC permission nodes are required**. We fix two spec-conformance defects (the profile header and feed are wrongly auth-gated and the header leaks PII), reuse existing sqlc queries for three tabs (Media/Spaces/Certificates), and add two new author-scoped queries (Likes, Replies). Username→id resolution: the frontend calls `GET /users/{username}` to get the header (incl. `id`), then hits `/users/{id}/*` for tabs — matching the existing catalog. Behavioral correctness (tenancy, counts, scoping) is verified by **real-Postgres integration tests** via a new shared harness (Task A0, ADR-061), because services take a concrete `*db.DB` (no mock seam).

**Tech Stack:** Go (Chi router) · sqlc + pgx/v5 · PostgreSQL 18 · `OptionalAuth` middleware (`internal/auth/middleware.go:88`) for public-but-personalizable reads · `slog` · integration tests behind `//go:build integration`, run via `make test-integration`.

## Global Constraints

- **Layered architecture, never skip layers:** Handler parses/validates + calls ONE service method; Service holds logic; Store is sqlc-generated only. No hand-concatenated SQL.
- **Response contract:** success `RenderJSON(w, status, data)`; lists `RenderPaginated(w, data, meta{total,page,limit})`; errors `RenderError(w, status, code, message)`. Timestamps RFC 3339 UTC. IDs as strings in JSON.
- **Tenancy (mandatory):** every store query filters by `tenant_id` from context. (Task A6 fixes a query that violates this.)
- **PII rule (Section 19 / ADR-014):** the public profile response MUST NOT expose `email`, `status`, `last_login_at`, or `groups`.
- **Anonymity rule (Section 08):** anonymous authorship must never be exposed via profile activity. (Phase A Replies tab uses the `comments` table, which has no anonymity flag; the rule is noted for the deferred forum-replies union.)
- **Public visibility:** profile reads only surface `status='active'` users and `status='published' AND visibility='public'` content.
- **sqlc regen:** after editing any `.sql` file, run `sqlc generate` from repo root before building.
- **Verification:** `go build ./... && go vet ./...` clean AND `make test-integration` green at the end of every task (run locally).
- **Catalogs:** `.jeos/conventions/api-routes-catalog.md` updated in the same change set (Task A8). An uncatalogued route is invisible to the frontend gates.

**Governing specs:** Section 23:47 (profile tabs), Section 31 + ADR-031 (follow counts), Section 32:12 (profile timeline audience = "Anyone"), Section 50:179 (certs-as-badges), Section 19 + ADR-014 (PII/erasure), Section 08 (anonymity exclusion), ADR-061 (integration test harness).

## Integration Test Pattern (binds Tasks A1–A8)

All behavioral tests are **real-Postgres integration tests** using the harness from Task A0. Each behavioral test file starts with `//go:build integration` and follows this exact skeleton. **Where a task's Step 1 below references a `newTestXxx(t)` / `withURLParam` helper, those were pre-harness placeholder names — use this pattern instead.** Pure JSON-tag/DTO tests (no DB) stay untagged and use plain struct literals (see the existing `TestUserProfileResponseJSONTags`).

```go
//go:build integration

package users_test

import (
	"context"
	"testing"

	"github.com/jedafe/japa/internal/testdb"
	"github.com/jedafe/japa/internal/users"
)

func TestGetPublicProfileByUsername_OmitsPII(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	database := testdb.Connect(t)                 // migrated japa_test pool (once/process)
	tenantID := testdb.NewTenant(t, database)     // fresh tenant = isolation boundary
	uid := testdb.SeedUser(t, database, tenantID, testdb.UserOpts{Username: "ada"})
	testdb.SeedFollowers(t, database, tenantID, uid, 3)  // 3 users follow ada
	testdb.SeedFollowing(t, database, tenantID, uid, 2)  // ada follows 2

	svc := users.NewService(database)             // real service on the test pool
	got, err := svc.GetPublicProfileByUsername(ctx, tenantID, "ada")
	// ...assert...
}
```

**Harness API (defined in Task A0, all `//go:build integration`):**
- `testdb.Connect(t *testing.T) *db.DB`
- `testdb.NewTenant(t *testing.T, d *db.DB) pgtype.UUID`
- `testdb.SeedUser(t, d, tenantID, testdb.UserOpts{Username, DisplayName, Status}) pgtype.UUID` (Status defaults `"active"`)
- `testdb.SeedFollowers(t, d, tenantID, entityUserID, n int)` / `SeedFollowing(t, d, tenantID, followerID, n int)`
- `testdb.SeedContent(t, d, tenantID, authorID, testdb.ContentOpts{Status, Visibility}) pgtype.UUID` (defaults `published`/`public`)
- `testdb.SeedComment(t, d, tenantID, authorID, contentID, body string) pgtype.UUID`
- `testdb.SeedReaction(t, d, tenantID, userID, entityType string, entityID pgtype.UUID, key string)`
- `testdb.SeedMedia(t, d, tenantID, uploaderID pgtype.UUID) pgtype.UUID`
- `testdb.SeedSpaceMembership(t, d, tenantID, userID pgtype.UUID) pgtype.UUID` (returns spaceID)
- `testdb.SeedCertificate(t, d, tenantID, userID pgtype.UUID) pgtype.UUID`

Handler-level tests construct the handler on the real service and drive it with `httptest`, injecting tenant/user/URL-params via the **existing** context helpers in the target package (e.g. `withTenant` in `internal/users/me_handlers_test.go`); add a chi URL-param helper in the harness if the package lacks one.

**Deferred (out of Phase A scope, documented for follow-on):**
- **User verification badges** (Section 07 / ADR-029): no `verified`/`verification_type` columns on `users`, no admin verify endpoint, no RBAC node. Needs its own migration + admin surface. Org verification already exists (Phase B).
- **Forum replies** in the Replies tab (would need the forum-reply table + `is_anonymous=false` filter).
- **Profile DTO Valkey caching** (avoid the known unexported-field cache serialization bug; ship uncached first).

---

### Task A0: Integration test harness (`internal/testdb`)

Establish the shared real-Postgres harness (ADR-061) that every later task's behavioral tests depend on. All files carry `//go:build integration`. Reuses the migrator's forward-only apply logic (skip `.down.sql`, AUDIT-BE-003). This is an **integration/judgment task — dispatch on a capable model.**

**Files:**
- Create: `internal/testdb/testdb.go` (`Connect`, `NewTenant`, migration apply once per process)
- Create: `internal/testdb/seed.go` (typed seed helpers + `UserOpts`/`ContentOpts`)
- Create: `internal/testdb/httptest.go` (chi URL-param + context helpers reused across packages)
- Create: `internal/testdb/testdb_smoke_test.go` (proves the harness itself works)
- Modify: `.jeos/sections/27-devops-testing-qa.md` (document tag + `japa_test` DB + per-tenant isolation)
- Reference (do not duplicate — extract shared logic if practical): `cmd/migrate/main.go:39-204` (`runMigrations`, `listMigrationFiles`, `applyMigration`).

**Interfaces:**
- Produces the full `testdb.*` API listed in "Integration Test Pattern" above. Exact signatures:
  - `func Connect(t *testing.T) *db.DB`
  - `func NewTenant(t *testing.T, d *db.DB) pgtype.UUID`
  - `type UserOpts struct { Username, DisplayName, Status string }`
  - `func SeedUser(t *testing.T, d *db.DB, tenantID pgtype.UUID, o UserOpts) pgtype.UUID`
  - `func SeedFollowers(t *testing.T, d *db.DB, tenantID, entityUserID pgtype.UUID, n int)`
  - `func SeedFollowing(t *testing.T, d *db.DB, tenantID, followerID pgtype.UUID, n int)`
  - `type ContentOpts struct { Title, Status, Visibility string }`
  - `func SeedContent(t *testing.T, d *db.DB, tenantID, authorID pgtype.UUID, o ContentOpts) pgtype.UUID`
  - `func SeedComment(t *testing.T, d *db.DB, tenantID, authorID, contentID pgtype.UUID, body string) pgtype.UUID`
  - `func SeedReaction(t *testing.T, d *db.DB, tenantID, userID pgtype.UUID, entityType string, entityID pgtype.UUID, key string)`
  - `func SeedMedia(t *testing.T, d *db.DB, tenantID, uploaderID pgtype.UUID) pgtype.UUID`
  - `func SeedSpaceMembership(t *testing.T, d *db.DB, tenantID, userID pgtype.UUID) pgtype.UUID`
  - `func SeedCertificate(t *testing.T, d *db.DB, tenantID, userID pgtype.UUID) pgtype.UUID`
  - `func WithChiURLParam(r *http.Request, key, val string) *http.Request`

- [ ] **Step 1: Write the harness smoke test first** (`internal/testdb/testdb_smoke_test.go`)

```go
//go:build integration

package testdb_test

import (
	"testing"

	"github.com/jedafe/japa/internal/testdb"
)

func TestHarness_ConnectAndIsolateByTenant(t *testing.T) {
	t.Parallel()
	d := testdb.Connect(t)
	tenantA := testdb.NewTenant(t, d)
	tenantB := testdb.NewTenant(t, d)
	if tenantA == tenantB {
		t.Fatal("NewTenant returned duplicate tenant ids")
	}
	// Seeding under A must not be visible under B (proves isolation boundary).
	uid := testdb.SeedUser(t, d, tenantA, testdb.UserOpts{Username: "iso_probe"})
	if !uid.Valid {
		t.Fatal("SeedUser returned invalid uuid")
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `make test-integration` (or `go test -tags integration ./internal/testdb/...`)
Expected: FAIL — `internal/testdb` package does not exist / undefined symbols.

- [ ] **Step 3: Ensure the `japa_test` database exists**

Run (one-time, safe to re-run):
```bash
psql "postgresql://japa:japa@localhost:5432/postgres?sslmode=disable" -tc "SELECT 1 FROM pg_database WHERE datname='japa_test'" | grep -q 1 || psql "postgresql://japa:japa@localhost:5432/postgres?sslmode=disable" -c "CREATE DATABASE japa_test"
```

- [ ] **Step 4: Implement `Connect` + migration apply** (`internal/testdb/testdb.go`)

Resolve DSN: `JC_TEST_DATABASE_URL` → `JC_DATABASE_URL` → `postgresql://japa:japa@localhost:5432/japa_test?sslmode=disable` (guard: if the resolved DSN's dbname is the primary `japa`, `t.Fatal` — never run destructive seeding against dev data). Use `sync.Once` to apply migrations once per process by walking `findMigrationsDir()` and applying each forward file (skip `.down.sql`) exactly as `cmd/migrate` does — extract the shared walk/apply into a small exported helper reused by both, or copy `listMigrationFiles`/`applyMigration` verbatim if extraction is impractical (note the duplication in your report so the final review can decide). Return `db.New(ctx, dsn)`; register `t.Cleanup(func(){ database.Close() })` — but keep the process pool alive across parallel tests (close in `TestMain` or leak intentionally with a comment; do NOT close a shared pool in per-test cleanup).

- [ ] **Step 5: Implement `NewTenant` + seed helpers** (`internal/testdb/testdb.go`, `seed.go`)

`NewTenant` inserts a `tenants` row (unique slug via uuid) plus the minimal FK scaffolding a seeded user needs (a default `user_groups` row for the tenant); return the tenant id. Seed helpers insert via raw `d.Writer.Exec` (not sqlc) so the harness never depends on query changes under test. Every seed helper `t.Fatal`s on error. `SeedFollowers`/`SeedFollowing` create `n` throwaway users and the corresponding `follows` rows (respecting the denormalized `follower_count` trigger if present; if counts are trigger-maintained, do not also hand-set them).

- [ ] **Step 6: Implement `WithChiURLParam`** (`internal/testdb/httptest.go`)

```go
func WithChiURLParam(r *http.Request, key, val string) *http.Request {
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add(key, val)
	return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
}
```

- [ ] **Step 7: Run the smoke test to verify it passes**

Run: `make test-integration` and `go build -tags integration ./... && go vet -tags integration ./...`
Expected: PASS, clean.

- [ ] **Step 8: Document + commit**

Add the harness note to `.jeos/sections/27-devops-testing-qa.md` (tag, `japa_test` DB, per-tenant isolation, `make test-integration`).
```bash
git add internal/testdb/ .jeos/sections/27-devops-testing-qa.md
git commit -m "test(harness): real-PG integration harness with per-tenant isolation (ADR-061)"
```

---

### Task A1: Lean, PII-safe public profile header (`GET /users/{username}`)

Replaces the leaky `UserProfileResponse` (exposes `email`, `status`, `last_login_at`, `groups`) with a lean public DTO, adds `following_count`, and restricts to active users. New sqlc query so the private full-profile query is untouched.

**Files:**
- Modify: `internal/db/queries/users.sql` (add `GetPublicProfileByUsername`)
- Modify: `internal/users/types.go` (add `PublicProfileResponse`)
- Modify: `internal/users/store.go` (add `getPublicProfileByUsername` wrapper)
- Modify: `internal/users/service.go` (add `GetPublicProfileByUsername` method)
- Modify: `internal/users/handlers.go:140-159` (repoint `GetUserByUsername` to the public service method)
- Test: `internal/users/users_test.go`

**Interfaces:**
- Produces: `Service.GetPublicProfileByUsername(ctx, tenantID pgtype.UUID, username string) (*PublicProfileResponse, error)`
- Produces DTO `PublicProfileResponse{ ID, Username string; DisplayName, AvatarURL, Bio *string; ProfileType string; FollowerCount, FollowingCount int32; CreatedAt string }`

- [ ] **Step 1: Write the failing test** (harness pattern; asserts no PII fields, counts correct)

```go
//go:build integration

package users_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/jedafe/japa/internal/testdb"
	"github.com/jedafe/japa/internal/users"
)

func TestGetPublicProfileByUsername_OmitsPII(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	d := testdb.Connect(t)
	tenantID := testdb.NewTenant(t, d)
	uid := testdb.SeedUser(t, d, tenantID, testdb.UserOpts{Username: "ada"})
	testdb.SeedFollowers(t, d, tenantID, uid, 3) // 3 users follow ada
	testdb.SeedFollowing(t, d, tenantID, uid, 2) // ada follows 2

	got, err := users.NewService(d).GetPublicProfileByUsername(ctx, tenantID, "ada")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got.Username != "ada" {
		t.Fatalf("username = %q, want ada", got.Username)
	}
	if got.FollowerCount != 3 || got.FollowingCount != 2 {
		t.Fatalf("counts = %d/%d, want 3/2", got.FollowerCount, got.FollowingCount)
	}
	b, _ := json.Marshal(got)
	for _, k := range []string{"email", "status", "last_login_at", "groups", "user_group_name"} {
		if string(b) != "" && contains(b, "\""+k+"\"") {
			t.Fatalf("public profile leaked field %q: %s", k, b)
		}
	}
}
```
(Also add a **non-DB** JSON-tag test `TestPublicProfileResponseJSONTags` in the untagged `users_test.go`, mirroring the existing `TestUserProfileResponseJSONTags`, asserting the struct serializes exactly the intended keys and no others — this catches PII regressions without a database.)

- [ ] **Step 2: Run test to verify it fails**

Run: `make test-integration` (runs `-tags integration`)
Expected: FAIL — `users.Service.GetPublicProfileByUsername` undefined.

- [ ] **Step 3: Add the sqlc query**

```sql
-- internal/db/queries/users.sql  (append)
-- name: GetPublicProfileByUsername :one
-- Public profile header for /u/[username]. NO PII (no email/status/last_login/groups).
SELECT DISTINCT ON (u.id)
       u.id AS user_id,
       u.username,
       u.profile_type,
       u.follower_count,
       up.display_name,
       up.avatar_url,
       up.bio,
       COALESCE(up.created_at, u.created_at) AS created_at,
       (SELECT COUNT(*)::int
          FROM follows f
         WHERE f.tenant_id = u.tenant_id AND f.follower_id = u.id) AS following_count
FROM users u
LEFT JOIN user_profiles up ON up.user_id = u.id AND up.tenant_id = u.tenant_id
WHERE u.tenant_id = @tenant_id
  AND u.username = @username
  AND u.status = 'active'
ORDER BY u.id;
```

- [ ] **Step 4: Regenerate sqlc**

Run: `sqlc generate`
Expected: `internal/db/gen/users.sql.go` gains `GetPublicProfileByUsername`, `GetPublicProfileByUsernameRow`, `GetPublicProfileByUsernameParams`.

- [ ] **Step 5: Add DTO** (`internal/users/types.go`)

```go
// PublicProfileResponse is the PII-safe API response for GET /users/{username}.
type PublicProfileResponse struct {
	ID             string  `json:"id"`
	Username       string  `json:"username"`
	DisplayName    *string `json:"display_name"`
	AvatarURL      *string `json:"avatar_url"`
	Bio            *string `json:"bio"`
	ProfileType    string  `json:"profile_type"`
	FollowerCount  int32   `json:"follower_count"`
	FollowingCount int32   `json:"following_count"`
	CreatedAt      string  `json:"created_at"`
}
```

- [ ] **Step 6: Add store wrapper** (`internal/users/store.go`, mirror the existing `getUserByUsername` wrapper)

```go
func (s *store) getPublicProfileByUsername(ctx context.Context, tenantID pgtype.UUID, username string) (gen.GetPublicProfileByUsernameRow, error) {
	return s.q.GetPublicProfileByUsername(ctx, gen.GetPublicProfileByUsernameParams{
		TenantID: tenantID,
		Username: username,
	})
}
```

- [ ] **Step 7: Add service method** (`internal/users/service.go`)

```go
func (s *Service) GetPublicProfileByUsername(ctx context.Context, tenantID pgtype.UUID, username string) (*PublicProfileResponse, error) {
	p, err := s.store.getPublicProfileByUsername(ctx, tenantID, username)
	if err != nil {
		return nil, fmt.Errorf("getting public profile by username: %w", err)
	}
	return &PublicProfileResponse{
		ID:             p.UserID.String(),
		Username:       p.Username,
		DisplayName:    p.DisplayName,
		AvatarURL:      p.AvatarUrl,
		Bio:            p.Bio,
		ProfileType:    p.ProfileType,
		FollowerCount:  p.FollowerCount,
		FollowingCount: p.FollowingCount,
		CreatedAt:      p.CreatedAt.Time.Format(time.RFC3339),
	}, nil
}
```

- [ ] **Step 8: Repoint the handler** (`internal/users/handlers.go:152`)

```go
	profile, err := h.Service.GetPublicProfileByUsername(r.Context(), tenantID, username)
```
(Leave the rest of `GetUserByUsername` unchanged; it already renders `{"data": profile}`.)

- [ ] **Step 9: Run tests to verify they pass**

Run: `go test ./internal/users/ -run TestGetPublicProfileByUsername -v && go build ./... && go vet ./...`
Expected: PASS, clean build/vet.

- [ ] **Step 10: Commit**

```bash
git add internal/db/queries/users.sql internal/db/gen/users.sql.go internal/users/
git commit -m "feat(users): lean PII-safe public profile header with following_count"
```

---

### Task A2: Make the profile header + feed publicly readable

Section 32:12 says the profile timeline audience is "Anyone." Both `GET /users/{username}` (server.go:1932) and `GET /feed/profile/{user_id}` (server.go:1155) are wrongly behind `RequireAuth`. Switch both to `OptionalAuth` and make `GetProfileFeed` tolerate a nil viewer.

**Files:**
- Modify: `internal/server/server.go:1155` and `:1932`
- Modify: `internal/feed/handlers.go:224` (`GetProfileFeed` — nil-viewer safe)
- Test: `internal/feed/handlers_test.go` (guest request)

**Interfaces:**
- Consumes: `auth.OptionalAuth(kr, vkClient.Redis())` — already exists at `internal/auth/middleware.go:88`.

- [ ] **Step 1: Write the failing test** — guest (no session) gets 200 from the profile feed

```go
// internal/feed/handlers_test.go
func TestGetProfileFeed_GuestAllowed(t *testing.T) {
	h, authorID := newTestFeedHandler(t) // existing helper pattern
	req := httptest.NewRequest(http.MethodGet, "/feed/profile/"+authorID, nil)
	req = withTenant(req)               // tenant in context, NO user in context
	rec := httptest.NewRecorder()

	h.GetProfileFeed(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("guest profile feed = %d, want 200; body=%s", rec.Code, rec.Body)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/feed/ -run TestGetProfileFeed_GuestAllowed -v`
Expected: FAIL — handler currently assumes an authed user (panics or 401).

- [ ] **Step 3: Make the handler nil-viewer safe** (`internal/feed/handlers.go`, inside `GetProfileFeed`)

Locate where it reads the viewer (e.g. `userID, _ := auth.UserFromContext(r.Context())`) and ensure the code path where no viewer is present still returns the public rows. Guard any viewer-dependent personalization:

```go
	viewerID, hasViewer := auth.UserIDFromContext(r.Context()) // ok == false for guests
	// ... build the feed from getProfileFeed (public rows only) ...
	if hasViewer {
		// optional: annotate is_following / has_reacted for the viewer
		_ = viewerID
	}
```
(Do not `return 401` when `hasViewer` is false.)

- [ ] **Step 4: Un-gate both routes** (`internal/server/server.go`)

Line ~1155:
```go
			r.With(auth.OptionalAuth(kr, vkClient.Redis())).Get("/feed/profile/{user_id}", feedHandlers.GetProfileFeed)
```
Line ~1932:
```go
			r.With(auth.OptionalAuth(kr, vkClient.Redis())).Get("/users/{username}", usersHandlers.GetUserByUsername)
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/feed/ ./internal/users/ -v && go build ./... && go vet ./...`
Expected: PASS, clean.

- [ ] **Step 6: Commit**

```bash
git add internal/server/server.go internal/feed/handlers.go internal/feed/handlers_test.go
git commit -m "fix(profile): make profile header + feed public (Section 32 audience=Anyone)"
```

---

### Task A3: Media tab (`GET /users/{id}/media`) — reuse `ListMediaByUploader`

**Files:**
- Modify: `internal/media/handlers.go` (add `ListByUser`)
- Modify: `internal/media/service.go` (add `ListByUser` if not present; wraps existing `ListMediaByUploader` + `CountMediaByUploader`)
- Modify: `internal/server/server.go` (register route, `OptionalAuth`)
- Test: `internal/media/handlers_test.go`

**Interfaces:**
- Consumes existing sqlc: `ListMediaByUploader`, `CountMediaByUploader`.
- Produces: `GET /users/{id}/media?page=&limit=` → `RenderPaginated`.

- [ ] **Step 1: Write the failing test**

```go
func TestListByUser_ReturnsUploaderMedia(t *testing.T) {
	h, uid := newTestMediaHandler(t)
	seedMedia(t, uid, 2)
	req := withTenant(httptest.NewRequest(http.MethodGet, "/users/"+uid+"/media", nil))
	req = withURLParam(req, "id", uid)
	rec := httptest.NewRecorder()

	h.ListByUser(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body)
	}
	var resp struct{ Meta struct{ Total int } `json:"meta"` }
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp.Meta.Total != 2 {
		t.Fatalf("total=%d want 2", resp.Meta.Total)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/media/ -run TestListByUser_ReturnsUploaderMedia -v`
Expected: FAIL — `ListByUser` undefined.

- [ ] **Step 3: Add service method** (`internal/media/service.go`) wrapping the existing queries, tenant-scoped, offset pagination via `parsePageSize`/`parsePageOffset` helpers.

```go
func (s *Service) ListByUser(ctx context.Context, tenantID, uploaderID pgtype.UUID, limit, offset int32) ([]MediaItem, int32, error) {
	rows, err := s.store.listMediaByUploader(ctx, tenantID, uploaderID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("listing media by uploader: %w", err)
	}
	total, err := s.store.countMediaByUploader(ctx, tenantID, uploaderID)
	if err != nil {
		return nil, 0, fmt.Errorf("counting media by uploader: %w", err)
	}
	return toMediaItems(rows), total, nil
}
```
(If `store.listMediaByUploader`/`countMediaByUploader` wrappers don't exist yet, add them mirroring existing wrappers. Confirm `ListMediaByUploader` params include `tenant_id`; if not, treat as a tenancy fix like Task A6.)

- [ ] **Step 4: Add handler** (`internal/media/handlers.go`)

```go
func (h *Handlers) ListByUser(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	var uid pgtype.UUID
	if err := uid.Scan(chi.URLParam(r, "id")); err != nil {
		RenderError(w, http.StatusBadRequest, "invalid_user_id", "Invalid user id")
		return
	}
	limit, offset := parsePageSize(r), parsePageOffset(r)
	items, total, err := h.Service.ListByUser(r.Context(), tenantID, uid, limit, offset)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	RenderPaginated(w, items, meta(total, limit, offset))
}
```

- [ ] **Step 5: Register route** (`internal/server/server.go`, near the other `/users/{id}/*` routes)

```go
			r.With(auth.OptionalAuth(kr, vkClient.Redis())).Get("/users/{id}/media", mediaHandlers.ListByUser)
```

- [ ] **Step 6: Run tests to verify they pass**

Run: `go test ./internal/media/ -v && go build ./... && go vet ./...`
Expected: PASS, clean.

- [ ] **Step 7: Commit**

```bash
git add internal/media/ internal/server/server.go
git commit -m "feat(profile): GET /users/{id}/media tab (reuse ListMediaByUploader)"
```

---

### Task A4: Spaces tab (`GET /users/{id}/spaces`) — reuse `ListSpacesByMemberUser`

**Files:**
- Modify: `internal/spaces/handlers.go` (add `ListByMemberUser`)
- Modify: `internal/spaces/service.go` (wrap `ListSpacesByMemberUser`)
- Modify: `internal/server/server.go` (register route)
- Test: `internal/spaces/handlers_test.go`

**Interfaces:**
- Consumes existing sqlc: `ListSpacesByMemberUser` (spaces.sql:101).
- Produces: `GET /users/{id}/spaces` → list of public spaces the user belongs to.

- [ ] **Step 1: Write the failing test**

```go
func TestListByMemberUser_ReturnsUserSpaces(t *testing.T) {
	h, uid, spaceID := newTestSpacesHandlerWithMember(t)
	req := withURLParam(withTenant(httptest.NewRequest(http.MethodGet, "/users/"+uid+"/spaces", nil)), "id", uid)
	rec := httptest.NewRecorder()

	h.ListByMemberUser(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body)
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte(spaceID)) {
		t.Fatalf("expected space %s in body: %s", spaceID, rec.Body)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/spaces/ -run TestListByMemberUser_ReturnsUserSpaces -v`
Expected: FAIL — `ListByMemberUser` undefined.

- [ ] **Step 3: Add service method** (`internal/spaces/service.go`)

```go
func (s *Service) ListByMemberUser(ctx context.Context, tenantID, userID pgtype.UUID, limit, offset int32) ([]SpaceListItem, error) {
	rows, err := s.store.listSpacesByMemberUser(ctx, tenantID, userID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("listing spaces by member: %w", err)
	}
	return toSpaceListItems(rows), nil
}
```
(Add the `listSpacesByMemberUser` store wrapper mirroring existing wrappers; confirm the sqlc query is tenant-scoped.)

- [ ] **Step 4: Add handler** (`internal/spaces/handlers.go`) — same shape as Task A3 Step 4, calling `ListByMemberUser`, rendering `RenderJSON(w, 200, items)` (bare list per feed/list convention — no meta unless a count query is added).

- [ ] **Step 5: Register route** (`internal/server/server.go`)

```go
			r.With(auth.OptionalAuth(kr, vkClient.Redis())).Get("/users/{id}/spaces", spacesHandlers.ListByMemberUser)
```

- [ ] **Step 6: Run tests to verify they pass**

Run: `go test ./internal/spaces/ -v && go build ./... && go vet ./...`
Expected: PASS, clean.

- [ ] **Step 7: Commit**

```bash
git add internal/spaces/ internal/server/server.go
git commit -m "feat(profile): GET /users/{id}/spaces tab (reuse ListSpacesByMemberUser)"
```

---

### Task A5: Certificate badges (`GET /users/{id}/certificates`) — reuse + tenancy fix

`ListLearningCertificatesByUser` (learning.sql:175) filters by `user_id` only — a multi-tenancy defect. Fix it to also filter `tenant_id`, then expose a public read for badges (Section 50:179). All issued certs are public social proof.

**Files:**
- Modify: `internal/db/queries/learning.sql` (add `tenant_id` filter to `ListLearningCertificatesByUser`)
- Modify: `internal/learning/handlers.go` (add `ListCertificatesByUser`)
- Modify: `internal/learning/service.go` / `store.go` (thread `tenant_id` through the certs list call; check `/me/certificates` caller passes it)
- Modify: `internal/server/server.go` (register route)
- Test: `internal/learning/handlers_test.go`

**Interfaces:**
- Consumes: `ListLearningCertificatesByUser` (now tenant-scoped).
- Produces: `GET /users/{id}/certificates` → list of `{ id, certificate_number, track_id, issued_at, media_id }`.

- [ ] **Step 1: Write the failing test** — a cert from another tenant is NOT returned

```go
func TestListCertificatesByUser_TenantIsolated(t *testing.T) {
	svc, tenantA, tenantB, uid := newTestLearningService(t)
	issueCert(t, svc, tenantA, uid) // 1 cert in tenant A for uid
	issueCert(t, svc, tenantB, uid) // 1 cert in tenant B for same uid

	got, err := svc.ListCertificatesByUser(ctx, tenantA, uid)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d certs, want 1 (tenant isolation broken)", len(got))
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/learning/ -run TestListCertificatesByUser_TenantIsolated -v`
Expected: FAIL — returns 2 certs (no tenant filter) or method undefined.

- [ ] **Step 3: Fix the query** (`internal/db/queries/learning.sql`)

```sql
-- name: ListLearningCertificatesByUser :many
SELECT id, tenant_id, user_id, track_id, certificate_number, media_id, issued_at
FROM learning_certificates
WHERE tenant_id = @tenant_id AND user_id = @user_id
ORDER BY issued_at DESC;
```

- [ ] **Step 4: Regenerate + thread tenant_id**

Run: `sqlc generate`
Then update every caller of `ListLearningCertificatesByUser` (the `/me/certificates` path and the new one) to pass `TenantID`. Add `Service.ListCertificatesByUser(ctx, tenantID, userID)`.

- [ ] **Step 5: Add handler + route** (same shape as A3/A4)

```go
			r.With(auth.OptionalAuth(kr, vkClient.Redis())).Get("/users/{id}/certificates", learningHandlers.ListCertificatesByUser)
```

- [ ] **Step 6: Run tests to verify they pass**

Run: `go test ./internal/learning/ -v && go build ./... && go vet ./...`
Expected: PASS, clean.

- [ ] **Step 7: Commit**

```bash
git add internal/db/queries/learning.sql internal/db/gen/learning.sql.go internal/learning/ internal/server/server.go
git commit -m "feat(profile): public certificate badges + fix cert tenancy leak"
```

---

### Task A6: Likes tab (`GET /users/{id}/likes`) — new author-scoped query

The existing `GetUserReactions` checks reactions on ONE entity — wrong shape. Add a query returning **content** a user reacted to, joined for card display.

**Files:**
- Modify: `internal/db/queries/reactions.sql` (add `ListContentLikedByUser`)
- Modify: `internal/reactions` (or the module owning reactions handlers) — add service + handler
- Modify: `internal/server/server.go` (register route)
- Test: reactions handler/service test

**Interfaces:**
- Produces: `GET /users/{id}/likes?page=&limit=` → content cards the user reacted to.

- [ ] **Step 1: Write the failing test**

```go
func TestListContentLikedByUser_ReturnsReactedContent(t *testing.T) {
	svc, tenantID, uid, contentID := newTestReactionsService(t)
	react(t, svc, tenantID, uid, "content", contentID, "like")

	got, err := svc.ListContentLikedByUser(ctx, tenantID, uid, 20, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != contentID {
		t.Fatalf("got %+v, want the reacted content %s", got, contentID)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/reactions/ -run TestListContentLikedByUser -v`
Expected: FAIL — method undefined.

- [ ] **Step 3: Add the query** (`internal/db/queries/reactions.sql`)

```sql
-- name: ListContentLikedByUser :many
-- Public "Likes" tab: content the user has reacted to (any reaction key).
SELECT c.id, c.title, c.slug, c.content_type, c.author_id, c.space_id,
       c.created_at, c.view_count, c.comment_count,
       COALESCE(up.display_name, u.username, '') AS author_name,
       COALESCE(up.avatar_url, '') AS author_avatar_url,
       LEFT(c.body, 200) AS excerpt,
       r.created_at AS reacted_at
FROM reactions r
JOIN contents c ON c.id = r.entity_id
LEFT JOIN users u ON u.id = c.author_id
LEFT JOIN user_profiles up ON up.user_id = c.author_id AND up.tenant_id = c.tenant_id
WHERE r.tenant_id = @tenant_id
  AND r.user_id = @user_id
  AND r.entity_type = 'content'
  AND c.status = 'published'
  AND c.visibility = 'public'
ORDER BY r.created_at DESC
LIMIT @page_limit OFFSET @page_offset;
```
(Confirm the `contents` column for the reacted entity is `entity_id` and content type key is `'content'` — verified against `GetProfileFeed`/reactions schema. If reactions store other entity types, the `entity_type='content'` filter isolates content likes.)

- [ ] **Step 4: Regenerate sqlc, add service + handler + route**

Run: `sqlc generate`, then add `Service.ListContentLikedByUser`, handler, and:
```go
			r.With(auth.OptionalAuth(kr, vkClient.Redis())).Get("/users/{id}/likes", reactionsHandlers.ListLikedByUser)
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/reactions/ -v && go build ./... && go vet ./...`
Expected: PASS, clean.

- [ ] **Step 6: Commit**

```bash
git add internal/db/queries/reactions.sql internal/db/gen/reactions.sql.go internal/reactions/ internal/server/server.go
git commit -m "feat(profile): GET /users/{id}/likes tab (content reacted to)"
```

---

### Task A7: Replies tab (`GET /users/{id}/replies`) — new author-scoped query

Comments authored by the user, joined to their parent content for context. (`comments` has no anonymity flag; the Section 08 anonymity rule is noted for the deferred forum-replies union.)

**Files:**
- Modify: `internal/db/queries/comments.sql` (add `ListCommentsByAuthor` + `CountCommentsByAuthor`)
- Modify: `internal/comments/handlers.go` + `service.go`
- Modify: `internal/server/server.go` (register route)
- Test: comments handler/service test

**Interfaces:**
- Produces: `GET /users/{id}/replies?page=&limit=` → the user's comments with parent-content title/slug.

- [ ] **Step 1: Write the failing test**

```go
func TestListCommentsByAuthor_ReturnsUserReplies(t *testing.T) {
	svc, tenantID, uid, contentID := newTestCommentsService(t)
	addComment(t, svc, tenantID, uid, contentID, "great post")

	got, total, err := svc.ListByAuthor(ctx, tenantID, uid, 20, 0)
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || len(got) != 1 || got[0].Body != "great post" {
		t.Fatalf("got %d/%+v want 1 reply", total, got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/comments/ -run TestListCommentsByAuthor -v`
Expected: FAIL — method undefined.

- [ ] **Step 3: Add queries** (`internal/db/queries/comments.sql`)

```sql
-- name: ListCommentsByAuthor :many
SELECT c.id, c.tenant_id, c.content_id, c.parent_id, c.body, c.vote_score,
       c.created_at,
       ct.title AS content_title, ct.slug AS content_slug
FROM comments c
JOIN contents ct ON ct.id = c.content_id
WHERE c.tenant_id = @tenant_id
  AND c.author_user_id = @author_user_id
  AND c.deleted_at IS NULL
  AND ct.status = 'published'
  AND ct.visibility = 'public'
ORDER BY c.created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: CountCommentsByAuthor :one
SELECT COUNT(*)::int AS count
FROM comments c
JOIN contents ct ON ct.id = c.content_id
WHERE c.tenant_id = @tenant_id
  AND c.author_user_id = @author_user_id
  AND c.deleted_at IS NULL
  AND ct.status = 'published'
  AND ct.visibility = 'public';
```

- [ ] **Step 4: Regenerate sqlc, add service + handler + route**

Run: `sqlc generate`, then add `Service.ListByAuthor` (returns items + total), handler, and:
```go
			r.With(auth.OptionalAuth(kr, vkClient.Redis())).Get("/users/{id}/replies", commentsHandlers.ListByAuthor)
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/comments/ -v && go build ./... && go vet ./...`
Expected: PASS, clean.

- [ ] **Step 6: Commit**

```bash
git add internal/db/queries/comments.sql internal/db/gen/comments.sql.go internal/comments/ internal/server/server.go
git commit -m "feat(profile): GET /users/{id}/replies tab (user comments)"
```

---

### Task A8: Fix the broken `/users/{id}/content` route + update catalog

`GET /users/{id}/content` is wired to `contentHandlers.ListContent` (server.go:2768), which ignores the `{id}` param and returns all tenant content. Repoint it to an author-scoped handler (Posts tab parity with `/feed/profile`), then update the routes catalog for every Phase A change.

**Files:**
- Modify: `internal/content/handlers.go` (add `ListByAuthor`) OR repoint route to the feed handler
- Modify: `internal/server/server.go:2768`
- Modify: `.jeos/conventions/api-routes-catalog.md`
- Test: `internal/content/handlers_test.go`

- [ ] **Step 1: Write the failing test** — `/users/{id}/content` returns only that author's content

```go
func TestListByAuthor_ScopesToAuthor(t *testing.T) {
	h, authorA, authorB := newTestContentHandler(t)
	seedContent(t, authorA, 2)
	seedContent(t, authorB, 3)
	req := withURLParam(withTenant(httptest.NewRequest(http.MethodGet, "/users/"+authorA+"/content", nil)), "id", authorA)
	rec := httptest.NewRecorder()

	h.ListByAuthor(rec, req)

	var resp struct{ Data []struct{ AuthorID string `json:"author_id"` } `json:"data"` }
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if len(resp.Data) != 2 {
		t.Fatalf("got %d items, want 2 (author scoping broken)", len(resp.Data))
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/content/ -run TestListByAuthor_ScopesToAuthor -v`
Expected: FAIL — `ListByAuthor` undefined (current route returns all content).

- [ ] **Step 3: Add author-scoped handler** reusing the existing `GetProfileFeed` query via the feed service, or a `ListContentsByAuthor` query mirroring `GetProfileFeed`. Simplest: repoint the route to the feed handler.

```go
// internal/server/server.go:2768 — repoint to the author feed
r.With(auth.OptionalAuth(kr, vkClient.Redis())).
	Get("/users/{id}/content", feedHandlers.GetProfileFeedByID) // reads {id}, author-scoped
```
(Add a thin `GetProfileFeedByID` that reads `chi.URLParam(r, "id")` instead of `user_id`, or normalize the param name. Do not leave the route pointing at `ListContent`.)

- [ ] **Step 4: Update the routes catalog** (`.jeos/conventions/api-routes-catalog.md`, "Users & Social" + "Feed" sections)

```
GET    /users/{username}                    [public] lean profile header (no PII)
GET    /users/{id}/content                  [public] author-scoped posts
GET    /users/{id}/media                    [public] Media tab
GET    /users/{id}/spaces                   [public] Spaces tab
GET    /users/{id}/likes                    [public] Likes tab (content reacted to)
GET    /users/{id}/replies                  [public] Replies tab (user comments)
GET    /users/{id}/certificates             [public] certificate badges
GET    /feed/profile/{user_id}              [public] user timeline (was auth-gated)
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/content/ ./internal/feed/ -v && go build ./... && go vet ./...`
Expected: PASS, clean.

- [ ] **Step 6: Commit**

```bash
git add internal/content/ internal/feed/ internal/server/server.go .jeos/conventions/api-routes-catalog.md
git commit -m "fix(profile): author-scope /users/{id}/content + catalog all profile routes"
```

---

## Self-Review

**1. Spec coverage:**
- ADR-061 integration harness → Task A0 (foundation for all behavioral tests) ✓
- Section 23:47 five tabs → Posts (A2/A8), Replies (A7), Media (A3), Likes (A6), Spaces (A4) ✓
- Section 32:12 profile feed public → A2 ✓
- Section 31 follow counts → follower_count + following_count in A1 ✓
- Section 50:179 cert badges → A5 ✓
- Section 19 PII → A1 (lean DTO) ✓
- Section 08 anonymity → noted; Phase A Replies uses `comments` (no anon flag) ✓
- Deferred: user verification badges, forum-reply union, DTO caching — documented above ✓

**2. Placeholder scan:** Steps carry real SQL/Go/test code and exact commands. Tasks A2–A8 Step 1 snippets still show pre-harness helper names (`newTestXxx`, `withURLParam`); the binding **Integration Test Pattern** section supersedes those with the real `testdb.*` API (Task A0) and the A1 worked example — dispatch prompts for A2–A8 must carry this resolution. Two spots say "confirm X against schema" (tenant param on `ListMediaByUploader`/`ListSpacesByMemberUser`, reacted entity column) — verification instructions; if a column differs, the fix pattern is Task A6's tenancy fix.

**3. Type consistency:** `PublicProfileResponse` (A1) is the single header DTO. Service method names are unique per module (`GetPublicProfileByUsername`, `ListByUser`, `ListByMemberUser`, `ListCertificatesByUser`, `ListContentLikedByUser`/`ListLikedByUser`, `ListByAuthor`). All routes use `OptionalAuth`. `{username}` for the header, `{id}` for tabs — consistent with the catalog and the resolve-then-fetch frontend flow.

**Phase B (organization public profile backend) will be a separate plan** (`2026-07-DD-org-public-profile-backend.md`) after org-module recon — `GET /organizations/{slug}` public shape, Members tab (existing `/organizations/{id}/members`), org content feed, and the org `verified` badge (column already exists per Section 07).
