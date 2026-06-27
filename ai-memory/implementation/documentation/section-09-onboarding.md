# Section 09 — Onboarding Flow

**Date:** 2026-06-27
**Phase:** 7
**Status:** Complete

## Overview

Section 09 implements the user onboarding experience for JapaConnect: a 5-step wizard (Welcome, Destination, Interests, Spaces, FirstAction) with server-side state persistence, progress tracking, personalized recommendations based on destination choice, and a product tour overlay that can be dismissed. The backend provides 5 endpoints for status retrieval, step updates, completion, recommendations, and tour dismissal.

## Components

### Backend (Go)

1. **internal/onboarding/types.go** — OnboardingStatus, StepData, Recommendation types
2. **internal/onboarding/store.go** — CRUD against `onboarding_sessions` table; upsert pattern for step updates
3. **internal/onboarding/service.go** — Service wrapping store with GetStatus, UpdateStep, Complete, GetRecommendations, DismissTour
4. **internal/onboarding/handlers.go** — 5 HTTP handlers for the onboarding API
5. **internal/onboarding/onboarding_test.go** — Tests for types, store, and service

### Migration

- **000031_onboarding.sql** — `onboarding_sessions` table with `current_step` (int), `completed` (bool), `step_data` (jsonb), `tour_dismissed` (bool), unique constraint on `(tenant_id, user_id)`

### Frontend (SvelteKit / Svelte 5)

1. **src/routes/onboarding/+page.server.ts** — Server load fetching onboarding status; redirects to `/` if already completed
2. **src/routes/onboarding/+page.svelte** — 5-step wizard with step state machine, back/next/complete actions
3. **src/lib/onboarding/index.ts** — Step definitions, validation rules, step labels
4. **src/lib/components/tour/ProductTour.svelte** — Overlay tour with highlighted elements and step indicators
5. **src/lib/components/tour/index.ts** — Barrel export

#### Wizard Steps
- Step 1: Welcome — greeting, brief platform description
- Step 2: Destination — select target country/city for relocation
- Step 3: Interests — select content categories of interest
- Step 4: Spaces — join recommended geo-spaces
- Step 5: FirstAction — prompt to create first post or explore feed

## Files Created

### Backend
1. `internal/onboarding/types.go`
2. `internal/onboarding/store.go`
3. `internal/onboarding/service.go`
4. `internal/onboarding/handlers.go`
5. `internal/onboarding/onboarding_test.go`
6. `migrations/000031_onboarding.sql`

### Frontend
7. `apps/main/src/routes/onboarding/+page.server.ts`
8. `apps/main/src/routes/onboarding/+page.svelte`
9. `apps/main/src/lib/onboarding/index.ts`
10. `apps/main/src/lib/components/tour/ProductTour.svelte`
11. `apps/main/src/lib/components/tour/index.ts`

## Files Modified

1. `internal/server/server.go` — Added onboarding import, service/handler constructors, 5 route registrations

## API Routes

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| GET | `/onboarding/status` | Required | Get current onboarding status |
| PUT | `/onboarding/step` | Required | Update current step data |
| POST | `/onboarding/complete` | Required | Mark onboarding as completed |
| GET | `/onboarding/recommendations` | Required | Get personalized space/content recommendations |
| POST | `/onboarding/tour/dismiss` | Required | Dismiss the product tour overlay |

## Key Decisions

1. **Step state machine** — Onboarding progress is tracked as a monotonic `current_step` integer (1-5). Steps can only move forward; going back is allowed by re-submitting earlier step data without incrementing the counter.

2. **JSONB step_data** — Each step stores its payload as a JSONB column rather than individual columns per step. This allows adding new steps or changing step fields without migrations.

3. **Completion redirect** — The `+page.server.ts` checks if onboarding is already completed and redirects to `/`. This prevents users from re-entering the wizard after finishing.

4. **Recommendations from destination** — The `GET /onboarding/recommendations` endpoint queries geo-spaces matching the user's chosen destination country/city from step 2.

5. **ProductTour as overlay** — The tour is a separate component that renders on top of the main content with highlighted element borders and a step indicator. It is dismissed via the API and stored in `tour_dismissed`.

6. **SSR-first data loading** — Onboarding status is fetched server-side in `+page.server.ts` to avoid a client-side flash of empty state.

## Verification

### Backend
- `go build ./...` — Clean build
- `go vet ./...` — No issues
- `go test ./internal/onboarding/...` — All tests pass

### Frontend
- `bun run check` — Clean
- `bun run build` — Build succeeds

## Known Deferrals

- Analytics tracking of onboarding funnel (step drop-off rates) — requires Section 40 integration
- Email follow-up for abandoned onboarding — requires email provider setup
- A/B testing of step ordering or content — requires feature flag wiring
- Localization of onboarding copy — requires i18n system (deferred)

## Next Steps

Section 26 (SEO & Performance) runs in parallel; Section 39 (Space Types & University) builds on the destination/interests data captured here.
