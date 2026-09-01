# Geo-Aware Japa Readiness Roadmap — Implementation Plan

> **Parent spec:** `docs/superpowers/specs/2026-08-21-journey-aware-recommendations-design.md`  
> **Parent plan:** `docs/superpowers/plans/2026-08-21-journey-aware-recommendations.md` (Phases 1–3 already deliver the schema, `VisitorContext`, and feed scoring)

**Goal:** Make the Japa Readiness Assessment capture the visitor’s journey context, use that context to generate a destination-, stage-, and visa-route-aware roadmap, and render the roadmap through the existing geo-aware content layer.

**Architecture:** Add journey custom fields to the default assessment form, collect them in the readiness UI, and pass them through `field_values` to the `assessment:score_submission` worker. The worker resolves the destination country to a `geoSpaceID`, passes it to the RAG retriever, and selects/filters roadmap content by `target_country`, `journey_stage`, and `visa_route`. A public `GET /api/v1/assessment/result/:id/roadmap` endpoint returns the result’s roadmap content ranked by its own journey context. The result page renders the endpoint’s content cards and shows the resolved journey context bar.

**Tech Stack:** Go, SvelteKit/Svelte 5, PostgreSQL, sqlc, Valkey, Asynq, Meilisearch (already wired).

## Global Constraints
- No hardcoded country/visa/purpose option lists in frontend (Rule 16). Options come from the API.
- All new Valkey keys use `jc:` prefix and are added to `conventions/valkey-keys-catalog.md`.
- API errors follow `conventions/api-response-format.md`.
- Do not edit `internal/db/gen` directly; edit `.sql` queries and run `make sqlc-gen`.
- Use `bun` for all JS/TS tooling.
- All commits to `main`.

---

## Task 1: Add journey custom fields to the default assessment form

**Files:**
- Create: `migrations/000536_assessment_journey_fields.up.sql`, `migrations/000536_assessment_journey_fields.down.sql`
- Modify: `migrations/000279_seed_default_assessment_form.up.sql`

**Work:**
- Add five custom fields to the default assessment field group (`japa_readiness_assessment_fields`):
  - `destination_country` — `select`, `config: { weight: 0, dimension: "" }` (non-scored; the frontend loads country options from the API)
  - `origin_country` — `select`, non-scored
  - `visa_type` — `select`, non-scored
  - `journey_stage` — `select`, non-scored
  - `purpose` — `select`, non-scored
- Ensure these keys match what `internal/visitor/resolver.go:parseAssessmentSubmission` already reads.
- Update the migration idempotently (ON CONFLICT DO NOTHING).
- For `journey_stage`, seed the canonical §87 option list (`considering`, `planning`, `preparing`, `applied`, `approved`, `relocated`, `settled`).
- For `visa_type` and `purpose`, seed an initial option list that mirrors `GET /api/v1/feed/journey-options` defaults; the frontend may still call the API and merge.
- For `destination_country` and `origin_country`, leave `options` empty or minimal; the readiness UI will load the country space list and render a country picker.

**Verification:**
- `make migrate` (or equivalent) applies cleanly.
- `bun .playwright-audit-readiness-...ts` or direct page load shows the new fields (or hidden pre-form inputs).

---

## Task 2: Score submissions without breaking existing scoring

**Files:**
- Modify: `internal/assessment/service.go`

**Work:**
- Verify `computeScore` skips any field whose `FieldScoringConfig.Dimension` is empty.
- Add a `parseJourneyFields` helper that extracts `destination_country`, `origin_country`, `visa_type`, `journey_stage`, and `purpose` from the submission `field_values`.
- Update `ScoreSubmissionInput` or `ScoreSubmission` to carry the parsed journey fields forward.

**Verification:**
- `go test ./internal/assessment/...` still passes.
- New submission with journey fields produces the same dimension scores as before.

---

## Task 3: Resolve destination to a geo space and pass to RAG/AI

**Files:**
- Modify: `internal/assessment/service.go`, `internal/assessment/store.go`, `internal/db/queries/assessment.sql`

**Work:**
- Add `internal/db/queries/assessment.sql` query `GetSpaceIDByGeoCode` (or reuse an existing space query) to map a destination country ISO code to a `spaces.id`.
- In `ScoreSubmission`:
  - Parse journey fields from `field_values`.
  - If `destination_country` is set, look up its `geoSpaceID`.
  - If `destination_country` is empty but the request/user has a `VisitorContext` (this only works for in-request scoring; the worker is async, so prefer `field_values`), fall back to that.
  - Pass `geoSpaceID` to `generateAIRoadmapSteps`.
- Update `generateAIRoadmapSteps`:
  - Accept `geoSpaceID` and the parsed journey fields.
  - Pass `geoSpaceID` to `s.rag.Retrieve`.
  - Include origin, destination, visa, stage, and purpose in the AI prompt.

**Verification:**
- `make sqlc-gen` succeeds.
- `go build ./...` succeeds.
- Unit test: mock RAG with/without `geoSpaceID` and assert the prompt contains destination.

---

## Task 4: Make `GenerateRoadmap` and the curated library destination-aware

**Files:**
- Modify: `internal/assessment/service.go`

**Work:**
- Extend `RoadmapStep` with optional `target_country`, `journey_stage`, and `visa_route` fields (JSON-only; no DB schema change beyond `roadmap_content` JSONB).
- Add a `JourneyContext` input struct (or use the parsed fields directly) containing destination, origin, stage, visa, purpose.
- Update `GenerateRoadmap` to accept the journey context:
  - Load the curated library (admin `scoring_config.curated_roadmap` or default fallback).
  - For each dimension, filter steps by `target_country` (empty or matching destination), `journey_stage` (empty or matching stage), and `visa_route` (empty or matching visa).
  - If no destination-specific step matches, fall back to a generic step (empty `target_country`).
  - Preserve ordering by weakest dimension first.
- Update `curatedRoadmapLibrary()` to return steps with empty `target_country` as the global defaults. Over time, admin `scoring_config.curated_roadmap` can hold per-destination overrides by including `target_country` in the step JSON.

**Verification:**
- `go test ./internal/assessment/...` with a test that a `destination_country=CA` context prefers a step whose `target_country=CA`.

---

## Task 5: Resolve destination-specific content slugs from the corpus

**Files:**
- Create/Modify: `internal/db/queries/assessment.sql` or `internal/db/queries/contents.sql`
- Modify: `internal/assessment/store.go`, `internal/assessment/service.go`

**Work:**
- Add a query `FindRoadmapContentByDimension` (or extend an existing content list query) that, given a `dimension`, `target_country`, `journey_stage`, `visa_route`, and `guide_category='roadmap'`, returns published content rows ordered by relevance (target match first, then global).
- In `GenerateRoadmap`, after filtering the curated library, for each step whose `content_slug` is a generic “family” slug, check whether a more specific content item exists for the destination/stage/visa. If yes, prefer that content. This can be optional and controlled by a system setting `assessment.geo_content_resolution_enabled` (default `true`).
- Store the final `content_slug` in the `assessment_results.roadmap_content` JSONB.

**Verification:**
- `make sqlc-gen`.
- `go build ./...`.
- Test with a generic and a Canada-specific guide; the Canada guide is selected when destination is CA.

---

## Task 6: Add `GET /api/v1/assessment/result/:id/roadmap` endpoint

**Files:**
- Modify: `internal/assessment/handlers.go`, `internal/assessment/service.go`, `internal/server/server.go`, `internal/feed/service.go`

**Work:**
- In `internal/server/server.go`, register a new public route under the assessment group:
  - `r.Get("/assessment/result/{id}/roadmap", assessmentHandlers.GetResultRoadmap)`
  - OptionalAuth so it works for anonymous results.
- Add `GetResultRoadmap` handler:
  - Load the assessment result by ID.
  - Enforce the same ownership rules as `GetResult` (public for anonymous, owner-only for user results).
  - Build a `visitor.VisitorContext` from the result’s submission `field_values` using the same parsing logic as `internal/visitor/resolver.go`.
  - Call `feed.Service.GetRoadmapFeed` with the result’s `roadmap_content` slugs and the temporary context.
  - Return `{ data: { context: {...}, items: [...] } }`.
- Update `internal/feed/service.go`:
  - Add a method `GetRoadmapFeedForContext` (or overload) that accepts a `visitor.VisitorContext` instead of relying on the auth user’s latest result.
  - Apply a `journeyBoost`-style reordering to the returned content rows using the context’s destination, stage, visa, and source country.

**Verification:**
- `curl` against the endpoint for an anonymous and a user result; both return content items.
- `go test ./internal/feed/...`.

---

## Task 7: Update the readiness UI to collect and display journey context

**Files:**
- Modify: `apps/main/src/routes/(app)/tools/readiness/+page.svelte`, `apps/main/src/routes/(app)/tools/readiness/+page.server.ts`, `apps/tools/src/routes/readiness/+page.svelte`, `apps/tools/src/routes/readiness/+page.server.ts`

**Work:**
- In `+page.server.ts`:
  - Load journey option lists from `GET /api/v1/feed/journey-options` and country list from `GET /api/v1/spaces?level=country` (or another existing country API).
  - Return them in page data so the form can render without hardcoding.
- In `+page.svelte`:
  - Render the new journey fields before or alongside the scored questions.
  - For `destination_country` and `origin_country`, use a select/autocomplete populated from the country list.
  - For `journey_stage`, use the canonical option list from the API.
  - For `visa_type` and `purpose`, use the options from `feed/journey-options`.
  - Ensure `buildFieldValues()` includes the new custom fields (it already iterates `form.fields`).
  - After loading a result, call `GET /api/v1/assessment/result/:id/roadmap` and render the returned content cards in roadmap order.
  - Add a `JourneyContextBar` above the result view showing origin → destination, stage, visa, purpose.
- In `apps/tools` readiness page, mirror the changes where feasible (form inputs, result rendering from the new endpoint).

**Verification:**
- `bun run check` in `apps/main` and `apps/tools`.
- Playwright: submit with destination=CA and assert the roadmap contains country-specific content.

---

## Task 8: Seed and tag roadmap guide content

**Files:**
- Modify: `migrations/000535_seed_roadmap_guides.up.sql`

**Work:**
- Update the seeded guides to set `journey_stage`, `visa_route`, `source_country`, and `target_country` where appropriate.
- Most current guides are global (`target_country` NULL), so they remain the fallback.
- If new destination-specific guides are added later, the scoring engine (Task 5) will pick them up automatically.

**Verification:**
- `make migrate` (or test migration) succeeds.
- Seeded guides have `target_country` NULL and relevant `journey_stage`/`visa_route` values.

---

## Task 9: Update `GET /api/v1/feed/roadmap` to use the existing `VisitorContext`

**Files:**
- Modify: `internal/feed/handlers.go`, `internal/feed/service.go`

**Work:**
- The authenticated `GET /api/v1/feed/roadmap` already loads the user’s latest roadmap slugs from `assessment_results`.
- Ensure the service applies the same `journeyBoost` pass using the resolved `VisitorContext` (which already contains destination, stage, visa, source, and roadmap slugs).
- This makes the feed endpoint itself geo-aware for authenticated users.

**Verification:**
- `go test ./internal/feed/...`.

---

## Task 10: Verification and cleanup

**Work:**
- `go build ./...`
- `bun run check` in `apps/main` and `apps/tools`
- Playwright: test anonymous and authenticated assessment flows with/without destination.
- Update `.jeos/conventions/valkey-keys-catalog.md` if any new Valkey keys are introduced.
- Update `.jeos/conventions/system-settings-catalog.md` if new settings are added.
- Update `sections/71-readiness-assessment.md` and `adr/ADR-071-japa-readiness-assessment.md` to document the journey context behavior (optional but recommended).

**Order of work:**
1. Task 1 (schema)
2. Tasks 2, 3, 4, 5 (backend scoring) in sequence
3. Task 6 (backend API)
4. Task 7 (frontend)
5. Task 8 (content seeding)
6. Task 9 (auth feed)
7. Task 10 (verification)

---

## Notes
- The `VisitorContext` and `journeyBoost` code already exists in `internal/visitor` and `internal/feed`. This plan reuses it rather than rebuilding.
- The assessment is a `form` with `form_type='assessment'`. Adding non-scored fields is the simplest way to capture journey context without changing the Form Builder.
- The result page currently renders `result.roadmap` directly. Task 6 and 7 move it to the geo-aware content layer.
