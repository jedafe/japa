# Journey-Aware Content Recommendations Design

**Status:** Design — pending implementation plan
**Refs:** §05 (Geo-first Space System), §06 (Custom Fields), §14 (Content Type System), §32 (Activity Feed), §71 (Japa Readiness Assessment), §87 (Japa Stage Profiles), §70 (Visa Checklist), ADR-031, ADR-071, ADR-087, ADR-070, ADR-088

---

## 1. Problem

JapaConnect users are not generic local-news consumers. They are people in an **origin country** planning travel to a **destination country** for a specific **purpose** (study, work, business, relocation, tourism, etc.) through a **visa route**, and they sit at a specific **journey stage** (research, documents, application, approved, arrival, settling). The current feed ranking treats them as “a person in country X” — it boosts content geo-matching the visitor’s current country. That is the *destination* for diaspora but the *origin* for most users. As a result, the feed can surface content that is chronologically close but journey-irrelevant (e.g. “how to open a UK bank account” shown to someone still choosing a country, or “how to choose a university” shown to someone with a visa in hand).

## 2. Goal

Make `/feed/for-you` and `/feed/explore` recommend content whose *journey context* matches the user’s likely journey context: origin, destination, purpose, visa route, and stage. Guests get a lightweight onboarding to provide this context; authenticated users get scoring driven by their profile and their latest Japa Readiness Assessment.

## 3. Domain model

### 3.1 Journey axes

Every user session and every content item can be described by five axes:

| Axis | User side | Content side | Values |
|---|---|---|---|
| `origin_country` | Where the user is now | Where the advice starts | ISO 3166-1 alpha-2 code, e.g. `NG` |
| `destination_country` | Where the user wants to go | Which country the content is about | ISO code, e.g. `CA` |
| `purpose` | Why they are traveling | Topic of the content | `study`, `work`, `business`, `tourism`, `relocation`, `family` |
| `visa_route` | Which visa they are pursuing | Which visa the content is for | `student`, `work`, `business`, `tourist`, `spouse`, `family`, `investment`, `transit` |
| `journey_stage` | Where they are in the journey | Which stage the content targets | `considering`, `planning`, `preparing`, `applied`, `approved`, `traveling`, `settled` |

`journey_stage` should align with the state machine in §87 Japa Stage Profiles when that feature is enabled. Until then, we read and write the `japa_status` custom field, and map the legacy values `newly_japa`, `planning`, `settled` to the §87 machine.

### 3.2 Japa Readiness Assessment integration

§71 produces an `assessment_results` row per attempt with:

- `dimension_scores` (weak/strong dimensions like `documents`, `finance`, `housing`, `health`, `career`)
- `threshold_label`
- `roadmap_content` — an array of `RoadmapStep{ title, description, content_slug, dimension, priority }`

This is the richest journey signal available. We will:

1. On completion, update the user’s journey context if the assessment form captured `origin_country`, `destination_country`, `visa_type`, and `journey_stage`.
2. Boost content whose slug appears in the user’s latest `roadmap_content` (they have already been told it is their next step).
3. Use the weakest `dimension_scores` to infer which journey-stage content to prioritise (e.g. low `documents` → boost `preparing` and `applied` content; low `housing` → boost `approved` and `traveling` content).

### 3.3 Japa Stage Profiles integration

When §87 is enabled, `user_profiles` will carry `japa_stage`, `japa_stage_dest_country_id`, and `japa_stage_history`. Until then, we derive the same signal from custom fields (`japa_status`, `destination_country`, `origin_country`) plus the new `visa_route` custom field.

## 4. Schema changes

### 4.1 `contents` table

Add four optional columns. `field_values` already exists but query performance and indexing are simpler with dedicated columns.

```sql
ALTER TABLE contents
    ADD COLUMN IF NOT EXISTS journey_stage text,
    ADD COLUMN IF NOT EXISTS visa_route text,
    ADD COLUMN IF NOT EXISTS source_country text,
    ADD COLUMN IF NOT EXISTS target_country text;

CREATE INDEX IF NOT EXISTS idx_contents_journey
    ON contents(tenant_id, journey_stage, visa_route)
    WHERE deleted_at IS NULL AND status = 'published' AND visibility = 'public';

CREATE INDEX IF NOT EXISTS idx_contents_target_country
    ON contents(tenant_id, target_country)
    WHERE deleted_at IS NULL AND status = 'published' AND visibility = 'public';
```

A separate migration will backfill these values for existing content where they can be inferred from `field_values`, tags, or space assignment.

### 4.2 Content scoring / cache

No new columns in `content_scores` are strictly necessary; the score is computed at query time from the `contents` columns plus the user’s context. If query cost becomes an issue, we can later add `journey_relevance_score` and update it asynchronously when content is created or the user context changes.

### 4.3 User custom field for visa route

Create a system custom field `visa_route` (or extend the existing visa-guide request form values) so users can declare their intended visa. The existing `GetUserGeoProfileFields` query in `internal/db/queries/university.sql` already returns `japa_status`, `destination_country`, and `origin_country`; we extend it to return `visa_route`.

## 5. Backend changes

### 5.1 Visitor context

`internal/visitor/resolver.go` already resolves `OriginCountry`, `CurrentCountry`, `DestinationCountry`, `JapaStage`, `VisaRoute` (placeholder), `CurrentCity`, `OriginCity`, `DestCity`, and `Archetype`. We will:

1. Read `visa_route` from the user profile custom field.
2. Read the latest `assessment_results` for the user. If it exists, set `DestinationCountry`, `OriginCountry`, `JapaStage`, and `VisaRoute` from the form submission (if present), and attach `RoadmapSlugs` and `WeakestDimensions`.
3. For guests, read from a lightweight cookie/localStorage token (`jc:journey`) with TTL (e.g. 30 days). The token contains the five axes; it is set after the guest onboarding selector.
4. Cache the resolved `VisitorContext` in Valkey using the existing cache key pattern.

### 5.2 Content creation

`internal/content/handlers.go` will accept optional `journey_stage`, `visa_route`, `source_country`, `target_country` in the create/update payload. The handler will:

1. Persist the values.
2. Auto-infer `target_country` from the selected primary space if it is a country/state/city geo space.
3. Auto-infer `source_country` from the author’s profile origin country if not provided.
4. Allow `journey_stage` to be left empty (general content) or set to `general` (equivalent to empty).

### 5.3 Feed queries

`internal/db/queries/feeds.sql`:

1. `GetForYouCandidates` will return the four new columns.
2. `GetExploreFeed`, `GetTopFeed`, and `GetRisingFeed` will return them as well.
3. The `content_geo` CTE will remain for physical geo proximity, but a new optional `content_journey` CTE or inline join will match content to the user’s destination and route. We should avoid multiplying CTEs; instead, add the columns to the existing SELECT and let Go compute the score.

### 5.4 Feed scoring

`internal/feed/foryou.go` `computeForYouScore` will add four new signals. The existing geo cascade (city → state → country → region → global/trending) is preserved; `global` is the final fallback when no destination match is found and the user has no explicit journey context. Content with no `target_country` or `journey_stage` (or with `journey_stage = 'general'`) is never excluded — it simply receives neutral journey scores and competes on velocity, follows, and recency.

Weights are hot-reloadable through `system_settings` with sensible defaults:

| Setting | Default | Description |
|---|---|---|
| `feed.journey.destination_weight` | `2.0` | Content `target_country` equals user `destination_country` |
| `feed.journey.origin_weight` | `0.8` | Content `source_country` equals user `origin_country` |
| `feed.journey.visa_route_weight` | `1.5` | Content `visa_route` equals user `visa_route` |
| `feed.journey.journey_stage_weight` | `2.0` | Content `journey_stage` equals user `journey_stage` |
| `feed.journey.roadmap_weight` | `3.0` | Content slug in user’s latest assessment roadmap |
| `feed.journey.dimension_weight` | `1.0` | Content stage matches a weak assessment dimension |

The geo boost stays as is but the *primary* geo signal is the user’s destination country, not their current country. This means the `VisitorContext.ContentCountry` for unauthenticated `origin_visitor` archetypes should already default to `DestinationCountry` if set (this is the existing archetype logic in `internal/visitor/resolver.go`; we verify and tighten it).

### 5.5 New API endpoints

1. `GET /api/v1/feed/context` — returns the resolved `VisitorContext` so the main app can show “Showing content for Nigerians planning to study in Canada”.
2. `POST /api/v1/feed/context` (guest) — stores the five axes in a cookie-backed Valkey session.
3. `PATCH /api/v1/me/journey` (authenticated) — updates the user’s custom fields (`japa_status`, `destination_country`, `origin_country`, `visa_route`) and optional travel date.

The feed endpoints (`/feed/for-you`, `/feed/explore`) do not change their URL shape; they consume the resolved context.

## 6. Frontend changes

### 6.1 Guest onboarding

A compact `JourneyOnboardingBanner` or modal appears on first visit to `/feed/explore` when the visitor context is default (no origin/destination):

1. “Where are you now?” — country autocomplete (origin).
2. “Where do you want to go?” — country autocomplete (destination).
3. “Why are you traveling?” — purpose chips.
4. “Which visa route?” — visa route chips, filtered by purpose.
5. “Where are you in the journey?” — stage chips.

Submission calls `POST /api/v1/feed/context`, then reloads the feed. The selections are also stored in `localStorage` as a fallback and synced to the cookie.

### 6.2 Content creation

The main content creation form (and admin content forms) add an optional “Journey Context” section:

- Target country (country autocomplete, defaults to the author’s destination or selected space)
- Source country (defaults to author’s origin)
- Visa route (select)
- Journey stage (select)

These fields are pre-populated from the user’s profile to reduce friction.

### 6.3 Feed UI

Add a `JourneyContextBar` above the feed:

- Shows the resolved origin → destination, purpose, visa route, and stage.
- Allows one-tap editing.
- Highlights content cards with matching journey metadata using small badges (e.g. “Preparing · Study · Canada”).

### 6.4 Japa Readiness Tools integration

The existing Readiness Assessment already captures much of this. After a user submits the assessment, we:

1. Offer to apply the inferred journey context to their profile.
2. Pre-fill the feed with their `roadmap_content` steps.
3. Show a “Next step from your Readiness Assessment” card at the top of `/feed/for-you`.

Other tools (Visa Checklist §70, Arrival Checklist, Housing Guide §104, Scholarship Finder §73) already record origin/destination/visa. We can write a lightweight `journey:sync` handler that updates the user’s journey context whenever they use one of these tools and explicitly select a destination/visa. This is written as an asynchronous, fan-out-friendly update, not a blocking request.

## 7. Implementation phases

### Phase 1 — Schema and user context
- Migration adding the four `contents` columns and indexes.
- Add `visa_route` custom field.
- Update `GetUserGeoProfileFields` to return `visa_route`.
- Update `VisitorContext` and resolver to read assessment results and guest cookie.
- Add `GET/POST /feed/context` and `PATCH /me/journey`.

### Phase 2 — Content journey metadata
- Update content create/update handlers and API models.
- Add auto-inference from primary space and author profile.
- Backfill existing content from `field_values` / tags / spaces.

### Phase 3 — Feed scoring
- Add new weights (system settings) and scoring logic in `foryou.go`.
- Update SQL to return the new columns.
- Update cache keys to include journey-stage and destination.
- Destination-first geo boost for `origin_visitor` archetype.

### Phase 4 — Frontend
- Guest onboarding selector.
- Content creation journey section.
- Feed `JourneyContextBar` and content badges.

### Phase 5 — Readiness/Tool integration
- Sync assessment results to journey context.
- Sync other readiness tools (opt-in).
- “Next step from your Readiness Assessment” card.

### Phase 6 — Tuning and analytics
- Expose feed context/engagement metrics.
- Make weights hot-reloadable and run A/B comparisons via `system_settings`.

## 8. Testing

- Unit tests for `computeForYouScore` with journey signals.
- Unit tests for `Resolver.resolveJourneyContext` (guest cookie, assessment, profile fields).
- SQL tests for new feed query columns using a test database.
- Frontend `svelte-check` and builds for `apps/main` and `apps/admin`.
- Go `go test ./...` and `go build ./...`.

## 9. Risks and mitigations

| Risk | Mitigation |
|---|---|
| Cold-start guests have no journey context | Show the onboarding banner; default to current geo and trending. |
| Too many new weights overfit to one axis | All weights are bounded, default values are conservative, and the strongest signal is still the existing follow/affinity. |
| Backfill inference is wrong | Infer only when confidence is high (explicit tag, space, or `field_values`); otherwise leave null. |
| Assessment data is not always available | Design falls back to profile custom fields and guest cookie. |
| §87 Japa Stage columns conflict with custom fields | Use `VisitorContext` as the canonical resolver: prefer §87 columns if present and enabled, otherwise custom fields. |

## 10. Summary

The recommendation engine should become journey-aware by treating every user and every content item as a tuple of `origin → destination → purpose → visa_route → journey_stage`. The Japa Readiness Assessment and the other readiness tools already capture most of this information; the work is to (1) store it on `contents`, (2) resolve it into `VisitorContext`, and (3) add a small set of bounded weights to the existing feed scoring algorithm. The geo-first space system becomes the *destination* signal, not just the *current location* signal.
