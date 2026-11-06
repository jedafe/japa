# Japa Stage Discovery Hub Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use subagent-driven-development or executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Evolve `/japa-stage` from a flat member directory into a community-powered discovery hub (hero, stage counts, search, enriched cards, trending widgets) per `docs/superpowers/specs/2026-11-06-japa-stage-hub-design.md`.

**Architecture:** JEOS spec updates first; then Go backend (`internal/japastage` stats endpoint + enriched `/members/by-stage`; `internal/users` interests plumbing; settings catalogs); then `apps/main` page rebuild inside `AppShell` with a `routeSidebar` snippet for the two widgets.

**Tech Stack:** Go + chi + pgx (direct SQL in japastage; sqlc in users), SvelteKit 2 + Svelte 5 Runes + Bits UI + Tailwind v4, Bun, PostgreSQL.

## Global Constraints

- **JEOS before code** — spec files updated in Task 1 before any Go/Svelte.
- **Rule 16** — no hardcoded data in frontend; every option/count/list from API.
- **Rule 17** — no AI attribution anywhere (commits, comments).
- **Rule 18 / repo AGENTS** — Bun only for JS tooling; `bun install` only at repo root.
- **Rule 2** — never hand-edit `internal/db/gen/`; edit `internal/db/queries/*.sql`, run `make sqlc-gen`.
- **Rule 3** — configurable values via `system_settings` (`GetIntSetting`), no magic numbers.
- Gender stays out of the directory (PII, /me-only — migration 000882).
- Feature flag gate `japa_stage.enabled` wraps everything new on the backend.
- All commits to `main` only.
- Svelte 5 strict Runes; no `export let`, `$:`, `on:click`; i18n keys in `en.json` AND `fr.json`.

---

### Task 1: JEOS spec updates

**Files:**
- Modify: `.jeos/sections/87-japa-stage-profiles.md`
- Modify: `.jeos/conventions/api-routes-catalog.md`
- Modify: `.jeos/conventions/system-settings-catalog.md`

- [ ] **Step 1:** In §87 under "API Routes", add `GET /api/v1/japa-stage/stats — {total, stages:[{stage,count}], destinations:[{space_id,name,geo_code,count}]}; ?intent= scopes stage counts`; extend `/members/by-stage` line to `?stage=&intent=&dest=&q=&sort=(recent|stage_updated|name)&page=&page_size=`; document the enriched DTO (`bio, profession, interests, verified, cover_url, last_active_at`); add schema line `ALTER TABLE user_profiles ADD COLUMN interests text[];` and a note that `gender` is excluded from directory filters (PII, /me-only).
- [ ] **Step 2:** In api-routes-catalog.md update the §87 block (lines ~2435–2443): new stats route + new by-stage params. Same update at line ~3249 region if the intents entry needs the block kept consistent.
- [ ] **Step 3:** In system-settings-catalog.md add `japa_stage.top_destinations_limit` (int, default `5`, control) and `japa_stage.guide` (json, public, admin — how-it-works slide-over for /japa-stage).
- [ ] **Step 4: Commit** — `git add .jeos && git commit -m "jeos: japa stage hub spec — stats route, enriched directory, interests column"`

### Task 2: Migration + sqlc queries for `user_profiles.interests`

**Files:**
- Create: `migrations/001082_user_profiles_interests.up.sql`
- Create: `migrations/001082_user_profiles_interests.down.sql`
- Modify: `internal/db/queries/users.sql` (UpsertUserProfile ~line 130, GetUserProfileForUpdate ~line 154, GetUserProfile line 1)

**Interfaces:**
- Produces: `interests` column readable/writable via `gen.GetUserProfileForUpdateRow.Interests`, `gen.UpsertUserProfileParams.Interests`, `gen.GetUserProfileRow.Interests` after regen.

- [ ] **Step 1:** Write migration. Up: `ALTER TABLE user_profiles ADD COLUMN interests text[];` — Down: `ALTER TABLE user_profiles DROP COLUMN interests;`
- [ ] **Step 2:** In `internal/db/queries/users.sql`: add `interests` to `GetUserProfile` and `GetUserProfileForUpdate` SELECT lists; add `interests` to `UpsertUserProfile` INSERT column list (param `sqlc.narg(interests)`), and `interests = EXCLUDED.interests` to the ON CONFLICT SET list (unconditional — caller merges, matching education/profession semantics).
- [ ] **Step 3:** Run `make sqlc-gen` (repo rule: never hand-edit gen). Verify `gen.UpsertUserProfileParams` gains `Interests` field.
- [ ] **Step 4:** `go build ./...` — expect failures in `internal/users` where new params must be wired; that's Task 3's job. Commit migration + queries + regen: `git commit -m "db: user_profiles.interests column + sqlc wiring"`

### Task 3: `internal/users` — interests end-to-end (/me read + PATCH)

**Files:**
- Modify: `internal/users/types.go` (~line 24 `UpdateProfileRequest`, ~line 95 `UserProfileResponse`)
- Modify: `internal/users/service.go` `UpdateProfile` (~line 494)
- Modify: `internal/users/store.go` `upsertProfileTx` (~line 274) + wherever GetProfile maps row→response
- Test: `internal/users/users_test.go` (existing harness)

**Interfaces:**
- Consumes: `gen` fields from Task 2.
- Produces: `PATCH /me/profile` accepts `{"interests": ["Data Science"]}`; `GET /me` returns `"interests": [...]`. Validation: trim, case-insensitive dedupe, ≤10 items × ≤40 chars, `[]` clears; `nil` = unchanged (read-merge-upsert preserved).

- [ ] **Step 1:** `UpdateProfileRequest` += `Interests *[]string `json:"interests"``; `UserProfileResponse` += `Interests []string `json:"interests,omitempty"`` (or `*[]string` if the response uses pointers — match surrounding field style).
- [ ] **Step 2:** In `UpdateProfile`, after the `gender` merge block (~line 594), add the interests merge: `if req.Interests != nil { normalize }` — normalize = trim each, drop empties, case-insensitive dedupe, reject >10 items or any item >40 chars with `errpkg.ErrInvalid`.
- [ ] **Step 3:** `upsertProfileTx` signature += `interests []string`, pass into `gen.UpsertUserProfileParams{Interests: interests}`; update the call site in `UpdateProfile`. Map `GetUserProfileRow.Interests` → response.
- [ ] **Step 4:** Extend `users_test.go` — a PATCH-merge test: set interests, assert round-trip; assert nil leaves prior value; assert >10 items → invalid.
- [ ] **Step 5:** `go test ./internal/users/... && go build ./...` — commit `users: interests field on /me read + PATCH /me/profile`.

### Task 4: `internal/japastage` — enriched directory (`?q=`, `?sort=`, new DTO fields)

**Files:**
- Modify: `internal/japastage/store.go` (DirectoryRow ~line 55, ListByStage ~line 302, scanDirectoryRows ~line 389)
- Modify: `internal/japastage/service.go` (ListByStage ~line 540, PublicProfileDTO + directoryRowToDTO ~line 673)
- Modify: `internal/japastage/handlers.go` (HandleListByStage ~line 241)

**Interfaces:**
- Produces: `GET /members/by-stage?stage=&intent=&dest=&q=&sort=&page=&page_size=`; item shape gains `bio, profession, interests, verified, cover_url, last_active_at`. Sort values: `recent` (default), `stage_updated`, `name`.

- [ ] **Step 1:** `DirectoryRow` += `Bio, Profession *string; Interests []string; Verified bool; CoverURL *string; LastActiveAt pgtype.Timestamptz`. `PublicProfileDTO` += matching JSON fields (`verified bool`, `last_active_at string`, `interests []string`, `bio`, `profession`, `cover_url`).
- [ ] **Step 2:** `ListByStage` signature → `(ctx, stage, intent, q, sort string, destSpaceID pgtype.UUID, limit, offset int32)`. SELECT adds `up.bio, up.profession, up.interests, u.verified, up.cover_url, u.last_login_at`. `?q=` predicate (escaped wildcards — `\%`, `\_`, `\\`): `(u.username ILIKE $n ESCAPE '\' OR up.display_name ILIKE $n ESCAPE '\' OR up.profession ILIKE $n ESCAPE '\' OR EXISTS (SELECT 1 FROM unnest(up.interests) i WHERE i ILIKE $n ESCAPE '\'))`, arg = `"%"+escaped+"%"`; apply to BOTH listPredicates (aliased `up.`/`u.`) and countPredicates (count query currently selects from `user_profiles` without join — extend count query to `FROM user_profiles up JOIN users u ON u.id = up.user_id` so the same predicate text works; add `up.` prefixes to existing count predicates for consistency).
- [ ] **Step 3:** ORDER BY from `sort` whitelist: `recent` → `u.last_login_at DESC NULLS LAST, up.user_id`; `stage_updated` → current `up.japa_stage_updated_at DESC NULLS LAST, up.user_id`; `name` → `up.display_name ASC NULLS LAST, u.username`. Unknown → `recent`.
- [ ] **Step 4:** `service.ListByStage` + `directoryRowToDTO` + `HandleListByStage` (read `q`, `sort` params; `sort` validated against whitelist — invalid → `invalid_sort` 400). `formatTs` for last_active_at.
- [ ] **Step 5:** `go build ./...` — commit `japastage: enriched directory DTO, ?q= search, ?sort= ordering`.

### Task 5: `internal/japastage` — `GET /japa-stage/stats` + route + settings key

**Files:**
- Modify: `internal/japastage/store.go` (new `StageStats` method)
- Modify: `internal/japastage/service.go` (new `Stats` method — `s.settings` already injected per server.go:1788)
- Modify: `internal/japastage/handlers.go` (new `HandleStats`)
- Modify: `internal/server/server.go` (~line 8348, inside the `japa_stage.enabled` gate group)

**Interfaces:**
- Produces: `GET /api/v1/japa-stage/stats` → `{data: {total, stages: [{stage,count}], destinations: [{space_id,name,geo_code,count}]}}` (standard envelope). Accepts optional `?intent=` (validated by existing `ValidIntent`).

- [ ] **Step 1:** Store: `StageCounts(ctx, intent string) (map[string]int64, int64, error)` — `SELECT japa_stage, COUNT(*) FROM user_profiles WHERE japa_stage_is_public AND japa_stage IS NOT NULL [AND japa_intent=$1] GROUP BY japa_stage`, plus total. `TopDestinations(ctx, limit int32)` — `SELECT ds.id, ds.name, ds.geo_code, COUNT(*) FROM user_profiles up JOIN spaces ds ON ds.id=up.japa_stage_dest_space_id WHERE up.japa_stage_is_public AND up.japa_stage IS NOT NULL GROUP BY ds.id, ds.name, ds.geo_code ORDER BY COUNT(*) DESC, ds.name LIMIT $1`.
- [ ] **Step 2:** Service `Stats(ctx, intent)` — dest limit from `s.settings`/`GetIntSetting(ctx, "japa_stage.top_destinations_limit", 5)` (Rule 3); returns a `StatsDTO`.
- [ ] **Step 3:** `HandleStats` — flag check identical to other handlers; optional `?intent=` validated → `invalid_intent` 400; writes `{data: dto}`.
- [ ] **Step 4:** Register `r.Get("/japa-stage/stats", japaStageHandlers.HandleStats)` inside the gated group (server.go ~8351, next to by-stage).
- [ ] **Step 5:** `go build ./...` — commit `japastage: stats endpoint — stage counts + top destinations`.

### Task 6: Settings catalogs — `japa_stage.guide` (public) + `japa_stage.top_destinations_limit`

**Files:**
- Modify: `internal/control/settings_catalog.go` (~line 944, next to `users.guide`)
- Modify: `internal/admin/settings.go` (~line 282, guide list)

- [ ] **Step 1:** control catalog += `"japa_stage.guide": {Type: "json", DefaultValue: "", Public: true, Description: "How-it-works slide-over content for /japa-stage.", Panel: "admin"}` and `"japa_stage.top_destinations_limit": {Type: "int", DefaultValue: "5", Min: 1, Description: "Max destinations in the japa-stage stats top list.", Panel: "control"}`.
- [ ] **Step 2:** admin settings.go guide list += `{Key: "japa_stage_guide", Label: "Japa Stage Guide Panel", Type: "json", Hidden: true, StorageKey: "japa_stage.guide"}`.
- [ ] **Step 3:** `go build ./...` — commit `settings: japa_stage.guide public key + top_destinations_limit`.

### Task 7: Frontend types + `/japa-stage` server load

**Files:**
- Modify: `apps/main/src/lib/types/index.ts` (MemberStage ~line 4179; add `JapaStageStats`)
- Modify: `apps/main/src/routes/(app)/japa-stage/+page.server.ts`

**Interfaces:**
- Produces for Task 8/9: `PageData` += `stats: JapaStageStats | null`, `discussions: {id,title,url,content_type}[]`, `filters: {stage,intent,dest,q,sort}`, `canManage` (guide edit — same derivation the `/users` page uses: check its `+page.server.ts` for `canManage` source).

- [ ] **Step 1:** `MemberStage` += `bio?: string; profession?: string; interests?: string[]; verified?: boolean; cover_url?: string; last_active_at?: string;`. New `JapaStageStats = { total: number; stages: {stage: string; count: number}[]; destinations: {space_id: string; name: string; geo_code?: string; count: number}[] }`. `TrendingContentItem = {id, title, url, content_type}` if not already defined — check `types/index.ts` for an existing trending type first.
- [ ] **Step 2:** `+page.server.ts`: read `q`, `sort` params; extend `params` sent to `/members/by-stage`; add `apiGet<JapaStageStats>(ctx, '/japa-stage/stats')` and `apiGet<TrendingContentItem[]>(ctx, '/trending/content?limit=8')` to the `Promise.all` — both `.catch(() => null/[])`; keep existing error isolation. Filter discussions to `content_type` in `('discussion','forum','question')` — check actual content_type values used by `contentURL()` in recommendations handlers and match.
- [ ] **Step 3:** Check `/users/+page.server.ts` for `canManage` derivation (likely `features`/`permissions` on parent data — e.g. an `admin.settings` permission); mirror it for guide editing.
- [ ] **Step 4:** Commit `main: japa-stage load — stats, discussions, q/sort params, types`.

### Task 8: `/japa-stage` page rebuild — hero, stage strip, filters, cards, CTA

**Files:**
- Create: `apps/main/src/lib/components/heroes/HeroArtJapaStage.svelte`
- Rewrite: `apps/main/src/routes/(app)/japa-stage/+page.svelte`
- Modify: `apps/main/src/lib/enumOptions.ts` if a stage→icon map doesn't exist (icons are presentation — static map allowed)

**Interfaces:**
- Consumes: PageData from Task 7; `EntityHero`, `GuideSlideOver`, `ListPageBar`, `UrlTabs`, `Modal`, `SearchInput`, `FilterToggle`, `PageAction`, `DirectoryCard`/`UserHoverCard`, `FollowButton`, `EmptyState`, `PaginationControls`, `RelativeTime` from `$lib/components/ui`; `japaStageColors`, `japaStageLabel`, `japaStageOptions`, `japaStagesForIntent`, `japaIntentLabel`, `japaIntentOptions` from `enumOptions`; `formatMetric` from `hub/charts/format`; `routeSidebar` from `shell/routeSidebar.svelte.ts` (Task 9).
- `/users/+page.svelte` is the reference implementation for EntityHero actions, GuideSlideOver settings plumbing, and Modal-based sort/search.

- [ ] **Step 1:** `HeroArtJapaStage.svelte` — SVG dot-map + flight arc + pin scene (reuse the `HeroArtUsers` MAP pattern, journey-styled) with the photo composited on top: `<img src="/illustrations/japa-stage-hero.webp" alt="" class="absolute bottom-0 right-0 h-full w-auto object-contain" />` over the SVG inside a `relative h-full w-full` wrapper; accent-tinted.
- [ ] **Step 2:** Page skeleton mirrors `/users`: `EntityHero` (icon `Compass`, heading `japa_stage.heading`, description, breadcrumbs Home→Japa Stage, stats `[{icon: Users, value: formatMetric(data.stats?.total ?? data.total), label: 'Members'}]`, art snippet → `HeroArtJapaStage`), actions: primary `Update My Japa Stage` → `/japa-stage/me` (guests `/login?redirect=/japa-stage/me`), ghost `How it works` → `guideOpen = true`.
- [ ] **Step 3:** GuideSlideOver block copied from `/users` with `siteSettings['japa_stage.guide']`, fallback body key `japa_stage.guide_body`, `saveGuide` → `api.put('/admin/settings/general', { japa_stage_guide: v })`, `canManage={data.canManage}`.
- [ ] **Step 4:** Stage strip below hero — horizontal scrollable row: "All" + 9 stages; each item icon + label + count (`data.stats?.stages` lookup by stage value; hide count when stats null); active = `data.filters.stage`; click → `selectStage(v)` (existing URL-param navigation preserved); accent colors from `japaStageColors`.
- [ ] **Step 5:** ListPageBar — UrlTabs keep stage parity OR keep tabs minimal since strip exists: keep `UrlTabs` for consistency (tabs=All+stages); actions: FilterToggle (intent/dest panel — existing markup preserved), Sort button → Modal (options: Recently active `recent` / Stage updated `stage_updated` / Name `name`), Search button → Modal (`SearchInput` + GET form preserving stage/intent/dest/sort hidden fields). URL contract: `?stage=&intent=&dest=&q=&sort=&page=`.
- [ ] **Step 6:** Member grid — replace card body: `cover_url` banner (h-16, fallback `bg-[var(--color-surface-subtle)]` + stage-tinted gradient via `color-mix`), avatar (-mt overlap), name + verified `BadgeCheck` icon, dest flag (`destination_space_code` → flag emoji helper if one exists — check `geo.ts`/`enumOptions` for existing flag util) + `destination_space_name`, 2-line-clamp `bio`, `profession` + interests chips (first 2 + `+N`), `last_active_at` via `RelativeTime` ("Active …" label, fall back to `updated_at`), action row: `FollowButton entityType="user"` + message icon button (auth → existing DM/open-message affordance used by member cards elsewhere — check `UserHoverCard`/`/users` for the established pattern; guest → `/login`). Hidden for self. Keep `UserHoverCard` wrapper + `EmptyState`/`PaginationControls`.
- [ ] **Step 7:** Bottom CTA card — "Where are you on your Japa Journey?" heading, 3 check bullets (Get matched / Relevant opportunities / Track progress), primary button → `/japa-stage/me` or login.
- [ ] **Step 8:** i18n keys for every new string in `en.json` + `fr.json` (japa_stage.* group). `bunx svelte-check` on the app — commit `main: japa-stage hub page — hero, stage strip, rich member cards`.

### Task 9: Right-sidebar widgets (routeSidebar snippet)

**Files:**
- Modify: `apps/main/src/routes/(app)/japa-stage/+page.svelte` (register snippet)
- Optionally create: `apps/main/src/lib/components/japa-stage/JapaStageSidebar.svelte` if the snippet body is large

**Interfaces:**
- Consumes: `routeSidebar` from `$lib/components/shell/routeSidebar.svelte.ts` — pattern: `const owner = Symbol(); $effect(() => { routeSidebar.owner = owner; routeSidebar.snippet = sidebar; return () => { if (routeSidebar.owner === owner) { routeSidebar.owner = null; routeSidebar.snippet = null; } }; });` (verify exact usage in `spaces/[slug]/+page.svelte`).

- [ ] **Step 1:** Copy the registration pattern from `apps/main/src/routes/(app)/spaces/[slug]/+page.svelte` (grep `routeSidebar` — it sets owner/snippet in an effect and cleans up).
- [ ] **Step 2:** Widget 1 "Trending Destinations" — `data.stats?.destinations` rows: flag (geo_code), name linking to the space page (check how other widgets link spaces — `/spaces/[slug]`? spaces have `slug` not id — if `space_id` can't link, link to `/japa-stage?dest={space_id}`), count bar (`color-mix` accent) + count; "View all" → `/japa-stage`.
- [ ] **Step 3:** Widget 2 "Popular Discussions" — `data.discussions` rows: title linking to `item.url`, content-type label, posted-ago if available; "View all" → `/forum`. Widget shell = surface card + heading + rows, matching `RightSidebar` widget style (check an existing widget component for class conventions).
- [ ] **Step 4:** Guard: render nothing when both lists empty (stats failed) — no broken chrome. Commit `main: japa-stage sidebar — trending destinations + discussions`.

### Task 10: `/settings/profile` — interests chips

**Files:**
- Modify: `apps/main/src/lib/components/hub/settings/sections/ProfileSection.svelte`
- Modify: `apps/main/src/lib/types/index.ts` (MeProfile += `interests?: string[]`)

- [ ] **Step 1:** `MeProfile` type += `interests?: string[]` (GET /me now returns it — Task 3).
- [ ] **Step 2:** `ProfileSection.svelte`: add `interests` to the form defaults (`profile?.interests ?? []`), a chips input under profession/education — check for an existing chips/tag-input component in `$lib` first (`grep -rn "chip\|TagInput\|ChipsInput" apps/main/src/lib/components/ui/`); if none, small inline input: Enter/comma adds chip, × removes, max 10, `form.set({ interests: [...] })` — PATCH sends the array. Label key `settings.profile.interests`.
- [ ] **Step 3:** i18n keys en/fr. `bunx svelte-check` — commit `settings: interests chips on /settings/profile`.

### Task 11: Verification

- [ ] `go build ./...` clean; `go test ./internal/japastage/... ./internal/users/...` pass.
- [ ] `cd apps/main && bun run check` clean.
- [ ] Dev-server pass (main on :3000, backend :8080): `/japa-stage` renders hero + photo art, stage strip counts, search/sort/filters, enriched cards, sidebar widgets, guide slide-over, CTA; `/settings/profile` saves interests which then appear as card chips.
- [ ] `git status` clean of stray files; commits on `main`, atomic per task.

## Spec coverage checklist

- Hero + member total + photo art → Tasks 7–8 · Stage counts → 5, 8 · ?q=/?sort= → 4, 7–8 · Enriched cards → 4, 8 · Trending destinations → 5, 9 · Popular discussions → 7, 9 · Update-stage CTA → 8 · Guide slide-over → 6, 8 · Interests (settings + cards + search) → 2, 3, 4, 10 · Gender filter excluded by design · `/japa-stage/me` unchanged.
