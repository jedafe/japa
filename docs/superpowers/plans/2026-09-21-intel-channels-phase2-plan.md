# Intel Channels Phase 2 (Pull Adapters) Implementation Plan

**Goal:** Implement the seven Phase-2 pull adapters per `docs/superpowers/specs/2026-09-20-intel-channels-curation-design.md` §3/§7: `render`, `sitemap` child-fetch fan-out, `git`, `social`, `wayback`, `oneshot`, `xhr_sniff` — plus the `provider_config.fallback_methods` degradation ladder.

**Architecture:** `Composite.Fetch` gains adapters for all seven methods. Sitemap/wayback manifests fan out to per-URL **derived fetches**: `intel:fetch_channel` payload gains `url_override` + `derived`; child raw docs stamp the new `intel_raw_documents.parent_document_id` column and bypass the manifest-only guard in `detect_changes`. `render`/`xhr_sniff` call an external headless service over HTTP, gated by `intel.render.service_url` (empty = adapter disabled, benign skip — no failure increment). `oneshot` is a never-scheduled channel under a shared system source, fired once by `POST /api/v1/sources/oneshot`.

**Tech Stack:** Go (chi, sqlc+pgx/v5, Asynq, pgxpool), PostgreSQL 18, Valkey, `github.com/go-git/go-git/v5` (new dep — git adapter only).

**Status: implemented.** All 10 tasks landed. Deviations/notes from the
original task text:

- Derived-fetch crawl locks scope to `jc:intel:channel_lock:{channel_id}:{url_hash}`
  (per-URL) — fanned-out siblings share the channel row but must not starve
  each other on the shared channel lock; `CheckChannelDerived` /
  `ReleaseChannelCrawlLockScoped` added to `PreflightChecker`.
- `GetPrevIntelRawDocByChannel` filters `parent_document_id IS NULL` —
  derived docs share `channel_id` and would otherwise become the diff
  baseline.
- `GetLatestIntelRawDocHashForChannelURL` added — a derived page's
  detect_changes compares against its own previous capture, not the
  manifest's hash.
- `resolveChannelProviderConfig` is method-aware: `api` channels keep the
  strict provider-config check (probe-stamped metadata like
  `{"probe_confidence":…}` still falls back to the source config); other
  methods accept any non-empty JSON object — render/social/git channel
  configs are not API-shaped.
- `intel:xhr_sniff` lands on the **low** queue tier; `probe_verify`
  suppresses `render` proposals and skips the sniff enqueue when
  `intel.render.service_url` is unset.
- The render service client and the xhr_sniff HAR client use
  `AllowPrivate` safetransport opts — the render service is operator
  infra typically on a private host; the SSRF guard still applies to the
  URLs it is asked to fetch.
- `oneshot` and `xhr_sniff` are excluded from `ListDueChannels`
  (never scheduled; oneshot is fired by the endpoint, xhr_sniff by
  probe_verify/admin crawl-now redirect).
- Settings seed: `migrations/000687_intel_phase2_settings.*` adds
  `intel.sitemap.max_children_per_crawl` (25). Catalogs updated for the
  new task, route, Valkey keys, and setting.

**Review-hardening pass (post-implementation audit):**

- Manifest docs carry the `application/x-intel-manifest` content-type
  marker — `detect_changes`/`curate_channel` identify manifests by the
  doc, not the channel row, so the guard survives channel
  deletion/repurposing (docs' `channel_id` is ON DELETE SET NULL).
- `GetLatestIntelRawDocHashByChannel` and `GetPrevIntelRawDocByChannel`
  both filter `parent_document_id IS NULL` — otherwise child docs would
  become the manifest's diff/hash baseline and every crawl would read as
  "changed".
- Derived children do not stamp `channel.last_crawled_at`, do not record
  crawl outcomes/`MarkChannelChanged` (a 25-child fan-out would skew the
  adaptive-interval signal), and their fetch/preflight failures do not
  increment the channel's `consecutive_failures` (5 dead manifest URLs
  must not auto-disable a healthy channel). They still return errors so
  Asynq retry policy applies to the child task.
- Derived docs of manifest channels route straight to `extract` — the
  curator registry dispatches on `crawl_method` and none handles
  sitemap/wayback, so `curate` was a guaranteed dead hop.
- `ErrAdapterDisabled` on the *primary* method enters the
  `fallback_methods` ladder (render-unconfigured → static is the designed
  degradation); if all hops fail the original error is returned,
  preserving benign-skip semantics.
- xhr_sniff proposals carry the real HTTP method (GET/POST) — a POST
  endpoint materialized as `method:GET` would 405 forever.
- The oneshot system source uses `source_type='system'` so
  `verifySkipSource` skips admin reprobes (prevents probing
  japaconnect.com and materializing channels onto the shared source).

## Global Constraints

- Layered architecture: Handler → Service → Store (sqlc only). Migrations via golang-migrate pairs (next number: **000686**).
- Every outbound HTTP request goes through `internal/intelligence/fetcher` safetransport (SSRF dial guard + redirect guard + body cap). git uses go-git transport with its own host allowlist check.
- `ErrAdapterDisabled` sentinel: adapter present but not configured (render service URL empty, social provider not enabled) → fetch returns nil-result benign skip; `handleChannelFetchFailure` must NOT increment `consecutive_failures`.
- New task names, routes, Valkey keys, and settings must land in the JEOS catalogs in the same change (Gate 2): `intel:xhr_sniff`, `POST /api/v1/sources/oneshot`, `jc:intel:render:circuit`, `intel.sitemap.max_children_per_crawl`.
- Worker deps stay package-level via `RegisterIntelDependencies`; new ports declared locally in `internal/worker` (import cycle).
- Verify each task with `go build ./... && go vet ./...` + focused `go test`.

---

### Task 1: `parent_document_id` + derived-fetch plumbing + sitemap fan-out

**Files:**
- Create: `migrations/000686_intel_raw_doc_parent.up.sql` / `.down.sql`
- Modify: `internal/db/queries/intel_channels.sql` (+ `internal/db/queries/intelligence.sql` if the raw-doc insert needs the column), regenerate `internal/db/gen`
- Modify: `internal/intelligence/fetcher/sitemap.go`-adjacent code (`channel_methods.go` — manifest diff helper)
- Modify: `internal/worker/intelligence.go` (payloads, fetch_channel url_override/derived, detect_changes fan-out)
- Modify: `.jeos/conventions/system-settings-catalog.md`, `valkey-keys-catalog.md` (if new keys)

**Details:**

```sql
ALTER TABLE intel_raw_documents
  ADD COLUMN IF NOT EXISTS parent_document_id uuid
    REFERENCES intel_raw_documents(id) ON DELETE SET NULL;
CREATE INDEX IF NOT EXISTS idx_intel_raw_documents_parent
  ON intel_raw_documents(parent_document_id) WHERE parent_document_id IS NOT NULL;
```

- New query `GetPrevIntelRawDocByChannel` — latest raw doc for a channel excluding a given doc id (manifest diff needs the previous body).
- `intelFetchChannelPayload` gains `URLOverride string` + `Derived bool`. Fetch resolves `entry_url = override ?? channel.entry_url`; override fetches use the `static` adapter regardless of channel method.
- `intelDetectChangesPayload` gains `Derived bool`. `detect_changes` manifest guard checks `rawDoc.ParentDocumentID` (or payload flag) — derived docs dispatch to the channel's pipeline normally; the manifest itself still terminates after outcome recording.
- `detect_changes` on a **changed** sitemap manifest: parse current + previous manifest (`url\tlastmod` lines), diff → URLs that are new or have a newer lastmod → enqueue `intel:fetch_channel{url_override, derived:true}` per URL, bounded by `provider_config.max_urls` else `intel.sitemap.max_children_per_crawl` (new setting, default 25). `provider_config.url_filter` (glob on path) pre-filters.
- `handleChannelFetchFailure` and preflight treat `ErrAdapterDisabled` like `ErrCrawlLocked` — log + nil, no failure increment.

---

### Task 2: Render adapter + circuit breaker

**Files:**
- Create: `internal/intelligence/fetcher/render.go` (+ `_test.go`)
- Modify: `internal/intelligence/fetcher/composite.go`, `internal/worker/intelligence.go` (disabled-skip handling), `internal/server/intel_adapters.go` (or wherever the composite is constructed — inject settings reader)
- Catalog: `jc:intel:render:circuit` in valkey-keys-catalog.md

**Contract** (internal — documented in code + plan): `POST {intel.render.service_url}/render` JSON `{url, wait_selector?, timeout_ms, a11y}` → `200 {html, final_url}`; non-2xx/timeout = failure. Request uses the safetransport client (render service is operator-deployed but still validated), `JC_INTERNAL_TOKEN`-style bearer auth if configured via provider_config `auth_token`.

- `intel.render.service_url` empty → `ErrAdapterDisabled`.
- Circuit breaker: `jc:intel:render:circuit` SET EX 300 on N consecutive render failures (N=3); while set, fetches return `ErrAdapterDisabled` (service flap doesn't churn channels).
- `a11y=true` → service returns accessibility-tree text instead of HTML (passed through as body with `text/plain` content type).

---

### Task 3: Git adapter

**Files:**
- Create: `internal/intelligence/fetcher/git.go` (+ `_test.go`)
- `go get github.com/go-git/go-git/v5` (pin a ≥7-days-old release)

**Details:**
- `provider_config{repo, branch, paths[], via: "files"|"tags"}`. `repo` defaults to channel `entry_url`; host must parse to a public host (SSRF — go-git doesn't use our transport, so validate the host is not a private literal; note the DNS-resolution caveat in code comment).
- `via: "files"` — in-memory shallow clone (`memory.NewStorage()` + `memfs.New()`, `Depth: 1`, `SingleBranch: true`, `Tags: NoTags`), read each configured path (or all text files under a size cap when paths empty), body = `{path}\n{sha256}\n{content}` blocks; content hash over the file-set.
- `via: "tags"` — `ls-remote`-style remote tag listing only (no clone), body = `tag\ttarget-sha` manifest.
- `via: "releases"` → warn + fall back to `tags` (GitHub releases API is out of Phase-2 scope).

---

### Task 4: Wayback adapter

**Files:**
- Create: `internal/intelligence/fetcher/wayback.go` (+ `_test.go`)

**Details:**
- Fetch `https://web.archive.org/cdx/search/cdx?url={host-path}&output=json&filter=statuscode:200&collapse=digest` (+ `&since=` from `provider_config.since` or channel `last_success_at`) via safetransport.
- Body = `timestamp\toriginal_url` manifest (same diff contract as sitemap) → changed → derived child fetches of `https://web.archive.org/web/{ts}id_/{original}`.
- `provider_config{since, field, max_snapshots}`; `max_snapshots` default 25.

---

### Task 5: Social adapter

**Files:**
- Create: `internal/intelligence/fetcher/social.go` (+ `_test.go`)

**Details:**
- `provider_config{provider, handle, endpoint_override}`; provider must be in `intel.social.enabled_providers` (jsonb array) else `ErrAdapterDisabled`.
- `reddit` → `https://www.reddit.com/{handle}/new.json?limit=50` (or `user/{handle}.json`) → normalize to JSON-lines `{id,title,url,author,created_utc,score,selftext_trunc}`.
- `telegram` → `https://t.me/s/{handle}` → goquery over `.tgme_widget_message` blocks → JSON-lines `{id,text,url,date}`.
- Body → channel pipeline (typically `extract`; curate/hybrid get a `social` curator only if trivially mappable — otherwise the existing `ErrIntelNoCurator`→extract fallback applies).
- Polite UA string via `provider_config.ua` default.

---

### Task 6: `oneshot` endpoint + never-scheduled channel

**Files:**
- Modify: `internal/intelligence/handlers/sources_public.go` (new `HandleOneshotSource`), route registration, `internal/intelligence/sources/service.go` (oneshot channel creation under shared system source)
- Modify: `internal/db/queries/intel_channels.sql` — `ListDueIntelChannels` excludes `oneshot` (add to the manual/inbound exclusion)
- Catalog: `POST /api/v1/sources/oneshot` in api-routes-catalog.md

**Details:**
- `POST /api/v1/sources/oneshot` `{url, categories?, entity_hint?}` (authenticated; shares `jc:intel:probe_quota` budget — same "fetch a user URL" action class).
- Lazily creates a system source (slug `user-oneshot-submissions`, `source_kind='community'`, `status='active'`, trust_level default 5 → review queue).
- Creates a channel `{entry_url: url, crawl_method: 'oneshot', pipeline: 'hybrid', origin: 'user', enabled: false}` — UNIQUE dedupe returns the existing channel on re-submit.
- Enqueues `intel:fetch_channel` directly (oneshot bypasses enabled check for this explicit trigger — or keep enabled=true + `oneshot` excluded from `ListDueChannels`; choose the latter for simpler semantics: channel row reflects "crawlable", scheduler just never picks it).
- Response `202 {channel_id, status: "queued"}`; records land in intel_records review queue.

---

### Task 7: `intel:xhr_sniff` task

**Files:**
- Create: `internal/worker/intel_xhrsniff.go` (+ test)
- Modify: worker task registration + `QueueForTask`, enqueuer opts
- Catalog: `intel:xhr_sniff` in asynq-tasks.md (low queue)

**Details:**
- Payload `{version, source_id, channel_id?}`. Load source (skip disabled/inactive + verifySkipSource), `intel.render.service_url` empty → log + nil.
- POST `{service_url}/har {url: entry|source_url, wait_ms}` → parse HAR `log.entries[].request/response` → keep entries whose response `mimeType` contains json → dedupe by `method + host + normalized path` → each becomes an api `ChannelProposalInput` → `MaterializeChannels` (origin='probe', pipeline='extract').
- `probe_verify` enqueues `intel:xhr_sniff` when `report.RequiresJS && service_url != ""`.
- Admin "crawl now" on an `xhr_sniff` channel routes to this task instead of `fetch_channel`.

---

### Task 8: `fallback_methods` degradation ladder

**Files:**
- Modify: `internal/intelligence/fetcher/composite.go`

**Details:**
- `provider_config.fallback_methods []string` — on a fetch error (not `ErrAdapterDisabled`/preflight rejections), try each method in order against the same `entry_url`, bounded to 3 hops, never repeating a method. First success wins; all fail → original error.
- `manual`/inbound/oneshot methods never appear in a ladder (validated).

---

### Task 9: Composite dispatch + pending-method cleanup + probe gating

- `Composite.Fetch` switch gains `render`, `git`, `social`, `wayback`, `oneshot`, `xhr_sniff` (`oneshot` → static path; `xhr_sniff` → `ErrAdapterDisabled`/never fetched directly — its work is the task).
- `pendingFetchMethods` shrinks to `{search}` (Phase 3); `email`/`webhook`/`manual` stay non-fetchable.
- `HandleIntelProbeVerify` additionally drops `render` proposals when `intel.render.service_url` is empty (settings-readable in worker) and `xhr_sniff` handling per Task 7.

---

### Task 10: Catalogs, settings seed, tests, verification

- `asynq-tasks.md`: `intel:xhr_sniff`.
- `api-routes-catalog.md`: `POST /api/v1/sources/oneshot`.
- `valkey-keys-catalog.md`: `jc:intel:render:circuit`.
- `system-settings-catalog.md`: `intel.sitemap.max_children_per_crawl`; seed migration `000687_seed_intel_phase2_settings` (idempotent upserts).
- Unit tests: manifest diff, render contract (httptest), git adapter (fixture repo via go-git memory fs or a local `git init` fixture), wayback CDX parse, social normalizers, oneshot dedupe, fallback ladder ordering, disabled-adapter skip semantics.
- `go build ./... && go vet ./...` + `go test ./internal/...` clean.

### Risks

- `render`/`xhr_sniff` are unverifiable end-to-end without a deployed service — mitigate with httptest contract tests + `ErrAdapterDisabled` clean-skip semantics so unconfigured channels never churn.
- go-git clone of a hostile repo — bound by depth=1, memory fs, size cap on read files.
- Derived-fetch fan-out cardinality — bounded by per-crawl caps + url_filter.
