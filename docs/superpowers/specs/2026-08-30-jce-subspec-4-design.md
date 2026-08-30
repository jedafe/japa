# Japa Comparison Engine (JCE) — Sub-spec 4: Data Intelligence & AI Decision Agent

**Status:** Design  
**Refs:** §30 (Generic Comparison System), §103 (AI Agent Executor), §11 (Job Queue), ADR-029, ADR-058, ADR-076, ADR-088, `docs/superpowers/specs/2026-08-22-jce-subspecs-2-4-outline.md`, `docs/superpowers/specs/2026-08-25-jce-subspec-2-design.md`, `docs/japa-comparison-engine.md`  
**Source of truth:** `.jeos/sections/30-generic-comparison-system.md`  

**Scope:** Sub-spec 4 closes the JCE loop by making comparison data fresh, traceable, and AI-advisable. It builds on the Sub-spec 1/2/3 foundation (`apps/jce`, `comparison_content_metadata`, `comparison_values`, `comparison_score_factors`, `comparison_scores`, `comparison_recommendations`, the `item_ref` resolver registry, and `/api/v1/jce/*`).

---

## 1. Problem

The JCE now supports quick/detailed comparisons, polymorphic `item_ref`, profiles, Smart Compare, JapaScore, eligibility, and recommendations. But it still has three intelligence gaps:

1. **No freshness / provenance UX.** A user cannot tell whether a tuition figure is from an official university site updated last week or an AI estimate from six months ago.
2. **No structured data source registry.** The platform has many authoritative sources (visa route feeds, cost-of-living indices, scholarship databases, job listings) but no canonical way to register them per tenant, track their trust level, or refresh comparison values from them.
3. **No AI decision agent.** Users cannot ask the platform "Which is best for me?" and receive a ranked, explained answer with next actions. The agent catalog does not yet expose the JCE scoring/recommendation results as tools.

---

## 2. Goal

Deliver a unified data-intelligence and AI-advisor layer:

1. A `comparison_data_sources` registry per tenant with `source_kind` (`official`, `third_party`, `ai_estimate`), `trust_level`, `freshness_score`, and `last_verified_at`.
2. Per-comparison value provenance (source, source_ref, `retrieved_at`, `confidence`, `is_estimate`, `data_source_id`) on every `comparison_value`.
3. `updated_at` on `comparison_values` plus an API to compute and expose freshness/trust per cell.
4. Manual and scheduled refresh of stale comparison values, backed by an Asynq task and a `comparison_data_ingestion_jobs` audit table.
5. AI agent tools (`comparison.analyze`, `comparison.rank`, `comparison.explain`, `comparison.recommend_next_step`) and synchronous HTTP endpoints (`/jce/comparisons/{id}/analyze`, `/rank`, `/explain`, `/recommend`, `/advisor`) that call the scoring engine and synthesize natural-language answers.
6. Admin UI to manage data sources and a user-facing freshness badge + AI advisor drawer in `apps/jce`.
7. Feature flags, RBAC nodes, system settings, Valkey keys, and catalog updates for all of the above.

---

## 3. Scope and non-scope

### In scope

- Schema: `comparison_data_sources`, `comparison_data_ingestion_jobs`, `comparison_values.updated_at`, `comparison_values.data_source_id`.
- New Go packages/files in `internal/jce`: `datasource.go`, `freshness.go`, `ingestion.go`, `advisor.go`.
- New AI tool handlers in `internal/ai/tools_jce.go` (or extension of `tools_comparison.go`) and registration in `internal/ai/tools_register.go`.
- New HTTP routes in `internal/jce/handlers.go` and `internal/server/server.go`.
- New Asynq task `jce:refresh_values` (added to Section 11 catalog) and worker in `internal/worker` or `internal/jce`.
- `apps/jce` UI: freshness badges, refresh button, AI advisor panel, data-source status chips.
- `apps/admin` UI: data source list/edit, ingestion job list.
- Catalog updates: `api-routes-catalog.md`, `valkey-keys-catalog.md`, `system-settings-catalog.md`, `asynq-tasks.md`.
- Feature flags: `feature.jce.data_sources`, `feature.jce.refresh`, `feature.jce.ai_advisor`.
- RBAC nodes: `jce.data_source.manage`, `jce.data_source.view`, `jce.refresh.trigger`, `jce.ai_advisor.use`, `jce.ingestion_job.view`.

### Out of scope

- New LLM provider / gateway capability (uses existing `chat` / `content_generation` AI Gateway categories).
- Real-time collaborative editing of comparison values.
- Automatic source discovery from the open web (reuses existing `external.search` and `intel:fetch_source` infrastructure).
- Differential pricing or credit deduction for AI advisor calls (tracked as `ai_tool_usage` but not billing integration).

---

## 4. Domain model

| Term | Definition |
|---|---|
| **Data source** | A `comparison_data_sources` row that names an authoritative feed for a tenant, entity type, or individual entity. It carries a `trust_level` (1–5), `source_kind`, and `freshness_score` (0–100). |
| **Ingestion job** | A `comparison_data_ingestion_jobs` row that tracks one refresh run for a comparison, including status, refreshed count, and error. |
| **Value provenance** | The `metadata` JSONB on `comparison_values` plus the `data_source_id` FK. It records where a cell value came from and how fresh it is. |
| **Freshness score** | A 0–100 score derived from `updated_at`, `source_kind`, `trust_level`, and a configurable age-decay curve. |
| **AI advisor** | A service that calls the JCE scoring engine, fetches recommendations, builds a prompt, and sends it through the AI Gateway. |

---

## 5. Schema changes

The next contiguous migration number is `000572` (000570 and 000571 are already used). Use the six migrations below. Each migration must have a paired `.down.sql` that reverses the schema change without data loss.

### 5.1 Migration `000572_jce_data_sources`

```sql
-- comparison_data_sources: tenant-wide registry of authoritative feeds.
CREATE TABLE IF NOT EXISTS comparison_data_sources (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    entity_type     text,   -- e.g. 'university', 'visa_route', 'cost_of_living'
    entity_id       uuid,   -- NULL means the source applies to the whole entity type
    dimension_slug  text,   -- NULL means all dimensions for the entity
    source_name     text NOT NULL,
    source_url      text,
    source_kind     text NOT NULL CHECK (source_kind IN ('official', 'third_party', 'ai_estimate')),
    trust_level     int NOT NULL DEFAULT 3 CHECK (trust_level BETWEEN 1 AND 5),
    freshness_score int NOT NULL DEFAULT 0 CHECK (freshness_score BETWEEN 0 AND 100),
    last_verified_at timestamptz,
    config          jsonb NOT NULL DEFAULT '{}',
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_comparison_data_sources_tenant
    ON comparison_data_sources(tenant_id);
CREATE INDEX IF NOT EXISTS idx_comparison_data_sources_entity
    ON comparison_data_sources(tenant_id, entity_type, entity_id);

-- Source names are unique per tenant.
CREATE UNIQUE INDEX IF NOT EXISTS idx_comparison_data_sources_tenant_name
    ON comparison_data_sources(tenant_id, source_name);
```

### 5.2 Migration `000573_jce_value_freshness`

```sql
-- Track when a comparison value was last updated and, optionally, which data source it came from.
ALTER TABLE comparison_values
    ADD COLUMN IF NOT EXISTS updated_at timestamptz,
    ADD COLUMN IF NOT EXISTS data_source_id uuid REFERENCES comparison_data_sources(id) ON DELETE SET NULL;

UPDATE comparison_values SET updated_at = created_at WHERE updated_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_comparison_values_data_source
    ON comparison_values(data_source_id);
```

Backfill `updated_at = created_at` for existing rows. Update `UpsertComparisonValue` and `UpsertComparisonValueWithMetadata` in `internal/db/queries/comparison.sql` to set `updated_at = now()`.

### 5.3 Migration `000574_jce_ingestion_jobs`

```sql
-- comparison_data_ingestion_jobs: audit/status table for refresh runs.
CREATE TABLE IF NOT EXISTS comparison_data_ingestion_jobs (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    content_id      uuid NOT NULL REFERENCES contents(id) ON DELETE CASCADE,
    status          text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','running','completed','failed','cancelled')),
    source_kind     text, -- optional filter: only refresh values of this kind
    refreshed_count int NOT NULL DEFAULT 0,
    error           text,
    started_at      timestamptz,
    completed_at    timestamptz,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_comp_ingestion_jobs_tenant_content
    ON comparison_data_ingestion_jobs(tenant_id, content_id, created_at DESC);
```

### 5.4 Migration `000575_jce_subspec4_rbac`

```sql
-- Feature flags (gated off by default).
INSERT INTO feature_flags (key, tenant_id, enabled, config)
VALUES
    ('feature.jce.data_sources', '00000000-0000-0000-0000-000000000001', false, '{}'),
    ('feature.jce.refresh',      '00000000-0000-0000-0000-000000000001', false, '{}'),
    ('feature.jce.ai_advisor',   '00000000-0000-0000-0000-000000000001', false, '{}')
ON CONFLICT (key, tenant_id) DO UPDATE SET
    enabled = EXCLUDED.enabled,
    config = EXCLUDED.config;

-- RBAC nodes under the JCE namespace.
INSERT INTO rbac_nodes (key, tenant_id, name, description)
VALUES
    ('jce.data_source.manage',   '00000000-0000-0000-0000-000000000001', 'Manage comparison data sources', 'Create/update/delete data source definitions'),
    ('jce.data_source.view',     '00000000-0000-0000-0000-000000000001', 'View comparison data sources',   'List and view data source definitions'),
    ('jce.refresh.trigger',      '00000000-0000-0000-0000-000000000001', 'Trigger comparison refresh',     'Enqueue a refresh of comparison values'),
    ('jce.ai_advisor.use',       '00000000-0000-0000-0000-000000000001', 'Use JCE AI advisor',             'Ask the AI advisor questions about a comparison'),
    ('jce.ingestion_job.view',   '00000000-0000-0000-0000-000000000001', 'View ingestion jobs',            'View refresh job history')
ON CONFLICT (key, tenant_id) DO UPDATE SET
    name = EXCLUDED.name,
    description = EXCLUDED.description;

-- Grant super_admin and admin all new nodes.
INSERT INTO permission_grants (role_id, node_id)
SELECT r.id, n.id
FROM user_roles r, rbac_nodes n
WHERE r.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND r.name IN ('super_admin','admin')
  AND n.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND n.key IN ('jce.data_source.manage','jce.data_source.view','jce.refresh.trigger','jce.ai_advisor.use','jce.ingestion_job.view')
ON CONFLICT DO NOTHING;
```

### 5.5 Migration `000576_jce_ai_advisor_tools`

Seed the Phase 7/9 comparison agent tools. The `function_name` and `handler` strings must match the Go registration in `internal/ai/tools_register.go`.

```sql
INSERT INTO ai_tools (tenant_id, name, description, category, config, is_enabled, required_permission) VALUES
('00000000-0000-0000-0000-000000000001', 'comparison.analyze',
 'Analyze a comparison against the user profile and return a ranked, explained answer with next actions.',
 'agent_comparison',
 '{
    "function_name": "comparison.analyze",
    "description": "Analyze a comparison and profile, return scores/explanations and next actions.",
    "parameters": {
        "type": "object",
        "properties": {
            "content_id": { "type": "string", "description": "Comparison content UUID" },
            "profile_id": { "type": "string", "description": "Optional comparison profile UUID" },
            "question":   { "type": "string", "description": "User question, e.g. 'Which is best for me?'" }
        },
        "required": ["content_id"]
    },
    "handler": "JCEAnalyze",
    "reads_user_data": true,
    "writes_user_data": false,
    "requires_hitl": false,
    "is_blocked": false
}',
 true, 'jce.ai_advisor.use'),

('00000000-0000-0000-0000-000000000001', 'comparison.rank',
 'Rank comparison items with reasoning and confidence.',
 'agent_comparison',
 '{
    "function_name": "comparison.rank",
    "description": "Rank items in a comparison with reasoning.",
    "parameters": {
        "type": "object",
        "properties": {
            "content_id": { "type": "string" },
            "profile_id": { "type": "string" }
        },
        "required": ["content_id"]
    },
    "handler": "JCERank",
    "reads_user_data": true,
    "writes_user_data": false,
    "requires_hitl": false,
    "is_blocked": false
}',
 true, 'jce.ai_advisor.use'),

('00000000-0000-0000-0000-000000000001', 'comparison.explain',
 'Explain a single JapaScore factor or recommendation for an item.',
 'agent_comparison',
 '{
    "function_name": "comparison.explain",
    "description": "Explain a single score or recommendation.",
    "parameters": {
        "type": "object",
        "properties": {
            "content_id": { "type": "string" },
            "item_id":    { "type": "string" },
            "factor_key": { "type": "string" }
        },
        "required": ["content_id", "item_id"]
    },
    "handler": "JCEExplain",
    "reads_user_data": true,
    "writes_user_data": false,
    "requires_hitl": false,
    "is_blocked": false
}',
 true, 'jce.ai_advisor.use'),

('00000000-0000-0000-0000-000000000001', 'comparison.recommend_next_step',
 'Get the top next actions and recommendations for a comparison and profile.',
 'agent_comparison',
 '{
    "function_name": "comparison.recommend_next_step",
    "description": "Get the top next actions and recommendations for a comparison and profile.",
    "parameters": {
        "type": "object",
        "properties": {
            "content_id": { "type": "string" },
            "profile_id": { "type": "string" }
        },
        "required": ["content_id"]
    },
    "handler": "JCERecommend",
    "reads_user_data": true,
    "writes_user_data": false,
    "requires_hitl": false,
    "is_blocked": false
}',
 true, 'jce.ai_advisor.use'),

('00000000-0000-0000-0000-000000000001', 'comparison.refresh',
 'Trigger a refresh of stale comparison values from data sources.',
 'agent_comparison',
 '{
    "function_name": "comparison.refresh",
    "description": "Refresh stale comparison values from registered data sources.",
    "parameters": {
        "type": "object",
        "properties": {
            "content_id": { "type": "string" },
            "source_kind": { "type": "string", "description": "Optional filter: official|third_party|ai_estimate" }
        },
        "required": ["content_id"]
    },
    "handler": "JCERefresh",
    "reads_user_data": true,
    "writes_user_data": true,
    "requires_hitl": false,
    "is_blocked": false
}',
 true, 'jce.refresh.trigger')
ON CONFLICT (tenant_id, name) DO UPDATE SET
    description = EXCLUDED.description,
    category = EXCLUDED.category,
    config = EXCLUDED.config,
    is_enabled = EXCLUDED.is_enabled,
    required_permission = EXCLUDED.required_permission;
```

### 5.6 Migration `000577_jce_subspec4_settings`

```sql
-- Default settings for freshness, advisor, and refresh scheduling.
INSERT INTO system_settings (tenant_id, key, value) VALUES
    ('00000000-0000-0000-0000-000000000001', 'jce.freshness_stale_hours',          to_jsonb(168)),
    ('00000000-0000-0000-0000-000000000001', 'jce.freshness_decay_per_hour',       to_jsonb(2)),
    ('00000000-0000-0000-0000-000000000001', 'jce.freshness_official_weight',      to_jsonb(100)),
    ('00000000-0000-0000-0000-000000000001', 'jce.freshness_third_party_weight',   to_jsonb(75)),
    ('00000000-0000-0000-0000-000000000001', 'jce.freshness_ai_estimate_weight',   to_jsonb(50)),
    ('00000000-0000-0000-0000-000000000001', 'jce.freshness_cache_ttl_minutes',    to_jsonb(10)),
    ('00000000-0000-0000-0000-000000000001', 'jce.advisor_model',                  '""'),
    ('00000000-0000-0000-0000-000000000001', 'jce.advisor_max_tokens',             to_jsonb(1024)),
    ('00000000-0000-0000-0000-000000000001', 'jce.advisor_cache_ttl_minutes',      to_jsonb(15))
ON CONFLICT (tenant_id, key) DO NOTHING;
```

---

## 6. Architecture

Sub-spec 4 adds four new modules inside `internal/jce` and extends the AI tool catalog.

```
internal/jce
├── datasource.go      -- comparison_data_sources CRUD + service
├── freshness.go       -- freshness scoring + summary
├── ingestion.go       -- refresh orchestration, Asynq payload, job tracking
├── advisor.go         -- AI advisor: scoring + LLM prompt + response
├── handlers.go        -- extended with /freshness, /refresh, /analyze, /rank, /explain, /recommend, /advisor
└── models.go          -- extended with DataSource, IngestionJob, AdvisorInput/Output

internal/ai
├── tools_jce.go       -- comparison.analyze/rank/explain/recommend/refresh handlers
└── tools_register.go  -- registration of the new tools

apps/jce
├── compare/[slug]/+page.svelte -- freshness chips + AI advisor drawer
├── comparisons/[slug]/+page.svelte -- same
└── lib/api/jce.ts     -- new client calls

apps/admin
└── comparison-data-sources/+page.svelte -- list/edit
```

Server wiring: extend `internal/server/server.go` to inject `jce.Service` as `ai.ToolDeps.JCE`, register the new JCE HTTP routes and the `jce:refresh_values` Asynq handler, and add the new feature gates.

---

## 7. Data source registry (`internal/jce/datasource.go`)

### 7.1 `DataSource` domain model

```go
type DataSource struct {
    ID             string         `json:"id"`
    TenantID       string         `json:"tenant_id"`
    EntityType     *string        `json:"entity_type,omitempty"`
    EntityID       *string        `json:"entity_id,omitempty"`
    DimensionSlug  *string        `json:"dimension_slug,omitempty"`
    SourceName     string         `json:"source_name"`
    SourceURL      *string        `json:"source_url,omitempty"`
    SourceKind     string         `json:"source_kind"` // official|third_party|ai_estimate
    TrustLevel     int            `json:"trust_level"`
    FreshnessScore int            `json:"freshness_score"`
    LastVerifiedAt *string        `json:"last_verified_at,omitempty"`
    Config         map[string]any `json:"config"`
    CreatedAt      string         `json:"created_at"`
    UpdatedAt      string         `json:"updated_at"`
}
```

### 7.2 Service interface

```go
type DataSourceService interface {
    ListDataSources(ctx context.Context, tenantID pgtype.UUID, entityType string, entityID *pgtype.UUID) ([]DataSource, error)
    CreateDataSource(ctx context.Context, tenantID pgtype.UUID, in DataSourceInput) (DataSource, error)
    GetDataSource(ctx context.Context, id, tenantID pgtype.UUID) (DataSource, error)
    UpdateDataSource(ctx context.Context, id, tenantID pgtype.UUID, in DataSourceInput) (DataSource, error)
    DeleteDataSource(ctx context.Context, id, tenantID pgtype.UUID) error
    MatchDataSource(ctx context.Context, tenantID pgtype.UUID, entityType string, entityID *pgtype.UUID, dimensionSlug, sourceName string) (*DataSource, error)
}

### 7.4 `DataSourceInput`

```go
type DataSourceInput struct {
    EntityType    *string        `json:"entity_type,omitempty"`
    EntityID      *string        `json:"entity_id,omitempty"`
    DimensionSlug *string        `json:"dimension_slug,omitempty"`
    SourceName    string         `json:"source_name"`
    SourceURL     *string        `json:"source_url,omitempty"`
    SourceKind    string         `json:"source_kind"`
    TrustLevel    int            `json:"trust_level"`
    Config        map[string]any `json:"config"`
}
```

### 7.3 Matching rules

- If `entity_id` is provided, look for the most specific source first `(entity_id, dimension_slug)`, then `(entity_id, NULL)`, then `(NULL, dimension_slug)`, then `(NULL, NULL)`.
- If only `entity_type` is provided, match within that entity type and fall back to global (`entity_type IS NULL`).
- A value's `metadata.source` is compared against `source_name` when no `data_source_id` is stored.

### 7.5 `IngestionJob`

```go
type IngestionJob struct {
    ID             string  `json:"id"`
    TenantID       string  `json:"tenant_id"`
    ContentID      string  `json:"content_id"`
    Status         string  `json:"status"`
    SourceKind     *string `json:"source_kind,omitempty"`
    RefreshedCount int     `json:"refreshed_count"`
    Error          *string `json:"error,omitempty"`
    StartedAt      *string `json:"started_at,omitempty"`
    CompletedAt    *string `json:"completed_at,omitempty"`
    CreatedAt      string  `json:"created_at"`
    UpdatedAt      string  `json:"updated_at"`
}
```

---

## 8. Freshness scoring (`internal/jce/freshness.go`)

### 8.1 Freshness inputs

For each `comparison_value`:

- `updated_at` (timestamp)
- `data_source_id` → `comparison_data_sources.trust_level`, `source_kind`, `last_verified_at`
- `metadata.retrieved_at` (RFC3339 fallback)
- `metadata.confidence` (`high`/`medium`/`low`)
- `metadata.is_estimate` (bool)

### 8.2 Settings

Add to `system_settings`:

- `jce.freshness_stale_hours` (int, default `168`) — value older than this is considered stale.
- `jce.freshness_decay_per_hour` (int, default `2`) — points lost per hour after `freshness_stale_hours`.
- `jce.freshness_official_weight` (int, default `100`)
- `jce.freshness_third_party_weight` (int, default `75`)
- `jce.freshness_ai_estimate_weight` (int, default `50`)

### 8.3 Formula

```
base = source_kind_weight / 100   (official=1.0, third_party=0.75, ai_estimate=0.5)
trust = trust_level / 5           (0.2 .. 1.0)
age_hours = max(0, now - value.updated_at).hours - freshness_stale_hours
decay = max(0, age_hours) * freshness_decay_per_hour
confidence = 1.0 if high, 0.75 if medium, 0.5 if low, 0.75 if missing
estimate_penalty = 0.85 if is_estimate else 1.0

freshness_score = clamp(0, 100, (100 - decay) * base * trust * confidence * estimate_penalty)
```

The result is rounded to the nearest integer and stored on `comparison_data_sources.freshness_score` for global sources and returned in API summaries for cells.

### 8.4 Summary output

`GET /jce/comparisons/{id}/freshness` returns:

```json
{
  "content_id": "...",
  "overall_freshness_score": 72,
  "stale_count": 3,
  "total_count": 12,
  "by_item": [
    {
      "item_id": "...",
      "item_title": "University of Toronto",
      "freshness_score": 85,
      "cells": [
        {
          "dimension_id": "...",
          "label": "Tuition",
          "freshness_score": 92,
          "source_name": "Official university site",
          "source_kind": "official",
          "retrieved_at": "2026-08-25T12:00:00Z",
          "is_stale": false
        }
      ]
    }
  ]
}
```

---

## 9. Ingestion jobs and refresh flow

### 9.1 Asynq task

Add to `.jeos/sections/11-job-queue-asynq.md` and register in worker:

```
jce:refresh_values     -- default tier. Payload: { tenant_id, content_id, source_kind, ingestion_job_id, requested_by_user_id }
```

### 9.2 Manual refresh API

`POST /jce/comparisons/{id}/refresh`

Request:

```json
{
  "source_kind": "official" // optional filter
}
```

Response:

```json
{
  "ingestion_job_id": "...",
  "status": "pending"
}
```

### 9.3 Worker behavior

The handler (`internal/jce/ingestion.go` worker, wired in `internal/worker`) does:

1. Load the comparison, items, and dimensions.
2. Create or update `comparison_data_ingestion_jobs` to `status='running'`.
3. For each `comparison_value` (or missing cell) determine the target `data_source_id`:
   - If already set and `source_kind` matches, use it.
   - If not set, call `MatchDataSource` using the item's `item_ref.type`, `item_ref.id`, and dimension slug.
4. If the value is stale or missing, refresh it:
   - If the source has a known internal resolver or adapter (cost of living, visa route, scholarship), call the existing service and write the new value.
   - If no internal source exists, use the existing AI Comparison Engine (`internal/ai/comparison.go`) to fill the value via `external.search` / RAG, writing provenance metadata.
5. Update `comparison_values` (`value`, `value_rating`, `value_boolean`, `metadata`, `data_source_id`, `updated_at = now()`).
6. Update `comparison_data_sources.last_verified_at` and recompute `freshness_score`.
7. After run, set `comparison_data_ingestion_jobs.status` to `completed` or `failed` and set `refreshed_count`.

### 9.4 Scheduled refresh

A periodic Asynq cron (or `maintenance:*` sweep) enqueues `jce:refresh_values` for every comparison whose latest completed `comparison_data_ingestion_jobs` row is older than `jce.freshness_stale_hours` (or whose smart mode is not `none` and has no completed job). The initial implementation can use a daily low-tier sweep.

### 9.5 Feature and permission gates

- Data source admin routes (`/jce/data-sources/*`) — `feature.comparison`, `feature.jce`, `feature.jce.data_sources`, and `jce.data_source.view` / `jce.data_source.manage`.
- `GET /jce/comparisons/{id}/freshness` — `feature.comparison`, `feature.jce`, `feature.jce.data_sources`, plus the comparison content view permission (`comparison.view` or `comparison.edit_any`).
- `POST /jce/comparisons/{id}/refresh` and `GET /jce/comparisons/{id}/jobs` — `feature.comparison`, `feature.jce`, `feature.jce.refresh`, plus `jce.refresh.trigger` (for refresh) or `jce.ingestion_job.view` (for jobs).
- AI advisor routes (`/jce/comparisons/{id}/analyze`, `/rank`, `/explain`, `/recommend`, `/advisor`) — `feature.comparison`, `feature.jce`, `feature.jce.ai_advisor`, and `jce.ai_advisor.use`.

---

## 10. AI decision agent

### 10.1 HTTP endpoints

Add to `internal/jce/handlers.go` and `internal/server/server.go` under the `feature.comparison` + `feature.jce` + `feature.jce.ai_advisor` gates:

```
POST /jce/comparisons/{id}/analyze   -- natural language analysis
POST /jce/comparisons/{id}/rank      -- ranked list with reasoning
POST /jce/comparisons/{id}/explain   -- explain one factor/recommendation
POST /jce/comparisons/{id}/recommend -- top next actions and recommendations
POST /jce/comparisons/{id}/advisor   -- streaming SSE chat endpoint
```

All require `jce.ai_advisor.use` and accept:

```json
{
  "profile_id": "...", // optional
  "question": "..."    // optional for /rank and /explain; required for /advisor
}
```

### 10.2 Advisor service

Add `internal/jce/advisor.go`:

```go
type Advisor interface {
    Analyze(ctx context.Context, contentID, tenantID, userID, profileID pgtype.UUID, question string) (AdvisorResult, error)
    Rank(ctx context.Context, contentID, tenantID, userID, profileID pgtype.UUID) (AdvisorResult, error)
    Explain(ctx context.Context, contentID, itemID, tenantID, userID, profileID pgtype.UUID, factorKey, question string) (AdvisorResult, error)
    Recommend(ctx context.Context, contentID, tenantID, userID, profileID pgtype.UUID) (AdvisorResult, error)
    StreamAdvisor(ctx context.Context, contentID, tenantID, userID, profileID pgtype.UUID, question string, w http.ResponseWriter) error
}
```

`AdvisorResult`:

```go
type AdvisorResult struct {
    ContentID      string            `json:"content_id"`
    ProfileID      *string           `json:"profile_id,omitempty"`
    Summary        string            `json:"summary"`
    Ranking        []AdvisorRankItem `json:"ranking,omitempty"`
    NextActions    []NextAction      `json:"next_actions,omitempty"`
    Citations      []string          `json:"citations,omitempty"`
    ProviderName   string            `json:"provider_name"`
    ProviderRef    string            `json:"provider_ref"`
    FreshnessScore int               `json:"freshness_score"`
}

type AdvisorRankItem struct {
    ItemID       string  `json:"item_id"`
    ItemTitle    string  `json:"item_title"`
    Rank         int     `json:"rank"`
    JapaScore    float64 `json:"japa_score"`
    Reasoning    string  `json:"reasoning"`
    Eligible     bool    `json:"eligible"`
}

type NextAction struct {
    ItemID     string `json:"item_id,omitempty"`
    Text       string `json:"text"`
    ActionType string `json:"action_type,omitempty"`
    ActionURL  string `json:"action_url,omitempty"`
    Priority   int    `json:"priority"`
}

type AdvisorInput struct {
    ContentID  pgtype.UUID  `json:"content_id"`
    TenantID   pgtype.UUID  `json:"tenant_id"`
    UserID     pgtype.UUID  `json:"user_id"`
    ProfileID  *pgtype.UUID `json:"profile_id,omitempty"`
    Question   string       `json:"question"`
    ItemID     *pgtype.UUID `json:"item_id,omitempty"`
    FactorKey  string       `json:"factor_key"`
    SourceKind string       `json:"source_kind"`
}
```

### 10.3 Prompt construction

1. Fetch `ScoreResult` from `jce.Service.ScoreComparison` (force=false, use cache).
2. Fetch `[]ComparisonRecommendation` from `jce.Service.ListRecommendations`.
3. Fetch `JCEComparisonMatrix` from `comparison.Service.GetJCEComparisonMatrix`.
4. Build a structured prompt including:
   - User question.
   - Comparison title, items, dimensions, values with freshness (from `freshness.FreshnessSummary`).
   - Per-item JapaScore, factor scores, eligibility status, explanations.
   - Top recommendations.
5. Build a single `prompt` string and an optional `context` (system) string, then call `ai.Dispatcher.Dispatch(ctx, "chat", Request{Payload: map[string]any{"model": model, "prompt": prompt, "context": context, "max_tokens": maxTokens}})`.
6. Parse the response text and return as JSON. For `comparison.explain`, the prompt focuses on one item/factor.

### 10.4 Streaming `/advisor`

Uses `http.Flusher` per §103:

```go
w.Header().Set("Content-Type", "text/event-stream")
// Send events: { "type": "tool_call", ... } and { "type": "message", "content": "..." }
```

For the initial implementation, the streaming endpoint dispatches a single `chat` completion and emits one final `message` event containing the generated answer. It does **not** run a multi-turn agent loop in v1 and does not stream individual tokens (the provider `Send` path is non-streaming).

---

## 11. AI agent tools

Add to `internal/ai/tools_jce.go` and register in `internal/ai/tools_register.go` under `RegisterPhase7ComparisonTools`.

The handlers use a new `JCE` field in `ToolDeps`:

```go
type ToolDeps struct {
    // ... existing fields ...
    JCE JCEService // implemented by internal/jce.Service or a thin adapter
}

type JCEService interface {
    AnalyzeComparison(ctx context.Context, tenantID, userID, contentID, profileID pgtype.UUID, question string) (map[string]any, error)
    RankComparison(ctx context.Context, tenantID, userID, contentID, profileID pgtype.UUID) (map[string]any, error)
    ExplainComparison(ctx context.Context, tenantID, userID, contentID, itemID, profileID pgtype.UUID, factorKey string) (map[string]any, error)
    RecommendComparison(ctx context.Context, tenantID, userID, contentID, profileID pgtype.UUID) (map[string]any, error)
    RefreshComparison(ctx context.Context, tenantID, userID, contentID pgtype.UUID, sourceKind string) (map[string]any, error)
}
```

### 11.1 Tool results

- `comparison.analyze` returns the advisor `summary`, `ranking`, and `next_actions`.
- `comparison.rank` returns `ranking` only.
- `comparison.explain` returns `explanation` and `citations`.
- `comparison.recommend_next_step` returns `next_actions` only.
- `comparison.refresh` returns `{ "ingestion_job_id": "...", "status": "pending" }`.

### 11.2 Privacy targets

Register `PrivacyTarget` extractors for `comparison.analyze`, `comparison.rank`, `comparison.explain`, `comparison.recommend_next_step`, and `comparison.refresh`. Each extractor returns `entity_type="content"` and `entity_id` from the `content_id` argument so the executor blocks reads/writes against `ai_exempt` comparison contents.

---

## 12. Frontend

### 12.1 `apps/jce`

- `ScoreExplanation.svelte` and `ComparisonMatrix` components show a freshness chip per cell (`Fresh`, `Stale`, `AI estimate`).
- A `Refresh` button on `/compare/[slug]` and `/comparisons/[slug]` calls `POST /jce/comparisons/{id}/refresh`.
- An `AIAdvisorDrawer.svelte` on the same pages accepts a question and calls either:
  - `POST /jce/comparisons/{id}/analyze` for a one-shot answer, or
  - `POST /jce/comparisons/{id}/advisor` for a streaming answer.
- i18n keys: `jce.freshness.*`, `jce.advisor.*`, `jce.refresh.*`.

### 12.2 `apps/admin`

- New route `/comparison-data-sources`:
  - List: source name, kind, trust level, freshness score, last verified.
  - Edit: form for `source_name`, `source_url`, `source_kind`, `trust_level`, `entity_type`, `entity_id`, `dimension_slug`, `config`.
- Add a `Data sources` tab to the comparison type editor.
- Add an `Ingestion jobs` view under Content → Comparisons.

---

## 13. Catalog updates

### 13.1 API routes catalog

Append to `.jeos/conventions/api-routes-catalog.md` under JCE Sub-spec 4:

```
GET    /jce/comparisons/{id}/freshness      freshness summary for a comparison
POST   /jce/comparisons/{id}/refresh        enqueue a refresh job
GET    /jce/comparisons/{id}/jobs           list refresh jobs for a comparison

GET    /jce/data-sources                    list data sources
POST   /jce/data-sources                    create a data source
GET    /jce/data-sources/{id}               view a data source
PATCH  /jce/data-sources/{id}               update a data source
DELETE /jce/data-sources/{id}               delete a data source

POST   /jce/comparisons/{id}/analyze        AI analysis of a comparison
POST   /jce/comparisons/{id}/rank           AI ranking of items
POST   /jce/comparisons/{id}/explain        AI explanation of a score
POST   /jce/comparisons/{id}/recommend      AI next actions for a comparison
POST   /jce/comparisons/{id}/advisor        streaming AI advisor (SSE)
```

### 13.2 Valkey keys catalog

Append to `.jeos/conventions/valkey-keys-catalog.md`:

```
jc:jce:freshness:{tenant_id}:{content_id}           JSON FreshnessSummary  jce.freshness_cache_ttl_minutes (10)
jc:jce:advisor:{tenant_id}:{content_id}:{profile_id}:{question_hash}  JSON AdvisorResult  jce.advisor_cache_ttl_minutes (15)
jc:jce:refresh_lock:{content_id}                    SET NX refresh lock  5 min
```

### 13.3 System settings catalog

Append to `.jeos/conventions/system-settings-catalog.md`:

```
jce.freshness_stale_hours          int     168
jce.freshness_decay_per_hour       int     2
jce.freshness_official_weight      int     100
jce.freshness_third_party_weight   int     75
jce.freshness_ai_estimate_weight   int     50
jce.freshness_cache_ttl_minutes    int     10
jce.advisor_model                  string  "" (empty → dispatcher default)
jce.advisor_max_tokens             int     1024
jce.advisor_cache_ttl_minutes      int     15
```

### 13.4 Asynq tasks catalog

Append to `.jeos/sections/11-job-queue-asynq.md` under `default` tier:

```
jce:refresh_values     -- refresh comparison values from registered data sources
```

---

## 14. Verification

- `make sqlc-gen` passes and `internal/db/gen` is current.
- `go test ./internal/jce/... ./internal/ai/... ./internal/worker/...` passes.
- `make vet` passes.
- `bun run check` and `bun run build` in `apps/jce` and `apps/admin` pass.
- Manual smoke test:
  1. Enable `feature.jce.data_sources`, `feature.jce.refresh`, `feature.jce.ai_advisor`.
  2. Register a data source in admin.
  3. Create a comparison and fill a value.
  4. Call `GET /jce/comparisons/{id}/freshness` and confirm non-zero score.
  5. Call `POST /jce/comparisons/{id}/refresh` and confirm `jce:refresh_values` task processes.
  6. Call `POST /jce/comparisons/{id}/analyze` and confirm a natural language answer with ranking and next actions.

---

## 15. JEOS amendments

Sub-spec 4 requires the following updates to JEOS source-of-truth files. These amendments are part of the design, not after-the-fact documentation.

### 15.1 `.jeos/sections/30-generic-comparison-system.md`

Update the `comparison_values` schema block to:

```sql
comparison_values (
  dimension_id FK, item_id FK,
  value TEXT, value_rating DECIMAL NULL, value_boolean BOOLEAN NULL,
  metadata     jsonb NOT NULL DEFAULT '{}',  -- provenance, source, source_ref, confidence, retrieved_at, is_estimate, data_source_id
  created_at, updated_at,
  PRIMARY KEY (item_id, dimension_id)
)
```

Append a new `comparison_data_sources` registry block and `comparison_data_ingestion_jobs` audit block under the existing schema sections. Add the Sub-spec 4 route and tool references to the JCE API route list.

### 15.2 `.jeos/sections/11-job-queue-asynq.md`

Append under the `default` tier:

```
jce:refresh_values     -- refresh comparison values from registered data sources
```

### 15.3 `.jeos/conventions/api-routes-catalog.md`

Append the Sub-spec 4 routes listed in §13.1.

### 15.4 `.jeos/conventions/valkey-keys-catalog.md`

Append the Sub-spec 4 keys listed in §13.2.

### 15.5 `.jeos/conventions/system-settings-catalog.md`

Append the Sub-spec 4 settings listed in §13.3.

---

## 16. Notes

- All JEOS rules apply: full RBAC CRUD for `comparison_data_sources`, catalog updates, no hand-editing `internal/db/gen`, `jc:` Valkey prefix.
- `metadata.source` on `comparison_values` remains backward-compatible with existing `custom_field|rag|external|llm_knowledge` provenance. Sub-spec 4 adds `data_source_id` and `is_estimate` to the same `metadata` object.
- The AI advisor reuses the Sub-spec 3 scoring engine; it does not invent its own ranking. The LLM only synthesizes natural language on top of computed scores and recommendations.
