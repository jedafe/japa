# Agent Data API Phase 2 — Design

**Date:** 2026-09-24
**Status:** Draft (awaiting review)
**Refs:** docs/superpowers/specs/2026-09-23-agent-data-api-design.md (v1),
japaconnect-engineering/sections/47-public-api-developer-portal.md,
sections/119-mcp-server.md, sections/81-scam-alert-board.md, sections/32-activity-feed-timeline.md

## Context & Motivation

Phase 1 (v1) shipped the moat domains — visa & immigration, education, and the
premium destination brief — through the §47 `publicapi` facade, plus MCP
hardening and five moat tools (commits through `739defd5`).

Phase 2 covers the deferred agent-facing surfaces chosen for this cycle:

- **Discovery + personalized feeds.** The web app has a full feed system
  (`/explore` anonymous, `/feed` primary, `/feed/for-you` ranked,
  `/feed/space/{id}`) but none of it is reachable under API-key auth. daily.dev
  parity is `/feeds/popular`/`/feeds/tag/{tag}`; API keys carry `UserID`, so we
  can additionally offer a key-owner-ranked feed — a surface daily.dev's
  anonymous-only API does not have.
- **Semantic search.** `rag.Service.Retrieve` already powers the `rag.search`
  MCP tool over the content_embeddings (pgvector) index. A REST endpoint makes
  it reachable to non-MCP agents; embedding-backed queries cost real money, so
  this is the Phase 2 premium surface.
- **Markdown representation of content.** `/llms.txt` + `/llms-full.txt`
  already exist at the site root, but individual content is only available as
  JSON. Agents and LLM crawlers benefit from a `text/markdown` representation.
- **Scam alerts as standalone endpoints.** v1 included them only inside the
  destination brief; `scamalert.Store.ListReports` already filters to
  `status='confirmed' AND deleted_at IS NULL`, which is exactly what a public
  read surface needs.

## Goals

- Expose anonymous discovery feeds (popular/latest/rising, by tag, by space)
  and a personalized `/public/me/feed` through the §47 facade.
- Ship premium semantic search at `/public/search/semantic`, tier-gated and
  Valkey-cached so repeated agent polling does not re-embed.
- Serve `text/markdown` representations of content on an explicit route and
  via `Accept` negotiation on the existing content route.
- Expose confirmed scam alerts as standalone list/get/schema endpoints.
- Keep REST and MCP backed by the same `publicapi.Service` methods via the
  established `AgentDataReader`/`agentDataAdapter` pattern — surfaces cannot
  drift, and no new import edges into `publicapi` are introduced.

## Non-Goals (Phase 3)

- Bookmarks surface (explicitly deferred again).
- Write APIs (bookmarking, posting, scam report submission via API key).
- Webhooks / push surfaces for agents.
- Browser extension — separate spec; will consume this API.
- Embedding *coverage* improvements (indexing more entity types) — the search
  endpoint consumes the existing index as-is.

## Architecture

Identical to v1: `publicapi.Service` gains curated read methods returning
public DTOs; each domain is accessed through a narrow reader interface so the
facade never couples to domain internals.

```go
// internal/publicapi/moat.go (extended) or a new phase2.go

type FeedReader interface {
    GetFeed(ctx context.Context, timeline string, userID, spaceID, authorID *pgtype.UUID,
        sort string, limit, offset int32, visitorCountry, visitorRegion string) ([]feed.FeedItem, error)
    GetForYouFeed(ctx context.Context, userID pgtype.UUID, sortMode string,
        limit, offset int32, vc *visitor.VisitorContext) ([]feed.FeedItem, error)
}

type SafetyReader interface {
    ListReports(ctx context.Context, f scamalert.ListReportsFilter) ([]scamalert.ScamReportRow, int64, error)
    GetReport(ctx context.Context, id pgtype.UUID) (scamalert.ScamReportRow, error)
}

type SemanticSearcher interface {
    Retrieve(ctx context.Context, query, language string, geoSpaceID *pgtype.UUID) ([]rag.RetrievedChunk, error)
    FormatContext(chunks []rag.RetrievedChunk) string // reuse for agent-ready text
}
```

The tag-feed path needs no reader interface — it delegates to generated
`gen.Queries` (`GetTagBySlug` + `ListContentByTag`) the same way
`service.go`/`handlers.go` already query content, spaces, and tags directly.

Import-cycle constraint (inherited from v1, unchanged): `internal/ai` must
never import `internal/publicapi` or moat domain packages. All new MCP tools
go through `ai.AgentDataReader` (extended with `any`-typed methods) +
`server.agentDataAdapter`.

## Endpoint Surface (Phase 2)

All routes mount inside the existing `RequireAPIKey` + sliding-window
rate-limit group (`/api/v1/public/...`).

### Free tier

| Route | Scope | Feature gate | Backing |
|---|---|---|---|
| `GET /public/feeds/{name}` | `api.read.content` | — | `GetFeed("explore", sort)` |
| `GET /public/feeds/tag/{tag}` | `api.read.content` | — | explore feed filtered by tag |
| `GET /public/feeds/space/{slug}` | `api.read.content` | — | `GetPublicSpaceBySlug` → space feed |
| `GET /public/me/feed` | `api.read.me` | — | `GetForYouFeed(key.UserID)` |
| `GET /public/content/{id}/markdown` | `api.read.content` | — | existing content reader |
| `GET /public/scam-alerts` | `api.read.safety` | `scam_alert.enabled` | `ListReports` |
| `GET /public/scam-alerts/schema` | `api.read.safety` | `scam_alert.enabled` | scam-alert schema handler equivalent |
| `GET /public/scam-alerts/{id}` | `api.read.safety` | `scam_alert.enabled` | `GetReport` |

`{name}` ∈ `{popular, latest, rising}` mapped onto the existing `parseSort`
vocabulary (`popular`→`top`). `?sort=` may still override on feed routes;
`?limit=`/`?offset=` follow the v1 pagination contract.

`GET /public/content/{id}` additionally honors `Accept: text/markdown` —
content negotiation is a convenience; the explicit `/markdown` route is the
primary, agent-fetchable surface.

### Premium tier (`RequireTier(publicAPIService, "pro")`)

| Route | Scope | Backing |
|---|---|---|
| `GET /public/search/semantic?q=&lang=&space=&limit=` | `api.read.search` | `rag.Service.Retrieve` |

### Scope catalog additions

- `api.read.safety` — confirmed scam reports (free).
- `api.read.search` — semantic search (premium-marked in the catalog, like
  `api.read.brief`).

## Semantic Search

`GET /public/search/semantic` promotes `RetrievedChunk` to a public DTO:

```json
{
  "data": [
    {
      "entity_type": "content",
      "entity_id": "…",
      "content_type": "post",
      "snippet": "…",
      "similarity": 0.81,
      "tier": "…",
      "title": "…",
      "href": "/post/…"
    }
  ],
  "query": "…", "language": "en", "space": null,
  "cached": false
}
```

- `title`/`href` are best-effort resolutions (space→slug, content→canonical
  path); unresolved entities return `null` rather than failing the request.
- If no embedder is configured on `rag.Service`, the endpoint returns
  `503 search_unavailable` — never a silent empty 200.
- Results cached in Valkey: `jc:api_semantic:{query_hash}:{lang}:{space}`,
  TTL via `system_settings` key `api.semantic_search.cache_ttl_seconds`
  (default 300). The key pattern and setting must be cataloged in JEOS
  before code (Rules 3 & 4).

## Feeds

- `{name}` feed delegates to `GetFeed("explore", nil, nil, nil, sort, …)` —
  same anonymous explore pipeline the web `/explore` route uses (including
  its Valkey cache/stampede behavior via the service).
- `/public/feeds/tag/{tag}` resolves the tag via `GetTagBySlug` then
  delegates to the generated `ListContentByTag` query with
  `CanSeeUnpublished=false`, `CanSeePrivate=false`, `CanSeeMembers=false` —
  i.e. `status='published' AND visibility='public'` only. That query is
  cursor-paginated, so this route uses `?cursor=` (opaque cursor token
  carrying `created_at,id`) instead of `?offset=`; `next_cursor` is returned
  in the envelope.
- `/public/feeds/space/{slug}` resolves the slug via
  `GetPublicSpaceBySlug` (public/active/non-deleted only, same guard as the
  brief) then calls `GetFeed` with the space filter.
- `/public/me/feed` resolves the key owner's `UserID` from API-key context
  and calls `GetForYouFeed`. `?sort=` maps to the same sort vocabulary.
- Public `FeedItem` DTO: content id, content type, title, space
  slug/name, author handle, vote/comment scores, published timestamp —
  never the internal `feed.FeedItem` verbatim.

## Markdown Representation

- `GET /public/content/{id}/markdown` → `200 text/markdown; charset=utf-8`
  with the content body plus a small front-matter block (title, author,
  space, canonical URL). Same visibility rules as `GET /public/content/{id}`.
- `GET /public/content/{id}` with `Accept: text/markdown` returns the same
  representation; JSON remains the default for all other Accept values.

## MCP Parity

Five new tools registered via the existing `ai_tools` catalog + seeded
migration (next free migration number — `000722` was taken in v1, verify at
plan time), surfaced through `agentDataAdapter`:

| Tool | API-key scope | minTier |
|---|---|---|
| `feeds.explore` (params: sort, space_slug, tag, limit) | `api.read.content` | — |
| `feeds.for_you` | `api.read.me` | — |
| `scam.list` | `api.read.safety` | — |
| `scam.get` | `api.read.safety` | — |
| `search.semantic` | `api.read.search` | `pro` |

All additions go through the existing `apiKeyToolGates` mechanism — scope
checks for API-key callers, `minTier` for all callers (mirrors REST premium
authorization exactly).

## Contract, Errors & Catalogs

- Same error envelope as v1 (`errpkg.WriteError`, snake_case codes):
  `insufficient_scope`, `tier_required`, `search_unavailable` (503),
  `destination_not_found` reuse pattern → `space_not_found` for bad feed
  slugs (404), `invalid_feed_name` (400).
- JEOS-first (Task 1, docs before code): `api-routes-catalog.md` entries for
  all routes, `jc:api_semantic:` in `valkey-keys-catalog.md`,
  `api.semantic_search.cache_ttl_seconds` in `system-settings-catalog.md`,
  §47/§119 updates.
- `public_catalog_openapi.json` regenerated via the repo script; every
  `/public/*` route must appear in the spec (existing completeness test).

## Testing

Same stub-based `httptest` pattern as v1 — no testdb, no pgxmock:

- Feed endpoints: sort-name mapping, tag/space filters, bad feed name → 400,
  non-public space slug → 404.
- `/public/me/feed`: uses key-owner `UserID`, not a query param.
- Markdown: `text/markdown` content type on explicit route + Accept
  negotiation; JSON default preserved.
- Scam alerts: confirmed-only rows; feature-gate off → 404; scope denial.
- Semantic: 200 with results, `503 search_unavailable` with nil embedder,
  tier denial on `regular`, cache hit path.
- MCP: tool-gate scope denials, `search.semantic` tier denial, for-you uses
  caller identity.

## Success Criteria

- `go build ./...` clean; `go vet` clean on touched packages.
- `go test ./internal/publicapi/ ./internal/mcp/ ./internal/ai/` green.
- Live smoke pass (same matrix as v1): scope denials, tier denials, real
  data on each surface, cache keys in Valkey.
- JEOS catalogs and OpenAPI spec fully synchronized.

## Phase 3 (deferred)

- Bookmarks read surface (revisit when a concrete agent use-case exists).
- Write APIs via API keys; webhooks.
- Browser extension (separate spec — consumes `/public/feeds/*` and the
  destination brief).
- Additional entity types in the semantic index (service directory,
  scholarship bodies) if Retrieval quality demands it.
