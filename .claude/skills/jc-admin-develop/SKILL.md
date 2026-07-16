---
name: jc-admin-develop
description: Use when building new pages or components for the JapaConnect Admin Panel (apps/admin, port 3001, SPA). Covers the mandatory DataTable spec (sort/search/filter/paginate/actions/bulk/create), the RBAC CRUD Gate, LayerChart dashboards, and SPA auth patterns.
---

# JapaConnect Admin Panel — Development Skill

Internal SPA (adapter-static) for moderation, user management, analytics, configuration. Design: dev.to admin — dense, action-first.

## ⛔ GATE 1 — API-READINESS (Run Before Any Implementation)
List every endpoint the feature consumes → verify each exists (JEOS `api-routes-catalog.md` or live probe) → if ANY missing: STOP, output:
```
⚠️ MISSING BACKEND API ROUTES — implementation paused
Feature: [name]
Missing routes: [list]
Action needed: implement these routes in the Go backend before frontend work continues.
```
Never mock, never hardcode around a gap.

## ⛔ GATE 2 — RBAC CRUD (Non-Negotiable, Run Per Entity)
**Super-admin must be able to perform full CRUD on every entity managed in this panel.** Before implementing ANY entity management page, verify the backend exposes ALL of:
- `GET /api/v1/admin/[entity]` (list, with `?page=&limit=&sort=&q=&filter=`)
- `GET /api/v1/admin/[entity]/:id` (detail)
- `POST /api/v1/admin/[entity]` (create)
- `PATCH /api/v1/admin/[entity]/:id` (update)
- `DELETE /api/v1/admin/[entity]/:id` (delete)
- State-transition routes where the entity has states: `POST .../suspend`, `POST .../unsuspend`, `POST .../ban`, `POST .../unban`, `POST .../approve`, `POST .../reject`, etc.
- All of the above RBAC-gated by permission nodes

If ANY CRUD or state route is missing: STOP, output:
```
⛔ MISSING RBAC CRUD ENDPOINTS — implementation paused
Entity: [entity]
Missing: [e.g. PATCH /api/v1/admin/tags/:id, POST /api/v1/admin/users/:id/unban]
Super-admin CRUD is non-negotiable. Backend must expose these before this page is built.
```

## 📋 DATATABLE SPEC — MANDATORY FOR EVERY ENTITY LIST PAGE
Every entity list uses the shared `<DataTable>` with ALL of the following. No exceptions, no "simple list" shortcuts:

1. **Sortable** — every sortable column header toggles asc/desc; state in URL params
2. **Searchable** — search input wired to `?q=` (debounced 300ms)
3. **Filterable** — collapsible filter panel (`<Accordion.Root>`) with entity-specific filters wired to `?filter=`
4. **Paginated** — page size selector (25/50/100) + navigation; from API `meta`
5. **Action column** — EVERY row, via `<DropdownMenu.Root>` (⋮):
   - **View** (opens `<Dialog.Root>` preview modal — never navigates away)
   - **Edit** (opens slide-over/dialog form)
   - **Delete** (opens `<Dialog.Root>` confirmation)
   - **State actions** where the entity supports them: Suspend/Unsuspend, Ban/Unban, Approve/Reject, Activate/Deactivate, Verify/Unverify — shown conditionally on current state (e.g. suspended row shows Unsuspend)
6. **Bulk actions** — per-row checkboxes (`Set<string>`), select-all, bulk bar visible when `selectedIds.size > 0` with contextual actions (Delete, Suspend, Approve, Export)
7. **Create button** — top-right of the table, for ALL non-readonly entities; opens the create form in `<Dialog.Root>`
8. **Readonly entities** (audit logs, impersonation logs, webhook deliveries, sessions): no Create/Edit, but the action column MUST still include **Delete/Remove/Clear** (single-row delete + a "Clear all" or "Clear older than…" bulk option), plus View

```svelte
<script lang="ts">
  let { columns, rows, totalRows, entity,
    page = $bindable(1), pageSize = $bindable(25),
    q = $bindable(''), filters = $bindable({}),
    sort = $bindable({ key: 'created_at', dir: 'desc' }),
    selectedIds = $bindable(new Set<string>()),
    readonly = false, stateActions = [],
    onCreate, onView, onEdit, onDelete, onStateAction, onBulkAction,
  }: DataTableProps = $props();
</script>
```

## LayerChart — Dashboard & Analytics (Mandatory)
Admin Dashboard and every Analytics page (Traffic, Content, Search, Community, Revenue) use **LayerChart** exclusively:
- Line/Area (`<Spline>`, `<Area>`): signups over time, DAU/MAU, content volume, revenue
- Bars (`<Bars>`): top spaces by activity, content by type, reports by category
- Arc/Pie (`<Arc>`): geo distribution, user group split
- Stat cards: value + delta + sparkline (`<Chart>` + `<Spline>`, no axes)
- Colors ONLY `var(--color-*)` · fixed-height containers · data from `/api/v1/admin/analytics/*` · skeleton per chart · date-range picker (`<Popover.Root>` + range) driving `?from=&to=` on the analytics endpoints

## SPA Architecture
`ssr = false`, `prerender = false` root layout · no `+page.server.ts` anywhere · auth guard on mount (`$effect` → `goto('/login')`) · session from `$page.data.session` · silent token refresh · `goto()` for navigation · fetch pattern: `$effect` with loading/error/`finally`, reactive to page/sort/filter params.

## RBAC-Gated UI
`let canX = $derived(session?.permissions.includes('entity.action') ?? false)` — hide/disable unauthorized controls proactively; never rely only on backend 403.

## Stack & Design
Same base stack as all JC apps (Svelte 5 Runes, Bits UI dot-notation, Tailwind v4, TypeScript, Lucide, `$t()` i18n, no AI attribution). Sidebar `#1C1B22`, content `#F7F7F7`/dark, accent `#3B49DF`, success/danger/warning tokens, 14px compact density, `[data-theme="dark"]`.

## Deliverables Per Task
1. Gate 1 + Gate 2 output (routes verified) · 2. `+page.svelte` with full-spec DataTable or LayerChart dashboard · 3. CRUD form dialogs · 4. types · 5. confirmation of every `/api/v1/admin/*` route used
