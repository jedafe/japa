# Japa Comparison Engine (JCE) — Sub-spec 2/3 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` (recommended) or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Spec:** `docs/superpowers/specs/2026-08-25-jce-subspec-2-design.md`

**Goal:** Implement JCE Sub-spec 2/3 — user comparison profiles, score factors, JapaScore, eligibility, recommendations, Smart Compare, and the admin score-factor editor — on top of the existing `apps/main` and `/api/v1/jce/*` foundation.

**Architecture:** Keep `internal/comparison` as the generic matrix/CRUD engine. Add a new `internal/jce` package for profiles, score factors, scoring, recommendations, and their `/api/v1/jce/*` handlers. Persist per-factor scores in `comparison_scores`, recommendations in `comparison_recommendations`, and the active profile/weight snapshot in `comparison_content_metadata.config`. Cache score results and profiles in Valkey. Extend every `item_ref` resolver to expose `score_inputs` in `ResolvedItem.Metadata`.

**Tech Stack:** Go 1.24+ (chi, pgx, sqlc), SvelteKit 2 + Svelte 5 Runes + Bits UI + Tailwind CSS v4, Bun, Valkey, PostgreSQL 18.

## Global Constraints

- All work targets the `main` branch; no feature branches without explicit instruction.
- Follow `AGENTS.md`, `JEOS` §30, §03, §22, §23, and the `writing-plans` skill.
- Use `bun` for all JS/TS tooling; never use `npm`, `pnpm`, or `yarn`.
- All UI text must go through `t()` and be added to the appropriate `en.json` (and `fr.json` if present).
- No hardcoded data in SvelteKit `load()` or components; all data comes from `/api/v1`.
- Update `.jeos/conventions/api-routes-catalog.md`, `.jeos/conventions/valkey-keys-catalog.md`, `.jeos/conventions/system-settings-catalog.md`, `.jeos/conventions/permission-key-naming.md`, and `.jeos/sections/30-generic-comparison-system.md` before using new keys/routes/permissions.
- Do not edit `internal/db/gen/` by hand; run `make sqlc-gen` after query changes.
- All SQL migrations go in `migrations/0005xx_*.up.sql` / `*.down.sql`; the next available number is `000551`.
- Use `pgtype.UUID` in Go and `uuid` in SQL for all IDs.
- Soft-delete user content only; do not `DELETE FROM` user data in application code.
- Test-driven approach: write backend tests (`*_test.go`) and frontend client tests (`*.test.ts`) for every exported function/client.
- Verification pipeline: `make sqlc-gen`, `go build ./...`, `make vet`, `go test ./internal/jce/...`, `bun run check` in `apps/main` and `apps/admin`, `bun run build` in both apps, `make test-integration` (or `go test -tags integration ./...`).

---

## Implementation Status

| # | Area | Status |
|---|---|---|
| 0 | Migrations `000551`–`000555` | `PLANNED` |
| 1 | sqlc queries | `PLANNED` |
| 2 | Go models + stores (`profiles`, `score-factors`) | `PLANNED` |
| 3 | Scoring engine | `PLANNED` |
| 4 | JCE service orchestration | `PLANNED` |
| 5 | Handlers + routes | `PLANNED` |
| 6 | Resolver `score_inputs` enrichment | `PLANNED` |
| 7 | `apps/main` profile API client + types | `PLANNED` |
| 8 | `apps/main` `/compare` profile drawer + smart compare | `PLANNED` |
| 9 | `apps/main` `/compare/[slug]` score/recommendation UI | `PLANNED` |
| 10 | `apps/admin` `/comparison-profiles` + score-factor editor | `PLANNED` |
| 11 | Flags, RBAC, Valkey, catalogs | `PLANNED` |
| 12 | Tests + build verification | `PLANNED` |

---

### Task 1: Schema migrations `000551`–`000555`

**Files:**
- Create `migrations/000551_jce_profiles.up.sql`
- Create `migrations/000551_jce_profiles.down.sql`
- Create `migrations/000552_jce_score_factors.up.sql`
- Create `migrations/000552_jce_score_factors.down.sql`
- Create `migrations/000553_jce_scores.up.sql`
- Create `migrations/000553_jce_scores.down.sql`
- Create `migrations/000554_jce_recommendations.up.sql`
- Create `migrations/000554_jce_recommendations.down.sql`
- Create `migrations/000555_jce_subspec_2_seed.up.sql`
- Create `migrations/000555_jce_subspec_2_seed.down.sql`

**Interfaces:**
- Produces the four new tables (`comparison_profiles`, `comparison_score_factors`, `comparison_scores`, `comparison_recommendations`) and indexes.
- Produces the `feature.jce.profiles`, `feature.jce.smart_compare`, `feature.jce.scoring` flag rows, `jce.default_goals` / `jce.profile_cache_ttl_minutes` / `jce.score_cache_ttl_minutes` system settings, `jce.*` permission nodes and grants, default `comparison_type_definitions.config`, and global `comparison_score_factors` seed rows.

**Depends on:** None.

- [ ] **Step 1: Write `migrations/000551_jce_profiles.up.sql`**

```sql
-- 000551_jce_profiles.up.sql
BEGIN;

CREATE TABLE IF NOT EXISTS comparison_profiles (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    user_id         uuid REFERENCES users(id) ON DELETE CASCADE,
    title           text NOT NULL,
    source_space_id uuid REFERENCES spaces(id) ON DELETE SET NULL,
    target_space_ids uuid[] NOT NULL DEFAULT '{}',
    goals           jsonb NOT NULL DEFAULT '[]'::jsonb,
    preferences     jsonb NOT NULL DEFAULT '{}'::jsonb,
    demographics    jsonb NOT NULL DEFAULT '{}'::jsonb,
    is_default      boolean NOT NULL DEFAULT false,
    is_template     boolean NOT NULL DEFAULT false,
    deleted_at      timestamptz,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_comparison_profiles_tenant_user
    ON comparison_profiles(tenant_id, user_id)
    WHERE user_id IS NOT NULL AND deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_comparison_profiles_tenant_templates
    ON comparison_profiles(tenant_id, is_template)
    WHERE is_template = true AND deleted_at IS NULL;

CREATE UNIQUE INDEX IF NOT EXISTS idx_comparison_profiles_one_default
    ON comparison_profiles(tenant_id, user_id)
    WHERE is_default = true AND user_id IS NOT NULL AND deleted_at IS NULL;

COMMIT;
```

- [ ] **Step 2: Write `migrations/000551_jce_profiles.down.sql`**

```sql
BEGIN;
DROP TABLE IF EXISTS comparison_profiles CASCADE;
COMMIT;
```

- [ ] **Step 3: Write `migrations/000552_jce_score_factors.up.sql`**

```sql
-- 000552_jce_score_factors.up.sql
BEGIN;

CREATE TABLE IF NOT EXISTS comparison_score_factors (
    id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id           uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    comparison_type_id  uuid REFERENCES comparison_type_definitions(id) ON DELETE CASCADE,
    factor_key          text NOT NULL,
    label               text NOT NULL,
    description         text,
    dimension_slug      text,
    data_source         text NOT NULL CHECK (data_source IN ('dimension', 'resolver_metadata', 'profile', 'eligibility', 'aggregate')),
    default_weight      numeric NOT NULL DEFAULT 0 CHECK (default_weight >= 0 AND default_weight <= 1),
    config              jsonb NOT NULL DEFAULT '{}'::jsonb,
    version             int NOT NULL DEFAULT 1,
    is_system           boolean NOT NULL DEFAULT false,
    created_at          timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now(),
    UNIQUE(tenant_id, comparison_type_id, factor_key)
);

CREATE INDEX IF NOT EXISTS idx_comparison_score_factors_tenant_type
    ON comparison_score_factors(tenant_id, comparison_type_id);

CREATE INDEX IF NOT EXISTS idx_comparison_score_factors_system
    ON comparison_score_factors(tenant_id, is_system)
    WHERE is_system = true;

COMMIT;
```

- [ ] **Step 4: Write `migrations/000552_jce_score_factors.down.sql`**

```sql
BEGIN;
DROP TABLE IF EXISTS comparison_score_factors CASCADE;
COMMIT;
```

- [ ] **Step 5: Write `migrations/000553_jce_scores.up.sql`**

```sql
-- 000553_jce_scores.up.sql
BEGIN;

CREATE TABLE IF NOT EXISTS comparison_scores (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    content_id      uuid NOT NULL REFERENCES contents(id) ON DELETE CASCADE,
    item_id         uuid NOT NULL REFERENCES comparison_items(id) ON DELETE CASCADE,
    factor_id       uuid NOT NULL REFERENCES comparison_score_factors(id) ON DELETE CASCADE,
    dimension_id    uuid REFERENCES comparison_dimensions(id) ON DELETE SET NULL,
    profile_id      uuid REFERENCES comparison_profiles(id) ON DELETE SET NULL,
    score_value     numeric NOT NULL DEFAULT 0,
    max_value       numeric NOT NULL DEFAULT 100,
    explanation     text,
    eligible_status text CHECK (eligible_status IN ('eligible', 'limited', 'ineligible')),
    calculated_at   timestamptz NOT NULL DEFAULT now(),
    version         int NOT NULL DEFAULT 1,
    UNIQUE(content_id, item_id, factor_id, profile_id)
);

CREATE INDEX IF NOT EXISTS idx_comparison_scores_content_profile
    ON comparison_scores(content_id, profile_id, version);

CREATE INDEX IF NOT EXISTS idx_comparison_scores_item
    ON comparison_scores(item_id);

CREATE INDEX IF NOT EXISTS idx_comparison_scores_calculated
    ON comparison_scores(content_id, calculated_at);

UPDATE comparison_content_metadata
SET config = COALESCE(config, '{}'::jsonb) || jsonb_build_object(
    'profile_id', null,
    'weights_snapshot', '{}'::jsonb,
    'recommendation_summary', '{}'::jsonb,
    'last_scored_at', null
);

COMMIT;
```

- [ ] **Step 6: Write `migrations/000553_jce_scores.down.sql`**

```sql
BEGIN;
DROP TABLE IF EXISTS comparison_scores CASCADE;

UPDATE comparison_content_metadata
SET config = config - 'profile_id' - 'weights_snapshot' - 'recommendation_summary' - 'last_scored_at';

COMMIT;
```

- [ ] **Step 7: Write `migrations/000554_jce_recommendations.up.sql`**

```sql
-- 000554_jce_recommendations.up.sql
BEGIN;

CREATE TABLE IF NOT EXISTS comparison_recommendations (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    content_id      uuid NOT NULL REFERENCES contents(id) ON DELETE CASCADE,
    item_id         uuid REFERENCES comparison_items(id) ON DELETE CASCADE,
    profile_id      uuid REFERENCES comparison_profiles(id) ON DELETE SET NULL,
    factor_id       uuid REFERENCES comparison_score_factors(id) ON DELETE SET NULL,
    text            text NOT NULL,
    action_type     text,
    action_url      text,
    priority        int NOT NULL DEFAULT 0,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_comparison_recommendations_content
    ON comparison_recommendations(content_id, profile_id, priority DESC);

CREATE INDEX IF NOT EXISTS idx_comparison_recommendations_item
    ON comparison_recommendations(item_id);

COMMIT;
```

- [ ] **Step 8: Write `migrations/000554_jce_recommendations.down.sql`**

```sql
BEGIN;
DROP TABLE IF EXISTS comparison_recommendations CASCADE;
COMMIT;
```

- [ ] **Step 9: Write `migrations/000555_jce_subspec_2_seed.up.sql`**

```sql
-- 000555_jce_subspec_2_seed.up.sql
BEGIN;

INSERT INTO feature_flags (key, tenant_id, enabled, config)
VALUES
    ('feature.jce.profiles', '00000000-0000-0000-0000-000000000001', false, '{}'::jsonb),
    ('feature.jce.smart_compare', '00000000-0000-0000-0000-000000000001', false, '{}'::jsonb),
    ('feature.jce.scoring', '00000000-0000-0000-0000-000000000001', false, '{}'::jsonb)
ON CONFLICT (key, tenant_id) DO NOTHING;

INSERT INTO system_settings (tenant_id, key, value)
VALUES
    ('00000000-0000-0000-0000-000000000001', 'jce.default_goals', '[
        {"key": "pr_fast", "label": "Get permanent residency fast", "description": "Prioritise routes with a clear, short PR pathway", "default_weight": 0.25},
        {"key": "low_cost", "label": "Low cost of living", "description": "Minimise tuition, rent, and daily expenses", "default_weight": 0.25},
        {"key": "high_income", "label": "High earning potential", "description": "Maximise post-study or post-move salary", "default_weight": 0.20},
        {"key": "family_friendly", "label": "Family friendly", "description": "Favour schooling, healthcare, and family visa options", "default_weight": 0.15},
        {"key": "scholarships", "label": "Scholarship opportunities", "description": "Favour funding and bursaries", "default_weight": 0.10},
        {"key": "language_ease", "label": "Language ease", "description": "Favour destinations that match my language skills", "default_weight": 0.05}
    ]'::jsonb),
    ('00000000-0000-0000-0000-000000000001', 'jce.profile_cache_ttl_minutes', to_jsonb(5)),
    ('00000000-0000-0000-0000-000000000001', 'jce.score_cache_ttl_minutes', to_jsonb(15))
ON CONFLICT (tenant_id, key) DO NOTHING;

INSERT INTO permissions (key, description)
VALUES
    ('jce.profile.list', 'List comparison profiles'),
    ('jce.profile.view', 'View a comparison profile'),
    ('jce.profile.create', 'Create a comparison profile'),
    ('jce.profile.update', 'Update a comparison profile'),
    ('jce.profile.delete', 'Delete a comparison profile'),
    ('jce.score.view', 'View JapaScore for a comparison'),
    ('jce.score.recalculate', 'Recalculate JapaScore for a comparison'),
    ('jce.recommendation.view', 'View comparison recommendations'),
    ('jce.factor.manage', 'Manage score factors and scoring rules')
ON CONFLICT (key) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug IN ('member', 'admin', 'super_admin')
  AND p.key IN (
      'jce.profile.list', 'jce.profile.view', 'jce.profile.create',
      'jce.profile.update', 'jce.profile.delete'
  )
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug IN ('member', 'admin', 'super_admin')
  AND p.key IN ('jce.score.view', 'jce.score.recalculate', 'jce.recommendation.view')
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug IN ('admin', 'super_admin')
  AND p.key = 'jce.factor.manage'
ON CONFLICT DO NOTHING;

UPDATE comparison_type_definitions
SET config = COALESCE(config, '{}'::jsonb) || jsonb_build_object(
    'allowed_goals', '[
        {"key": "pr_fast", "label": "Get permanent residency fast", "description": "Prioritise routes with a clear, short PR pathway", "default_weight": 0.25},
        {"key": "low_cost", "label": "Low cost of living", "description": "Minimise tuition, rent, and daily expenses", "default_weight": 0.25},
        {"key": "high_income", "label": "High earning potential", "description": "Maximise post-study or post-move salary", "default_weight": 0.20},
        {"key": "family_friendly", "label": "Family friendly", "description": "Favour schooling, healthcare, and family visa options", "default_weight": 0.15},
        {"key": "scholarships", "label": "Scholarship opportunities", "description": "Favour funding and bursaries", "default_weight": 0.10},
        {"key": "language_ease", "label": "Language ease", "description": "Favour destinations that match my language skills", "default_weight": 0.05}
    ]'::jsonb,
    'profile_defaults', jsonb_build_object(
        'demographics', jsonb_build_object(
            'language_proficiency', 'intermediate',
            'budget_currency', 'USD'
        ),
        'goals', '[]'::jsonb,
        'preferences', '{}'::jsonb
    ),
    'eligibility_rules', '[]'::jsonb,
    'goal_factor_map', jsonb_build_object(
        'pr_fast', '["pr_path", "speed"]'::jsonb,
        'low_cost', '["cost"]'::jsonb,
        'high_income', '["income"]'::jsonb,
        'family_friendly', '["family"]'::jsonb,
        'scholarships', '["funding"]'::jsonb,
        'language_ease', '["language_match"]'::jsonb
    ),
    'scoring_enabled', true,
    'default_weights', jsonb_build_object(
        'eligibility', 0.25,
        'cost', 0.20,
        'income', 0.15,
        'speed', 0.10,
        'language_match', 0.10,
        'location_match', 0.10,
        'pr_path', 0.10
    ),
    'smart_modes', jsonb_build_object(
        'eligibility', jsonb_build_object(
            'filter_ineligible', true,
            'factor_multipliers', jsonb_build_object('eligibility', 2.0)
        ),
        'cost', jsonb_build_object(
            'factor_multipliers', jsonb_build_object('cost', 2.0)
        ),
        'pathway', jsonb_build_object(
            'factor_multipliers', jsonb_build_object('pr_path', 1.5, 'speed', 1.5)
        )
    ),
    'recommendation_templates', '[
        {"factor_key": "language_match", "condition": "score_lt", "threshold": 50, "text": "Book a language test to improve your match.", "action_type": "book", "priority": 20},
        {"factor_key": "cost", "condition": "score_lt", "threshold": 40, "text": "Review your budget against this option.", "action_type": "review", "priority": 10},
        {"factor_key": "eligibility", "condition": "status_in", "statuses": ["limited", "ineligible"], "text": "Check the eligibility rules for this option.", "action_type": "check", "priority": 30}
    ]'::jsonb
)
WHERE is_system = true;

INSERT INTO comparison_score_factors (
    tenant_id, comparison_type_id, factor_key, label, description,
    data_source, default_weight, config, is_system
)
VALUES
    ('00000000-0000-0000-0000-000000000001', NULL, 'japa_score', 'JapaScore', 'Overall 0–100 decision score', 'aggregate', 0, '{"is_output": true, "max": 100}'::jsonb, true),
    ('00000000-0000-0000-0000-000000000001', NULL, 'eligibility', 'Eligibility', 'Hard and soft eligibility rules from the comparison type', 'eligibility', 0.25, '{"penalty_ineligible": 0}'::jsonb, true),
    ('00000000-0000-0000-0000-000000000001', NULL, 'cost', 'Cost', 'Cost of living, tuition, or fees (lower is better)', 'resolver_metadata', 0.20, '{"kind": "number", "direction": "lower_is_better", "value_key": "cost", "missing_score": 0}'::jsonb, true),
    ('00000000-0000-0000-0000-000000000001', NULL, 'income', 'Income', 'Salary or earning potential (higher is better)', 'resolver_metadata', 0.15, '{"kind": "number", "direction": "higher_is_better", "value_key": "income", "missing_score": 0}'::jsonb, true),
    ('00000000-0000-0000-0000-000000000001', NULL, 'speed', 'Speed', 'Time to outcome (lower is better)', 'resolver_metadata', 0.10, '{"kind": "number", "direction": "lower_is_better", "value_key": "speed", "missing_score": 0}'::jsonb, true),
    ('00000000-0000-0000-0000-000000000001', NULL, 'language_match', 'Language match', 'How well the item matches the profile language', 'profile', 0.10, '{"profile_key": "demographics.language_proficiency", "item_key": "language", "match_strategy": "exact"}'::jsonb, true),
    ('00000000-0000-0000-0000-000000000001', NULL, 'location_match', 'Location match', 'Whether the item is in the profile target spaces', 'profile', 0.10, '{"profile_key": "target_space_ids", "item_key": "geo_space_id", "match_strategy": "exact"}'::jsonb, true),
    ('00000000-0000-0000-0000-000000000001', NULL, 'pr_path', 'PR pathway', 'Strength of the permanent residency pathway', 'resolver_metadata', 0.10, '{"kind": "number", "direction": "higher_is_better", "value_key": "pr_path_ease", "missing_score": 0}'::jsonb, true)
ON CONFLICT (tenant_id, comparison_type_id, factor_key) DO NOTHING;

COMMIT;
```

- [ ] **Step 10: Write `migrations/000555_jce_subspec_2_seed.down.sql`**

```sql
BEGIN;

DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT id FROM permissions
    WHERE key LIKE 'jce.%'
);

DELETE FROM permissions WHERE key LIKE 'jce.%';

DELETE FROM feature_flags
WHERE key IN ('feature.jce.profiles', 'feature.jce.smart_compare', 'feature.jce.scoring')
  AND tenant_id = '00000000-0000-0000-0000-000000000001';

DELETE FROM system_settings
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND key IN ('jce.default_goals', 'jce.profile_cache_ttl_minutes', 'jce.score_cache_ttl_minutes');

DELETE FROM comparison_score_factors
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND comparison_type_id IS NULL
  AND factor_key IN ('japa_score', 'eligibility', 'cost', 'income', 'speed', 'language_match', 'location_match', 'pr_path');

UPDATE comparison_type_definitions
SET config = config
    - 'allowed_goals'
    - 'profile_defaults'
    - 'eligibility_rules'
    - 'goal_factor_map'
    - 'scoring_enabled'
    - 'default_weights'
    - 'smart_modes'
    - 'recommendation_templates'
WHERE is_system = true;

COMMIT;
```

- [ ] **Step 11: Apply and generate**

Run `make migrate` to apply all five migrations, then `make sqlc-gen`.

**Verification:** `go test -tags integration ./...` passes migration round-trip. `make sqlc-gen` exits `0`.

---

### Task 2: sqlc queries for new tables

**Files:**
- Modify `internal/db/queries/comparison.sql` (or create `internal/db/queries/jce.sql`; sqlc compiles everything in `internal/db/queries/`).

**Interfaces:**
- Consumes the tables from Task 1.
- Produces `gen.Queries` methods used by `internal/jce/profileStore`, `internal/jce/scoreFactorStore`, `internal/jce/scoring.Engine`, and `internal/jce/service`.

**Depends on:** Task 1.

- [ ] **Step 1: Append the following query declarations to the query file**

```sql
-- =====================
-- Comparison profiles
-- =====================

-- name: CreateComparisonProfile :one
INSERT INTO comparison_profiles (
    tenant_id, user_id, title, source_space_id, target_space_ids,
    goals, preferences, demographics, is_default, is_template
) VALUES (
    @tenant_id, @user_id, @title, @source_space_id, @target_space_ids,
    @goals, @preferences, @demographics, @is_default, @is_template
)
RETURNING id, tenant_id, user_id, title, source_space_id, target_space_ids,
    goals, preferences, demographics, is_default, is_template, deleted_at, created_at, updated_at;

-- name: ListComparisonProfilesByUser :many
SELECT id, tenant_id, user_id, title, source_space_id, target_space_ids,
    goals, preferences, demographics, is_default, is_template, deleted_at, created_at, updated_at
FROM comparison_profiles
WHERE tenant_id = @tenant_id
  AND user_id = @user_id
  AND deleted_at IS NULL
ORDER BY is_default DESC, updated_at DESC;

-- name: ListComparisonProfileTemplates :many
SELECT id, tenant_id, user_id, title, source_space_id, target_space_ids,
    goals, preferences, demographics, is_default, is_template, deleted_at, created_at, updated_at
FROM comparison_profiles
WHERE tenant_id = @tenant_id
  AND is_template = true
  AND deleted_at IS NULL
ORDER BY title ASC;

-- name: GetComparisonProfile :one
SELECT id, tenant_id, user_id, title, source_space_id, target_space_ids,
    goals, preferences, demographics, is_default, is_template, deleted_at, created_at, updated_at
FROM comparison_profiles
WHERE id = @id AND tenant_id = @tenant_id AND deleted_at IS NULL;

-- name: UpdateComparisonProfile :one
UPDATE comparison_profiles
SET title = @title,
    source_space_id = @source_space_id,
    target_space_ids = @target_space_ids,
    goals = @goals,
    preferences = @preferences,
    demographics = @demographics,
    is_default = @is_default,
    is_template = @is_template,
    updated_at = now()
WHERE id = @id AND tenant_id = @tenant_id AND deleted_at IS NULL
RETURNING id, tenant_id, user_id, title, source_space_id, target_space_ids,
    goals, preferences, demographics, is_default, is_template, deleted_at, created_at, updated_at;

-- name: DeleteComparisonProfile :exec
UPDATE comparison_profiles
SET deleted_at = now()
WHERE id = @id AND tenant_id = @tenant_id AND deleted_at IS NULL;

-- name: ClearDefaultProfileForUser :exec
UPDATE comparison_profiles
SET is_default = false, updated_at = now()
WHERE tenant_id = @tenant_id
  AND user_id = @user_id
  AND is_default = true
  AND id <> @exclude_id;

-- =====================
-- Score factors
-- =====================

-- name: CreateScoreFactor :one
INSERT INTO comparison_score_factors (
    tenant_id, comparison_type_id, factor_key, label, description,
    dimension_slug, data_source, default_weight, config, is_system
) VALUES (
    @tenant_id, @comparison_type_id, @factor_key, @label, @description,
    @dimension_slug, @data_source, @default_weight, @config, @is_system
)
RETURNING id, tenant_id, comparison_type_id, factor_key, label, description,
    dimension_slug, data_source, default_weight, config, version, is_system, created_at, updated_at;

-- name: ListScoreFactorsByType :many
SELECT id, tenant_id, comparison_type_id, factor_key, label, description,
    dimension_slug, data_source, default_weight, config, version, is_system, created_at, updated_at
FROM comparison_score_factors
WHERE tenant_id = @tenant_id
  AND (
      comparison_type_id = @comparison_type_id
      OR (sqlc.narg(include_global)::boolean = true AND comparison_type_id IS NULL)
  )
  AND (sqlc.narg(data_source)::text IS NULL OR data_source = sqlc.narg(data_source)::text)
ORDER BY is_system DESC, factor_key ASC;

-- name: GetScoreFactor :one
SELECT id, tenant_id, comparison_type_id, factor_key, label, description,
    dimension_slug, data_source, default_weight, config, version, is_system, created_at, updated_at
FROM comparison_score_factors
WHERE id = @id AND tenant_id = @tenant_id;

-- name: UpdateScoreFactor :one
UPDATE comparison_score_factors
SET comparison_type_id = @comparison_type_id,
    factor_key = @factor_key,
    label = @label,
    description = @description,
    dimension_slug = @dimension_slug,
    data_source = @data_source,
    default_weight = @default_weight,
    config = @config,
    is_system = @is_system,
    updated_at = now()
WHERE id = @id AND tenant_id = @tenant_id
RETURNING id, tenant_id, comparison_type_id, factor_key, label, description,
    dimension_slug, data_source, default_weight, config, version, is_system, created_at, updated_at;

-- name: DeleteScoreFactor :exec
DELETE FROM comparison_score_factors
WHERE id = @id AND tenant_id = @tenant_id;

-- name: GetScoreFactorByKey :one
SELECT id, tenant_id, comparison_type_id, factor_key, label, description,
    dimension_slug, data_source, default_weight, config, version, is_system, created_at, updated_at
FROM comparison_score_factors
WHERE tenant_id = @tenant_id
  AND comparison_type_id = @comparison_type_id
  AND factor_key = @factor_key;

-- =====================
-- Scores
-- =====================

-- name: UpsertComparisonScore :one
INSERT INTO comparison_scores (
    content_id, item_id, factor_id, dimension_id, profile_id,
    score_value, max_value, explanation, eligible_status, version
) VALUES (
    @content_id, @item_id, @factor_id, @dimension_id, @profile_id,
    @score_value, @max_value, @explanation, @eligible_status, @version
)
ON CONFLICT (content_id, item_id, factor_id, profile_id) DO UPDATE
SET score_value = EXCLUDED.score_value,
    max_value = EXCLUDED.max_value,
    explanation = EXCLUDED.explanation,
    eligible_status = EXCLUDED.eligible_status,
    version = EXCLUDED.version,
    calculated_at = now()
RETURNING id, content_id, item_id, factor_id, dimension_id, profile_id,
    score_value, max_value, explanation, eligible_status, calculated_at, version;

-- name: ListComparisonScoresByContent :many
SELECT id, content_id, item_id, factor_id, dimension_id, profile_id,
    score_value, max_value, explanation, eligible_status, calculated_at, version
FROM comparison_scores
WHERE content_id = @content_id
  AND (profile_id = @profile_id OR @profile_id IS NULL)
ORDER BY item_id, factor_id;

-- name: DeleteComparisonScoresByContent :exec
DELETE FROM comparison_scores
WHERE content_id = @content_id AND (profile_id = @profile_id OR @profile_id IS NULL);

-- =====================
-- Recommendations
-- =====================

-- name: CreateComparisonRecommendation :one
INSERT INTO comparison_recommendations (
    content_id, item_id, profile_id, factor_id, text, action_type, action_url, priority
) VALUES (
    @content_id, @item_id, @profile_id, @factor_id, @text, @action_type, @action_url, @priority
)
RETURNING id, content_id, item_id, profile_id, factor_id, text, action_type, action_url, priority, created_at, updated_at;

-- name: ListComparisonRecommendationsByContent :many
SELECT id, content_id, item_id, profile_id, factor_id, text, action_type, action_url, priority, created_at, updated_at
FROM comparison_recommendations
WHERE content_id = @content_id
  AND (profile_id = @profile_id OR @profile_id IS NULL)
ORDER BY priority DESC, created_at ASC;

-- name: DeleteComparisonRecommendationsByContent :exec
DELETE FROM comparison_recommendations
WHERE content_id = @content_id AND (profile_id = @profile_id OR @profile_id IS NULL);

-- =====================
-- Scoring helpers
-- =====================

-- name: GetComparisonItemsForScoring :many
SELECT id, content_id, type_id, title, description, image_id, geo_space_id, position, item_ref, created_at, deleted_at
FROM comparison_items
WHERE content_id = @content_id AND tenant_id = @tenant_id AND deleted_at IS NULL
ORDER BY position ASC;

-- name: GetComparisonValuesForScoring :many
SELECT cv.dimension_id, cv.item_id, cv.value, cv.value_rating, cv.value_boolean, cd.slug AS dimension_slug
FROM comparison_values cv
JOIN comparison_dimensions cd ON cd.id = cv.dimension_id
WHERE cv.tenant_id = @tenant_id
  AND cd.content_id = @content_id
  AND cv.item_id = ANY(@item_ids::uuid[]);

-- name: CountComparisonProfilesBySourceSpace :many
SELECT source_space_id, count(*)::int AS profile_count
FROM comparison_profiles
WHERE tenant_id = @tenant_id
  AND is_template = false
  AND deleted_at IS NULL
GROUP BY source_space_id;
```

- [ ] **Step 2: Run `make sqlc-gen`.**

- [ ] **Step 3: Verify `internal/db/gen/comparison.sql.go` (or `jce.sql.go`) now contains the new methods.**

**Verification:** `go build ./...` and `make vet` pass.

---

### Task 3: Go models and store for `internal/jce/profiles.go` and `internal/jce/score_factors.go`

**Files:**
- Create `internal/jce/models.go`
- Create `internal/jce/profiles.go`
- Create `internal/jce/score_factors.go`
- Create `internal/jce/recommendations.go`
- Create `internal/jce/profiles_test.go`
- Create `internal/jce/score_factors_test.go`

**Interfaces:**
- Consumes `gen.Queries` from Task 2, `github.com/jedafe/japa/internal/db`.
- Produces domain models and store methods used by `internal/jce/service.go` and `internal/jce/scoring/engine.go`.

**Depends on:** Tasks 1–2.

- [ ] **Step 1: Define domain models in `internal/jce/models.go`**

```go
package jce

import "github.com/jackc/pgx/v5/pgtype"

type ProfileGoal struct {
    Key    string  `json:"key"`
    Weight float64 `json:"weight"`
}

type ProfileDemographics struct {
    Age                 *int     `json:"age"`
    Occupation          string   `json:"occupation"`
    FamilyStatus        string   `json:"family_status"`
    LanguageProficiency string   `json:"language_proficiency"`
    BudgetMin           *float64 `json:"budget_min"`
    BudgetMax           *float64 `json:"budget_max"`
    BudgetCurrency      string   `json:"budget_currency"`
}

type ComparisonProfile struct {
    ID             string
    TenantID       string
    UserID         *string
    Title          string
    SourceSpaceID  *string
    TargetSpaceIDs []string
    Goals          []ProfileGoal
    Preferences    map[string]float64
    Demographics   ProfileDemographics
    IsDefault      bool
    IsTemplate     bool
    CreatedAt      string
    UpdatedAt      string
}

type ComparisonProfileInput struct {
    Title          string
    SourceSpaceID  *string
    TargetSpaceIDs []string
    Goals          []ProfileGoal
    Preferences    map[string]float64
    Demographics   ProfileDemographics
    IsDefault      bool
    IsTemplate     bool
}

type ScoreFactorConfig struct {
    Kind          string   `json:"kind"`
    Direction     string   `json:"direction"`
    Min           float64  `json:"min"`
    Max           float64  `json:"max"`
    ValueKey      string   `json:"value_key"`
    ProfileKey    string   `json:"profile_key"`
    ItemKey       string   `json:"item_key"`
    MatchStrategy string   `json:"match_strategy"`
    IdealTexts    []string `json:"ideal_texts"`
    MissingScore  float64  `json:"missing_score"`
    IsOutput      bool     `json:"is_output"`
}

type ScoreFactor struct {
    ID               string
    TenantID         string
    ComparisonTypeID *string
    FactorKey        string
    Label            string
    Description      string
    DimensionSlug    *string
    DataSource       string
    DefaultWeight    float64
    Config           ScoreFactorConfig
    Version          int
    IsSystem         bool
    CreatedAt        string
    UpdatedAt        string
}

type ScoreFactorInput struct {
    ComparisonTypeID *string
    FactorKey        string
    Label            string
    Description      string
    DimensionSlug    *string
    DataSource       string
    DefaultWeight    float64
    Config           ScoreFactorConfig
    IsSystem         bool
}

type FactorScore struct {
    FactorKey   string
    Label       string
    ScoreValue  float64
    MaxValue    float64
    Weight      float64
    Explanation string
}

type JapaScore struct {
    ItemID         string
    ItemTitle      string
    JapaScore      float64
    MaxValue       float64
    EligibleStatus string
    FactorScores   []FactorScore
    Explanation    string
}

type ComparisonRecommendation struct {
    ID         string
    ItemID     *string
    Text       string
    ActionType *string
    ActionURL  *string
    Priority   int
}

type ScoreResult struct {
    ContentID             string
    ProfileID             *string
    SmartMode             string
    Scores                []JapaScore
    Recommendations       []ComparisonRecommendation
    WeightsSnapshot       map[string]float64
    RecommendationSummary map[string]any
    Version               int
}

type ScoreInput struct {
    ContentID string
    TenantID  string
    UserID    *string
    ProfileID *string
    SmartMode string
    Force     bool
}

type GetScoreInput struct {
    ContentID string
    TenantID  string
    UserID    *string
    ProfileID *string
}

type ScorePreviewInput struct {
    TypeID       string
    SmartMode    string
    ItemRefs     []map[string]any
    ProfileInput *ComparisonProfileInput
}

type ProfileStat struct {
    SourceSpaceID string
    ProfileCount  int32
}
```

- [ ] **Step 2: Implement `internal/jce/profiles.go` (profile store)**

```go
package jce

import (
    "context"
    "encoding/json"
    "fmt"

    "github.com/jackc/pgx/v5/pgtype"
    "github.com/jedafe/japa/internal/db"
    "github.com/jedafe/japa/internal/db/gen"
)

type profileStore struct {
    read  *gen.Queries
    write *gen.Queries
}

func newProfileStore(database *db.DB) *profileStore {
    return &profileStore{
        read:  gen.New(database.Reader),
        write: gen.New(database.Writer),
    }
}

func (s *profileStore) create(ctx context.Context, tenantID, userID pgtype.UUID, in ComparisonProfileInput) (ComparisonProfile, error) { /* implement per the step notes */ }
func (s *profileStore) listByUser(ctx context.Context, tenantID, userID pgtype.UUID) ([]ComparisonProfile, error) { /* implement per the step notes */ }
func (s *profileStore) listTemplates(ctx context.Context, tenantID pgtype.UUID) ([]ComparisonProfile, error) { /* implement per the step notes */ }
func (s *profileStore) get(ctx context.Context, id, tenantID pgtype.UUID) (ComparisonProfile, error) { /* implement per the step notes */ }
func (s *profileStore) update(ctx context.Context, id, tenantID, userID pgtype.UUID, in ComparisonProfileInput) (ComparisonProfile, error) { /* implement per the step notes */ }
func (s *profileStore) delete(ctx context.Context, id, tenantID pgtype.UUID) error { /* implement per the step notes */ }
func (s *profileStore) setDefault(ctx context.Context, id, tenantID, userID pgtype.UUID) error { /* implement per the step notes */ }
func (s *profileStore) countBySourceSpace(ctx context.Context, tenantID, userID pgtype.UUID) ([]ProfileStat, error) { /* implement per the step notes */ }

func profileFromRow(row gen.ComparisonProfile) (ComparisonProfile, error) { /* implement per the step notes */ }
func marshalProfileGoals(goals []ProfileGoal) []byte { /* implement per the step notes */ }
func marshalProfilePreferences(p map[string]float64) []byte { /* implement per the step notes */ }
func marshalProfileDemographics(d ProfileDemographics) []byte { /* implement per the step notes */ }
```

Implementation notes:
- Convert `pgtype.UUID` to string with `u.String()`; map SQL `uuid[]` to `[]string` by iterating.
- Convert `goals`, `preferences`, `demographics` to `[]byte` with `json.Marshal` for SQL.
- In `profileFromRow`, `json.Unmarshal` the JSONB columns into `[]ProfileGoal`, `map[string]float64`, and `ProfileDemographics`.
- `setDefault` wraps a transaction: call `ClearDefaultProfileForUser` with the target `id` as `exclude_id`, then read the profile and call `UpdateComparisonProfile` with the same fields but `is_default = true`.

- [ ] **Step 3: Implement `internal/jce/score_factors.go` (score-factor store)**

```go
package jce

type scoreFactorStore struct {
    read  *gen.Queries
    write *gen.Queries
}

func newScoreFactorStore(database *db.DB) *scoreFactorStore {
    return &scoreFactorStore{
        read:  gen.New(database.Reader),
        write: gen.New(database.Writer),
    }
}

func (s *scoreFactorStore) create(ctx context.Context, tenantID pgtype.UUID, in ScoreFactorInput) (ScoreFactor, error) { /* implement per the step notes */ }
func (s *scoreFactorStore) listByType(ctx context.Context, tenantID, comparisonTypeID pgtype.UUID, includeGlobal bool) ([]ScoreFactor, error) { /* implement per the step notes */ }
func (s *scoreFactorStore) get(ctx context.Context, id, tenantID pgtype.UUID) (ScoreFactor, error) { /* implement per the step notes */ }
func (s *scoreFactorStore) update(ctx context.Context, id, tenantID pgtype.UUID, in ScoreFactorInput) (ScoreFactor, error) { /* implement per the step notes */ }
func (s *scoreFactorStore) delete(ctx context.Context, id, tenantID pgtype.UUID) error { /* implement per the step notes */ }
func (s *scoreFactorStore) getByKey(ctx context.Context, tenantID, comparisonTypeID pgtype.UUID, factorKey string) (ScoreFactor, error) { /* implement per the step notes */ }
```

Implementation notes:
- Marshal `ScoreFactorInput.Config` to `[]byte` with `json.Marshal` before passing to `CreateScoreFactor` or `UpdateScoreFactor`.
- For `listByType`, call `ListScoreFactorsByType` with `include_global` set to `true` or `false` and `data_source` set to a `pgtype.Text{Valid: false}` value.
- A `nil` `comparisonTypeID` (or `pgtype.UUID{Valid: false}`) in `getByKey` selects global factors.
- Convert `comparison_type_id`, `dimension_slug`, and `config` between `pgtype.UUID`/`[]byte` and the domain `*string`/`ScoreFactorConfig` in a `scoreFactorFromRow` helper.

- [ ] **Step 4: Implement `internal/jce/recommendations.go` (recommendation store)**

```go
package jce

type recommendationStore struct {
    read  *gen.Queries
    write *gen.Queries
}

func newRecommendationStore(database *db.DB) *recommendationStore {
    return &recommendationStore{
        read:  gen.New(database.Reader),
        write: gen.New(database.Writer),
    }
}

func (s *recommendationStore) listByContent(ctx context.Context, contentID, profileID pgtype.UUID, limit int32) ([]ComparisonRecommendation, error) { /* implement per the step notes */ }
func (s *recommendationStore) create(ctx context.Context, contentID, itemID, profileID, factorID pgtype.UUID, text, actionType, actionURL string, priority int32) (ComparisonRecommendation, error) { /* implement per the step notes */ }
func (s *recommendationStore) deleteByContent(ctx context.Context, contentID, profileID pgtype.UUID) error { /* implement per the step notes */ }
```

Implementation notes:
- `listByContent` calls `ListComparisonRecommendationsByContent` and applies the `limit` to the returned slice.
- `create` calls `CreateComparisonRecommendation` with the provided IDs and strings; `action_type` and `action_url` may be empty and should map to `pgtype.Text{Valid: false}` when null.
- `deleteByContent` calls `DeleteComparisonRecommendationsByContent` and passes `profile_id` as `pgtype.UUID{Valid: false}` when it is the zero UUID.

- [ ] **Step 5: Write `internal/jce/profiles_test.go` and `internal/jce/score_factors_test.go` first, then make them pass**

Test functions:
- `TestProfileStore_CreateListGetUpdateDeleteDefault`
- `TestProfileStore_TemplateAccess`
- `TestScoreFactorStore_CreateListUpdateDelete`
- `TestScoreFactorStore_GlobalOverride`

**Verification:** `go test ./internal/jce/...` and `go build ./...` pass.

---

### Task 4: Scoring engine package `internal/jce/scoring/engine.go`

**Files:**
- Create `internal/jce/scoring/engine.go`
- Create `internal/jce/scoring/engine_test.go`

**Interfaces:**
- Consumes `gen.Queries`, `internal/comparison/resolver.Registry`, `internal/config.Settings`, `github.com/valkey-io/valkey-go/valkeycompat.Cmdable`, `internal/jce.ScoreResult`, `internal/jce.ScoreInput`, `internal/jce.ScorePreviewInput`, `internal/jce.ComparisonProfile`, `internal/jce.ScoreFactor`.
- Produces `*scoring.Engine` with `Score`, `GetScore`, `PreviewScoreForItems` methods.

**Depends on:** Tasks 1–3.

- [ ] **Step 1: Define the `Engine` struct and constructor**

```go
package scoring

import (
    "context"
    "encoding/json"
    "fmt"
    "strconv"
    "time"

    "github.com/jackc/pgx/v5/pgtype"
    "github.com/jedafe/japa/internal/comparison"
    "github.com/jedafe/japa/internal/comparison/resolver"
    "github.com/jedafe/japa/internal/config"
    "github.com/jedafe/japa/internal/db"
    "github.com/jedafe/japa/internal/db/gen"
    "github.com/jedafe/japa/internal/jce"
    "github.com/valkey-io/valkey-go/valkeycompat"
)

type Engine struct {
    read    *gen.Queries
    write   *gen.Queries
    compSvc *comparison.Service
    r       *resolver.Registry
    settings *config.Settings
    vk      valkeycompat.Cmdable
}

func NewEngine(database *db.DB, compSvc *comparison.Service, r *resolver.Registry, settings *config.Settings, vk valkeycompat.Cmdable) *Engine {
    return &Engine{
        read:     gen.New(database.Reader),
        write:    gen.New(database.Writer),
        compSvc:  compSvc,
        r:        r,
        settings: settings,
        vk:       vk,
    }
}

func (e *Engine) Score(ctx context.Context, input jce.ScoreInput) (jce.ScoreResult, error)
func (e *Engine) GetScore(ctx context.Context, input jce.GetScoreInput) (jce.ScoreResult, error)
func (e *Engine) PreviewScoreForItems(ctx context.Context, input jce.ScorePreviewInput) (jce.ScoreResult, error)
```

- [ ] **Step 2: Implement `Score` with the following exact algorithm**

```go
func (e *Engine) Score(ctx context.Context, input jce.ScoreInput) (jce.ScoreResult, error) {
    // 1. Load content, metadata, type, and visibility check.
    // 2. Resolve profile (load from DB/Valkey or build from type defaults).
    // 3. Load comparison items, dimensions, and values.
    // 4. Load score factors for the comparison type (with global fallback).
    // 5. Build weight map:
    //    - Start with type config default_weights.
    //    - Apply profile preferences.
    //    - Apply goal_factor_map boosts (add goal weight to mapped factors).
    //    - Apply smart_mode multipliers.
    // 6. Resolve every item_ref to get ResolvedItem.Metadata["score_inputs"].
    // 7. For each item × factor, compute a 0–100 factor score:
    //    - data_source='dimension': lookup dimension value by factor.dimension_slug and normalize.
    //    - data_source='resolver_metadata': read ResolvedItem.Metadata["score_inputs"][factor.config.value_key].
    //    - data_source='profile': compare profile field (factor.config.profile_key) against item field (factor.config.item_key) using factor.config.match_strategy.
    //    - data_source='eligibility': evaluate type config eligibility_rules.
    //    - data_source='aggregate' (japa_score): computed as weighted sum.
    // 8. Missing inputs use factor.config.missing_score (default 0).
    // 9. Compute JapaScore as weighted average of factor scores; store as a comparison_scores row for the 'japa_score' factor.
    // 10. Generate recommendations from type config recommendation_templates.
    // 11. Upsert factor scores and recommendations.
    // 12. Update comparison_content_metadata.config with profile_id, weights_snapshot, recommendation_summary, last_scored_at and increment score_version.
    // 13. Cache the ScoreResult in Valkey at `jc:jce:score:{tenant_id}:{content_id}:{score_version}` with TTL from `jce.score_cache_ttl_minutes`.
    // 14. Return ScoreResult.
}
```

- [ ] **Step 3: Implement `GetScore`**

```go
func (e *Engine) GetScore(ctx context.Context, input jce.GetScoreInput) (jce.ScoreResult, error) {
    // 1. Load metadata and read config.profile_id.
    // 2. Build Valkey cache key `jc:jce:score:{tenant_id}:{content_id}:{metadata.score_version}`.
    // 3. If cached, unmarshal and return.
    // 4. Otherwise, load comparison_scores and comparison_recommendations and assemble a ScoreResult.
    // 5. Return from DB; do not recalculate unless cache miss and DB has no rows.
}
```

- [ ] **Step 4: Implement `PreviewScoreForItems`**

```go
func (e *Engine) PreviewScoreForItems(ctx context.Context, input jce.ScorePreviewInput) (jce.ScoreResult, error) {
    // 1. Load comparison type config by input.TypeID.
    // 2. Resolve each input.ItemRefs through resolver.Registry to build a list of ResolvedItem.
    // 3. Build a synthetic profile from input.ProfileInput or type defaults.
    // 4. Run the same scoring pipeline as Score, but do not persist to comparison_scores/recommendations or metadata.
    // 5. Return ScoreResult with content_id="" and version=0.
}
```

- [ ] **Step 5: Implement helper functions in the same package**

```go
func normalizeNumber(value, min, max float64, direction string) float64
func compareProfileItem(profileVal, itemVal any, strategy string) float64
func evaluateEligibilityRules(profile jce.ComparisonProfile, inputs map[string]any, rules []map[string]any) (status string, score float64, explanation string)
func buildWeightMap(typeConfig, profile jce.ComparisonProfile, smartMode string) map[string]float64
func generateRecommendations(itemID, factorKey string, factorScore float64, eligibleStatus string, templates []map[string]any) []jce.ComparisonRecommendation
```

- [ ] **Step 6: Write `internal/jce/scoring/engine_test.go`**

Test functions:
- `TestEngine_NumberFactorHigherIsBetter`
- `TestEngine_NumberFactorLowerIsBetter`
- `TestEngine_MissingInputUsesMissingScore`
- `TestEngine_ProfileMatchExact`
- `TestEngine_EligibilityRuleIneligible`
- `TestEngine_SmartModeCostMultiplier`
- `TestEngine_JapaScoreWeightedAverage`
- `TestEngine_RecommendationTemplateScoreLt`

**Verification:** `go test ./internal/jce/scoring/...` and `go build ./...` pass.

---

### Task 5: JCE service orchestration in `internal/jce/service.go`

**Files:**
- Create `internal/jce/service.go`
- Modify `internal/comparison/jce_service.go` (add `SmartMode` and `Config` to `CreateJCEComparisonInput` and `createJCEContentAndMetadata`)
- Modify `internal/comparison/jce_handlers.go` (add `smart_mode` and `config` to `createJCEComparisonBody`)
- Create `internal/jce/service_test.go`

**Interfaces:**
- Consumes `internal/jce.profileStore`, `internal/jce.scoreFactorStore`, `internal/jce.recommendationStore`, `internal/jce.scoring.Engine`, `internal/comparison.Service`, `internal/comparison/resolver.Registry`, `internal/config.Settings`, `valkeycompat.Cmdable`.
- Produces `*jce.Service` with the exact method signatures from the spec.

**Depends on:** Tasks 1–4.

- [ ] **Step 1: Define `internal/jce/service.go` with the spec signatures**

```go
package jce

import (
    "context"
    "encoding/json"
    "fmt"

    "github.com/jackc/pgx/v5/pgtype"
    "github.com/jedafe/japa/internal/comparison"
    "github.com/jedafe/japa/internal/comparison/resolver"
    "github.com/jedafe/japa/internal/config"
    "github.com/jedafe/japa/internal/db"
    "github.com/valkey-io/valkey-go/valkeycompat"
)

type Service struct {
    db       *db.DB
    compSvc  *comparison.Service
    r        *resolver.Registry
    settings *config.Settings
    vk       valkeycompat.Cmdable
    profiles *profileStore
    factors  *scoreFactorStore
    recs     *recommendationStore
    engine   *scoring.Engine
}

func NewService(
    database *db.DB,
    compSvc *comparison.Service,
    r *resolver.Registry,
    settings *config.Settings,
    vk valkeycompat.Cmdable,
) *Service {
    return &Service{
        db:       database,
        compSvc:  compSvc,
        r:        r,
        settings: settings,
        vk:       vk,
        profiles: newProfileStore(database),
        factors:  newScoreFactorStore(database),
        recs:     newRecommendationStore(database),
        engine:   scoring.NewEngine(database, compSvc, r, settings, vk),
    }
}

// Profiles
func (s *Service) CreateComparisonProfile(ctx context.Context, tenantID, userID pgtype.UUID, in ComparisonProfileInput) (ComparisonProfile, error)
func (s *Service) ListComparisonProfiles(ctx context.Context, tenantID, userID pgtype.UUID, includeTemplates bool) ([]ComparisonProfile, error)
func (s *Service) GetComparisonProfile(ctx context.Context, id, tenantID, userID pgtype.UUID) (ComparisonProfile, error)
func (s *Service) UpdateComparisonProfile(ctx context.Context, id, tenantID, userID pgtype.UUID, in ComparisonProfileInput) (ComparisonProfile, error)
func (s *Service) DeleteComparisonProfile(ctx context.Context, id, tenantID, userID pgtype.UUID) error
func (s *Service) SetDefaultComparisonProfile(ctx context.Context, id, tenantID, userID pgtype.UUID) error
func (s *Service) GetProfileStats(ctx context.Context, tenantID, userID pgtype.UUID) ([]ProfileStat, error)

// Score factors
func (s *Service) ListScoreFactors(ctx context.Context, tenantID, comparisonTypeID pgtype.UUID, includeGlobal bool) ([]ScoreFactor, error)
func (s *Service) CreateScoreFactor(ctx context.Context, tenantID, userID pgtype.UUID, in ScoreFactorInput) (ScoreFactor, error)
func (s *Service) GetScoreFactor(ctx context.Context, id, tenantID pgtype.UUID) (ScoreFactor, error)
func (s *Service) UpdateScoreFactor(ctx context.Context, id, tenantID, userID pgtype.UUID, in ScoreFactorInput) (ScoreFactor, error)
func (s *Service) DeleteScoreFactor(ctx context.Context, id, tenantID, userID pgtype.UUID) error

// Scoring
func (s *Service) ScoreComparison(ctx context.Context, contentID, tenantID, userID pgtype.UUID, profileID *pgtype.UUID, force bool) (ScoreResult, error)
func (s *Service) GetComparisonScore(ctx context.Context, contentID, tenantID, userID pgtype.UUID, profileID *pgtype.UUID) (ScoreResult, error)
func (s *Service) PreviewScoreForItems(ctx context.Context, tenantID, userID pgtype.UUID, in ScorePreviewInput) (ScoreResult, error)

// Recommendations
func (s *Service) ListRecommendations(ctx context.Context, contentID, tenantID, userID pgtype.UUID, profileID *pgtype.UUID, limit int) ([]ComparisonRecommendation, error)
```

- [ ] **Step 2: Implement profile CRUD with validation rules**

- `CreateComparisonProfile`:
  - Reject empty `Title`.
  - For non-template profiles, require at least one `TargetSpaceID`.
  - Validate every `Goals[].key` against `typeConfig.allowed_goals` when `in.SourceSpaceID`/`TypeID` is available; if no type is passed, validate against `jce.default_goals` from settings.
  - Validate `Preferences` keys exist as factor keys for the type.
  - If `IsDefault` is true, clear the flag on all other profiles for this user.
  - For templates, `user_id` is `NULL`; only admins with `comparison.edit_any` may create templates.
- `ListComparisonProfiles` returns user-owned non-deleted profiles plus all `is_template=true` templates for the tenant.
- `GetComparisonProfile` returns the profile if the caller owns it, it is a template, or the caller has `comparison.edit_any`.
- `UpdateComparisonProfile` enforces the same ownership rules and partial updates.
- `DeleteComparisonProfile` soft-deletes by setting `deleted_at = now()`.
- `SetDefaultComparisonProfile` calls `profileStore.setDefault`.
- `GetProfileStats` calls `CountComparisonProfilesBySourceSpace`.

- [ ] **Step 3: Implement score-factor CRUD**

- `ListScoreFactors` lists type-specific factors plus global factors when `includeGlobal` is true. A `nil` `comparisonTypeID` means global.
- `CreateScoreFactor` validates `data_source` in the allowed set, `default_weight` between 0 and 1, and that `factor_key` is unique for `(tenant_id, comparison_type_id)`.
- `GetScoreFactor` returns by `id` and `tenant_id`.
- `UpdateScoreFactor` rejects writes to `is_system=true` factors unless the caller has `comparison.edit_any`.
- `DeleteScoreFactor` rejects deletion of `is_system=true` factors.

- [ ] **Step 4: Implement scoring orchestration**

- `ScoreComparison` checks visibility (owner or `comparison.edit_any` for quick; public detailed can be scored by any authenticated user with `jce.score.recalculate`), then calls `engine.Score`.
- `GetComparisonScore` checks `jce.score.view` and visibility, then calls `engine.GetScore`.
- `PreviewScoreForItems` checks `jce.score.view` and `feature.jce.smart_compare`, then calls `engine.PreviewScoreForItems`.
- `ListRecommendations` checks `jce.recommendation.view` and visibility, then reads from `recommendationStore` or falls back to calling `engine.Score`.

- [ ] **Step 5: Extend `internal/comparison.CreateJCEComparisonInput` to carry Smart Compare state**

In `internal/comparison/jce_service.go`:

```go
type CreateJCEComparisonInput struct {
    Title      string
    Body       string
    Visibility string
    Status     string
    SpaceID    pgtype.UUID
    TypeID     pgtype.UUID
    Depth      string
    Items      []JCEItemInput
    Dimensions []string
    SmartMode  string // 'none' | 'eligibility' | 'cost' | 'pathway'
    Config     []byte // JSON metadata config (e.g. profile_id)
}
```

Then update `createJCEContentAndMetadata` to use `input.SmartMode` and `input.Config` instead of hard-coding `SmartModeNone` and `[]byte("{}")`.

- [ ] **Step 6: Update `internal/comparison/jce_handlers.go` `createJCEComparisonBody`**

```go
type createJCEComparisonBody struct {
    Title      string        `json:"title"`
    Body       string        `json:"body"`
    SpaceID    string        `json:"space_id"`
    TypeID     string        `json:"type_id"`
    Depth      string        `json:"depth"`
    Items      []jceItemBody `json:"items"`
    Dimensions []string      `json:"dimensions"`
    SmartMode  string        `json:"smart_mode"`
    Config     any           `json:"config"`
}
```

In `CreateJCEComparison`, marshal `body.Config` to `[]byte` and pass `SmartMode` to `CreateJCEComparisonInput`.

- [ ] **Step 7: Write `internal/jce/service_test.go`**

Test functions:
- `TestService_ProfileCRUDAndDefault`
- `TestService_ScoreFactorCRUD`
- `TestService_ScoreComparison`
- `TestService_PreviewScoreForItems`
- `TestService_ListRecommendations`

**Verification:** `go test ./internal/jce/...` and `go build ./...` pass.

---

### Task 6: Handlers and routes in `internal/jce/handlers.go` and `internal/server/server.go`

**Files:**
- Create `internal/jce/handlers.go`
- Modify `internal/server/server.go`

**Interfaces:**
- Consumes `*jce.Service`, `auth`, `tenant`, `features`, `rbac` middleware.
- Produces HTTP routes under `/api/v1/jce/*`.

**Depends on:** Task 5.

- [ ] **Step 1: Create `internal/jce/handlers.go`**

```go
package jce

import (
    "encoding/json"
    "net/http"
    "strconv"

    "github.com/go-chi/chi/v5"
    "github.com/jackc/pgx/v5"
    "github.com/jackc/pgx/v5/pgtype"
    "github.com/jedafe/japa/internal/auth"
    "github.com/jedafe/japa/internal/rbac"
    errpkg "github.com/jedafe/japa/internal/errors"
    "github.com/jedafe/japa/internal/tenant"
)

type Handlers struct {
    service *Service
}

func NewHandlers(service *Service) *Handlers { return &Handlers{service: service} }

func requireTenant(w http.ResponseWriter, r *http.Request) (pgtype.UUID, bool) {
    t := tenant.FromContext(r.Context())
    if t == nil {
        errpkg.WriteError(w, http.StatusServiceUnavailable, "tenant_unavailable", "No tenant available")
        return pgtype.UUID{}, false
    }
    return t.ID, true
}

func requireUser(w http.ResponseWriter, r *http.Request) (*auth.AuthenticatedUser, bool) {
    u := auth.FromContext(r.Context())
    if u == nil {
        errpkg.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
        return nil, false
    }
    return u, true
}

func hasPermission(r *http.Request, user *auth.AuthenticatedUser, perm string) bool {
    resolver := rbac.FromContext(r.Context())
    if resolver == nil || user == nil {
        return false
    }
    return resolver.HasPermission(r.Context(), user.UserID, user.TenantID, perm)
}

func (h *Handlers) ListProfiles(w http.ResponseWriter, r *http.Request)
func (h *Handlers) CreateProfile(w http.ResponseWriter, r *http.Request)
func (h *Handlers) GetProfile(w http.ResponseWriter, r *http.Request)
func (h *Handlers) UpdateProfile(w http.ResponseWriter, r *http.Request)
func (h *Handlers) DeleteProfile(w http.ResponseWriter, r *http.Request)
func (h *Handlers) SetDefaultProfile(w http.ResponseWriter, r *http.Request)
func (h *Handlers) GetProfileStats(w http.ResponseWriter, r *http.Request)

func (h *Handlers) ListScoreFactors(w http.ResponseWriter, r *http.Request)
func (h *Handlers) CreateScoreFactor(w http.ResponseWriter, r *http.Request)
func (h *Handlers) GetScoreFactor(w http.ResponseWriter, r *http.Request)
func (h *Handlers) UpdateScoreFactor(w http.ResponseWriter, r *http.Request)
func (h *Handlers) DeleteScoreFactor(w http.ResponseWriter, r *http.Request)

func (h *Handlers) ScoreComparison(w http.ResponseWriter, r *http.Request)
func (h *Handlers) GetComparisonScore(w http.ResponseWriter, r *http.Request)
func (h *Handlers) ListRecommendations(w http.ResponseWriter, r *http.Request)
func (h *Handlers) PreviewScoreForItems(w http.ResponseWriter, r *http.Request)
```

- [ ] **Step 2: Implement route wiring in `internal/server/server.go`**

Inside the existing `/api/v1/jce` route group (`internal/server/server.go` lines 3339–3409), after the value routes, add:

```go
// JCE Sub-spec 2/3: profiles
r.With(auth.RequireAuth(kr, vkClient.Redis()), features.Gate(featureChecker, "feature.jce.profiles"), rbac.RequirePermission(rbacResolver, "jce.profile.list")).
    Get("/jce/profiles", jceHandlers.ListProfiles)
r.With(auth.RequireAuth(kr, vkClient.Redis()), features.Gate(featureChecker, "feature.jce.profiles"), rbac.RequirePermission(rbacResolver, "jce.profile.create")).
    Post("/jce/profiles", jceHandlers.CreateProfile)
r.With(auth.RequireAuth(kr, vkClient.Redis()), features.Gate(featureChecker, "feature.jce.profiles"), rbac.RequirePermission(rbacResolver, "jce.profile.list")).
    Get("/jce/profiles/stats", jceHandlers.GetProfileStats)
r.With(auth.RequireAuth(kr, vkClient.Redis()), features.Gate(featureChecker, "feature.jce.profiles"), rbac.RequirePermission(rbacResolver, "jce.profile.view")).
    Get("/jce/profiles/{id}", jceHandlers.GetProfile)
r.With(auth.RequireAuth(kr, vkClient.Redis()), features.Gate(featureChecker, "feature.jce.profiles"), rbac.RequirePermission(rbacResolver, "jce.profile.update")).
    Patch("/jce/profiles/{id}", jceHandlers.UpdateProfile)
r.With(auth.RequireAuth(kr, vkClient.Redis()), features.Gate(featureChecker, "feature.jce.profiles"), rbac.RequirePermission(rbacResolver, "jce.profile.delete")).
    Delete("/jce/profiles/{id}", jceHandlers.DeleteProfile)
r.With(auth.RequireAuth(kr, vkClient.Redis()), features.Gate(featureChecker, "feature.jce.profiles"), rbac.RequirePermission(rbacResolver, "jce.profile.update")).
    Post("/jce/profiles/{id}/default", jceHandlers.SetDefaultProfile)

// JCE Sub-spec 2/3: score factors
r.With(auth.RequireAuth(kr, vkClient.Redis()), features.Gate(featureChecker, "feature.jce.scoring"), rbac.RequirePermission(rbacResolver, "jce.factor.manage")).
    Get("/jce/comparison-types/{id}/score-factors", jceHandlers.ListScoreFactors)
r.With(auth.RequireAuth(kr, vkClient.Redis()), features.Gate(featureChecker, "feature.jce.scoring"), rbac.RequirePermission(rbacResolver, "jce.factor.manage")).
    Post("/jce/comparison-types/{id}/score-factors", jceHandlers.CreateScoreFactor)
r.With(auth.RequireAuth(kr, vkClient.Redis()), features.Gate(featureChecker, "feature.jce.scoring"), rbac.RequirePermission(rbacResolver, "jce.factor.manage")).
    Get("/jce/score-factors/{id}", jceHandlers.GetScoreFactor)
r.With(auth.RequireAuth(kr, vkClient.Redis()), features.Gate(featureChecker, "feature.jce.scoring"), rbac.RequirePermission(rbacResolver, "jce.factor.manage")).
    Patch("/jce/score-factors/{id}", jceHandlers.UpdateScoreFactor)
r.With(auth.RequireAuth(kr, vkClient.Redis()), features.Gate(featureChecker, "feature.jce.scoring"), rbac.RequirePermission(rbacResolver, "jce.factor.manage")).
    Delete("/jce/score-factors/{id}", jceHandlers.DeleteScoreFactor)

// JCE Sub-spec 2/3: scoring and recommendations
r.With(auth.RequireAuth(kr, vkClient.Redis()), features.Gate(featureChecker, "feature.jce.scoring"), rbac.RequirePermission(rbacResolver, "jce.score.recalculate")).
    Post("/jce/comparisons/{id}/score", jceHandlers.ScoreComparison)
r.With(auth.RequireAuth(kr, vkClient.Redis()), features.Gate(featureChecker, "feature.jce.scoring"), rbac.RequireAnyPermission(rbacResolver, "jce.score.view", "jce.score.recalculate")).
    Get("/jce/comparisons/{id}/score", jceHandlers.GetComparisonScore)
r.With(auth.RequireAuth(kr, vkClient.Redis()), features.Gate(featureChecker, "feature.jce.scoring"), rbac.RequirePermission(rbacResolver, "jce.recommendation.view")).
    Get("/jce/comparisons/{id}/recommendations", jceHandlers.ListRecommendations)

// JCE Sub-spec 2/3: score preview
r.With(auth.RequireAuth(kr, vkClient.Redis()), features.Gate(featureChecker, "feature.jce.smart_compare"), rbac.RequirePermission(rbacResolver, "jce.score.view")).
    Post("/jce/score-preview", jceHandlers.PreviewScoreForItems)
```

- [ ] **Step 3: Add `jceHandlers` construction in `internal/server/server.go` service wiring**

Near the `comparisonService` and `jceResolver` wiring (around lines 704 and 1458–1492), add:

```go
jceService := jce.NewService(database, comparisonService, jceResolver, settings, vkClient.Redis())
jceHandlers := jce.NewHandlers(jceService)
```

Pass `jceHandlers` into the route closure.

- [ ] **Step 4: Write `internal/jce/handlers_test.go`**

Test each route with the existing `internal/server/test` helpers or a `httptest` `Server`. Include route-level tests for 404 when feature flags are disabled.

**Verification:** `go build ./...`, `make vet`, and `go test ./internal/jce/...` pass.

---

### Task 7: Resolver metadata enrichment (`ResolvedItem.Metadata.score_inputs`)

**Files:**
- Modify `internal/comparison/resolver/space.go`
- Modify `internal/comparison/resolver/university.go`
- Modify `internal/comparison/resolver/course.go`
- Modify `internal/comparison/resolver/visa.go`
- Modify `internal/comparison/resolver/city.go`
- Modify `internal/comparison/resolver/scholarship.go`
- Modify `internal/comparison/resolver/languagetest.go`
- Modify `internal/comparison/resolver/mentor.go`
- Modify `internal/comparison/resolver/job_listing.go`
- Modify `internal/comparison/resolver/organization.go` (or `db.go` for generic `organization`/`bank`/`remittance`/`travel`)
- Modify `internal/comparison/resolver/content.go`
- Create or modify `internal/comparison/resolver/resolver_test.go`

**Interfaces:**
- Consumes existing resolver service methods.
- Produces `ResolvedItem.Metadata["score_inputs"]` map for every resolver.

**Depends on:** Task 1 (schema). Can run in parallel with Task 5/6.

- [ ] **Step 1: Add `score_inputs` map to `space.go`**

In `spaceMetadata` (and all `spaceItemFrom*` helpers), add:

```go
meta["score_inputs"] = map[string]any{
    "cost":            nil,
    "income":          nil,
    "speed":           nil,
    "pr_path_ease":    nil,
    "safety_index":    nil,
    "healthcare_index": nil,
    "language":        []string{},
    "geo_space_id":    id.String(),
}
```

Populate values from the existing space fields when available (e.g. cost index, average salary, safety index, healthcare index, official language array); use `nil` for missing values.

- [ ] **Step 2: Add `score_inputs` map to `university.go`**

In `makeSpaceResolvedItem`:

```go
meta["score_inputs"] = map[string]any{
    "cost":             nil, // tuition/year
    "income":           nil, // graduate salary
    "acceptance_rate":  nil,
    "duration_years":   nil,
    "qs_ranking":       nil,
    "pr_path_ease":     nil,
    "post_study_work_months": nil,
    "language":         []string{},
    "accreditation":    []string{},
    "fields_of_study":  []string{},
    "geo_space_id":     id.String(),
}
```

- [ ] **Step 3: Add `score_inputs` map to `course.go`, `visa.go`, `city.go`, `scholarship.go`, `languagetest.go`, `mentor.go`, `job_listing.go`, `organization.go`, `content.go`**

Use the exact key sets from the spec §12:

| Resolver | `score_inputs` keys |
|---|---|
| `space` (country/state/city) | `cost`, `income`, `speed`, `pr_path_ease`, `safety_index`, `healthcare_index`, `language`, `geo_space_id` |
| `university` | `cost`, `income`, `acceptance_rate`, `duration_years`, `qs_ranking`, `pr_path_ease`, `post_study_work_months`, `language`, `accreditation`, `fields_of_study`, `geo_space_id` |
| `course` | `cost`, `duration_weeks`, `income`, `certification`, `remote`, `prerequisite_level`, `language`, `field` |
| `visa_route` | `cost`, `income`, `speed`, `pr_path_ease`, `minimum_funds`, `work_allowed`, `pr_eligible`, `family_included`, `language`, `age_min`, `age_max`, `occupation_eligible`, `destination_geo_space_id` |
| `col_city` | `cost`, `rent`, `groceries`, `transport`, `utilities`, `income`, `language`, `safety_index`, `healthcare_index`, `geo_space_id` |
| `scholarship` | `cost`, `deadline_days`, `fields_of_study`, `country`, `institution`, `level`, `language` |
| `languagetest` | `cost`, `speed`, `validity_months`, `accepted_by`, `score_required`, `language` |
| `mentor` | `cost`, `rating`, `sessions_count`, `areas`, `language`, `availability_days` |
| `job_listing` | `income`, `cost`, `speed`, `contract_type`, `sponsorship`, `remote`, `required_experience_years`, `language`, `location_geo_space_id` |
| `organization` / `bank` | `cost`, `income`, `speed`, `atm_fee`, `min_balance`, `countries` |
| `content` (nested comparison) | `type_slug`, `child_japa_score`, `freshness_score` |

- [ ] **Step 4: Ensure missing values are `nil`**

If the underlying tool service does not provide a value, leave the key set to `nil`. The scoring engine uses `factor.config.missing_score`.

- [ ] **Step 5: Update `internal/comparison/resolver/resolver_test.go`**

Add `TestResolveScoreInputs` that iterates each registered resolver, resolves a known entity, and asserts that `metadata["score_inputs"]` is a `map[string]any` with the expected keys.

**Verification:** `go test ./internal/comparison/resolver/...` and `go build ./...` pass.

---

### Task 8: `apps/main` profile API client and types updates

**Files:**
- Modify `apps/main/src/lib/types/jce.ts`
- Modify `apps/main/src/lib/api/jce.ts`
- Modify `apps/main/src/lib/api/jce.test.ts`
- Modify `apps/main/src/lib/i18n/en.json`

**Interfaces:**
- Consumes `/api/v1/jce/profiles`, `/api/v1/jce/comparison-types/{id}/score-factors`, `/api/v1/jce/score-factors/{id}`, `/api/v1/jce/comparisons/{id}/score`, `/api/v1/jce/comparisons/{id}/recommendations`, `/api/v1/jce/score-preview`.
- Produces TypeScript types and API client functions used by Tasks 9–10.

**Depends on:** Task 6.

- [ ] **Step 1: Append types to `apps/main/src/lib/types/jce.ts`**

```ts
export interface ComparisonProfile {
    id: string;
    title: string;
    source_space_id?: string | null;
    target_space_ids: string[];
    goals: { key: string; weight: number }[];
    preferences: Record<string, number>;
    demographics: {
        age?: number;
        occupation?: string;
        family_status?: string;
        language_proficiency?: string;
        budget_min?: number;
        budget_max?: number;
        budget_currency?: string;
    };
    is_default: boolean;
    is_template: boolean;
    created_at?: string;
    updated_at?: string;
}

export interface ScoreFactor {
    id: string;
    comparison_type_id?: string | null;
    factor_key: string;
    label: string;
    description?: string;
    dimension_slug?: string | null;
    data_source: 'dimension' | 'resolver_metadata' | 'profile' | 'eligibility' | 'aggregate';
    default_weight: number;
    config: Record<string, any>;
    version: number;
    is_system: boolean;
}

export interface FactorScore {
    factor_key: string;
    label: string;
    score_value: number;
    max_value: number;
    weight: number;
    explanation: string;
}

export interface JapaScore {
    item_id: string;
    item_title: string;
    japa_score: number;
    max_value: number;
    eligible_status: 'eligible' | 'limited' | 'ineligible';
    factor_scores: FactorScore[];
    explanation: string;
}

export interface ComparisonRecommendation {
    id: string;
    item_id?: string | null;
    text: string;
    action_type?: string | null;
    action_url?: string | null;
    priority: number;
}

export interface ScoreResult {
    content_id: string;
    profile_id?: string | null;
    smart_mode: string;
    scores: JapaScore[];
    recommendations: ComparisonRecommendation[];
    weights_snapshot: Record<string, number>;
    recommendation_summary: {
        top_item_id: string;
        top_japa_score: number;
        recommendations_count: number;
        last_scored_at: string;
    };
    version: number;
}

export interface ScorePreviewInput {
    type_id: string;
    smart_mode: string;
    item_refs: Record<string, unknown>[];
    profile?: ComparisonProfileInput;
}

export interface ComparisonProfileInput {
    title?: string;
    source_space_id?: string | null;
    target_space_ids: string[];
    goals?: { key: string; weight: number }[];
    preferences?: Record<string, number>;
    demographics?: ComparisonProfile['demographics'];
    is_default?: boolean;
    is_template?: boolean;
}
```

- [ ] **Step 2: Append client functions to `apps/main/src/lib/api/jce.ts`**

```ts
export async function listProfiles(): Promise<ComparisonProfile[]> {
    const res = await api.get<ComparisonProfile[]>('/jce/profiles');
    return res.data ?? [];
}

export async function createProfile(body: ComparisonProfileInput): Promise<ComparisonProfile | null> {
    const res = await api.post<ComparisonProfile>('/jce/profiles', body);
    return res.data;
}

export async function getProfile(id: string): Promise<ComparisonProfile | null> {
    const res = await api.get<ComparisonProfile>(`/jce/profiles/${id}`);
    return res.data;
}

export async function updateProfile(id: string, body: ComparisonProfileInput): Promise<ComparisonProfile | null> {
    const res = await api.patch<ComparisonProfile>(`/jce/profiles/${id}`, body);
    return res.data;
}

export async function deleteProfile(id: string): Promise<void> {
    await api.delete<unknown>(`/jce/profiles/${id}`);
}

export async function setDefaultProfile(id: string): Promise<void> {
    await api.post<unknown>(`/jce/profiles/${id}/default`);
}

export async function listScoreFactors(typeID: string): Promise<ScoreFactor[]> {
    const res = await api.get<ScoreFactor[]>(`/jce/comparison-types/${typeID}/score-factors`);
    return res.data ?? [];
}

export async function getScore(comparisonID: string, profileID?: string): Promise<ScoreResult | null> {
    const qs = profileID ? `?profile_id=${encodeURIComponent(profileID)}` : '';
    const res = await api.get<ScoreResult>(`/jce/comparisons/${comparisonID}/score${qs}`);
    return res.data;
}

export async function scoreComparison(comparisonID: string, profileID?: string): Promise<ScoreResult | null> {
    const body = profileID ? { profile_id: profileID } : {};
    const res = await api.post<ScoreResult>(`/jce/comparisons/${comparisonID}/score`, body);
    return res.data;
}

export async function listRecommendations(comparisonID: string, profileID?: string): Promise<ComparisonRecommendation[]> {
    const qs = profileID ? `?profile_id=${encodeURIComponent(profileID)}` : '';
    const res = await api.get<ComparisonRecommendation[]>(`/jce/comparisons/${comparisonID}/recommendations${qs}`);
    return res.data ?? [];
}

export async function previewScore(input: ScorePreviewInput): Promise<ScoreResult | null> {
    const res = await api.post<ScoreResult>('/jce/score-preview', input);
    return res.data;
}
```

- [ ] **Step 3: Append the following i18n keys to `apps/main/src/lib/i18n/en.json`**

```json
{
    "jce.profile.title": "My Japa Profile",
    "jce.profile.create": "Create profile",
    "jce.profile.edit": "Edit profile",
    "jce.profile.select": "Select a profile",
    "jce.profile.default": "Default profile",
    "jce.profile.templates": "Templates",
    "jce.profile.title_label": "Profile name",
    "jce.profile.source_space": "Source country",
    "jce.profile.target_spaces": "Target countries / cities",
    "jce.profile.goals": "Goals",
    "jce.profile.demographics.age": "Age",
    "jce.profile.demographics.occupation": "Occupation",
    "jce.profile.demographics.family_status": "Family status",
    "jce.profile.demographics.language_proficiency": "Language proficiency",
    "jce.profile.demographics.budget_min": "Budget min",
    "jce.profile.demographics.budget_max": "Budget max",
    "jce.profile.demographics.budget_currency": "Currency",
    "jce.smart_mode.title": "Smart mode",
    "jce.smart_mode.none": "None",
    "jce.smart_mode.eligibility": "Eligibility",
    "jce.smart_mode.cost": "Cost",
    "jce.smart_mode.pathway": "Pathway",
    "jce.score.title": "JapaScore",
    "jce.score.best_match": "Best match",
    "jce.score.ineligible": "Ineligible",
    "jce.score.limited": "Limited",
    "jce.score.no_data": "No data",
    "jce.recommendations.title": "Recommendations",
    "jce.recommendations.empty": "No recommendations yet."
}
```

- [ ] **Step 4: Add client tests to `apps/main/src/lib/api/jce.test.ts`**

Add tests for `listProfiles`, `createProfile`, `getScore`, and `previewScore` with `vi.mock` on `../api/client`.

**Verification:** `bun run check` and `bun run test` pass in `apps/main`.

---

### Task 9: `apps/main` `/compare` profile drawer and smart compare

**Files:**
- Create `apps/main/src/lib/components/compare/ProfileDrawer.svelte`
- Create `apps/main/src/lib/components/compare/SmartToggle.svelte`
- Modify `apps/main/src/routes/compare/+page.server.ts`
- Modify `apps/main/src/routes/compare/+page.svelte`
- Modify `apps/main/src/lib/types/jce.ts` (already in Task 8)
- Modify `apps/main/src/lib/api/jce.ts` (already in Task 8)

**Interfaces:**
- Consumes `GET /api/v1/jce/profiles` and `POST /api/v1/jce/comparisons`.
- Produces profile drawer UI and smart-mode selection in the `/compare` picker.

**Depends on:** Tasks 6–8.

- [ ] **Step 1: Update `apps/main/src/routes/compare/+page.server.ts`**

After the existing `myComparisons` load, add:

```ts
let profiles: ComparisonProfile[] = [];
if (event.locals.user) {
    try {
        const res = await apiGet<ComparisonProfile[]>(ctx, '/jce/profiles');
        profiles = res ?? [];
    } catch (err) {
        // Feature may be disabled; ignore 404 feature_disabled.
        if (err instanceof ApiError && err.status !== 404) {
            loadError = loadError || 'jce.compare.errors.load_profiles';
            loadErrorParams = { message: err.message };
        }
    }
}

let defaultProfile = profiles.find((p) => p.is_default) ?? profiles[0] ?? null;

return {
    // (return existing fields alongside the new ones)
    profiles,
    defaultProfile
};
```

- [ ] **Step 2: Update `apps/main/src/routes/compare/+page.svelte`**

Add near the action buttons:

```svelte
<script lang="ts">
    import { listProfiles, createProfile, setDefaultProfile } from '$lib/api/jce';
    import ProfileDrawer from '$lib/components/compare/ProfileDrawer.svelte';
    import SmartToggle from '$lib/components/compare/SmartToggle.svelte';

    let profiles = $derived(data.profiles ?? []);
    let activeProfile = $state<ComparisonProfile | null>(data.defaultProfile ?? null);
    let smartMode = $state<string>('none');
    let profileOpen = $state(false);

    // When calling createQuickComparison, include smart_mode and config.profile_id in the request body.
</script>
```

In the markup, add:

```svelte
<div class="flex items-center gap-2">
    <SmartToggle bind:mode={smartMode} />
    <button type="button" onclick={() => profileOpen = true}>
        {t('jce.profile.title')}
    </button>
</div>

<ProfileDrawer bind:open={profileOpen} {profiles} bind:activeProfile onsave={(p) => { activeProfile = p; profileOpen = false; }} />
```

- [ ] **Step 3: Implement `ProfileDrawer.svelte`**

```svelte
<script lang="ts">
    import type { ComparisonProfile, ComparisonProfileInput } from '$lib/types/jce';
    import { createProfile, updateProfile } from '$lib/api/jce';
    import { t } from '$lib/i18n';

    let { open = $bindable(), profiles, activeProfile = $bindable(), onsave }: Props = $props();

    type Props = {
        open: boolean;
        profiles: ComparisonProfile[];
        activeProfile: ComparisonProfile | null;
        onsave?: (p: ComparisonProfile) => void;
    };

    // Form state for create/edit with fields matching ComparisonProfileInput.
    // Save via createProfile() or updateProfile().
    // Selecting a profile sets activeProfile and calls onsave.
</script>
```

Use `t('jce.profile.*')` for all labels and `Bits UI` `Dialog` or `Sheet` for the drawer.

- [ ] **Step 4: Implement `SmartToggle.svelte`**

```svelte
<script lang="ts">
    import { t } from '$lib/i18n';
    let { mode = $bindable() }: { mode: string } = $props();
    const modes = ['none', 'eligibility', 'cost', 'pathway'];
</script>

<div class="flex gap-1 rounded border p-1">
    {#each modes as m}
        <button
            type="button"
            class:font-bold={mode === m}
            onclick={() => mode = m}
        >
            {t(`jce.smart_mode.${m}`)}
        </button>
    {/each}
</div>
```

- [ ] **Step 5: Pass `smart_mode` and `config.profile_id` when creating the comparison**

Update the `createQuickComparison` call in `+page.svelte`:

```ts
const cmp = await createQuickComparison({
    title: selectedItems.map((i) => i.title).join(t('jce.compare.title_separator')),
    type_id: selectedType.id,
    depth: 'quick',
    status: 'draft',
    visibility: 'private',
    items: selectedItems.map((it, i) => ({
        title: it.title,
        description: it.description ?? '',
        item_ref: it
    })),
    dimensions: [],
    smart_mode: smartMode,
    config: { profile_id: activeProfile?.id ?? null }
});
```

- [ ] **Step 6: Add `smart_mode` and `config` support to `createQuickComparison` in `apps/main/src/lib/api/jce.ts`**

Update the `CreateQuickComparisonInput` type to include `smart_mode?: string` and `config?: Record<string, unknown>`.

**Verification:** `bun run check` and `bun run build` pass in `apps/main`.

---

### Task 10: `apps/main` `/compare/[slug]` score/recommendation UI

**Files:**
- Create `apps/main/src/lib/components/compare/ScoreBadge.svelte`
- Create `apps/main/src/lib/components/compare/ScoreExplanation.svelte`
- Create `apps/main/src/lib/components/compare/RecommendationPanel.svelte`
- Modify `apps/main/src/routes/compare/[slug]/+page.server.ts`
- Modify `apps/main/src/routes/compare/[slug]/+page.svelte`
- Modify `apps/main/src/lib/i18n/en.json`

**Interfaces:**
- Consumes `GET /api/v1/jce/comparisons/{slug}/score` and `GET /api/v1/jce/comparisons/{slug}/recommendations`.
- Produces score badges, per-factor explanations, and recommendation panel on the quick preview.

**Depends on:** Tasks 6–9.

- [ ] **Step 1: Update `apps/main/src/routes/compare/[slug]/+page.server.ts`**

After loading the comparison and matrix, add:

```ts
let scoreResult: ScoreResult | null = null;
let recommendations: ComparisonRecommendation[] = [];
if (event.locals.user && comparison.metadata.smart_mode && comparison.metadata.smart_mode !== 'none') {
    try {
        const profileID = (comparison.metadata?.config as Record<string, any> | undefined)?.profile_id as string | undefined;
        scoreResult = await apiGet<ScoreResult>(ctx, `/jce/comparisons/${slug}/score?profile_id=${encodeURIComponent(profileID ?? '')}`);
        recommendations = await apiGet<ComparisonRecommendation[]>(ctx, `/jce/comparisons/${slug}/recommendations?profile_id=${encodeURIComponent(profileID ?? '')}`);
    } catch {
        // Allow the page to render without scores.
    }
}

return {
    // (return existing fields alongside the new ones)
    scoreResult,
    recommendations
};
```

- [ ] **Step 2: Update `apps/main/src/routes/compare/[slug]/+page.svelte`**

```svelte
<script lang="ts">
    import { scoreComparison, listRecommendations } from '$lib/api/jce';
    import ScoreBadge from '$lib/components/compare/ScoreBadge.svelte';
    import ScoreExplanation from '$lib/components/compare/ScoreExplanation.svelte';
    import RecommendationPanel from '$lib/components/compare/RecommendationPanel.svelte';
    import SmartToggle from '$lib/components/compare/SmartToggle.svelte';
    import ProfileDrawer from '$lib/components/compare/ProfileDrawer.svelte';

    let comparison = $derived(data.comparison);
    let scoreResult = $state(data.scoreResult ?? null);
    let recommendations = $state(data.recommendations ?? []);
    let smartMode = $state(comparison.metadata.smart_mode ?? 'none');
    let activeProfile = $state<ComparisonProfile | null>(null);

    async function refreshScore() {
        const res = await scoreComparison(comparison.content.id, activeProfile?.id);
        if (res) {
            scoreResult = res;
            recommendations = res.recommendations;
        }
    }

    // Sort matrix items by japa_score descending when scoreResult is present.
    let sortedItems = $derived(scoreResult
        ? data.matrix.slice().sort((a, b) => {
            const sa = scoreResult.scores.find((s) => s.item_id === a.id)?.japa_score ?? 0;
            const sb = scoreResult.scores.find((s) => s.item_id === b.id)?.japa_score ?? 0;
            return sb - sa;
        })
        : data.matrix);
</script>
```

- [ ] **Step 3: Implement `ScoreBadge.svelte`**

```svelte
<script lang="ts">
    import type { JapaScore } from '$lib/types/jce';
    let { score, best }: { score: JapaScore; best: boolean } = $props();
</script>

<div class="relative inline-flex items-center gap-1 rounded px-2 py-1 text-sm font-semibold"
     class:bg-green-100={best && score.eligible_status === 'eligible'}
     class:bg-gray-100={!best}
     class:opacity-50={score.eligible_status === 'ineligible'}>
    <span>{Math.round(score.japa_score)}</span>
    {#if best}<span class="text-xs">{t('jce.score.best_match')}</span>{/if}
    {#if score.eligible_status === 'ineligible'}<span>{t('jce.score.ineligible')}</span>{/if}
    {#if score.eligible_status === 'limited'}<span>{t('jce.score.limited')}</span>{/if}
</div>
```

- [ ] **Step 4: Implement `ScoreExplanation.svelte`**

```svelte
<script lang="ts">
    import type { JapaScore } from '$lib/types/jce';
    import { t } from '$lib/i18n';
    let { score }: { score: JapaScore } = $props();
    let expanded = $state(false);
</script>

<button type="button" class="text-sm" onclick={() => expanded = !expanded}>
    {expanded ? '▾' : '▸'} {t('jce.score.title')}
</button>
{#if expanded}
    <div class="space-y-1 text-sm">
        {#each score.factor_scores as fs}
            <div class="flex justify-between">
                <span>{fs.label}</span>
                <span class={fs.score_value >= 0 ? 'text-green-600' : 'text-red-600'}>
                    {fs.explanation || `${Math.round(fs.score_value)}/${Math.round(fs.max_value)}`}
                </span>
            </div>
        {/each}
    </div>
{/if}
```

- [ ] **Step 5: Implement `RecommendationPanel.svelte`**

```svelte
<script lang="ts">
    import type { ComparisonRecommendation } from '$lib/types/jce';
    import { t } from '$lib/i18n';
    let { recommendations }: { recommendations: ComparisonRecommendation[] } = $props();
</script>

<div class="rounded border p-3">
    <h3 class="font-semibold">{t('jce.recommendations.title')}</h3>
    {#if recommendations.length === 0}
        <p class="text-sm text-muted">{t('jce.recommendations.empty')}</p>
    {:else}
        <ul class="space-y-2">
            {#each recommendations.slice(0, 5) as rec}
                <li class="flex items-start gap-2">
                    <span class="text-sm">{rec.text}</span>
                    {#if rec.action_url}
                        <a href={rec.action_url} class="text-sm underline">{rec.action_type ?? t('jce.common.view')}</a>
                    {/if}
                </li>
            {/each}
        </ul>
    {/if}
</div>
```

- [ ] **Step 6: Render score components in the matrix header row in `[slug]/+page.svelte`**

For each `item` in `sortedItems`, find `scoreResult.scores.find((s) => s.item_id === item.id)` and render `<ScoreBadge>` and `<ScoreExplanation>`. Add the `RecommendationPanel` below the matrix.

- [ ] **Step 7: Add i18n keys to `apps/main/src/lib/i18n/en.json`** (if not already from Task 8):

```json
{
    "jce.compare.score.refresh": "Refresh score",
    "jce.compare.score.loading": "Scoring…",
    "jce.compare.errors.load_score": "Could not load scores: {message}",
    "jce.compare.errors.load_recommendations": "Could not load recommendations: {message}"
}
```

**Verification:** `bun run check` and `bun run build` pass in `apps/main`.

---

### Task 11: `apps/admin` `/comparison-profiles` and score-factor editor

**Files:**
- Create `apps/admin/src/routes/comparison-profiles/+page.svelte`
- Create `apps/admin/src/routes/comparison-profiles/+page.server.ts`
- Modify `apps/admin/src/lib/api.ts`
- Modify `apps/admin/src/routes/comparison-types/+page.svelte`
- Create or modify admin i18n keys in `apps/admin/src/lib/stores/translations.ts` or use `t(key, fallback)`.

**Interfaces:**
- Consumes `GET/POST/PATCH/DELETE /api/v1/jce/profiles`, `GET/POST /api/v1/jce/comparison-types/{id}/score-factors`, `GET/PATCH/DELETE /api/v1/jce/score-factors/{id}`.
- Produces admin UI for managing profile templates and score factors.

**Depends on:** Tasks 6–8.

- [ ] **Step 1: Add `jceAdminApi` to `apps/admin/src/lib/api.ts`**

```ts
export interface ComparisonProfileAdmin {
    id: string;
    title: string;
    source_space_id?: string | null;
    target_space_ids: string[];
    goals: { key: string; weight: number }[];
    preferences: Record<string, number>;
    demographics: Record<string, any>;
    is_default: boolean;
    is_template: boolean;
}

export interface ScoreFactorAdmin {
    id: string;
    comparison_type_id?: string | null;
    factor_key: string;
    label: string;
    description?: string;
    dimension_slug?: string | null;
    data_source: string;
    default_weight: number;
    config: Record<string, any>;
    is_system: boolean;
}

export const jceAdminApi = {
    listProfiles: () => api<ComparisonProfileAdmin[]>('/jce/profiles'),
    createProfile: (body: Record<string, any>) => api<ComparisonProfileAdmin>('/jce/profiles', { method: 'POST', body }),
    updateProfile: (id: string, body: Record<string, any>) => api<ComparisonProfileAdmin>(`/jce/profiles/${id}`, { method: 'PATCH', body }),
    deleteProfile: (id: string) => api<unknown>(`/jce/profiles/${id}`, { method: 'DELETE' }),
    listScoreFactors: (typeID: string) => api<ScoreFactorAdmin[]>(`/jce/comparison-types/${typeID}/score-factors`),
    createScoreFactor: (typeID: string, body: Record<string, any>) => api<ScoreFactorAdmin>(`/jce/comparison-types/${typeID}/score-factors`, { method: 'POST', body }),
    updateScoreFactor: (id: string, body: Record<string, any>) => api<ScoreFactorAdmin>(`/jce/score-factors/${id}`, { method: 'PATCH', body }),
    deleteScoreFactor: (id: string) => api<unknown>(`/jce/score-factors/${id}`, { method: 'DELETE' }),
    getProfileStats: () => api<{ source_space_id: string; profile_count: number }[]>('/jce/profiles/stats'),
};
```

- [ ] **Step 2: Implement `apps/admin/src/routes/comparison-profiles/+page.svelte`**

Use the `EntityPage` scaffold. Fields:

```ts
const fields: FieldDef[] = [
    { key: 'title', label: t('comparison_profiles.title', 'Title'), type: 'text', required: true },
    { key: 'source_space_id', label: t('comparison_profiles.source_space', 'Source country'), type: 'text' },
    { key: 'target_space_ids', label: t('comparison_profiles.target_spaces', 'Target spaces'), type: 'text' },
    { key: 'goals', label: t('comparison_profiles.goals', 'Goals'), type: 'textarea' },
    { key: 'preferences', label: t('comparison_profiles.preferences', 'Preferences'), type: 'textarea' },
    { key: 'demographics', label: t('comparison_profiles.demographics', 'Demographics'), type: 'textarea' },
    { key: 'is_template', label: t('comparison_profiles.is_template', 'Template'), type: 'checkbox' },
];
```

Use `JSON.parse` for `goals`, `preferences`, `demographics` in `formSnippet` if `EntityForm` cannot handle nested JSON.

`config.load` calls `jceAdminApi.listProfiles()`. `create`, `update`, `remove` call the matching API methods.

- [ ] **Step 3: Implement `apps/admin/src/routes/comparison-profiles/+page.server.ts`**

```ts
import type { PageServerLoad } from './$types';
import { redirect } from '@sveltejs/kit';

export const load: PageServerLoad = async (event) => {
    if (!event.locals.user) redirect(302, '/login');
    return { user: event.locals.user };
};
```

- [ ] **Step 4: Add a score-factor editor to `apps/admin/src/routes/comparison-types/+page.svelte`**

In the existing `SlideOver` or form snippet used for comparison-type create/edit, add a tab or secondary slide-over:

```svelte
<script lang="ts">
    import { jceAdminApi } from '$lib/api';
    // (import alongside the existing page imports)

    let scoreFactors = $state<ScoreFactorAdmin[]>([]);
    let scoreFactorOpen = $state(false);
    let selectedTypeId = $state<string | null>(null);

    async function openScoreFactors(typeId: string) {
        selectedTypeId = typeId;
        const res = await jceAdminApi.listScoreFactors(typeId);
        if (res.ok) scoreFactors = res.data ?? [];
        scoreFactorOpen = true;
    }
</script>
```

Add a row action in `config.actions`:

```ts
{
    label: t('comparison_types.score_factors', 'Score factors'),
    icon: Sliders,
    onclick: (row) => openScoreFactors(row.id)
}
```

Implement the score-factor form inside the existing or a new `<SlideOver>` with fields:
- `factor_key` (text)
- `label` (text)
- `description` (textarea)
- `data_source` (select: `dimension`, `resolver_metadata`, `profile`, `eligibility`, `aggregate`)
- `dimension_slug` (text, shown when `data_source === 'dimension'`)
- `default_weight` (number 0–1)
- `config` (textarea with JSON: `kind`, `direction`, `value_key`, `profile_key`, `item_key`, `match_strategy`, `missing_score`)

CRUD buttons: Add, Save, Delete, Reset to global defaults.

- [ ] **Step 5: Add i18n keys to admin translations**

If admin uses a translations file, add:

```json
{
    "comparison_profiles.title": "Comparison Profiles",
    "comparison_profiles.name": "Title",
    "comparison_profiles.source_space": "Source country",
    "comparison_profiles.target_spaces": "Target spaces",
    "comparison_profiles.goals": "Goals",
    "comparison_profiles.preferences": "Preferences",
    "comparison_profiles.demographics": "Demographics",
    "comparison_profiles.is_template": "Template",
    "comparison_types.score_factors": "Score factors"
}
```

If admin uses `t('key','fallback')` only, use the fallback strings inline as in the Svelte snippets above.

**Verification:** `bun run check` and `bun run build` pass in `apps/admin`.

---

### Task 12: Feature flags, RBAC nodes, Valkey keys, and catalogs

**Files:**
- Modify `internal/features/registry.go`
- Modify `.jeos/conventions/valkey-keys-catalog.md`
- Modify `.jeos/conventions/system-settings-catalog.md`
- Modify `.jeos/conventions/api-routes-catalog.md`
- Modify `.jeos/conventions/permission-key-naming.md`
- Modify `.jeos/sections/30-generic-comparison-system.md`

**Interfaces:**
- Produces the registry/catalog entries that gate and document the new surface.

**Depends on:** Tasks 1 and 6.

- [ ] **Step 1: Add the three JCE feature flags to `internal/features/registry.go`**

```go
// Japa Comparison Engine — Sub-spec 2/3
{Key: "feature.jce.profiles",     Label: "JCE Comparison Profiles", Default: false},
{Key: "feature.jce.smart_compare", Label: "JCE Smart Compare",       Default: false},
{Key: "feature.jce.scoring",       Label: "JCE Scoring Engine",      Default: false},
```

- [ ] **Step 2: Append JCE Valkey keys to `.jeos/conventions/valkey-keys-catalog.md`**

```markdown
### Japa Comparison Engine (JCE)

| Key Pattern | Value | TTL | Invalidated by |
|---|---|---|---|
| `jc:jce:profile:{tenant_id}:{user_id}:{profile_id}` | JSON profile object | `jce.profile_cache_ttl_minutes` (default 5 min) | profile mutation or natural expiry |
| `jc:jce:score:{tenant_id}:{content_id}:{score_version}` | JSON `ScoreResult` | `jce.score_cache_ttl_minutes` (default 15 min) | `POST /jce/comparisons/{id}/score`, `score_version` change, or natural expiry |
```

- [ ] **Step 3: Append JCE settings to `.jeos/conventions/system-settings-catalog.md`**

```markdown
## Japa Comparison Engine (JCE)

| Key | Default | Type | Description |
|---|---|---|---|
| `jce.default_goals` | `[{"key":"pr_fast","label":"Get permanent residency fast","description":"Prioritise routes with a clear, short PR pathway","default_weight":0.25},{"key":"low_cost","label":"Low cost of living","description":"Minimise tuition, rent, and daily expenses","default_weight":0.25},{"key":"high_income","label":"High earning potential","description":"Maximise post-study or post-move salary","default_weight":0.20},{"key":"family_friendly","label":"Family friendly","description":"Favour schooling, healthcare, and family visa options","default_weight":0.15},{"key":"scholarships","label":"Scholarship opportunities","description":"Favour funding and bursaries","default_weight":0.10},{"key":"language_ease","label":"Language ease","description":"Favour destinations that match my language skills","default_weight":0.05}]` | json | Fallback allowed-goals list for all comparison types. |
| `jce.profile_cache_ttl_minutes` | `5` | int | Valkey TTL for `jc:jce:profile:*` cache. |
| `jce.score_cache_ttl_minutes` | `15` | int | Valkey TTL for `jc:jce:score:*` cache. |
```

- [ ] **Step 4: Append JCE routes to `.jeos/conventions/api-routes-catalog.md`**

Under the existing JCE section, add:

```markdown
GET    /jce/profiles                          list own profiles and templates
POST   /jce/profiles                          create profile
GET    /jce/profiles/stats                    anonymized profile counts per type
GET    /jce/profiles/{id}                     view profile
PATCH  /jce/profiles/{id}                     update profile
DELETE /jce/profiles/{id}                     delete profile
POST   /jce/profiles/{id}/default             set default profile
GET    /jce/comparison-types/{id}/score-factors list score factors
POST   /jce/comparison-types/{id}/score-factors create score factor
GET    /jce/score-factors/{id}                view score factor
PATCH  /jce/score-factors/{id}                update score factor
DELETE /jce/score-factors/{id}                delete score factor
POST   /jce/score-preview                     score a list of items without creating content
POST   /jce/comparisons/{id}/score            recalculate JapaScore and recommendations
GET    /jce/comparisons/{id}/score            get cached scores
GET    /jce/comparisons/{id}/recommendations  get recommendations
```

- [ ] **Step 5: Append JCE RBAC nodes to `.jeos/conventions/permission-key-naming.md`**

```markdown
## Japa Comparison Engine (JCE)

| Node | Default grant |
|---|---|
| `jce.profile.list` | member, admin, super_admin |
| `jce.profile.view` | member, admin, super_admin |
| `jce.profile.create` | member, admin, super_admin |
| `jce.profile.update` | member, admin, super_admin |
| `jce.profile.delete` | member, admin, super_admin |
| `jce.score.view` | member, admin, super_admin |
| `jce.score.recalculate` | member, admin, super_admin |
| `jce.recommendation.view` | member, admin, super_admin |
| `jce.factor.manage` | admin, super_admin |
```

- [ ] **Step 6: Update `.jeos/sections/30-generic-comparison-system.md`**

In the JCE amendment section, add a paragraph:

```markdown
### Sub-spec 2/3 (2026-08-25)

Added tables: `comparison_profiles`, `comparison_score_factors`, `comparison_scores`, `comparison_recommendations`. Added `feature.jce.profiles`, `feature.jce.smart_compare`, `feature.jce.scoring`. Added permission nodes `jce.profile.*`, `jce.score.*`, `jce.recommendation.view`, `jce.factor.manage`. Added system settings `jce.default_goals`, `jce.profile_cache_ttl_minutes`, `jce.score_cache_ttl_minutes`. The `comparison_content_metadata.config` stores `profile_id`, `weights_snapshot`, `recommendation_summary`, and `last_scored_at`. The `ResolvedItem.Metadata["score_inputs"]` map feeds the scoring engine.
```

**Verification:** `go build ./...` and `bun run check` in `apps/main` and `apps/admin` pass.

---

### Task 13: Backend tests and frontend build verification

**Files:**
- Create `internal/jce/profiles_test.go`
- Create `internal/jce/score_factors_test.go`
- Create `internal/jce/scoring/engine_test.go`
- Create `internal/jce/recommendations_test.go`
- Create `internal/jce/handlers_test.go`
- Modify `apps/main/src/lib/api/jce.test.ts`
- Create `apps/admin/src/routes/comparison-profiles/page.test.ts` (optional)

**Interfaces:**
- Consumes the service and handler APIs from previous tasks.
- Produces passing test suite and build artifacts.

**Depends on:** Tasks 1–12.

- [ ] **Step 1: Write backend tests in a test-driven order**

`internal/jce/profiles_test.go`:
- `TestProfileStore_Create`
- `TestProfileStore_ListOwnAndTemplates`
- `TestProfileStore_SetDefault`
- `TestProfileStore_SoftDelete`
- `TestProfileService_OwnershipEnforcement`

`internal/jce/score_factors_test.go`:
- `TestScoreFactorStore_GlobalAndTyped`
- `TestScoreFactorStore_WeightValidation`
- `TestScoreFactorService_SystemProtection`

`internal/jce/scoring/engine_test.go`:
- `TestEngine_NumberNormalization`
- `TestEngine_MissingInput`
- `TestEngine_ProfileMatch`
- `TestEngine_EligibilityRules`
- `TestEngine_SmartModeMultiplier`
- `TestEngine_JapaScoreMath`
- `TestEngine_RecommendationGeneration`

`internal/jce/handlers_test.go`:
- `TestHandlers_ProfileRoutes_RequireJceProfilesFlag`
- `TestHandlers_ScoreRoutes_RequireJceScoringFlag`
- `TestHandlers_ScoreFactorRoutes_RequireJceFactorManagePermission`

- [ ] **Step 2: Run the full Go verification pipeline**

```bash
go test ./internal/jce/...
go test ./internal/comparison/...
go build ./...
make vet
make test-integration
```

- [ ] **Step 3: Run the full frontend verification pipeline**

```bash
cd apps/main && bun run check && bun run build && bun run test
cd apps/admin && bun run check && bun run build && bun run test
```

- [ ] **Step 4: End-to-end spot check**

- Create a profile, select it, choose smart mode, and create a quick comparison.
- Open `/compare/[slug]`, click **Score**, and verify `ScoreBadge`, `ScoreExplanation`, and `RecommendationPanel` render.
- In `apps/admin`, open a comparison type, add a custom score factor, return to `apps/main`, and see the factor in the explanation.
- Promote the scored quick comparison and verify it still opens in `apps/main` as a detailed draft.

**Verification:** `go test ./...`, `make vet`, `bun run build` in `apps/main` and `apps/admin` all pass.

---

## Exit Criteria

- Migrations `000551`–`000555` are applied and reversible.
- `internal/jce/*` compiles with `go build ./...` and passes `go test ./internal/jce/...`.
- All new `/api/v1/jce/*` routes are wired and gated by the correct feature flags and RBAC nodes.
- Every resolver exposes `ResolvedItem.Metadata["score_inputs"]`.
- `apps/main` renders the profile drawer, smart toggle, score badges, explanations, and recommendations.
- `apps/admin` can manage profile templates and score factors.
- Catalogs and registry are updated with the new flags, keys, settings, routes, and permissions.
- `bun run check` and `bun run build` pass in `apps/main` and `apps/admin`.
