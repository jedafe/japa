# Agent Data API Phase 2 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Extend the §47 public API facade with Phase 2 surfaces — anonymous + personalized feeds, premium semantic search, markdown content representation, and standalone scam alerts — with MCP parity throughout.

**Architecture:** Identical to v1. `publicapi.Service` gains curated read methods returning public DTOs via narrow reader interfaces (`FeedReader`, `SafetyReader`, `SemanticSearcher`); MCP tools go through `ai.AgentDataReader` + `server.agentDataAdapter` so `internal/ai` never imports `publicapi` or domain packages.

**Spec:** `docs/superpowers/specs/2026-09-24-agent-data-api-phase2-design.md`

**Tech Stack:** Go 1.x, chi, pgx/v5, sqlc `internal/db/gen`, Valkey, `golang.org/x/sync/errgroup` (already in go.mod), `internal/rag` pgvector retrieval, `internal/feed` service.

**Verified constraints (do not regress):**

- **Import cycles:** `publicapi` may import `feed`, `scamalert`, `rag` — all three dep trees verified clean of `publicapi`/`ai`/`rbac` edges. `internal/ai` must keep importing only `internal/publicapi/scopes` (the leaf package), never `internal/publicapi` itself.
- **JEOS-first:** new `/public/*` routes, the `jc:api_semantic:` Valkey key, and the `api.semantic_search.cache_ttl_seconds` setting must be cataloged in `.jeos/` BEFORE the code that uses them (Rules 3 & 4).
- **Route ordering:** chi prefers static segments, so `/public/feeds/tag/{tag}` and `/public/feeds/space/{slug}` coexist with `/public/feeds/{name}` — register them; chi resolves correctly.
- **Commit on `main`.** No AI-attribution comments or trailers (Rule 17). There are unrelated worktree changes — stage only the files each task touches.
- **`Retrieve` lies by omission:** `rag.Service.Retrieve` returns `nil, nil` both when the embedder is missing AND when `rag.enabled=false`. The endpoint must check availability itself (new `EmbedderConfigured()` + the `rag.enabled` setting) to return an honest `503`.
- **Scam alerts are already safe at SQL level:** `scamalert.Store.GetReport`/`ListReports` enforce `status='confirmed' AND deleted_at IS NULL`. Delegate to `scamalert.Service` (`Enabled`, `Schema`, `Get`, `List`) — do not query the store directly.

---

### Task 1: JEOS catalogs (docs-first)

Per AGENTS.md Rule: JEOS is always updated before code. All new routes, the Valkey key, the system setting, and §47/§119 amendments go in first.

**Files:**
- Modify: `.jeos/conventions/api-routes-catalog.md`
- Modify: `.jeos/conventions/valkey-keys-catalog.md`
- Modify: `.jeos/conventions/system-settings-catalog.md`
- Modify: `.jeos/sections/47-public-api-developer-portal.md`
- Modify: `.jeos/sections/119-mcp-server.md`

- [ ] **Step 1: Catalog the routes** in `api-routes-catalog.md` under the existing `/api/v1` public section (mirror the v1 agent-data entries' format):

```
GET /api/v1/public/feeds/{name}          — explore feed; name ∈ popular|latest|rising; ?limit&offset
GET /api/v1/public/feeds/tag/{tag}       — published+public content by tag slug; ?cursor&limit
GET /api/v1/public/feeds/space/{slug}    — feed for a public/active space; ?limit&offset
GET /api/v1/public/me/feed               — key owner's ranked for-you feed; ?sort&limit&offset
GET /api/v1/public/content/{id}/markdown — text/markdown representation of published public content
GET /api/v1/public/scam-alerts           — confirmed scam reports; ?scam_type&destination_space_id&limit&offset
GET /api/v1/public/scam-alerts/schema    — scam_type/platform enum options
GET /api/v1/public/scam-alerts/{id}      — single confirmed scam report
GET /api/v1/public/search/semantic       — premium; ?q&lang&space&limit → pgvector chunks
```

- [ ] **Step 2: Catalog the Valkey key** in `valkey-keys-catalog.md` (match the v1 `jc:api_dest_brief:` entry format):

```
jc:api_semantic:{query_sha256}:{lang}:{space} — cached semantic search result set — TTL api.semantic_search.cache_ttl_seconds (default 300)
```

- [ ] **Step 3: Catalog the setting** in `system-settings-catalog.md`:

```
api.semantic_search.cache_ttl_seconds — int — default 300 — TTL for jc:api_semantic:* cache entries (§47 Phase 2)
```

- [ ] **Step 4: Amend §47** — add a "Phase 2 — Feeds, Semantic Search & Safety" subsection listing the routes above, the two new scopes (`api.read.safety`, `api.read.search` — premium), and the markdown negotiation rule (`Accept: text/markdown` on `GET /public/content/{id}` plus the explicit `/markdown` route).

- [ ] **Step 5: Amend §119** — extend the tool-gate table with the five new tools:

```
feeds.explore    — api.read.content — all tiers
feeds.for_you    — api.read.me      — all tiers
scam.list        — api.read.safety  — all tiers
scam.get         — api.read.safety  — all tiers
search.semantic  — api.read.search  — pro+
```

- [ ] **Step 6: Commit** — `cd .jeos && git add -A && git commit -m "docs: agent data API phase 2 catalogs"` then bump the submodule pointer in the parent repo: `cd .. && git add .jeos && git commit -m "docs(jeos): bump submodule for phase 2 agent data catalogs"`.

---

### Task 2: Scopes + reader interfaces + service methods + wiring

Foundation: two new scopes, three reader interfaces, the `publicapi.Service` read methods everything else calls, and server wiring.

**Files:**
- Modify: `internal/publicapi/scopes/scopes.go` (the leaf catalog — entries added to `ScopeCatalog` BEFORE the `api.read.*` wildcard entries)
- Modify: `internal/publicapi/scopes.go` (shim — only if it re-lists scopes; check whether it re-exports the slice wholesale)
- Modify: `internal/publicapi/moat.go` (add interfaces) — or create `internal/publicapi/phase2.go` for the Phase 2 readers/methods to keep files navigable
- Modify: `internal/rag/service.go` (add `EmbedderConfigured`)
- Modify: `internal/server/server.go` (wiring only)
- Test: `internal/publicapi/scopes_test.go` (or existing scopes test file — extend)

- [ ] **Step 1: Add scopes** to `ScopeCatalog` in `internal/publicapi/scopes/scopes.go`, immediately after `api.read.brief` (before `agent.use` and wildcards):

```go
{
    Value:       "api.read.safety",
    Label:       "Read Scam Alerts",
    Description: "Read confirmed scam reports and the scam alert schema",
},
{
    Value:       "api.read.search",
    Label:       "Semantic Search",
    Description: "Semantic (embedding) search over indexed content and spaces (premium endpoint)",
},
```

- [ ] **Step 2: Add `EmbedderConfigured`** to `internal/rag/service.go` (next to `SetEmbedder`):

```go
// EmbedderConfigured reports whether an embedding provider is wired.
// The public semantic-search endpoint uses this to answer 503 honestly —
// Retrieve returns nil, nil without an embedder and cannot be distinguished
// from a legitimately empty result set.
func (s *Service) EmbedderConfigured() bool { return s.embedder != nil }
```

- [ ] **Step 3: Add reader interfaces** — new file `internal/publicapi/phase2.go` (imports `feed`, `scamalert`, `rag`, `visitor` — all verified cycle-free):

```go
// FeedReader is the subset of feed.Service the public API needs.
type FeedReader interface {
    GetFeed(ctx context.Context, timeline string, userID, spaceID, authorID *pgtype.UUID,
        sort string, limit, offset int32, visitorCountry, visitorRegion string) ([]feed.FeedItem, error)
    GetForYouFeed(ctx context.Context, userID pgtype.UUID, sortMode string,
        limit, offset int32, vc *visitor.VisitorContext) ([]feed.FeedItem, error)
}

// SafetyReader is the subset of scamalert.Service the public API needs.
// Get/List only ever return status='confirmed' reports (enforced in SQL).
type SafetyReader interface {
    Enabled(ctx context.Context) bool
    Schema() scamalert.SchemaOptions
    Get(ctx context.Context, id pgtype.UUID) (scamalert.ScamReportDTO, error)
    List(ctx context.Context, destSpaceID pgtype.UUID, scamType string, page, limit int32) ([]scamalert.ScamReportDTO, int64, error)
}

// SemanticSearcher is the subset of rag.Service the public API needs.
type SemanticSearcher interface {
    Retrieve(ctx context.Context, query, language string, geoSpaceID *pgtype.UUID) ([]rag.RetrievedChunk, error)
    EmbedderConfigured() bool
}
```

- [ ] **Step 4: Add `Service` methods** in `phase2.go` (with `SetFeedReader`/`SetSafetyReader`/`SetSemanticSearcher` setters on `Service`, matching the v1 `Set*Reader` pattern):

```go
// PublicFeedItem is the stable feed-card shape for API consumers.
type PublicFeedItem struct {
    ID           string   `json:"id"`              // FeedItem.ID
    Title        string   `json:"title"`           // FeedItem.Title
    Slug         string   `json:"slug"`            // FeedItem.Slug
    ContentType  string   `json:"content_type"`    // FeedItem.ContentType
    Excerpt      string   `json:"excerpt,omitempty"`
    SpaceID      string   `json:"space_id,omitempty"`
    SpaceSlug    string   `json:"space_slug,omitempty"`
    SpaceName    string   `json:"space_name,omitempty"`
    AuthorName   string   `json:"author_name,omitempty"`
    AuthorHandle string   `json:"author_handle,omitempty"` // FeedItem.AuthorUsername
    VoteScore    int32    `json:"vote_score"`
    CommentCount int32    `json:"comment_count"`
    ViewCount    int32    `json:"view_count"`
    Tags         []string `json:"tags,omitempty"`
    PublishedAt  string   `json:"published_at,omitempty"` // FeedItem.CreatedAt
}

// PublicScamReport is the curated public DTO — scamalert.ScamReportDTO
// carries moderation internals (reporter_id, reviewed_by, moderator_note,
// ai_model, hitl_*) that must not enter the public contract.
type PublicScamReport struct {
    ID                    string  `json:"id"`
    ScamType              string  `json:"scam_type"`
    DestinationSpaceName  string  `json:"destination_space_name,omitempty"`
    DestinationSpaceCode  string  `json:"destination_space_code,omitempty"`
    OriginSpaceName       string  `json:"origin_space_name,omitempty"`
    OriginSpaceCode       string  `json:"origin_space_code,omitempty"`
    OriginCommunity       string  `json:"origin_community,omitempty"`
    PlatformUsed          string  `json:"platform_used,omitempty"`
    ApproximateDate       string  `json:"approximate_date,omitempty"`
    AmountRange           string  `json:"amount_range,omitempty"`
    Description           string  `json:"description"`
    ReportedToAuthorities bool    `json:"reported_to_authorities"`
    IsPinned              bool    `json:"is_pinned"`
    CreatedAt             string  `json:"created_at"`
}
```

Methods:

```go
func (s *Service) ExploreFeed(ctx context.Context, name, sort string, limit, offset int32) ([]PublicFeedItem, error)
// name ∈ {popular,latest,rising}: popular→"top", latest→"latest", rising→"rising".
// Unknown name → ErrInvalidFeedName sentinel (handler → 400 invalid_feed_name).
// ?sort= query may override; invalid sort → fall back to the name mapping.
// Delegates: feed.GetFeed(ctx, "explore", nil, nil, nil, mappedSort, limit, offset, "", "")

func (s *Service) TagFeed(ctx context.Context, tagSlug, cursor string, limit int32) ([]PublicFeedItem, string, error)
// GetTagBySlug(slug) → 404 tag_not_found. Then ListContentByTag with
// CanSeeUnpublished=false, CanSeePrivate=false, CanSeeMembers=false,
// RequesterID=zero UUID (public surface).
// cursor = base64("{created_at_rfc3339nano},{uuid}") — decode failures → 400.
// next_cursor emitted only when len(rows) == limit.
// Map ListContentByTagRow → PublicFeedItem (rows carry title/slug/excerpt/tags/space/author).

func (s *Service) SpaceFeed(ctx context.Context, viewerID pgtype.UUID, slug, sort string, limit, offset int32) ([]PublicFeedItem, error)
// GetPublicSpaceBySlug (existing spaceReader) → 404 space_not_found, then
// GetFeed(ctx, "space", &viewerID, &space.ID, nil, sort, limit, offset, "", "")
// — matches feedHandlers.GetSpaceFeed (handlers.go:330); viewerID is the key
// owner's ID so member-only items resolve exactly as the web feed does.

func (s *Service) MyFeed(ctx context.Context, userID pgtype.UUID, sort string, limit, offset int32) ([]PublicFeedItem, error)
// Delegates GetForYouFeed(ctx, userID, sort, limit, offset, nil) — nil VisitorContext
// is handled inside the service (geo personalization skipped).

func (s *Service) ListScamAlerts(ctx context.Context, destSpaceID pgtype.UUID, scamType string, limit, offset int32) ([]PublicScamReport, int64, error)
// Convert offset→page for the service call: page = offset/limit + 1.
// Map ScamReportDTO → PublicScamReport (drop moderation internals).
// The route's features.Gate("scam_alert.enabled") covers enablement.

func (s *Service) GetScamAlert(ctx context.Context, id pgtype.UUID) (PublicScamReport, error)
func (s *Service) ScamAlertSchema() scamalert.SchemaOptions

func (s *Service) SemanticSearch(ctx context.Context, query, lang, spaceSlug string, limit int32) (*SemanticSearchResult, error)
// - Empty query → ErrMissingQuery (400).
// - rag.enabled=false OR !EmbedderConfigured() → ErrSearchUnavailable (503).
//   Read rag.enabled via the Service's settings reader (same mechanism the
//   brief uses for its TTL setting).
// - spaceSlug non-empty → resolve via spaceReader.GetPublicSpaceBySlug → 404 space_not_found,
//   pass &space.ID as geoSpaceID.
// - Check Valkey jc:api_semantic:{sha256(query)}:{lang}:{spaceSlug} first;
//   on hit return cached result with Cached=true.
// - Retrieve → map chunks to PublicSemanticHit DTO; slice to limit.
// - Best-effort title/href: entity_type "space" → Queries.GetSpaceByID for
//   name+slug (href "/space/{slug}"); entity_type "content"/"forum_thread" →
//   Queries.GetPublishedContentByID for title+slug (href "/content/{slug}").
//   Any resolution error → title/href null, never fail the request.
// - Write-through cache with TTL from api.semantic_search.cache_ttl_seconds.

type SemanticSearchResult struct {
    Data     []PublicSemanticHit `json:"data"`
    Query    string              `json:"query"`
    Language string              `json:"language"`
    Space    *string             `json:"space"`
    Cached   bool                `json:"cached"`
}

type PublicSemanticHit struct {
    EntityType  string   `json:"entity_type"`
    EntityID    string   `json:"entity_id"`
    ContentType string   `json:"content_type"`
    Snippet     string   `json:"snippet"`
    Similarity  float64  `json:"similarity"`
    Tier        string   `json:"tier,omitempty"`
    Title       *string  `json:"title"`
    Href        *string  `json:"href"`
}
```

- [ ] **Step 5: Wire in `internal/server/server.go`** — inside the existing `publicAPIService` configuration block (where `SetMoatReaders` etc. are called, ~line 2311+):

```go
publicAPIService.SetFeedReader(feedService)
publicAPIService.SetSafetyReader(scamAlertService)
publicAPIService.SetSemanticSearcher(ragService)
```

Verify the exact variable names (`feedService` is constructed ~line 700s; `scamAlertService` at 1278; find `ragService` — `grep 'rag.NewService'`). All three must exist before this point — `publicAPIService` is built late in `setupRoutes`, so they do.

- [ ] **Step 6: Commit** — `git add internal/publicapi internal/rag/service.go internal/server/server.go && git commit -m "feat(publicapi): phase 2 readers — feeds, scam alerts, semantic search"`.

---

### Task 3: Feed + markdown REST endpoints

**Files:**
- Create: `internal/publicapi/handlers_feeds.go`
- Modify: `internal/publicapi/handlers.go` (`GetContentByID` — add Accept negotiation)
- Modify: `internal/server/server.go` (route registrations inside the public group, after the brief route ~line 4618)
- Test: `internal/publicapi/handlers_feeds_test.go` (stub-based httptest pattern — mirror `handlers_moat_test.go`)

- [ ] **Step 1: Handlers** in `handlers_feeds.go`:

```go
// GetExploreFeed handles GET /api/v1/public/feeds/{name}.
// name ∈ {popular, latest, rising}; ?sort= overrides, ?limit&offset paginate.
func (h *Handlers) GetExploreFeed(w http.ResponseWriter, r *http.Request)

// GetTagFeed handles GET /api/v1/public/feeds/tag/{tag}. ?cursor&limit.
func (h *Handlers) GetTagFeed(w http.ResponseWriter, r *http.Request)

// GetSpaceFeed handles GET /api/v1/public/feeds/space/{slug}. ?sort&limit&offset.
func (h *Handlers) GetSpaceFeed(w http.ResponseWriter, r *http.Request)

// GetMyFeed handles GET /api/v1/public/me/feed — the key owner's for-you feed.
// UserID comes from APIKeyFromContext, never a query param.
func (h *Handlers) GetMyFeed(w http.ResponseWriter, r *http.Request)
```

Each follows the v1 handler shape: nil-Service → 503 `not_ready`; nil keyCtx → 401 `api_key_required`; service errors → `handleServiceError` (extend it to map `ErrInvalidFeedName`→400 `invalid_feed_name`, `ErrSearchUnavailable`→503 `search_unavailable`, `ErrMissingQuery`→400 `missing_query`).

- [ ] **Step 2: Markdown handler** — `GetContentMarkdown` in `handlers_feeds.go` (same fetch as `GetContentByID` — `Queries.GetPublishedContentByID`):

```go
// GET /api/v1/public/content/{id}/markdown → 200 text/markdown; charset=utf-8
// Front matter: title, author, space, canonical URL; then the body verbatim.
// fmt.Fprintf(w, "---\ntitle: %q\nauthor: %s\nspace: %s\ncanonical: %s\n---\n\n%s", …)
```

In `GetContentByID`, add before the JSON write:

```go
if strings.Contains(r.Header.Get("Accept"), "text/markdown") {
    writeContentMarkdown(w, content) // shared with GetContentMarkdown
    return
}
```

- [ ] **Step 3: Register routes** in `server.go` after the `/public/destinations/{slug}/brief` route:

```go
// Agent Data API Phase 2 — feeds, markdown, safety, semantic search
r.With(publicapi.RequireScope("api.read.content")).Get("/public/feeds/tag/{tag}", publicAPIHandlers.GetTagFeed)
r.With(publicapi.RequireScope("api.read.content")).Get("/public/feeds/space/{slug}", publicAPIHandlers.GetSpaceFeed)
r.With(publicapi.RequireScope("api.read.content")).Get("/public/feeds/{name}", publicAPIHandlers.GetExploreFeed)
r.With(publicapi.RequireScope("api.read.me")).Get("/public/me/feed", publicAPIHandlers.GetMyFeed)
r.With(publicapi.RequireScope("api.read.content")).Get("/public/content/{id}/markdown", publicAPIHandlers.GetContentMarkdown)
r.With(features.Gate(featureChecker, "scam_alert.enabled"), publicapi.RequireScope("api.read.safety")).Get("/public/scam-alerts", publicAPIHandlers.ListScamAlerts)
r.With(features.Gate(featureChecker, "scam_alert.enabled"), publicapi.RequireScope("api.read.safety")).Get("/public/scam-alerts/schema", publicAPIHandlers.GetScamAlertSchema)
r.With(features.Gate(featureChecker, "scam_alert.enabled"), publicapi.RequireScope("api.read.safety")).Get("/public/scam-alerts/{id}", publicAPIHandlers.GetScamAlert)
r.With(publicapi.RequireScope("api.read.search"), publicapi.RequireTier(publicAPIService, "pro")).Get("/public/search/semantic", publicAPIHandlers.SemanticSearch)
```

Static `tag`/`space` segments beat the `{name}` param in chi — `/public/feeds/tag` alone hits `{name}` and 400s, which is correct.

**Order caution:** `/public/scam-alerts/schema` before `/public/scam-alerts/{id}` (same static-before-param rule the v1 visa routes followed).

- [ ] **Step 4: Scam-alert + semantic handlers** (same file or `handlers_safety.go`):

```go
func (h *Handlers) ListScamAlerts(w http.ResponseWriter, r *http.Request)
// ?scam_type, ?destination_space_id (parse UUID; invalid → 400), ?limit&offset

func (h *Handlers) GetScamAlertSchema(w http.ResponseWriter, r *http.Request)
func (h *Handlers) GetScamAlert(w http.ResponseWriter, r *http.Request)
// pgx.ErrNoRows → 404 report_not_found (mirrors web handler)

func (h *Handlers) SemanticSearch(w http.ResponseWriter, r *http.Request)
// ?q (required→400 missing_query), ?lang (default "en"), ?space (slug), ?limit (max 50)
```

- [ ] **Step 5: Tests** — stub `FeedReader`/`SafetyReader`/`SemanticSearcher` (same pattern as `handlers_moat_test.go` stubs):

- `{name}` mapping incl. `popular`→`top`; unknown → 400 `invalid_feed_name`
- tag feed 404 `tag_not_found`; cursor round-trip (`next_cursor` only when page full)
- space feed 404 `space_not_found` for non-public space
- me/feed uses `keyCtx.UserID` (assert the stub received it)
- markdown route → `Content-Type: text/markdown`; `Accept: text/markdown` on JSON route → markdown; `Accept: application/json` → JSON
- scam list maps confirmed DTOs; `{id}` no-rows → 404; schema returns options
- semantic: results mapped; nil embedder → 503; `rag.enabled` off → 503; pro-tier gate covered by `RequireTier` tests already, assert scope gate order

- [ ] **Step 6: `go build ./... && go test ./internal/publicapi/ -v` — then commit** — `feat(publicapi): phase 2 feed, markdown, safety & semantic endpoints`.

---

### Task 4: MCP tools + gates + seed migration

**Files:**
- Modify: `internal/ai/tools_deps.go` (extend `AgentDataReader`)
- Modify: `internal/ai/tools_moat.go` (new tool handlers — mirror `destination.brief`/`visa.search`)
- Modify: `internal/server/agent_adapters.go` (extend `agentDataAdapter`)
- Modify: `internal/server/server.go` (extend `apiKeyToolGates` map)
- Create: `migrations/000726_seed_ai_tools_phase2.up.sql` + `.down.sql` (`000722`–`000725` are taken — verify `ls migrations/ | tail` again at write time)
- Test: `internal/mcp/server_test.go`, `internal/ai/tools_moat_test.go` (extend)

- [ ] **Step 1: Extend `AgentDataReader`** in `tools_deps.go`:

```go
ExploreFeed(ctx context.Context, name, sort string, limit, offset int32) (any, error)
TagFeed(ctx context.Context, tagSlug, cursor string, limit int32) (any, error)
SpaceFeed(ctx context.Context, viewerID pgtype.UUID, slug, sort string, limit, offset int32) (any, error)
MyFeed(ctx context.Context, userID pgtype.UUID, sort string, limit, offset int32) (any, error)
ListScamAlerts(ctx context.Context, destSpaceID pgtype.UUID, scamType string, limit, offset int32) (any, int64, error)
GetScamAlert(ctx context.Context, id pgtype.UUID) (any, error)
SemanticSearch(ctx context.Context, query, lang, spaceSlug string, limit int32) (any, error)
```

`TagFeed`/`SpaceFeed` are in the interface because the `feeds.explore` tool
routes to them via its `tag`/`space_slug` args — there are no separate MCP
tools for tag/space feeds.

- [ ] **Step 2: Extend `agentDataAdapter`** — one-line delegations to `publicapi.Service` methods (TagFeed result goes through `feeds.explore`'s `tag` param: adapter calls `TagFeed` when `tag` arg present, `SpaceFeed` when `space_slug` present — see tool handler).

- [ ] **Step 3: Tool handlers** in `tools_moat.go` — follow the existing `destination.brief` handler pattern (parse args → call `deps.AgentData.X` → `json.Marshal` into content block):

```go
"feeds.explore"  — args: name (enum popular|latest|rising, default popular),
                 space_slug (optional), tag (optional), sort, limit
                 — adapter routes: tag→TagFeed, space_slug→SpaceFeed, else ExploreFeed
"feeds.for_you"  — args: sort, limit — needs caller userID: read from deps
                 (check how content.read gets the caller — likely deps.Content
                 carries it or tools get *auth.AuthenticatedUser from ctx)
"scam.list"      — args: destination_space_id, scam_type, limit
"scam.get"       — args: id (required)
"search.semantic"— args: query (required), lang, space, limit
```

For `feeds.for_you` the caller identity comes from `auth.FromContext(ctx)` — verify against an existing user-scoped tool (`content.read`?) and mirror it.

- [ ] **Step 4: Extend `apiKeyToolGates`** in `server.go`:

```go
"feeds.explore":   {scope: "api.read.content"},
"feeds.for_you":   {scope: "api.read.me"},
"scam.list":       {scope: "api.read.safety"},
"scam.get":        {scope: "api.read.safety"},
"search.semantic": {scope: "api.read.search", minTier: "pro"},
```

- [ ] **Step 5: Seed migration** `000726_seed_ai_tools_phase2.up.sql` — copy `000723`'s shape exactly (de-tenanted `ai_tools`, `ON CONFLICT (name)`), five rows with required scopes/permissions mirroring how `destination.brief` was seeded (check 000723 for the exact column set). `.down.sql` deletes the five names.

- [ ] **Step 6: Cycle check + tests** — `go list -deps ./internal/ai | grep publicapi` must show ONLY `internal/publicapi/scopes`. Tests: tool-gate denials (scope + tier), `search.semantic` regular-tier denial, adapter arg routing (tag vs space vs explore), `feeds.for_you` caller identity.

- [ ] **Step 7: `go build ./... && go test ./internal/mcp/ ./internal/ai/ -v` — commit** — `feat(ai): phase 2 agent tools — feeds, scam alerts, semantic search`.

---

### Task 5: OpenAPI + final verification

**Files:**
- Modify: `internal/publicapi/openapi.go` (path entries + schemas for all 9 routes)
- Regenerate: `internal/publicapi/openapi_public_catalog.go` + `public_catalog_openapi.json` via `python3 scripts/generate_openapi_public_catalog_json.py` (check the script's invocation — v1 ran it the same way)

- [ ] **Step 1: Spec paths** — mirror the v1 entries' structure: every route gets `get` with `tags`, `summary`, `security` (apiKey), `x-required-scope`, `x-required-tier` on the semantic route, `x-feature-flag` on scam routes, parameters (path/query with schemas), and response envelopes (`data` + pagination fields). New schemas: `PublicFeedItem`, `PublicScamReport` (exactly the fields defined in Task 2 — moderation internals are already dropped there), `SemanticSearchResult`, `SemanticHit`, `ScamAlertSchema`.

- [ ] **Step 2: Regenerate catalog** — run the script; verify 9 new paths appear in the JSON.

- [ ] **Step 3: Full verification:**

```bash
go build ./...
go vet ./internal/publicapi/ ./internal/mcp/ ./internal/ai/ ./internal/server/ ./internal/rag/
go test ./internal/publicapi/ ./internal/mcp/ ./internal/ai/ -v
```

- [ ] **Step 4: Checklist** — spec coverage: all 9 routes + 5 tools + 2 scopes + 1 Valkey key + 1 setting cataloged; markdown negotiation; `503 search_unavailable`; public-only feed/space/tag/scam data; MCP gates mirror REST tier/scope.

- [ ] **Step 5: Commit** — `feat(publicapi): OpenAPI catalog for phase 2 agent data endpoints`.

---

### Notes for smoke testing (post-implementation, live server)

- `/public/feeds/popular` on real data; `/public/me/feed` with a scoped key (needs the key owner to have a feed — may return empty, still 200).
- `/public/content/{id}/markdown` and `Accept: text/markdown`.
- `/public/scam-alerts` (needs `scam_alert.enabled` + confirmed reports — may be empty).
- `/public/search/semantic` will likely 503 locally (no embedder configured — smoke test showed AI providers skipped) — that IS the expected path; verify it's a clean 503 not a silent empty 200.
- MCP `tools/list` shows the 5 new tools; `search.semantic` hidden/denied for regular tier.
