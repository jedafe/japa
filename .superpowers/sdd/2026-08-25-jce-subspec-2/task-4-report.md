# Task 4 Report — JapaScore Scoring Engine

## Scope

Implement the JapaScore scoring engine as specified in:

- `.superpowers/sdd/2026-08-25-jce-subspec-2/task-4-brief.md`
- `docs/superpowers/specs/2026-08-25-jce-subspec-2-design.md` §6.5

Deliverables:

- `internal/jce/scoring/engine.go`
- `internal/jce/scoring/engine_test.go`
- This report

## Files Created or Modified

| File | Change |
|------|--------|
| `internal/jce/scoring/engine.go` | Created. Contains `Engine`, `NewEngine`, `Score`, `GetScore`, `PreviewScoreForItems`, and all required helpers. |
| `internal/jce/scoring/engine_test.go` | Created. Contains the eight named tests plus supporting cases. |
| `internal/jce/models.go` | Added `TenantID` to `ScorePreviewInput` so the preview path can load the comparison type and its factors. |
| `internal/jce/profiles.go` | Added exported `ProfileFromRow` wrapper for cross-package reuse. |
| `internal/jce/score_factors.go` | Added exported `ScoreFactorFromRow` wrapper for cross-package reuse. |

## Implementation Summary

### Engine structure

```go
type Engine struct {
    read     *gen.Queries
    write    *gen.Queries
    database *db.DB
    compSvc  *comparison.Service
    r        *resolver.Registry
    settings *config.Settings
    vk       valkeycompat.Cmdable
}
```

`NewEngine` initializes reader/writer `gen.Queries` and stores the `*db.DB` for transaction support.

### Public methods

- `Score(ctx, jce.ScoreInput) (jce.ScoreResult, error)` — full scoring pipeline, persistence, metadata update, Valkey cache.
- `GetScore(ctx, jce.GetScoreInput) (jce.ScoreResult, error)` — cache-first, then DB assembly; recalculates only when no persisted rows exist.
- `PreviewScoreForItems(ctx, jce.ScorePreviewInput) (jce.ScoreResult, error)` — synthetic scoring for item refs without persistence or cache.

### Pipeline (Score)

1. Load content and enforce ownership/public visibility.
2. Load comparison items and the comparison type.
3. Resolve the requested profile (DB + optional Valkey cache) or build a default from type config.
4. Load dimension values and score factors (type-specific with global fallback, de-duplicated by `factor_key`).
5. Build a runtime `typeConfig` that merges `default_weights` with each factor's `default_weight`.
6. Resolve every `item_ref` through `resolver.Registry`.
7. Build the weight map from defaults, profile preferences, goal factor map, and smart-mode multipliers.
8. For each item × factor, compute a 0–100 factor score for `dimension`, `resolver_metadata`, `profile`, and `eligibility` data sources.
9. Compute the weighted JapaScore (aggregate) for each item.
10. Generate recommendations from `recommendation_templates` and always add a top-item recommendation.
11. Persist scores, recommendations, and updated `comparison_content_metadata` (with `profile_id`, `weights_snapshot`, `recommendation_summary`, `last_scored_at`, incremented `score_version`) inside a transaction.
12. Cache the `ScoreResult` at `jc:jce:score:{tenant_id}:{content_id}:{score_version}` with TTL from `jce.score_cache_ttl_minutes`.

### Required helpers

- `normalizeNumber(value, min, max, direction) float64`
- `compareProfileItem(profileVal, itemVal any, strategy string) float64`
- `evaluateEligibilityRules(profile, inputs, rules) (status, score, explanation)`
- `buildWeightMap(typeConfig, profile, smartMode) map[string]float64`
- `generateRecommendations(itemID, factorKey, factorScore, eligibleStatus, templates) []jce.ComparisonRecommendation`

### Valkey keys

- Score cache: `jc:jce:score:{tenant_id}:{content_id}:{score_version}`
- Profile cache (best-effort): `jc:jce:profile:{tenant_id}:{user_id}:{profile_id}`

Both use the cataloged `jc:` prefix.

## Verification Commands

```bash
go test ./internal/jce/scoring/...
go build ./...
```

### Results

```
ok  	github.com/jedafe/japa/internal/jce/scoring	0.004s
```

`go build ./...` completed with no errors.

## Test Summary

Eight named tests in `internal/jce/scoring/engine_test.go`:

- `TestEngine_NumberFactorHigherIsBetter`
- `TestEngine_NumberFactorLowerIsBetter`
- `TestEngine_MissingInputUsesMissingScore`
- `TestEngine_ProfileMatchExact`
- `TestEngine_EligibilityRuleIneligible`
- `TestEngine_SmartModeCostMultiplier`
- `TestEngine_JapaScoreWeightedAverage`
- `TestEngine_RecommendationTemplateScoreLt`

All tests pass without requiring a live database. They exercise the core helpers and the in-memory scoring pipeline.

## Deviations, Concerns, and Known Limitations

1. **Profile and factor row mapping.** The scoring package cannot call the unexported `profileFromRow` / `scoreFactorFromRow` in `internal/jce`, so small exported wrappers were added. This is the minimal change needed to reuse existing mapping logic rather than duplicate it.

2. **`ScorePreviewInput.TenantID`.** The engine needs a tenant to load the comparison type and score factors. The existing `jce.ScorePreviewInput` in `internal/jce/models.go` did not contain `TenantID`, so it was added. The service layer in Task 5 is expected to populate it from the request context.

3. **Permission checks are minimal.** `Score` and `GetScore` verify that the caller is the comparison owner or the comparison is `public`. Full RBAC enforcement (`jce.score.recalculate`, `jce.score.view`) should be handled by the HTTP handler layer as in the rest of the codebase.

4. **Recommendation `factor_id`.** `comparison_recommendations.factor_id` is stored as `NULL` for all recommendations. The domain type `jce.ComparisonRecommendation` does not currently expose a factor key, and the schema allows `factor_id` to be null. If the design later requires per-recommendation factor linkage, the domain model can be extended.

5. **Eligibility rule model.** Eligibility rules compare a profile value and an item value using `gte`, `lte`, `eq`, `in`, `gt`, `lt`, and `contains`. Literal `value` fields in rules are used as fallbacks when `item_key` or `profile_key` is absent. More complex rule shapes (e.g. age ranges, multi-clause rules) can be added by extending `evaluateRule`.

6. **Profile Valkey cache is best-effort.** It is only used when both `input.UserID` and `input.ProfileID` are present. TTL comes from `jce.profile_cache_ttl_minutes` with a 5-minute default.

7. **Type config `default_weights` merge.** The runtime weight map starts with each factor's `default_weight`, then is overridden by the type config's `default_weights`. This ensures custom or newly added factors still have a sensible weight even if the type config is empty or stale.

8. **Generated queries were not modified.** No hand-editing of `internal/db/gen/` was performed. All persistence uses existing SQLC-generated `UpsertComparisonScore`, `CreateComparisonRecommendation`, `DeleteComparisonScoresByContent`, `DeleteComparisonRecommendationsByContent`, `UpdateComparisonContentMetadata`, and read queries.

## Review Round Fixes

A second pass addressed the findings in the Task 4 review. The changes are on `main`.

### Changes made

1. **Public scoring permission clarified (`canScoreContent`).** The engine now performs a pure content-visibility check: `public` is visible to unauthenticated callers, `members` requires an authenticated caller, and `private` requires the author. A comment documents that full RBAC (`jce.score.recalculate`, `jce.score.view`, `comparison.edit_any`) is the service/handler layer's responsibility.

2. **True exact-match strategy.** `compareProfileItem` no longer falls back to `containsValue` for the `exact` strategy. New `matchExact` helper implements case-insensitive, trimmed equality for strings and exact membership for arrays. The `contains`/`in` strategy keeps substring/membership semantics.

3. **Exported engine API tests.** Added `internal/jce/scoring/engine_integration_test.go` (`//go:build integration`) with tests for `NewEngine`, `Score`, `GetScore`, and `PreviewScoreForItems` using `internal/testdb`, a `resolver.Registry` stub, and a small `TestMain` that locates the repo-root `migrations` directory. Existing `engine_test.go` gained unit tests for `matchExact`, `canScoreContent`, and `bestFitRecommendationText`.

4. **`ScoreInput.Force` removed.** The field was unused; the model now omits it and the engine no longer has a dead field.

5. **`FilterIneligible` implemented.** `scoreRun` now filters ineligible items from the final result when the active smart mode's `filter_ineligible` is `true` (e.g. `eligibility` smart mode), with a comment explaining that this affects what `Score` persists.

6. **`ScoringEnabled` enforced.** `Score` and `PreviewScoreForItems` return `errors.ErrScoringDisabled` (`scoring_disabled`) when the comparison type's `scoring_enabled` is explicitly `false`. Unset defaults to `true` so existing types keep working.

7. **Recommendation `factor_id` populated.** `jce.ComparisonRecommendation` now has a `FactorID` field. `generateRecommendations` attaches the generating factor's ID, `Score` persists it, and `GetScore` returns it.

8. **Top recommendation text no longer hardcoded.** New `bestFitRecommendationText` helper checks `recommendation_templates` for a `factor_key="japa_score"` / `condition="top"` template and falls back to the default "This is your best-fit option.".

9. **`ScorePreviewInput.TenantID` kept internal.** Added `json:"-"` tag and a comment so JSON clients are not required to send `tenant_id`; the handler/service must set it from context.

### Files modified

- `internal/jce/scoring/engine.go`
- `internal/jce/scoring/engine_test.go`
- `internal/jce/scoring/engine_integration_test.go` (new)
- `internal/jce/models.go`
- `internal/errors/sentinel.go` (new `ErrScoringDisabled`)

### Verification commands and output

```bash
gofmt -w internal/jce/scoring/engine.go internal/jce/scoring/engine_test.go \
  internal/jce/scoring/engine_integration_test.go internal/jce/models.go \
  internal/errors/sentinel.go
go build ./...
go vet ./...
go test ./internal/jce/...
go test -tags integration ./internal/jce/...
```

Output:

```
ok  	github.com/jedafe/japa/internal/jce	0.003s
ok  	github.com/jedafe/japa/internal/jce/scoring	0.003s
ok  	github.com/jedafe/japa/internal/jce	0.059s
ok  	github.com/jedafe/japa/internal/jce/scoring	0.103s
```

`go build ./...` and `go vet ./...` passed with no output. `go test -tags integration ./...` passed for `internal/jce` and `internal/jce/scoring`; the only failures were pre-existing unrelated tests in `internal/content` (`TestCreateContent_OrgAttribution_Permission`, `TestUpdateContent_OrgAttribution_Permission`).

## Commit

```
[main 7535a45d] JCE Task 4: implement JapaScore scoring engine
 5 files changed, 2050 insertions(+)
```

The work is on `main` and ready for the next phase.

## Fix Round — Review Findings

This pass addresses the remaining Task 4 review findings.

### Changes made

1. **Permission-aware scoring inputs.** `ScoreInput`, `GetScoreInput`, and `ScorePreviewInput` in `internal/jce/models.go` now carry a `PermissionChecker func(perm string) bool` field (tagged `json:"-"`) and `ScorePreviewInput` also carries `UserID`. The callback is populated by the service/handler layer and lets the engine enforce RBAC without importing `internal/rbac` directly. A nil checker is treated as a deny-all fallback.

2. **`canScoreContent` now enforces the design spec §6.5 step 1 visibility rules:**
   - The comparison author is always allowed.
   - Callers with `comparison.edit_any` are always allowed.
   - Public comparisons require an authenticated caller with the appropriate public permission (`jce.score.recalculate` for `Score`, `jce.score.view` for `GetScore` and `PreviewScoreForItems`).
   - Members comparisons require an authenticated caller.
   - Private comparisons require the author or `comparison.edit_any`.

3. **Preview permission check.** `PreviewScoreForItems` now applies `canScoreContent` against a synthetic public comparison row, requiring the caller to be authenticated and have `jce.score.view` (or `comparison.edit_any`).

4. **Symmetric `matchExact` for `[]string` vs `[]any`.** `matchExact` now normalizes `[]string` arguments to `[]any` before comparing, so a `[]string` first argument and a `[]any` second argument (e.g. profile `target_space_ids` vs item `geo_space_id`) are compared element-by-element recursively. A new unit test exercises both orderings.

### Files modified

- `internal/jce/models.go`
- `internal/jce/scoring/engine.go`
- `internal/jce/scoring/engine_test.go`
- `internal/jce/scoring/engine_integration_test.go` (updated existing tests to populate `UserID` and `PermissionChecker`)

### Verification

```bash
gofmt -w internal/jce/models.go internal/jce/scoring/engine.go internal/jce/scoring/engine_test.go internal/jce/scoring/engine_integration_test.go
go build ./...
go vet ./...
go test ./internal/jce/...
go test -tags integration ./internal/jce/scoring
```

Output:

```
ok  	github.com/jedafe/japa/internal/jce	0.002s
ok  	github.com/jedafe/japa/internal/jce/scoring	0.004s
ok  	github.com/jedafe/japa/internal/jce/scoring	0.075s
```

`go build ./...` and `go vet ./...` passed with no output.
