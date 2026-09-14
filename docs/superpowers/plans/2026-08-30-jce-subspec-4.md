# Japa Comparison Engine (JCE) — Sub-spec 4: Data Intelligence & AI Decision Agent Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` (recommended) or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Spec:** `docs/superpowers/specs/2026-08-30-jce-subspec-4-design.md`

**Goal:** Implement the unified data-intelligence and AI-advisor layer for the Japa Comparison Engine: tenant-scoped `comparison_data_sources`, freshness scoring and UX, value refresh through `jce:refresh_values`, `comparison_data_ingestion_jobs` audit, and the AI decision agent (`analyze`, `rank`, `explain`, `recommend_next_step`, `refresh`) exposed as HTTP and AI tool endpoints.

**Architecture:** Add `comparison_data_sources`, `comparison_data_ingestion_jobs`, and `comparison_values.updated_at/data_source_id` to the schema. Build focused `internal/jce` modules (`datasource.go`, `freshness.go`, `ingestion.go`, `advisor.go`) and AI tool handlers in `internal/ai`. Expose new `/api/v1/jce/*` routes and wire an Asynq worker. Update `apps/main` and `apps/admin` UIs. The AI advisor synthesizes natural language and next actions on top of the existing scoring engine and recommendations; it does not compute its own rankings.

**Tech Stack:** Go 1.24+ (chi, pgx/v5, sqlc), SvelteKit 2 + Svelte 5 Runes + Bits UI + Tailwind CSS v4, Bun, Valkey, PostgreSQL 16+.

## Global Constraints

- Migrations are numbered `000573`–`000578` (`000570`, `000571`, and `000572_seed_spaces_settings` already exist). Every migration has a paired `.down.sql` and runs inside `BEGIN; ... COMMIT;`.
- Never hand-edit `internal/db/gen`. Edit `internal/db/queries/*.sql` and run `make sqlc-gen`.
- All new Valkey keys use the `jc:` prefix and are added to `.jeos/conventions/valkey-keys-catalog.md` before code uses them.
- The new Asynq task `jce:refresh_values` is added to `.jeos/sections/11-job-queue-asynq.md` (default tier) before code uses it.
- All configurable values come from `system_settings` or `JC_*` env vars; no magic numbers.
- All new routes, permissions, feature flags, and settings are catalogued in JEOS before implementation.
- All API errors follow `conventions/api-response-format.md`: `code` is `snake_case`, `message` is human-readable, `status` is the HTTP status.
- Use `bun` for all frontend tooling; use `bunx` when invoking a package binary (e.g. `bunx svelte-check`). Never `npm`, `npx`, `pnpm`, or `yarn`.
- Preserve tenant isolation, RBAC, privacy exemptions, GDPR, and soft deletes.
- No AI-generated attribution in commits, PRs, or files.
- Every frontend page and component must consume the backend API; no hardcoded option lists, mock data, or fallback values in production.

---

## File Map

| File | Responsibility |
|---|---|
| `migrations/000573_jce_data_sources.up.sql` / `.down.sql` | Create/drop `comparison_data_sources` table and indexes. |
| `migrations/000574_jce_value_freshness.up.sql` / `.down.sql` | Add `updated_at` and `data_source_id` to `comparison_values`, backfill `updated_at`. |
| `migrations/000575_jce_ingestion_jobs.up.sql` / `.down.sql` | Create/drop `comparison_data_ingestion_jobs` table and index. |
| `migrations/000576_jce_subspec4_rbac.up.sql` / `.down.sql` | Seed `feature.jce.data_sources`, `feature.jce.refresh`, `feature.jce.ai_advisor`, RBAC nodes, and default grants. |
| `migrations/000577_jce_ai_advisor_tools.up.sql` / `.down.sql` | Seed `ai_tools` rows for `comparison.analyze`, `rank`, `explain`, `recommend_next_step`, `refresh`. |
| `migrations/000578_jce_subspec4_settings.up.sql` / `.down.sql` | Seed `jce.freshness_*` and `jce.advisor_*` system settings. |
| `internal/db/queries/jce.sql` | Add data source and ingestion job CRUD queries. |
| `internal/db/queries/comparison.sql` | Update `UpsertComparisonValue` and `UpsertComparisonValueWithMetadata` to set `updated_at` and `data_source_id`. |
| `internal/jce/models.go` | Add domain models: `DataSource`, `DataSourceInput`, `IngestionJob`, `FreshnessSummary`, `AdvisorInput`, `AdvisorResult`, `AdvisorRankItem`, `NextAction`. |
| `internal/jce/datasource.go` | `dataSourceStore` and `Service` CRUD methods for `comparison_data_sources`. |
| `internal/jce/freshness.go` | Freshness score calculation, `FreshnessSummary`, and `Service.FreshnessForComparison` method. |
| `internal/jce/ingestion.go` | `ingestionJobStore`, `Service.RefreshComparison`, `Service.ProcessRefreshValues`, and `internal/worker` adapter. |
| `internal/jce/advisor.go` | `Service.AnalyzeComparison`, `RankComparison`, `ExplainComparison`, `RecommendComparison`, and `StreamAdvisor` methods. |
| `internal/jce/handlers.go` | New HTTP handlers: data sources, freshness, refresh, jobs, and AI advisor endpoints. |
| `internal/jce/service.go` | Add `dataSources`, `ingestion` fields to `Service` struct; add `SetAIDispatcher`; update `NewService`. |
| `internal/ai/tools_jce.go` | Tool handler functions and privacy targets for the five JCE agent tools. |
| `internal/ai/tools_register.go` | Register the new tools in `RegisterPhase7ComparisonTools`. |
| `internal/ai/tools_deps.go` | Add `JCE JCEService` field to `ToolDeps` and `JCEService` interface. |
| `internal/server/server.go` | Wire `agentDeps.JCE`, add new JCE routes with feature/permission gates, set AI dispatcher on JCE service. |
| `internal/worker/tasks.go` | Add `TypeJCERefreshValues = "jce:refresh_values"`. |
| `internal/worker/handlers.go` | Add `HandleJceRefreshValues` and register it in `RegisterHandlers`. |
| `.jeos/sections/30-generic-comparison-system.md` | Amend `comparison_values` schema and add new tables/routes/tools. |
| `.jeos/sections/11-job-queue-asynq.md` | Add `jce:refresh_values` to the default tier. |
| `.jeos/conventions/api-routes-catalog.md` | Add Sub-spec 4 routes. |
| `.jeos/conventions/valkey-keys-catalog.md` | Add `jc:jce:freshness:*`, `jc:jce:advisor:*`, `jc:jce:refresh_lock:*`. |
| `.jeos/conventions/system-settings-catalog.md` | Add `jce.freshness_*` and `jce.advisor_*` settings. |
| `apps/main/src/lib/api/jce.ts` | New client calls for freshness, refresh, and AI advisor endpoints. |
| `apps/main/src/lib/types/jce.ts` | New TypeScript types for `DataSource`, `FreshnessSummary`, `AdvisorResult`. |
| `apps/main/src/lib/components/compare/FreshnessChip.svelte` | New component for cell-level freshness status. |
| `apps/main/src/lib/components/compare/AIAdvisorDrawer.svelte` | New component for the streaming/non-streaming AI advisor. |
| `apps/main/src/routes/compare/[slug]/+page.svelte` and `comparisons/[slug]/+page.svelte` | Integrate freshness chips and advisor drawer. |
| `apps/main/src/lib/i18n/{en,fr}.json` | Add `jce.freshness.*`, `jce.advisor.*`, `jce.refresh.*` keys. |
| `apps/admin/src/routes/comparison-data-sources/+page.svelte` | Admin list/edit for data sources. |
| `apps/admin/src/routes/comparison-data-sources/+page.server.ts` | Server load/actions for data sources. |
| `internal/jce/adapter.go` | Convert typed `AdvisorResult`/`FreshnessSummary` values into `map[string]any` for AI tools. |
| `apps/admin/src/routes/comparison-ingestion-jobs/+page.svelte` | Admin list for ingestion jobs. |
| `apps/admin/src/routes/comparison-ingestion-jobs/+page.server.ts` | Server load for ingestion jobs. |
| `apps/admin/src/lib/api/comparison-data-sources.ts` | Admin client calls for data source CRUD. |
| `apps/admin/src/lib/types/comparison-data-sources.ts` | Admin Svelte types for data sources and ingestion jobs. |

---

## Implementation Status

| # | Area | Status |
|---|---|---|
| 1 | Schema and migrations `000573`–`000578` | `PLANNED` |
| 2 | sqlc queries + domain models | `PLANNED` |
| 3 | Data source service | `PLANNED` |
| 4 | Freshness service | `PLANNED` |
| 5 | Ingestion jobs + refresh worker | `PLANNED` |
| 6 | AI advisor service | `PLANNED` |
| 7 | HTTP handlers + routes | `PLANNED` |
| 8 | AI agent tools + registration | `PLANNED` |
| 9 | JEOS catalog updates | `PLANNED` |
| 10 | Server wiring + service integration | `PLANNED` |
| 11 | `apps/main` freshness, refresh, and advisor UI | `PLANNED` |
| 12 | `apps/admin` data source + ingestion job UI | `PLANNED` |
| 13 | Verification + smoke test | `PLANNED` |

---

## Task 1: Schema and migrations

**Files:**
- Create: `migrations/000573_jce_data_sources.up.sql`
- Create: `migrations/000573_jce_data_sources.down.sql`
- Create: `migrations/000574_jce_value_freshness.up.sql`
- Create: `migrations/000574_jce_value_freshness.down.sql`
- Create: `migrations/000575_jce_ingestion_jobs.up.sql`
- Create: `migrations/000575_jce_ingestion_jobs.down.sql`
- Create: `migrations/000576_jce_subspec4_rbac.up.sql`
- Create: `migrations/000576_jce_subspec4_rbac.down.sql`
- Create: `migrations/000577_jce_ai_advisor_tools.up.sql`
- Create: `migrations/000577_jce_ai_advisor_tools.down.sql`
- Create: `migrations/000578_jce_subspec4_settings.up.sql`
- Create: `migrations/000578_jce_subspec4_settings.down.sql`

**Interfaces:**
- Produces: `comparison_data_sources`, `comparison_data_ingestion_jobs` tables and new `comparison_values` columns, plus seed rows for feature flags, RBAC, `ai_tools`, and `system_settings`.

- [ ] **Step 1: Write migration 000573 (data sources)**

```sql
-- migrations/000573_jce_data_sources.up.sql
BEGIN;

CREATE TABLE IF NOT EXISTS comparison_data_sources (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    entity_type     text,
    entity_id       uuid,
    dimension_slug  text,
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
CREATE UNIQUE INDEX IF NOT EXISTS idx_comparison_data_sources_tenant_name
    ON comparison_data_sources(tenant_id, source_name);

COMMIT;
```

```sql
-- migrations/000573_jce_data_sources.down.sql
BEGIN;
DROP TABLE IF EXISTS comparison_data_sources CASCADE;
COMMIT;
```

- [ ] **Step 2: Write migration 000574 (value freshness)**

```sql
-- migrations/000574_jce_value_freshness.up.sql
BEGIN;

ALTER TABLE comparison_values
    ADD COLUMN IF NOT EXISTS updated_at timestamptz,
    ADD COLUMN IF NOT EXISTS data_source_id uuid REFERENCES comparison_data_sources(id) ON DELETE SET NULL;

UPDATE comparison_values SET updated_at = created_at WHERE updated_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_comparison_values_data_source
    ON comparison_values(data_source_id);

COMMIT;
```

```sql
-- migrations/000574_jce_value_freshness.down.sql
BEGIN;
DROP INDEX IF EXISTS idx_comparison_values_data_source;
ALTER TABLE comparison_values
    DROP COLUMN IF EXISTS data_source_id,
    DROP COLUMN IF EXISTS updated_at;
COMMIT;
```

- [ ] **Step 3: Write migration 000575 (ingestion jobs)**

```sql
-- migrations/000575_jce_ingestion_jobs.up.sql
BEGIN;

CREATE TABLE IF NOT EXISTS comparison_data_ingestion_jobs (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    content_id      uuid NOT NULL REFERENCES contents(id) ON DELETE CASCADE,
    status          text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','running','completed','failed','cancelled')),
    source_kind     text,
    refreshed_count int NOT NULL DEFAULT 0,
    error           text,
    started_at      timestamptz,
    completed_at    timestamptz,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_comp_ingestion_jobs_tenant_content
    ON comparison_data_ingestion_jobs(tenant_id, content_id, created_at DESC);

COMMIT;
```

```sql
-- migrations/000575_jce_ingestion_jobs.down.sql
BEGIN;
DROP TABLE IF EXISTS comparison_data_ingestion_jobs CASCADE;
COMMIT;
```

- [ ] **Step 4: Write migration 000576 (RBAC + feature flags)**

```sql
-- migrations/000576_jce_subspec4_rbac.up.sql
BEGIN;

INSERT INTO feature_flags (key, tenant_id, enabled, config)
VALUES
    ('feature.jce.data_sources', '00000000-0000-0000-0000-000000000001', false, '{}'),
    ('feature.jce.refresh',      '00000000-0000-0000-0000-000000000001', false, '{}'),
    ('feature.jce.ai_advisor',   '00000000-0000-0000-0000-000000000001', false, '{}')
ON CONFLICT (key, tenant_id) DO UPDATE SET
    enabled = EXCLUDED.enabled,
    config = EXCLUDED.config;

INSERT INTO rbac_nodes (key, tenant_id, name, description)
VALUES
    ('jce.data_source.manage', '00000000-0000-0000-0000-000000000001', 'Manage comparison data sources', 'Create/update/delete data source definitions'),
    ('jce.data_source.view',   '00000000-0000-0000-0000-000000000001', 'View comparison data sources',   'List and view data source definitions'),
    ('jce.refresh.trigger',    '00000000-0000-0000-0000-000000000001', 'Trigger comparison refresh',     'Enqueue a refresh of comparison values'),
    ('jce.ai_advisor.use',     '00000000-0000-0000-0000-000000000001', 'Use JCE AI advisor',             'Ask the AI advisor questions about a comparison'),
    ('jce.ingestion_job.view', '00000000-0000-0000-0000-000000000001', 'View ingestion jobs',            'View refresh job history')
ON CONFLICT (key, tenant_id) DO UPDATE SET
    name = EXCLUDED.name,
    description = EXCLUDED.description;

INSERT INTO permission_grants (role_id, node_id)
SELECT r.id, n.id
FROM user_roles r, rbac_nodes n
WHERE r.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND r.name IN ('super_admin','admin')
  AND n.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND n.key IN ('jce.data_source.manage','jce.data_source.view','jce.refresh.trigger','jce.ai_advisor.use','jce.ingestion_job.view')
ON CONFLICT DO NOTHING;

COMMIT;
```

```sql
-- migrations/000576_jce_subspec4_rbac.down.sql
BEGIN;
DELETE FROM permission_grants
WHERE node_id IN (
    SELECT id FROM rbac_nodes
    WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
      AND key IN ('jce.data_source.manage','jce.data_source.view','jce.refresh.trigger','jce.ai_advisor.use','jce.ingestion_job.view')
);
DELETE FROM rbac_nodes
WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND key IN ('jce.data_source.manage','jce.data_source.view','jce.refresh.trigger','jce.ai_advisor.use','jce.ingestion_job.view');
DELETE FROM feature_flags
WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND key IN ('feature.jce.data_sources','feature.jce.refresh','feature.jce.ai_advisor');
COMMIT;
```

- [ ] **Step 5: Write migration 000577 (AI tools)**

```sql
-- migrations/000577_jce_ai_advisor_tools.up.sql
BEGIN;

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
            "question":   { "type": "string", "description": "User question, e.g. Which is best for me?" }
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

COMMIT;
```

```sql
-- migrations/000577_jce_ai_advisor_tools.down.sql
BEGIN;
DELETE FROM ai_tools
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND name IN ('comparison.analyze','comparison.rank','comparison.explain','comparison.recommend_next_step','comparison.refresh');
COMMIT;
```

- [ ] **Step 6: Write migration 000578 (system settings)**

```sql
-- migrations/000578_jce_subspec4_settings.up.sql
BEGIN;

INSERT INTO system_settings (tenant_id, key, value) VALUES
    ('00000000-0000-0000-0000-000000000001', 'jce.freshness_stale_hours',        to_jsonb(168)),
    ('00000000-0000-0000-0000-000000000001', 'jce.freshness_decay_per_hour',     to_jsonb(2)),
    ('00000000-0000-0000-0000-000000000001', 'jce.freshness_official_weight',    to_jsonb(100)),
    ('00000000-0000-0000-0000-000000000001', 'jce.freshness_third_party_weight', to_jsonb(75)),
    ('00000000-0000-0000-0000-000000000001', 'jce.freshness_ai_estimate_weight', to_jsonb(50)),
    ('00000000-0000-0000-0000-000000000001', 'jce.freshness_cache_ttl_minutes',  to_jsonb(10)),
    ('00000000-0000-0000-0000-000000000001', 'jce.advisor_model',                '""'),
    ('00000000-0000-0000-0000-000000000001', 'jce.advisor_max_tokens',           to_jsonb(1024)),
    ('00000000-0000-0000-0000-000000000001', 'jce.advisor_cache_ttl_minutes',    to_jsonb(15))
ON CONFLICT (tenant_id, key) DO NOTHING;

COMMIT;
```

```sql
-- migrations/000578_jce_subspec4_settings.down.sql
BEGIN;
DELETE FROM system_settings
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND (key LIKE 'jce.freshness_%' OR key LIKE 'jce.advisor_%');
COMMIT;
```

- [ ] **Step 7: Apply migrations locally and verify**

Run: `make migrate` (or `go run ./cmd/migrate up` per project convention)
Expected: all six migrations apply without error.

- [ ] **Step 8: Commit**

```bash
git add migrations/000573_jce_data_sources.* migrations/000574_jce_value_freshness.* migrations/000575_jce_ingestion_jobs.* migrations/000576_jce_subspec4_rbac.* migrations/000577_jce_ai_advisor_tools.* migrations/000578_jce_subspec4_settings.*
git commit -m "feat(jce): add Sub-spec 4 schema, permissions, and settings"
```

---

## Task 2: sqlc queries and domain models

**Files:**
- Modify: `internal/db/queries/jce.sql`
- Modify: `internal/db/queries/comparison.sql`
- Modify: `internal/jce/models.go`
- Test: `internal/jce/models_test.go` (new, or extend `internal/jce/helpers_test.go`)

**Interfaces:**
- Consumes: schema from Task 1.
- Produces: `gen.CreateDataSource`, `gen.ListDataSources`, `gen.GetDataSource`, `gen.UpdateDataSource`, `gen.DeleteDataSource`, `gen.MatchDataSource`, `gen.CreateIngestionJob`, `gen.GetIngestionJob`, `gen.UpdateIngestionJob`, `gen.ListIngestionJobsByContent`, plus updated `gen.UpsertComparisonValue` / `gen.UpsertComparisonValueWithMetadata` returning `updated_at` and `data_source_id`.

- [ ] **Step 1: Add data source queries to `internal/db/queries/jce.sql`**

```sql
-- name: CreateDataSource :one
INSERT INTO comparison_data_sources (
    tenant_id, entity_type, entity_id, dimension_slug, source_name, source_url, source_kind, trust_level, config
) VALUES (
    @tenant_id, sqlc.narg(entity_type), sqlc.narg(entity_id), sqlc.narg(dimension_slug), @source_name, sqlc.narg(source_url), @source_kind, @trust_level, @config
)
RETURNING *;

-- name: ListDataSources :many
SELECT * FROM comparison_data_sources
WHERE tenant_id = @tenant_id
  AND (sqlc.narg(entity_type)::text IS NULL OR entity_type = sqlc.narg(entity_type)::text)
  AND (sqlc.narg(entity_id)::uuid IS NULL OR entity_id = sqlc.narg(entity_id)::uuid)
ORDER BY source_name ASC;

-- name: GetDataSource :one
SELECT * FROM comparison_data_sources
WHERE id = @id AND tenant_id = @tenant_id;

-- name: UpdateDataSource :one
UPDATE comparison_data_sources
SET entity_type = sqlc.narg(entity_type),
    entity_id = sqlc.narg(entity_id),
    dimension_slug = sqlc.narg(dimension_slug),
    source_name = @source_name,
    source_url = sqlc.narg(source_url),
    source_kind = @source_kind,
    trust_level = @trust_level,
    freshness_score = sqlc.narg(freshness_score),
    last_verified_at = sqlc.narg(last_verified_at),
    config = @config,
    updated_at = now()
WHERE id = @id AND tenant_id = @tenant_id
RETURNING *;

-- name: DeleteDataSource :exec
DELETE FROM comparison_data_sources
WHERE id = @id AND tenant_id = @tenant_id;

-- name: MatchDataSource :one
-- Finds the most specific data source for the given scope, falling back to more general scopes.
SELECT * FROM comparison_data_sources
WHERE tenant_id = @tenant_id
  AND source_name = @source_name
  AND (entity_type IS NOT DISTINCT FROM sqlc.narg(entity_type)::text)
  AND (entity_id IS NOT DISTINCT FROM sqlc.narg(entity_id)::uuid)
  AND (dimension_slug IS NOT DISTINCT FROM sqlc.narg(dimension_slug)::text);
```

- [ ] **Step 2: Add ingestion job queries to `internal/db/queries/jce.sql`**

```sql
-- name: CreateIngestionJob :one
INSERT INTO comparison_data_ingestion_jobs (
    tenant_id, content_id, source_kind
) VALUES (
    @tenant_id, @content_id, sqlc.narg(source_kind)
)
RETURNING *;

-- name: GetIngestionJob :one
SELECT * FROM comparison_data_ingestion_jobs
WHERE id = @id AND tenant_id = @tenant_id;

-- name: UpdateIngestionJob :one
UPDATE comparison_data_ingestion_jobs
SET status = @status,
    refreshed_count = @refreshed_count,
    error = sqlc.narg(error),
    started_at = sqlc.narg(started_at),
    completed_at = sqlc.narg(completed_at),
    updated_at = now()
WHERE id = @id AND tenant_id = @tenant_id
RETURNING *;

-- name: ListIngestionJobsByContent :many
SELECT * FROM comparison_data_ingestion_jobs
WHERE tenant_id = @tenant_id AND content_id = @content_id
ORDER BY created_at DESC
LIMIT @limit_val OFFSET @offset_val;

-- name: GetLatestCompletedIngestionJob :one
SELECT * FROM comparison_data_ingestion_jobs
WHERE tenant_id = @tenant_id AND content_id = @content_id AND status = 'completed'
ORDER BY completed_at DESC
LIMIT 1;
```

- [ ] **Step 3: Update `comparison_values` queries in `internal/db/queries/comparison.sql`**

Update `GetComparisonValue` and `GetComparisonValuesByContent` to select `updated_at` and `data_source_id` so they scan into the regenerated `gen.ComparisonValue` struct.

Change `UpsertComparisonValue` and `UpsertComparisonValueWithMetadata` insert columns to include `updated_at` (set to `now()` in both the `INSERT` and `ON CONFLICT ... SET` clauses). Add `data_source_id` to both variants, and update the `RETURNING` clause to include `updated_at` and `data_source_id`.

Also modify `internal/comparison/service.go` (the input struct used by `Service.UpsertComparisonValueWithMetadata`) to add `DataSourceID`; the existing metadata bytes argument stays a separate `[]byte` parameter:

```go
type UpsertComparisonValueInput struct {
    DimensionID  pgtype.UUID
    ItemID       pgtype.UUID
    Value        string
    ValueRating  *float64
    ValueBoolean *bool
    DataSourceID *pgtype.UUID
}
```

Keep `Service.UpsertComparisonValueWithMetadata(ctx, ID, input UpsertComparisonValueInput, metadata []byte)`. Update `Store.upsertComparisonValueWithMetadata` to pass `DataSourceID` to the generated query.

Example snippet for `UpsertComparisonValue`:

```sql
INSERT INTO comparison_values (
    dimension_id, item_id, tenant_id, value, value_rating, value_boolean, data_source_id, updated_at
) VALUES (
    @dimension_id, @item_id, @tenant_id, sqlc.narg(value), sqlc.narg(value_rating), sqlc.narg(value_boolean), sqlc.narg(data_source_id), now()
)
ON CONFLICT (dimension_id, item_id) DO UPDATE
SET value = EXCLUDED.value,
    value_rating = EXCLUDED.value_rating,
    value_boolean = EXCLUDED.value_boolean,
    data_source_id = EXCLUDED.data_source_id,
    updated_at = now()
RETURNING dimension_id, item_id, tenant_id, value, value_rating, value_boolean, created_at, updated_at, metadata, data_source_id;
```

Example snippet for `UpsertComparisonValueWithMetadata`:

```sql
INSERT INTO comparison_values (
    dimension_id, item_id, tenant_id, value, value_rating, value_boolean, metadata, data_source_id, updated_at
) VALUES (
    @dimension_id, @item_id, @tenant_id, sqlc.narg(value), sqlc.narg(value_rating), sqlc.narg(value_boolean), sqlc.narg(metadata), sqlc.narg(data_source_id), now()
)
ON CONFLICT (dimension_id, item_id) DO UPDATE
SET value = EXCLUDED.value,
    value_rating = EXCLUDED.value_rating,
    value_boolean = EXCLUDED.value_boolean,
    metadata = COALESCE(EXCLUDED.metadata, comparison_values.metadata),
    data_source_id = EXCLUDED.data_source_id,
    updated_at = now()
RETURNING dimension_id, item_id, tenant_id, value, value_rating, value_boolean, created_at, updated_at, metadata, data_source_id;
```

- [ ] **Step 4: Add domain models to `internal/jce/models.go`**

```go
type DataSource struct {
    ID             string         `json:"id"`
    ID       string         `json:"tenant_id"`
    EntityType     *string        `json:"entity_type,omitempty"`
    EntityID       *string        `json:"entity_id,omitempty"`
    DimensionSlug  *string        `json:"dimension_slug,omitempty"`
    SourceName     string         `json:"source_name"`
    SourceURL      *string        `json:"source_url,omitempty"`
    SourceKind     string         `json:"source_kind"`
    TrustLevel     int            `json:"trust_level"`
    FreshnessScore int            `json:"freshness_score"`
    LastVerifiedAt *string        `json:"last_verified_at,omitempty"`
    Config         map[string]any `json:"config"`
    CreatedAt      string         `json:"created_at"`
    UpdatedAt      string         `json:"updated_at"`
}

type DataSourceInput struct {
    EntityType    *string
    EntityID      *string
    DimensionSlug *string
    SourceName    string
    SourceURL     *string
    SourceKind    string
    TrustLevel    int
    Config        map[string]any
}

type IngestionJob struct {
    ID             string  `json:"id"`
    ID       string  `json:"tenant_id"`
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

type FreshnessCell struct {
    DimensionID    string  `json:"dimension_id"`
    Label          string  `json:"label"`
    FreshnessScore int     `json:"freshness_score"`
    SourceName     string  `json:"source_name,omitempty"`
    SourceKind     string  `json:"source_kind,omitempty"`
    RetrievedAt    *string `json:"retrieved_at,omitempty"`
    IsStale        bool    `json:"is_stale"`
}

type FreshnessItem struct {
    ItemID         string          `json:"item_id"`
    ItemTitle      string          `json:"item_title"`
    FreshnessScore int             `json:"freshness_score"`
    Cells          []FreshnessCell `json:"cells"`
}

type FreshnessSummary struct {
    ContentID             string          `json:"content_id"`
    OverallFreshnessScore int             `json:"overall_freshness_score"`
    StaleCount            int             `json:"stale_count"`
    TotalCount            int             `json:"total_count"`
    ByItem                []FreshnessItem `json:"by_item"`
}

type AdvisorInput struct {
    ContentID  pgtype.UUID  `json:"content_id"`
    ID   pgtype.UUID  `json:"tenant_id"`
    UserID     pgtype.UUID  `json:"user_id"`
    ProfileID  *pgtype.UUID `json:"profile_id,omitempty"`
    Question   string       `json:"question"`
    ItemID     *pgtype.UUID `json:"item_id,omitempty"`
    FactorKey  string       `json:"factor_key"`
    SourceKind string       `json:"source_kind"`
}

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
    ItemID    string  `json:"item_id"`
    ItemTitle string  `json:"item_title"`
    Rank      int     `json:"rank"`
    JapaScore float64 `json:"japa_score"`
    Reasoning string  `json:"reasoning"`
    Eligible  bool    `json:"eligible"`
}

type NextAction struct {
    ItemID     string `json:"item_id,omitempty"`
    Text       string `json:"text"`
    ActionType string `json:"action_type,omitempty"`
    ActionURL  string `json:"action_url,omitempty"`
    Priority   int    `json:"priority"`
}
```

Copy the exact struct definitions from `docs/superpowers/specs/2026-08-30-jce-subspec-4-design.md` §7 and §10.

- [ ] **Step 5: Regenerate sqlc code**

Run: `make sqlc-gen`
Expected: `go build ./...` still compiles and `internal/db/gen` includes the new query methods.

- [ ] **Step 6: Write a model mapping test**

Add `internal/jce/models_test.go` with a round-trip test that JSON-marshals and unmarshals `DataSource`, `IngestionJob`, and `AdvisorResult` to catch field mismatches.

Run: `go test ./internal/jce/... -run TestModelJSON -v`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/db/queries/jce.sql internal/db/queries/comparison.sql internal/jce/models.go internal/jce/models_test.go
git commit -m "feat(jce): add Sub-spec 4 sqlc queries and domain models"
```

---

## Task 3: Data source service

**Files:**
- Create: `internal/jce/datasource.go`
- Modify: `internal/jce/service.go` (add data source methods)
- Test: `internal/jce/datasource_test.go`

**Interfaces:**

```go
package jce

import (
    "context"
    "errors"

    "github.com/jackc/pgx/v5/pgtype"
    "github.com/japaconnect/japaconnect/internal/db/gen"
)

type DataSourceService interface {
    ListDataSources(ctx context.Context, ID pgtype.UUID, entityType *string, entityID *pgtype.UUID) ([]DataSource, error)
    CreateDataSource(ctx context.Context, ID pgtype.UUID, in DataSourceInput) (DataSource, error)
    GetDataSource(ctx context.Context, id, ID pgtype.UUID) (DataSource, error)
    UpdateDataSource(ctx context.Context, id, ID pgtype.UUID, in DataSourceInput) (DataSource, error)
    DeleteDataSource(ctx context.Context, id, ID pgtype.UUID) error
    MatchDataSource(ctx context.Context, ID pgtype.UUID, entityType *string, entityID *pgtype.UUID, dimensionSlug, sourceName string) (*DataSource, error)
}
```

**Steps:**

- [ ] **Step 1: Implement `dataSourceStore` struct** wrapping `gen.Queries` (construct with `gen.New(database.Reader)` and `gen.New(database.Writer)`).
- [ ] **Step 2: Implement `toDataSource` and `fromDataSourceInput` mapping functions** using jsonb for `config`. Validate `SourceKind` is one of `official`, `third_party`, `ai_estimate` and `TrustLevel` is 1–5.
- [ ] **Step 3: Implement `ListDataSources`**, handling `entityType` as optional via pointer. Return `[]DataSource` ordered by `source_name`.
- [ ] **Step 4: Implement `MatchDataSource`** with five specificity passes:
    1. Exact `(entityType, entityID, dimensionSlug, sourceName)`.
    2. `(entityType, entityID, NULL, sourceName)`.
    3. `(entityType, NULL, dimensionSlug, sourceName)`.
    4. `(entityType, NULL, NULL, sourceName)`.
    5. If entity-specific match not found, repeat with `entityType = NULL`.
    Stop at first result; if no `data_source` row, fall back to `metadata.source` of the value.
- [ ] **Step 5: Add `DataSourceService` methods to `Service` in `internal/jce/service.go`.**
- [ ] **Step 6: Write unit tests** covering:
    - create/read/update/delete isolation by tenant,
    - `source_kind` and `trust_level` validation,
    - source matching specificity and fallback.

**Verification:**
```bash
go test ./internal/jce/... -run TestDataSource -v
go vet ./...
```
Expected: all PASS.

**Commit:**
```bash
git add internal/jce/datasource.go internal/jce/datasource_test.go internal/jce/service.go
git commit -m "feat(jce): add data source service"
```

---

## Task 4: Freshness service

**Files:**
- Create: `internal/jce/freshness.go`
- Modify: `internal/jce/service.go` (add `freshness` field and `FreshnessForComparison`)
- Test: `internal/jce/freshness_test.go`

**Interfaces:**

```go
type FreshnessService interface {
    FreshnessForComparison(ctx context.Context, contentID, ID, userID pgtype.UUID) (FreshnessSummary, error)
    cellFreshness(ctx context.Context, v gen.ComparisonValue, source *DataSource, settings freshnessSettings) (int, bool, error)
}

// Signature on Service:
func (s *Service) FreshnessForComparison(ctx context.Context, contentID, ID, userID pgtype.UUID) (FreshnessSummary, error)

// freshnessSettings groups the freshness-related system_settings with safe defaults.
type freshnessSettings struct {
    staleHours        int
    decayPerHour      int
    officialWeight    int
    thirdPartyWeight  int
    aiEstimateWeight  int
    cacheTTLMinutes   int
}
```

**Steps:**

- [ ] **Step 1: Add `loadFreshnessSettings` to `freshness.go`** using the injected `*config.Settings` (`s.settings`) with `GetIntOr` and safe defaults:

```go
func (s *Service) loadFreshnessSettings(ctx context.Context, ID pgtype.UUID) freshnessSettings {
    return freshnessSettings{
        staleHours:       s.settings.GetIntOr(ctx, ID, "jce.freshness_stale_hours", 168),
        decayPerHour:     s.settings.GetIntOr(ctx, ID, "jce.freshness_decay_per_hour", 2),
        officialWeight:   s.settings.GetIntOr(ctx, ID, "jce.freshness_official_weight", 100),
        thirdPartyWeight: s.settings.GetIntOr(ctx, ID, "jce.freshness_third_party_weight", 75),
        aiEstimateWeight: s.settings.GetIntOr(ctx, ID, "jce.freshness_ai_estimate_weight", 50),
        cacheTTLMinutes:  s.settings.GetIntOr(ctx, ID, "jce.freshness_cache_ttl_minutes", 10),
    }
}
```

- [ ] **Step 2: Implement `cellFreshness` in `internal/jce/freshness.go`**:

```go
func (s *Service) cellFreshness(
    ctx context.Context,
    v gen.ComparisonValue,
    source *DataSource,
    settings freshnessSettings,
) (int, bool, error) {
    now := time.Now().UTC()

    kindWeight := settings.officialWeight
    trustLevel := 3
    if source != nil {
        trustLevel = source.TrustLevel
        switch source.SourceKind {
        case "third_party":
            kindWeight = settings.thirdPartyWeight
        case "ai_estimate":
            kindWeight = settings.aiEstimateWeight
        }
    }

    updatedAt := v.CreatedAt.Time
    if v.UpdatedAt.Valid && !v.UpdatedAt.Time.IsZero() {
        updatedAt = v.UpdatedAt.Time
    }

    staleHours := settings.staleHours
    decayPerHour := settings.decayPerHour

    ageHours := now.Sub(updatedAt).Hours() - float64(staleHours)
    if ageHours < 0 {
        ageHours = 0
    }
    decay := ageHours * float64(decayPerHour)

    var md map[string]any
    if len(v.Metadata) > 0 {
        _ = json.Unmarshal(v.Metadata, &md)
    }

    confidence := 1.0
    if c, ok := md["confidence"].(string); ok {
        switch c {
        case "high":
            confidence = 1.0
        case "medium":
            confidence = 0.75
        case "low":
            confidence = 0.5
        }
    }

    penalty := 1.0
    if est, ok := md["is_estimate"].(bool); ok && est {
        penalty = 0.85
    }

    base := float64(kindWeight) / 100.0
    trust := float64(trustLevel) / 5.0
    score := (100.0 - decay) * base * trust * confidence * penalty

    if score < 0 {
        score = 0
    }
    if score > 100 {
        score = 100
    }
    isStale := now.Sub(updatedAt).Hours() > float64(staleHours)
    return int(math.Round(score)), isStale, nil
}
```

- [ ] **Step 3: Implement `FreshnessForComparison`**.
    - Load items and dimensions with `s.compSvc.GetJCEComparisonMatrix`.
    - Load all raw `comparison_values` for the content with `gen.New(s.db.Reader).GetComparisonValuesByContent` (this returns `[]gen.ComparisonValue` including `updated_at`, `data_source_id`, and `metadata`).
    - Load all tenant data sources with `s.dataSources.ListDataSources(ctx, ID, nil, nil)` and index them by ID.
    - For each value, resolve the source: if `v.DataSourceID.Valid`, use the indexed data source; otherwise call `Service.MatchDataSource` using `entityType = "content"` (as a `*string`), the value's `item_id` and `dimension_id`, and `metadata.source` as the source name.
    - Call `cellFreshness` and aggregate overall and per-item scores. Return the JSON shape from design spec §8.
- [ ] **Step 4: Cache summary in Valkey**. After computing the summary in `FreshnessForComparison`, store it with `s.vk.Set(ctx, fmt.Sprintf("jc:jce:freshness:%s:%s", ID, contentID), b, time.Duration(ttlMinutes)*time.Minute)`. Before computing, attempt a cache hit with `s.vk.Get(ctx, key)` and unmarshal the JSON on a hit.
- [ ] **Step 5: Write tests** covering:
    - missing metadata falls back to default confidence,
    - `is_estimate` applies 0.85 penalty,
    - stale threshold (`updated_at > stale_hours`) begins decay,
    - `source_kind` weights (official=1.0, third_party=0.75, ai_estimate=0.5),
    - trust 1–5 scales linearly.

**Verification:**
```bash
go test ./internal/jce/... -run TestFreshness -v
go vet ./...
```
Expected: all PASS.

**Commit:**
```bash
git add internal/jce/freshness.go internal/jce/freshness_test.go internal/jce/service.go
git commit -m "feat(jce): add freshness scoring and summary"
```

---

## Task 5: Ingestion jobs and refresh worker

**Files:**
- Create: `internal/jce/ingestion.go`
- Modify: `internal/jce/service.go`
- Create: `internal/jce/ingestion_test.go`
- Modify: `internal/worker/tasks.go`
- Modify: `internal/worker/handlers.go`
- Create: `internal/worker/jce_refresh.go` (or append to `handlers.go`)

**Interfaces:**

```go
type IngestionService interface {
    RefreshComparison(ctx context.Context, contentID, ID, userID pgtype.UUID, sourceKind *string) (IngestionJob, error)
    GetIngestionJob(ctx context.Context, id, ID pgtype.UUID) (IngestionJob, error)
    ListIngestionJobs(ctx context.Context, contentID, ID pgtype.UUID, limit, offset int) ([]IngestionJob, error)
    ProcessRefreshValues(ctx context.Context, ID, contentID, jobID pgtype.UUID, sourceKind *string, userID pgtype.UUID) error
}
```

Worker payload:

```go
const TypeJCERefreshValues = "jce:refresh_values"

type JceRefreshValuesPayload struct {
    ID       string  `json:"tenant_id"`
    ContentID      string  `json:"content_id"`
    SourceKind     *string `json:"source_kind,omitempty"`
    IngestionJobID string  `json:"ingestion_job_id"`
    RequestedBy    string  `json:"requested_by_user_id"`
}
```

**Steps:**

- [ ] **Step 1: Add `TypeJCERefreshValues` to `internal/worker/tasks.go` under default-tier tasks.**
- [ ] **Step 2: Implement `CreateIngestionJob`, `UpdateIngestionJob`, `GetIngestionJob`, and `ListIngestionJobs` in `internal/jce/ingestion.go`.**
- [ ] **Step 3: Implement `RefreshComparison`.**
    - Check the cache key `jc:jce:refresh_lock:{content_id}` using `s.vk.SetNX(ctx, key, "1", 5*time.Minute)`.
    - If the lock exists, find the most recent pending `ingestion_job` for this content and return it.
    - If no job exists, create a `pending` ingestion job in `comparison_data_ingestion_jobs`.
    - Build a `JceRefreshValuesPayload`, then enqueue it with `s.enqueuer.Enqueue(TypeJCERefreshValues, payloadBytes, asynq.Queue("default"), asynq.MaxRetry(3))`.
    - Return the job with `status: "pending"`.
- [ ] **Step 4: Implement `ProcessRefreshValues` in `internal/jce/ingestion.go`.**

```go
func (s *Service) ProcessRefreshValues(
    ctx context.Context,
    ID, contentID, jobID pgtype.UUID,
    sourceKind *string,
    userID pgtype.UUID,
) error {
    job, err := s.UpdateIngestionJob(ctx, jobID, ID, IngestionJob{
        Status: "running",
    })
    if err != nil {
        return err
    }

    matrix, err := s.comparison.GetJCEComparisonMatrix(ctx, contentID, ID)
    if err != nil {
        return s.failJob(ctx, job.ID, ID, err)
    }

    refreshed := 0
    for _, item := range matrix {
        itemID := item.Item.ID
        for _, dim := range item.Dimensions {
            dimID := dim.DimensionID

            source, err := s.resolveRefreshSource(ctx, ID, contentID, itemID, dimID, sourceKind)
            if err != nil {
                continue
            }

            filled, metaBytes, err := s.resolveRefreshValue(ctx, ID, contentID, itemID, dimID, source, sourceKind)
            if err != nil {
                continue
            }

            dsID := strToUUID(source.ID)
            filled.DataSourceID = &dsID
            _, err = s.comparison.UpsertComparisonValueWithMetadata(ctx, ID, filled, metaBytes)
            if err != nil {
                continue
            }
            refreshed++
        }
    }

    _, err = s.UpdateIngestionJob(ctx, job.ID, ID, IngestionJob{
        Status:         "completed",
        RefreshedCount: refreshed,
        CompletedAt:    ptrString(time.Now().UTC().Format(time.RFC3339)),
    })
    if err != nil {
        return s.failJob(ctx, job.ID, ID, err)
    }

    // invalidate caches
    _ = s.vk.Del(ctx, fmt.Sprintf("jc:jce:freshness:%s:%s", ID, contentID))
    return nil
}
```

Provide helper methods and conversions:

```go
func ptrString(s string) *string { return &s }

func strToUUID(s string) pgtype.UUID {
    var u pgtype.UUID
    _ = u.Scan(s)
    return u
}

func strToUUIDPtr(s string) *pgtype.UUID {
    if s == "" { return nil }
    u := strToUUID(s)
    return &u
}

func (s *Service) failJob(ctx context.Context, jobID, ID pgtype.UUID, cause error) error {
    _, _ = s.UpdateIngestionJob(ctx, jobID, ID, IngestionJob{
        Status:      "failed",
        Error:       ptrString(cause.Error()),
        CompletedAt: ptrString(time.Now().UTC().Format(time.RFC3339)),
    })
    return cause
}

func (s *Service) resolveRefreshSource(
    ctx context.Context,
    ID, contentID, itemID, dimID pgtype.UUID,
    sourceKind *string,
) (*DataSource, error) {
    // Match a source for this cell. The exact scope (entity_type, entity_id, dimension_slug) is derived from
    // the comparison context and the dimension; the source name falls back to the existing value's metadata.
    // If sourceKind is set, only sources of that kind are eligible.
    return nil, errors.New("not implemented")
}

func (s *Service) resolveRefreshValue(
    ctx context.Context,
    ID, contentID, itemID, dimID pgtype.UUID,
    source *DataSource,
    sourceKind *string,
) (comparison.UpsertComparisonValueInput, []byte, error) {
    switch source.SourceKind {
    case "official", "third_party":
        // Use the resolver/fetcher registered for this entity type, entity, or dimension, or fall back to an
        // external.fetch call if the source config provides a URL. The exact adapter is chosen by the source
        // config key `adapter` ("resolver", "external", "static"). The value must be auditable and tenant-scoped.
    case "ai_estimate":
        // Dispatch the existing `comparison.fill_values` tool through the AI executor with the source metadata
        // `{"source":"ai_estimate","data_source_id":source.ID}`. Only use this when no official/third-party
        // source is configured or when the source_kind filter explicitly requests `ai_estimate`.
    default:
        return comparison.UpsertComparisonValueInput{}, nil, fmt.Errorf("unsupported source_kind: %s", source.SourceKind)
    }
    return comparison.UpsertComparisonValueInput{}, nil, nil
}
```

Implementation notes:

- Skip cells that already have a value and a `cellFreshness` score >= 50; to compute this, load the raw `gen.ComparisonValue` rows (with `updated_at`, `data_source_id`, `metadata`) for the content and index them by `(item_id, dimension_id)`.
- `resolveRefreshSource` must call `s.MatchDataSource` with the appropriate scope, falling back to `metadata.source` when no scoped source exists.
- `resolveRefreshValue` must branch on `source.SourceKind`: use resolver/external/static adapters for `official`/`third_party`; use the existing comparison AI fill-values path for `ai_estimate`; preserve `data_source_id` and provenance metadata.
- After a successful refresh, delete the freshness cache key `jc:jce:freshness:{tenant_id}:{content_id}` and release the refresh lock.

- [ ] **Step 5: Implement worker handler `HandleJceRefreshValues` and register in `RegisterHandlers`.**

Follow the existing worker dependency pattern (package-level registration) to avoid import cycles and to keep `RegisterHandlers` parameter-free:

```go
// internal/worker/handlers.go (or a new internal/worker/jce_refresh.go file)

var jceIngestionSvc jce.IngestionService

func RegisterJCEService(s jce.IngestionService) {
    jceIngestionSvc = s
}

func HandleJceRefreshValues(ctx context.Context, t *asynq.Task) error {
    if jceIngestionSvc == nil {
        return fmt.Errorf("jce refresh handler not wired: call worker.RegisterJCEService")
    }
    var p JceRefreshValuesPayload
    if err := json.Unmarshal(t.Payload(), &p); err != nil {
        return fmt.Errorf("unmarshal refresh payload: %w", err)
    }
    ID, err := pgtype.UUIDFromString(p.ID)
    if err != nil {
        return fmt.Errorf("invalid tenant_id in refresh payload: %w", err)
    }
    contentID, err := pgtype.UUIDFromString(p.ContentID)
    if err != nil {
        return fmt.Errorf("invalid content_id in refresh payload: %w", err)
    }
    jobID, err := pgtype.UUIDFromString(p.IngestionJobID)
    if err != nil {
        return fmt.Errorf("invalid ingestion_job_id in refresh payload: %w", err)
    }
    var userID pgtype.UUID
    if p.RequestedBy != "" {
        userID, err = pgtype.UUIDFromString(p.RequestedBy)
        if err != nil {
            return fmt.Errorf("invalid requested_by_user_id in refresh payload: %w", err)
        }
    }
    return jceIngestionSvc.ProcessRefreshValues(ctx, ID, contentID, jobID, p.SourceKind, userID)
}
```

In `internal/worker/tasks.go`, add `mux.HandleFunc(TypeJCERefreshValues, HandleJceRefreshValues)` inside `RegisterHandlers`.

In `internal/server/server.go`, after constructing `jceService`, call `worker.RegisterJCEService(jceService)` before `worker.NewMux()`.

- [ ] **Step 6: Add scheduled refresh job.** Add `ScheduleStaleRefresh(ctx context.Context, sweepHours int) error` in `internal/jce/ingestion.go`. The function queries `comparison_data_ingestion_jobs` for each content whose latest completed job is older than `sweepHours` (or has no completed job) and is not already pending/running. For each found content, call `RefreshComparison` with `asynq.Queue("low")`. Wire this function into the existing daily cron scheduler or create a low-tier periodic task `jce:refresh_stale` if the scheduler uses Asynq.
- [ ] **Step 7: Write tests** covering:
    - lock acquisition and deduplication,
    - job lifecycle pending→running→completed/failed,
    - missing and stale values get refreshed,
    - worker payload JSON round-trip.

**Verification:**
```bash
go test ./internal/jce/... ./internal/worker/... -run TestIngestion -v
go vet ./...
```
Expected: all PASS.

**Commit:**
```bash
git add internal/jce/ingestion.go internal/jce/ingestion_test.go internal/jce/service.go internal/worker/tasks.go internal/worker/handlers.go
git commit -m "feat(jce): add refresh ingestion jobs and worker handler"
```

---

## Task 6: AI advisor service

**Files:**
- Create: `internal/jce/advisor.go`
- Modify: `internal/jce/service.go` (add `advisor` dispatcher field and methods)
- Test: `internal/jce/advisor_test.go`

**Interfaces:**

```go
type Advisor interface {
    Analyze(ctx context.Context, contentID, ID, userID, profileID pgtype.UUID, question string) (AdvisorResult, error)
    Rank(ctx context.Context, contentID, ID, userID, profileID pgtype.UUID) (AdvisorResult, error)
    Explain(ctx context.Context, contentID, itemID, ID, userID, profileID pgtype.UUID, factorKey, question string) (AdvisorResult, error)
    Recommend(ctx context.Context, contentID, ID, userID, profileID pgtype.UUID) (AdvisorResult, error)
    StreamAdvisor(ctx context.Context, contentID, ID, userID, profileID pgtype.UUID, question string, w http.ResponseWriter) error
}

type AIDispatcher interface {
    Dispatch(ctx context.Context, category string, req ai.Request) (ai.Response, error)
}
```

**Steps:**

- [ ] **Step 1: Implement prompt builder `buildAdvisorPrompt(ctx context.Context, input AdvisorInput, matrix []comparison.ComparisonMatrixItem, scoreResult ScoreResult, recommendations []ComparisonRecommendation, freshness FreshnessSummary) (string, string)`**. The first return is the user prompt; the second is the optional system/context string. Collect:
    - Comparison title, items, and dimensions from `comparison.Service.GetJCEComparisonMatrix` (returns `[]comparison.ComparisonMatrixItem`).
    - `ScoreResult` from `jce.Service.ScoreComparison`.
    - `[]ComparisonRecommendation` from `jce.Service.ListRecommendations`.
    - `FreshnessSummary` from `Service.FreshnessForComparison`.
- [ ] **Step 2: Implement `Analyze`, `Rank`, `Explain`, `Recommend`** as methods on `Service`.
    - Each method loads settings `jce.advisor_model` and `jce.advisor_max_tokens`.
    - Builds a single `prompt` string and optional `context` string.
    - Calls `s.dispatcher.Dispatch(ctx, "chat", ai.Request{Category: "chat", Payload: map[string]any{"model": model, "prompt": prompt, "context": context, "max_tokens": maxTokens}})`.
    - Extract the response text with a helper `extractResponseText(resp ai.Response) (string, error)` that checks, in order, `resp.Raw["text"]`, `resp.Raw["reply"]`, `resp.Raw["generated_text"]`, and the first choice in `resp.Raw["choices"]` (`.message.content` or `.text`).
    - Try to unmarshal the text as JSON into `AdvisorResult`; if that fails, wrap the raw text in `AdvisorResult.Summary` and fall back to a manually built result from the precomputed score/recommendation data.
    - Cache the result in Valkey:

```go
b, _ := json.Marshal(result)
_ = s.vk.Set(ctx, fmt.Sprintf("jc:jce:advisor:%s:%s:%s:%x", ID, contentID, profileID, sha256.Sum256([]byte(question))), b, time.Duration(ttlMinutes)*time.Minute)
```

Use a cache hit at the start of `Analyze` when `force` is false.
- [ ] **Step 3: Implement `StreamAdvisor`**.
    - Set headers: `Content-Type: text/event-stream; Cache-Control: no-cache; Connection: keep-alive`.
    - Call `Service.Analyze` to generate the answer.
    - Emit one final event: `data: {"type":"message","content":"..."}\n\n`.
    - Flush with `http.Flusher` and return.
- [ ] **Step 4: Write tests** covering:
    - prompt includes JapaScore and ranking,
    - dispatcher payload uses `prompt`/`context`/`max_tokens`, not `messages`,
    - result JSON has `summary`, `ranking`, `next_actions`, `citations`, `freshness_score`,
    - SSE output starts with `data:` and contains valid JSON.

**Verification:**
```bash
go test ./internal/jce/... -run TestAdvisor -v
go vet ./...
```
Expected: all PASS.

**Commit:**
```bash
git add internal/jce/advisor.go internal/jce/advisor_test.go internal/jce/service.go
git commit -m "feat(jce): add AI advisor service"
```

---

## Task 7: HTTP handlers and routes

**Files:**
- Modify: `internal/jce/handlers.go`
- Modify: `internal/server/server.go`
- Test: `internal/jce/handlers_test.go`

**Interfaces (extend `service` in `internal/jce/handlers.go`):**

```go
type service interface {
    // existing Sub-spec 2/3 methods already declared in this file

    // Data sources
    ListDataSources(ctx context.Context, ID pgtype.UUID, entityType *string, entityID *pgtype.UUID) ([]DataSource, error)
    CreateDataSource(ctx context.Context, ID pgtype.UUID, in DataSourceInput) (DataSource, error)
    GetDataSource(ctx context.Context, id, ID pgtype.UUID) (DataSource, error)
    UpdateDataSource(ctx context.Context, id, ID pgtype.UUID, in DataSourceInput) (DataSource, error)
    DeleteDataSource(ctx context.Context, id, ID pgtype.UUID) error

    // Freshness and refresh
    FreshnessForComparison(ctx context.Context, contentID, ID, userID pgtype.UUID) (FreshnessSummary, error)
    RefreshComparison(ctx context.Context, contentID, ID, userID pgtype.UUID, sourceKind *string) (IngestionJob, error)
    ListIngestionJobs(ctx context.Context, contentID, ID pgtype.UUID, limit, offset int) ([]IngestionJob, error)

    // Advisor
    Analyze(ctx context.Context, contentID, ID, userID, profileID pgtype.UUID, question string) (AdvisorResult, error)
    Rank(ctx context.Context, contentID, ID, userID, profileID pgtype.UUID) (AdvisorResult, error)
    Explain(ctx context.Context, contentID, itemID, ID, userID, profileID pgtype.UUID, factorKey, question string) (AdvisorResult, error)
    Recommend(ctx context.Context, contentID, ID, userID, profileID pgtype.UUID) (AdvisorResult, error)
    StreamAdvisor(ctx context.Context, contentID, ID, userID, profileID pgtype.UUID, question string, w http.ResponseWriter) error
}
```

**Steps:**

- [ ] **Step 1: Define request/response JSON structs** in `internal/jce/handlers.go`:

```go
type dataSourceListQuery struct {
    EntityType string `json:"entity_type"`
    EntityID   string `json:"entity_id"`
}

type dataSourceCreateBody struct {
    EntityType    *string        `json:"entity_type,omitempty"`
    EntityID      *string        `json:"entity_id,omitempty"`
    DimensionSlug *string        `json:"dimension_slug,omitempty"`
    SourceName    string         `json:"source_name"`
    SourceURL     *string        `json:"source_url,omitempty"`
    SourceKind    string         `json:"source_kind"`
    TrustLevel    int            `json:"trust_level"`
    Config        map[string]any `json:"config"`
}

type refreshBody struct {
    SourceKind *string `json:"source_kind,omitempty"`
}

type advisorBody struct {
    ProfileID *string `json:"profile_id,omitempty"`
    Question  string  `json:"question"`
    ItemID    *string `json:"item_id,omitempty"`
    FactorKey *string `json:"factor_key,omitempty"`
}
```

- [ ] **Step 2: Append handler methods to `internal/jce/handlers.go`**. Each method:
    - Calls `requireTenant(w, r)` and `requireUser(w, r)`.
    - Parses UUIDs with `parseUUIDParam` and optional UUIDs with `optionalUUIDPtr`.
    - Decodes the body with `decodeBody(w, r, &body)`.
    - Calls the service method and translates errors with `handleServiceError(w, err)`.
    - Writes success with `errpkg.WriteJSON(w, http.StatusX, map[string]any{"data": value})`.

Example handler skeletons:

```go
func (h *Handlers) handleListDataSources(w http.ResponseWriter, r *http.Request) {
    ID, ok := requireTenant(w, r)
    if !ok { return }

    q := r.URL.Query()
    entityType := q.Get("entity_type")
    var et *string
    if entityType != "" { et = &entityType }

    var entityID *pgtype.UUID
    if s := q.Get("entity_id"); s != "" {
        id, err := optionalUUIDPtr(s)
        if err != nil {
            errpkg.WriteError(w, http.StatusBadRequest, "invalid_entity_id", err.Error())
            return
        }
        entityID = id
    }

    sources, err := h.service.ListDataSources(r.Context(), ID, et, entityID)
    if err != nil {
        handleServiceError(w, err)
        return
    }
    errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": sources})
}

func (h *Handlers) handleCreateDataSource(w http.ResponseWriter, r *http.Request) {
    ID, ok := requireTenant(w, r)
    if !ok { return }

    var body dataSourceCreateBody
    if !decodeBody(w, r, &body) { return }

    in := DataSourceInput{
        EntityType:    body.EntityType,
        EntityID:      body.EntityID,
        DimensionSlug: body.DimensionSlug,
        SourceName:    body.SourceName,
        SourceURL:     body.SourceURL,
        SourceKind:    body.SourceKind,
        TrustLevel:    body.TrustLevel,
        Config:        body.Config,
    }
    if body.EntityID != nil {
        if _, err := optionalUUIDPtr(*body.EntityID); err != nil {
            errpkg.WriteError(w, http.StatusBadRequest, "invalid_entity_id", err.Error())
            return
        }
    }

    ds, err := h.service.CreateDataSource(r.Context(), ID, in)
    if err != nil {
        handleServiceError(w, err)
        return
    }
    errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": ds})
}

func (h *Handlers) handleGetDataSource(w http.ResponseWriter, r *http.Request) {
    ID, ok := requireTenant(w, r)
    if !ok { return }
    id, err := parseUUIDParam(r, "id")
    if err != nil {
        errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", err.Error())
        return
    }
    ds, err := h.service.GetDataSource(r.Context(), id, ID)
    if err != nil {
        handleServiceError(w, err)
        return
    }
    errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": ds})
}

func (h *Handlers) handleUpdateDataSource(w http.ResponseWriter, r *http.Request) {
    ID, ok := requireTenant(w, r)
    if !ok { return }
    id, err := parseUUIDParam(r, "id")
    if err != nil {
        errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", err.Error())
        return
    }
    var body dataSourceCreateBody
    if !decodeBody(w, r, &body) { return }

    in := DataSourceInput{
        EntityType:    body.EntityType,
        EntityID:      body.EntityID,
        DimensionSlug: body.DimensionSlug,
        SourceName:    body.SourceName,
        SourceURL:     body.SourceURL,
        SourceKind:    body.SourceKind,
        TrustLevel:    body.TrustLevel,
        Config:        body.Config,
    }
    if body.EntityID != nil {
        if _, err := optionalUUIDPtr(*body.EntityID); err != nil {
            errpkg.WriteError(w, http.StatusBadRequest, "invalid_entity_id", err.Error())
            return
        }
    }

    ds, err := h.service.UpdateDataSource(r.Context(), id, ID, in)
    if err != nil {
        handleServiceError(w, err)
        return
    }
    errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": ds})
}

func (h *Handlers) handleDeleteDataSource(w http.ResponseWriter, r *http.Request) {
    ID, ok := requireTenant(w, r)
    if !ok { return }
    id, err := parseUUIDParam(r, "id")
    if err != nil {
        errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", err.Error())
        return
    }
    if err := h.service.DeleteDataSource(r.Context(), id, ID); err != nil {
        handleServiceError(w, err)
        return
    }
    errpkg.WriteJSON(w, http.StatusNoContent, nil)
}

func (h *Handlers) handleGetFreshness(w http.ResponseWriter, r *http.Request) {
    ID, userID, ok := h.requireTenantAndUser(w, r)
    if !ok { return }
    contentID, err := parseUUIDParam(r, "id")
    if err != nil {
        errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", err.Error())
        return
    }
    summary, err := h.service.FreshnessForComparison(r.Context(), contentID, ID, userID)
    if err != nil {
        handleServiceError(w, err)
        return
    }
    errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": summary})
}

func (h *Handlers) handleRefreshComparison(w http.ResponseWriter, r *http.Request) {
    ID, userID, ok := h.requireTenantAndUser(w, r)
    if !ok { return }
    contentID, err := parseUUIDParam(r, "id")
    if err != nil {
        errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", err.Error())
        return
    }
    var body refreshBody
    if !decodeBody(w, r, &body) { return }
    job, err := h.service.RefreshComparison(r.Context(), contentID, ID, userID, body.SourceKind)
    if err != nil {
        handleServiceError(w, err)
        return
    }
    errpkg.WriteJSON(w, http.StatusAccepted, map[string]any{"data": job})
}

func (h *Handlers) handleListIngestionJobs(w http.ResponseWriter, r *http.Request) {
    ID, userID, ok := h.requireTenantAndUser(w, r)
    if !ok { return }
    contentID, err := parseUUIDParam(r, "id")
    if err != nil {
        errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", err.Error())
        return
    }
    limit := parseLimit(r, 20)
    offset := 0
    if s := r.URL.Query().Get("offset"); s != "" {
        if n, err := strconv.Atoi(s); err == nil && n >= 0 { offset = n }
    }
    jobs, err := h.service.ListIngestionJobs(r.Context(), contentID, ID, limit, offset)
    if err != nil {
        handleServiceError(w, err)
        return
    }
    errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": jobs})
}

func (h *Handlers) handleAnalyze(w http.ResponseWriter, r *http.Request) {
    ID, userID, ok := h.requireTenantAndUser(w, r)
    if !ok { return }
    contentID, err := parseUUIDParam(r, "id")
    if err != nil {
        errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", err.Error())
        return
    }
    var body advisorBody
    if !decodeBody(w, r, &body) { return }
    var profileID *pgtype.UUID
    if body.ProfileID != nil { profileID, _ = optionalUUIDPtr(*body.ProfileID) }
    res, err := h.service.Analyze(r.Context(), contentID, ID, userID, profileID, body.Question)
    if err != nil {
        handleServiceError(w, err)
        return
    }
    errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": res})
}

func (h *Handlers) handleRank(w http.ResponseWriter, r *http.Request) {
    ID, userID, ok := h.requireTenantAndUser(w, r)
    if !ok { return }
    contentID, err := parseUUIDParam(r, "id")
    if err != nil {
        errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", err.Error())
        return
    }
    var body advisorBody
    if !decodeBody(w, r, &body) { return }
    var profileID *pgtype.UUID
    if body.ProfileID != nil { profileID, _ = optionalUUIDPtr(*body.ProfileID) }
    res, err := h.service.Rank(r.Context(), contentID, ID, userID, profileID)
    if err != nil {
        handleServiceError(w, err)
        return
    }
    errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": res})
}

func (h *Handlers) handleExplain(w http.ResponseWriter, r *http.Request) {
    ID, userID, ok := h.requireTenantAndUser(w, r)
    if !ok { return }
    contentID, err := parseUUIDParam(r, "id")
    if err != nil {
        errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", err.Error())
        return
    }
    var body advisorBody
    if !decodeBody(w, r, &body) { return }
    var profileID *pgtype.UUID
    if body.ProfileID != nil { profileID, _ = optionalUUIDPtr(*body.ProfileID) }
    var itemID *pgtype.UUID
    if body.ItemID != nil { itemID, _ = optionalUUIDPtr(*body.ItemID) }
    factorKey := ""
    if body.FactorKey != nil { factorKey = *body.FactorKey }
    res, err := h.service.Explain(r.Context(), contentID, itemID, ID, userID, profileID, factorKey, body.Question)
    if err != nil {
        handleServiceError(w, err)
        return
    }
    errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": res})
}

func (h *Handlers) handleRecommend(w http.ResponseWriter, r *http.Request) {
    ID, userID, ok := h.requireTenantAndUser(w, r)
    if !ok { return }
    contentID, err := parseUUIDParam(r, "id")
    if err != nil {
        errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", err.Error())
        return
    }
    var body advisorBody
    if !decodeBody(w, r, &body) { return }
    var profileID *pgtype.UUID
    if body.ProfileID != nil { profileID, _ = optionalUUIDPtr(*body.ProfileID) }
    res, err := h.service.Recommend(r.Context(), contentID, ID, userID, profileID)
    if err != nil {
        handleServiceError(w, err)
        return
    }
    errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": res})
}

func (h *Handlers) handleStreamAdvisor(w http.ResponseWriter, r *http.Request) {
    ID, userID, ok := h.requireTenantAndUser(w, r)
    if !ok { return }
    contentID, err := parseUUIDParam(r, "id")
    if err != nil {
        errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", err.Error())
        return
    }
    var body advisorBody
    if !decodeBody(w, r, &body) { return }
    if strings.TrimSpace(body.Question) == "" {
        errpkg.WriteError(w, http.StatusBadRequest, "missing_question", "question is required")
        return
    }
    var profileID *pgtype.UUID
    if body.ProfileID != nil { profileID, _ = optionalUUIDPtr(*body.ProfileID) }
    if err := h.service.StreamAdvisor(r.Context(), contentID, ID, userID, profileID, body.Question, w); err != nil {
        handleServiceError(w, err)
    }
}
```

Add a small helper for handlers that need both tenant and user:

```go
func (h *Handlers) requireTenantAndUser(w http.ResponseWriter, r *http.Request) (pgtype.UUID, *auth.AuthenticatedUser, bool) {
    ID, ok := requireTenant(w, r)
    if !ok { return pgtype.UUID{}, nil, false }
    user, ok := requireUser(w, r)
    if !ok { return pgtype.UUID{}, nil, false }
    return ID, user, true
}
```

- [ ] **Step 3: Register new routes in `internal/server/server.go`**. Inside the existing JCE route group (after the Sub-spec 2/3 score/recommendation routes), add:

```go
// Sub-spec 4: data sources
r.With(auth.RequireAuth(kr, vkClient.Redis()), features.Gate(featureChecker, "feature.jce.data_sources"), rbac.RequirePermission(rbacResolver, "jce.data_source.view")).
    Get("/jce/data-sources", jceHandlers.handleListDataSources)
r.With(auth.RequireAuth(kr, vkClient.Redis()), features.Gate(featureChecker, "feature.jce.data_sources"), rbac.RequirePermission(rbacResolver, "jce.data_source.manage")).
    Post("/jce/data-sources", jceHandlers.handleCreateDataSource)
r.With(auth.RequireAuth(kr, vkClient.Redis()), features.Gate(featureChecker, "feature.jce.data_sources"), rbac.RequirePermission(rbacResolver, "jce.data_source.view")).
    Get("/jce/data-sources/{id}", jceHandlers.handleGetDataSource)
r.With(auth.RequireAuth(kr, vkClient.Redis()), features.Gate(featureChecker, "feature.jce.data_sources"), rbac.RequirePermission(rbacResolver, "jce.data_source.manage")).
    Patch("/jce/data-sources/{id}", jceHandlers.handleUpdateDataSource)
r.With(auth.RequireAuth(kr, vkClient.Redis()), features.Gate(featureChecker, "feature.jce.data_sources"), rbac.RequirePermission(rbacResolver, "jce.data_source.manage")).
    Delete("/jce/data-sources/{id}", jceHandlers.handleDeleteDataSource)

// Sub-spec 4: freshness, refresh, and ingestion jobs
r.With(auth.RequireAuth(kr, vkClient.Redis()), features.Gate(featureChecker, "feature.jce"), rbac.RequirePermission(rbacResolver, "jce.score.view")).
    Get("/jce/comparisons/{id}/freshness", jceHandlers.handleGetFreshness)
r.With(auth.RequireAuth(kr, vkClient.Redis()), features.Gate(featureChecker, "feature.jce.refresh"), rbac.RequirePermission(rbacResolver, "jce.refresh.trigger")).
    Post("/jce/comparisons/{id}/refresh", jceHandlers.handleRefreshComparison)
r.With(auth.RequireAuth(kr, vkClient.Redis()), features.Gate(featureChecker, "feature.jce.refresh"), rbac.RequirePermission(rbacResolver, "jce.ingestion_job.view")).
    Get("/jce/comparisons/{id}/ingestion-jobs", jceHandlers.handleListIngestionJobs)

// Sub-spec 4: AI advisor
r.With(auth.RequireAuth(kr, vkClient.Redis()), features.Gate(featureChecker, "feature.jce.ai_advisor"), rbac.RequirePermission(rbacResolver, "jce.ai_advisor.use")).
    Post("/jce/comparisons/{id}/analyze", jceHandlers.handleAnalyze)
r.With(auth.RequireAuth(kr, vkClient.Redis()), features.Gate(featureChecker, "feature.jce.ai_advisor"), rbac.RequirePermission(rbacResolver, "jce.ai_advisor.use")).
    Post("/jce/comparisons/{id}/rank", jceHandlers.handleRank)
r.With(auth.RequireAuth(kr, vkClient.Redis()), features.Gate(featureChecker, "feature.jce.ai_advisor"), rbac.RequirePermission(rbacResolver, "jce.ai_advisor.use")).
    Post("/jce/comparisons/{id}/explain", jceHandlers.handleExplain)
r.With(auth.RequireAuth(kr, vkClient.Redis()), features.Gate(featureChecker, "feature.jce.ai_advisor"), rbac.RequirePermission(rbacResolver, "jce.ai_advisor.use")).
    Post("/jce/comparisons/{id}/recommend", jceHandlers.handleRecommend)
r.With(auth.RequireAuth(kr, vkClient.Redis()), features.Gate(featureChecker, "feature.jce.ai_advisor"), rbac.RequirePermission(rbacResolver, "jce.ai_advisor.use")).
    Post("/jce/comparisons/{id}/advisor", jceHandlers.handleStreamAdvisor)
```

- [ ] **Step 4: Write HTTP tests** in `internal/jce/handlers_test.go` covering:
    - data-source CRUD and tenant isolation,
    - `GET /jce/comparisons/{id}/freshness` returns `200` and a `FreshnessSummary`,
    - `POST /jce/comparisons/{id}/refresh` returns `202` with an `IngestionJob`,
    - `POST /jce/comparisons/{id}/analyze` returns `200` with `summary` and `ranking`,
    - missing `feature.jce.ai_advisor` returns `403` `feature_disabled`,
    - missing `jce.data_source.manage` returns `403` `forbidden`,
    - `POST /jce/comparisons/{id}/advisor` streams `Content-Type: text/event-stream` and a `data:` line.

**Verification:**
```bash
go test ./internal/jce/... -run TestHandlers -v
go vet ./...
```
Expected: all PASS.

**Commit:**
```bash
git add internal/jce/handlers.go internal/jce/handlers_test.go internal/server/server.go
git commit -m "feat(jce): add Sub-spec 4 HTTP handlers and routes"
```

---

## Task 8: AI tool handlers and registration

**Files:**
- Create: `internal/ai/tools_jce.go`
- Create: `internal/jce/adapter.go`
- Modify: `internal/ai/tools_register.go`
- Modify: `internal/ai/tools_deps.go`
- Modify: `internal/server/server.go`
- Test: `internal/ai/tools_jce_test.go`

**Interfaces:**

`internal/ai/tools_deps.go`:

```go
type ToolDeps struct {
    // existing ToolDeps fields (DB, Valkey, settings, comparison, etc.) remain unchanged
    JCE JCEService
}

func (t *ToolDeps) WithJCE(svc JCEService) *ToolDeps {
    t.JCE = svc
    return t
}

type JCEService interface {
    Analyze(ctx context.Context, contentID, ID, userID, profileID pgtype.UUID, question string) (map[string]any, error)
    Rank(ctx context.Context, contentID, ID, userID, profileID pgtype.UUID) (map[string]any, error)
    Explain(ctx context.Context, contentID, itemID, ID, userID, profileID pgtype.UUID, factorKey, question string) (map[string]any, error)
    Recommend(ctx context.Context, contentID, ID, userID, profileID pgtype.UUID) (map[string]any, error)
    Refresh(ctx context.Context, contentID, ID, userID pgtype.UUID, sourceKind *string) (map[string]any, error)
}
```

`internal/jce/adapter.go`:

```go
package jce

import (
    "context"
    "encoding/json"

    "github.com/jackc/pgx/v5/pgtype"
    "github.com/jedafe/japa/internal/ai"
)

type ToolAdapter struct { svc *Service }

func NewToolAdapter(svc *Service) ai.JCEService { return &ToolAdapter{svc: svc} }

func (a *ToolAdapter) Analyze(ctx context.Context, contentID, ID, userID, profileID pgtype.UUID, question string) (map[string]any, error) {
    res, err := a.svc.Analyze(ctx, contentID, ID, userID, profileID, question)
    if err != nil { return nil, err }
    return toMap(res)
}

func (a *ToolAdapter) Rank(ctx context.Context, contentID, ID, userID, profileID pgtype.UUID) (map[string]any, error) {
    res, err := a.svc.Rank(ctx, contentID, ID, userID, profileID)
    if err != nil { return nil, err }
    return toMap(res)
}

func (a *ToolAdapter) Explain(ctx context.Context, contentID, itemID, ID, userID, profileID pgtype.UUID, factorKey, question string) (map[string]any, error) {
    res, err := a.svc.Explain(ctx, contentID, &itemID, ID, userID, profileID, factorKey, question)
    if err != nil { return nil, err }
    return toMap(res)
}

func (a *ToolAdapter) Recommend(ctx context.Context, contentID, ID, userID, profileID pgtype.UUID) (map[string]any, error) {
    res, err := a.svc.Recommend(ctx, contentID, ID, userID, profileID)
    if err != nil { return nil, err }
    return toMap(res)
}

func (a *ToolAdapter) Refresh(ctx context.Context, contentID, ID, userID pgtype.UUID, sourceKind *string) (map[string]any, error) {
    res, err := a.svc.RefreshComparison(ctx, contentID, ID, userID, sourceKind)
    if err != nil { return nil, err }
    return toMap(res)
}

func toMap(v any) (map[string]any, error) {
    b, err := json.Marshal(v)
    if err != nil { return nil, err }
    var m map[string]any
    if err := json.Unmarshal(b, &m); err != nil { return nil, err }
    return m, nil
}
```

**Steps:**

- [ ] **Step 1: Add `JCE` field and `WithJCE` setter to `internal/ai/tools_deps.go`.**
- [ ] **Step 2: Create `internal/jce/adapter.go`** with the `ToolAdapter` above.
- [ ] **Step 3: Implement tool handlers in `internal/ai/tools_jce.go`**. Each handler:
    - Decodes `args` into a `jceArgs` struct:

```go
type jceArgs struct {
    ContentID string  `json:"content_id"`
    ProfileID string  `json:"profile_id,omitempty"`
    ItemID    string  `json:"item_id,omitempty"`
    FactorKey string  `json:"factor_key,omitempty"`
    SourceKind string `json:"source_kind,omitempty"`
    Question  string  `json:"question"`
}
```

    - Validates and converts UUIDs with `pgtype.UUIDFromString`.
    - For `comparison.refresh`, ensure the user has the `jce.refresh.trigger` node by calling the provided permission checker on `deps`.
    - Calls the matching `deps.JCE` method and returns `successToolResult(result)` or `errorToolResult(code, message)`.

Example:

```go
func JCEAnalyze(deps *ai.ToolDeps) ai.ToolHandler {
    return func(ctx context.Context, args json.RawMessage, userID, ID pgtype.UUID) (*ai.ToolResult, error) {
        var a jceArgs
        if err := json.Unmarshal(args, &a); err != nil {
            return errorToolResult("invalid_args", err.Error()), nil
        }
        contentID, err := pgtype.UUIDFromString(a.ContentID)
        if err != nil {
            return errorToolResult("invalid_content_id", err.Error()), nil
        }
        var profileID pgtype.UUID
        if a.ProfileID != "" {
            p, err := pgtype.UUIDFromString(a.ProfileID)
            if err != nil {
                return errorToolResult("invalid_profile_id", err.Error()), nil
            }
            profileID = p
        }
        result, err := deps.JCE.Analyze(ctx, contentID, ID, userID, profileID, a.Question)
        if err != nil {
            return errorToolResult("advisor_failed", err.Error()), nil
        }
        return successToolResult(result), nil
    }
}
```

Implement `JCERank`, `JCEExplain`, `JCERecommend`, and `JCERefresh` similarly.

- [ ] **Step 4: Register privacy target extractors** in `internal/ai/tools_jce.go`. Each extractor returns `PrivacyTarget{EntityType: "content", EntityID: contentID}` and `BlockIfAIExempt: true`. Use the same pattern as existing comparison tools (`comparison.fill_values` etc.).
- [ ] **Step 5: Register handlers in `internal/ai/tools_register.go` under `RegisterPhase7ComparisonTools`**. Tool names must match the migration `000576` rows:
    - `comparison.analyze` → `JCEAnalyze`
    - `comparison.rank` → `JCERank`
    - `comparison.explain` → `JCEExplain`
    - `comparison.recommend_next_step` → `JCERecommend`
    - `comparison.refresh` → `JCERefresh`
- [ ] **Step 6: Wire `ToolDeps.JCE` in `internal/server/server.go`**. After `jceService` is constructed, build the agent dependencies:

```go
agentDeps := ai.NewToolDeps().WithJCE(jce.NewToolAdapter(jceService))
```

- [ ] **Step 7: Write tests** in `internal/ai/tools_jce_test.go` covering:
    - each tool handler decodes valid args and returns a ToolResult,
    - invalid UUIDs return `errorToolResult`,
    - `comparison.refresh` is gated by the `jce.refresh.trigger` permission,
    - privacy target extracts `content` entity and blocks `ai_exempt` content.

**Verification:**
```bash
go test ./internal/ai/... -run TestJCE -v
go vet ./...
```
Expected: all PASS.

**Commit:**
```bash
git add internal/ai/tools_jce.go internal/ai/tools_jce_test.go internal/ai/tools_register.go internal/ai/tools_deps.go internal/jce/adapter.go internal/server/server.go
git commit -m "feat(ai): register JCE advisor and refresh agent tools"
```

---

## Task 9: JEOS catalog updates

**Files:**
- Modify: `.jeos/sections/30-generic-comparison-system.md`
- Modify: `.jeos/sections/11-job-queue-asynq.md`
- Modify: `.jeos/conventions/api-routes-catalog.md`
- Modify: `.jeos/conventions/valkey-keys-catalog.md`
- Modify: `.jeos/conventions/system-settings-catalog.md`

**Steps:**

- [ ] **Step 1: Update `.jeos/sections/30-generic-comparison-system.md`**. Append a `JCE Sub-spec 4 Amendment` block with:

```markdown
## JCE Sub-spec 4 Amendment (2026-08-30)

### Schema additions

```sql
CREATE TABLE comparison_data_sources (
  id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id       uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  entity_type     text,
  entity_id       uuid,
  dimension_slug  text,
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

CREATE TABLE comparison_data_ingestion_jobs (
  id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id       uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  content_id      uuid NOT NULL REFERENCES contents(id) ON DELETE CASCADE,
  status          text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','running','completed','failed','cancelled')),
  source_kind     text,
  refreshed_count int NOT NULL DEFAULT 0,
  error           text,
  started_at      timestamptz,
  completed_at    timestamptz,
  created_at      timestamptz NOT NULL DEFAULT now(),
  updated_at      timestamptz NOT NULL DEFAULT now()
);

ALTER TABLE comparison_values
  ADD COLUMN updated_at timestamptz NOT NULL DEFAULT now(),
  ADD COLUMN data_source_id uuid REFERENCES comparison_data_sources(id) ON DELETE SET NULL;
```

### Feature flags

- `feature.jce.data_sources`
- `feature.jce.refresh`
- `feature.jce.ai_advisor`

### RBAC nodes

- `jce.data_source.view`
- `jce.data_source.manage`
- `jce.refresh.trigger`
- `jce.ingestion_job.view`
- `jce.ai_advisor.use`

### API routes

- `GET/POST /jce/data-sources`
- `GET/PATCH/DELETE /jce/data-sources/{id}`
- `GET /jce/comparisons/{id}/freshness`
- `POST /jce/comparisons/{id}/refresh`
- `GET /jce/comparisons/{id}/ingestion-jobs`
- `POST /jce/comparisons/{id}/analyze`
- `POST /jce/comparisons/{id}/rank`
- `POST /jce/comparisons/{id}/explain`
- `POST /jce/comparisons/{id}/recommend`
- `POST /jce/comparisons/{id}/advisor`

### AI tools

- `comparison.analyze`
- `comparison.rank`
- `comparison.explain`
- `comparison.recommend_next_step`
- `comparison.refresh`
```

- [ ] **Step 2: Update `.jeos/sections/11-job-queue-asynq.md`**. In the default-tier list, add:

```text
jce:refresh_values              — refresh comparison values from registered data sources (Sub-spec 4)
                                  Payload: { tenant_id, content_id, source_kind, ingestion_job_id, requested_by_user_id }
```

- [ ] **Step 3: Update `.jeos/conventions/api-routes-catalog.md`**. In the `## Japa Comparison Engine / Generic Comparison System` section, add:

```text
### JCE Sub-spec 4 (data sources, freshness, refresh, AI advisor)

All gated by `feature.comparison` + `feature.jce` plus the specific `feature.jce.*` flag.

GET    /jce/data-sources                       list comparison data sources
POST   /jce/data-sources                       create a data source
GET    /jce/data-sources/{id}                  view a data source
PATCH  /jce/data-sources/{id}                  update a data source
DELETE /jce/data-sources/{id}                  delete a data source
GET    /jce/comparisons/{id}/freshness         freshness summary
POST   /jce/comparisons/{id}/refresh           enqueue a refresh job
GET    /jce/comparisons/{id}/ingestion-jobs    list refresh jobs
POST   /jce/comparisons/{id}/analyze           ask the AI advisor
POST   /jce/comparisons/{id}/rank              ranked summary
POST   /jce/comparisons/{id}/explain           explain a single item/factor
POST   /jce/comparisons/{id}/recommend         next actions
POST   /jce/comparisons/{id}/advisor           generic advisor question
```

- [ ] **Step 4: Update `.jeos/conventions/valkey-keys-catalog.md`**. Append a `JCE Sub-spec 4` table:

```markdown
## JCE Sub-spec 4

| Key Pattern | Value | TTL | Invalidated by |
|---|---|---|---|
| `jc:jce:freshness:{tenant_id}:{content_id}` | JSON `FreshnessSummary` | `jce.freshness_cache_ttl_minutes` (default 10m) | value refresh, source mutation, score recalc |
| `jc:jce:advisor:{tenant_id}:{content_id}:{profile_id}:{question_hash}` | JSON `AdvisorResult` | `jce.advisor_cache_ttl_minutes` (default 15m) | score recalc, refresh, source mutation |
| `jc:jce:refresh_lock:{content_id}` | `"1"` | 5m | job completion or natural expiry |
```

- [ ] **Step 5: Update `.jeos/conventions/system-settings-catalog.md`**. Append a `JCE` section:

```markdown
## Japa Comparison Engine (JCE)

| Key | Default | Type | Description |
|---|---|---|---|
| `jce.freshness_stale_hours` | 168 | int | Hours before a value is considered stale |
| `jce.freshness_decay_per_hour` | 2 | int | Freshness score decay per hour after stale |
| `jce.freshness_official_weight` | 100 | int | Weight numerator for official sources |
| `jce.freshness_third_party_weight` | 75 | int | Weight numerator for third-party sources |
| `jce.freshness_ai_estimate_weight` | 50 | int | Weight numerator for AI-estimate sources |
| `jce.freshness_cache_ttl_minutes` | 10 | int | Freshness summary Valkey TTL in minutes |
| `jce.advisor_model` | "" | string | Default model for advisor; empty = dispatcher default |
| `jce.advisor_max_tokens` | 1024 | int | Max tokens for advisor synthesis |
| `jce.advisor_cache_ttl_minutes` | 15 | int | Advisor result Valkey TTL in minutes |
```

- [ ] **Step 6: Verification**. Run:

```bash
grep -E 'jce:refresh_values|jc:jce:freshness|jc:jce:advisor|jc:jce:refresh_lock|jce\.freshness_|jce\.advisor_' .jeos/sections/30-generic-comparison-system.md .jeos/sections/11-job-queue-asynq.md .jeos/conventions/api-routes-catalog.md .jeos/conventions/valkey-keys-catalog.md .jeos/conventions/system-settings-catalog.md
```
Expected: every new name appears in at least one catalog file.

**Commit:**
```bash
git add .jeos/sections/30-generic-comparison-system.md .jeos/sections/11-job-queue-asynq.md .jeos/conventions/api-routes-catalog.md .jeos/conventions/valkey-keys-catalog.md .jeos/conventions/system-settings-catalog.md
git commit -m "docs(jeos): catalog Sub-spec 4 routes, tasks, keys, and settings"
```

---

## Task 10: Server wiring and service integration

**Files:**
- Modify: `internal/server/server.go`
- Modify: `internal/jce/service.go`
- Modify: `internal/ai/tools_register.go`
- Test: `internal/server/server_test.go` (or extend `internal/jce/service_test.go`)

**Steps:**

- [ ] **Step 1: Extend `internal/jce/service.go`**. Add these fields (and any required constructor arguments) to the existing `Service` struct:

```go
type taskEnqueuer interface {
    Enqueue(taskType string, payload []byte, opts ...any) error
}

type Service struct {
    // existing fields: db, compSvc, r, settings, vk, features, profiles, factors, recs, engine
    dataSources *dataSourceStore
    ingestion   *ingestionStore
    dispatcher  AIDispatcher
    enqueuer    taskEnqueuer
}

func (s *Service) SetAIDispatcher(d AIDispatcher) { s.dispatcher = d }
func (s *Service) SetEnqueuer(e taskEnqueuer)     { s.enqueuer = e }
```

- [ ] **Step 2: Update `NewService`** in `internal/jce/service.go` to instantiate `dataSourceStore` and `ingestionStore` from `s.db.Reader`/`s.db.Writer`. Valkey and `*config.Settings` are already injected through the existing constructor; the Asynq enqueuer and AI dispatcher are set via `SetEnqueuer`/`SetAIDispatcher` from `server.go`.
- [ ] **Step 3: Update `internal/server/server.go`**.
    - Construct `jceService := jce.NewService(database, comparisonService, jceResolver, settingsService, vkClient.Redis())`.
    - Set the Asynq enqueuer: `jceService.SetEnqueuer(taskEnqueuer)`.
    - Set the AI dispatcher: `jceService.SetAIDispatcher(aiDispatcher)`.
    - Call `worker.RegisterJCEService(jceService)` before `worker.NewMux()` so `HandleJceRefreshValues` is wired.
    - Build `agentDeps := ai.NewToolDeps().WithJCE(jce.NewToolAdapter(jceService))`.
    - In `internal/server/server.go`, reuse the existing `jceHandlers := jce.NewHandlers(jceService)` and add the Sub-spec 4 route group from Task 7, Step 3.
- [ ] **Step 4: Write integration tests** that start the server or service in a test DB and call:
    - `POST /jce/data-sources`
    - `GET /jce/comparisons/{id}/freshness`
    - `POST /jce/comparisons/{id}/refresh`
    - `POST /jce/comparisons/{id}/analyze` (with a mock dispatcher)

**Verification:**
```bash
go test ./internal/jce/... ./internal/ai/... ./internal/worker/... -v
go vet ./...
```
Expected: all PASS.

**Commit:**
```bash
git add internal/jce/service.go internal/server/server.go internal/ai/tools_register.go
git commit -m "feat(server): wire JCE data intelligence and advisor dependencies"
```

---

## Task 11: JCE UI

**Files:**
- Modify: `apps/main/src/lib/types/jce.ts`
- Modify: `apps/main/src/lib/api/jce.ts`
- Create: `apps/main/src/lib/components/compare/FreshnessChip.svelte`
- Create: `apps/main/src/lib/components/compare/AIAdvisorDrawer.svelte`
- Create: `apps/main/src/lib/components/compare/RefreshButton.svelte`
- Modify: `apps/main/src/lib/i18n/en.json`
- Modify: `apps/main/src/lib/i18n/fr.json` (if present)
- Modify: `apps/main/src/routes/compare/[slug]/+page.svelte`
- Modify: `apps/main/src/routes/comparisons/[slug]/+page.svelte`
- Test: `apps/main/src/lib/components/compare/freshness-chip.test.ts`

**Steps:**

- [ ] **Step 1: Add TypeScript types** in `apps/main/src/lib/types/jce.ts`:

```ts
export interface DataSource {
  id: string;
  source_name: string;
  source_kind: 'official' | 'third_party' | 'ai_estimate';
  trust_level: number;
  freshness_score: number;
  source_url?: string | null;
  last_verified_at?: string | null;
}

export interface FreshnessCell {
  dimension_id: string;
  label: string;
  freshness_score: number;
  source_name?: string;
  source_kind?: string;
  retrieved_at?: string | null;
  is_stale: boolean;
}

export interface FreshnessItem {
  item_id: string;
  item_title: string;
  freshness_score: number;
  cells: FreshnessCell[];
}

export interface FreshnessSummary {
  content_id: string;
  overall_freshness_score: number;
  stale_count: number;
  total_count: number;
  by_item: FreshnessItem[];
}

export interface AdvisorRankItem {
  item_id: string;
  item_title: string;
  rank: number;
  japa_score: number;
  reasoning: string;
  eligible: boolean;
}

export interface NextAction {
  item_id?: string;
  text: string;
  action_type?: string;
  action_url?: string;
  priority: number;
}

export interface AdvisorResult {
  content_id: string;
  profile_id?: string | null;
  summary: string;
  ranking?: AdvisorRankItem[];
  next_actions?: NextAction[];
  citations?: string[];
  freshness_score: number;
}

export interface IngestionJob {
  id: string;
  content_id: string;
  status: 'pending' | 'running' | 'completed' | 'failed' | 'cancelled';
  source_kind?: string | null;
  refreshed_count: number;
  error?: string | null;
  created_at: string;
}
```

- [ ] **Step 2: Add API client functions** in `apps/main/src/lib/api/jce.ts`. Use the content UUID (from `data.comparison.content.id`) rather than the URL slug:

```ts
import { api } from '$lib/api/client';

export async function getFreshness(comparisonID: string): Promise<FreshnessSummary | null> {
  const res = await api.get<FreshnessSummary>(`/jce/comparisons/${comparisonID}/freshness`);
  return res.data ?? null;
}

export async function refreshComparison(comparisonID: string, sourceKind?: string): Promise<IngestionJob | null> {
  const res = await api.post<IngestionJob>(`/jce/comparisons/${comparisonID}/refresh`, sourceKind ? { source_kind: sourceKind } : {});
  return res.data ?? null;
}

export async function listIngestionJobs(comparisonID: string): Promise<IngestionJob[] | null> {
  const res = await api.get<IngestionJob[]>(`/jce/comparisons/${comparisonID}/ingestion-jobs`);
  return res.data ?? [];
}

export async function analyzeComparison(comparisonID: string, question: string, profileID?: string): Promise<AdvisorResult | null> {
  const res = await api.post<AdvisorResult>(`/jce/comparisons/${comparisonID}/analyze`, { question, profile_id: profileID });
  return res.data ?? null;
}

export async function rankComparison(comparisonID: string, profileID?: string): Promise<AdvisorResult | null> {
  const res = await api.post<AdvisorResult>(`/jce/comparisons/${comparisonID}/rank`, { profile_id: profileID });
  return res.data ?? null;
}

export async function askAdvisor(comparisonID: string, question: string, profileID?: string): Promise<AdvisorResult | null> {
  const res = await api.post<AdvisorResult>(`/jce/comparisons/${comparisonID}/advisor`, { question, profile_id: profileID });
  return res.data ?? null;
}
```

- [ ] **Step 3: Create `apps/main/src/lib/components/compare/FreshnessChip.svelte`**:

```svelte
<script lang="ts">
  import { t } from '$lib/i18n';
  import type { FreshnessCell } from '$lib/types/jce';

  let { cell }: { cell: FreshnessCell } = $props();

  const color = $derived.by(() => {
    if (cell.is_stale || cell.freshness_score < 40) return 'var(--color-error)';
    if (cell.freshness_score < 75) return 'var(--color-warning)';
    return 'var(--color-success)';
  });

  const label = $derived.by(() => {
    if (cell.is_stale) return t('jce.freshness.stale', 'Stale');
    if (cell.freshness_score >= 75) return t('jce.freshness.fresh', 'Fresh');
    return t('jce.freshness.aging', 'Aging');
  });
</script>

<span
  class="inline-flex items-center gap-1 px-2 py-0.5 text-xs rounded-full border"
  style="border-color: {color}; color: {color}; background-color: var(--color-surface)"
  title={cell.source_name ? t('jce.freshness.source', 'Source: {source}', { source: cell.source_name }) : ''}
>
  {label} {cell.freshness_score}
</span>
```

- [ ] **Step 4: Create `apps/main/src/lib/components/compare/RefreshButton.svelte`**:

```svelte
<script lang="ts">
  import { t } from '$lib/i18n';
  import { refreshComparison } from '$lib/api/jce';

  let { comparisonID }: { comparisonID: string } = $props();
  let refreshing = $state(false);
  let lastJob = $state<IngestionJob | null>(null);

  async function onClick() {
    if (refreshing) return;
    refreshing = true;
    lastJob = await refreshComparison(comparisonID);
    refreshing = false;
  }
</script>

{#if lastJob}
  <p class="text-xs" style="color: var(--color-muted)">
    {t('jce.refresh.status', 'Status: {status}', { status: lastJob.status })} {lastJob.refreshed_count}
  </p>
{/if}
<button
  class="px-3 py-1.5 text-sm rounded-md border"
  style="border-color: var(--color-border); background-color: var(--color-surface); color: var(--color-text)"
  on:click={onClick}
  disabled={refreshing}
>
  {refreshing ? t('jce.refresh.refreshing', 'Refreshing...') : t('jce.refresh.button', 'Refresh data')}
</button>
```

- [ ] **Step 5: Create `apps/main/src/lib/components/compare/AIAdvisorDrawer.svelte`**:

```svelte
<script lang="ts">
  import { t } from '$lib/i18n';
  import { analyzeComparison } from '$lib/api/jce';
  import type { AdvisorResult } from '$lib/types/jce';

  let { comparisonID, open = $bindable(false), profileID }: { comparisonID: string; open?: boolean; profileID?: string } = $props();
  let question = $state('Which is best for me?');
  let loading = $state(false);
  let result = $state<AdvisorResult | null>(null);

  async function submit() {
    loading = true;
    result = await analyzeComparison(comparisonID, question, profileID);
    loading = false;
  }
</script>

{#if open}
  <div class="fixed inset-y-0 right-0 w-full sm:w-[28rem] p-4 sm:p-6 overflow-y-auto border-l z-50" style="background-color: var(--color-bg); border-color: var(--color-border)">
    <div class="flex items-center justify-between mb-4">
      <h2 style="color: var(--color-text)">{t('jce.advisor.title', 'AI Advisor')}</h2>
      <button on:click={() => (open = false)} class="text-sm" style="color: var(--color-muted)">{t('common.close', 'Close')}</button>
    </div>
    <div class="space-y-3">
      <label class="block text-sm" style="color: var(--color-muted)">{t('jce.advisor.question', 'Question')}</label>
      <input bind:value={question} class="w-full px-3 py-2 text-sm rounded border" style="background-color: var(--color-surface); border-color: var(--color-border); color: var(--color-text)" />
      <button on:click={submit} disabled={loading} class="w-full px-3 py-2 text-sm rounded" style="background-color: var(--color-primary); color: white">
        {loading ? t('jce.advisor.thinking', 'Thinking...') : t('jce.advisor.ask', 'Ask')}
      </button>
    </div>
    {#if result}
      <div class="mt-6 space-y-4 text-sm" style="color: var(--color-text)">
        <p>{result.summary}</p>
        {#if result.ranking?.length}
          <ol class="list-decimal pl-4 space-y-1">
            {#each result.ranking as item}
              <li>{item.item_title} — {item.japa_score}</li>
            {/each}
          </ol>
        {/if}
        {#if result.next_actions?.length}
          <ul class="list-disc pl-4 space-y-1">
            {#each result.next_actions as action}
              <li>{action.text}</li>
            {/each}
          </ul>
        {/if}
      </div>
    {/if}
  </div>
{/if}
```

- [ ] **Step 6: Integrate into comparison pages**. In both `apps/main/src/routes/compare/[slug]/+page.svelte` and `apps/main/src/routes/comparisons/[slug]/+page.svelte`:

```svelte
<script lang="ts">
  // existing page imports and props remain unchanged
  import { getFreshness } from '$lib/api/jce';
  import FreshnessChip from '$lib/components/compare/FreshnessChip.svelte';
  import RefreshButton from '$lib/components/compare/RefreshButton.svelte';
  import AIAdvisorDrawer from '$lib/components/compare/AIAdvisorDrawer.svelte';

  let { data }: { data: PageData } = $props();
  let comparison = $derived(data.comparison);
  let freshness = $state<FreshnessSummary | null>(null);
  let advisorOpen = $state(false);

  $effect(() => {
    if (comparison?.content?.id) {
      getFreshness(comparison.content.id).then((f) => { freshness = f; });
    }
  });

  function findCell(itemId: string, dimId: string): FreshnessCell | undefined {
    const item = freshness?.by_item.find((i) => i.item_id === itemId);
    return item?.cells.find((c) => c.dimension_id === dimId);
  }
</script>

<!-- near the matrix header -->
<div class="flex items-center gap-2 mb-2">
  <RefreshButton comparisonID={comparison.content.id} />
  <button on:click={() => (advisorOpen = true)} class="px-3 py-1.5 text-sm rounded" style="background-color: var(--color-primary); color: white">
    {t('jce.advisor.open', 'Ask AI Advisor')}
  </button>
</div>

<!-- inside each value cell -->
{#if freshness}
  <FreshnessChip cell={findCell(item.id, dimension.id)} />
{/if}

<AIAdvisorDrawer bind:open={advisorOpen} comparisonID={comparison.content.id} />
```

- [ ] **Step 7: Add i18n keys** in `apps/main/src/lib/i18n/en.json`:

```json
{
  "jce.freshness.fresh": "Fresh",
  "jce.freshness.aging": "Aging",
  "jce.freshness.stale": "Stale",
  "jce.freshness.source": "Source: {source}",
  "jce.refresh.button": "Refresh data",
  "jce.refresh.refreshing": "Refreshing...",
  "jce.refresh.status": "Status: {status}",
  "jce.advisor.title": "AI Advisor",
  "jce.advisor.question": "Question",
  "jce.advisor.ask": "Ask",
  "jce.advisor.thinking": "Thinking...",
  "jce.advisor.open": "Ask AI Advisor"
}
```

Add the same keys to `apps/main/src/lib/i18n/fr.json` if the file exists.

- [ ] **Step 8: Add a component test** in `apps/main/src/lib/components/compare/freshness-chip.test.ts`:

```ts
import { describe, it, expect } from 'vitest';
import { render } from '@testing-library/svelte';
import FreshnessChip from './FreshnessChip.svelte';

describe('FreshnessChip', () => {
  it('renders the score and stale label', () => {
    const { getByText } = render(FreshnessChip, {
      props: { cell: { dimension_id: '1', label: 'Tuition', freshness_score: 88, source_name: 'Official', source_kind: 'official', is_stale: false } }
    });
    expect(getByText('88')).toBeTruthy();
  });
});
```

- [ ] **Step 9: Verification**:

```bash
cd apps/main
bun run check
bun run build
bun run test
```
Expected: no type or build errors; tests pass.

**Commit:**
```bash
git add apps/main/src/lib/types/jce.ts apps/main/src/lib/api/jce.ts apps/main/src/lib/components/compare apps/main/src/routes/compare apps/main/src/routes/comparisons apps/main/src/lib/i18n
git commit -m "feat(jce): freshness, refresh, and AI advisor UI"
```

---

## Task 12: Admin UI

**Files:**
- Create: `apps/admin/src/lib/types/jce.ts`
- Create: `apps/admin/src/lib/api/comparison-data-sources.ts`
- Create: `apps/admin/src/routes/comparison-data-sources/+page.svelte`
- Create: `apps/admin/src/routes/comparison-data-sources/+page.server.ts`
- Create: `apps/admin/src/routes/comparison-data-sources/page.test.ts`
- Create: `apps/admin/src/routes/comparison-ingestion-jobs/+page.svelte`
- Create: `apps/admin/src/routes/comparison-ingestion-jobs/page.test.ts`
- Modify: `apps/admin/src/lib/i18n/en.json`
- Modify: `apps/admin/src/lib/i18n/fr.json` (if present)

**Steps:**

- [ ] **Step 1: Add TypeScript types** in `apps/admin/src/lib/types/jce.ts`:

```ts
export interface DataSourceAdmin {
  id: string;
  tenant_id: string;
  entity_type?: string | null;
  entity_id?: string | null;
  dimension_slug?: string | null;
  source_name: string;
  source_url?: string | null;
  source_kind: 'official' | 'third_party' | 'ai_estimate';
  trust_level: number;
  freshness_score: number;
  last_verified_at?: string | null;
  config: Record<string, unknown>;
  created_at: string;
  updated_at: string;
}

export interface IngestionJobAdmin {
  id: string;
  tenant_id: string;
  content_id: string;
  status: 'pending' | 'running' | 'completed' | 'failed' | 'cancelled';
  source_kind?: string | null;
  refreshed_count: number;
  error?: string | null;
  started_at?: string | null;
  completed_at?: string | null;
  created_at: string;
  updated_at: string;
}
```

- [ ] **Step 2: Add admin API client functions** in `apps/admin/src/lib/api/comparison-data-sources.ts`:

```ts
import { api } from '$lib/api/client';

export async function listDataSources(): Promise<DataSourceAdmin[]> {
  const res = await api.get<DataSourceAdmin[]>('/jce/data-sources');
  return res.data ?? [];
}

export async function getDataSource(id: string): Promise<DataSourceAdmin | null> {
  const res = await api.get<DataSourceAdmin>(`/jce/data-sources/${id}`);
  return res.data ?? null;
}

export async function createDataSource(body: Record<string, any>): Promise<DataSourceAdmin | null> {
  const res = await api.post<DataSourceAdmin>('/jce/data-sources', body);
  return res.data ?? null;
}

export async function updateDataSource(id: string, body: Record<string, any>): Promise<DataSourceAdmin | null> {
  const res = await api.patch<DataSourceAdmin>(`/jce/data-sources/${id}`, body);
  return res.data ?? null;
}

export async function deleteDataSource(id: string): Promise<boolean> {
  const res = await api.delete<unknown>(`/jce/data-sources/${id}`);
  return res.ok;
}

export async function listIngestionJobs(contentID: string): Promise<IngestionJobAdmin[]> {
  const res = await api.get<IngestionJobAdmin[]>(`/jce/comparisons/${contentID}/ingestion-jobs`);
  return res.data ?? [];
}
```

- [ ] **Step 3: Create `apps/admin/src/routes/comparison-data-sources/+page.svelte`**. Use the shared `EntityPage`/`DataTable` scaffold and `SlideOver` for create/edit. Render the fields from the design spec with i18n keys. Show the `source_kind` select, `trust_level` 1–5, and `source_url` URL input. Hook server actions to the API client from Step 2.

- [ ] **Step 4: Create `apps/admin/src/routes/comparison-data-sources/+page.server.ts`**. `load()` calls `listDataSources()` and returns `{ sources }`. Actions `create`, `update`, and `delete` call the matching client functions. After a mutating action, use `SvelteKit invalidation` to refetch the list.

- [ ] **Step 5: Create `apps/admin/src/routes/comparison-ingestion-jobs/+page.svelte`** using the same `EntityPage` pattern. Accept `content_id` from URL query params. Load jobs with `listIngestionJobs(contentID)`. Display status, source kind, refreshed count, and error. No creation action; this page is read-only.

- [ ] **Step 6: Add i18n keys** in `apps/admin/src/lib/i18n/en.json`:

```json
{
  "data_sources.title": "Comparison data sources",
  "data_sources.source_name": "Source name",
  "data_sources.source_kind": "Kind",
  "data_sources.trust_level": "Trust level",
  "data_sources.freshness_score": "Freshness",
  "data_sources.entity_type": "Entity type",
  "data_sources.dimension_slug": "Dimension slug",
  "data_sources.source_url": "Source URL",
  "data_sources.official": "Official",
  "data_sources.third_party": "Third party",
  "data_sources.ai_estimate": "AI estimate",
  "ingestion_jobs.title": "Comparison refresh jobs",
  "ingestion_jobs.content_id": "Comparison",
  "ingestion_jobs.status": "Status",
  "ingestion_jobs.source_kind": "Source kind",
  "ingestion_jobs.refreshed": "Refreshed"
}
```

Add the same keys to `apps/admin/src/lib/i18n/fr.json` if present.

- [ ] **Step 7: Add page tests** for both admin pages. Mock `$lib/api/comparison-data-sources.ts` and `$app/state`. Verify the list headings and that actions are disabled when the user lacks `jce.data_source.manage`.

- [ ] **Step 8: Verification**:

```bash
cd apps/admin
bun run check
bun run build
bun run test
```
Expected: no type or build errors; tests pass.

**Commit:**
```bash
git add apps/admin/src/lib/types/jce.ts apps/admin/src/lib/api/comparison-data-sources.ts apps/admin/src/routes/comparison-data-sources apps/admin/src/routes/comparison-ingestion-jobs apps/admin/src/lib/i18n
git commit -m "feat(admin): data sources and ingestion job UI"
```

---

## Task 13: Verification and smoke test

**Files:**
- All Sub-spec 4 backend and frontend files.

**Automated verification:**

- [ ] **Step 1: Run `make sqlc-gen`** and confirm `internal/db/gen` has no unstaged changes (if it does, they are from the SQL edits and should be committed).
- [ ] **Step 2: Run backend tests**:

```bash
go test ./internal/jce/... ./internal/ai/... ./internal/worker/... -count=1
go vet ./...
```

Expected: PASS, no vet failures.

- [ ] **Step 3: Run frontend checks**:

```bash
cd apps/main && bun run check && bun run build
cd ../admin && bun run check && bun run build
```

Expected: PASS.

**Manual smoke test:**

- [ ] **Step 4: Enable feature flags** `feature.jce.data_sources`, `feature.jce.refresh`, `feature.jce.ai_advisor` for the default tenant.
- [ ] **Step 5: Register a data source** through admin (`POST /jce/data-sources`).
- [ ] **Step 6: Create a comparison**, fill a value, and call `GET /jce/comparisons/{id}/freshness`. Verify `overall_freshness_score` is a non-zero integer and `by_item` contains cell details.
- [ ] **Step 7: Call `POST /jce/comparisons/{id}/refresh`**. Verify it returns `ingestion_job_id` and `status: "pending"`. Confirm the worker processes `jce:refresh_values` and the job becomes `completed`.
- [ ] **Step 8: Call `POST /jce/comparisons/{id}/analyze`** with `question: "Which is best for me?"`. Verify the response contains `summary`, a non-empty `ranking`, and `next_actions`.
- [ ] **Step 9: Call `POST /jce/comparisons/{id}/advisor`** with `Accept: text/event-stream`. Verify the response headers and one `data:` SSE event.

**Commit final verification fixes (if any):**

```bash
git add -A
git commit -m "fix(jce): Sub-spec 4 verification and smoke-test fixes"
```

---

## Self-review checklist

Before marking this plan complete, the implementer must verify:

- [ ] All six migrations (`000573`–`000578`) have paired `.down.sql` files and are reversible.
- [ ] `comparison_values.updated_at` backfill is present and all new columns are indexed.
- [ ] sqlc query files are updated and `make sqlc-gen` is run; `internal/db/gen` is not hand-edited.
- [ ] No placeholder text remains (no `TBD`, `TODO`, `implement later`, or `...` in code).
- [ ] Every new route, Valkey key, Asynq task, permission node, feature flag, and system setting is catalogued in JEOS.
- [ ] AI dispatcher payload uses `prompt`, `context`, and `max_tokens`, not `messages`.
- [ ] The `/advisor` endpoint emits SSE with a single final `message` event (no token streaming, no multi-turn loop).
- [ ] The advisor does not compute its own ranking; it calls the existing `ScoreComparison` engine.
- [ ] Privacy target extractors return `entity_type="content"` and block `ai_exempt` content.
- [ ] `bun` is used for all frontend commands; no `npm/npx/pnpm/yarn`.
- [ ] No hardcoded option lists, mock data, or fallback values in frontend.
- [ ] Commit messages are descriptive; no AI-generated attribution in commits or files.
- [ ] `go test`, `go vet ./...`, `bun run check`, and `bun run build` all pass.

---

## Execution options

1. **Subagent-driven execution (recommended).** Hand this plan to `superpowers:subagent-driven-development` and have a subagent work task-by-task. Provide the plan path and require checkbox updates after each commit.

2. **Inline execution with `executing-plans`.** Use the `executing-plans` skill to run the plan in the current session. Mark tasks `in_progress` in `todo_write` and complete them sequentially.

In either mode, do **not** begin implementing until the user confirms which execution approach to use. The design spec is approved and the plan is ready.
