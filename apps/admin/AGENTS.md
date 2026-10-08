# AGENTS.md — JapaConnect Admin Panel

This document captures the conventions for the `apps/admin` SvelteKit 2 + Svelte 5 Runes SPA.
It is the single source of truth for the shared UI components introduced by the
admin-panel shared-component refactor and the recipe for migrating the
remaining ~55 legacy entity pages to the new scaffold.

---

## Shared UI components

Every entity list page in the admin panel MUST use the shared components below.
Do not re-implement modals, filter bars, pagination, bulk bars, or row-action
buttons by hand on new pages. Legacy pages keep their hand-rolled UI until they
are migrated via the recipe in the next section.

### Component inventory

| Component | Path | Purpose |
|---|---|---|
| `EntityPage` | `src/lib/components/entity/entity-page.svelte` | Mandatory scaffold for every entity list page. Composes PageHeader + DataTable + SlideOver + confirm Modal into a declarative CRUD page driven by an `EntityPageConfig`. |
| `EntityForm` | `src/lib/components/entity/entity-form.svelte` | Declarative form renderer driven by a `FieldDef[]`. Used by `EntityPage` when no `formSnippet` is supplied. |
| `DataTable` | `src/lib/components/ui/data-table/data-table.svelte` | Table with server-side pagination, search, sort, filters, bulk selection, and row actions. Use `serverSide` for all server-paginated lists. Every table gets a built-in "Export Selected" bulk action (row checkboxes appear automatically) unless it uses the legacy `bulkActions` snippet path or sets `bulkExport={false}`; use `exportFilename` to name the CSV. |
| `ActionMenu` | `src/lib/components/ui/data-table/action-menu.svelte` | The ⋮ dropdown for row actions. Do not render inline action buttons — wire actions through the `actions` config. |
| `FilterPanel` | `src/lib/components/ui/data-table/filter-panel.svelte` | Accordion-based multi-filter UI. Do not use `<FilterBar>` for new pages (legacy pages keep it until migrated). |
| `BulkBar` | `src/lib/components/ui/data-table/bulk-bar.svelte` | Bulk-action bar shown when rows are selected. Driven by `bulkActionsList` / `BulkAction[]`. |
| `SlideOver` | `src/lib/components/ui/slide-over/slide-over.svelte` | Right-edge slide-in panel for create/edit/view-detail. Replaces `<Modal>` for those flows. `<Modal>` is only for short confirmations. |
| Sidebar store | `src/lib/stores/sidebar.svelte.ts` | `sidebarCollapsed()`, `setSidebarCollapsed()`, `toggleSidebar()`, `initSidebarFromStorage()`. State persists to `localStorage` under `jc.admin.sidebar.collapsed`. Toggle via `Ctrl/Cmd+B` or the TopBar PanelLeft button. |

### Sidebar collapse

- Toggle with `Ctrl/Cmd+B` or the TopBar PanelLeft button.
- State persists to `localStorage` (`jc.admin.sidebar.collapsed`).
- Call `initSidebarFromStorage()` once on app load (the root layout does this).

---

## Migration recipe — how to migrate a legacy page to `EntityPage`

A standard CRUD list page can be replaced wholesale with `<EntityPage {config} />`.
Pages with rich forms, sub-flows, or state actions keep `EntityPage` for the list
and use the `formSnippet` / `viewSnippet` escape hatches plus page-local state.

1. **Replace `<Modal>` create/edit with `<SlideOver>`.** Create/edit/view-detail
   open in a right-edge slide-over, not a centered modal. `<Modal>` is retained
   only for short confirmations (delete, ban, warn, etc.).
2. **Replace `<FilterBar>` + external `<Pagination>` with
   `<DataTable serverSide filters bulkActionsList actions>`.** DataTable owns
   pagination, search, sort, filter, and bulk selection state internally.
3. **Replace the hand-rolled bulk bar with `<BulkBar>` / `bulkActionsList`.**
   Pass a `BulkAction[]` to `config.bulkActions`; `EntityPage` wires
   `selectable` and `selectedIds` automatically. The `disabled` predicate
   receives `(ids: Set<string>, selectedRows: any[])` so it can filter by
   row data (e.g. disable "Hard Delete" unless a deleted row is selected).
   `EntityPage` auto-reloads and clears the selection after each bulk
   handler completes.
4. **Replace inline action buttons with `<ActionMenu>` `actions` config.** Row
   actions render in the ⋮ dropdown. Use `show()` predicates to filter actions
   per-row and `variant: 'destructive'` for dangerous actions.
5. **Where the page is a standard CRUD list, replace the whole page with
   `<EntityPage {config} />`.** Build an `EntityPageConfig` (shape below) and
   render `{#key reloadKey}<EntityPage {config} />{/key}` (the `{#key}` block is
   needed when the page defines bulk actions — see "Known gaps").

### Reference migrations

Three pages were migrated as reference templates — read them before migrating a
new page:

- **`/tags`** (`src/routes/tags/+page.svelte`) — rich page: `formSnippet` (media
  pickers + custom fields), `viewSnippet`, custom `componentSnippet` column,
  moderators sub-flow as a page-local `<SlideOver>`, bulk delete with remount.
- **`/share-platforms`** (`src/routes/share-platforms/+page.svelte`) — simple
  page: declarative `fields` (EntityForm), `viewSnippet`, bulk delete. The
  canonical "easy migration" template.
- **`/users`** (`src/routes/users/+page.svelte`) — complex page: state actions
  (ban/unban/suspend/warn/verify/restore/hard-delete/impersonate) via a custom
  `actions` array with `show()` predicates, page-local confirm `<Modal>`s,
  dynamic filter options loaded on mount, `config` built as `$derived` so filter
  updates propagate.

---

## `EntityPageConfig` shape

Defined in `src/lib/components/entity/entity-page.svelte`. Key fields:

```ts
interface EntityPageConfig {
  title: string;
  description?: string;
  columns: Column[];              // data-table/types.ts
  filters?: FilterDef[];          // data-table/types.ts
  actions?: ActionItem[];         // omit → default View/Edit/Delete/Restore/HardDelete (permission-gated)
  bulkActions?: BulkAction[];     // custom actions appended to the built-ins
  bulkDelete?: boolean;           // default true — set false to opt out of the
                                  // confirm-gated built-in "Delete Selected"
                                  // (added automatically when `remove` is set)
  hideCreate?: boolean;           // default false — set true to hide the built-in
                                  // Create button when the page provides its own
                                  // create flow (e.g. /spaces navigates to a
                                  // dedicated page). Unlike `readonly`, edit,
                                  // delete and bulk actions stay enabled.
  createLabel?: string;
  createPermission?: string;      // edit/delete fall back to this when omitted
  editPermission?: string;
  deletePermission?: string;      // also fallback for hardDeletePermission/restorePermission
  hardDeletePermission?: string;  // defaults to deletePermission → createPermission
  restorePermission?: string;     // defaults to deletePermission → createPermission
  fields: FieldDef[];             // drives EntityForm + edit-form value seeding
  readonly?: boolean;             // hides create/edit/delete
  pageSize?: number;
  pageSizeOptions?: number[];     // default [10, 25, 50, 100]
  load: (params: { page; pageSize; q; sort; filter }) =>
    Promise<{ data: any[]; total: number; totalPages: number } | any[]>;
  create?: (body) => Promise<{ ok: boolean; error?: string }>;
  update?: (id, body) => Promise<{ ok: boolean; error?: string }>;
  remove?: (id) => Promise<{ ok: boolean; error?: string }>;       // soft delete
  hardRemove?: (id) => Promise<{ ok: boolean; error?: string }>;   // permanent delete (opt-in)
  restore?: (id) => Promise<{ ok: boolean; error?: string }>;      // restore soft-deleted (opt-in)
  isDeleted?: (row: any) => boolean;  // default: row.status==='deleted' || row.deleted_at!=null
  exportable?: boolean;           // show Export CSV button (opt-in)
  exportFilename?: string;        // default: `${title}-export`
  viewSnippet?: Snippet<[{ row: any }]>;   // custom view slide-over body
  formSnippet?: Snippet<[{ value; editing }]>; // custom create/edit form body
}
```

### Soft delete, Hard delete, and Restore

When `hardRemove` and/or `restore` are provided in the config, the default row
actions (when `config.actions` is omitted) automatically include:

- **Delete** (soft) — shown when `isDeleted(row)` is false
- **Restore** — shown when `isDeleted(row)` is true and `config.restore` is set
- **Hard Delete** — shown when `isDeleted(row)` is true and `config.hardRemove`
  is set; opens a separate confirm modal with a stronger warning

The `isDeleted` predicate defaults to `row.status === 'deleted' || row.deleted_at != null`
to cover both backend patterns. Override it for entities that use a different
soft-delete signal.

Pages that supply custom `config.actions` must include these actions explicitly
(as `/users` does).

### Export (CSV)

Set `exportable: true` to show an "Export" button in the page header. On click,
`EntityPage` paginates through all filtered rows (using the current
search/sort/filter state) via `config.load`, builds a CSV from the configured
columns, and triggers a browser download. The filename defaults to
`${title}-export.csv` or can be customized via `exportFilename`.

### Resizable and hideable columns

The `Column` type (in `data-table/types.ts`) now supports:

- `resizable?: boolean` — default `true`; set `false` to disable the resize
  handle on a column. Drag the handle to resize; double-click to reset to
  auto-width.
- `hideable?: boolean` — default `true`; set `false` to exclude a column from
  the hide/unhide toggle (e.g. the actions column).
- `width?: number` — initial column width in pixels.
- `defaultHidden?: boolean` — hide the column by default; the user can toggle
  it on via the "Columns" dropdown in the table toolbar.

A "Columns" dropdown button appears in the DataTable toolbar whenever there are
hideable columns. It lists each toggleable column with a checkbox.

### When to use `fields` vs `formSnippet`

- **`fields` only (declarative `EntityForm`)** — use when the form is a flat set
  of standard inputs (text, textarea, number, email, password, date,
  datetime-local, url, checkbox, select). This is the common case
  (`/share-platforms` is the template). `EntityPage` seeds the edit form from
  the row automatically.
- **`formSnippet`** — use when the form needs media pickers, color pickers,
  autocomplete, dependent fields, custom validation UI, or any input
  `EntityForm` does not support. The snippet receives `{ value, editing }` and
  binds to `value[fieldKey]` directly. You MUST still pass a `fields` array
  (even if the inputs are in the snippet) because `EntityPage` uses it to seed
  the edit-form value. `/tags` is the template.

### `FieldDef` shape

Defined in `src/lib/components/entity/index.ts`. Supported `type` values:
`text`, `textarea`, `number`, `email`, `password`, `date`, `datetime-local`,
`url`, `checkbox`, `color` (native color picker), `select` (requires
`options: Array<{ value; label }>`).

Every variant also accepts the per-row lock props `disabledWhen` and
`lockedHelp`. `disabledWhen(editing)` is evaluated against the row being
edited (never called on create, where `editing` is null); when it returns
`true` the control renders disabled and `lockedHelp` replaces `help`, so a
field whose value is derived elsewhere (e.g. a roadmap item's status while
a `feature_flag_key` is linked) can be locked with an explanatory hint.

---

## `ActionItem` pattern for state actions

Defined in `src/lib/components/ui/data-table/types.ts`:

```ts
interface ActionContext {
  openView: (row: any) => void;
  openEdit: (row: any) => void;
  openDelete: (row: any) => void;
  reload: () => void;
}

interface ActionItem {
  label: string;
  icon?: Component;
  onclick: (row: any, ctx?: ActionContext) => void;
  variant?: 'default' | 'destructive';
  show?: (row: any) => boolean;   // omit → always shown
}
```

- **Omit `config.actions`** to get the default View/Edit/Delete set
  (permission-gated via `createPermission`/`editPermission`/`deletePermission`).
- **Provide `config.actions`** to customize the menu. When you do, the defaults
  are **replaced entirely** — include View/Edit/Delete explicitly if you want
  them alongside custom actions (see `/users`).
- Use `show(row)` to filter actions per-row (e.g. show "Unban" only when
  `row.status === 'banned'`).
- Use `variant: 'destructive'` for dangerous actions so they render in the
  danger color.
- The second argument `ctx` gives custom action handlers access to the
  scaffold's built-in `openView`/`openEdit`/`openDelete`/`reload` functions,
  so a custom action can trigger a slide-over or refetch without a page-local
  workaround.
- State actions that need a confirmation dialog keep a page-local `<Modal>`
  (EntityPage's built-in confirm only covers the default delete). After the
  API call succeeds, call `entityController?.reload()` to refetch (see
  "EntityPage controller" below).

## EntityPage controller

`<EntityPage>` exposes a `bind:controller` prop of type `EntityPageController`:

```ts
interface EntityPageController {
  reload: () => void;
  openView: (row: any) => void;
  openEdit: (row: any) => void;
  openDelete: (row: any) => void;
}
```

Page-local code (modals, sub-flows, external buttons) can call
`controller.reload()` to trigger a refetch without remounting the component:

```svelte
let entityController = $state<EntityPageController | undefined>(undefined);
// ... in a modal confirm handler:
entityController?.reload();

<EntityPage {config} bind:controller={entityController} />
```

`EntityPage` also auto-reloads and clears the row selection after every
`bulkActions` handler completes — no `{#key reloadKey}` remount is needed.

---

## Known `EntityPage` gaps + workarounds

These are acknowledged limitations of the initial shared-component release.

1. **Generic confirm modal (no entity name).** The built-in delete confirm
     shows a generic "Confirm Delete. This cannot be undone." message without
     the entity name.
   - *Status:* acceptable. Pages needing richer copy keep a page-local
     `<Modal>` (as `/users` does for its state actions).

## URL-sync (sort/filter/page)

`EntityPage` reflects its full state in the URL so list views are
shareable/bookmarkable and a reload restores the exact view:

- **`sort` / `dir`** and **`q`** (search) are written by the `DataTable`
  (`urlSync` is enabled on the scaffold). The sort icon and search box are
  restored from the URL on load.
- **`page`** (1-based), **`pageSize`** (only when != the configured default),
  and **filters** are written by `EntityPage` via SvelteKit `replaceState`
  (matching `Pagination.svelte`). Each filter key becomes its own query param
  (e.g. `?status=active&tier=partner`); the reserved names `page`, `pageSize`,
  `q`, `sort`, `dir` are never treated as filter keys.
- All state is seeded from the URL once at init (via `$state` initializers),
  so the first `load` call uses the restored params — no double-fetch.

No per-page work is needed; the scaffold handles this for every `<EntityPage>`.

---

## Bulk-delete in-flight guard pattern

> **Superseded for the standard case.** `EntityPage` now ships a built-in,
> confirm-gated "Delete Selected" action that runs `config.remove` per selected
> row, collects per-row failures, clears the selection, and reloads. A
> page-declared `bulkActions` entry labeled "Delete Selected" with
> `variant: 'destructive'` is replaced by the built-in automatically, so most
> pages can simply delete their custom bulk-delete handler. Opt out of the
> built-in with `bulkDelete: false`. When `config.forceRemove` is set, the
> built-in bulk delete is two-phase like the single-row flow: rows whose
> `remove` returns `{ ok: false, inUse: true }` are gathered into a second
> confirm modal that runs `config.forceRemove` per row (e.g. `?force=true`).
> The guard pattern below still applies to *custom* bulk actions that call
> APIs directly.

Learned during the `/tags` migration: without a guard, a user can trigger bulk delete
twice (e.g. double-click) and fire duplicate API calls. Every bulk-action
handler in an `EntityPage` page MUST include an in-flight guard:

```ts
let pageError = $state('');
let bulkDeleting = $state(false);

async function bulkDelete(ids: Set<string>) {
  const idArr = Array.from(ids);
  if (idArr.length === 0) return;
  if (bulkDeleting) return;        // ← in-flight guard
  bulkDeleting = true;
  pageError = '';
  try {
    await Promise.all(idArr.map((id) => someApi.delete(id)));
    // EntityPage auto-reloads + clears selection after this returns.
  } catch (e) {
    pageError = e instanceof Error ? e.message : t('common.error', 'Failed to delete');
  } finally {
    bulkDeleting = false;
  }
}
```

Then in the template:

```svelte
{#if pageError}
  <div class="mb-4 rounded-lg p-3 text-sm" style="...">
    {pageError}
  </div>
{/if}

<EntityPage {config} bind:controller={entityController} />
```

All three reference migrations (`/tags`, `/share-platforms`, `/users`) use this
pattern.

---

## Testing conventions

Tests use **vitest** + **@testing-library/svelte**. Co-located as
`*.test.ts` next to the component/page.

### Page tests

- Mock `$lib/api` with `vi.mock('$lib/api', ...)` — return `{ ok: true, data: [] }`
  shapes so `EntityPage`'s `load` resolves.
- Mock `$app/state`, `$app/navigation`, and `$lib/stores/permissions.svelte`
  (see `src/routes/tags/page.test.ts` for the canonical mock block).
- Assert the page heading renders and the empty state appears via `waitFor`
  (data loads asynchronously).

### Component tests

- **bits-ui portals:** `SlideOver` (and any bits-ui Dialog/Popover) renders
  content into a portal on `document.body`. Use `waitFor` before asserting on
  portaled content, and query `document.body.querySelector(...)` for
  portal-rendered nodes. Do not assume content is in the component's
  `container`.
- **Snippet props:** Svelte 5 snippets cannot be passed as plain props in
  tests. Use a **test wrapper component** that renders the component under test
  and declares the snippets inline. See
  `src/lib/components/ui/slide-over/slide-over-test-wrapper.svelte` — it wraps
  `<SlideOver>` and forwards `body`/`footerText` as snippet content. The test
  renders the wrapper, not `SlideOver` directly, when snippets are needed.
- **ActionMenu:** click the `[data-action-menu-trigger]` element, then
  `waitFor` the menu item text, then click the item. Assert `onclick` was
  called with the row object (see `action-menu.test.ts`).

### Running tests

```bash
cd apps/admin && bun run vitest run
```
