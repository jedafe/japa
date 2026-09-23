# Agent Data API v1 — Design

**Date:** 2026-09-23
**Status:** Draft (awaiting review)
**Refs:** japaconnect-engineering/sections/47-public-api-developer-portal.md, sections/119-mcp-server.md, adr/ADR-116-mcp-server.md

## Context & Motivation

daily.dev's model — a personalized feed + community + agent-facing API for a
vertical audience — maps directly onto JapaConnect for migrants/travelers.
Their newest differentiator is `api.daily.dev/public/v1`: a governed,
token-authenticated REST surface for agents and third-party developers.

JapaConnect's equivalent moat is not generic content — it is the **structured
migration data** produced by the intelligence pipeline: visa routes, visa
processing timelines, scholarships, universities, credential pathways, and
language-test requirements. No agent or aggregator can get this data elsewhere.

Today most of this data is technically reachable via unauthenticated web-app
endpoints (`/visa-routes`, `/universities`, `/scam-alerts`, …), but those are
not a stable public contract: shapes are built for the SvelteKit apps,
pagination/envelopes are inconsistent, and internal changes would silently
break programmatic consumers. The §47 `publicapi` facade already exists for
content/spaces/tags/users with scoped API keys, per-tier rate limits, IP
allowlists, and a developer portal — it just doesn't cover the moat domains.

The §119 MCP bridge (`/api/v1/mcp`) already accepts `X-API-Key` auth via
`mcp.AuthMiddleware`, but has three gaps: the `agent.use` scope is not in
`ScopeCatalog`, API-key calls bypass the per-key sliding-window rate limiter,
and API-key callers inherit the key owner's full tool catalog (including
`content.create`/`content.delete`).

## Goals

- Expose v1 moat domains — **visa & immigration** and **education** — through
  the §47 public API facade under `/public/*` with scoped API-key auth.
- Freemium gating: core list/get endpoints free; compare/stats-trend/brief
  endpoints require a paid user tier, enforced per request.
- Ship a flagship **destination brief** endpoint: one aggregate JSON document
  per destination that no other platform offers.
- Close the three MCP gaps so external agents get rate-limited, scope-checked,
  read-only tool access with the same audit trail.
- Keep REST and MCP backed by the same service methods so the surfaces cannot
  drift.

## Non-Goals (Phase 2)

- Feed endpoints (`/feeds/popular`, tag/source feeds), semantic search
  (`/recommend/semantic` via pgvector/rag), personalized feeds, bookmarks.
- Content-as-markdown endpoints (`.md` URLs, expanded llms.txt).
- Browser extension (new-tab digest surface) — separate spec; will consume
  this API.
- Write scopes beyond the existing catalog entries.
- Fine-grained scope→tool mapping for MCP beyond the read-only allowlist.

## Architecture

Extend the existing §47 facade rather than mounting domain handlers directly.

- `publicapi.Service` gains curated read methods per moat domain, returning
  public DTOs defined in `types.go`. Internal/sqlc shapes never cross the
  boundary.
- Where a domain exposes a clean read service (e.g. visa-route compare,
  timeline stats), the facade delegates through a narrow interface injected
  into `NewService`. For plain list/get, it queries domain tables directly —
  consistent with how `service.go` reads content/spaces/tags today.
- New public routes live inside the existing API-key group
  (`APIKeyMiddleware` → `SlidingWindowRateLimitWithSettings` → `RequireScope`)
  and additionally respect the domain `features.Gate` flags — a disabled tool
  returns 404 on the public API exactly as it does on the web API.
- chi radix note: all new routes stay under the `/public/` prefix to avoid the
  wildcard shadowing documented at server.go:4401-4408.

## Endpoint Surface (v1)

Free tier — requires `api.read.visa` or `api.read.education` scope:

| Domain | Scope | Endpoints |
|---|---|---|
| Visa routes | `api.read.visa` | `GET /public/visa-routes`, `GET /public/visa-routes/{id}` |
| Visa timelines | `api.read.visa` | `GET /public/visa-timelines/stats` |
| Credentials | `api.read.visa` | `GET /public/credentials`, `GET /public/credentials/{id}`, `GET /public/credentials/professions` |
| Language tests | `api.read.visa` | `GET /public/language-tests`, `GET /public/language-tests/{slug}`, `GET /public/language-tests/requirements` |
| Scholarships | `api.read.education` | `GET /public/scholarships`, `GET /public/scholarships/{id}`, `GET /public/scholarships/statuses`, `GET /public/scholarships/levels` |
| Universities | `api.read.education` | `GET /public/universities`, `GET /public/universities/{slug}` |

Premium tier — requires the domain scope **and** `RequireTier(pro)`:

| Endpoint | Extra scope |
|---|---|
| `GET /public/visa-routes/compare` | `api.read.visa` |
| `GET /public/visa-timelines/stats/trend` | `api.read.visa` |
| `GET /public/universities/{slug}/compare` | `api.read.education` |
| `GET /public/destinations/{slug}/brief` | `api.read.brief` |

### Tier gating

`RequireTier(minTier)` is new middleware that resolves the key owner's unified
tier per request via the existing `TierResolver` (implemented by
`internal/tiers`; `capRateLimitTier` already uses it for rate limits).
Per-request resolution means a lapsed subscription loses premium access
immediately, without revoking keys. `regular` → free endpoints only;
`pro`/`business` → premium endpoints. Failure returns `403 tier_required`.

Rate limiting needs no changes: `SlidingWindowRateLimitWithSettings` already
caps `rate_limit_tier` by the resolved user tier
(`regular→basic, pro→standard, business→premium`) via
`api.rate_limit.{basic,standard,premium}` settings.

### Scope catalog additions

Added to `ScopeCatalog` so `/developer/scopes` and `/control/api-scopes` can
offer them (single source of truth, per scopes.go:78-83):

- `api.read.visa` — Read Visa & Immigration Data
- `api.read.education` — Read Education Data
- `api.read.brief` — Read Destination Briefs (premium)
- `agent.use` — Use Agent Tools via MCP

Issuance rule: `api.read.brief` (and premium endpoint access generally) is
offered in the portal to all users, but enforced at request time by
`RequireTier` — so no special issuance logic is needed.

## Destination Brief (flagship)

`GET /public/destinations/{slug}/brief?include=visa,timelines,universities,scholarships,scam_alerts`

One agent-consumable JSON document:

```json
{
  "destination": { "slug": "...", "name": "...", "type": "country|state|city|university" },
  "summary": "<markdown overview>",
  "sections": {
    "visa": { "routes": [...], "stats": {...} },
    "timelines": { "stats": [...] },
    "universities": [...],
    "scholarships": [...],
    "scam_alerts": [...]
  },
  "sections_unavailable": [],
  "generated_at": "...",
  "cache_ttl_seconds": 900
}
```

- Section fan-out in parallel via errgroup; each section independently
  skippable — a failed section is omitted and named in
  `sections_unavailable` rather than failing the request. Destination
  resolution failure is the only hard error (`404 destination_not_found`).
- `include` filters which sections are composed (default: all).
- `summary` is a markdown overview generated by a deterministic Go template
  over the composed sections — no LLM call in v1 (keeps the endpoint cheap,
  cacheable, and testable).
- The `scam_alerts` section composes data that is already public on the web
  API (`/scam-alerts`); standalone `/public/scam-alerts*` endpoints remain a
  Phase 2 item — only the aggregate brief exposes them in v1.
- Valkey-cached per (slug, include-set); TTL default 900s, tunable via a new
  `api.destination_brief.cache_ttl_seconds` system setting (Rule 3) added to
  the system-settings catalog.
- Requires `api.read.brief` scope + `RequireTier(pro)`.

## MCP Changes

All in `internal/mcp` + `internal/server` wiring:

1. **`agent.use` in `ScopeCatalog`** — enforced by `AuthMiddleware` today but
   not issuable through the portal.
2. **Rate-limit gap fix** — `mcp.AuthMiddleware` stores the user in
   `auth.UserContextKey` but never stores the key in publicapi's context, so
   API-key calls skip the sliding-window limiter. Export a
   `publicapi.WithAPIKeyContext(ctx, keyCtx)` helper; the MCP middleware calls
   it for the API-key path; mount `SlidingWindowRateLimitWithSettings` after
   `AuthMiddleware` on `/mcp`.
3. **Scope-overreach fix** — API-key callers currently receive the key owner's
   full tool catalog (RBAC-driven), which can include `content.create`,
   `content.delete`, `rbac.grant`. For API-key-authenticated calls, the
   `tools/list` and `tools/call` paths are restricted to a static read-only
   allowlist:
   `content.read, content.search, rag.search, visa.search, visa.get,
   visa.compare, scholarship.search, university.get, destination.brief`.
   JWT-authenticated calls keep existing RBAC behavior.
4. **New read tools** — registered in the ai tool registry and backed by the
   same `publicapi.Service` methods as the REST endpoints:
   `visa.search`, `visa.get`, `visa.compare`, `scholarship.search`,
   `university.get`, `destination.brief`. Same audit trail
   (`ai_agent_sessions` / `ai_agent_tool_calls`) as existing tools.

## Contract, Errors & Catalogs

- Response envelope and error format follow existing `publicapi` handlers;
  errors via `errpkg.WriteError`: `invalid_api_key`, `api_key_required`,
  `insufficient_scope`, `tier_required`, `rate_limit_exceeded`,
  `ip_not_allowed`, `destination_not_found`, `feature_disabled`.
- `public_catalog_openapi.json` + `openapi.go` extended for every new
  endpoint — `/developer/swagger` renders them automatically.
- `ScopeCatalog` additions listed above.
- New system setting: `api.destination_brief.cache_ttl_seconds` (default 900)
  — add to system-settings catalog.
- JEOS §47 spec updated to record the expanded catalog (route additions and
  new scopes).

## Data Flow

```
Agent ──X-API-Key──▶ APIKeyMiddleware ──▶ SlidingWindowRateLimit
                                          (capped by user tier)
                      │
                      ├─ RequireScope(api.read.visa|education|brief)
                      ├─ RequireTier(pro)            [premium endpoints]
                      ├─ features.Gate(domain flag)
                      ▼
              publicapi.Handlers ──▶ publicapi.Service
                      │                    │ delegate via narrow iface
                      │                    ▼
                      │              domain services / domain tables
                      ▼
                 public DTOs (JSON)

Agent ──X-API-Key──▶ mcp.AuthMiddleware ──▶ WithAPIKeyContext ──▶ rate limit
                      │  agent.use scope check
                      ▼
              mcp.Server tools/call ──▶ read-only allowlist (API-key path)
                      ▼
              ai.Executor ──▶ publicapi.Service methods (same impl)
```

## Testing

- `testdb`-backed service tests per domain (list/get, filters, pagination).
- Handler tests: missing key → 401 `api_key_required`; missing scope → 403
  `insufficient_scope`; regular-tier key on premium endpoint → 403
  `tier_required`; disabled feature flag → 404.
- Brief: full composition, partial failure → `sections_unavailable`,
  `include` filtering, cache hit path, unknown slug → 404.
- MCP: API-key tools/list shows only allowlisted tools; `tools/call` on a
  non-allowlisted tool → error; API-key calls consume rate-limit budget;
  JWT path behavior unchanged.
- OpenAPI/catalog test: every `/public/*` route present in
  `public_catalog_openapi.json` (extend the existing `openapi_test.go`
  pattern).

## Success Criteria

- An external agent with a free key can query visa routes, timelines,
  credentials, language tests, scholarships, and universities via documented
  OpenAPI endpoints.
- A pro/business key can call compare/trend endpoints and the destination
  brief; a regular key gets `403 tier_required`.
- Claude Desktop (or any MCP client) can connect to `/api/v1/mcp` with
  `X-API-Key` + `agent.use` scope and call only read-only tools, rate-limited
  per key.
- No endpoint leaks internal shapes; public DTOs are stable under internal
  refactors.

## Phase 2 (deferred)

Feed + semantic-search parity with daily.dev, `.md`/llms.txt content
endpoints, and the new-tab browser extension consuming this API (likely via a
`/public/me/digest`-style endpoint added then).
