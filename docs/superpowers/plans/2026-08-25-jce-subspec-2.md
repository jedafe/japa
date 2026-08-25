# JCE Sub-spec 2 — Personalization & JapaScore Engine Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `subagent-driven-development` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add comparison profiles, Smart Compare, and a full JapaScore engine to the Japa Comparison Engine.

**Architecture:** New tables (`comparison_profiles`, `comparison_score_factors`, `comparison_scores`, `comparison_recommendations`) plus a new `internal/jce` package for profile/score-factor/scoring orchestration. Existing `internal/comparison` remains the generic CRUD/matrix engine. Frontend changes live in `apps/jce`; admin changes in `apps/admin`.

**Tech Stack:** Go 1.24, Chi, pgx/sqlc, SvelteKit 2 + Svelte 5 + Bits UI + Tailwind v4, Bun, Valkey.

## Global Constraints

- All commits on `main`.
- Follow `AGENTS.md`, JEOS §30, §06, §03, §14, §23, §22.
- Every new table has `tenant_id`; create via migration pair.
- Never hand-edit `internal/db/gen/*`; regenerate via `make sqlc-gen`.
- No hardcoded data in SvelteKit `load()` or components (Rule 16).
- All UI text through `t()` and added to `apps/jce/src/lib/i18n/en.json` and `apps/admin/src/lib/i18n/en.json`.
- Use `bun` for JS tooling (Rule 18); no `npm`, `pnpm`, `yarn`.
- New Valkey keys added to `.jeos/conventions/valkey-keys-catalog.md` first; use `jc:` prefix.
- New/updated routes recorded in `.jeos/conventions/api-routes-catalog.md`.
- New RBAC nodes seeded in a migration.
- New feature flags added to `internal/features/registry.go`.
- Backend errors use `errpkg.WriteError` with a `snake_case` code.
- Soft deletes for user-facing content.

---

## Task 1: Schema migrations

**Files:**
- Create: `migrations/000551_jce_profiles.{up,down}.sql`
- Create: `migrations/000552_jce_score_factors.{up,down}.sql`
- Create: `migrations/000553_jce_scores.{up,down}.sql`
- Create: `migrations/000554_jce_recommendations.{up,down}.sql`
- Create: `migrations/000555_jce_subspec2_rbac_nodes.{up,down}.sql`

**Interfaces:**
- Produces tables and RBAC nodes: `jce.profile.*`, `jce.score.*`, `jce.recommendation.view`, `jce.factor.manage`.

- [ ] **Step 1: Write 000551 `comparison_profiles` migration and down.** Include `system_settings` seeds for `jce.default_goals` and `jce.profile_cache_ttl_minutes`.
- [ ] **Step 2: Write 000552 `comparison_score_factors` migration and down.** Seed global default factors for the system tenant and add scoring config to `comparison_type_definitions.config` for `is_system = true`.
- [ ] **Step 3: Write 000553 `comparison_scores` migration and down.** Extend `comparison_content_metadata.config` with `profile_id`, `weights_snapshot`, `recommendation_summary`, `last_scored_at`.
- [ ] **Step 4: Write 000554 `comparison_recommendations` migration and down.** Add `recommendation_templates` to `comparison_type_definitions.config` for `is_system = true`.
- [ ] **Step 5: Write 000555 RBAC seed migration and down.** Insert permissions and role-permissions for `member`, `admin`, `super_admin`.
- [ ] **Step 6: Run `make migrate` and `make sqlc-gen` locally.**
- [ ] **Step 7: Commit.**

**Verification:** `make migrate` up/down clean; `go build ./...` passes after queries exist.

---

## Task 2: sqlc queries

**Files:**
- Create: `internal/db/queries/jce_profiles.sql`
- Create: `internal/db/queries/jce_score_factors.sql`
- Create: `internal/db/queries/jce_scores.sql`
- Create: `internal/db/queries/jce_recommendations.sql`

**Interfaces:**
- Consumes: Task 1 schema.
- Produces: `internal/db/gen/jce_profiles.sql.go`, `jce_score_factors.sql.go`, `jce_scores.sql.go`, `jce_recommendations.sql.go`.

- [ ] **Step 1: Write CRUD queries for `comparison_profiles`:** `CreateComparisonProfile`, `ListComparisonProfiles`, `GetComparisonProfile`, `UpdateComparisonProfile`, `DeleteComparisonProfile`, `ClearDefaultComparisonProfiles`, `SetDefaultComparisonProfile`.
- [ ] **Step 2: Write CRUD queries for `comparison_score_factors`:** `CreateScoreFactor`, `ListScoreFactors`, `GetScoreFactor`, `UpdateScoreFactor`, `DeleteScoreFactor`.
- [ ] **Step 3: Write queries for `comparison_scores`:** `UpsertComparisonScore`, `GetComparisonScores`, `DeleteComparisonScoresByContentProfile`.
- [ ] **Step 4: Write queries for `comparison_recommendations`:** `UpsertComparisonRecommendation`, `ListComparisonRecommendations`, `DeleteComparisonRecommendationsByContentProfile`.
- [ ] **Step 5: Run `make sqlc-gen`.**
- [ ] **Step 6: Commit.**

**Verification:** `go build ./...` passes; `internal/db/gen` has the new files.

---

## Task 3: `internal/jce` package skeleton

**Files:**
- Create: `internal/jce/models.go`
- Create: `internal/jce/store.go`
- Create: `internal/jce/profiles.go`
- Create: `internal/jce/score_factors.go`
- Create: `internal/jce/service.go`
- Create: `internal/jce/scoring/engine.go`
- Create: `internal/jce/scoring/factors.go`
- Create: `internal/jce/scoring/rules.go`
- Create: `internal/jce/scoring/explain.go`

**Interfaces:**
- Produces: `internal/jce.Service` with `CreateProfile`, `ListProfiles`, `GetProfile`, `UpdateProfile`, `DeleteProfile`, `SetDefaultProfile`, `ListScoreFactors`, `CreateScoreFactor`, `GetScoreFactor`, `UpdateScoreFactor`, `DeleteScoreFactor`, `ScoreComparison`, `GetComparisonScore`, `PreviewScoreForItems`, `ListRecommendations`.

- [ ] **Step 1: Write `internal/jce/models.go`** with `ComparisonProfile`, `ProfileGoal`, `ProfileDemographics`, `ScoreFactor`, `FactorScore`, `ScoredItem`, `ScoreResult`, `Recommendation`.
- [ ] **Step 2: Write `internal/jce/store.go`** wrapping `gen.Queries` for the new tables.
- [ ] **Step 3: Write `internal/jce/profiles.go`** implementing profile CRUD and default-handling.
- [ ] **Step 4: Write `internal/jce/score_factors.go`** implementing score factor CRUD.
- [ ] **Step 5: Write `internal/jce/scoring/factors.go`** with `FactorEvaluator` and evaluators for `dimension`, `resolver_metadata`, `profile`, `eligibility`, `aggregate`.
- [ ] **Step 6: Write `internal/jce/scoring/rules.go`** to parse and evaluate `eligibility_rules`.
- [ ] **Step 7: Write `internal/jce/scoring/explain.go`** for explanations and recommendation templates.
- [ ] **Step 8: Write `internal/jce/scoring/engine.go`** `Score` method that loads comparison, profile, factors, items, resolves `item_ref`, evaluates factors, computes weighted JapaScore, generates recommendations, persists results, updates metadata.
- [ ] **Step 9: Write `internal/jce/service.go`** wiring store, engine, resolver, and comparison service.
- [ ] **Step 10: Run `go build ./internal/jce/...` and add table-driven `engine_test.go`.**
- [ ] **Step 11: Commit.**

**Verification:** `go build ./...` and `go test ./internal/jce/...` pass.

---

## Task 4: `internal/jce/handlers.go` and routes

**Files:**
- Create: `internal/jce/handlers.go`
- Modify: `internal/server/server.go`
- Modify: `internal/features/registry.go`

**Interfaces:**
- Produces: routes `/jce/profiles*`, `/jce/score-factors*`, `/jce/comparisons/{id}/score`, `/jce/comparisons/{id}/recommendations`, `/jce/score-preview`.

- [ ] **Step 1: Write `internal/jce/handlers.go`** with all CRUD/scoring handlers.
- [ ] **Step 2: Add feature flags `feature.jce.profiles`, `feature.jce.smart_compare`, `feature.jce.scoring` to `internal/features/registry.go`.**
- [ ] **Step 3: Wire routes in `internal/server/server.go` inside the existing `/api/v1/jce` group with feature/permission gates.**
- [ ] **Step 4: Add `internal/jce/handlers_test.go` with `httptest` for routes and RBAC denial cases.**
- [ ] **Step 5: Run `go build ./...`, `go test ./internal/jce/...`, `make vet`.**
- [ ] **Step 6: Commit.**

**Verification:** `go test ./internal/jce/...` passes.

---

## Task 5: Resolver metadata enrichment

**Files:**
- Modify: `internal/comparison/resolver/resolver.go`
- Modify: `internal/comparison/resolver/university.go`, `space.go`, `visa.go`, `scholarship.go`, `course.go`, `job_listing.go`, `col_city.go`, `mentor.go`

**Interfaces:**
- Produces: `ResolvedItem.ScoreInputs` map for each resolver.

- [ ] **Step 1: Add `ScoreInputs map[string]any` to `ResolvedItem`.**
- [ ] **Step 2: Update `university` resolver to return `score_inputs` with cost, language, duration, target_countries, pr_path_ease.**
- [ ] **Step 3: Update `space` resolver with geo_code, level, country_code, cost.**
- [ ] **Step 4: Update `visa_route`, `col_city`, `scholarship`, `course`, `job_listing`, `mentor` resolvers with domain-relevant `score_inputs`.**
- [ ] **Step 5: Add/extend resolver tests to assert `score_inputs`.**
- [ ] **Step 6: Run `go test ./internal/comparison/resolver/...` and `go build ./...`.**
- [ ] **Step 7: Commit.**

**Verification:** Resolver tests pass.

---

## Task 6: `apps/jce` types and API client

**Files:**
- Modify: `apps/jce/src/lib/types/jce.ts`
- Modify: `apps/jce/src/lib/api/jce.ts`

**Interfaces:**
- Produces: TypeScript types and API calls for profiles, score factors, scores, recommendations.

- [ ] **Step 1: Add `ComparisonProfile`, `ProfileGoal`, `ProfileDemographics`, `ScoreFactor`, `FactorScore`, `ScoredItem`, `ScoreResult`, `ComparisonRecommendation`, `ScorePreviewInput` types.**
- [ ] **Step 2: Add `listProfiles`, `createProfile`, `getProfile`, `updateProfile`, `deleteProfile`, `setDefaultProfile`, `scoreComparison`, `getComparisonScore`, `getRecommendations`, `previewScore` functions.**
- [ ] **Step 3: Run `bun run check` in `apps/jce`.**
- [ ] **Step 4: Commit.**

**Verification:** `bun run check` passes.

---

## Task 7: `apps/jce` profile UI on `/compare`

**Files:**
- Create: `apps/jce/src/lib/components/compare/ProfileDrawer.svelte`
- Modify: `apps/jce/src/routes/compare/+page.svelte`
- Modify: `apps/jce/src/routes/compare/+page.server.ts`
- Modify: `apps/jce/src/lib/i18n/en.json`

**Interfaces:**
- Consumes: `listProfiles`, `createProfile`, `searchEntities`, `createQuickComparison`.

- [ ] **Step 1: Load profiles in `+page.server.ts` for authenticated users.**
- [ ] **Step 2: Create `ProfileDrawer.svelte` with form for title, source country, target spaces, demographics, goals with weights, factor preferences.**
- [ ] **Step 3: Add profile selector to `+page.svelte`; pass `profile_id` and `smart_mode` on quick-comparison creation.**
- [ ] **Step 4: Add `jce.profile.*` and `jce.smart.*` i18n keys.**
- [ ] **Step 5: Run `bun run check` and `bun run build` in `apps/jce`.**
- [ ] **Step 6: Commit.**

**Verification:** `bun run build` in `apps/jce` passes.

---

## Task 8: `apps/jce` Smart view and JapaScore UI on `/compare/[slug]`

**Files:**
- Create: `apps/jce/src/lib/components/compare/SmartToggle.svelte`
- Create: `apps/jce/src/lib/components/compare/ScoreBadge.svelte`
- Create: `apps/jce/src/lib/components/compare/ScoreExplanation.svelte`
- Create: `apps/jce/src/lib/components/compare/RecommendationPanel.svelte`
- Modify: `apps/jce/src/routes/compare/[slug]/+page.svelte`
- Modify: `apps/jce/src/routes/compare/[slug]/+page.server.ts`
- Modify: `apps/jce/src/lib/i18n/en.json`

**Interfaces:**
- Consumes: `scoreComparison`, `getComparisonScore`, `getRecommendations`.

- [ ] **Step 1: Load score and recommendations in `+page.server.ts` when `smart_mode` is set.**
- [ ] **Step 2: Create `SmartToggle.svelte` for Raw/Smart and profile selection.**
- [ ] **Step 3: Create `ScoreBadge.svelte` and `ScoreExplanation.svelte`.**
- [ ] **Step 4: Create `RecommendationPanel.svelte`.**
- [ ] **Step 5: Update `+page.svelte` to re-order matrix by rank, hide mismatches, show scores/explanations/recommendations.**
- [ ] **Step 6: Add `jce.score.*` and `jce.recommendation.*` i18n keys.**
- [ ] **Step 7: Run `bun run check` and `bun run build` in `apps/jce`.**
- [ ] **Step 8: Commit.**

**Verification:** `bun run build` in `apps/jce` passes.

---

## Task 9: `apps/admin` comparison profiles and score-factor editor

**Files:**
- Create: `apps/admin/src/routes/comparison-profiles/+page.svelte`
- Create: `apps/admin/src/routes/comparison-profiles/+page.server.ts`
- Modify: `apps/admin/src/routes/comparison-types/+page.svelte`
- Modify: `apps/admin/src/lib/i18n/en.json`

**Interfaces:**
- Consumes: `GET/POST/PATCH/DELETE /jce/profiles`, `GET/POST/PATCH/DELETE /jce/score-factors`.

- [ ] **Step 1: Create `/comparison-profiles` admin page listing templates with DataTable.**
- [ ] **Step 2: Extend `/comparison-types` with a **Score factors** tab and structured form/repeater for `allowed_goals`, `eligibility_rules`, `recommendation_templates`.**
- [ ] **Step 3: Add `admin.comparison_profiles.*` and `admin.comparison_types.score_factors.*` i18n keys.**
- [ ] **Step 4: Run `bun run check` and `bun run build` in `apps/admin`.**
- [ ] **Step 5: Commit.**

**Verification:** `bun run build` in `apps/admin` passes.

---

## Task 10: Catalogs and final verification

**Files:**
- Modify: `.jeos/conventions/api-routes-catalog.md`
- Modify: `.jeos/conventions/valkey-keys-catalog.md`

**Interfaces:**
- Catalog all new routes and Valkey keys.

- [ ] **Step 1: Append new `/api/v1/jce/*` routes to `api-routes-catalog.md`.**
- [ ] **Step 2: Append new Valkey keys to `valkey-keys-catalog.md`:**
  - `jc:jce:profile:{tenant_id}:{user_id}:{profile_id}` TTL 5 min.
  - `jc:jce:score:{tenant_id}:{content_id}:{score_version}` TTL 15 min.
- [ ] **Step 3: Run full verification:**
  - `make sqlc-gen && make migrate && make vet`
  - `go build ./... && go test ./internal/jce/... ./internal/comparison/...`
  - `bun run check && bun run build` in `apps/jce` and `apps/admin`.
- [ ] **Step 4: Commit.**

**Verification:** All checks pass.

---

## Task 11: Tests and QA

**Files:**
- Create/extend: `apps/jce/src/lib/api/jce.test.ts`
- Create: `internal/jce/service_test.go`

**Interfaces:**
- Backend and frontend test coverage.

- [ ] **Step 1: Extend `apps/jce/src/lib/api/jce.test.ts` with profile and score client tests.**
- [ ] **Step 2: Write `internal/jce/service_test.go` for profile CRUD, score factor CRUD, and scoring.**
- [ ] **Step 3: Manual QA: create profile, create smart comparison, view score, edit factor, re-score, verify recommendations.**
- [ ] **Step 4: Run final verification and commit.**

**Verification:** `go test ./internal/jce/...` and `bun run test` (if available) pass.

---

## Order of work

1. Task 1 — migrations
2. Task 2 — sqlc queries
3. Task 3 — `internal/jce` package
4. Task 4 — handlers and routes
5. Task 5 — resolver metadata
6. Task 6 — `apps/jce` types/API
7. Task 7 — `/compare` profile UI
8. Task 8 — `/compare/[slug]` Smart view
9. Task 9 — admin UI
10. Task 10 — catalogs and verification
11. Task 11 — tests and QA

## Exit criteria

- All migrations run clean up and down.
- `go build ./...`, `make vet`, `go test ./internal/jce/...`, `go test ./internal/comparison/...` pass.
- `bun run check` and `bun run build` pass in `apps/jce` and `apps/admin`.
- New routes are cataloged and reachable.
- JapaScore returns 0–100 item scores with explanations and recommendations.
- `/compare` supports profile creation and Smart Compare.
- Admin can manage score factors and profile templates.
