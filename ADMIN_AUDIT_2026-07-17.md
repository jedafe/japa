# Admin Panel Audit — 2026-07-17

**Scope:** 80+ routes in `apps/admin` (SvelteKit SPA). Ran `jc-admin-debug` skill across four parallel audit agents + direct verification.
**Reference-compliant pages:** `tags`, `moderation`, `users/groups`, `users` (partial).

## Gates

- **Gate 0 (backend routes): PASS** — all flagged entities have `GET/POST/PATCH/DELETE /api/v1/admin/*` handlers in `internal/server/server.go`.
- **Gate 2 (RBAC CRUD): PASS** — backend CRUD exists for every entity. All "missing actions" are **frontend gaps, not missing endpoints**. No pause needed.

---

## BLOCKERS

### 1. `window.confirm` (forbidden) instead of Modal/Dialog
- `broadcast/+page.svelte:97` → `if (!confirm(t('broadcast.confirm_delete'))) return;`
- `page-builder/pages/+page.svelte:91` → `if (!confirm(t('page_builder.confirm_delete'))) return;`

Fix: replace with `<Modal>` confirmation state + `executeDelete()`.

### 2. `goto()` to NON-EXISTENT routes (dead navigation; should open modal)
- `newsletter/+page.svelte:267` → `goto(\`/newsletter/subscribers/${id}\`)` — no such route
- `newsletter/+page.svelte:274` → `goto(\`/newsletter/campaigns/${id}\`)` — no such route
- `broadcasts/+page.svelte:193` → `goto(\`/broadcasts/${id}\`)` — no such route

Fix: open a `<Modal>` preview/edit dialog instead of navigating.

### 3. Systemic `window.__*` global action handlers (SPA pattern violation)
Action buttons rendered as raw HTML strings (`render: (v,row) => \`<button onclick="window.__x(${row.id})">\``) wired to functions mounted on `window`. Bypasses Svelte reactivity/safety. Canonical pattern = `component:`/`{#snippet}` cell (see `tags/TagActionsCell`, `users/UserActionsCell`, `moderation/ModActionsCell`).

Affected (all need migration to snippet/component action cells):
`spam:72`, `learning:117/154/190`, `newsletter:110/111/168/169`, `broadcasts:107/108`, `email-templates:54-56`, `redirects:54/55`, `menus:71-90`, `jsonld-templates:80/83`, `page-builder/themes:47-49`, `content:109-119`, `mentorship:103-108`, `bots:98-107`, `forms:97-103`, `tickets:97-100`, `affiliate:96-141`, `discount-codes:63-116`, `ads:117-122`, `media:116-137`.

Several of these `window.__*` handlers perform **destructive/state-changing ops with no confirmation modal**: `affiliate` `deleteProvider`/`updateMemberStatus`, `discount-codes` `__deleteCode`, `ads` `__endCampaign`/`__endBoost`, `media` delete, `custom-fields` `deleteGroup`, `widgets` `deleteWidget`, `notification-types` (uses `window.confirm`).

### 4. Missing action columns (spec §1.5: View+Edit+Delete on EVERY row)
| Page | Missing |
|---|---|
| `professional-types` | Entire actions column |
| `tickets` | Edit + Delete (View only) |
| `forms` | Edit (View/Delete present) |
| `content` | View + Edit (only Restore/Remove) |
| `mentorship` | View |
| `bots` | View |
| `space-types` | View |
| `comparisons` | View |
| `reactions` | View |
| `media` | View + Edit |
| `blog` | Delete for normal posts (only Reject when `status==='review'`) |
| `reviews` (readonly) | All actions (spec §1.9) |
| `profiles` (readonly) | All actions (spec §1.9) |
| `organisations` (readonly-ish) | All actions (spec §1.9) |
| `gdpr/consents` (readonly) | No Delete/Clear (spec §1.9) |

Backend CRUD exists for all (Gate 2 pass) → frontend additions only.

### 5. Missing Create button on non-readonly entities (spec §1.8)
- `profiles`, `organisations` — no Create and no Delete (both arguably provisioned, but spec requires at least Delete/Clear for readonly-style entities).

---

## HIGH

### Shared DataTable infra defects (`data-table.svelte`, affects every DataTable page)
1. **Sort not URL-param driven** (`:40-65`) — `sortColumn`/`sortDirection` are local `$state`; sort lost on navigation. Must read/write `?sort=`+`?dir=` via `goto`/`$page.url`.
2. **No `?q=` / `?filter=` URL persistence** — `DataTable` has no search/filter UI; pages hold search/filter in local state (e.g. `tags` passes to API but never to URL). Filter state lost on navigation.
3. **No bulk-action bar** — `DataTable` supports `selectable`/`selectedIds` but renders no contextual bulk bar; `tags` builds its own, but `blog`/`media`/`tickets`/`content` pass `selectable` with no bulk UI (dead selection).

### SPA auth: `window.location.*` instead of `goto()` (spec §2)
| File | Line | Issue |
|---|---|---|
| `users/+page.svelte` | 231, 268 | `window.location.reload()` |
| `users/[userId]/+page.svelte` | 257 | `window.location.href = "/"` |
| `users/groups/GroupActionsCell.svelte` | 49, 58 | `window.location.reload()` |
| `(auth)/login/+page.svelte` | 31 | `window.location.href = '/'` |
| `(auth)/logout/+page.svelte` | 24 | `window.location.href = '/login'` |
| `TopBar.svelte` | 42, 106 | `window.location.href` (incl. notification links) |
| `spaces/[spaceId]/forms/[formId]/+page.svelte` | 99 | `window.location.href` |

Never use `window.location.href='/login'` on 401 in the API client (corrections: `api_client_401_no_redirect`).

### `var(--color-*)` violations (spec §4, §design_system_approach)
Raw Tailwind palette / hex used instead of theme tokens in: `learning`, `newsletter`, `broadcasts`, `jsonld-templates`, `ai/+page.svelte`, `ai/tools`, `settings` (hand-rolled switch + `bg-primary-600`/`bg-gray-200`), `blog/review-queue`, `broadcast/+page.svelte` (`bg-danger-bg` undefined), `affiliate`/`referral`/`wallet`/`feature-flags`/`i18n`/`data-exports` (raw `text-gray-*`/`bg-gray-*`). Also `settings` + `ai/tools` use hand-rolled `<button role="switch">` instead of Bits UI `Switch`.

---

## MEDIUM

### RBAC UI: missing permission gating (spec §5)
Most mutation pages rely on backend 403 only and never call `hasPermission()` on destructive/state controls: `moderation/appeals`, `moderation/reports`, `moderation/alerts`, `users/roles`, `users/groups`, `page-templates`, `page-builder/*`, `jsonld-templates`, `menus`, `redirects`, `translations`, `join-requests`, `blog/review-queue`, `learning`, `newsletter`, `broadcasts`, `broadcast`, `email-templates`, `settings`, `gdpr/*`. State actions must be gated individually (ban ≠ suspend).

### Search / filter / pagination gaps (spec §1.2–1.4)
- **No search box** on most list pages (`moderation/reports|appeals|content`, `gdpr/*`, `users/roles`, `translations`, `join-requests`, `page-templates`, `page-builder/*`, `jsonld-templates`, `menus`, `redirects`, `ai/*`).
- **Filter decorative/no-op** on `newsletter` + `broadcasts` (`handleFilter` stores state, never re-queries).
- **`pageSize={data.length}`** disables pagination on `spam`, `gdpr/audit-logs`, `gdpr/consents`, `users/groups`, `redirects`, `menus`, `jsonld-templates`, `page-builder/themes`, `coupons`, `webhooks` — all rows load at once.

---

## PASS (clean areas)
- ✅ No `export let` / `$:` / `on:click` / `createEventDispatcher` (Svelte 5 runes correct everywhere).
- ✅ All pages fetch via `$effect` (no `onMount` data fetch misuse).
- ✅ No hardcoded mock data rows (Rule 16 OK) — only static label maps in `settings`/`ai` (acceptable fallbacks).
- ✅ RBAC uses permission-node checks (`permissions.includes(...)`), not role strings.
- ✅ LayerChart pages (`analytics`, `(app)` dashboard): fixed-height `h-64` wrappers, `var(--color-*)` fills, `Date()` conversion, skeleton loaders, API-sourced data, no LayerChart/Bits-UI `Tooltip` collision.
  - **Minor LayerChart:** `(app)/+page.svelte:217` LineChart series has no explicit `var(--color-*)` stroke (falls back to LayerChart default); `analytics` + dashboard lack a `?from=&to=` date-range picker (uses `period=` instead).

---

## Prioritized fix order
1. `window.confirm` → Modal (`broadcast`, `page-builder/pages`) — 2 files, quick.
2. `goto()` to missing routes (`newsletter`, `broadcasts`) → modal View/Edit.
3. `window.location.*` → `goto()` (8 spots across 7 files).
4. Migrate `window.__*` action cells → snippet/component cells with confirmation (systemic; use `tags`/`users`/`moderation` as template).
5. Add missing View/Edit/Delete + Create buttons (14 pages).
6. Fix shared `DataTable` (URL sort / `?q=` / `?filter=` / bulk bar) — unblocks all list pages.
7. `var(--color-*)` + Bits UI `Switch` sweep; add `hasPermission()` gating.

No backend endpoint work required — all CRUD/state routes already exist.
