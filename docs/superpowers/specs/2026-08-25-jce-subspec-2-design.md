# Japa Comparison Engine (JCE) — Sub-spec 2/3: Personalization, Smart Compare & JapaScore

**Status:** Design
**Refs:** §30 (Generic Comparison System), §103 (AI Agent Executor), ADR-029, ADR-058, ADR-076, ADR-088, `docs/japa-comparison-engine.md`, `docs/superpowers/specs/2026-08-22-jce-subspec-1-design.md`, `docs/superpowers/specs/2026-08-22-jce-subspecs-2-4-outline.md`
**Scope:** The second JCE sub-spec. It combines Sub-spec 2 (personalization and Smart Compare) with Sub-spec 3 (JapaScore, eligibility, ranking, explainability, and recommendations) into one deliverable: a tenant-configurable, profile-driven decision engine. It builds on the Sub-spec 1 foundation (`apps/jce`, `comparison_content_metadata`, `item_ref`, resolver registry, and `/api/v1/jce/*`). Out of scope: data-freshness/ingestion and the AI decision agent — those remain Sub-spec 4.

---

## 1. Problem

Sub-spec 1 gave JCE a solid foundation: quick and detailed comparisons, a polymorphic `item_ref` resolver registry, the `/compare` picker, the `/comparisons` directory, and promotion. However the product is still a static matrix:

1. **No personalization.** Every user sees the same rows and columns. There is no way to say "I am 28, a software engineer, moving with family, and I care most about PR speed and low cost."
2. **No scoring or ranking.** Users must manually compare tuition, visa rules, cost of living, and salary. There is no 0–100 JapaScore that tells them which option fits best.
3. **No explainability.** When one item looks better, there is no "why" — no breakdown of which factors helped or hurt the score.
4. **No eligibility engine.** There is no way to mark a visa route or scholarship as "eligible", "limited", or "ineligible" based on the user's profile.
5. **No recommendations.** The system cannot suggest next actions such as "Book IELTS", "Check scholarship deadline", or "Apply to this university."
6. **No tenant-level scoring configuration.** Comparison types cannot define their own goals, weights, or eligibility rules.

This sub-spec closes those gaps by introducing user comparison profiles, a JapaScore engine, and a recommendation generator.

---

## 2. Goal

Deliver a complete decision-engine layer on top of the Sub-spec 1 foundation:

1. A `comparison_profiles` table and CRUD UI so users can save reusable decision profiles.
2. A **Smart Compare** toggle and profile selector on the `/compare` picker and `/compare/[slug]` preview.
3. A **JapaScore** (0–100) for each item in a comparison, computed from score factors, profile, and eligibility rules.
4. Per-item, per-factor **explanations** that answer "why this score".
5. **Ranking and next-best action recommendations** persisted in `comparison_recommendations`.
6. Tenant-configurable **goals**, **weights**, and **eligibility rules** stored in `comparison_type_definitions.config`.
7. A **score-factor admin editor** in `apps/admin` so admins can tune the decision engine per comparison type.
8. Feature flags, RBAC nodes, and Valkey cache patterns that keep the new surface gated and fast.

---

## 3. Scope and non-scope

### In scope

- New tables: `comparison_profiles`, `comparison_score_factors`, `comparison_scores`, `comparison_recommendations`.
- Schema extensions to `comparison_content_metadata.config` and `comparison_type_definitions.config`.
- New Go package `internal/jce` for profile, scoring, factor, and recommendation orchestration.
- New `/api/v1/jce/profiles`, `/api/v1/jce/score-factors`, `/api/v1/jce/comparisons/{id}/score`, and `/api/v1/jce/comparisons/{id}/recommendations` endpoints.
- `apps/jce` updates: profile drawer, smart toggle, score badges, explanations, recommendation panel.
- `apps/admin` updates: new `/comparison-profiles` page and a score-factor editor on the comparison-type form.
- Resolver metadata enrichment: every `ResolvedItem` must expose a `score_inputs` map for the scoring engine.
- Feature flags, RBAC nodes, and Valkey keys.
- Synchronous scoring with request-time caching in `comparison_scores`, `comparison_recommendations`, and Valkey.

### Out of scope

- Data ingestion, freshness scores, and AI decision-agent tools — Sub-spec 4.
- Async/real-time scoring pipelines or machine-learning model training.
- Public, community-driven scoring (all scores are private to the comparison owner and computed from the user's profile).
- Promotion of smart-scored comparisons into detailed comparisons is unchanged from Sub-spec 1; only the scored view is new.

---

## 4. Domain model

| Term | Definition |
|---|---|
| **Comparison profile** | A saved user preferences object in `comparison_profiles`. Contains source/target spaces, goals, weighted preferences, and demographics. |
| **Smart mode** | A value from `comparison_content_metadata.smart_mode` (`none`, `eligibility`, `cost`, `pathway`, `ai_advisor`) that tells the UI and scoring engine which lens to apply. |
| **JapaScore** | A 0–100 aggregate score for a single item, computed by the scoring engine from weighted factor scores. |
| **Score factor** | A `comparison_score_factors` row. A factor has a `factor_key`, `data_source` (`dimension`, `resolver_metadata`, `profile`, `eligibility`, `aggregate`), a `default_weight`, and a `config` that defines how to normalize inputs into a 0–100 score. |
| **Recommendation** | A `comparison_recommendations` row with a human-readable `text`, optional `action_type`/`action_url`, and a `priority`. Generated after scoring. |
| **Eligibility status** | One of `eligible`, `limited`, or `ineligible`. Stored on `comparison_scores.eligible_status` for eligibility factors and surfaced on the matrix. |
| **Weights snapshot** | The map of `factor_key → weight` actually used for a given score run, persisted in `comparison_content_metadata.config.weights_snapshot`. |

---

## 5. Schema changes

The next available migration number is `000551` (migrations `000540`–`000550` already exist). Use the four sequential files below.

### 5.1 Migration `000551_jce_profiles`

`up`:

```sql
-- 000551_jce_profiles.up.sql
-- JCE Sub-spec 2/3: comparison profiles and goal/type config.

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
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_comparison_profiles_tenant_user
    ON comparison_profiles(tenant_id, user_id)
    WHERE user_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_comparison_profiles_tenant_templates
    ON comparison_profiles(tenant_id, is_template)
    WHERE is_template = true;

CREATE UNIQUE INDEX IF NOT EXISTS idx_comparison_profiles_one_default
    ON comparison_profiles(tenant_id, user_id)
    WHERE is_default = true AND user_id IS NOT NULL;

-- Update all system comparison types with allowed goals and profile defaults.
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
    )
)
WHERE is_system = true;

-- Global default goals fallback in system_settings.
INSERT INTO system_settings (tenant_id, key, value)
SELECT '00000000-0000-0000-0000-000000000001', 'jce.default_goals', '[
    {"key": "pr_fast", "label": "Get permanent residency fast", "description": "Prioritise routes with a clear, short PR pathway", "default_weight": 0.25},
    {"key": "low_cost", "label": "Low cost of living", "description": "Minimise tuition, rent, and daily expenses", "default_weight": 0.25},
    {"key": "high_income", "label": "High earning potential", "description": "Maximise post-study or post-move salary", "default_weight": 0.20},
    {"key": "family_friendly", "label": "Family friendly", "description": "Favour schooling, healthcare, and family visa options", "default_weight": 0.15},
    {"key": "scholarships", "label": "Scholarship opportunities", "description": "Favour funding and bursaries", "default_weight": 0.10},
    {"key": "language_ease", "label": "Language ease", "description": "Favour destinations that match my language skills", "default_weight": 0.05}
]'::jsonb
ON CONFLICT (tenant_id, key) DO NOTHING;

INSERT INTO system_settings (tenant_id, key, value)
SELECT '00000000-0000-0000-0000-000000000001', 'jce.profile_cache_ttl_minutes', to_jsonb(5)
ON CONFLICT (tenant_id, key) DO NOTHING;

-- Feature flag.
INSERT INTO feature_flags (key, tenant_id, enabled)
VALUES ('feature.jce.profiles', '00000000-0000-0000-0000-000000000001', false)
ON CONFLICT (key, tenant_id) DO NOTHING;

-- RBAC nodes.
INSERT INTO permissions (key, description) VALUES
    ('jce.profile.list', 'List comparison profiles'),
    ('jce.profile.view', 'View a comparison profile'),
    ('jce.profile.create', 'Create a comparison profile'),
    ('jce.profile.update', 'Update a comparison profile'),
    ('jce.profile.delete', 'Delete a comparison profile')
ON CONFLICT (key) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug IN ('member', 'admin', 'super_admin')
  AND p.key IN ('jce.profile.list', 'jce.profile.view', 'jce.profile.create', 'jce.profile.update', 'jce.profile.delete')
ON CONFLICT DO NOTHING;

COMMIT;
```

`down`:

```sql
-- 000551_jce_profiles.down.sql

BEGIN;

DROP TABLE IF EXISTS comparison_profiles CASCADE;

DELETE FROM system_settings
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND key IN ('jce.default_goals', 'jce.profile_cache_ttl_minutes');

DELETE FROM feature_flags
WHERE key = 'feature.jce.profiles'
  AND tenant_id = '00000000-0000-0000-0000-000000000001';

DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT id FROM permissions WHERE key LIKE 'jce.profile.%'
);

DELETE FROM permissions WHERE key LIKE 'jce.profile.%';

UPDATE comparison_type_definitions
SET config = config - 'allowed_goals' - 'profile_defaults' - 'eligibility_rules' - 'goal_factor_map'
WHERE is_system = true;

COMMIT;
```

### 5.2 Migration `000552_jce_score_factors`

`up`:

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

-- Add scoring config to system comparison types.
UPDATE comparison_type_definitions
SET config = COALESCE(config, '{}'::jsonb) || jsonb_build_object(
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
    )
)
WHERE is_system = true;

-- Global default score factors. comparison_type_id IS NULL means "global default".
INSERT INTO comparison_score_factors (
    tenant_id, comparison_type_id, factor_key, label, description,
    data_source, default_weight, config, is_system
)
SELECT '00000000-0000-0000-0000-000000000001', NULL, f.*, true
FROM (VALUES
    ('japa_score', 'JapaScore', 'Overall 0–100 decision score', 'aggregate', 0, '{"is_output": true, "max": 100}'::jsonb),
    ('eligibility', 'Eligibility', 'Hard and soft eligibility rules from the comparison type', 'eligibility', 0.25, '{"penalty_ineligible": 0}'::jsonb),
    ('cost', 'Cost', 'Cost of living, tuition, or fees (lower is better)', 'resolver_metadata', 0.20, '{"kind": "number", "direction": "lower_is_better", "value_key": "cost", "missing_score": 0}'::jsonb),
    ('income', 'Income', 'Salary or earning potential (higher is better)', 'resolver_metadata', 0.15, '{"kind": "number", "direction": "higher_is_better", "value_key": "income", "missing_score": 0}'::jsonb),
    ('speed', 'Speed', 'Time to outcome (lower is better)', 'resolver_metadata', 0.10, '{"kind": "number", "direction": "lower_is_better", "value_key": "speed", "missing_score": 0}'::jsonb),
    ('language_match', 'Language match', 'How well the item matches the profile language', 'profile', 0.10, '{"profile_key": "demographics.language_proficiency", "item_key": "language", "match_strategy": "exact"}'::jsonb),
    ('location_match', 'Location match', 'Whether the item is in the profile target spaces', 'profile', 0.10, '{"profile_key": "target_space_ids", "item_key": "geo_space_id", "match_strategy": "exact"}'::jsonb),
    ('pr_path', 'PR pathway', 'Strength of the permanent residency pathway', 'resolver_metadata', 0.10, '{"kind": "number", "direction": "higher_is_better", "value_key": "pr_path_ease", "missing_score": 0}'::jsonb)
) AS f(factor_key, label, description, data_source, default_weight, config)
WHERE NOT EXISTS (
    SELECT 1 FROM comparison_score_factors x
    WHERE x.tenant_id = '00000000-0000-0000-0000-000000000001'
      AND x.comparison_type_id IS NULL
      AND x.factor_key = f.factor_key
);

INSERT INTO system_settings (tenant_id, key, value)
SELECT '00000000-0000-0000-0000-000000000001', 'jce.score_cache_ttl_minutes', to_jsonb(15)
ON CONFLICT (tenant_id, key) DO NOTHING;

INSERT INTO feature_flags (key, tenant_id, enabled)
VALUES ('feature.jce.smart_compare', '00000000-0000-0000-0000-000000000001', false)
ON CONFLICT (key, tenant_id) DO NOTHING;

INSERT INTO permissions (key, description) VALUES
    ('jce.score.view', 'View JapaScore for a comparison'),
    ('jce.score.recalculate', 'Recalculate JapaScore for a comparison'),
    ('jce.recommendation.view', 'View comparison recommendations'),
    ('jce.factor.manage', 'Manage score factors and scoring rules')
ON CONFLICT (key) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND (
      (p.key IN ('jce.score.view', 'jce.score.recalculate', 'jce.recommendation.view')
       AND ur.slug IN ('member', 'admin', 'super_admin'))
      OR (p.key = 'jce.factor.manage' AND ur.slug IN ('admin', 'super_admin'))
  )
ON CONFLICT DO NOTHING;

COMMIT;
```

`down`:

```sql
-- 000552_jce_score_factors.down.sql

BEGIN;

DROP TABLE IF EXISTS comparison_score_factors CASCADE;

DELETE FROM system_settings
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND key = 'jce.score_cache_ttl_minutes';

DELETE FROM feature_flags
WHERE key = 'feature.jce.smart_compare'
  AND tenant_id = '00000000-0000-0000-0000-000000000001';

DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT id FROM permissions
    WHERE key IN ('jce.score.view', 'jce.score.recalculate', 'jce.recommendation.view', 'jce.factor.manage')
);

DELETE FROM permissions
WHERE key IN ('jce.score.view', 'jce.score.recalculate', 'jce.recommendation.view', 'jce.factor.manage');

UPDATE comparison_type_definitions
SET config = config - 'scoring_enabled' - 'default_weights' - 'smart_modes'
WHERE is_system = true;

COMMIT;
```

### 5.3 Migration `000553_jce_scores`

`up`:

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

-- Add smart/scoring config keys to comparison metadata.
UPDATE comparison_content_metadata
SET config = COALESCE(config, '{}'::jsonb) || jsonb_build_object(
    'profile_id', null,
    'weights_snapshot', '{}'::jsonb,
    'recommendation_summary', '{}'::jsonb,
    'last_scored_at', null
);

INSERT INTO feature_flags (key, tenant_id, enabled)
VALUES ('feature.jce.scoring', '00000000-0000-0000-0000-000000000001', false)
ON CONFLICT (key, tenant_id) DO NOTHING;

COMMIT;
```

`down`:

```sql
-- 000553_jce_scores.down.sql

BEGIN;

DROP TABLE IF EXISTS comparison_scores CASCADE;

DELETE FROM feature_flags
WHERE key = 'feature.jce.scoring'
  AND tenant_id = '00000000-0000-0000-0000-000000000001';

UPDATE comparison_content_metadata
SET config = config - 'profile_id' - 'weights_snapshot' - 'recommendation_summary' - 'last_scored_at';

COMMIT;
```

### 5.4 Migration `000554_jce_recommendations`

`up`:

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

-- Recommendation templates for system types.
UPDATE comparison_type_definitions
SET config = COALESCE(config, '{}'::jsonb) || jsonb_build_object(
    'recommendation_templates', '[
        {"factor_key": "language_match", "condition": "score_lt", "threshold": 50, "text": "Book a language test to improve your match.", "action_type": "book", "priority": 20},
        {"factor_key": "cost", "condition": "score_lt", "threshold": 40, "text": "Review your budget against this option.", "action_type": "review", "priority": 10},
        {"factor_key": "eligibility", "condition": "status_in", "statuses": ["limited", "ineligible"], "text": "Check the eligibility rules for this option.", "action_type": "check", "priority": 30}
    ]'::jsonb
)
WHERE is_system = true;

COMMIT;
```

`down`:

```sql
-- 000554_jce_recommendations.down.sql

BEGIN;

DROP TABLE IF EXISTS comparison_recommendations CASCADE;

UPDATE comparison_type_definitions
SET config = config - 'recommendation_templates'
WHERE is_system = true;

COMMIT;
```

---

## 6. Backend architecture

### 6.1 Package structure

Keep `internal/comparison` as the generic CRUD/matrix engine. Introduce a new `internal/jce` package for product-specific scoring and personalization.

```
internal/
├── comparison/            # existing; CRUD, matrix, resolver, generic JCE API
│   ├── jce_service.go     # quick/detailed CRUD, promotion, entity search
│   ├── jce_handlers.go    # /api/v1/jce/comparisons/* routes
│   └── resolver/          # item_ref resolvers
├── jce/                   # new; profiles, scoring, factors, recommendations
│   ├── service.go         # orchestration, dependency wiring
│   ├── profiles.go        # profile CRUD
│   ├── score_factors.go   # score-factor CRUD
│   ├── scoring.go         # JapaScore engine
│   ├── recommendations.go # recommendation generator
│   ├── handlers.go        # /api/v1/jce/profiles, /score-factors, /score, /recommendations
│   └── models.go          # input/output structs
```

`internal/jce.Service` depends on:

- `*db.DB` for read/write pools.
- `*comparison.Service` for comparison CRUD, matrix, and `item_ref` resolution.
- `*resolver.Registry` for resolving `ResolvedItem.Metadata.score_inputs`.
- `*config.Settings` for `jce.*` hot-reloadable settings.
- a Valkey client (`valkeycompat.Cmdable`) for profile and score caching.

### 6.2 Service method signatures

```go
// Profiles
func (s *Service) CreateComparisonProfile(ctx context.Context, tenantID, userID pgtype.UUID, in ComparisonProfileInput) (ComparisonProfile, error)
func (s *Service) ListComparisonProfiles(ctx context.Context, tenantID, userID pgtype.UUID, includeTemplates bool) ([]ComparisonProfile, error)
func (s *Service) GetComparisonProfile(ctx context.Context, id, tenantID, userID pgtype.UUID) (ComparisonProfile, error)
func (s *Service) UpdateComparisonProfile(ctx context.Context, id, tenantID, userID pgtype.UUID, in ComparisonProfileInput) (ComparisonProfile, error)
func (s *Service) DeleteComparisonProfile(ctx context.Context, id, tenantID, userID pgtype.UUID) error
func (s *Service) SetDefaultComparisonProfile(ctx context.Context, id, tenantID, userID pgtype.UUID) error

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
func (s *Service) ListRecommendations(ctx context.Context, contentID, tenantID, userID pgtype.UUID, profileID *pgtype.UUID, limit int) ([]Recommendation, error)
```

### 6.3 Profile model

```go
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

type ProfileGoal struct {
    Key    string  `json:"key"`
    Weight float64 `json:"weight"`
}

type ProfileDemographics struct {
    Age                *int     `json:"age"`
    Occupation         string   `json:"occupation"`
    FamilyStatus       string   `json:"family_status"`
    LanguageProficiency string   `json:"language_proficiency"`
    BudgetMin          *float64 `json:"budget_min"`
    BudgetMax          *float64 `json:"budget_max"`
    BudgetCurrency     string   `json:"budget_currency"`
}
```

### 6.4 Score-factor model

```go
type ScoreFactor struct {
    ID               string
    TenantID         string
    ComparisonTypeID *string
    FactorKey        string
    Label            string
    Description      string
    DimensionSlug    *string
    DataSource       string // dimension, resolver_metadata, profile, eligibility, aggregate
    DefaultWeight    float64
    Config           ScoreFactorConfig
    Version          int
    IsSystem         bool
}

type ScoreFactorConfig struct {
    Kind          string   `json:"kind"`          // number, rating, boolean, text, eligibility
    Direction     string   `json:"direction"`     // higher_is_better, lower_is_better
    Min           float64  `json:"min"`
    Max           float64  `json:"max"`
    ValueKey      string   `json:"value_key"`     // key inside ResolvedItem.Metadata.score_inputs
    ProfileKey    string   `json:"profile_key"`   // e.g. demographics.budget_max
    ItemKey       string   `json:"item_key"`      // e.g. language, geo_space_id
    MatchStrategy string   `json:"match_strategy"` // exact, contains, budget_lte, budget_gte
    IdealTexts    []string `json:"ideal_texts"`
    MissingScore  float64  `json:"missing_score"`
    IsOutput      bool     `json:"is_output"`
}
```

### 6.5 Scoring engine flow

```mermaid
flowchart TD
    A[POST /jce/comparisons/{id}/score] --> B[Load comparison + metadata + items + dimensions + values]
    B --> C[Resolve profile or use defaults]
    C --> D[Resolve item_ref for every item]
    D --> E[Build final weights map:<br/>type defaults + profile preferences + smart-mode multipliers + goals]
    E --> F[For each item × factor, compute a 0–100 factor score]
    F --> G[Run eligibility rules from type config]
    G --> H[Compute JapaScore as weighted sum of available factor scores]
    H --> I[Persist comparison_scores + japa_score aggregate row]
    I --> J[Generate comparison_recommendations]
    J --> K[Update metadata.config weights_snapshot, recommendation_summary, last_scored_at]
    K --> L[Return ScoreResult and set Valkey cache]
```

Detailed steps:

1. **Visibility check.** The caller must be the comparison owner or have `comparison.edit_any`. Public comparisons can also be scored by any authenticated user with `jce.score.recalculate`.
2. **Profile resolution.** If `profileID` is supplied, load it from `comparison_profiles` (and Valkey). If not, build a default profile from the type's `profile_defaults`.
3. **Weight map.** Start with `comparison_type_definitions.config.default_weights`. Apply `comparison_profiles.preferences` overrides. Apply `comparison_type_definitions.config.goal_factor_map` boosts for each selected goal. Apply `smart_mode` multipliers from `config.smart_modes`.
4. **Item resolution.** For each `comparison_items.item_ref`, call `resolver.Resolve` to obtain `ResolvedItem.Metadata`. Each resolver must provide a `score_inputs` map. The scoring engine also reads `comparison_values` for each item × dimension.
5. **Factor scoring.**
   - `data_source='dimension'`: read the dimension value from `comparison_values` using `factor.dimension_slug = comparison_dimensions.slug`. Normalize with `factor.config`.
   - `data_source='resolver_metadata'`: read `ResolvedItem.Metadata["score_inputs"][factor.config.value_key]`. Normalize.
   - `data_source='profile'`: compare the profile field (`factor.config.profile_key`) against the item field (`factor.config.item_key`) using `factor.config.match_strategy`.
   - `data_source='eligibility'`: evaluate `comparison_type_definitions.config.eligibility_rules`. Set `eligible_status` and score.
   - `data_source='aggregate'`: reserved for the `japa_score` factor; computed as the weighted sum.
6. **Missing inputs.** If an input is missing, the factor score is `factor.config.missing_score` (default `0`). The explanation should note the missing source so the user can see why the score is low.
7. **Eligibility.** Each rule in `eligibility_rules` compares a profile value and an item value with an operator (`gte`, `lte`, `eq`, `in`). If a rule fails with `on_fail='ineligible'`, the item's `eligibility` factor score is `0` and `eligible_status='ineligible'`; if `on_fail='limited'`, `50` and `limited`; otherwise `100` and `eligible`.
8. **JapaScore.** `japa_score = Σ(factor_score × normalized_weight) / Σ(normalized_weight)` for all factors with usable inputs. The result is stored as a `comparison_scores` row for the `japa_score` factor.
9. **Recommendations.** For each item, generate recommendations from `recommendation_templates` in the type config. Trigger templates when a factor score is below a threshold or the eligibility status is `limited`/`ineligible`. Always generate a top-item recommendation such as "This is your best-fit option."
10. **Persistence.** Upsert per-factor rows into `comparison_scores` and recommendations into `comparison_recommendations`. Update `comparison_content_metadata.score_version` (`score_version = score_version + 1`) and `config` with the weights snapshot and recommendation summary.
11. **Cache.** Write `jc:jce:score:{tenant_id}:{content_id}:{score_version}` with the TTL from `jce.score_cache_ttl_minutes`.

### 6.6 Profile CRUD

- **Create.** Validates `title`, `target_space_ids` (at least one for the engine to be meaningful), `goals` (must use keys from the comparison type's `allowed_goals`), and `preferences` (factor keys must exist for the type). Resets `is_default` on other profiles if the new one is marked default.
- **List.** Returns the caller's own profiles plus system templates (`is_template=true`).
- **Get.** Returns the profile if the caller owns it or it is a template. Admins with `comparison.edit_any` can view any profile.
- **Update.** Same ownership rules as get. Allows partial updates to `demographics`, `goals`, and `preferences`.
- **Delete.** Soft-delete by id. Admins can delete templates.
- **Set default.** Mark one of the user's profiles as default and clear the flag on all others.

### 6.7 Routes

Wire the new handlers in `internal/server/server.go` under the existing `/api/v1/jce` route group:

```go
r.Route("/api/v1/jce", func(r chi.Router) {
    r.Use(featureMiddleware(feature.comparison), featureMiddleware(feature.jce))

    // Profiles
    r.Get("/profiles", jceHandler.ListProfiles)
    r.Post("/profiles", jceHandler.CreateProfile)
    r.Get("/profiles/{id}", jceHandler.GetProfile)
    r.Patch("/profiles/{id}", jceHandler.UpdateProfile)
    r.Delete("/profiles/{id}", jceHandler.DeleteProfile)
    r.Post("/profiles/{id}/default", jceHandler.SetDefaultProfile)

    // Score factors
    r.Get("/comparison-types/{id}/score-factors", jceHandler.ListScoreFactors)
    r.Post("/comparison-types/{id}/score-factors", jceHandler.CreateScoreFactor)
    r.Get("/score-factors/{id}", jceHandler.GetScoreFactor)
    r.Patch("/score-factors/{id}", jceHandler.UpdateScoreFactor)
    r.Delete("/score-factors/{id}", jceHandler.DeleteScoreFactor)

    // Scoring and recommendations
    r.Post("/comparisons/{id}/score", jceHandler.ScoreComparison)
    r.Get("/comparisons/{id}/score", jceHandler.GetComparisonScore)
    r.Get("/comparisons/{id}/recommendations", jceHandler.ListRecommendations)
    r.Post("/score-preview", jceHandler.PreviewScoreForItems)
})
```

Each new route is additionally gated by the appropriate feature flag (`feature.jce.profiles`, `feature.jce.smart_compare`, `feature.jce.scoring`) and the permission node listed in §10.

---

## 7. Frontend

### 7.1 `apps/jce` changes

New and updated files:

```
apps/jce/src/lib/types/jce.ts             # add profile, factor, score, recommendation types
apps/jce/src/lib/api/jce.ts               # add profile/score/recommendation client calls
apps/jce/src/lib/components/compare/
  ├── ProfileDrawer.svelte
  ├── SmartToggle.svelte
  ├── ScoreBadge.svelte
  ├── ScoreExplanation.svelte
  └── RecommendationPanel.svelte
apps/jce/src/routes/compare/+page.svelte            # profile drawer, smart toggle, score preview
apps/jce/src/routes/compare/+page.server.ts         # load profiles and templates
apps/jce/src/routes/compare/[slug]/+page.svelte     # smart view, score display, explanations, recommendations
apps/jce/src/routes/compare/[slug]/+page.server.ts  # load score + recommendations if smart mode active
```

### 7.2 Profile drawer on `/compare`

- A "My Japa Profile" button opens a drawer from the right.
- The drawer lists the user's saved profiles and a "Create profile" form (`title`, source country, target countries/cities, age, occupation, family status, language proficiency, budget, goals, weighted preferences).
- Selecting a profile sets it as the active profile for the current comparison. The profile id is passed to `POST /api/v1/jce/comparisons` inside `metadata.config.profile_id`.
- Templates (`is_template=true`) are shown to all authenticated users as starting points; creating from a template copies it into the user's own profile.

### 7.3 Smart toggle on `/compare/[slug]`

- Add a `Smart mode` segmented control or dropdown at the top of the quick preview: `none | eligibility | cost | pathway`.
- A profile selector next to it loads the user's profiles. The default profile is pre-selected.
- When the user toggles smart mode or changes the profile, the page calls `POST /api/v1/jce/comparisons/{id}/score` (or `GET` for cached) and re-renders.
- The matrix columns re-order by `japa_score` descending. Ineligible items are greyed and collapsed, with a badge showing `Limited` or `Ineligible`.
- Each item header shows a `ScoreBadge` with the JapaScore and, on hover/click, expands `ScoreExplanation` to show per-factor bars and explanations.

### 7.4 Score display, explanations, and recommendations

- **Score display.** Each item column header shows the JapaScore as a 0–100 badge. The best-fit item gets a "Best match" label and a distinct colour.
- **Explanations.** Expandable per-factor rows appear below the matrix: `+12 PR pathway — this route has a 3-year PR track`, `-15 Cost — your budget is $5,000 below the required funds`, `+10 Language match — English is your selected language`. Missing data is shown as `No data` with a warning icon.
- **Recommendations.** A `RecommendationPanel` lists the top 5 recommendations for the best-fit item and the user's lowest-scoring gaps, with action chips (`Apply`, `Book test`, `Check deadline`, `Review budget`) and deep links to the relevant tool pages.

### 7.5 New `jce.ts` types

```typescript
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

export interface JapaScore {
    item_id: string;
    item_title: string;
    japa_score: number;
    max_value: number;
    eligible_status: 'eligible' | 'limited' | 'ineligible';
    factor_scores: FactorScore[];
    explanation: string;
}

export interface FactorScore {
    factor_key: string;
    label: string;
    score_value: number;
    max_value: number;
    weight: number;
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
```

---

## 8. Admin

### 8.1 New page: `/comparison-profiles`

`apps/admin/src/routes/comparison-profiles/+page.svelte` and `+page.server.ts`:

- Lists system profile templates (`is_template=true`) for the tenant.
- Allows an admin to create/edit a template: title, source space, target spaces, goals, preferences, demographics, `is_template`.
- Shows an anonymised count of user-created profiles per type (no PII exposure).
- Links each template to a preview score against an example comparison.

### 8.2 Score-factor editor

Extend `apps/admin/src/routes/comparison-types/+page.svelte` with a **Score factors** tab:

- Lists factors for the selected comparison type (or global defaults if the type has no overrides).
- Add/edit drawer with fields: `factor_key`, `label`, `description`, `data_source`, `dimension_slug`, `default_weight`, and a structured `config` form whose visible fields depend on `data_source` (`kind`, `direction`, `min`, `max`, `value_key`, `profile_key`, `item_key`, `match_strategy`, `ideal_texts`, `missing_score`).
- Buttons to clone a global factor for this type, reset to global defaults, and reorder factors.
- Use dedicated Svelte repeater form components for the type's list-shaped config: `allowed_goals`, `eligibility_rules`, and `recommendation_templates`. Each repeater row contains typed sub-fields (e.g. `key`, `label`, `operator`, `value`, `on_fail`).
- `default_weights` is shown as a per-factor weight list (one row per factor key) rather than a raw JSON map.
- The score-factor `config` remains a structured form keyed by `data_source` (not a JSON textarea), because each data source has a fixed set of options (`kind`, `direction`, `value_key`, `match_strategy`, etc.).

---

## 9. API endpoints

| Method | Route | Purpose | Permission |
|---|---|---|---|
| `GET` | `/api/v1/jce/profiles` | List own profiles and tenant templates | `jce.profile.list` |
| `POST` | `/api/v1/jce/profiles` | Create a profile | `jce.profile.create` |
| `GET` | `/api/v1/jce/profiles/{id}` | View a profile | `jce.profile.view` |
| `PATCH` | `/api/v1/jce/profiles/{id}` | Update a profile | `jce.profile.update` |
| `DELETE` | `/api/v1/jce/profiles/{id}` | Delete a profile | `jce.profile.delete` |
| `POST` | `/api/v1/jce/profiles/{id}/default` | Mark a profile as default | `jce.profile.update` |
| `GET` | `/api/v1/jce/comparison-types/{id}/score-factors` | List score factors for a type | `jce.score.view` |
| `POST` | `/api/v1/jce/comparison-types/{id}/score-factors` | Create a factor for a type | `jce.factor.manage` |
| `GET` | `/api/v1/jce/score-factors/{id}` | View a score factor | `jce.factor.manage` |
| `PATCH` | `/api/v1/jce/score-factors/{id}` | Update a score factor | `jce.factor.manage` |
| `DELETE` | `/api/v1/jce/score-factors/{id}` | Delete a score factor | `jce.factor.manage` |
| `POST` | `/api/v1/jce/score-preview` | Score a list of items without creating content | `jce.score.view` |
| `POST` | `/api/v1/jce/comparisons/{id}/score` | Recalculate JapaScore and recommendations | `jce.score.recalculate` |
| `GET` | `/api/v1/jce/comparisons/{id}/score` | Get cached scores | `jce.score.view` |
| `GET` | `/api/v1/jce/comparisons/{id}/recommendations` | Get recommendations | `jce.recommendation.view` |

All routes require the parent feature flags (`feature.comparison`, `feature.jce`, and the relevant sub-flag).

---

## 10. Feature flags and RBAC nodes

### 10.1 Feature flags

Add to `internal/features/registry.go`:

```go
{Key: "feature.jce.profiles",     Label: "JCE Comparison Profiles", Default: false},
{Key: "feature.jce.smart_compare", Label: "JCE Smart Compare",       Default: false},
{Key: "feature.jce.scoring",       Label: "JCE Scoring Engine",      Default: false},
```

Gating:

| Flag | Gates |
|---|---|
| `feature.jce.profiles` | `/jce/profiles` and the profile drawer |
| `feature.jce.smart_compare` | smart-mode toggle, `/jce/score-preview` |
| `feature.jce.scoring` | `/jce/comparisons/{id}/score` and `/recommendations` |

### 10.2 RBAC nodes

| Node | Default grant | Purpose |
|---|---|---|
| `jce.profile.list` | `member`, `admin`, `super_admin` | List profiles |
| `jce.profile.view` | `member`, `admin`, `super_admin` | View a profile |
| `jce.profile.create` | `member`, `admin`, `super_admin` | Create a profile |
| `jce.profile.update` | `member`, `admin`, `super_admin` | Update a profile |
| `jce.profile.delete` | `member`, `admin`, `super_admin` | Delete a profile |
| `jce.score.view` | `member`, `admin`, `super_admin` | View JapaScore |
| `jce.score.recalculate` | `member`, `admin`, `super_admin` | Trigger score recalculation |
| `jce.recommendation.view` | `member`, `admin`, `super_admin` | View recommendations |
| `jce.factor.manage` | `admin`, `super_admin` | Manage score factors |

The service layer still enforces ownership: users can only view or mutate their own profiles unless they have `comparison.edit_any` (admin/super_admin/moderator).

---

## 11. Valkey keys and catalogs to update

### 11.1 New Valkey keys

| Key pattern | Value | TTL | Invalidated by |
|---|---|---|---|
| `jc:jce:profile:{tenant_id}:{user_id}:{profile_id}` | JSON profile object | `jce.profile_cache_ttl_minutes` (default 5 min) | profile mutation or natural expiry |
| `jc:jce:score:{tenant_id}:{content_id}:{score_version}` | JSON `ScoreResult` | `jce.score_cache_ttl_minutes` (default 15 min) | `POST .../score`, `score_version` change, or natural expiry |

### 11.2 Catalogs to update

- `.jeos/conventions/valkey-keys-catalog.md` — add the two JCE keys above under the "Japa Comparison Engine (JCE)" section.
- `.jeos/conventions/system-settings-catalog.md` — add:
  - `jce.default_goals` (JSON, fallback allowed-goals list)
  - `jce.profile_cache_ttl_minutes` (int, default `5`)
  - `jce.score_cache_ttl_minutes` (int, default `15`)
- `.jeos/conventions/api-routes-catalog.md` — add the new `/jce/profiles`, `/jce/score-factors`, `/jce/comparisons/{id}/score`, `/jce/comparisons/{id}/recommendations`, and `/jce/score-preview` routes.
- `.jeos/conventions/permission-key-naming.md` — add a "Japa Comparison Engine" section with the new `jce.*` nodes.
- `.jeos/sections/30-generic-comparison-system.md` — update the JCE amendment to list the new tables, feature flags, and the `score_version` / `smart_mode` usage.

---

## 12. Resolver metadata enrichment

The scoring engine needs a uniform source of per-item inputs. Every resolver must add a `score_inputs` map inside `ResolvedItem.Metadata` in addition to the existing display fields (`geo_space_id`, `country`, `tags`, `slug`).

| Resolver | Fields to add to `ResolvedItem.Metadata["score_inputs"]` |
|---|---|
| `space` (country/state/city) | `cost` (numeric monthly cost index), `income` (average salary), `speed` (processing months to settle), `pr_path_ease` (0–100), `safety_index` (0–100), `healthcare_index` (0–100), `language` (string array), `geo_space_id` |
| `university` | `cost` (tuition/year), `income` (graduate salary), `acceptance_rate` (0–100), `duration_years`, `qs_ranking` (1–N, lower is better), `pr_path_ease`, `post_study_work_months`, `language` (array), `accreditation` (array), `fields_of_study` (array), `geo_space_id` |
| `course` | `cost` (price), `duration_weeks`, `income` (estimated post-course salary), `certification` (bool), `remote` (bool), `prerequisite_level`, `language` (array), `field` |
| `visa_route` | `cost` (fees), `income` (minimum or typical), `speed` (processing months), `pr_path_ease`, `minimum_funds` (numeric), `work_allowed` (bool), `pr_eligible` (bool), `family_included` (bool), `language` (array), `age_min`, `age_max`, `occupation_eligible` (array), `destination_geo_space_id` |
| `col_city` | `cost` (cost-of-living index), `rent` (1BR city centre), `groceries` (monthly), `transport` (monthly), `utilities` (monthly), `income` (salary), `language` (array), `safety_index`, `healthcare_index`, `geo_space_id` |
| `scholarship` | `cost` (award amount, treated as negative cost), `deadline_days` (days until deadline), `fields_of_study` (array), `country` (string), `institution`, `level` (undergrad/postgrad/PhD), `language` (array) |
| `languagetest` | `cost` (test fee), `speed` (result turnaround days), `validity_months`, `accepted_by` (array of country codes), `score_required`, `language` |
| `mentor` | `cost` (hourly rate), `rating` (0–5), `sessions_count`, `areas` (array), `language` (array), `availability_days` |
| `job_listing` | `income` (salary min/max), `cost` (relocation package or signing bonus), `speed` (time to start), `contract_type`, `sponsorship` (bool), `remote` (bool), `required_experience_years`, `language` (array), `location_geo_space_id` |
| `organization` / `bank` | `cost` (monthly fee), `income` (interest or cashback), `speed` (account opening days), `atm_fee`, `min_balance`, `countries` (array of supported country codes) |
| `content` (nested comparison) | `type_slug`, `child_japa_score` (if the nested comparison is itself scored), `freshness_score` (0–100) |

Resolvers should use existing service methods to populate these keys. If a value is not yet available from the underlying tool, the resolver returns `null` for that key; the scoring engine treats `null` as a missing input and uses `factor.config.missing_score`. Sub-spec 4 will backfill missing values from the data-intelligence layer.

---

## 13. Tests

### 13.1 Backend unit tests

- `internal/jce/profiles_test.go`: create, list, get, update, delete, default-toggle, ownership enforcement.
- `internal/jce/score_factors_test.go`: CRUD, global-vs-type override, weight validation.
- `internal/jce/scoring_test.go`: factor scoring for each `data_source`, JapaScore math, eligibility rule evaluation, missing-input handling, smart-mode multipliers.
- `internal/jce/recommendations_test.go`: template matching, top-item recommendation, priority ordering.
- `internal/comparison/resolver/resolver_test.go` or per-resolver tests: verify `score_inputs` keys are present and correctly typed.

### 13.2 Integration tests

- Migration tests for `000551`–`000554` (`make test-integration` / `goose up` and `goose down`).
- End-to-end score flow: create comparison → create profile → `POST /jce/comparisons/{id}/score` → verify `comparison_scores`, `comparison_recommendations`, and metadata.config.
- Cache invalidation: score, then modify a factor, then `POST .../score` with `force=true`, verify `score_version` increments and old Valkey key misses.
- Visibility: ensure quick comparisons are only scored by owner/admin; public detailed comparisons can be scored by any authenticated user.

### 13.3 Frontend tests

- `apps/jce/src/lib/api/jce.test.ts`: profile, score, and recommendation API client calls.
- Component tests (Storybook or Vitest): `ProfileDrawer`, `SmartToggle`, `ScoreBadge`, `RecommendationPanel`.
- Route tests for `/compare` and `/compare/[slug]`: profile selection, smart toggle, score preview, ineligible item rendering.

### 13.4 E2E

- Create a profile on `/compare`, select smart mode, pick items, see score preview, create the comparison, and view scored `/compare/[slug]` with recommendations.
- Admin: open a comparison type in `apps/admin`, add a custom score factor, return to `apps/jce`, and see the factor in the explanation.
- Promotion: smart-scored quick comparison can still be promoted to a detailed comparison (Sub-spec 1 behaviour remains intact).
