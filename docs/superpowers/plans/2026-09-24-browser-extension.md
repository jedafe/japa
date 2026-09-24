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
  For the digest's small "latest N" need, reusing ONE high-signal domain
  table beats re-implementing the polymorphic merge: **visa_routes**
  (`destination_space_id`, `updated_at`) plus **intel_jobs**
  (`country_space_id`) — merge by recency. Both have `*BySpaces` queries.
- `destination_country` lives in profile custom fields; resolution seam:
  user → `custom_field_values` (key `destination_country`) → ISO code →
  `spaces.Service.GetSpaceByGeoCode` (`internal/spaces/store.go:477`).
  Verify the exact cfv query at implementation (ai_agent.sql:135 pattern).
- `publicapi.Service` already has `moat.Spaces` (has `GetSpaceByGeoCode`? —
  the moat reader iface wraps `GetPublicSpaceBySlug`; confirm whether
  `GetSpaceByGeoCode` is exposed or needs adding to the moat readers iface).
- Feed/scam readers from Phase 2 (`FeedReader`, `SafetyReader`) are the
  reuse seams for for_you/community/scam sections.
- Scope catalog is `internal/publicapi/scopes/scopes.go` — add
  `api.read.digest` there + JEOS scope catalog.

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
  - `Name`: `extension:{install_id}` (validate installID is UUID-shaped)
  - `Scopes`: `{api.read.digest, api.read.content, api.read.me,
    api.read.safety}`
  - `RateLimitTier`: `standard`
  - Same `api.keys.max_per_user` cap (inherited from CreateAPIKey).
- `Service.ListExtensionTokens(ctx, userID)` → `ListAPIKeys`-style read
  filtered to `name LIKE 'extension:%'`. Check `ListAPIKeys` for the
  existing list query — reuse it and filter in SQL or in Go.
- Handlers: `CreateExtensionToken` (201 `{key, key_id, prefix}`),
  `ListExtensionTokens` (200 `{data:[…]}`) — same
  `requireUser`/`handleServiceError` patterns as `CreateKey`.
- Routes in `internal/server/server.go` next to `/me/api-keys` (same
  middleware chain: `RequireAuth` + `rbac.RequirePermission
  ("api.keys.self_manage")`).

**Commit:** `feat(publicapi): extension token mint + list`

---

## Task 3 — `GET /public/me/digest`

- New scope `api.read.digest` in `scopes.ScopeCatalog` (Task 1 catalogs
  it first).
- New reader iface `IntelReader` in `internal/publicapi/phase2.go`
  (alongside `FeedReader`/`SafetyReader`):
  `ListRecentForSpace(ctx, spaceID, limit) ([]IntelDigestItem, error)` —
  implement in a wiring-layer adapter (server.go) backed by the intel
  domain queries: `ListVisaRoutesBySpaces` + `ListIntelJobsBySpaces`,
  merged by `updated_at` desc, capped at limit. `IntelDigestItem` DTO:
  `{id, kind("visa_route"|"job"), title, summary, updated_at}` — curated
  fields only.
- Destination resolution in `Service.Digest`:
  1. `custom_field_values` for key `destination_country` on the key
     owner's user → ISO code. (Add/verify a `GetUserFieldValueByKey`
     query — the cfv lookup pattern at `db/queries/ai_agent.sql:135`
     shows the join shape; put the new query in `custom_fields.sql`,
     regen via `make sqlc-gen`.)
  2. `spaces.Service.GetSpaceByGeoCode` → country space. Wire it through
     the existing moat `Spaces` reader iface (add `GetSpaceByGeoCode`
     if absent) — NO new publicapi→spaces import.
  3. Missing/invalid → `destination: null`.
- `Service.Digest(ctx, viewerID)`:
  - `for_you`: `FeedReader.GetForYouFeed`-equivalent (top
    `section_limit`)
  - `community.popular`: `FeedReader` explore popular
  - `destination.scam_alerts`: `SafetyReader.ListByOffset` (top 3,
    confirmed-only already in SQL)
  - `destination.intel`: `IntelReader` (top 3)
  - Cache: `jc:api_digest:{scopeHash}` — reuse the same scope-hash
    input as `api_semantic` (viewer + member-space IDs + privileged) —
    call it via the injected `SemanticSearcher.ScopeHash` or factor the
    hash into a shared helper (`ScopeHasher` iface both use).
  - TTL: `api.digest.cache_ttl_seconds` default 120.
- Handler `GetDigest` in `handlers_feeds.go`-adjacent file (new
  `handlers_digest.go` if it grows); route under the public API group:
  `RequireScope("api.read.digest")`.
- Tests (mirror `handlers_feeds_test.go` stubs): full digest happy path,
  destination null path, cache hit path, section caps, scope rejection.

**Commit:** `feat(publicapi): GET /public/me/digest aggregate endpoint`

---

## Task 4 — Anonymous `feeds/popular`

- Move `feeds/{name}` registration out of the `RequireScope` group into
  a variant: `OptionalScope("api.read.content")` — new middleware
  sibling of `RequireScope` that lets `keyCtx == nil` through but still
  enforces scope when a key IS present (a key lacking the scope still
  403s — fail closed for authed callers).
- Wrap the route in `mw.RateLimit(rdb, "loose")` for the anonymous
  path — keyed requests additionally carry the per-key limiter already.
- Decide whether anonymous applies to ALL feed names or only `popular`:
  spec recommends popular only — implement by name check in the
  optional-scope handler path (`name=="popular" && keyCtx==nil` → allow;
  other names still require key). Simplest correct version: an
  `AnonymousOptional` wrapper that requires `name=="popular"` for
  unauthenticated pass-through.
- Test: no key + popular → 200; no key + latest → 401; key without
  scope → 403; valid key → 200.

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
  (`crypto.randomUUID`, persisted), instructions + link to
  `{site}/settings/developer` (the settings page needs the
  "Connect browser extension" button — **main-app frontend task**:
  small addition to the existing API-keys settings section in
  `apps/main`, calling the new token endpoints; include a copyable
  key display + paste field in the extension).
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

- Add `/public/me/digest` to the OpenAPI spec + regen catalog
  (`make` target used for OpenAPI — check Task 5 of the phase-2 plan's
  regen script name).
- Full build/vet/tests: `go build ./...`, `go vet ./internal/...`,
  `go test ./internal/publicapi/... ./internal/server/...`.
- Live smoke on `:8081`: mint token (needs a JWT session — can stub via
  direct key row insert like the scoped-retrieval smoke, or exercise
  through the real endpoint if a session exists), digest call,
  anonymous popular call.

---

## Risks / watch-outs

- **cfv query**: `custom_field_values` schema must be verified before
  writing `GetUserFieldValueByKey` — check the table's actual columns
  (entity_type/entity_id polymorphic? user_id direct?) in migration
  DDL, not by inference.
- **Intel domain merge**: `ListVisaRoutesBySpaces`/`ListIntelJobsBySpaces`
  are keyset-paginated radar queries with many params — if they're
  awkward to reuse, write a dedicated small `ListIntelDigestForSpace`
  query per domain in `intelligence_domains.sql` (two UNION-able
  selects) rather than fighting the radar cursor params.
- **`api.read.*` covers digest** — correct per scope semantics; the
  spec doc must be amended (Task 1 step 6) so it doesn't claim
  exclusivity.
- **`apps/extension` is a new workspace** — check root `package.json`
  workspaces / bunfig so the app registers correctly; don't disturb the
  other apps' lockfile entries.
- **Unrelated worktree changes remain unstaged** (japastage, apps/main
  modifications, `.jeos` dirty files) — stage only task files.
