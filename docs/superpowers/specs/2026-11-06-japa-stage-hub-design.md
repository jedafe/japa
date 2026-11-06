# Japa Stage → Japa Journey Discovery Hub — Design

**Date:** 2026-11-06
**Scope:** Full stack — JEOS spec updates, Go backend (`internal/japastage` + migration), SvelteKit main app (`apps/main`)
**Spec refs:** §87 (Japa Stage Profiles), ADR-095, ADR-122, ADR-058 (app shell), ADR-064 (geo)

## Goal

Evolve `/japa-stage` from a flat member directory into a community-powered
Japa Journey Discovery Hub: hero band, interactive stage navigation with live
member counts, search + filters, enriched member cards, trending destinations,
popular discussions, and stage-update CTAs — all inside the unified app shell.

## Non-goals / explicit exclusions

- **Gender filter: dropped.** `user_profiles.gender` is PII-flagged
  (migration 000882 — surfaced only on `/me` routes). It must not become a
  public directory filter.
- No new table — `interests` is a column on `user_profiles`, consistent with
  `mentor_areas text[]`.
- No Valkey cache for the stats endpoint initially (two cheap GROUP BYs).
- `/japa-stage/me` does **not** gain an interests field — interests are edited
  at `/settings/profile` (user decision).

## Backend

### JEOS updates first (Rule: JEOS before code)

- `sections/87-japa-stage-profiles.md`: document the stats route, `?q=`/`?sort=`
  params, enriched directory DTO, and `user_profiles.interests`. Note gender
  exclusion.
- `conventions/api-routes-catalog.md`: add `GET /api/v1/japa-stage/stats`;
  update `/members/by-stage` params.
- `conventions/system-settings-catalog.md`: register
  `japa_stage.top_destinations_limit` (default 5) and `japa_stage.guide`
  (public JSON, GuideSlideOver content).
- Migrations catalog: register the new migration entry.

### Migration

```sql
ALTER TABLE user_profiles ADD COLUMN interests text[];
```

### `GET /api/v1/japa-stage/stats` (public, inside `japa_stage.enabled`)

```json
{
  "total": 12540,
  "stages": [{ "stage": "planning", "count": 3200 }],
  "destinations": [{ "space_id": "…", "name": "Canada", "geo_code": "CA", "count": 4200 }]
}
```

- `total`/`stages`: GROUP BY `japa_stage` over `user_profiles` where
  `japa_stage_is_public AND japa_stage IS NOT NULL`.
- `destinations`: join `spaces` on `japa_stage_dest_space_id`, ordered by
  count DESC, limited by `japa_stage.top_destinations_limit` (Rule 3 — from
  system settings, default 5).
- Optional `?intent=` scopes stage counts to a journey class.

### `GET /api/v1/members/by-stage` extensions

- `?q=` — escaped ILIKE over `u.username`, `up.display_name`,
  `up.profession`, and `up.interests` elements.
- `?sort=` — `recent` (default; `u.last_login_at DESC NULLS LAST`),
  `stage_updated` (`japa_stage_updated_at DESC`), `name` (`display_name ASC`).
- DTO additions (additive, non-breaking): `bio`, `profession`,
  `interests[]`, `verified` (`users.verified`), `cover_url`,
  `last_active_at` (`users.last_login_at`).

### `PATCH /me/profile`

Accept `interests` (`text[]`): trim, dedupe case-insensitively, cap ~10
items × ~40 chars; `[]` clears. Free-form chips — no option list, so no
Rule 16 conflict.

### Admin settings

Add `{Key: "japa_stage_guide", StorageKey: "japa_stage.guide", Type: "json",
Hidden: true}` to the guide-settings list in `internal/admin/settings.go` —
same pattern as `users_guide` → `users.guide`. Public via
`GET /site/settings/public` so `GuideSlideOver` can read it.

## Frontend — `apps/main`

### `/japa-stage` page (inside `AppShell` — not a shell exception)

`+page.server.ts` — one parallel wave: members (`?stage=&intent=&dest=&q=
&sort=&page=`), stats, countries, stages, intents, trending discussions.
Per-section error isolation: stats failure degrades counts to hidden, never
blanks the page; members failure keeps the existing `loadError` EmptyState.

Center column, top to bottom:

1. **`EntityHero`** (same component as `/users`, `/org`): heading "Japa
   Stage", description, breadcrumbs, member-total stat (`formatMetric`),
   `HeroArtJapaStage` art snippet (new, matching the heroes/ pattern), and
   actions — primary "Update My Japa Stage" (→ `/japa-stage/me`; guests →
   `/login?redirect=/japa-stage/me`), ghost **"How it works"** that opens
   `GuideSlideOver` fed by `siteSettings['japa_stage.guide']` with
   `users.guide`-style fallback body + admin inline edit
   (`PUT /admin/settings/general`, `japa_stage_guide`).
2. **Stage strip:** horizontal row — "All" + all 9 stages with icon, label,
   and live count from `stats.stages`; click sets `?stage=`; active stage
   highlighted via `japaStageColors`. Icons from a static presentation map
   (allowed — presentation, not data).
3. **ListPageBar + filters:** `UrlTabs` keep stage filter parity with the
   strip; `FilterToggle` opens the panel with intent select + destination
   `Autocomplete` (existing); `?q=` search and `?sort=` follow the
   `/users` modal pattern (Search/Sort buttons → `Modal`), preserving the
   existing URL-param contract.
4. **Member grid:** enriched card — `cover_url` banner (styled fallback),
   avatar, display name + verified badge, destination flag + space name,
   2-line bio, top-2 interest chips + "+N", last-active `RelativeTime`,
   message + `FollowButton` actions (guests → login redirect; hidden for
   self). Reuse `UserHoverCard` trigger pattern from the current page.
5. **Bottom CTA card:** "Where are you on your Japa Journey?" + Update My
   Japa Stage button + value-prop bullets.

### Right sidebar (`routeSidebar` snippet, ADR-058)

- **Trending Destinations** — `stats.destinations`: flag, space name, count
  bar, member count; link each to its destination space.
- **Popular Discussions** — `GET /trending/content` (public): title, space,
  reply/like counts, relative age; link to thread.

### `/settings/profile` (ProfileSection.svelte)

Add an **Interests** free-form chips input alongside profession/education,
submitted with the existing `PATCH /me/profile` save. `/japa-stage/me` is
unchanged.

### Types & i18n

- Extend `MemberStage` (`bio`, `profession`, `interests`, `verified`,
  `cover_url`, `last_active_at`); add `JapaStageStats`.
- All new strings via `$t()`; keys added to `en.json` AND `fr.json`.
- Svelte 5 Runes, Bits UI primitives, Tailwind v4 tokens, no hex colors,
  skeletons distinct from EmptyState.

## Errors & edge cases

- Standard error envelope; `invalid_stage_for_intent` 400 preserved.
- Guests: full directory readable; write actions redirect to login.
- `japa_stage.enabled=false` → existing 404 gate unchanged.
- `?q=` with no matches → existing EmptyState.

## Verification

- `go build ./...`; japastage package tests (extend if the package has a
  test harness).
- `bun run check` in `apps/main` (bun only — Rule 18 / repo AGENTS).
- Dev-server pass on `/japa-stage` (port 3000): stage strip counts, search,
  filters, sort, sidebar widgets, guide slide-over, CTA.
