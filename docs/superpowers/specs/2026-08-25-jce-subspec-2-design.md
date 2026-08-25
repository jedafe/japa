# Japa Comparison Engine (JCE) — Sub-spec 2: Personalization & JapaScore Engine

**Status:** Design — pending implementation plan  
**Refs:** §14 (Content Type System), §23 (Main Site Frontend), §30 (Generic Comparison System), §103 (AI Agent Executor), ADR-029, ADR-058, ADR-064, ADR-088, `docs/superpowers/specs/2026-08-22-jce-subspecs-2-4-outline.md`, `docs/superpowers/specs/2026-08-22-jce-subspec-1-design.md`  
**Scope:** Second JCE sub-spec. Builds on the Sub-spec 1 foundation (`apps/jce`, `comparison_content_metadata`, `item_ref`, resolver registry, `/api/v1/jce/*`). Adds authenticated **Comparison Profiles**, **Smart Compare** mode, and a full **JapaScore** engine with scores, explanations, ranking, and recommendations. Because the user chose the full scoring engine, this sub-spec intentionally combines the original Sub-specs 2 (personalization) and 3 (JapaScore / eligibility / explainability) into one shippable design. Data freshness, ingestion jobs, and the AI decision agent remain out of scope and are deferred to Sub-spec 4.

---

## 1. Problem

Sub-spec 1 gives users a fast way to compare 2+ platform items in a private quick comparison. But it is still a neutral table: every item is shown with the same dimensions, and the user must manually decide which is best. The product cannot yet answer "Which is best for *me*?".

The missing pieces are:

1. **No user decision profile.** Users have no place to record their source country, target countries/cities, budget, language level, family status, and goals.
2. **No Smart Compare.** The `/compare` picker and the `/compare/[slug]` preview cannot re-rank or filter items against a profile.
3. **No JapaScore.** There is no normalized 0–100 score, no per-factor explanations, no ranked winner, and no recommendations.
4. **No admin controls for scoring.** Comparison types have `config`, but there is no concept of a *score factor* (cost, language, eligibility, etc.) or a weight.

This sub-spec closes those gaps.

---

## 2. Goal

Deliver a personalized, explainable decision layer on top of the Sub-spec 1 foundation:

1. **Comparison Profiles** — authenticated users can save one or more profiles (`source_country_id`, `target_space_ids`, `demographics`, `goals`, `preferences`) and mark one as default.
2. **Smart Compare flow** — on `/compare`, a user can select or create a profile, then create a quick comparison with `smart_mode` and `profile_id` set. The picker filters out obvious mismatches at item-selection time where resolver metadata allows it.
3. **JapaScore engine** — every item in a quick or detailed comparison can be scored 0–100 from a profile, with per-factor contributions and explanations.
4. **Ranking & recommendations** — the comparison detail view shows a ranked item strip, the matrix re-ordered by score, and actionable recommendation cards ("Apply to this university", "Book IELTS", etc.).
5. **Admin tooling** — a new `/comparison-profiles` admin page and a score-factor editor inside `/comparison-types`.
6. **Forward compatibility** — the schema and APIs are designed so Sub-spec 4 can add data sources, freshness, and AI-driven scoring without re-migrating.

---

## 3. Scope and non-scope

### In scope

- New table `comparison_profiles` and CRUD API.
- New tables `comparison_score_factors`, `comparison_scores`, `comparison_recommendations`.
- New Go package `internal/jce/scoring` (or `internal/comparison/scoring` for Sub-spec 2, extracted to `internal/jce` in Sub-spec 4) with the JapaScore engine.
- New routes under `/api/v1/jce/profiles` and `/api/v1/jce/comparisons/{id}/score|score-matrix|recommendations`.
- `apps/jce` UI: profile selector/drawer on `/compare`, Raw/Smart toggle on `/compare/[slug]` and `/comparisons/[slug]`, score strip, ranking, explanations, recommendations.
- `apps/admin` UI: `/comparison-profiles` management and score-factor editor in `/comparison-types`.
- Feature flags, RBAC nodes, Valkey keys, and catalog updates.
- Resolver metadata enrichment for scoring (cost, language, eligibility, duration, target countries).
- Backend and frontend tests.

### Out of scope

- Data ingestion jobs / `comparison_data_sources` / freshness lifecycle (Sub-spec 4).
- AI decision agent tools (`comparison.analyze`, `comparison.rank`, `comparison.explain`) beyond the `smart_mode='ai_advisor'` placeholder (Sub-spec 4).
- Real-time re-scoring via Asynq; scoring is synchronous on request for this sub-spec.
- Advanced rule language (CEL/JSON Logic) for eligibility; rules live in `comparison_type_definitions.config` JSONB.

---

## 4. Domain model

| Term | Definition |
|---|---|
| **Comparison Profile** | A user-owned decision context: source country, target spaces, demographics, goals, and per-factor preference weights. |
| **Smart Mode** | A comparison-level setting from `comparison_content_metadata.smart_mode`: `none`, `eligibility`, `cost`, `pathway`, `ai_advisor`. Drives which scoring factors are emphasized. |
| **JapaScore** | A 0–100 score for an item in the context of a profile and a smart mode. |
| **Score Factor** | An admin-configurable scoring dimension (e.g. `cost`, `language`, `eligibility`, `location_match`) with a default weight and a `data_source` mapping. |
| **Score** | A single factor/value entry in `comparison_scores` with a value, max, explanation, and version. |
| **Recommendation** | A natural-language next step generated by the scoring engine for a winning item, stored in `comparison_recommendations`. |
| **Smart View** | A view of `/compare/[slug]` or `/comparisons/[slug]` that uses the JapaScore engine to rank, filter, and explain. |

---

## 5. Schema changes

Next migration number is `000551`.

### 5.1 `comparison_profiles`

Stores user decision profiles.

```sql
-- migrations/000551_comparison_profiles.up.sql
CREATE TABLE comparison_profiles (
    id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id         uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    user_id           uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    title             text NOT NULL,
    source_country_id uuid REFERENCES countries(id) ON DELETE SET NULL,
    target_space_ids  uuid[] NOT NULL DEFAULT '{}',
    goals             jsonb NOT NULL DEFAULT '[]',
    preferences       jsonb NOT NULL DEFAULT '{}',
    demographics      jsonb NOT NULL DEFAULT '{}',
    is_default        boolean NOT NULL DEFAULT false,
    created_at        timestamptz NOT NULL DEFAULT now(),
    updated_at        timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_comparison_profiles_tenant_user ON comparison_profiles(tenant_id, user_id);
CREATE INDEX idx_comparison_profiles_default ON comparison_profiles(tenant_id, user_id) WHERE is_default = true;

-- migrations/000551_comparison_profiles.down.sql
DROP TABLE IF EXISTS comparison_profiles;
```

JSONB shapes:

- `goals`: `[{"key":"low_cost","weight":0.9},{"key":"get_pr_fast","weight":0.7}]`
- `preferences`: `{"cost":0.8,"language":0.6,"location_match":0.9}`
- `demographics`: `{"age":28,"occupation":"software_engineer","family_status":"single","language_proficiency":{"ielts":7.0},"budget_min":20000,"budget_max":50000,"budget_currency":"USD"}`

### 5.2 `comparison_score_factors`

Admin-configurable factors per comparison type (or global when `type_id IS NULL`).

```sql
-- migrations/000552_comparison_score_factors.up.sql
CREATE TABLE comparison_score_factors (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id      uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    type_id        uuid REFERENCES comparison_type_definitions(id) ON DELETE CASCADE,
    factor_key     text NOT NULL,
    label          text NOT NULL,
    description    text,
    dimension_type text,
    default_weight numeric(4,3) NOT NULL DEFAULT 1.0,
    data_source    text NOT NULL, -- e.g. 'dimension:value', 'resolver:cost_min', 'rule:age'
    config         jsonb NOT NULL DEFAULT '{}',
    version        int NOT NULL DEFAULT 1,
    created_at     timestamptz NOT NULL DEFAULT now(),
    updated_at     timestamptz NOT NULL DEFAULT now(),
    UNIQUE(tenant_id, type_id, factor_key)
);

CREATE INDEX idx_comparison_score_factors_tenant_type ON comparison_score_factors(tenant_id, type_id);

-- migrations/000552_comparison_score_factors.down.sql
DROP TABLE IF EXISTS comparison_score_factors;
```

`config` can hold rule templates or resolver hints, e.g. `{"cost_currency":"USD","missing_value_penalty":0.25}`.

### 5.3 `comparison_scores`

Per-item, per-factor, per-comparison cached scores.

```sql
-- migrations/000553_comparison_scores.up.sql
CREATE TABLE comparison_scores (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    content_id    uuid NOT NULL REFERENCES contents(id) ON DELETE CASCADE,
    item_id       uuid NOT NULL REFERENCES comparison_items(id) ON DELETE CASCADE,
    factor_id     uuid REFERENCES comparison_score_factors(id) ON DELETE SET NULL,
    score_value   numeric(5,2) NOT NULL,
    max_value     numeric(5,2) NOT NULL DEFAULT 100,
    explanation   text,
    calculated_at timestamptz NOT NULL DEFAULT now(),
    version       int NOT NULL DEFAULT 1
);

CREATE INDEX idx_comparison_scores_content ON comparison_scores(content_id);
CREATE INDEX idx_comparison_scores_item ON comparison_scores(item_id);
CREATE INDEX idx_comparison_scores_content_item ON comparison_scores(content_id, item_id);

-- migrations/000553_comparison_scores.down.sql
DROP TABLE IF EXISTS comparison_scores;
```

### 5.4 `comparison_recommendations`

Generated recommendations for a comparison.

```sql
-- migrations/000554_comparison_recommendations.up.sql
CREATE TABLE comparison_recommendations (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    content_id  uuid NOT NULL REFERENCES contents(id) ON DELETE CASCADE,
    item_id     uuid REFERENCES comparison_items(id) ON DELETE CASCADE,
    text        text NOT NULL,
    action_type text, -- 'apply', 'book', 'check', 'save', 'visit'
    action_url  text,
    priority    int NOT NULL DEFAULT 0,
    created_at  timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_comparison_recommendations_content ON comparison_recommendations(content_id);

-- migrations/000554_comparison_recommendations.down.sql
DROP TABLE IF EXISTS comparison_recommendations;
```

### 5.5 `comparison_content_metadata` and `comparison_type_definitions` config

`comparison_content_metadata.config` is the canonical per-comparison smart settings store:

```json
{
  "profile_id": "uuid",
  "smart_mode": "cost",
  "weights_snapshot": {"cost": 0.9, "language": 0.5},
  "recommendation_summary": "University A is the best fit...",
  "score_version": 1
}
```

`comparison_type_definitions.config` gets new reserved keys:

```json
{
  "allowed_goals": ["low_cost", "get_pr_fast", "scholarships", "study", "work", "family"],
  "default_smart_mode": "cost",
  "score_factor_defaults": [
    {"factor_key":"cost","label":"Cost","default_weight":0.8,"data_source":"resolver:cost_min"}
  ],
  "eligibility_rules": []
}
```

---

## 6. Backend architecture

### 6.1 Package layout

```
internal/
├── comparison/
│   ├── jce_service.go        # existing; add profile CRUD helpers
│   ├── jce_handlers.go       # existing; add profile/score route handlers
│   ├── jce_store.go          # existing; add profile/score store methods
│   ├── scoring/
│   │   ├── engine.go         # Score(ctx, contentID, profileID, smartMode) ScoreResult
│   │   ├── factors.go        # default factors, factor loading
│   │   ├── rules.go          # eligibility rule evaluation
│   │   ├── resolver_source.go # pull resolver metadata into score inputs
│   │   └── explain.go        # generate human explanations
│   └── resolver/
│       └── resolver.go       # extend ResolvedItem.Metadata schema
```

`internal/jce/scoring` may be created now or extracted later; the design uses `internal/comparison/scoring` for Sub-spec 2 to avoid a new top-level package before the intelligence layer is fully formed.

### 6.2 New service methods

In `internal/comparison/jce_service.go` or dedicated `profile_service.go`:

```go
func (s *Service) CreateProfile(ctx context.Context, tenantID, userID pgtype.UUID, input ProfileInput) (Profile, error)
func (s *Service) ListProfiles(ctx context.Context, tenantID, userID pgtype.UUID) ([]Profile, error)
func (s *Service) GetProfile(ctx context.Context, tenantID, userID, profileID pgtype.UUID) (Profile, error)
func (s *Service) UpdateProfile(ctx context.Context, tenantID, userID, profileID pgtype.UUID, input ProfileInput) (Profile, error)
func (s *Service) DeleteProfile(ctx context.Context, tenantID, userID, profileID pgtype.UUID) error
func (s *Service) SetDefaultProfile(ctx context.Context, tenantID, userID, profileID pgtype.UUID) error
```

In `internal/comparison/scoring/engine.go`:

```go
type Engine struct { ... }

func (e *Engine) Score(ctx context.Context, contentID, profileID pgtype.UUID, smartMode string) (*ScoreResult, error)
func (e *Engine) GetScore(ctx context.Context, contentID pgtype.UUID) (*ScoreResult, error)
```

`ScoreResult`:

```go
type ScoreResult struct {
    ContentID      string
    ProfileID      string
    SmartMode      string
    ScoreVersion   int
    CalculatedAt   time.Time
    Items          []ScoredItem
    Recommendations []Recommendation
}

type ScoredItem struct {
    ItemID     string
    Title      string
    TotalScore float64
    MaxScore   float64
    Rank       int
    Factors    []FactorScore
    Explanation string
    MismatchReasons []string
}

type FactorScore struct {
    FactorKey   string
    Label       string
    Weight      float64
    RawValue    float64
    MaxValue    float64
    Score       float64
    Explanation string
}
```

### 6.3 Scoring flow

1. Load the comparison, items, dimensions, and values.
2. Load the profile and the comparison type.
3. Load score factors for the type (or system defaults).
4. For each item:
   - Resolve `item_ref` to get resolver metadata.
   - Evaluate eligibility rules from `comparison_type_definitions.config` against the profile.
   - For each factor, compute a 0–100 raw sub-score using the appropriate data source:
     - `dimension:value` / `dimension:value_rating` / `dimension:value_boolean`.
     - `resolver:<path>` from `ResolvedItem.Metadata`.
     - `rule:<key>` from `config.eligibility_rules`.
   - Apply the user's `preferences` weight for that factor, or the factor's `default_weight`.
   - Sum weighted scores and divide by total weights to produce a 0–100 total.
5. Generate explanations per factor and a final natural-language explanation.
6. Generate recommendations for the top 1–3 items.
7. Write `comparison_scores` and `comparison_recommendations` for the current `score_version`.
8. Return `ScoreResult`.

Missing values are treated as 0 for the relevant factor and an explanation notes the missing data.

### 6.4 Resolver metadata enrichment

`ResolvedItem.Metadata` should include the following keys where the data exists:

- `cost_min` / `cost_max` / `cost_currency`
- `language_requirement` / `language_test`
- `eligibility_tags` (e.g. `["requires_degree","min_funds","work_experience_years"]`)
- `duration_months`
- `target_countries` (ISO-3166-1 alpha-2 array)
- `source_countries` (ISO-3166-1 alpha-2 array)
- `geo_space_id`, `country`, `country_code`, `level` (existing)
- `last_updated` (best-effort, optional)

Resolvers should be extended incrementally per entity type; missing attributes leave the field absent.

---

## 7. API endpoints

All routes are gated by `feature.comparison` + `feature.jce`. New sub-routes are additionally gated by `feature.jce.profiles`, `feature.jce.smart_compare`, and `feature.jce.scoring`.

| Method | Route | Purpose | Permission / Auth |
|---|---|---|---|
| `GET` | `/api/v1/jce/profiles` | List current user's profiles | authenticated + `jce.profile.list` |
| `POST` | `/api/v1/jce/profiles` | Create a profile | authenticated + `jce.profile.create` |
| `GET` | `/api/v1/jce/profiles/{id}` | Get one profile | authenticated + `jce.profile.view` (own or admin) |
| `PATCH` | `/api/v1/jce/profiles/{id}` | Update a profile | authenticated + `jce.profile.update` (own or admin) |
| `DELETE` | `/api/v1/jce/profiles/{id}` | Delete a profile | authenticated + `jce.profile.delete` (own or admin) |
| `POST` | `/api/v1/jce/profiles/{id}/default` | Set default profile | authenticated + `jce.profile.update` |
| `POST` | `/api/v1/jce/comparisons/{id}/score` | Recalculate JapaScore | `jce.score.recalculate` |
| `GET` | `/api/v1/jce/comparisons/{id}/score` | Get cached JapaScore | `jce.score.view` |
| `GET` | `/api/v1/jce/comparisons/{id}/score-matrix` | Get scored matrix | `jce.score.view` |
| `GET` | `/api/v1/jce/comparisons/{id}/recommendations` | Get recommendations | `jce.recommendation.view` |

`POST /api/v1/jce/comparisons` is extended to accept optional `profile_id` and `smart_mode`; when present and `depth='quick'`, both are saved into `comparison_content_metadata.config` and the comparison is created in smart mode.

### 7.1 Admin endpoints

Comparison profiles are user-owned, so the admin surface is read/delete only by super-admins:

| Method | Route | Purpose | Permission |
|---|---|---|---|
| `GET` | `/api/v1/admin/jce/profiles` | Admin list with search/filter/paginate | `jce.profile.list` (admin) |
| `GET` | `/api/v1/admin/jce/profiles/{id}` | Admin profile detail | `jce.profile.view` (admin) |
| `DELETE` | `/api/v1/admin/jce/profiles/{id}` | Admin delete | `jce.profile.delete` (admin) |

Score factors are managed through the existing `/api/v1/jce/comparison-types` endpoints by including `score_factors` in the type `config` or via dedicated sub-routes:

| Method | Route | Purpose | Permission |
|---|---|---|---|
| `GET` | `/api/v1/jce/comparison-types/{id}/factors` | List score factors for a type | `jce.factor.manage` |
| `POST` | `/api/v1/jce/comparison-types/{id}/factors` | Add score factor | `jce.factor.manage` |
| `PATCH` | `/api/v1/jce/comparison-types/factors/{id}` | Update score factor | `jce.factor.manage` |
| `DELETE` | `/api/v1/jce/comparison-types/factors/{id}` | Delete score factor | `jce.factor.manage` |

---

## 8. Frontend changes

### 8.1 `apps/jce/src/lib/types/jce.ts`

Add:

```ts
export interface ComparisonProfile {
  id: string;
  title: string;
  source_country_id?: string | null;
  target_space_ids: string[];
  goals: { key: string; weight: number }[];
  preferences: Record<string, number>;
  demographics: Record<string, unknown>;
  is_default: boolean;
  created_at: string;
}

export interface JapaScore {
  content_id: string;
  profile_id?: string | null;
  smart_mode: string;
  score_version: number;
  calculated_at: string;
  items: ScoredItem[];
  recommendations: Recommendation[];
}

export interface ScoredItem {
  item_id: string;
  title: string;
  total_score: number;
  max_score: number;
  rank: number;
  factors: FactorScore[];
  explanation: string;
  mismatch_reasons: string[];
}

export interface FactorScore { ... }
export interface Recommendation { text: string; action_type?: string; action_url?: string; priority: number; }
```

### 8.2 `/compare` page

- Add a **“Smart Compare”** panel above the item picker.
- Authenticated users see a profile selector (default profile pre-selected) and a **Create profile** button that opens a slide-over drawer.
- The drawer collects: title, source country, target countries/cities, goals (chips with weight slider), demographics, and per-factor preference weights.
- When a profile is selected, the item list can hide items whose resolver metadata clearly mismatches (e.g. the item’s `target_countries` does not include any of the profile’s target countries — only when the metadata is present).
- The submit payload to `POST /jce/comparisons` gains `profile_id` and `smart_mode`.
- On submit, the quick comparison is created with `depth='quick'`, `smart_mode`, and `profile_id` in metadata.

### 8.3 `/compare/[slug]` and `/comparisons/[slug]`

- Add a **Raw / Smart** view toggle.
- **Raw view:** existing matrix.
- **Smart view:**
  - Ranked item strip at the top: 1st, 2nd, 3rd badges with JapaScore.
  - Matrix re-ordered by rank.
  - Items below a configurable low-score threshold are hidden behind a **“Show mismatches”** button.
  - Hover/click on a score opens an explanation panel with per-factor bars and natural-language reasons.
  - Recommendation cards appear under the top item.
- For quick comparisons, only the author/admin sees the Smart toggle by default. For public detailed comparisons, the toggle is public if `feature.jce.scoring` is on.

### 8.4 `apps/jce/src/lib/i18n/en.json`

New key groups:

- `jce.profile.*` — title, create, edit, delete, source_country, target_countries, goals, preferences, budget.
- `jce.score.*` — score, rank, best_fit, explanation, factors, missing_data.
- `jce.smart.*` — smart_compare, raw, show_mismatches, hide_mismatches.
- `jce.recommendation.*` — heading, action_apply, action_book, action_check, action_save.

---

## 9. Admin changes

### 9.1 New page: `/comparison-profiles`

Uses the standard EntityPage/DataTable pattern:

- List all profiles in the tenant with search, sort, pagination.
- Columns: title, user, source country, default, created.
- Actions: view, delete (bulk delete optional).

### 9.2 `/comparison-types` enhancement

The comparison type create/edit form gains a **Score factors** tab/slide-over:

- Add/edit factor `key`, `label`, `description`, `dimension_type`, `default_weight`, `data_source`.
- List default system factors as read-only where appropriate.
- A **Goals** multi-input for `config.allowed_goals`.
- A JSON textarea for `config.eligibility_rules` (validated on save).

---

## 10. Feature flags, RBAC, and catalogs

### 10.1 New feature flags

| Flag | Default | Purpose |
|---|---|---|
| `feature.jce.profiles` | `false` | Enables profile CRUD and UI. |
| `feature.jce.smart_compare` | `false` | Enables Smart Compare item filtering and creation flow. |
| `feature.jce.scoring` | `false` | Enables JapaScore display and engine. |

### 10.2 New RBAC nodes

Seed migration `000555_seed_jce_subspec2_rbac_nodes.up.sql`:

- `jce.profile.list`, `jce.profile.view`, `jce.profile.create`, `jce.profile.update`, `jce.profile.delete`
- `jce.score.view`, `jce.score.recalculate`
- `jce.recommendation.view`
- `jce.factor.manage`

### 10.3 Valkey keys

| Key | TTL | Purpose |
|---|---|---|
| `jc:jce:profile:{tenant_id}:{user_id}:{profile_id}` | 5 min | Profile detail cache. |
| `jc:jce:score:{tenant_id}:{content_id}:{score_version}` | 15 min | Score result cache. |

### 10.4 Catalogs to update

- `.jeos/conventions/api-routes-catalog.md`
- `.jeos/conventions/valkey-keys-catalog.md`
- RBAC nodes catalog (existing convention file)
- `internal/features/registry.go`

---

## 11. AI integration

No new AI tools for this sub-spec beyond the existing `comparison.create` tool. The `ai_advisor` smart mode is a placeholder: when selected, the engine returns the same rule-based score but labels the top recommendation as “AI-assisted” and defers deep reasoning to Sub-spec 4. The `comparison_content_metadata.config` stores `smart_mode='ai_advisor'` so the UI can show the correct badge.

---

## 12. Tests

### 12.1 Backend

- `internal/comparison/scoring/engine_test.go`: table-driven tests for scoring with sample profiles, items, dimensions, and resolver metadata.
- `internal/comparison/profile_test.go`: CRUD tests, default profile logic.
- `internal/comparison/jce_test.go` additions: score routes, recommendation routes.
- `internal/comparison/resolver/*_test.go` additions: metadata fields.

### 12.2 Frontend

- `apps/jce/src/lib/api/jce.test.ts`: profile and score API client tests.
- `bun run check` and `bun run build` in `apps/jce` and `apps/admin`.

### 12.3 Integration

- Manual: create profile → create smart comparison → view `/compare/[slug]` in Smart mode → see ranked items and recommendations.
- Admin: add score factor → re-score comparison → confirm new factor influences score.

---

## 13. Exit criteria

- `comparison_profiles`, `comparison_score_factors`, `comparison_scores`, `comparison_recommendations` tables exist.
- Profile CRUD API is live and tested.
- JapaScore engine returns 0–100 item scores with per-factor explanations.
- `/compare/[slug]` Smart view ranks items and shows recommendations.
- Admin can manage profiles and score factors.
- Feature flags and catalogs are updated.
- `go test ./internal/comparison/...`, `go build ./...`, `bun run check`, `bun run build` all pass.
