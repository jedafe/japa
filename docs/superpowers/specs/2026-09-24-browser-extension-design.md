# Browser Extension — Design Spec

**Date:** 2026-09-24
**Status:** Draft for review
**Depends on:** Agent Data API Phase 1 (`2026-09-23-agent-data-api-design.md`),
Phase 2 (`2026-09-24-agent-data-api-phase2-design.md`),
RAG scoped retrieval (`2026-09-24-rag-scoped-retrieval-design.md`)

## Summary

A Manifest V3 browser extension that turns every new tab into a JapaConnect
digest — "daily.dev for travelers/migrants." The new-tab page shows a
personalized card grid: the user's for-you feed, destination-specific
safety/intel updates, and community activity, all deep-linked back to
japa.to.

The extension is a **thin client of the Agent Data API** — it adds no new
domain logic. Backend work is limited to two additions: a one-click token
mint for connect flow, and a `/public/me/digest` aggregation endpoint so a
tab open costs one API call instead of five.

## Goals

- New-tab override page rendering a personalized digest in <1.5s perceived
  load (cached digest + progressive section render).
- One-click connect from a logged-in japa.to settings page — no key
  copy-paste.
- Fully functional anonymous mode (public feeds only) with an upgrade path.
- Chrome/Edge V1; Firefox MV3-compatible structure from day one.
- Zero write capability — the extension is read-only by scope design.

## Non-Goals (V1)

- Bookmarks/save-from-extension (needs write scopes — Phase 3).
- Push notifications, badge counts, background alarms.
- In-extension semantic search box (stretch goal; `api.read.search`+pro
  already exists if we want it later).
- Sidebar/side-panel UI, omnibox, content scripts on third-party pages.
- Firefox/Edge store submission — manifest-compatible, but only Chrome Web
  Store submission in V1.

## Product surface

`apps/extension` — new workspace app. **WXT** (MV3 framework with first-class
Svelte 5 + TypeScript support and Bun compatibility — Rule 18) rather than
hand-rolled manifest plumbing; Plasmo is the viable alternative but its React
bias fights the codebase's Svelte conventions.

### New-tab page layout

```
┌─────────────────────────────────────────────┐
│  [JapaConnect]   [search box→site]  [avatar] │
├─────────────────────────────────────────────┤
│  FOR YOU                        (for-you)   │
│  ┌─────┐ ┌─────┐ ┌─────┐ ┌─────┐           │
│  │ card│ │ card│ │ card│ │ card│           │
│  └─────┘ └─────┘ └─────┘ └─────┘           │
├──────────────────┬──────────────────────────┤
│  YOUR DEST.      │  SAFETY + INTEL           │
│  Germany         │  • scam alert card        │
│                  │  • intel radar card       │
├──────────────────┴──────────────────────────┤
│  COMMUNITY     [popular|latest|rising]      │
│  ┌─────┐ ┌─────┐ ┌─────┐ ┌─────┐           │
│  │ card│ │ card│ │ card│ │ card│           │
│  └─────┘ └─────┘ └─────┘ └─────┘           │
└─────────────────────────────────────────────┘
```

- **For You** — top items from `/public/me/feed` (journey-aware ranking
  already handles destination/stage/visa boosts server-side). Hidden in
  anonymous mode.
- **Your Destination** — resolved from the key owner's
  `destination_country` profile field → country space. Shows the space
  name/card + link.
- **Safety + Intel** — confirmed scam alerts for the destination space
  (`/public/scam-alerts?destination_space_id=`) plus destination intel
  items (see Backend: intel section).
- **Community** — `/public/feeds/{popular|latest|rising}` with tab
  switching client-side; no extra calls until tab switch.
- Every card links to japa.to (`/content/{slug}`, `/space/{slug}`,
  `/forum/{slug}`) — `utm_source=extension` query param for attribution.

### Anonymous mode

No key configured → For You and Destination sections collapse; Community
shows `popular`. A "Connect your account" card deep-links to
`{site}/settings/developer` (the existing API-key UI) with a one-click
mint option (below). All anonymous traffic goes through the same public
endpoints — no key = 401, so anonymous mode requires a decision:

> **Decision needed:** anonymous extension installs cannot call
> `/public/feeds/*` at all today (401 without key). Options:
> (a) ship a bundled read-only "anonymous install" key — **rejected**,
> one key's quota shared by all installs and it's extractable;
> (b) allow unauthenticated access to `feeds/popular` only — small
> server change, rate-limited by IP;
> (c) anonymous mode shows static content + connect prompt.
> **Recommendation: (b)** — `GET /public/feeds/{name}` gains optional
> auth: valid key → scoped path; absent key → IP-rate-limited anonymous
> path returning the same public-only data. Popular/latest/rising are
> already public-filtered so there is no data-leak surface.

### Connect flow (auth model)

The blocker identified in the API spec — extensions can't reasonably hold a
hand-copied `jc_live_` key for mainstream users. V1 model:

1. Settings page on japa.to gets a **"Connect browser extension"** button
   (session-authed, `api.keys.self_manage` permission).
2. Click → `POST /api/v1/me/extension-tokens` → server mints an API key:
   - `name`: `extension:{install_id}` (install ID = random UUID generated
     by the extension, passed in the request)
   - `scopes`: `{api.read.digest, api.read.content, api.read.me,
     api.read.safety}` — narrow; no search/brief (premium) in V1
   - `rate_limit_tier`: `standard`
   - `environment`: `live`
3. Response shows the full key **once**; the user pastes it into the
   extension's connect screen, OR the settings page renders it as a
   copyable token + `chrome.runtime`-detectable deep link
   (`japa-extension://connect?key=...` is unreliable across browsers —
   stick with copy-paste, which is now one field, not a dev-portal flow).
4. Extension stores the key in `chrome.storage.local` only. Never in
   `sync` storage (would propagate the key to other signed-in devices —
   that IS acceptable to some users, but default to local).
5. Revocation = existing `/me/api-keys` delete UI (name prefix makes
   extension keys identifiable). Reconnect = mint a new key.

**Rejected alternatives:**

- **Full device pairing (OAuth-style code flow)** — best UX, but needs a
  pending-grants table, polling endpoint, and approval UI. Revisit if
  extension adoption justifies it.
- **Session cookie reuse** — extension would need `cookies` permission +
  CSRF handling; API keys are purpose-built for this.
- **Bundled shared key** — extractable, shared quota, no revocation path.

## Backend additions

### 1. `POST /api/v1/me/extension-tokens`

Session-authed (`auth.RequireAuth` + `rbac.RequirePermission
("api.keys.self_manage")`) thin wrapper over `Service.CreateAPIKey`:

- Request: `{install_id: string}` (UUID the extension generates once).
- Enforces `api.keys.max_per_user` like normal keys (extension keys count
  against the cap — simplest consistent rule).
- Response: `{key, key_id, prefix}` — full key returned once.

Also `GET /api/v1/me/extension-tokens` — lists the user's keys whose
name starts with `extension:` (for a "connected installs" settings row).
Revocation stays on the existing `DELETE /me/api-keys/{id}`.

### 2. `GET /public/me/digest`

API-key authed, `RequireScope("api.read.digest")` — **new scope** (JEOS
scope catalog update required, docs-first). Single aggregate call:

```json
{
  "for_you":   { "items": [FeedItem…≤8], "source": "for_you" },
  "destination": {
    "space": { "slug": "germany", "name": "Germany", "level": "country" },
    "scam_alerts": [ScamAlert…≤3],
    "intel": [IntelItem…≤3]
  },
  "community": { "popular": [FeedItem…≤8] },
  "generated_at": "…", "cached": false
}
```

Destination resolution order (key owner):

1. `destination_country` profile custom field → `GetSpaceByGeoCode` →
   country space (seam exists: `spaces.Service.GetSpaceByGeoCode`,
   `internal/spaces/store.go:477`).
2. If unset → `destination` section is `null` (extension hides it —
   `current_geo_space_id` is deliberately NOT a fallback: it describes
   where the user *is*, not where they're going).

`for_you` = existing `feed.Service` for-you timeline (top 8).
`community.popular` = `ListPublicFeed("popular")` (top 8).
`scam_alerts` = confirmed alerts for destination space (top 3).
`intel` = destination intel items — **needs a read seam**: the intel
radar (`internal/intelligence/handlers`, routes `/intel/radar*`) is the
curated destination-updates corpus; spec a narrow `IntelReader` interface
(latest N items for a space ID) — same adapter pattern as `FeedReader`/
`SafetyReader`.

Caching: `jc:api_digest:{scope_hash}` (new catalog key — docs-first),
TTL from new setting `api.digest.cache_ttl_seconds` (default 120). The
scope hash is the same mechanism as `jc:api_semantic:` (viewer +
memberships) since for-you/community content is viewer-scoped.

Anonymous (b) option from above: digest is always key-authed; anonymous
mode calls only `feeds/popular` directly.

### 3. CORS — no change needed

MV3 extensions declaring `host_permissions` for the API origin fetch
cross-origin **without CORS enforcement** — `chrome-extension://` and
`moz-extension://` origins never need entries in `JC_CORS_ORIGINS`.
(For WXT dev-server mode, requests come from `localhost` — covered by
`IsLocalOrigin` outside production.)

Manifest:

```json
{
  "manifest_version": 3,
  "permissions": ["storage"],
  "host_permissions": ["https://api.japa.to/*", "https://japa.to/*"],
  "chrome_url_overrides": { "newtab": "newtab.html" }
}
```

No `tabs`, `cookies`, `webRequest`, or content-script permissions —
the permission prompt at install stays minimal.

## Data contract details

- `FeedItem` DTO — reuse the Phase 2 feed DTO verbatim (`id, title,
  slug, content_type, excerpt, author, space, published_at, counts`).
- `ScamAlert` — reuse `PublicScamReport` verbatim (already curated —
  no moderation internals).
- `IntelItem` — new small DTO: `{id, title, summary, source_url?,
  published_at, destination_space_id}`. Source: intel radar items
  filtered to the destination space; reuse whatever curated fields the
  radar feed already returns — no internal intel shapes.
- Section cardinality caps are server-side constants
  (`api.digest.section_limit`, default 8/3/3/8 — setting-cataloged).
- `destination == null` is a valid response — the extension must render
  gracefully, not error.

## Refresh + rate-limiting model

- Extension fetches digest on tab open, then caches in
  `chrome.storage.local` for `api.digest.cache_ttl_seconds` (client
  reuses server TTL; shows stale-while-refresh pattern).
- Manual refresh button + automatic background refresh every 15 min
  while a tab is open (no `alarms` permission needed if done on
  visibility events; add `alarms` only if we want true background
  refresh — V1: skip).
- Community tab switching calls `feeds/{name}` lazily (cached 60s
  client-side).
- Server-side: keys minted for extension are `standard` tier — sliding
  window applies; digest being one call keeps burst cost at 1 req/tab-open.

## Security + privacy

- Extension key scopes are read-only and exclude `agent.use` (no MCP),
  `api.read.search`/`api.read.brief` (premium surfaces stay web/paid-API
  only in V1).
- `api.read.digest` is a normal catalog scope — correction from the
  original draft, which claimed it would be mintable only through
  `/me/extension-tokens`. That is unenforceable: `CreateKey` does not
  validate submitted scopes against the catalog, and `api.read.*`
  wildcards cover `api.read.digest` regardless. The mint endpoint's
  value is convenience and narrow defaults, not exclusivity — any key
  holding `api.read.digest` (or `api.read.*`) may call the endpoint.
- The key is a bearer secret in `chrome.storage.local` — the manifest
  requests no host permissions beyond japa.to origins, and the packaged
  build embeds no secrets.
- Request logging inherits existing API-key audit trail (`last_used_at`,
  `request_count`).

## Delivery + QA

- `apps/extension` builds via WXT → `zip` for Chrome Web Store upload.
  Submission is a manual store step (owner's developer account), not CI.
- Test plan: unit tests for the digest service method + extension-token
  route; WXT extension smoke-tested via `wxt dev` (Chromium) — load
  unpacked, connect flow, anonymous mode, stale-cache render.
- Store listing assets (icon, screenshots, description) — follow-up task.

## Open questions

1. Anonymous `feeds/popular` unauthenticated access — recommend (b)
   above; alternative is requiring connect before ANY content shows.
2. ~~Should `api.read.digest` be mintable via `/me/extension-tokens`
   only~~ — resolved during plan-writing: it is a normal catalog scope;
   exclusivity is unenforceable via scopes (see Security section).
3. Destination override in extension settings (user picks a different
   destination than their profile) — V1: no, profile field is canonical;
   revisit after launch.
4. Intel radar read seam — is there already a service method returning
   per-space intel items, or does the radar feed need a small list query
   added? (Implementation plan must verify.)

## Commits-as-plan

JEOS-first per AGENTS.md: before any code, update —

1. `.jeos/sections/47-public-api.md` (or the digest's owning section):
   `POST /me/extension-tokens`, `GET /public/me/digest` routes,
   `api.read.digest` scope, `jc:api_digest:` Valkey key,
   `api.digest.cache_ttl_seconds` + `api.digest.section_limit` settings.
2. `.jeos/sections/` new or amended section for the extension surface
   itself (routes catalog has no extension entries — the extension is an
   app, not a route set; document manifest + permissions instead).
3. Implementation plan doc, then code.
