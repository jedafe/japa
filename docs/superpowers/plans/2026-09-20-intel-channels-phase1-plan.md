# Intel Channels Phase 1 (Spine) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement the source→channels fan-out model, auto-detection probe (preview + verify), deterministic curator path, per-item relevance gate, and trust-gated publishing per `docs/superpowers/specs/2026-09-20-intel-channels-curation-design.md`.

**Architecture:** `intel_sources` gains `categories[]`, `probe_report`, `detected_methods`; new `intel_source_channels` table owns per-entry-point crawl config; scheduler iterates channels; `internal/intelligence/probe` detects channels; `internal/intelligence/curator` produces `Extraction`-shaped records without LLM; a relevance gate filters per-item topicality.

**Tech Stack:** Go (chi, sqlc+pgx/v5, Asynq, pgxpool), PostgreSQL 18, Valkey, SvelteKit 2 + Svelte 5 runes (apps/main port 3000, apps/control port 3002).

## Global Constraints

- Layered architecture: Handler → Service → Store (sqlc only, no hand-written SQL in Go). Migrations via golang-migrate pairs (next number: **000681**).
- Every outbound HTTP request goes through `internal/intelligence/fetcher.safetransport` (SSRF guard).
- Valkey keys prefixed `jc:`; catalog every new key in `.jeos/conventions/valkey-keys.md`, tasks in `asynq-tasks.md`, routes in `api-routes-catalog.md`, settings in `system-settings.md` (Gate 2).
- RBAC nodes seeded per feature-domain migration naming `000NNN_seed_intel_channels_rbac_nodes.sql`.
- Frontend: Rule 16 — no hardcoded option lists; all selects render from `/api/v1/sources/meta`.
- Errors wrap with `fmt.Errorf("...: %w", err)`; handlers use `errpkg.WriteJSON`/`WriteError`.
- Worker deps are package-level vars set by `RegisterIntelDependencies`; new ports must be declared locally in `internal/worker` to avoid the import cycle (worker → extractor → ai → auth → worker).
- Verify each task with `go build ./... && go vet ./...` and focused `go test` runs before committing.

---

### Task 1: Migration + sqlc queries for channels

**Files:**
- Create: `migrations/000681_intel_source_channels.up.sql`, `migrations/000681_intel_source_channels.down.sql`
- Create: `internal/db/queries/intel_channels.sql`
- Modify: generated `internal/db/gen/*` via `sqlc generate`

**Interfaces:**
- Produces: `gen.IntelSourceChannel`, `gen.CreateIntelSourceChannelParams`, `gen.UpdateIntelSourceChannelParams`, `gen.ListDueIntelChannelsParams`, `gen.IntelInboundEvent`, `gen.CreateIntelInboundEventParams`; `gen.IntelSource` gains `Categories []string`, `ProbeReport []byte`, `DetectedMethods []string`, `LastProbedAt pgtype.Timestamptz`.

- [ ] **Step 1: Write the up migration**

```sql
-- 000681_intel_source_channels.up.sql
-- Ref: docs/superpowers/specs/2026-09-20-intel-channels-curation-design.md §1
BEGIN;

ALTER TABLE intel_sources
  ADD COLUMN IF NOT EXISTS categories        text[] NOT NULL DEFAULT '{}',
  ADD COLUMN IF NOT EXISTS probe_report      jsonb  NOT NULL DEFAULT '{}',
  ADD COLUMN IF NOT EXISTS detected_methods  text[] NOT NULL DEFAULT '{}',
  ADD COLUMN IF NOT EXISTS last_probed_at    timestamptz;

ALTER TABLE intel_sources DROP CONSTRAINT IF EXISTS intel_sources_source_kind_check;
ALTER TABLE intel_sources ADD CONSTRAINT intel_sources_source_kind_check
  CHECK (source_kind IN ('publisher','institution','individual','aggregator',
    'channel_profile','institution_site','personal_blog',
    'job_board','gov_portal','community'));

ALTER TABLE intel_records DROP CONSTRAINT IF EXISTS intel_records_status_check;
ALTER TABLE intel_records ADD CONSTRAINT intel_records_status_check
  CHECK (status IN ('extracted','deduped','auto_published','review_required',
    'approved','rejected','published','stale','off_topic'));

CREATE TABLE intel_source_channels (
  id                     uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id              uuid NOT NULL REFERENCES tenants(id),
  source_id              uuid NOT NULL REFERENCES intel_sources(id) ON DELETE CASCADE,
  label                  text NOT NULL,
  entry_url              text NOT NULL,
  crawl_method           text NOT NULL,
  entity_types           text[] NOT NULL DEFAULT '{}',
  pipeline               text NOT NULL DEFAULT 'extract'
                           CHECK (pipeline IN ('extract','curate','hybrid')),
  provider_config        jsonb NOT NULL DEFAULT '{}',
  crawl_frequency_mins   int NOT NULL DEFAULT 1440,
  enabled                boolean NOT NULL DEFAULT true,
  origin                 text NOT NULL DEFAULT 'probe'
                           CHECK (origin IN ('probe','admin','user')),
  relevance_mode         text NOT NULL DEFAULT 'filtered'
                           CHECK (relevance_mode IN ('open','filtered','custom')),
  relevance_topics       text[] NOT NULL DEFAULT '{}',
  relevance_url_patterns text[] NOT NULL DEFAULT '{}',
  last_crawled_at        timestamptz,
  last_success_at        timestamptz,
  last_error_at          timestamptz,
  last_error_message     text,
  last_changed_at        timestamptz,
  consecutive_failures   int NOT NULL DEFAULT 0,
  created_at             timestamptz NOT NULL DEFAULT now(),
  updated_at             timestamptz NOT NULL DEFAULT now(),
  UNIQUE(source_id, entry_url, entity_types)
);
CREATE INDEX idx_intel_channels_due ON intel_source_channels (tenant_id, enabled, crawl_frequency_mins) WHERE enabled;
CREATE INDEX idx_intel_channels_source ON intel_source_channels (source_id);

CREATE TABLE intel_inbound_events (
  id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id    uuid NOT NULL,
  channel_id   uuid NOT NULL REFERENCES intel_source_channels(id) ON DELETE CASCADE,
  payload      bytea NOT NULL,
  headers      jsonb,
  content_type text,
  content_hash text NOT NULL,
  received_at  timestamptz NOT NULL DEFAULT now(),
  processed_at timestamptz,
  status       text NOT NULL DEFAULT 'received'
                 CHECK (status IN ('received','processed','failed','ignored'))
);
CREATE INDEX idx_intel_inbound_channel ON intel_inbound_events (channel_id, received_at DESC);

ALTER TABLE intel_raw_documents
  ADD COLUMN IF NOT EXISTS channel_id uuid REFERENCES intel_source_channels(id) ON DELETE SET NULL;

-- Backfill: categories from category; one channel per existing source.
UPDATE intel_sources SET categories = ARRAY[category] WHERE categories = '{}';

INSERT INTO intel_source_channels
  (tenant_id, source_id, label, entry_url, crawl_method, entity_types,
   pipeline, crawl_frequency_mins, enabled, origin, last_crawled_at,
   last_success_at, last_error_at, last_error_message, last_changed_at,
   consecutive_failures)
SELECT tenant_id, id, 'Default', source_url, crawl_method,
       '{}'::text[],
       'extract', crawl_frequency_mins, enabled, 'admin',
       last_crawled_at, last_success_at, last_error_at,
       last_error_message, last_changed_at, consecutive_failures
FROM intel_sources
WHERE crawl_method <> 'manual';

COMMIT;
```

Down migration drops both new tables/columns and restores the original CHECKs (original `source_kind` set: `publisher|institution|individual|aggregator`; original status set without `off_topic`) — **before** re-adding the status CHECK, re-state `off_topic` rows: `UPDATE intel_records SET status='rejected' WHERE status='off_topic';` (and re-state any new source_kind values to `publisher`). `categories` column dropped after copying `categories[1]` back to `category` where category was empty.

Note on record tracing: `intel_records` does **not** get a `channel_id` column — the channel is reachable transitively via `raw_document_id → intel_raw_documents.channel_id`, and `UNIQUE(source_id, external_key)` dedup intentionally stays source-level (the same job discovered by two channels of one source dedupes, which is desired).

- [ ] **Step 2: Write `internal/db/queries/intel_channels.sql`**

```sql
-- name: CreateIntelSourceChannel :one
INSERT INTO intel_source_channels
  (tenant_id, source_id, label, entry_url, crawl_method, entity_types,
   pipeline, provider_config, crawl_frequency_mins, enabled, origin,
   relevance_mode, relevance_topics, relevance_url_patterns)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)
RETURNING *;

-- name: GetIntelSourceChannel :one
SELECT * FROM intel_source_channels WHERE id = $1 AND tenant_id = $2;

-- name: ListIntelSourceChannels :many
SELECT * FROM intel_source_channels
WHERE source_id = $1 AND tenant_id = $2 ORDER BY created_at;

-- name: ListDueIntelChannels :many
SELECT c.* FROM intel_source_channels c
JOIN intel_sources s ON s.id = c.source_id
WHERE c.tenant_id = $1 AND c.enabled AND s.enabled AND s.status = 'active'
  AND c.crawl_method NOT IN ('manual','email','webhook')
  AND (c.last_crawled_at IS NULL
       OR c.last_crawled_at < now() - make_interval(mins => c.crawl_frequency_mins))
ORDER BY c.last_crawled_at ASC NULLS FIRST LIMIT $2;

-- name: UpdateIntelSourceChannel :one
-- NOTE: array columns are NOT here — COALESCE can't distinguish "don't
-- change" from "set empty". Arrays go through UpdateIntelChannelRelevance
-- (direct assignment, empty allowed) and entity_types through
-- UpdateIntelChannelEntityTypes.
UPDATE intel_source_channels SET
  label = COALESCE($3, label), entry_url = COALESCE($4, entry_url),
  crawl_method = COALESCE($5, crawl_method),
  pipeline = COALESCE($6, pipeline),
  provider_config = COALESCE($7, provider_config),
  crawl_frequency_mins = COALESCE($8, crawl_frequency_mins),
  enabled = COALESCE($9, enabled),
  updated_at = now()
WHERE id = $1 AND tenant_id = $2 RETURNING *;

-- name: UpdateIntelChannelEntityTypes :one
UPDATE intel_source_channels SET entity_types = $3, updated_at = now()
WHERE id = $1 AND tenant_id = $2 RETURNING *;

-- name: UpdateIntelChannelRelevance :one
UPDATE intel_source_channels SET
  relevance_mode = $3, relevance_topics = $4, relevance_url_patterns = $5,
  updated_at = now()
WHERE id = $1 AND tenant_id = $2 RETURNING *;

-- name: GetIntelSourceDefaultChannel :one
-- Legacy intel:fetch_source path resolves the source's oldest enabled
-- channel (the backfilled 'admin' default) to delegate to fetch_channel.
SELECT * FROM intel_source_channels
WHERE source_id = $1 AND tenant_id = $2 AND enabled
ORDER BY created_at ASC LIMIT 1;

-- name: DeleteIntelSourceChannel :exec
DELETE FROM intel_source_channels WHERE id = $1 AND tenant_id = $2;

-- name: UpdateIntelChannelCrawlStatus :exec
UPDATE intel_source_channels SET
  last_crawled_at = now(),
  last_success_at = CASE WHEN $3 THEN now() ELSE last_success_at END,
  last_error_at   = CASE WHEN $3 THEN last_error_at ELSE now() END,
  last_error_message = $4,
  consecutive_failures = CASE WHEN $3 THEN 0 ELSE consecutive_failures + 1 END
WHERE id = $1 AND tenant_id = $2;

-- name: UpdateIntelChannelEnabled :exec
UPDATE intel_source_channels SET enabled = $3, updated_at = now()
WHERE id = $1 AND tenant_id = $2;

-- name: CreateIntelInboundEvent :one
INSERT INTO intel_inbound_events (tenant_id, channel_id, payload, headers, content_type, content_hash)
VALUES ($1,$2,$3,$4,$5,$6) RETURNING *;

-- name: MarkIntelInboundEvent :exec
UPDATE intel_inbound_events SET status = $3, processed_at = now()
WHERE id = $1 AND tenant_id = $2;

-- name: UpdateIntelSourceProbe :exec
UPDATE intel_sources SET probe_report = $3, detected_methods = $4, last_probed_at = now()
WHERE id = $1 AND tenant_id = $2;
```

Also extend `internal/db/queries/intelligence.sql`: add `channel_id` to the raw-document insert query (`CreateIntelRawDocument` gains a nullable `channel_id` param).

- [ ] **Step 3: Run migration + sqlc**

Run: `make migrate-up` (or `migrate -path migrations -database $JC_DATABASE_URL up 1`), then `sqlc generate`.
Expected: `gen.IntelSourceChannel` exists; `go build ./internal/db/gen` clean.

- [ ] **Step 4: Commit** `feat(intel): channels + inbound events schema`

---

### Task 2: Channel domain types + store/service

**Files:**
- Create: `internal/intelligence/sources/channels.go`, `internal/intelligence/sources/channel_store.go`
- Modify: `internal/intelligence/sources/sources.go` (new consts), `service.go` (channel methods)

**Interfaces:**
- Produces: `sources.Channel = gen.IntelSourceChannel`; `Pipeline` consts (`PipelineExtract`, `PipelineCurate`, `PipelineHybrid`); `RelevanceMode` consts (`RelevanceOpen`, `RelevanceFiltered`, `RelevanceCustom`); `Service.{CreateChannel,ListChannels,GetChannel,UpdateChannel,DeleteChannel,UpdateChannelCrawlStatus,ListDueChannels}`.

- [ ] **Step 1: Write failing test** `channels_test.go` — `TestChannelStore_CreateAndList` (against test PG per store-test convention) and a pure validation test: `TestRelevanceMode_IsValid`.

- [ ] **Step 2: Implement `channels.go`**

```go
package sources

type Channel = gen.IntelSourceChannel
type ChannelID = pgtype.UUID

type Pipeline string
const (
  PipelineExtract Pipeline = "extract"
  PipelineCurate  Pipeline = "curate"
  PipelineHybrid  Pipeline = "hybrid"
)

type RelevanceMode string
const (
  RelevanceOpen     RelevanceMode = "open"
  RelevanceFiltered RelevanceMode = "filtered"
  RelevanceCustom   RelevanceMode = "custom"
)

// InboundMethod reports whether m is a push method (never scheduled).
func InboundMethod(m CrawlMethod) bool {
  return m == "email" || m == "webhook"
}
```

New `CrawlMethod` consts in `sources.go`: `CrawlMethodRender`, `CrawlMethodJSONLD`, `CrawlMethodTabular`, `CrawlMethodSitemap`, `CrawlMethodICal`, `CrawlMethodGit`, `CrawlMethodSocial`, `CrawlMethodSearch`, `CrawlMethodWayback`, `CrawlMethodXHRSniff`, `CrawlMethodOneshot`, `CrawlMethodEmail`, `CrawlMethodWebhook`. New `SourceKind` type + consts matching the expanded CHECK.

- [ ] **Step 3: Implement `channel_store.go`** — thin wrappers mirroring `store.go` style (`"sources: <op>: %w"`), tenant-stamped params; service methods delegating with validation (entry_url http/https, entity_types non-empty for extract/curate, method known).

- [ ] **Step 4: Run tests + build** — `go test ./internal/intelligence/sources/... && go build ./...`.

- [ ] **Step 5: Commit** `feat(intel): channel domain types, store, service`

---

### Task 3: Category→method map resolver

**Files:**
- Create: `internal/intelligence/sources/categorymap.go`, `categorymap_test.go`

**Interfaces:**
- Produces: `CategoryMapping{EntityTypes []string; Methods []string; Pipeline Pipeline; Topics []string}`; `ResolveCategoryMap(settingsJSON []byte, categories []string) ResolvedMap` where `ResolvedMap{EntityTypes, Methods []string; Pipeline Pipeline; Topics []string}`; `DefaultCategoryMap() []byte` (embedded JSON matching spec §2).

- [ ] **Step 1: Failing test** — union of entity_types across categories, methods ranked by first-seen order, pipeline = `extract` if ANY category is extract, topics union.

- [ ] **Step 2: Implement** — `//go:embed category_map.json` holding the spec's map; pure functions; deterministic ordering; unknown categories skipped with a logged warning (pass `*slog.Logger` or return warnings slice).

- [ ] **Step 3: Test + build + commit** `feat(intel): category→method map resolver`

---

### Task 4: Probe package — detectors + preview prober

**Files:**
- Create: `internal/intelligence/probe/{probe.go,types.go,detectors.go,platforms.go,probe_test.go}`, `internal/intelligence/probe/testdata/*`

**Interfaces:**
- Produces: `probe.Report`, `probe.ChannelProposal`, `probe.NewProber(client *http.Client /* safetransport-wrapped */, maxRequests int)`; `Prober.Preview(ctx, rawURL, kind string, categories []string) (Report, error)`; `Prober.Verify(ctx, sourceID)` handled by worker task (Task 7).

- [ ] **Step 1: Failing test** — `httptest.Server` serving fixture HTML with `<link rel="alternate" type="application/rss+xml" href="/feed">` + `ld+json` `JobPosting`; `Preview` must report `rss` method, a feed channel proposal, `jsonld_types=["JobPosting"]`, `requires_js=false`.

```go
func TestPreview_DetectsFeedAndJSONLD(t *testing.T) { /* httptest fixture */ }
func TestPreview_SSRFBlocked(t *testing.T)          { /* http://169.254.169.254/ → error */ }
func TestPreview_SPAMarkers(t *testing.T)           { /* <div id="root"></div> + no text → RequiresJS */ }
```

- [ ] **Step 2: Implement detectors** — `Detector` interface `Detect(ctx, page *FetchedPage, out *Report)`; implement feed, sitemap, jsonld (golang.org/x/net/html parse), spa, documents, websub, platform-signature, api-sniff, email-hint detectors. All fetches through the injected `safetransport` client; respect `maxRequests` budget via a shared counter on the probe ctx.

- [ ] **Step 3: Channel proposal ranking** — intersect detected capabilities with `ResolveCategoryMap` output; assign `Pipeline` + `EntityTypes` per proposal; confidence = detector weight (feed 0.95, jsonld 0.9, sitemap 0.7, spa→render 0.6, static fallback 0.3).

- [ ] **Step 4: Test + build + commit** `feat(intel): probe package — detectors + preview prober`

---

### Task 5: Probe endpoint + meta extension

**Files:**
- Modify: `internal/intelligence/handlers/sources_public.go`, `internal/intelligence/handlers/handlers.go`
- Modify: `internal/server/server.go` (route registration)

**Interfaces:**
- Consumes: `probe.Prober`.
- Produces: `POST /api/v1/sources/probe` → `{data: ProbeReport}`; meta gains `"source_kinds"` (expanded) and `"channel_pipelines": ["extract","curate","hybrid"]`.

- [ ] **Step 1: Failing handler test** — `POST /sources/probe` with private-IP URL → 422/400; unauthenticated → 401; over-quota → 429.

- [ ] **Step 2: Implement** `HandleProbeSource` — auth via `requireUser`, decode `{url}`, Valkey `INCR jc:intel:probe_quota:{user_id}` with 1h `EXPIRE` vs `intel.probe.rate_limit_per_hr`, call prober, return report. Wire `Handlers.SetProber(*probe.Prober)`.

- [ ] **Step 3: Extend meta** — `source_kinds` list expands to the 10-kind set with `{value,label}` options; add `channel_pipelines`. The public `categories` list in `HandleGetSourcesMeta` is currently hardcoded — extend it with the new categories (`blog`, `event`, `dataset`, `announcement`, `scam_alert`) or derive it from `schemas.RegisteredCategories()` so the propose form can offer multi-category selection (Task 9 registers them).

- [ ] **Step 4: Test + build + commit** `feat(intel): POST /sources/probe endpoint + meta extension`

---

### Task 6: Propose flow — categories[], source_kind, probe snapshot

**Files:**
- Modify: `internal/intelligence/handlers/sources_public.go` (`proposeSourceRequest`, `HandleProposeSource`), `internal/intelligence/sources/service.go` (`ProposeInput`, `Propose`)
- Test: extend `service_test.go` / handler tests

**Interfaces:**
- `ProposeInput` gains `Categories []string`, `SourceKind string`, `ProbeSnapshot []byte`; `Category` deprecated (set = categories[0]).

- [ ] **Step 1: Failing test** — propose with `categories=["news","job"]`, `source_kind="publisher"` persists both; missing kind → defaults `publisher`; empty categories → 400.

- [ ] **Step 2: Implement** — request fields `categories []string`, `source_kind string`, `probe_snapshot json.RawMessage`; service validates kind against enum, requires ≥1 category, stores `categories`, `probe_report = probe_snapshot`, `source_type` derived (`"auto"`), `crawl_method='manual'`/`crawl_frequency='manual'` (channels will own crawling post-approval). Video-platform URLs: `source_kind=channel_profile` + `isVideoProviderHost` → `video_channel` row (existing §106 logic moved to kind check).

- [ ] **Step 3: Test + build + commit** `feat(intel): propose flow — multi-category + source_kind`

---

### Task 7: Verify probe task + channel materialization

**Files:**
- Create: `internal/worker/intel_probe.go`
- Modify: `internal/worker/tasks.go` (`TypeIntelProbeVerify = "intel:probe_verify"`), `enqueuer.go` (register, queue `default`), `internal/worker/server.go` mux registration
- Modify: `internal/intelligence/sources/service.go` — `ApproveProposed` enqueues verify via a `SetProbeQueue` func (same pattern as `SetMetadataQueue`); `CreateChannelsFromProposals(ctx, sourceID, []probe.ChannelProposal)` (defined via a port interface to avoid cycles — probe types mirrored as plain structs in `sources`).

**Interfaces:**
- Produces: `intel:probe_verify` payload `{version, source_id}`; `Service.MaterializeChannels(ctx, sourceID, proposals []ChannelProposalInput)` where `ChannelProposalInput` is a plain struct in `sources` (no probe import).

- [ ] **Step 1: Failing test** — verify handler loads source, runs deep probe (fixture server), writes `probe_report` + `detected_methods`, creates channels; idempotent on re-run (UNIQUE(source_id, entry_url, entity_types) → upsert-skip).

- [ ] **Step 2: Implement worker handler** — `HandleIntelProbeVerify`: resolve source → `probe.Verify` (deeper walk: sitemap crawl up to `intel.probe.max_urls`, per-category path probing, feed re-check) → `UpdateIntelSourceProbe` → `MaterializeChannels`. Manual/video_channel sources skip cleanly.

- [ ] **Step 3: Wire enqueue on approve** — `ApproveProposed` → `probeQueue(sourceID)` after activation; admin "re-probe" endpoint enqueues the same task.

- [ ] **Step 4: Test + build + commit** `feat(intel): verify probe task + channel materialization`

---

### Task 8: Scheduler → channels; `intel:fetch_channel`

**Files:**
- Modify: `internal/intelligence/scheduler/scheduler.go` (`SelectDue` → channels), `internal/worker/intelligence.go` (new `HandleIntelFetchChannel`; `resolveIntelFetchSource` gains channel resolution)
- Modify: `internal/worker/tasks.go`, `enqueuer.go`, `server.go`

**Interfaces:**
- `scheduler.SelectDue` returns `[]sources.Channel` (joined source fields needed: trust_level, tenant, status). `intel:fetch_channel` payload `{version, channel_id, source_id}`. `intelDetectChangesPayload` and `intelExtractPayload` gain `channel_id` (bump `intelPayloadVersion`).

- [ ] **Step 1: Failing test** — `HandleIntelScheduleCrawls` enqueues `fetch_channel` per due channel; `fetch_channel` persists raw doc with `channel_id` stamped; `detect_changes` propagates `channel_id` into the extract/curate enqueue.

- [ ] **Step 2: Implement** — `HandleIntelFetchChannel`: resolve channel via `GetIntelSourceChannel` + parent source → enabled/manual/`InboundMethod` skips → config check → preflight (lock key `jc:intel:channel_lock:{channel_id}`) → `Composite.Fetch(ctx, source, channel)` → persist raw doc (`channel_id` set) → enqueue `detect_changes` with `channel_id`. `Fetch` signature: `Fetch(ctx context.Context, src sources.Source, ch sources.Channel)`.
- `scheduler.RecordCrawlOutcome` / `EffectiveIntervalMins` callers switch from source_id to **channel_id** — volatility tuning keys become `jc:intel:effective_interval::{channel_id}[:unchanged_count]` per spec.
- Entity-type resolution order in the extract path becomes: `channel.entity_types` → `source.provider_config.entity_types` → `schemas.CategoryToEntityType(category)` (existing fallback — required so backfilled channels with `entity_types='{}'` keep extracting correctly).

- [ ] **Step 3: Backward compat** — `HandleIntelFetchSource` resolves the source's default channel via `GetIntelSourceDefaultChannel` and delegates to `HandleIntelFetchChannel`.

- [ ] **Step 4: Update `Composite.Fetch`** for channel-aware dispatch (phase 1 methods: static/rss/api/pdf/ocr/jsonld/ical/tabular/sitemap; new adapters return `ErrNotImplemented` until Phase 2 — they must not be proposed unless configured).

- [ ] **Step 5: Test + build + commit** `feat(intel): channel-granular scheduler + fetch task`

---

### Task 9: Curator package + `intel:curate_channel`

**Files:**
- Create: `internal/intelligence/curator/{curator.go,rss_news.go,jsonld.go,ical.go,tabular.go,curator_test.go,testdata/*}`
- Modify: `internal/worker/intelligence.go` (`HandleIntelDetectChanges` routes by channel.pipeline → extract or curate; new `HandleIntelCurateChannel`), tasks/enqueuer/server wiring

**Interfaces:**
- `curator.Curate(ctx, body []byte, contentType string, ch sources.Channel) ([]extractor.Extraction, error)` — declared in worker as a port (import-cycle rule). `intel:curate_channel` payload `{version, source_id, channel_id, raw_doc_id}`.

- [ ] **Step 1: Register curate-only entity schemas** — add `schemas/article.go`, `event.go`, `dataset.go` (EntitySchema with plain fields, **no prompt template** — they never reach `buildPrompt`); add registry entries; extend `categoryEntityTypes` (`blog`→`article`, `event`→`event`, `dataset`→`dataset`, `announcement`→`news`, `scam_alert`→`scam_alert` if a schema exists else leave unmapped→review) and `RegisteredCategories()` (add `blog`, `event`, `dataset`, `announcement`, `scam_alert`) so `admin_categories` and the public `categories` meta expose them. Guard: `Extract` on a curate-only type must return a clear error (missing prompt template already does this — add an explicit `ErrCurateOnlyEntityType` check in the extractor port for a cleaner message).

- [ ] **Step 2: Failing test** — RSS fixture → N `Extraction` records with `external_key` = item GUID, confidence 1.0 fields; JSON-LD `JobPosting` fixture → job-shaped extraction; ICS fixture → `event` records.

- [ ] **Step 3: Implement curators** — `rss_news` (gofeed or existing rss parse — check `internal/intelligence/extractor/rss_extract.go` for the existing parser to reuse), `jsonld` (unmarshal ld+json → field map per @type: JobPosting→job fields, NewsArticle→news, Event→event, FAQPage→guide-shaped), `ical` (golang-ical VEVENTs incl. RRULE window), `tabular` (csv/xlsx → `provider_config.column_map`). Each emits `extractor.Extraction` + stable `ExternalKey` (feed GUID / ld+json identifier-or-URL / ICS UID / row hash). Curator package may import `extractor` for the `Extraction` type (the import cycle is only through `worker` — the worker consumes curators via a local `intelCuratorPort` interface with mirrored types, same pattern as `intelExtractorPort`).

- [ ] **Step 4: Worker routing** — in `HandleIntelDetectChanges`, after DecisionChanged/FirstCrawl: load channel → `pipeline=extract` → existing `intel:extract` enqueue (with `channel_id`); `curate`/`hybrid` → `intel:curate_channel`. `HandleIntelCurateChannel` runs curators → per-item relevance gate (Task 10) → records → trust gate (Task 11) → dedup/publish_gate enqueue. `hybrid`: curator records missing required schema fields are routed individually to `intel:extract` (with channel_id); curate-only entity types can't take that path — flag them `review_required` instead.

- [ ] **Step 5: Test + build + commit** `feat(intel): curator package + curate_channel task`

---

### Task 10: Relevance gate

**Files:**
- Create: `internal/intelligence/curator/relevance.go` (or `internal/intelligence/relevance/` package), `relevance_test.go`
- Modify: `internal/worker/intelligence.go` (gate call inside curate + extract item fan-out)

**Interfaces:**
- `relevance.Evaluate(ctx, item ItemInput{Title, Summary, URL}, ch Channel, mapTopics []string) Verdict` → `VerdictOnTopic|VerdictOffTopic|VerdictBorderline`; `relevance.Classifier` port for borderline band.

- [ ] **Step 1: Failing tests** — URL pattern match short-circuits; keyword scorer thresholds; `mode=open` always on-topic; `mode=custom` ignores map topics; borderline → classifier called; classifier verdict cached under `jc:intel:relevance:verdict:{hash}`.

- [ ] **Step 2: Implement** — glob matcher (`path.Match` + `**` support via doublestar or simple prefix rules), token-based scorer (case-insensitive phrase match, score = hits/tokens-weighted), thresholds from settings (`intel.relevance.on_topic_threshold`, `off_topic_threshold`), borderline → AI gateway `category="classification"` bounded by `borderline_rate_cap` per channel-crawl (counter in ctx/Valkey), 7d verdict cache.

- [ ] **Step 3: Wire into worker** — **curate/hybrid path**: each curator item gated *before* record creation; `off_topic` → `intel_records` row status `off_topic` (with `external_key` for dedup) and NOT forwarded to publish_gate. **extract path**: the raw body is not itemized pre-LLM, so gating runs *post-extraction* per returned record (evaluate its `title`/`summary`/`article_url` fields) — off-topic records get status `off_topic` instead of `extracted`.

- [ ] **Step 4: Test + build + commit** `feat(intel): per-item relevance gate`

---

### Task 11: Trust gate — auto-publish curated records

**Files:**
- Modify: `internal/worker/intelligence.go` (`HandleIntelCurateChannel` post-gate routing; publish_gate handler accepts a `direct_publish` flag)

**Interfaces:**
- Curated record routing: `source.trust_level <= intel.curate.auto_publish_trust_max` (default 2) → status `auto_published` → enqueue `intel:promote` directly (skip `publish_gate`); else → normal `dedup` → `publish_gate` → review.

- [ ] **Step 1: Failing test** — trust=1 source + curate pipeline → record lands `auto_published` and promote enqueued; trust=4 → `extracted` + dedup path.

- [ ] **Step 2: Implement** + **Step 3: test/build** + **commit** `feat(intel): trust-gated direct publish for curated records`

---

### Task 12: Admin channel CRUD + RBAC seed

**Files:**
- Modify: `internal/intelligence/handlers/sources_admin.go` (channel handlers)
- Create: `migrations/000682_seed_intel_channels_rbac_nodes.up/down.sql`
- Modify: `internal/server/server.go` routes

**Interfaces:**
- Routes per spec §5; `PATCH` accepts all channel-editable fields; `POST .../reprobe` enqueues `intel:probe_verify`.

- [ ] **Step 1: Failing handler tests** — RBAC denial per node; happy-path CRUD; reprobe enqueues task.

- [ ] **Step 2: Implement handlers** + routes + seed migration (nodes: `intel.channels.list|view|create|update|delete|crawl`, `intel.sources.probe`, `intel.inbound.manage`). The PATCH handler composes up to three sqlc calls: `UpdateIntelSourceChannel` (scalar fields), `UpdateIntelChannelEntityTypes` (when `entity_types` key present — empty array allowed = explicit clear), `UpdateIntelChannelRelevance` (when any `relevance_*` key present). Presence-check via `json.RawMessage` field detection, not zero-value checks.

- [ ] **Step 3: Test + build + commit** `feat(intel): admin channel CRUD + RBAC nodes`

---

### Task 13: Control app — SourceForm + channels editor

**Files:**
- Modify: `apps/control/src/lib/components/intelligence/SourceForm.svelte`, `apps/control/src/routes/intelligence/sources/+page.svelte`
- Create: `apps/control/src/lib/components/intelligence/ChannelEditor.svelte`, `ChannelsTable.svelte`

- [ ] **Step 1:** Multi-category select (checkbox/chips from `meta.admin_categories`) + `source_kind` select in SourceForm.
- [ ] **Step 2:** Channels editor — list channels (method, entity_types, pipeline, enabled, last_error), per-channel edit drawer (all PATCH fields incl. relevance controls), add-channel form, crawl-now + reprobe buttons.
- [ ] **Step 3:** Proposal review shows `probe_report` + proposed channels.
- [ ] **Step 4:** `bun run check`/build + commit `feat(control): channels editor + multi-category source form`

---

### Task 14: Main app — ProposeSourceForm

**Files:**
- Modify: `apps/main/src/lib/components/source/ProposeSourceForm.svelte`, `apps/main/src/lib/types/sources.ts`, i18n `en.json`/`fr.json` keys

- [ ] **Step 1:** Replace `source_type` select with `source_kind` select (from `meta.source_kinds`); categories → multi-select chips.
- [ ] **Step 2:** Live probe preview — debounced `POST /sources/probe` on URL blur; render detected-method chips + proposed channels list; submit `probe_snapshot` with payload.
- [ ] **Step 3:** Types: `SourceMeta` unchanged shape; `categories` submission field.
- [ ] **Step 4:** `bun run check` + commit `feat(main): propose form — source_kind, multi-category, probe preview`

---

### Task 15: Settings seeds + catalogs

**Files:**
- Create: `migrations/000683_seed_intel_channel_settings.up/down.sql`
- Modify: `.jeos/conventions/{api-routes-catalog.md,asynq-tasks.md,valkey-keys.md,system-settings.md}`

- [ ] **Step 1:** Seed settings rows: `intel.category_method_map` (spec §2 JSON), `intel.probe.max_requests=5`, `intel.probe.rate_limit_per_hr=20`, `intel.curate.auto_publish_trust_max=2`, `intel.relevance.*` (classifier_enabled=true, borderline_rate_cap=50, thresholds), `intel.render.service_url=''`, `intel.search.provider=''`, `intel.social.enabled_providers=[]`.
- [ ] **Step 2:** Catalog all new routes/tasks/keys/settings.
- [ ] **Step 3:** `go build ./... && go vet ./...` clean; commit `feat(intel): channel settings seed + catalog updates`

---

## Phase-1 Exit Criteria

- A user submits `jobberman.com` + categories `[news, job]` + kind `publisher` → proposal carries probe snapshot → admin approves → `intel:probe_verify` materializes `/feed` (rss→curate→news_article) and `/jobs` (jsonld→extract→job) channels → scheduler fetches channels → relevance gate drops off-topic items → jobs flow through LLM extraction to `intel_records`; news items curate deterministically, auto-publish if trust ≤2 else review.
- All existing sources keep crawling via their backfilled default channel.

## Out of scope (Phase 2/3 — separate plans)

`render`/`social`/`git`/`wayback`/`oneshot`/`xhr_sniff`/`search` adapters; `email`/`webhook` inbound endpoints; `intel_inbound_events` consumers (table exists now, consumers Phase 3).
