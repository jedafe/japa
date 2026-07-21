# Authenticated User Hub — Design Spec

- **Date:** 2026-07-21
- **App:** Main App (`apps/main`, SvelteKit 2 · Svelte 5 Runes · SSR · port 3000)
- **Surface:** The private authenticated user hub — two route families, `/settings/*` (config) and `/dashboard/*` (activity) — rendered inside the ADR-058 shell center column.
- **Status:** Approved design; ready for implementation planning.
- **Build strategy:** One spec covering the whole hub, internally decomposed into well-bounded module surfaces sharing common infrastructure, so it ships incrementally even though it is specified together.
- **Related:** `docs/superpowers/specs/2026-07-21-create-composer-design.md` (the content manager reuses `/create`). The public profile `/u/[username]` is a separate future cycle.

---

## 1. Summary & Locked Scope

The hub is the private self-management + activity area for a signed-in user. Every scope fork was decided toward the widest option:

| Decision | Choice |
|---|---|
| Surfaces | **Both families** — `/settings` (config) **and** `/dashboard` (activity), as one hub |
| Settings sections | **All 12**: profile, account, privacy, security, connected, devices⚠️, notifications, api-keys, affiliate, wallet, subscription, danger |
| Dashboard views | **7**: overview, analytics⚠️, content, relationships, learning, certificates, knowledge-base |
| Routing | **Route-per-section** — dynamic `[slug]` + a client-meta/server-loader **two-registry** split, one per family |
| Save model | **Per-section dirty-tracked save** (`useDirtyForm`) for form sections; **immediate optimistic** actions for discrete controls |
| Blocked modules | **devices** (needs `GET /me/push-tokens`) and **analytics** (needs 5 analytics reads) are designed but `backendReady:false` |

### Non-goals
- Public profile `/u/[username]`, `/org/[slug]` (separate cycle).
- Building the Learn/Docs app surfaces (owned by `jc-learn-*` / `jc-docs-*`); the hub only deep-links to them.
- OAuth *linking* start flow for connected accounts if no start endpoint exists (unlink is in scope; see Risks).
- Admin analytics; content moderation.

---

## 2. Module Inventory & Readiness

| Family | Module (slug) | Status | Primary endpoints |
|---|---|---|---|
| settings | `profile` | ✅ | `GET /me`, `PATCH /me/profile` |
| settings | `account` | ✅ | `GET /me`, `PATCH /me/account` (incl. `ui.theme`, `ui.nav_collapsed`) |
| settings | `privacy` | ✅ | `GET /me/privacy`, `PATCH /me/privacy` |
| settings | `security` | ✅ | `POST /me/change-password`; `GET /me/totp/status`, `POST /me/totp/enable`, `POST /me/totp/confirm`, `DELETE /me/totp`; `GET /me/sessions`, `DELETE /me/sessions/{id}`, `DELETE /me/sessions` |
| settings | `connected` | ✅ | `GET /me/connected-accounts`, `DELETE /me/connected-accounts/{provider}` |
| settings | `devices` | ⚠️ **BLOCKED** | needs `GET /me/push-tokens`; existing `POST`/`DELETE /me/push-tokens/{id}` |
| settings | `notifications` | ✅ | `GET /me/notification-preferences`, `PATCH /me/notification-preferences(/{type})` |
| settings | `api-keys` | ✅ | `GET /me/api-keys(/{id})`, `POST`, `PATCH`, `DELETE`, `POST …/rotate`, `POST/DELETE …/ip-allowlist` |
| settings | `affiliate` | ✅ | `GET /me/affiliate-links(/{id})`, `POST`, `PATCH`, `DELETE` |
| settings | `wallet` | ✅ | `GET /me/wallet(/points\|/credits\|/ledger)`, `POST /me/wallet/convert` |
| settings | `subscription` | ✅ | `GET /me/subscription`, `POST /me/subscription/cancel`, `POST /me/subscription/reactivate` |
| settings | `danger` | ✅ | `POST /me/data-export`, `POST /me/deactivate`, `DELETE /me` |
| dashboard | `overview` | ✅ | `GET /me`, `GET /me/analytics` (4 counts), `GET /me/content?limit=5` |
| dashboard | `analytics` | ⚠️ **BLOCKED** | needs 5 reads (§5) |
| dashboard | `content` | ✅ | `GET /me/content?type=&status=`, `GET /me/drafts`, `GET /me/scheduled`, `POST /content/{id}/publish\|archive`, `DELETE /content/{id}` |
| dashboard | `relationships` | ✅ | `GET /me/following`, `GET /users/{me}/followers`, `GET /me/spaces(/owned\|/moderated)`, `GET /me/followed-tags` |
| dashboard | `learning` | ✅ | `GET /me/enrollments` (+ deep-link `learn.japaconnect.com`) |
| dashboard | `certificates` | ✅ | `GET /me/certificates` (+ links to `/certificates/:id`) |
| dashboard | `knowledge-base` | ✅ | `GET /me/bookmarks` (+ deep-link `docs.japaconnect.com/docs/users`) |

---

## 3. Architecture

### Route families
```
routes/(app)/
  settings/
    +page.server.ts     → redirect(307, '/settings/profile')
    +layout.svelte      → settings rail (from registry) + {@render children()}
    [section]/
      +page.server.ts   → dispatch to settingsLoaders.server.ts[section]; error(404) if unknown/!backendReady
      +page.svelte      → render settingsSections[section].component with {data}
  dashboard/
    +page.server.ts     → redirect(307, '/dashboard/overview')
    +layout.svelte      → dashboard rail (from registry) + {@render children()}
    [view]/
      +page.server.ts   → dispatch to dashboardLoaders.server.ts[view]; error(404) if unknown/!backendReady
      +page.svelte      → render dashboardViews[view].component with {data}
```

### The two-registry split (per family) — SvelteKit correctness rule
- Client meta registry (`settingsSections.ts` / `dashboardViews.ts`): `slug → { labelKey, icon, group, backendReady, component }`. Imported by the rail, the panel, and the `/…` redirect. **No server code.**
- Server loader registry (`settingsLoaders.server.ts` / `dashboardLoaders.server.ts`): `slug → (ctx) => Promise<data>`. The `.server.ts` suffix keeps API-fetch code out of the client bundle. Imported ONLY by `[slug]/+page.server.ts`.
- They share only slug strings. This prevents server-only fetch code leaking into the browser bundle — the reason for two registries rather than one.

### Shell integration
Both families render inside the shell center column as a `[rail | panel]` two-pane: `grid-template-columns: 220px 1fr`, rail `position: sticky`. Below 768px the rail collapses to a `Select`/accordion above the panel. The global right sidebar stays per the shell default (auto-hidden <1280px). No `AppShell` changes required. Left-nav + account menu link into `/settings` and `/dashboard`.

### File structure
```
src/lib/components/hub/
  useDirtyForm.svelte.ts        dirty-tracked form rune (settings crown jewel)
  optimisticAction.ts           apply→call→rollback helper (discrete actions)
  SettingsSection.svelte        form chrome: header + Save/Discard footer (dirty-gated)
  DashboardPanel.svelte         read chrome: header + slot (no footer)
  MetricCard.svelte             label + rounded number (+ delta)
  charts/                       AreaChartCard, LineChartCard, BarChartCard (built when analytics unblocks)
  settings/
    settingsSections.ts         client meta registry
    settingsLoaders.server.ts   server loader registry
    sections/                   ProfileSection, AccountSection, PrivacySection, SecuritySection,
                                ConnectedSection, DevicesSection(blocked), NotificationsSection,
                                ApiKeysSection, AffiliateSection, WalletSection, SubscriptionSection,
                                DangerSection
    flows/                      TotpEnrollFlow.svelte, ApiKeyRevealDialog.svelte,
                                DeleteAccountDialog.svelte, ChangePasswordForm.svelte
  dashboard/
    dashboardViews.ts           client meta registry
    dashboardLoaders.server.ts  server loader registry
    views/                      OverviewView, AnalyticsView(blocked), ContentManagerView,
                                RelationshipsView, LearningView, CertificatesView, KnowledgeBaseView
routes/(app)/settings/…        (as above)
routes/(app)/dashboard/…       (as above)
src/lib/api/hub.ts             typed client wrappers for hub mutations + classifyError (reuse /create's)
src/lib/i18n/en.json, fr.json  hub.* keys
```

---

## 4. Shared Primitives

### `useDirtyForm.svelte.ts` (crown jewel)
```
useDirtyForm(initial: T, save: (values: T) => Promise<Result>)
  values: T        $state seeded from the loaded snapshot
  dirty: boolean   $derived — DEEP compare values vs committed snapshot (handles nested maps)
  saving, error    $state
  save()           await save(values); on success rebase snapshot; on error classify (4xx/5xx/409)
  reset()          restore snapshot (Discard)
```
Owns a `beforeNavigate` guard when dirty. Fully unit-tested (`.svelte.ts`, mocked save).

### `optimisticAction.ts`
```
optimisticAction({ apply, rollback, call }): apply() → await call() → rollback() on error; returns result
```
Every "act now" control (revoke session, unlink, rotate/delete key, unfollow, single-toggle) uses it.

### Chrome & display
- `SettingsSection.svelte` — title/description → fields slot → sticky **Save/Discard** footer, shown only when `form.dirty`; binds a `useDirtyForm`.
- `DashboardPanel.svelte` — title + optional actions → slot; per-panel `LoadingSkeleton` at final dimensions. No footer.
- `MetricCard.svelte` — label + big rounded number (+ optional delta); SSR-rendered.
- `charts/*ChartCard.svelte` — LayerChart wrappers (fixed-height container, `var(--chart-1..5)` colors, per-chart skeleton, rounded numbers). Built when Analytics unblocks.

---

## 5. Blocked-Module Backend Contracts

The frontend for these modules is specified but implemented only when the routes exist. Registry `backendReady:false` → disabled rail entry + `error(404)` on direct nav until then.

### Devices/Push — one new route
```
GET /me/push-tokens
→ { data: [ { id, label, platform("web"|"ios"|"android"), user_agent,
             last_seen_at(RFC3339), created_at, is_current } ] }
```

### Analytics — five reads (one extended, four new); all `?range=7d|30d|90d|all`, daily-bucketed, RFC3339 dates
```
GET /me/analytics   (EXTEND existing 4 counts)
→ { data: { total_content, draft_content, published_content, total_bookmarks,
            total_views, total_reactions, total_comments, follower_count } }

GET /me/analytics/impressions?range=30d  → { data: [ { date, views } ] }              (AreaChart)
GET /me/analytics/followers?range=30d    → { data: [ { date, followers } ] }          (LineChart)
GET /me/analytics/content?range=30d      → { data: { top: [ {content_id,title,content_type,views,reactions,comments} ],
                                                      by_type: [ {content_type,views,reactions,comments} ] } }  (BarChart)
GET /me/analytics/geo?range=30d          → { data: [ { country, readers } ] }         (readers-by-country BarChart, Umami §40)
```
Each maps 1:1 to a LayerChart widget from the jc-main-develop chart table.

---

## 6. Settings Family — Data Flow & Special Flows

**Uniform pattern:** `[section]/+page.server.ts` → `settingsLoaders.server.ts[section](ctx)` → `+page.svelte` renders the component → form sections use `SettingsSection` + `useDirtyForm` (PATCH on save); action controls use `optimisticAction`.

**Four flows break the simple pattern (dedicated sub-components in `settings/flows/`):**

- **TOTP 2FA** (`TotpEnrollFlow`, in security) — local state machine `idle → enrolling(secret+otpauth shown) → confirming → enabled`; `GET /me/totp/status` seeds; `POST /me/totp/enable` (secret/URI) → `POST /me/totp/confirm{code}` (backup codes shown ONCE in a Dialog) ; `DELETE /me/totp` (confirm) to disable.
- **API keys** (`ApiKeyRevealDialog` + list, api-keys) — mini-CRUD with **secret-shown-once**: `POST`/`rotate` return the full key once → reveal Dialog ("copy now"); list shows prefix only; `PATCH` name/scopes; `DELETE` confirm; IP allowlist add/remove.
- **Danger zone** (`DeleteAccountDialog`, danger) — destructive Dialogs: data-export ("we'll email you"), deactivate (reversible confirm), **delete account** (`DELETE /me`, **type-username-to-confirm**, then logout+redirect).
- **Change password** (`ChangePasswordForm`, in security) — action form (not dirty-tracked; clears on submit): `POST /me/change-password{current,new}` + client match/strength validation.

---

## 7. Dashboard Family — Modules

- **overview** — SSR landing (redirect target): `MetricCard` row from `/me/analytics` (4 counts), recent content (`/me/content?limit=5`), links into other views.
- **content** (Content manager) — status tabs Published/Drafts/Scheduled (`/me/content?status=&type=`, `/me/drafts`, `/me/scheduled`) + content-type filter; rows with **Edit** (deep-link `/create?draft={id}`, reusing the composer), **Publish/Archive**, **Delete** (confirm); cursor pagination.
- **relationships** — tabs Following/Followers/Spaces/Tags (`/me/following`, `/users/{me}/followers`, `/me/spaces`+owned/moderated, `/me/followed-tags`); count `MetricCard`s; unfollow/leave via `optimisticAction`.
- **learning** (ADR-060) — continue-learning cards from `/me/enrollments`; **progress bars** (scalar %, no chart); **Resume** deep-links to `learn.japaconnect.com`.
- **certificates** (ADR-060) — grid from `/me/certificates`; each links to public `/certificates/:id`.
- **knowledge-base** (ADR-060) — bookmarked docs (`/me/bookmarks`) + deep-link to `docs.japaconnect.com/docs/users`.

Cross-app links use absolute subdomain URLs (shared session cookie, ADR-054); the hub does not re-implement Learn/Docs surfaces.

---

## 8. Error Handling

- **Load** — each section loader degrades independently; failed read → `EmptyState` error variant in the panel; rail + other sections unaffected. Blocked modules `error(404)`.
- **Form saves** — `useDirtyForm.save()` classifies: 4xx → inline/field message, keep dirty; 5xx/network → retry, keep dirty buffer; 409 → non-destructive "changed elsewhere" notice.
- **Optimistic actions** — rollback local state + surface error on failure.
- **Terminal action (delete account)** — typed-username confirm Dialog → success redirect to logged-out; failure stays with error. Not optimistic.
- **Secret-once** — response captured before any dismissal; failed create shows nothing.
- **Navigation** — `beforeNavigate` warns when any `useDirtyForm` is dirty.

---

## 9. Testing

- **Unit (Vitest, co-located `.test.ts`):**
  - **`useDirtyForm.svelte.test.ts` (crown jewel)** — dirty detection incl. **nested objects** (notification/privacy maps), save→rebase, reset, 4xx/5xx classification.
  - `optimisticAction.test.ts` — success (no rollback) vs error (rollback) vs return value.
  - Registries — resolution, **`backendReady` gating**, group order, unknown slug.
  - Pure helpers: password match/strength, delete-confirm typed-username matcher, content filter→query-string, analytics range→param.
- **Component (`@testing-library/svelte` + jsdom):** `SettingsSection` footer dirty-gating; TOTP `idle→enrolling→enabled`; API-key reveal shows secret once; delete-account button enables only on exact typed confirmation; content-manager publish/delete confirm.
- **Blocked modules:** registry test asserts `devices`/`analytics` 404 while `backendReady:false`; chart wrappers get number-format tests when built.
- **Contract verification (manual):** live round-trip for ready modules — edit profile→save→reload persists; toggle a notification pref; revoke a session; create+reveal an API key; delete-confirm gating (without deleting). Mock-shaped tests can't catch envelope drift (cf. the card-style double-encoding bugs).
- **Baseline:** all five suites green; `bun run test` + `bun run check` pass before done.

---

## 10. Cross-cutting Requirements
- **Svelte 5 strict Runes**; no `export let`, `$:`, `on:click`, `createEventDispatcher`, or `localStorage`/`sessionStorage` for state.
- **Bits UI** dot-notation for all interactive primitives (`Select.*`, `Tabs.*`, `Dialog.*`, `Popover.*`).
- **Tailwind v4** utilities; colors via `var(--color-*)`/`var(--chart-*)`; dark mode via `[data-theme]`; never hardcode hex.
- **LayerChart 2.0.1** for all charts (analytics module); data ONLY from API (Rule 16); fixed-height containers; per-chart skeletons; rounded numbers; dashboards carry no ad slots.
- **i18n:** every string via `t('key')`; keys added to `en.json` AND `fr.json`.
- **Rule 16:** all data from `/api/v1`; `load()` never returns static data; mutations use the client wrapper + CSRF where required.
- Every async section gets a `LoadingSkeleton`; empty/error states use `EmptyState`.
- **Cross-app links** (ADR-060): absolute subdomain URLs to `learn.`/`docs.`; do not re-implement those surfaces.

---

## 11. Acceptance Criteria
1. `/settings` and `/dashboard` render the `[rail | panel]` two-pane inside the shell; `/settings`→`/settings/profile`, `/dashboard`→`/dashboard/overview`.
2. Each ready section/view is its own SSR route loading only its data; rail highlights the active slug.
3. Form sections dirty-track: Save enables only on change (incl. nested toggles), PATCHes, and rebases; Discard restores; navigating away while dirty warns.
4. Discrete actions apply optimistically and roll back on failure (revoke session, unlink, delete key, unfollow).
5. TOTP enroll→confirm→enabled works; backup codes + new API keys are shown exactly once.
6. Delete-account requires exact typed confirmation and logs out.
7. Content manager lists by status + type and Edit deep-links into `/create?draft={id}`.
8. `devices` and `analytics` return 404 and render disabled rail entries while `backendReady:false`.
9. `bun run test` + `bun run check` green; live round-trip acceptance passes for ready modules.

---

## 12. Backend Dependencies (consolidated)
Deliver before the corresponding module unblocks (see §5 for shapes):
1. `GET /me/push-tokens` → unblocks **devices**.
2. `GET /me/analytics` (extended) + `/me/analytics/{impressions,followers,content,geo}` → unblocks **analytics**.

---

## 13. Risks & Open Questions
- **TOTP QR rendering** (minor, for the plan): render the `otpauth://` URI as a QR (needs a small self-contained QR generator — no external CDN) or offer manual secret entry only. Recommendation: manual entry now; add QR if a generator is already vendored.
- **Connected-accounts linking** — unlink (`DELETE /me/connected-accounts/{provider}`) is in scope; *linking* needs an OAuth-start endpoint not confirmed in the catalog. If absent, the section shows linked accounts + unlink only, and linking is deferred (flagged, not mocked).
- **`/me/notification-preferences` value shape** — confirm nested map structure during planning to make `useDirtyForm`'s deep-compare exact.
- **Scale** — this is a large single spec by explicit choice; the plan MUST sequence the shared infrastructure first, then modules independently, so each lands reviewed. Blocked modules (devices, analytics) sequence last.
