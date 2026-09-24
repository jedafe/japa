# Browser Extension (New-Tab Digest) — Implementation Plan

**Spec:** `docs/superpowers/specs/2026-09-24-browser-extension-design.md`
**Date:** 2026-09-24

Two halves that ship independently: **backend** (token mint + digest
endpoint + anonymous feeds) lands first and is fully testable without the
extension; **extension app** (`apps/extension`, WXT) consumes it.

---

## Verified during plan-writing

- `APIKeyMiddleware` already passes through when `X-API-Key` is absent —
  the anonymous-feeds change is a scope-gate variant only.
- `GetExploreFeed` uses **no viewer identity** — `feeds/popular` returns
  identical data for every caller, so anonymous access exposes nothing new.
- `internal/middleware/ratelimit.go` `RateLimit(rdb, tier)` is the
  IP-based limiter for the anonymous path ("loose" tier exists in `Tiers`).
- `CreateKey` does NOT validate `input.Scopes` against `ScopeCatalog` —
  a key can carry any scope string. Therefore the spec's "digest scope
  mintable only via extension-tokens" is **unenforceable via scopes**
  (and `api.read.*` wildcards cover `api.read.digest` anyway). Correction:
  `api.read.digest` is a normal catalog scope; the digest route requires
  it; extension keys just happen to get the narrow set. The value of the
  mint endpoint is convenience + narrow defaults, not exclusivity.
- Intel radar (`internal/intelligence/handlers/radar.go`) aggregates
  promoted items across ~6 domain tables with `country_space_id`/
  `destination_space_id` keyset queries (`ListIntelJobsBySpaces` etc.).
  For the digest's small "latest N" need, reusing two high-signal domain
  tables beats re-implementing the polymorphic merge: **visa_routes**
  (`destination_space_id`) plus **intel_jobs** (`country_space_id`) —
  merged by `created_at` desc (the queries' ORDER BY).
- `destination_country` resolution is **already built**:
  `gen.GetUserGeoProfileFields` (`db/queries/university.sql:684`,
  ADR-064a) returns it by `user_id` in one round-trip via
  `user_profiles → custom_field_values → custom_fields`. `publicapi.
  Service` already holds `s.queries *gen.Queries` (service.go:332) —
  call it directly. NO new cfv query.
- `GetSpaceByGeoCode` (`spaces.sql:643`) does NOT enforce the public
  guard — only `deleted_at IS NULL` + `level='country'`. The moat
  `SpaceReader` contract puts the guard in SQL (`GetPublicSpaceBySlug`
  enforces `visibility='public' AND status='active'`). Using the raw
  geo-code query could leak an archived/non-public country space into
  digests → add `GetPublicSpaceByGeoCode` to `spaces.sql` (same SELECT
  shape, plus `visibility='public' AND status='active'`), regen sqlc,
  extend `SpaceReader`.
- `visitor.Resolver.resolve` takes `*http.Request` — unusable from the
  API-key path. `MyFeed` already passes a nil VisitorContext to
  `GetForYouFeed` (geo personalization skipped); the digest does the
  same for `for_you` — consistent with existing behavior. No visitor-
  resolver coupling.
- Feed/scam readers from Phase 2 (`FeedReader`, `SafetyReader`) are the
  reuse seams for for_you/community/scam sections.
- Scope catalog is `internal/publicapi/scopes/scopes.go` — add
  `api.read.digest` there + JEOS scope catalog (§47).
- Digest cache key is `jc:api_digest:{viewer_hash}` — NOT the
  `SemanticSearcher.ScopeHash` output. The digest reads no
  member-scoped data, so viewerID alone is the correct cache
  identity; destination/profile changes self-heal via the short TTL.
- The scam-alerts section must honor `scam_alert.enabled` — the route
  gate doesn't apply inside the aggregate; call
  `SafetyReader.Enabled(ctx)` and omit the section when disabled.
- `ListVisaRoutesBySpaces`/`ListIntelJobsBySpaces` take nullable
  keyset params (`Column1`=spaceIDs, `Column2`=ts, `ID`, `Limit`) —
  pass NULL cursor for latest-N (radar.go:505-517 pattern). ORDER BY
  is `created_at` DESC (merge order), not `updated_at`. visa_routes
  requires `intel_record_id IS NOT NULL` — promoted rows only, which
  is the correct corpus. Radar does no status post-filtering, so
  behavior matches. The adapter wraps `*gen.Queries` directly — no
  new queries needed.
- OpenAPI is **hand-maintained** (`docs/openapi/api.yaml`, no gen
  target in the Makefile) — hand-edit it.

---

## Task 1 — JEOS docs-first

Update before code:

1. `.jeos/conventions/api-routes-catalog.md` (or §47's route table):
   - `POST /api/v1/me/extension-tokens` (JWT, `api.keys.self_manage`)
   - `GET /api/v1/me/extension-tokens` (JWT, `api.keys.self_manage`)
   - `GET /api/v1/public/me/digest` (API key, `api.read.digest`)
   - `GET /api/v1/public/feeds/{name}` gains optional-anonymous note.
2. Scope catalog (`.jeos` §47 scope table + `scopes.go` comment
   references): `api.read.digest` — "Read personalized digest
   (extension/aggregator endpoint)".
3. `.jeos/conventions/valkey-keys-catalog.md`: `jc:api_digest:{scope_hash}`
   — digest response cache.
4. `.jeos/conventions/system-settings-catalog.md`:
   `api.digest.cache_ttl_seconds` (default 120),
   `api.digest.section_limit` (default 8; scam/intel caps derived 3).
5. `.jeos/sections/` — add an extension-surface note (manifest +
   permissions + WXT app location) either in §47 or a short new section
   file, per AGENTS.md docs-first.
6. `docs/superpowers/specs/2026-09-24-browser-extension-design.md` —
   amend the "mintable only via extension-tokens" claim to the corrected
   normal-scope semantics (see plan-writing note above).

**Commit:** `docs(jeos): extension digest + token-mint catalogs`

---

## Task 2 — Extension token mint

New service method + handlers in `internal/publicapi`:

- `Service.CreateExtensionToken(ctx, userID, installID)` → wraps
  `CreateAPIKey` with:
  - `Name`: `extension:{install_id}` when installID is present
    (validate UUID-shaped); `extension:{short-uuid}` generated
    server-side when absent — the settings page cannot know the
    extension-generated install ID, so the field is OPTIONAL.
  - `Scopes`: `{api.read.digest, api.read.content, api.read.me,
    api.read.safety}`
  - `RateLimitTier`: `standard`; `ExpiresAt`: nil (V1 no expiry —
    revocation is manual)
  - Same `api.keys.max_per_user` cap (inherited from CreateAPIKey).
- `Service.ListExtensionTokens(ctx, userID)` → `ListAPIKeys` output
  filtered in Go to `name` prefix `extension:`
  (`ListAPIKeysByUser` returns the name column).
- NO separate revoke route — extension tokens are plain `api_keys`
  rows; `DELETE /me/api-keys/{id}` already covers revocation.
- Handlers: `CreateExtensionToken` (201 `{key, key_id, prefix}`),
  `ListExtensionTokens` (200 `{data:[…]}`) — same
  `requireUser`/`handleServiceError` patterns as `CreateKey`
  (handlers.go:110-150).
- Routes in `internal/server/server.go` next to `/me/api-keys` (same
  middleware chain: `auth.RequireAuth` + `rbac.RequirePermission
  ("api.keys.self_manage")` — server.go:4538-4549).

**Commit:** `feat(publicapi): extension token mint + list`

---

## Task 3 — `GET /public/me/digest`

- New scope `api.read.digest` in `scopes.ScopeCatalog` (Task 1 catalogs
  it first).
- New sqlc query `GetPublicSpaceByGeoCode` in `db/queries/spaces.sql`
  — same SELECT column list as `GetPublicSpaceBySlug`, `WHERE
  geo_code = $1 AND level='country' AND visibility='public' AND
  status='active' AND deleted_at IS NULL`. Regen via `make sqlc-gen`;
  add `GetPublicSpaceByGeoCode` to the `SpaceReader` iface (moat.go:79)
  and to the spaces-side adapter + test stubs.
- New reader iface `IntelReader` in `internal/publicapi/phase2.go`
  (alongside `FeedReader`/`SafetyReader`):
  `ListRecentForSpace(ctx, spaceID, limit) ([]IntelDigestItem, error)` —
  adapter in server.go wrapping `*gen.Queries` directly:
  `ListVisaRoutesBySpaces` + `ListIntelJobsBySpaces` with NULL keyset
  cursors, merged by `created_at` desc, capped at limit.
  `IntelDigestItem` DTO: `{id, kind("visa_route"|"job"), title,
  summary, created_at}` — curated fields only.
- Destination resolution in `Service.Digest`:
  1. `s.queries.GetUserGeoProfileFields(ctx, userID)` →
     `DestinationCountry` ISO code (already exists — do NOT write a
     new cfv query).
  2. `s.moat.Spaces.GetPublicSpaceByGeoCode(code)` → public+active
     country space only.
  3. Empty code / pgx.ErrNoRows → `destination: null`.
- `Service.Digest(ctx, viewerID)`:
  - `for_you`: `FeedReader.GetForYouFeed(ctx, viewerID, sort, limit,
    0, nil)` — nil VisitorContext, matching `MyFeed` exactly.
  - `community.popular`: `FeedReader.GetFeed` explore-popular path
    (same call `ExploreFeed` makes internally).
  - `destination.scam_alerts`: `SafetyReader.ListByOffset` top 3 —
    ONLY when `SafetyReader.Enabled(ctx)` (honors
    `scam_alert.enabled` inside the aggregate; the route-level
    `features.Gate` does not apply here).
  - `destination.intel`: `IntelReader` top 3.
  - Partial failure policy: a failing section degrades to `null`/
    empty with the rest intact (log the error); only a feed-reader
    outage fails the whole request — decide per-section, document in
    the handler comment.
  - Cache: `jc:api_digest:{sha256(viewer_uuid)}` — viewer identity
    only, NOT `SemanticSearcher.ScopeHash` (digest reads no
    member-scoped data). TTL `api.digest.cache_ttl_seconds` (120).
- Handler `GetDigest` in new `handlers_digest.go`; route under the
  public API group: `RequireScope("api.read.digest")`.
- Tests (mirror `handlers_feeds_test.go` stubs): full digest happy
  path, destination null path, scam-disabled path, cache hit,
  section caps, scope rejection, per-section partial failure.

**Commit:** `feat(publicapi): GET /public/me/digest aggregate endpoint`

---

## Task 4 — Anonymous `feeds/popular`

- Replace `RequireScope("api.read.content")` on `feeds/{name}` with a
  new sibling middleware `OptionalScope(scope, anonNames ...string)`
  in `internal/publicapi/middleware.go`. `r.With` middleware runs
  post-routing so `chi.URLParam(r, "name")` is available. Exact
  contract:
  - `keyCtx == nil` AND `name` ∈ anonNames → pass through
  - `keyCtx == nil` AND `name` ∉ anonNames → 401 `api_key_required`
  - `keyCtx != nil` → enforce `HasScope` exactly like `RequireScope`
    (key without the scope still 403s)
- Register with `anonNames = {"popular"}` — `latest`/`rising` stay
  key-required (ExploreFeed's name set is `popular|latest|rising`,
  phase2.go:177-179).
- Wrap the route in `mw.RateLimit(vkClient.Redis(), "loose")` —
  IP-based (300/min) for anonymous; keyed requests additionally
  carry the existing per-key sliding-window limiter.
- `feeds/tag/{tag}` and `feeds/space/{slug}` stay fully scoped —
  they're separate literal routes, no chi conflict with `{name}`.
- Test: no key + popular → 200; no key + latest → 401; key without
  `api.read.content` → 403; valid key → 200; `feeds/tag` unkeyed
  → 401.

**Commit:** `feat(publicapi): anonymous access to feeds/popular`

---

## Task 5 — `apps/extension` (WXT)

Scaffold with Bun (`bunx wxt@latest init` — verify the WXT init flow +
pin a version ≥7 days old per dep policy). Svelte 5 + TS template.

Structure:

```
apps/extension/
  wxt.config.ts          # manifest: newtab override, storage perm,
                         # host_permissions for api origin
  entrypoints/newtab/    # the digest page (Svelte)
  entrypoints/connect/   # minimal connect/settings page or options page
  lib/api.ts             # fetch client — X-API-Key from storage
  lib/storage.ts         # chrome.storage.local wrapper (key, cache, prefs)
  lib/types.ts           # digest DTO mirror types
```

- `lib/api.ts`: `getDigest()` → `GET {api}/api/v1/public/me/digest`;
  `getFeed(name)` → lazy community tabs. Base URL: build-time config
  (`WXT_API_BASE` env → `import.meta.env`), default production API.
- `newtab` page: anonymous branch (no key → popular feed + connect card)
  vs connected branch (digest). Stale-while-revalidate from
  `chrome.storage.local` cache keyed `digest` + `fetched_at`.
- `connect` (options page or in-page modal): install ID generated once
  (`crypto.randomUUID`, persisted), instructions + link to the
  settings page. **Main-app frontend task:** add a "Connect browser
  extension" affordance to `apps/main/src/lib/components/hub/settings/
  sections/ApiKeysSection.svelte` (exists — the natural home) that
  calls `POST /me/extension-tokens` and shows the key once; the
  extension provides the paste field.
- Manifest (wxt.config): `chrome_url_overrides.newtab`,
  `permissions: ["storage"]`, `host_permissions` for the API origin.
  `browser_specific_settings.gecko.id` set for future Firefox.
- `bun run build` produces `.output/` + `zip` for store upload.

**Verification:** `wxt dev` in Chromium — connect flow end-to-end with
the dev API on :8081, anonymous mode, stale cache, tab-switch lazy load.

**Commit(s):** `feat(extension): WXT scaffold + new-tab digest` (+ a
main-app settings-page commit for the connect button)

---

## Task 6 — OpenAPI + final verification

- Hand-edit `docs/openapi/api.yaml` — it is maintained by hand, there
  is no gen target. Add `/public/me/digest` (+ the anonymous note on
  `/public/feeds/{name}` and the two `/me/extension-tokens` routes).
- Full build/vet/tests: `go build ./...`, `go vet ./internal/...`,
  `go test ./internal/publicapi/... ./internal/server/...`.
- Live smoke on `:8081`: mint token (needs a JWT session — can stub via
  direct key row insert like the scoped-retrieval smoke, or exercise
  through the real endpoint if a session exists), digest call,
  anonymous popular call.

---

## Risks / watch-outs

- **`GetPublicSpaceByGeoCode` is a new sqlc query** — do not reuse raw
  `GetSpaceByGeoCode` (no public guard); a non-public/archived country
  space would otherwise leak into digests.
- **`api.read.*` covers digest** — correct per scope semantics; the
  spec doc must be amended (Task 1 step 6) so it doesn't claim
  exclusivity.
- **`apps/extension` registers automatically** — root `package.json`
  workspaces glob is `apps/*`; don't disturb the other apps' lockfile
  entries. `apps/main` connect UI lives in
  `ApiKeysSection.svelte` (exists).
- **Unrelated worktree changes remain unstaged** (japastage, apps/main
  modifications, `.jeos` dirty files) — stage only task files.
