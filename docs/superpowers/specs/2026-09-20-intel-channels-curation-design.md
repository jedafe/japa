# Intelligence Channels, Auto-Probe & Trust-Gated Curation

**Date:** 2026-09-20
**Status:** Draft — awaiting review
**Scope:** Source submission UX (propose form + control panel), source auto-detection probe, `intel_source_channels` fan-out model, new fetcher adapters, deterministic curator path, trust-gated publishing.
**Approach:** Source → channels. Each `intel_sources` row owns one or more channels (entry point + crawl method + entity pipeline). The scheduler iterates channels; a two-stage probe discovers them automatically.
**Refs:** docs/superpowers/specs/2026-08-12-japa-intelligence-system-design.md (Sections 3–4), docs/superpowers/specs/2026-09-11-sources-directory-design.md, ADR-112, §106 channel-sync amendment, .jeos/AGENTS.md Rules 12, 16

---

## Background

The intelligence spine (`fetch → detect_changes → extract → dedup → promote`) currently supports five crawl methods (`static`, `rss`, `api`, `pdf`, `ocr`) selected manually per source, a single `category` per source, and routes every record through LLM extraction. This design extends it to:

1. **Multi-category sources.** A user proposing `jobberman.com` with categories `news` + `job` gets separate discovered channels — a `/jobs` JSON-LD channel feeding the `job` extractor, and a `/feed` RSS channel curating `news_article` records — rather than one crawl of the landing page.
2. **Auto-detected crawl methods.** Users no longer pick `source_type`; a probe inspects the URL and proposes channels with the right method automatically.
3. **Direct curation.** Deterministic content types (RSS items, JSON-LD, ICS events, CSV rows) bypass LLM extraction entirely; trust level decides whether curated records auto-publish or queue for review.
4. **Publisher kinds.** `source_kind` expands to cover channels/profiles on community platforms, institution sites, personal blogs, job boards, and government portals — and drives probe behavior (e.g. YouTube → channel sync, Medium → `/feed`).

---

## Decisions

1. **Channels are first-class rows** (`intel_source_channels`), not JSONB on the source or multiple source rows. Each channel is independently schedulable, admin-editable, and carries its own crawl bookkeeping. Rejected alternatives: channels-in-`provider_config` (no per-channel scheduling/status) and one-source-per-entry-point (directory pollution).
2. **Curation output reuses the `Extraction` shape.** Curators emit `map[field]→{value,confidence}` records identical to LLM output, so validation, `intel_records`, dedup, and the promoter are reused unchanged.
3. **Trust gate decides publish path.** Curated records from `trust_level ≤ 2` sources auto-publish to `contents`; `trust_level ≥ 3` records enter the review queue. LLM `extract` pipelines always go through `intel_records` regardless of trust.
4. **Probe runs twice.** A fast `preview` probe powers live UX in the propose form; a deeper `verify` probe runs on admin approval and materializes channels.
5. **Category→pipeline mapping is configuration**, stored in `system_settings.intel.category_method_map` — tunable without redeploy.
6. **`source_type` becomes non-user-facing.** The propose form drops it; the backend derives it from `source_kind` + detected methods. The column stays for descriptive/backcompat use.
7. **`intel_sources.category` stays** for backward compat (first selected category); `categories[]` is the real multi-select axis.
8. **Push methods are never scheduled.** `email` and `webhook` channels ingest via inbound endpoints writing `intel_inbound_events`; `ListDueChannels` excludes them.

---

## 1. Data model

### `intel_sources` alterations

```sql
ALTER TABLE intel_sources
  ADD COLUMN categories        text[] NOT NULL DEFAULT '{}',
  ADD COLUMN probe_report      jsonb  NOT NULL DEFAULT '{}',
  ADD COLUMN detected_methods  text[] NOT NULL DEFAULT '{}',
  ADD COLUMN last_probed_at    timestamptz;

-- existing source_kind CHECK constraint dropped and replaced with:
-- ('publisher','institution','individual','aggregator',
--  'channel_profile','institution_site','personal_blog',
--  'job_board','gov_portal','community')
-- existing 'publisher' default retained.
```

Backfill: `categories = ARRAY[category]`.

### `intel_source_channels` (new)

```sql
CREATE TABLE intel_source_channels (
  id                   uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id            uuid NOT NULL REFERENCES tenants(id),
  source_id            uuid NOT NULL REFERENCES intel_sources(id) ON DELETE CASCADE,
  label                text NOT NULL,                  -- "Jobs listing", "Blog feed"
  entry_url            text NOT NULL,
  crawl_method         text NOT NULL,                  -- extended enum (below)
  entity_types         text[] NOT NULL DEFAULT '{}',   -- {job}, {news_article}…
  pipeline             text NOT NULL DEFAULT 'extract'
                         CHECK (pipeline IN ('extract','curate','hybrid')),
  provider_config      jsonb NOT NULL DEFAULT '{}',
  crawl_frequency_mins int NOT NULL DEFAULT 1440,
  enabled              boolean NOT NULL DEFAULT true,
  origin               text NOT NULL DEFAULT 'probe'
                         CHECK (origin IN ('probe','admin','user')),
  last_crawled_at      timestamptz,
  last_success_at      timestamptz,
  last_error_at        timestamptz,
  last_error_message   text,
  last_changed_at      timestamptz,
  consecutive_failures int NOT NULL DEFAULT 0,
  created_at           timestamptz NOT NULL DEFAULT now(),
  updated_at           timestamptz NOT NULL DEFAULT now(),
  UNIQUE(source_id, entry_url, entity_types)
);
```

`intel_raw_documents.channel_id uuid NULL REFERENCES intel_source_channels(id)` added — raw docs trace to the producing channel.

Existing sources get one backfilled channel each (`entry_url = source_url`, `crawl_method`, `crawl_frequency_mins`, entity_types derived from category via the map, `origin='admin'`). Source-level `crawl_method`/`crawl_frequency*` remain as defaults new channels inherit.

### `intel_inbound_events` (new — push sources)

```sql
CREATE TABLE intel_inbound_events (
  id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id    uuid NOT NULL,
  channel_id   uuid NOT NULL REFERENCES intel_source_channels(id),
  payload      bytea NOT NULL,
  headers      jsonb,
  content_type text,
  content_hash text NOT NULL,
  received_at  timestamptz NOT NULL DEFAULT now(),
  processed_at timestamptz,
  status       text NOT NULL DEFAULT 'received'
                 CHECK (status IN ('received','processed','failed','ignored'))
);
```

### New `crawl_method` values

`render`, `jsonld`, `tabular`, `sitemap`, `ical`, `git`, `social`, `search`, `wayback`, `xhr_sniff`, `oneshot`, `email`, `webhook` — alongside existing `static`, `rss`, `api`, `pdf`, `ocr`, `manual`.

### New `Category` constants

`news`, `job`, `scholarship`, `visa`, `event`, `blog`, `video`, `dataset`, `fx`, `remittance`, `travel_alert`, `scam_alert`, `announcement` (existing scholarship/job/visa/fx/remittance/news/travel_alert retained).

---

## 2. Probe subsystem

New package `internal/intelligence/probe`. Input: candidate URL + `source_kind` hint + selected categories. Output:

```go
type ProbeReport struct {
  FinalURL        string            // after redirects
  DetectedKinds   []string          // platform signatures: youtube, medium, github…
  Methods         []DetectedMethod  // ranked methods with evidence
  Channels        []ChannelProposal // proposed intel_source_channels rows
  JSONLDTypes     []string          // schema.org @types found
  FeedURLs        []string
  SitemapURLs     []string
  HasWebSub       bool
  RequiresJS      bool              // SPA markers → suggests render
  Robots          RobotVerdict      // via existing fetcher/robots.go
  ProbedAt        time.Time
}

type ChannelProposal struct {
  Label       string
  EntryURL    string
  CrawlMethod string
  EntityTypes []string
  Pipeline    string  // extract | curate | hybrid
  Confidence  float64
}
```

### Detectors (parallel `Detector` implementations)

| Detector | Signal | Produces |
|---|---|---|
| Feed discovery | `<link rel="alternate">`, `/feed`, `/rss`, `?feed=rss2`, `/.well-known/` | `rss` channel per feed |
| Sitemap | `/sitemap.xml`, robots.txt `Sitemap:` | `sitemap` channel + sub-URL discovery for `verify` |
| JSON-LD | `application/ld+json` `@type` | `jsonld`/`curate` channel; types → entity_types (JobPosting→job, NewsArticle→news_article) |
| SPA markers | empty `<body>`, `__NEXT_DATA__`, `#root`/`#app`, JS framework signatures | `RequiresJS` → `render` fallback |
| Documents | PDF/DOCX links, `.csv`/`.xlsx` endpoints, `.ics` links | `pdf`/`tabular`/`ical` channels |
| WebSub | `Link: <hub>` headers, feed `<link rel="hub">` | `webhook` channel + hub config |
| Platform signature | host allowlist: youtube→video_channel sync, medium→`/feed`, github→`git`, reddit→`.json`, telegram→social | provider-specific channels |
| API sniff | OpenAPI/`.json` well-known endpoints | `api` channel candidates |
| Email hint | `mailto:`, newsletter signup forms | `email` channel proposal (admin wires inbound address) |

### Stages

- **`preview`** — single GET + robots.txt + HEAD probes of a shortlist of common paths. Budget ~5 requests / 10s. Served by `POST /api/v1/sources/probe` (authenticated; Valkey quota `jc:intel:probe_quota:{user_id}`; all outbound via `safetransport` SSRF guard). Powers live chips in the form.
- **`verify`** — Asynq `intel:probe_verify` on admin approval. Walks sitemap, probes per-category discovery paths (`/jobs`, `/blog`, `/careers` when categories include job/news), re-checks feeds, writes final channels + `probe_report`. Admin can edit/disable channels before activation.

### Category → pipeline map (`system_settings.intel.category_method_map`)

```json
{
  "job":        {"entity_types": ["job"],          "methods": ["jsonld","api","tabular","render","static"], "pipeline": "extract"},
  "news":       {"entity_types": ["news_article"],  "methods": ["rss","jsonld","render","static"],         "pipeline": "curate"},
  "blog":       {"entity_types": ["article"],       "methods": ["rss","render","static"],                  "pipeline": "curate"},
  "event":      {"entity_types": ["event"],         "methods": ["ical","jsonld"],                          "pipeline": "curate"},
  "scholarship":{"entity_types": ["scholarship"],   "methods": ["rss","jsonld","render","static"],         "pipeline": "extract"},
  "visa":       {"entity_types": ["visa"],          "methods": ["static","pdf","render"],                  "pipeline": "extract"},
  "dataset":    {"entity_types": ["dataset"],       "methods": ["tabular","git","api"],                    "pipeline": "curate"},
  "video":      {"entity_types": ["video"],         "methods": ["channel_sync"],                           "pipeline": "curate"}
}
```

The probe intersects detected capabilities × selected categories to rank channel proposals.

---

## 3. Fetcher adapters

All implement `Fetch(ctx, channel) → FetchResult` (signature gains the channel). Each declares a `provider_config` schema with a per-method parser.

| Method | Behavior | `provider_config` | Notes |
|---|---|---|---|
| `render` | Headless-render entry_url → post-JS DOM (or accessibility tree when `a11y=true`) | `{render_service_url, wait_selector, timeout_ms, a11y}` | chromedp pool or remote Playwright service; gated by `intel.render.service_url` |
| `jsonld` | Fetch page → extract `ld+json` blocks → raw JSON array body | `{jsonld_types: ["JobPosting"]}` | Structured output; extractor skipped or cheap |
| `tabular` | CSV/XLSX/TSV endpoint → normalized JSON lines | `{format, sheet, header_row}` | `encoding/csv` + xlsx lib; per-row hashing |
| `sitemap` | Sitemap index → URL+lastmod manifest as body | `{url_filter, max_urls}` | Manifest hash drives change detection; per-URL fetches become child raw docs |
| `ical` | `.ics` → VEVENTs incl. RRULE window expansion | `{window_days}` | `go-ical` / `arran4/golang-ical` |
| `git` | Shallow clone → read configured paths → diff vs last commit/tag | `{repo, branch, paths[], via: "releases|tags|files"}` | `go-git`; hash = file-set hash |
| `social` | Per-provider sub-adapters: reddit `.json`, telegram public channels, oEmbed | `{provider, handle, endpoint_override}` | Provider allowlist via `intel.social.enabled_providers`; ToS-respecting |
| `search` | SERP query → results as fetched body | `{query, provider, result_count, recency}` | `intel.search.provider` + encrypted `intel.search.api_key`; dedupe by result URL |
| `wayback` | CDX API → snapshot list + capture diffs | `{since, field}` | `web.archive.org/cdx/search/cdx`; powers "what changed" intel |
| `xhr_sniff` | Headless pass capturing XHR/fetch (HAR) → discovered JSON APIs | `{har_capture, promote_to_api: true}` | Writes discovered endpoints as new `api` channels (`origin='probe'`) |
| `oneshot` | Single fetch of a user-submitted URL (share-target/bookmarklet) | `{}` | Never scheduled; fires once on creation |
| `email` | Inbound only — provider webhook → `intel_inbound_events` → MIME parse → raw doc | `{inbound_address, allowed_senders}` | Signature-verified endpoint |
| `webhook` | Inbound — WebSub subscribe/renew + push callback | `{hub, topic, secret}` | `intel:websub_*` tasks own subscription lifecycle |

`Composite.Fetch` switches on `channel.crawl_method`. Push methods (`email`, `webhook`) are excluded from `ListDueChannels`. `render`/`xhr_sniff`/`social` share headless infra.

**Hardening applying to all adapters:** per-channel circuit breaker (`consecutive_failures` → auto-disable at channel level), per-method timeout budgets, response size caps, `safetransport` on every outbound request, and a `provider_config.fallback_methods` degradation ladder (e.g. `rss` → `jsonld` → `render` → `static`).

---

## 4. Curator path & category routing

New package `internal/intelligence/curator`. Deterministic normalizers emit the existing `Extraction` shape (`map[field]→{value, confidence}`), so validation → `intel_records` → dedup → promoter are reused unchanged.

```go
type Curator interface {
  Curate(ctx context.Context, raw RawDoc, channel Channel) ([]extractor.Extraction, error)
  Handles() string // "rss_news" | "jsonld" | "ical" | "tabular" | "git" | …
}
```

| Curator | Input | Output |
|---|---|---|
| `rss_news` | RSS/Atom feed | one `news_article`/`article` record per item (confidence 1.0) |
| `jsonld` | ld+json blocks | `JobPosting`→job, `NewsArticle`→news, `Event`→event, `FAQPage`→guide |
| `ical` | VEVENTs | `event` records (RRULE → recurrence) |
| `tabular` | CSV/XLSX rows | one record per row via `provider_config.column_map` |
| `git` | repo JSON/CSV files | records per file/object |
| `websub`/`email`/`oneshot` | inbound payload | feed-item or article records |

**Pipeline routing** (`channel.pipeline`):

```
fetch_channel → detect_changes → ┌ extract (LLM)    → intel_records
                                 ├ curate  (deterministic) → intel_records OR direct
                                 └ hybrid  (curate first; missing required fields → LLM for that record only)
```

**Trust gate** after curate:
- `trust_level ≤ 2` → status `auto_published` → promoter writes `contents` row (`content_type='news'|'article'|'event'`) immediately.
- `trust_level ≥ 3` → `intel_records` `extracted` → dedup → review queue → promote on approval.
- LLM `extract` pipelines always go through `intel_records` (confidence machinery).

**Contract:** `channel.entity_types[]` is the single source of truth for which schema validates each record. `intel_records.external_key` = stable per-item key (feed GUID, JSON-LD `identifier`/URL, ICS UID, row hash) so re-crawls dedupe and editorial overrides keep winning.

---

## 5. Forms & API surface

### Propose form (`apps/main` — `ProposeSourceForm`)

- Fields: name, source_url, domain (optional), **`source_kind`** select (replaces `source_type`), **categories multi-select**, description.
- Live preview: URL blur → `POST /api/v1/sources/probe` → detected-method chips + proposed-channels preview.
- Payload: `{name, source_url, domain, categories[], source_kind, description, probe_snapshot}` — no `source_type`.
- `GET /api/v1/sources/meta` extended for kinds/categories/labels (Rule 16 — no hardcoded lists).

### Control form (`apps/control` — `SourceForm`)

- Same `source_kind` + multi-category inputs; admin keeps crawl-method override.
- New channels editor on source detail: list/add/edit/disable/crawl-now per channel; "Re-probe" enqueues `intel:probe_verify`.
- Proposal review shows `probe_report` + proposed channels with per-channel approve/edit/reject.

### Endpoints

```
POST   /api/v1/sources/probe                       — live preview probe (auth, rate-limited)
POST   /api/v1/sources/propose                     — categories[], source_kind (source_type dropped)
POST   /api/v1/sources/oneshot                     — share-target single-URL curate
GET    /api/v1/admin/intel/sources/:id/channels
POST   /api/v1/admin/intel/sources/:id/channels
PATCH  /api/v1/admin/intel/channels/:id
DELETE /api/v1/admin/intel/channels/:id
POST   /api/v1/admin/intel/channels/:id/crawl
POST   /api/v1/admin/intel/sources/:id/reprobe
POST   /api/v1/intel/inbound/email                 — provider inbound webhook (signature-verified)
GET    /api/v1/intel/inbound/websub/:channel_id    — WebSub verify challenge
POST   /api/v1/intel/inbound/websub/:channel_id    — WebSub push delivery
```

**RBAC nodes** (dedicated seed migration `000NNN_seed_intel_channels_rbac_nodes.sql`): `intel.channels.{list,view,create,update,delete,crawl}`, `intel.sources.probe`, `intel.inbound.manage`.

**Validation:** `source_kind` required (default `publisher`); `categories` ≥ 1; URL uniqueness unchanged; video-platform URLs map to `video_channel` via `source_kind=channel_profile` + platform detection (§106 preserved).

---

## 6. Infra

### Asynq tasks

| Task | Queue | Trigger | Purpose |
|---|---|---|---|
| `intel:probe_verify` | default | approve / re-probe | Deep probe → `probe_report` + channels |
| `intel:fetch_channel` | default | scheduler | Channel fetch → raw doc → detect_changes |
| `intel:curate_channel` | default | detect_changes on curate/hybrid | Curator → records → trust gate |
| `intel:inbound_email` | critical | inbound webhook | MIME parse → raw doc → pipeline |
| `intel:websub_subscribe` | low | webhook channel create | Hub subscribe; store lease |
| `intel:websub_renew` | low | scheduled before lease expiry | Renew subscriptions |
| `intel:xhr_sniff` | low | verify / admin | HAR capture → propose `api` channels |
| `intel:search_poll` | default | scheduler (search channels) | SERP query → curate |

`intel:fetch_source` retained for legacy rows; `SelectDue` switches to `ListDueChannels`.

### Valkey keys

| Key | TTL | Purpose |
|---|---|---|
| `jc:intel:probe_quota:{user_id}` | 1h | public probe rate limit |
| `jc:intel:probe_report:{source_id}` | until next probe | cached preview |
| `jc:intel:channel_lock:{channel_id}` | crawl TTL | per-channel crawl lock |
| `jc:intel:websub:lease:{channel_id}` | lease lifetime | WebSub subscription state |
| `jc:intel:inbound:idem:{hash}` | 24h | inbound idempotency |
| `jc:intel:render:circuit` | 5m | render-service circuit breaker |
| `jc:intel:effective_interval::{channel_id}` | 7d | volatility tuner → channel granularity |

### System settings

```
intel.category_method_map          jsonb   category → {entity_types, methods, pipeline}
intel.render.service_url           string  headless renderer endpoint (empty = disabled)
intel.render.timeout_ms            int
intel.search.provider              string  brave | google_cse | serpapi
intel.search.api_key               string  encrypted at rest (payment-secret pattern)
intel.email.inbound_domain         string
intel.email.allowed_senders        jsonb
intel.probe.max_requests           int     preview request budget (default 5)
intel.probe.rate_limit_per_hr      int     per-user (default 20)
intel.curate.auto_publish_trust_max int    trust ceiling for direct publish (default 2)
intel.social.enabled_providers     jsonb
```

**Catalog updates (Gate 2, same PR):** `api-routes-catalog.md`, `asynq-tasks.md`, `valkey-keys.md`, `system-settings.md`.

---

## 7. Rollout & testing

### Migration/backfill (zero-downtime)

1. Additive migration: `categories[]`, `probe_report`, `detected_methods`, `last_probed_at`, expanded `source_kind` check, `intel_source_channels`, `intel_inbound_events`, `intel_raw_documents.channel_id`.
2. Backfill `categories = ARRAY[category]`; one channel per existing source (`origin='admin'`).
3. Scheduler flips to `ListDueChannels` — behaviorally a no-op for existing sources.
4. In-flight `intel:fetch_source` tasks resolve source → default channel.

### Phases (one spec, ordered delivery)

- **Phase 1 — spine**: channels table, scheduler switch, curator package (`rss_news`, `jsonld`, `ical`, `tabular`), probe package + preview/verify, form changes, trust-gated publish, channels admin UI.
- **Phase 2 — pull adapters**: `render`, `sitemap`, `git`, `social`, `wayback`, `oneshot`, `xhr_sniff`.
- **Phase 3 — push adapters**: `email` inbound, `webhook`/WebSub, `search` monitoring.

### Testing

- **Unit**: detectors (fixture HTML/feed/ics/csv in `testdata/`), curators (deterministic record assertions), category→method resolution, trust gate, channel bookkeeping.
- **Integration**: propose→probe→approve→channels flow; `fetch→detect→curate→promote` against `httptest` fixtures; per-channel circuit breaker; hybrid LLM fallback only for incomplete records.
- **Handlers**: RBAC denial per new node; probe rate-limit + SSRF rejection (private IPs, `file://`); email signature verification; WebSub challenge.
- **Regression**: existing sources crawl identically post-backfill.

### Risks

- SSRF surface on public probe → `safetransport` + quota + auth required.
- Render-service dependency → config-gated; probe degrades to static.
- Social platform ToS → provider allowlist.
- LLM cost on `xhr_sniff`/`hybrid` → existing extraction cache + curate-first ordering.
