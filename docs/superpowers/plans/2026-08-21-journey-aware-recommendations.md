# Journey-Aware Content Recommendations — Implementation Plan

**Spec:** `docs/superpowers/specs/2026-08-21-journey-aware-recommendations-design.md`

---

## Phase 1 — Schema and user context

**Goal:** Add the database columns and resolve the user’s journey context.

1. Create migration `000531_content_journey_context.up.sql`/`.down.sql`.
   - Add `journey_stage`, `visa_route`, `source_country`, `target_country` to `contents`.
   - Add partial indexes for the new columns.
2. Create migration `000532_user_visa_route_field.up.sql`/`.down.sql`.
   - Seed a system custom field `visa_route` (select) for user profiles.
3. Update `internal/db/queries/university.sql` `GetUserGeoProfileFields` to return `visa_route`.
   - Regenerate `sqlc`.
4. Update `internal/visitor/resolver.go`.
   - Read `visa_route` from custom fields.
   - Read latest `assessment_results` and attach `RoadmapSlugs` and `WeakestDimensions`.
   - Add guest cookie/`localStorage` fallback for journey context.
5. Add API endpoints in `internal/server/server.go`.
   - `GET /api/v1/feed/context`
   - `POST /api/v1/feed/context`
   - `PATCH /api/v1/me/journey`
6. Implement handler/service logic for the new endpoints.

**Verification:** `make sqlc-gen`, `go build ./...`, `go test ./internal/visitor/...`.

---

## Phase 2 — Content journey metadata

**Goal:** Allow content to be tagged with journey context.

1. Update `internal/db/queries/contents.sql`.
   - Add columns to `InsertContent` / `UpdateContent` / `GetContent` queries.
   - Add query `BackfillContentJourney` for migration data.
2. Update `internal/db/gen/contents.sql.go` via `make sqlc-gen`.
3. Update `internal/content/content.go` models and `internal/content/handlers.go`.
   - Accept journey fields in create/update payloads.
   - Auto-infer `target_country` from primary space.
   - Auto-infer `source_country` from author origin.
4. Backfill existing content where possible.
5. Update API request/response shapes in `internal/server/handlers.go` if needed.

**Verification:** `go test ./internal/content/...`, `go build ./...`.

---

## Phase 3 — Feed scoring

**Goal:** Use journey context to score and rank content.

1. Update `internal/db/queries/feeds.sql`.
   - Add `journey_stage`, `visa_route`, `source_country`, `target_country` to all feed candidate queries.
2. Regenerate `internal/db/gen/feeds.sql.go`.
3. Update `internal/feed/foryou.go`.
   - Add `wDestinationCountry`, `wSourceCountry`, `wVisaRoute`, `wJourneyStage`, `wRoadmap`, `wDimension` as hot-reloadable settings.
   - Add fields to `Candidate`.
   - Update `computeForYouScore` to include journey signals.
   - Preserve global fallback for unmatched content.
4. Update `internal/feed/service.go` and `internal/feed/handlers.go`.
   - Pass user journey context into feed generation.
   - Update cache keys to include journey axes.
5. Update `internal/feed/foryou_test.go` with journey scoring tests.

**Verification:** `go test ./internal/feed/...`, `go build ./...`.

---

## Phase 4 — Frontend

**Goal:** Capture and display journey context in the main app.

1. `apps/main`
   - Add `JourneyOnboardingBanner` for guests.
   - Add journey fields to content creation form.
   - Add `JourneyContextBar` above feed.
   - Add journey badges to content cards.
2. `apps/admin`
   - Add journey fields to admin content edit form if applicable.
3. Update `apps/main/src/lib/api.ts` with new feed and me/journey endpoints.
4. Add i18n keys for new UI copy.

**Verification:** `bun run check` and `bun run build` in `apps/main` and `apps/admin`.

---

## Phase 5 — Readiness/Tool integration

**Goal:** Sync Japa Readiness Assessment and other tools into the journey context.

1. Update `internal/assessment/service.go` or worker.
   - After scoring, write journey context to user custom fields or fire a `journey:sync` Asynq task.
2. Add `journey:sync` Asynq task (check §11 catalog before adding).
   - Sync latest assessment results into `VisitorContext` cache.
3. Optionally sync other readiness tools on destination/visa selection.
4. Add “Next step from your Readiness Assessment” card to feed UI.

**Verification:** `go build ./...`, `go test ./internal/assessment/...`.

---

## Phase 6 — Tuning and analytics

**Goal:** Make the system observable and tunable.

1. Add `system_settings` keys for journey weights.
2. Add lightweight logging/metrics for journey context hits and misses.
3. Run manual spot checks with representative user journeys.

**Verification:** `go test ./...`, `bun run build` for frontend apps.

---

## Order of work

Start with **Phase 1** and **Phase 3** in parallel where possible, because feed scoring depends on the schema but not on the UI. Phase 2 and Phase 4 can follow. Phase 5 and Phase 6 are polish.
