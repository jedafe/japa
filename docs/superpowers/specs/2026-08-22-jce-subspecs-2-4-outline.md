# Japa Comparison Engine — Sub-specs 2–4 Outline

This is a high-level roadmap for the three follow-up sub-specs. The purpose is to validate that the Sub-spec 1 foundation (`apps/jce`, `comparison_content_metadata`, `item_ref`, resolver registry, and the `/api/v1/jce/*` API) is extensible enough to support them without rework.

---

## Sub-spec 2 — Personalization & Smart Compare

### Goal
Let users save a decision profile and run comparisons that are filtered/scored against that profile.

### User-facing behavior
- A user fills a **Comparison Profile** once: source country, target countries/cities, age, occupation, budget, language proficiency, family status, goals (e.g. "get PR fast", "low cost", "scholarships"), and weighted preferences.
- On `/compare`, the user can pick **Smart Compare** and select a profile.
- The comparison matrix can highlight which item best matches the profile, re-order items, and hide items that are clearly mismatched.
- Users can have multiple profiles and reuse them across comparisons.

### New tables/entities
- `comparison_profiles`
  - `id`, `tenant_id`, `user_id`, `title`, `source_country_id`, `target_space_ids` (JSONB), `goals` (JSONB array), `preferences` (JSONB weights), `demographics` (JSONB), `created_at`, `updated_at`.
- Extend `comparison_content_metadata.config`:
  - `profile_id`, `smart_mode`, `weights_snapshot`, `recommendation_summary`.

### New API / UI (high-level)
- `GET/POST/PATCH/DELETE /api/v1/jce/profiles`
- `/compare` Smart Compare flow: select or create profile → run comparison → show personalized results.
- `/compare/[slug]` can switch between “Raw” and “Smart” views.

### Foundation from Sub-spec 1
- `comparison_content_metadata.smart_mode` enum already has `none`, `eligibility`, `cost`, `pathway`, `ai_advisor`.
- `comparison_content_metadata.config` is JSONB and can store `profile_id` and weights.
- `item_ref` resolver registry can fetch per-item attributes needed for scoring (cost, visa, scholarships).
- `comparison_type_definitions.config` can define type-specific smart defaults and allowed dimensions.

### Gaps / things to confirm during Sub-spec 1
- `comparison_content_metadata.config` should be treated as the canonical place for per-comparison smart settings.
- `comparison_type_definitions.config` should reserve keys for `smart_mode_defaults`, `default_weights`, and `profile_questions`.
- Resolver `ResolvedItem` should include enough metadata (e.g. `geo_space_id`, country, tags) to drive personalization without extra DB calls.

---

## Sub-spec 3 — JapaScore, Eligibility, Ranking, Explainability, Recommendations

### Goal
Turn a comparison into an explainable, scored decision output.

### User-facing behavior
- Each item gets a **JapaScore** (0–100 or A–F) computed from dimensions, profile, and eligibility rules.
- Hover/expand a score to see **why** (“You qualify for this visa route because you have 2+ years of experience and enough funds.”).
- The system can **rank** items and show a winner/best fit.
- It can generate **recommendations** and **next actions**: "Apply to this university", "Book IELTS", "Check scholarship deadline".
- Support **eligibility rules**: if item X requires Y, mark items as eligible/limited/ineligible based on profile.

### New tables/entities
- `comparison_score_factors`
  - `id`, `tenant_id`, `factor_key`, `label`, `description`, `dimension_type`, `default_weight`, `data_source`, `config`, `version`.
- `comparison_scores`
  - `id`, `content_id`, `item_id`, `dimension_id`, `factor_id`, `score_value`, `max_value`, `explanation`, `calculated_at`, `version`.
- `comparison_recommendations` (or live in `comparison_content_metadata.config` if small)
  - `id`, `content_id`, `item_id`, `text`, `action_type`, `action_url`, `priority`, `created_at`.
- Eligibility rules stored as JSON in `comparison_type_definitions.config` or a new `comparison_eligibility_rules` table.

### New API / UI (high-level)
- `POST /api/v1/jce/comparisons/{id}/score` — recalculate JapaScore.
- `GET /api/v1/jce/comparisons/{id}/score` — get scored matrix with explanations.
- `GET /api/v1/jce/comparisons/{id}/recommendations`
- `/compare/[slug]` and `/comparisons/[slug]` show JapaScore, ranking, and explanations.
- Admin UI to manage score factors and eligibility rules per comparison type.

### Foundation from Sub-spec 1
- `comparison_content_metadata.score_version` is already reserved for versioning.
- `comparison_values.metadata` JSONB can store provenance and raw values used for scoring.
- `item_ref` resolvers provide per-item structured data.
- `comparison_type_definitions.config` can define default dimensions, factor keys, and eligibility rule templates.

### Gaps / things to confirm during Sub-spec 1
- `comparison_values` currently only has `created_at`. Data ingestion (Sub-spec 4) will want to know when a value was last refreshed. We can either add `updated_at` to `comparison_values` now or store `retrieved_at` inside `metadata`. Recommendation: defer `updated_at` to Sub-spec 4 unless it is trivial.
- The matrix query should be written so that adding `comparison_scores` join later is straightforward.
- `ResolvedItem` should expose a stable `id` and `geo_space_id` so scoring can link items to geography.

---

## Sub-spec 4 — Data Intelligence & AI Decision Agent

### Goal
Keep comparison data fresh, traceable, and expandable with AI-driven decision assistance.

### User-facing behavior
- Each value in a comparison shows **freshness** and **source trust**: e.g. "Updated 3 days ago from official university site" or "Estimated by Japa AI".
- Users can ask the AI: “Which is best for me?” and get a ranked, explained answer with next actions.
- The AI can auto-fill values for new dimensions from data sources.
- Data is ingested from internal tools (cost of living, visa routes, scholarships, jobs) and external sources.

### New tables/entities
- `comparison_data_sources`
  - `id`, `tenant_id`, `entity_type` (e.g. `university`), `entity_id`, `source_name`, `source_url`, `source_kind` (`official`, `third_party`, `ai_estimate`), `trust_level`, `freshness_score`, `last_verified_at`, `config`.
- `comparison_data_ingestion_jobs` / Asynq tasks for periodic refresh.
- Extend `comparison_values.metadata`:
  - `source`, `source_ref`, `retrieved_at`, `confidence`, `is_estimate`.

### New API / UI (high-level)
- `GET /api/v1/jce/comparisons/{id}/freshness` — freshness summary.
- `POST /api/v1/jce/comparisons/{id}/refresh` — trigger data refresh.
- AI tools:
  - `comparison.analyze` — analyze a comparison and profile, return scores/explanations.
  - `comparison.rank` — rank items with reasoning.
  - `comparison.explain` — explain a single score or recommendation.
  - Extend `comparison.create` (Sub-spec 1 Phase 12) to support `depth=detailed` with AI-filled body and values.

### Foundation from Sub-spec 1
- `item_ref` resolver registry is the uniform interface the AI will use to resolve entities and fetch attributes.
- `comparison_values.metadata` already exists for provenance (Section 103, `000373_comparison_provenance.up.sql`).
- `comparison_content_metadata.config` can hold agent settings (model, prompt version, data-source rules).
- `feature.jce.ai_quick` flag is the starting gate for AI quick comparisons.

### Gaps / things to confirm during Sub-spec 1
- AI tools must be added to the Asynq/Section 11 catalog before use.
- Resolver `ResolvedItem` should include `last_updated` and `data_source` hints where available.
- The `comparison.create` AI tool must support both `depth=quick` and `depth=detailed` cleanly.

---

## Foundation Validation Summary

| Future sub-spec requirement | Sub-spec 1 support | Status | Notes |
|---|---|---|---|
| Quick / detailed depth split | `comparison_content_metadata.depth` | OK | Already in schema. |
| Smart mode per comparison | `comparison_content_metadata.smart_mode` | OK | Enum ready. |
| Per-comparison config (profile, weights, agent) | `comparison_content_metadata.config` JSONB | OK | Use consistently. |
| Score versioning | `comparison_content_metadata.score_version` | OK | Already in schema. |
| Cross-domain / nested items | `comparison_items.item_ref` + resolver registry | OK | Core of Sub-spec 1. |
| Tool data in comparisons | Resolver `Search` and `Resolve` for cost of living, visa, etc. | OK | Sub-spec 1 registers resolvers. |
| Value provenance / trust | `comparison_values.metadata` JSONB | OK | Already exists. |
| User profiles | New table `comparison_profiles` | Gap | Defer to Sub-spec 2. Sub-spec 1 should not add it. |
| Score factors / scores | New tables `comparison_score_factors`, `comparison_scores` | Gap | Defer to Sub-spec 3. |
| Eligibility rules | New table or JSONB in type config | Gap | Defer to Sub-spec 3. |
| Data sources / freshness | New table `comparison_data_sources` | Gap | Defer to Sub-spec 4. Can store `retrieved_at` in `metadata` until then. |
| AI decision tools | `internal/ai` tool registry + `feature.jce.ai_quick` | OK | Sub-spec 1 adds `depth` support; advanced tools in Sub-spec 4. |

## Conclusion

The Sub-spec 1 foundation is sufficient: the metadata, `item_ref`, resolver registry, and JSONB config columns provide the right extension points for personalization, scoring, and data intelligence. The missing tables are intentionally deferred to later sub-specs and do not block Sub-spec 1.

**Recommendation:** proceed with Sub-spec 1 implementation. While it is being built, keep this outline as the north star and add the deferred tables only when their sub-spec is designed.