# Admin Panel Refactor — Foundation Layer Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the shared "single source of truth" component layer for the JapaConnect Admin Panel (`apps/admin`) — an upgraded `DataTable` with server pagination / action column / bulk bar / filter panel, a right-side `SlideOver` for create/edit/view-detail, an `EntityPage` scaffold that collapses 500-line pages to ~100 lines, and a wired-up collapsible left nav — then migrate 3 reference pages to validate the layer before the incremental sweep.

**Architecture:** All shared UI lives under `apps/admin/src/lib/components/ui/`. The upgraded `DataTable` becomes the only table primitive: it gains a `serverSide` mode (consumes API `meta` for pagination instead of client-slicing), a built-in `<DropdownMenu>` action column (View/Edit/Delete + state actions), a built-in bulk bar, and a collapsible `<Accordion>` filter panel. A new `SlideOver` component wraps bits-ui `Dialog` primitives positioned at the right edge with slide-in animation — it replaces `<Modal>` for create/edit/view-detail forms; `<Modal>` stays only for short delete confirmations. A new `EntityPage` scaffold composes `PageHeader` + `Card` + `DataTable` + `SlideOver` + confirm `Modal` + loading/error/success states, exposing a typed config object so pages declare columns/filters/actions/form-fields instead of reimplementing plumbing. The root `+layout.svelte` owns the sidebar `collapsed` state (persisted to `localStorage`, toggled from a new TopBar button and `Ctrl/Cmd+B`).

**Tech Stack:** SvelteKit 2.63 + Svelte 5.56 Runes, bits-ui 2.18.1 (Dialog, DropdownMenu, Accordion, Popover), Tailwind CSS 4.3, lucide-svelte 1.0.1, clsx, vitest 4.1 + @testing-library/svelte 5.4.

## Global Constraints

- **Behavior-preserving:** No entity loses functionality during migration. Every migrated page must retain its current CRUD + state actions + filters.
- **Stack:** Svelte 5 Runes only (`$state`, `$derived`, `$effect`, `$props`, `$bindable`). No Svelte 4 `on:` / stores-as-values patterns.
- **Styling:** Tailwind v4 + CSS custom properties (`var(--color-*)`). No hardcoded colors. Dark mode via `[data-theme="dark"]`.
- **i18n:** All user-facing strings via `$t()` from `$lib/stores/i18n.svelte` with fallbacks.
- **No AI attribution** in code or comments.
- **One-line comment per change** max (per jc-admin-refactor skill).
- **API-Readiness Gate:** This plan does NOT add new backend endpoints. It only reorganizes frontend consumption of existing `/api/v1/admin/*` routes. No mocking, no hardcoded data (Rule 16).
- **RBAC:** Permission checks stay via `hasPermission('node.name')`; the scaffold derives `canCreate`/`canEdit`/`canDelete` from permission nodes passed in config.
- **SPA:** `ssr=false`, no `+page.server.ts`. All fetch in `$effect` with loading/error/`finally`.
- **Tests:** vitest + @testing-library/svelte. Component tests use the existing `vitest.setup.ts` mock for `$lib/stores/i18n.svelte`. Run with `pnpm vitest run` (or `bun run vitest run`) from `apps/admin`.

---

## File Structure

**New files (created):**
- `apps/admin/src/lib/components/ui/slide-over/index.ts` — barrel export
- `apps/admin/src/lib/components/ui/slide-over/slide-over.svelte` — right-edge Dialog-based slide-over (header/body/footer, sizes)
- `apps/admin/src/lib/components/ui/data-table/types.ts` — shared `DataTableProps`, `Column`, `ActionItem`, `FilterDef`, `BulkAction` types (single source of truth for table types)
- `apps/admin/src/lib/components/ui/data-table/action-menu.svelte` — `<DropdownMenu>` ⋮ action cell (replaces inline-button `action-cell.svelte` as the default; `action-cell.svelte` stays for legacy inline use)
- `apps/admin/src/lib/components/ui/data-table/filter-panel.svelte` — `<Accordion>` collapsible multi-filter panel
- `apps/admin/src/lib/components/ui/data-table/bulk-bar.svelte` — extracted bulk-action bar (used internally by DataTable)
- `apps/admin/src/lib/components/entity/index.ts` — barrel
- `apps/admin/src/lib/components/entity/entity-page.svelte` — the scaffold composing PageHeader + Card + DataTable + SlideOver + confirm Modal
- `apps/admin/src/lib/components/entity/entity-form.svelte` — declarative form renderer driven by a field config (text/textarea/select/checkbox/checkbox-group/media-picker/user-autocomplete)
- `apps/admin/src/lib/stores/sidebar.svelte.ts` — `collapsed` rune state + localStorage persistence + toggle
- `apps/admin/src/lib/components/ui/slide-over/slide-over.test.ts` — render/animation/a11y tests
- `apps/admin/src/lib/components/ui/data-table/data-table.test.ts` — server-paginated mode, action menu, bulk bar, filter panel tests
- `apps/admin/src/lib/components/entity/entity-page.test.ts` — scaffold composition test
- `apps/admin/src/lib/stores/sidebar.test.ts` — persistence + toggle tests

**Modified files:**
- `apps/admin/src/lib/components/ui/data-table/data-table.svelte` — add `serverSide` mode, built-in action column, built-in filter panel, built-in bulk bar, Create button slot; import new sub-components
- `apps/admin/src/lib/components/ui/data-table/index.ts` — export new sub-components + types
- `apps/admin/src/lib/components/layout/TopBar.svelte` — add desktop `PanelLeft` collapse toggle button + accept `onCollapseToggle` prop
- `apps/admin/src/routes/+layout.svelte` — own `collapsed` state via new sidebar store, wire TopBar `onCollapseToggle`, add `Ctrl/Cmd+B` shortcut
- `apps/admin/src/lib/components/layout/sidebar.svelte` — consume `collapsed` from store (keep `$bindable` for backward compat), persist on toggle
- `apps/admin/src/routes/tags/+page.svelte` — migrate to `EntityPage` scaffold (reference page 1)
- `apps/admin/src/routes/share-platforms/+page.svelte` — migrate to `EntityPage` scaffold (reference page 2, simple)
- `apps/admin/src/routes/users/+page.svelte` — migrate to `EntityPage` scaffold (reference page 3, has state actions + separate UserActionsCell)
- `apps/admin/src/routes/users/UserActionsCell.svelte` — adapt to action-menu config (or delete if fully absorbed)
- `apps/admin/AGENTS.md` — append the new shared-component conventions + migration recipe (if file exists; else create)

---

### Task 1: Sidebar collapse store + persistence

**Files:**
- Create: `apps/admin/src/lib/stores/sidebar.svelte.ts`
- Test: `apps/admin/src/lib/stores/sidebar.test.ts`

**Interfaces:**
- Produces: `sidebarCollapsed` (a `$state` boolean exported via a getter), `toggleSidebar()`, `setSidebarCollapsed(v: boolean)`, `initSidebarFromStorage()`. The store reads/writes `localStorage` key `jc.admin.sidebar.collapsed`.

- [ ] **Step 1: Write the failing test**

```ts
// apps/admin/src/lib/stores/sidebar.test.ts
import { describe, it, expect, beforeEach } from 'vitest';
import { toggleSidebar, setSidebarCollapsed, initSidebarFromStorage, sidebarCollapsed } from './sidebar.svelte';

describe('sidebar store', () => {
  beforeEach(() => {
    localStorage.clear();
    initSidebarFromStorage();
  });

  it('defaults to expanded when no stored value', () => {
    initSidebarFromStorage();
    expect(sidebarCollapsed()).toBe(false);
  });

  it('toggle flips the state and persists', () => {
    expect(sidebarCollapsed()).toBe(false);
    toggleSidebar();
    expect(sidebarCollapsed()).toBe(true);
    expect(localStorage.getItem('jc.admin.sidebar.collapsed')).toBe('1');
  });

  it('reads persisted value on init', () => {
    localStorage.setItem('jc.admin.sidebar.collapsed', '1');
    initSidebarFromStorage();
    expect(sidebarCollapsed()).toBe(true);
  });

  it('setSidebarCollapsed writes through', () => {
    setSidebarCollapsed(true);
    expect(sidebarCollapsed()).toBe(true);
    expect(localStorage.getItem('jc.admin.sidebar.collapsed')).toBe('1');
    setSidebarCollapsed(false);
    expect(localStorage.getItem('jc.admin.sidebar.collapsed')).toBe('0');
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd apps/admin && pnpm vitest run src/lib/stores/sidebar.test.ts`
Expected: FAIL — module not found / `sidebarCollapsed` is not a function.

- [ ] **Step 3: Write minimal implementation**

```ts
// apps/admin/src/lib/stores/sidebar.svelte.ts
const STORAGE_KEY = 'jc.admin.sidebar.collapsed';

let collapsed = $state(false);

export function sidebarCollapsed(): boolean {
  return collapsed;
}

export function setSidebarCollapsed(v: boolean): void {
  collapsed = v;
  if (typeof localStorage !== 'undefined') {
    localStorage.setItem(STORAGE_KEY, v ? '1' : '0');
  }
}

export function toggleSidebar(): void {
  setSidebarCollapsed(!collapsed);
}

export function initSidebarFromStorage(): void {
  if (typeof localStorage === 'undefined') return;
  collapsed = localStorage.getItem(STORAGE_KEY) === '1';
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd apps/admin && pnpm vitest run src/lib/stores/sidebar.test.ts`
Expected: PASS (4 tests).

- [ ] **Step 5: Commit**

```bash
git add apps/admin/src/lib/stores/sidebar.svelte.ts apps/admin/src/lib/stores/sidebar.test.ts
git commit -m "feat(admin): add sidebar collapse store with localStorage persistence"
```

---

### Task 2: Wire sidebar collapse into TopBar + root layout + keyboard shortcut

**Files:**
- Modify: `apps/admin/src/lib/components/layout/TopBar.svelte` (add `onCollapseToggle` prop + desktop `PanelLeft` button)
- Modify: `apps/admin/src/routes/+layout.svelte` (own collapsed state via store, wire TopBar, add `Ctrl/Cmd+B`)
- Modify: `apps/admin/src/lib/components/layout/sidebar.svelte` (consume store, keep `$bindable` fallback)
- Test: `apps/admin/src/routes/layout.test.ts` (extend existing)

**Interfaces:**
- Consumes: `initSidebarFromStorage`, `toggleSidebar`, `sidebarCollapsed`, `setSidebarCollapsed` from Task 1.
- Produces: TopBar accepts `onCollapseToggle?: () => void`; root layout calls `initSidebarFromStorage()` on mount and binds a `svelte:window` keydown handler for `Ctrl/Cmd+B`.

- [ ] **Step 1: Write the failing test (extend existing layout test)**

Append to `apps/admin/src/routes/layout.test.ts`:

```ts
import { fireEvent } from '@testing-library/svelte';
// ... after existing imports ...
import TopBar from '$lib/components/layout/TopBar.svelte';

describe('TopBar collapse toggle', () => {
  it('renders a desktop collapse button that fires onCollapseToggle', async () => {
    let toggled = false;
    const { container } = render(TopBar, { onCollapseToggle: () => (toggled = true) });
    const btn = container.querySelector('[data-sidebar-collapse]');
    expect(btn).toBeTruthy();
    await fireEvent.click(btn!);
    expect(toggled).toBe(true);
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd apps/admin && pnpm vitest run src/routes/layout.test.ts`
Expected: FAIL — `[data-sidebar-collapse]` not found.

- [ ] **Step 3: Add the collapse button to TopBar**

In `apps/admin/src/lib/components/layout/TopBar.svelte`:
- Add `PanelLeft` to the lucide import.
- Add `onCollapseToggle?: () => void` to props.
- Insert a desktop-only button (next to the existing mobile `<Menu>` button) with `data-sidebar-collapse` attribute:

```svelte
<button
  type="button"
  data-sidebar-collapse
  class="hidden rounded-md p-1.5 transition-colors hover:bg-black/5 dark:hover:bg-white/10 md:inline-flex"
  style="color: var(--color-foreground)"
  onclick={onCollapseToggle}
  aria-label={t('lib.components.layout.TopBar.svelte.toggle_sidebar', 'Toggle sidebar')}
>
  <PanelLeft class="h-5 w-5" />
</button>
```

- [ ] **Step 4: Wire the root layout**

In `apps/admin/src/routes/+layout.svelte`:
- Import `initSidebarFromStorage`, `toggleSidebar`, `sidebarCollapsed`, `setSidebarCollapsed` from `$lib/stores/sidebar.svelte`.
- In `onMount`, call `initSidebarFromStorage()`.
- Pass `onCollapseToggle={toggleSidebar}` to `<TopBar>`.
- Pass `collapsed={sidebarCollapsed()}` to `<Sidebar>` (replace the existing `bind:mobileOpen` only — do not remove).
- Add a `svelte:window` keydown handler:

```svelte
<svelte:window
  onkeydown={(e) => {
    if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 'b') {
      e.preventDefault();
      toggleSidebar();
    }
  }}
/>
```

- [ ] **Step 5: Make Sidebar consume the store**

In `apps/admin/src/lib/components/layout/sidebar.svelte`:
- Import `sidebarCollapsed` from `$lib/stores/sidebar.svelte`.
- Change the `collapsed` usage so that when no `collapsed` prop is bound, it reads `sidebarCollapsed()`. Keep the `$bindable` prop for backward compat. Simplest: derive `const effectiveCollapsed = $derived(collapsed ?? sidebarCollapsed())` and use `effectiveCollapsed` in the class logic. (The `collapsed` prop default stays `false`; the root layout will pass `collapsed={sidebarCollapsed()}`.)

- [ ] **Step 6: Run tests to verify they pass**

Run: `cd apps/admin && pnpm vitest run src/routes/layout.test.ts`
Expected: PASS.

- [ ] **Step 7: Manual smoke check**

Run: `cd apps/admin && pnpm dev` → open `http://localhost:3001`, log in, click the new PanelLeft button in the TopBar, confirm the sidebar collapses to the icon rail and reloads persist; press `Ctrl+B` to toggle.

- [ ] **Step 8: Commit**

```bash
git add apps/admin/src/lib/components/layout/TopBar.svelte apps/admin/src/routes/+layout.svelte apps/admin/src/lib/components/layout/sidebar.svelte apps/admin/src/routes/layout.test.ts
git commit -m "feat(admin): wire desktop sidebar collapse toggle + Ctrl/Cmd+B shortcut"
```

---

### Task 3: SlideOver component (right-edge slide-in panel)

**Files:**
- Create: `apps/admin/src/lib/components/ui/slide-over/slide-over.svelte`
- Create: `apps/admin/src/lib/components/ui/slide-over/index.ts`
- Test: `apps/admin/src/lib/components/ui/slide-over/slide-over.test.ts`

**Interfaces:**
- Produces: `<SlideOver>` component with props:
  ```ts
  {
    open: boolean (bindable),
    title?: string,
    description?: string,
    side?: 'right' | 'left' (default 'right'),
    size?: 'sm' | 'default' | 'lg' | 'xl' (widths 384 / 480 / 640 / 832 px),
    onclose?: () => void,
    children: Snippet,          // body
    footer?: Snippet,           // optional footer (action buttons)
  }
  ```
  Built on bits-ui `Dialog.Root`/`Dialog.Portal`/`Dialog.Overlay`/`Dialog.Content` with the Content pinned to the right edge (`fixed inset-y-0 right-0 h-full w-[...]`) and slide-in-from-right animation. Includes a header (title + description + close `X`) and a scrollable body (`flex-1 overflow-y-auto`) and an optional sticky footer.

- [ ] **Step 1: Write the failing test**

```ts
// apps/admin/src/lib/components/ui/slide-over/slide-over.test.ts
import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import SlideOver from './slide-over.svelte';

describe('SlideOver', () => {
  it('does not render content when closed', () => {
    const { container } = render(SlideOver, { open: false, title: 'Edit Tag' });
    expect(container.querySelector('[data-slideover-content]')).toBeNull();
  });

  it('renders title and body when open', () => {
    render(SlideOver, { open: true, title: 'Edit Tag', children: createSnippet('<p>form fields</p>') });
    expect(screen.getByText('Edit Tag')).toBeTruthy();
    expect(screen.getByText('form fields')).toBeTruthy();
  });

  it('renders footer snippet when provided', () => {
    render(SlideOver, { open: true, title: 'Edit', children: createSnippet(''), footer: createSnippet('<button>Save</button>') });
    expect(screen.getByRole('button', { name: 'Save' })).toBeTruthy();
  });
});

// helper: turn an HTML string into a Snippet for tests
function createSnippet(html: string) {
  return { render: () => ({ html }) } as any;
}
```

Note: bits-ui Dialog uses portals; if `jsdom` does not render the portal content, fall back to asserting on `data-slideover-content` presence via `container.querySelector` after `waitFor`. Adjust the test to use `waitFor` if the first render is async.

- [ ] **Step 2: Run test to verify it fails**

Run: `cd apps/admin && pnpm vitest run src/lib/components/ui/slide-over/slide-over.test.ts`
Expected: FAIL — module not found.

- [ ] **Step 3: Write the SlideOver component**

```svelte
<!-- apps/admin/src/lib/components/ui/slide-over/slide-over.svelte -->
<script lang="ts">
  import { t } from '$lib/stores/i18n.svelte';
  import { Dialog } from 'bits-ui';
  import { X } from 'lucide-svelte';
  import { clsx, type ClassValue } from 'clsx';
  import type { Snippet } from 'svelte';

  let {
    open = $bindable(false),
    title,
    description,
    side = 'right',
    size = 'default',
    class: className,
    children,
    footer,
    onclose
  }: {
    open?: boolean;
    title?: string;
    description?: string;
    side?: 'right' | 'left';
    size?: 'sm' | 'default' | 'lg' | 'xl';
    class?: ClassValue;
    children?: Snippet;
    footer?: Snippet;
    onclose?: () => void;
  } = $props();

  const widths = { sm: 'w-[384px]', default: 'w-[480px]', lg: 'w-[640px]', xl: 'w-[832px]' };

  function handleClose() {
    open = false;
    onclose?.();
  }
</script>

<Dialog.Root bind:open>
  <Dialog.Portal>
    <Dialog.Overlay class="fixed inset-0 z-50 bg-black/60 data-[state=open]:animate-in data-[state=open]:fade-in-0 data-[state=closed]:animate-out data-[state=closed]:fade-out-0" />
    <Dialog.Content
      data-slideover-content
      class={clsx(
        'fixed inset-y-0 z-50 flex h-full flex-col border shadow-xl duration-300 data-[state=open]:animate-in data-[state=closed]:animate-out',
        side === 'right'
          ? clsx('right-0 data-[state=open]:slide-in-from-right data-[state=closed]:slide-out-to-right')
          : clsx('left-0 data-[state=open]:slide-in-from-left data-[state=closed]:slide-out-to-left'),
        widths[size],
        className
      )}
      style="border-color: var(--color-border); background-color: var(--color-surface)"
    >
      <div class="flex items-start justify-between border-b px-5 py-4" style="border-color: var(--color-border)">
        <div class="flex flex-col gap-1 pr-6">
          {#if title}
            <Dialog.Title class="text-base font-semibold leading-none" style="color: var(--color-foreground)">{title}</Dialog.Title>
          {/if}
          {#if description}
            <Dialog.Description class="text-sm" style="color: var(--color-muted)">{description}</Dialog.Description>
          {/if}
        </div>
        <Dialog.Close
          class="absolute right-4 top-4 rounded-lg p-1 opacity-70 transition-opacity hover:opacity-100"
          style="color: var(--color-muted)"
          onclick={handleClose}
          aria-label={t('lib.components.ui.modal.modal.svelte.close', 'Close')}
        >
          <X class="h-4 w-4" />
        </Dialog.Close>
      </div>
      <div class="flex-1 overflow-y-auto px-5 py-4">
        {@render children?.()}
      </div>
      {#if footer}
        <div class="flex items-center justify-end gap-2 border-t px-5 py-3" style="border-color: var(--color-border)">
          {@render footer()}
        </div>
      {/if}
    </Dialog.Content>
  </Dialog.Portal>
</Dialog.Root>
```

Create `index.ts`:
```ts
export { default as SlideOver } from './slide-over.svelte';
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd apps/admin && pnpm vitest run src/lib/components/ui/slide-over/slide-over.test.ts`
Expected: PASS. If bits-ui portal does not mount in jsdom, switch assertions to `waitFor` and query `document.body` — update the test, do not weaken the assertion.

- [ ] **Step 5: Commit**

```bash
git add apps/admin/src/lib/components/ui/slide-over/
git commit -m "feat(admin): add SlideOver right-edge panel component"
```

---

### Task 4: DataTable shared types + action-menu sub-component

**Files:**
- Create: `apps/admin/src/lib/components/ui/data-table/types.ts`
- Create: `apps/admin/src/lib/components/ui/data-table/action-menu.svelte`
- Modify: `apps/admin/src/lib/components/ui/data-table/index.ts` (export new)
- Test: `apps/admin/src/lib/components/ui/data-table/action-menu.test.ts`

**Interfaces:**
- Produces:
  ```ts
  // types.ts
  export type SortDir = 'asc' | 'desc';
  export interface Column {
    key: string;
    label: string;
    sortable?: boolean;
    class?: string;
    render?: (value: any, row: any) => string;
    component?: Component<any>;
    componentSnippet?: Snippet<[{ value: any; row: any; [k: string]: any }]>;
    componentProps?: Record<string, any>;
  }
  export interface ActionItem {
    label: string;
    icon?: Component;
    onclick: (row: any) => void;
    variant?: 'default' | 'destructive';
    show?: (row: any) => boolean; // conditional visibility (e.g. state actions)
  }
  export interface FilterDef {
    key: string;
    label: string;
    options: Array<{ value: string; label: string }>;
  }
  export interface BulkAction {
    label: string;
    icon?: Component;
    onclick: (ids: Set<string>) => void | Promise<void>;
    variant?: 'default' | 'destructive';
    disabled?: (ids: Set<string>) => boolean;
  }
  ```
- `<ActionMenu>` renders a bits-ui `<DropdownMenu.Root>` with a ⋮ trigger and a list of `ActionItem`s (filtered by `show`), firing `onclick(row)` on select. Destructive items get danger color.

- [ ] **Step 1: Write the failing test for ActionMenu**

```ts
// apps/admin/src/lib/components/ui/data-table/action-menu.test.ts
import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/svelte';
import ActionMenu from './action-menu.svelte';
import { Eye, Pencil, Trash2 } from 'lucide-svelte';

describe('ActionMenu', () => {
  it('renders a ⋮ trigger button', () => {
    const { container } = render(ActionMenu, { actions: [], row: { id: '1' } });
    expect(container.querySelector('[data-action-menu-trigger]')).toBeTruthy();
  });

  it('shows only actions whose show() returns true', async () => {
    const onView = vi.fn();
    const onDelete = vi.fn();
    const { container } = render(ActionMenu, {
      row: { id: '1', status: 'active' },
      actions: [
        { label: 'View', icon: Eye, onclick: onView },
        { label: 'Delete', icon: Trash2, onclick: onDelete, show: (r) => r.status === 'inactive' }
      ]
    });
    await fireEvent.click(container.querySelector('[data-action-menu-trigger]')!);
    expect(screen.getByText('View')).toBeTruthy();
    expect(screen.queryByText('Delete')).toBeNull();
  });

  it('fires onclick with the row when an item is selected', async () => {
    const onEdit = vi.fn();
    const { container } = render(ActionMenu, {
      row: { id: '42' },
      actions: [{ label: 'Edit', icon: Pencil, onclick: onEdit }]
    });
    await fireEvent.click(container.querySelector('[data-action-menu-trigger]')!);
    await fireEvent.click(screen.getByText('Edit'));
    expect(onEdit).toHaveBeenCalledWith({ id: '42' });
  });
});
```

Note: bits-ui DropdownMenu may require `waitFor` for the items to appear after trigger click in jsdom. Use `waitFor` if the synchronous `getByText` fails.

- [ ] **Step 2: Run test to verify it fails**

Run: `cd apps/admin && pnpm vitest run src/lib/components/ui/data-table/action-menu.test.ts`
Expected: FAIL — module not found.

- [ ] **Step 3: Write types.ts**

Create `apps/admin/src/lib/components/ui/data-table/types.ts` with the interfaces above.

- [ ] **Step 4: Write action-menu.svelte**

```svelte
<!-- apps/admin/src/lib/components/ui/data-table/action-menu.svelte -->
<script lang="ts">
  import { DropdownMenu } from 'bits-ui';
  import { MoreVertical } from 'lucide-svelte';
  import { clsx } from 'clsx';
  import type { ActionItem } from './types';

  let { actions, row }: { actions: ActionItem[]; row: any } = $props();

  const visibleActions = $derived(actions.filter((a) => !a.show || a.show(row)));
</script>

<DropdownMenu.Root>
  <DropdownMenu.Trigger
    data-action-menu-trigger
    class="inline-flex h-8 w-8 items-center justify-center rounded-md transition-colors hover:bg-black/5 dark:hover:bg-white/10"
    style="color: var(--color-muted)"
    aria-label="Row actions"
  >
    <MoreVertical class="h-4 w-4" />
  </DropdownMenu.Trigger>
  <DropdownMenu.Portal>
    <DropdownMenu.Content
      class="z-50 min-w-[160px] rounded-md border p-1 shadow-lg"
      style="border-color: var(--color-border); background-color: var(--color-surface)"
      sideOffset={4}
    >
      {#each visibleActions as action (action.label)}
        <DropdownMenu.Item
          class={clsx(
            'flex cursor-pointer items-center gap-2 rounded px-2 py-1.5 text-sm outline-none transition-colors hover:bg-black/5 dark:hover:bg-white/10',
            action.variant === 'destructive' && 'text-danger hover:bg-danger/10'
          )}
          style={action.variant === 'destructive' ? 'color: var(--color-danger)' : 'color: var(--color-foreground)'}
          onSelect={() => action.onclick(row)}
        >
          {#if action.icon}
            <action.icon class="h-4 w-4" />
          {/if}
          {action.label}
        </DropdownMenu.Item>
      {/each}
    </DropdownMenu.Content>
  </DropdownMenu.Portal>
</DropdownMenu.Root>
```

- [ ] **Step 5: Update index.ts**

```ts
export { default as DataTable } from './data-table.svelte';
export { default as ActionCell } from './action-cell.svelte';
export { default as ActionMenu } from './action-menu.svelte';
export type { Column, ActionItem, FilterDef, BulkAction, SortDir } from './types';
```

- [ ] **Step 6: Run test to verify it passes**

Run: `cd apps/admin && pnpm vitest run src/lib/components/ui/data-table/action-menu.test.ts`
Expected: PASS (add `waitFor` to the test if bits-ui renders async in jsdom).

- [ ] **Step 7: Commit**

```bash
git add apps/admin/src/lib/components/ui/data-table/types.ts apps/admin/src/lib/components/ui/data-table/action-menu.svelte apps/admin/src/lib/components/ui/data-table/action-menu.test.ts apps/admin/src/lib/components/ui/data-table/index.ts
git commit -m "feat(admin): add DataTable shared types + DropdownMenu action cell"
```

---

### Task 5: DataTable filter-panel + bulk-bar sub-components

**Files:**
- Create: `apps/admin/src/lib/components/ui/data-table/filter-panel.svelte`
- Create: `apps/admin/src/lib/components/ui/data-table/bulk-bar.svelte`
- Modify: `apps/admin/src/lib/components/ui/data-table/index.ts` (export new)
- Test: `apps/admin/src/lib/components/ui/data-table/filter-panel.test.ts`, `bulk-bar.test.ts`

**Interfaces:**
- `<FilterPanel>` props: `{ filters: FilterDef[], value: Record<string,string> (bindable), onapply: () => void, onclear: () => void }`. Renders inside a bits-ui `<Accordion.Root>` (single collapsible item "Filters") with a `<Select>` per filter and Apply/Clear buttons.
- `<BulkBar>` props: `{ selectedIds: Set<string>, actions: BulkAction[], onclear: () => void }`. Renders the count + action buttons + Clear. Hidden when `selectedIds.size === 0` (caller controls, but component also returns null when empty).

- [ ] **Step 1: Write failing tests**

```ts
// filter-panel.test.ts
import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/svelte';
import FilterPanel from './filter-panel.svelte';

describe('FilterPanel', () => {
  it('renders a Select per filter definition', () => {
    render(FilterPanel, {
      filters: [{ key: 'status', label: 'Status', options: [{ value: 'active', label: 'Active' }] }],
      value: {}
    });
    expect(screen.getByText('Status')).toBeTruthy();
  });

  it('fires onclear when Clear is clicked', async () => {
    const onclear = vi.fn();
    const { container } = render(FilterPanel, { filters: [], value: {}, onclear });
    await fireEvent.click(screen.getByRole('button', { name: /clear/i }));
    expect(onclear).toHaveBeenCalled();
  });
});
```

```ts
// bulk-bar.test.ts
import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/svelte';
import BulkBar from './bulk-bar.svelte';
import { Trash2 } from 'lucide-svelte';

describe('BulkBar', () => {
  it('renders nothing when no ids selected', () => {
    const { container } = render(BulkBar, { selectedIds: new Set(), actions: [], onclear: () => {} });
    expect(container.querySelector('[data-bulk-bar]')).toBeNull();
  });

  it('shows count and fires action onclick with the id set', async () => {
    const onDelete = vi.fn();
    const ids = new Set(['1', '2']);
    render(BulkBar, {
      selectedIds: ids,
      actions: [{ label: 'Delete', icon: Trash2, onclick: onDelete, variant: 'destructive' }],
      onclear: () => {}
    });
    expect(screen.getByText(/2 selected/)).toBeTruthy();
    await fireEvent.click(screen.getByRole('button', { name: /delete/i }));
    expect(onDelete).toHaveBeenCalledWith(ids);
  });
});
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd apps/admin && pnpm vitest run src/lib/components/ui/data-table/filter-panel.test.ts src/lib/components/ui/data-table/bulk-bar.test.ts`
Expected: FAIL — modules not found.

- [ ] **Step 3: Write filter-panel.svelte**

```svelte
<!-- apps/admin/src/lib/components/ui/data-table/filter-panel.svelte -->
<script lang="ts">
  import { Accordion } from 'bits-ui';
  import { Select } from '$lib/components/ui/select';
  import { Button } from '$lib/components/ui/button';
  import { Filter, X } from 'lucide-svelte';
  import { t } from '$lib/stores/i18n.svelte';
  import type { FilterDef } from './types';

  let {
    filters = [],
    value = $bindable({}),
    onapply,
    onclear
  }: {
    filters?: FilterDef[];
    value?: Record<string, string>;
    onapply?: () => void;
    onclear?: () => void;
  } = $props();
</script>

<Accordion.Root type="single" collapsible>
  <Accordion.Item value="filters">
    <Accordion.Trigger class="flex w-full items-center gap-2 rounded-lg px-3 py-2 text-sm font-medium" style="color: var(--color-foreground)">
      <Filter class="h-4 w-4" style="color: var(--color-muted)" />
      {t('common.filters', 'Filters')}
      {#if Object.values(value).some((v) => v)}
        <span class="rounded-full px-2 py-0.5 text-xs" style="background: color-mix(in srgb, var(--color-primary) 15%, transparent); color: var(--color-primary)">
          {Object.values(value).filter(Boolean).length}
        </span>
      {/if}
    </Accordion.Trigger>
    <Accordion.Content class="space-y-3 px-3 pb-3">
      <div class="flex flex-wrap gap-3">
        {#each filters as filter (filter.key)}
          <div class="w-44">
            <Select
              bind:value={value[filter.key]}
              options={filter.options}
              placeholder={filter.label}
            />
          </div>
        {/each}
      </div>
      <div class="flex gap-2">
        <Button size="sm" onclick={onapply}>{t('common.apply', 'Apply')}</Button>
        <Button size="sm" variant="ghost" onclick={onclear}><X class="mr-1 h-3.5 w-3.5" />{t('common.clear', 'Clear')}</Button>
      </div>
    </Accordion.Content>
  </Accordion.Item>
</Accordion.Root>
```

- [ ] **Step 4: Write bulk-bar.svelte**

```svelte
<!-- apps/admin/src/lib/components/ui/data-table/bulk-bar.svelte -->
<script lang="ts">
  import { Button } from '$lib/components/ui/button';
  import { clsx } from 'clsx';
  import { t } from '$lib/stores/i18n.svelte';
  import type { BulkAction } from './types';

  let {
    selectedIds,
    actions = [],
    onclear
  }: {
    selectedIds: Set<string>;
    actions?: BulkAction[];
    onclear?: () => void;
  } = $props();
</script>

{#if selectedIds.size > 0}
  <div data-bulk-bar class="mt-3 flex flex-wrap items-center gap-3 rounded-lg p-3" style="background: color-mix(in srgb, var(--color-primary) 12%, transparent)">
    <span class="text-sm font-medium" style="color: var(--color-foreground)">
      {t('common.selected', '{count} selected').replace('{count}', String(selectedIds.size))}
    </span>
    {#each actions as action (action.label)}
      <Button
        size="sm"
        variant={action.variant === 'destructive' ? 'destructive' : 'default'}
        disabled={action.disabled?.(selectedIds) ?? false}
        onclick={() => action.onclick(selectedIds)}
      >
        {#if action.icon}<action.icon class="mr-1 h-3.5 w-3.5" />{/if}
        {action.label}
      </Button>
    {/each}
    <Button size="sm" variant="ghost" style="margin-left: auto" onclick={onclear}>{t('common.clear', 'Clear')}</Button>
  </div>
{/if}
```

- [ ] **Step 5: Update index.ts to export `FilterPanel` and `BulkBar`**

- [ ] **Step 6: Run tests to verify they pass**

Run: `cd apps/admin && pnpm vitest run src/lib/components/ui/data-table/filter-panel.test.ts src/lib/components/ui/data-table/bulk-bar.test.ts`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add apps/admin/src/lib/components/ui/data-table/filter-panel.svelte apps/admin/src/lib/components/ui/data-table/bulk-bar.svelte apps/admin/src/lib/components/ui/data-table/filter-panel.test.ts apps/admin/src/lib/components/ui/data-table/bulk-bar.test.ts apps/admin/src/lib/components/ui/data-table/index.ts
git commit -m "feat(admin): add DataTable FilterPanel + BulkBar sub-components"
```

---

### Task 6: Upgrade DataTable — server-side pagination, built-in action column, filter panel, bulk bar, Create slot

**Files:**
- Modify: `apps/admin/src/lib/components/ui/data-table/data-table.svelte`
- Test: `apps/admin/src/lib/components/ui/data-table/data-table.test.ts`

**Interfaces:**
- Consumes: `ActionMenu`, `FilterPanel`, `BulkBar` from Tasks 4–5; `Column`, `ActionItem`, `FilterDef`, `BulkAction` types.
- Produces: upgraded `<DataTable>` with new optional props (all backward-compatible — existing pages keep working unchanged):
  ```ts
  {
    // existing props retained: columns, data, selectable, selectedIds, onrowclick, rowKey,
    //   emptyMessage, class, showPagination, loading, pageSize, bulkActions, urlSync,
    //   defaultBulkActions, onbulkclear, onsortchange, searchable, searchPlaceholder,
    //   searchParam, onsearch, filterParam, filterValue, onfilterchange

    // NEW — server-side pagination:
    serverSide?: boolean,            // default false (keep client-side behavior)
    totalRows?: number,              // from API meta; required when serverSide
    totalPages?: number,             // from API meta; required when serverSide
    page?: number (bindable),        // 0-based current page; when serverSide, emits onpagechange
    onpagechange?: (page: number) => void,
    onpagesizechange?: (size: number) => void,
    pageSizeOptions?: number[],      // default [10, 25, 50, 100]

    // NEW — built-in action column:
    actions?: ActionItem[],          // rendered via <ActionMenu> in a trailing column
    actionsLabel?: string,           // default t('common.actions')

    // NEW — built-in filter panel:
    filters?: FilterDef[],           // renders <FilterPanel> above the table
    filterValue?: Record<string,string> (bindable),  // multi-filter state
    onfilterapply?: () => void,
    onfilterclear?: () => void,

    // NEW — typed bulk actions:
    bulkActionsList?: BulkAction[],  // rendered via <BulkBar>; coexists with legacy bulkActions snippet

    // NEW — Create button slot:
    createLabel?: string,
    onCreate?: () => void,
  }
  ```
  Behavior rules:
  - When `serverSide` is true: do NOT slice `data` (render all rows as-is — the API already returned the page); use `totalRows`/`totalPages` props for the `<Pagination>` footer; emit `onpagechange`/`onpagesizechange` instead of mutating internal `currentPage`.
  - When `serverSide` is false (default): keep the existing client-side sort + slice behavior unchanged.
  - When `actions` is provided: append a trailing non-sortable column rendering `<ActionMenu {actions} {row} />`.
  - When `filters` is provided: render `<FilterPanel>` above the search box.
  - When `bulkActionsList` is provided and `selectable`: render `<BulkBar>` below the table (replaces the legacy inline bulk bar when present; legacy `bulkActions` snippet still supported for pages not yet migrated).

- [ ] **Step 1: Write failing tests for the upgraded DataTable**

```ts
// apps/admin/src/lib/components/ui/data-table/data-table.test.ts
import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/svelte';
import { DataTable } from './index';
import { Eye, Pencil, Trash2 } from 'lucide-svelte';

vi.mock('$app/state', () => ({ page: { params: {}, url: new URL('http://localhost/') } }));
vi.mock('$app/navigation', () => ({ goto: vi.fn() }));

const rows = [{ id: '1', name: 'Alpha' }, { id: '2', name: 'Beta' }];

describe('DataTable server-side mode', () => {
  it('renders all rows without client slicing when serverSide=true', () => {
    const { container } = render(DataTable, {
      columns: [{ key: 'name', label: 'Name' }],
      data: rows,
      serverSide: true,
      totalRows: 100,
      totalPages: 10,
      page: 0,
      showPagination: true
    });
    // Both rows present (no client-side slice)
    expect(screen.getByText('Alpha')).toBeTruthy();
    expect(screen.getByText('Beta')).toBeTruthy();
  });

  it('emits onpagechange when pagination advances', async () => {
    const onpage = vi.fn();
    const { container } = render(DataTable, {
      columns: [{ key: 'name', label: 'Name' }],
      data: rows,
      serverSide: true,
      totalRows: 100,
      totalPages: 10,
      page: 0,
      onpagechange: onpage,
      showPagination: true
    });
    const next = container.querySelector('[aria-label="Next page"]');
    await fireEvent.click(next!);
    expect(onpage).toHaveBeenCalledWith(1);
  });
});

describe('DataTable action column', () => {
  it('renders an ActionMenu trigger per row when actions provided', () => {
    const { container } = render(DataTable, {
      columns: [{ key: 'name', label: 'Name' }],
      data: rows,
      actions: [{ label: 'View', icon: Eye, onclick: () => {} }]
    });
    const triggers = container.querySelectorAll('[data-action-menu-trigger]');
    expect(triggers.length).toBe(2);
  });
});

describe('DataTable bulk bar', () => {
  it('renders the bulk bar with count when ids selected', () => {
    render(DataTable, {
      columns: [{ key: 'name', label: 'Name' }],
      data: rows,
      selectable: true,
      selectedIds: new Set(['1']),
      bulkActionsList: [{ label: 'Delete', icon: Trash2, onclick: () => {}, variant: 'destructive' }]
    });
    expect(screen.getByText(/1 selected/)).toBeTruthy();
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd apps/admin && pnpm vitest run src/lib/components/ui/data-table/data-table.test.ts`
Expected: FAIL — `serverSide`/`actions`/`bulkActionsList` props not yet implemented.

- [ ] **Step 3: Implement the upgrade in data-table.svelte**

Add the new props to the destructure (all optional, defaults preserve current behavior). Import `ActionMenu`, `FilterPanel`, `BulkBar`, and the types. Key changes:
- Add `serverSide`, `totalRows`, `totalPages`, `page = $bindable(0)`, `onpagechange`, `onpagesizechange`, `pageSizeOptions = [10,25,50,100]`, `actions`, `actionsLabel`, `filters`, `filterValue = $bindable({})`, `onfilterapply`, `onfilterclear`, `bulkActionsList`, `createLabel`, `onCreate`.
- Replace `paginatedData` derivation: when `serverSide`, `paginatedData = data` (no slice); else keep existing slice logic.
- Replace `totalRows`/`totalPages` derivations: when `serverSide`, use the props; else keep `sortedData.length` logic. (Rename the internal derived to `clientTotalRows`/`clientTotalPages` to avoid shadowing the new `totalRows` prop.)
- In the `<Pagination>` footer: when `serverSide`, pass `bind:page` (the new prop), `totalPages` (the prop), `totalItems={totalRows}`, `bind:pageSize`, and an `onpagechange`-aware handler. The existing `<Pagination>` component already supports `bind:page` + `totalPages` + `totalItems` + `bind:pageSize`; add an `onchange` wrapper that calls `onpagechange(page)` after the Pagination updates `page`. (Pagination uses `bind:page` so the parent's `page` updates; add a `$effect` that calls `onpagechange?.(page)` when `serverSide` and `page` changes — guard against the initial run.)
- When `actions` is provided, append a trailing `<th>` (non-sortable, `actionsLabel`) and a trailing `<td>` per row rendering `<ActionMenu {actions} {row} />`. Stop event propagation on the cell click.
- When `filters` is provided, render `<FilterPanel {filters} bind:value={filterValue} onapply={onfilterapply} onclear={onfilterclear} />` above the search box.
- When `bulkActionsList` is provided and `selectable`, render `<BulkBar {selectedIds} actions={bulkActionsList} onclear={clearSelection} />` below the table (replaces the legacy inline bulk bar block when `bulkActionsList` is set; keep the legacy block for the `bulkActionsSnippet` path).
- When `createLabel` + `onCreate` are provided, render a Create `<Button>` top-right (above the table, next to the search box).

Keep the existing `urlSync` sort/search/filter logic intact for backward compat.

- [ ] **Step 4: Run test to verify it passes**

Run: `cd apps/admin && pnpm vitest run src/lib/components/ui/data-table/data-table.test.ts`
Expected: PASS.

- [ ] **Step 5: Run the full existing test suite to confirm no regressions**

Run: `cd apps/admin && pnpm vitest run`
Expected: All pre-existing tests still pass (the upgrade is additive; no existing prop was removed or renamed).

- [ ] **Step 6: Commit**

```bash
git add apps/admin/src/lib/components/ui/data-table/data-table.svelte apps/admin/src/lib/components/ui/data-table/data-table.test.ts
git commit -m "feat(admin): upgrade DataTable with server pagination, action column, filter panel, bulk bar"
```

---

### Task 7: EntityForm — declarative form renderer

**Files:**
- Create: `apps/admin/src/lib/components/entity/entity-form.svelte`
- Create: `apps/admin/src/lib/components/entity/index.ts` (barrel — will also export EntityPage in Task 8)
- Test: `apps/admin/src/lib/components/entity/entity-form.test.ts`

**Interfaces:**
- Produces: `<EntityForm>` with props:
  ```ts
  type FieldDef =
    | { key: string; label: string; type: 'text' | 'textarea' | 'number' | 'email' | 'checkbox'; placeholder?: string; required?: boolean; disabled?: (form: Record<string,any>) => boolean }
    | { key: string; label: string; type: 'select'; options: Array<{value:string;label:string}>; required?: boolean }
    | { key: string; label: string; type: 'checkbox-group'; options: Array<{value:string;label:string}> }
    | { key: string; label: string; type: 'media'; pickerLabel?: string }   // uses MediaPickerModal
    | { key: string; label: string; type: 'user-autocomplete' };             // uses UserAutocomplete

  let { fields, value = $bindable({}), disabled = false }: { fields: FieldDef[]; value?: Record<string,any>; disabled?: boolean } = $props();
  ```
  Renders a vertical stack of labeled inputs bound to `value[key]`. For `media` it opens `<MediaPickerModal>` and stores `{ media_id, url }` into `value[key]`. For `user-autocomplete` it uses `<UserAutocomplete>` and stores the selected user id. Emits no events — the parent reads `value` on submit.

- [ ] **Step 1: Write failing test**

```ts
// entity-form.test.ts
import { describe, it, expect } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/svelte';
import EntityForm from './entity-form.svelte';

describe('EntityForm', () => {
  it('renders a labeled text input for type=text', () => {
    render(EntityForm, {
      fields: [{ key: 'name', label: 'Name', type: 'text', placeholder: 'e.g. javascript' }],
      value: { name: '' }
    });
    expect(screen.getByLabelText('Name')).toBeTruthy();
  });

  it('renders a textarea for type=textarea', () => {
    render(EntityForm, {
      fields: [{ key: 'desc', label: 'Description', type: 'textarea' }],
      value: { desc: '' }
    });
    expect(screen.getByLabelText('Description').tagName).toBe('TEXTAREA');
  });

  it('binds input back into value', async () => {
    const value = { name: '' };
    render(EntityForm, { fields: [{ key: 'name', label: 'Name', type: 'text' }], value });
    await fireEvent.input(screen.getByLabelText('Name'), { target: { value: 'hello' } });
    expect(value.name).toBe('hello');
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd apps/admin && pnpm vitest run src/lib/components/entity/entity-form.test.ts`
Expected: FAIL — module not found.

- [ ] **Step 3: Write entity-form.svelte**

Implement the renderer with a `{#each fields as field}` switch on `field.type`. Use the same input styling as the existing tag edit form (rounded-lg border p-2 text-sm with `var(--color-*)`). For `media`, render a thumbnail preview + "Pick" button that toggles a local `showPicker` state and renders `<MediaPickerModal>`; on select, set `value[field.key] = { media_id, url }`. For `user-autocomplete`, render `<UserAutocomplete bind:value={value[field.key]} />`. Add `aria-label={field.label}` to every input for testability.

Create `index.ts`:
```ts
export { default as EntityForm } from './entity-form.svelte';
export { default as EntityPage } from './entity-page.svelte'; // added in Task 8
```
(Leave the EntityPage export commented or create a stub in Task 8 — for now only export EntityForm.)

- [ ] **Step 4: Run test to verify it passes**

Run: `cd apps/admin && pnpm vitest run src/lib/components/entity/entity-form.test.ts`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add apps/admin/src/lib/components/entity/entity-form.svelte apps/admin/src/lib/components/entity/entity-form.test.ts apps/admin/src/lib/components/entity/index.ts
git commit -m "feat(admin): add declarative EntityForm renderer"
```

---

### Task 8: EntityPage scaffold — composes PageHeader + DataTable + SlideOver + confirm Modal

**Files:**
- Create: `apps/admin/src/lib/components/entity/entity-page.svelte`
- Modify: `apps/admin/src/lib/components/entity/index.ts` (export EntityPage)
- Test: `apps/admin/src/lib/components/entity/entity-page.test.ts`

**Interfaces:**
- Produces: `<EntityPage>` with a single config prop:
  ```ts
  interface EntityPageConfig {
    title: string;
    description?: string;
    columns: Column[];
    filters?: FilterDef[];
    actions?: ActionItem[];            // row actions (View/Edit/Delete + state)
    bulkActions?: BulkAction[];
    createLabel?: string;
    createPermission?: string;
    fields: FieldDef[];                // for the create/edit form
    readonly?: boolean;                // hides Create/Edit, keeps Delete + Clear-all
    // Data fetching — the parent passes a loader fn the scaffold calls with {page,pageSize,q,sort,filter}
    load: (params: { page: number; pageSize: number; q: string; sort: string; filter: Record<string,string> }) =>
      Promise<{ data: any[]; total: number; totalPages: number } | any[]>;
    // Mutations
    create?: (body: Record<string,any>) => Promise<{ ok: boolean; error?: string }>;
    update?: (id: string, body: Record<string,any>) => Promise<{ ok: boolean; error?: string }>;
    remove?: (id: string) => Promise<{ ok: boolean; error?: string }>;
    // Optional: detail-view snippet for the View slide-over
    viewSnippet?: Snippet<[{ row: any }]>;
    // Optional: custom form snippet (overrides fields)
    formSnippet?: Snippet<[{ value: Record<string,any>; editing: any | null }]>;
  }
  let { config }: { config: EntityPageConfig } = $props();
  ```
  The scaffold owns: `loading`/`error`/`success` state, `selectedIds`, `currentPage`, `pageSize`, `searchQuery`, `filterValue`, `sortKey`, the `$effect` that calls `config.load(...)`, the create/edit `<SlideOver>` (header + `<EntityForm>` body + Save/Cancel footer), the view `<SlideOver>` (renders `viewSnippet` or a default key-value grid), and the delete-confirm `<Modal>`. It wires the DataTable's `actions` to open the slide-overs. It derives `canCreate`/`canEdit`/`canDelete` from `hasPermission(config.createPermission)` etc. (accept optional `editPermission`/`deletePermission` in config; default to `createPermission`).

- [ ] **Step 1: Write failing test**

```ts
// entity-page.test.ts
import { describe, it, expect, vi, waitFor } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/svelte';
import { EntityPage } from './index';

vi.mock('$lib/api', () => ({}));
vi.mock('$app/state', () => ({ page: { params: {}, url: new URL('http://localhost/') } }));
vi.mock('$app/navigation', () => ({ goto: vi.fn() }));
vi.mock('$lib/stores/permissions.svelte', () => ({
  hasPermission: () => true,
  getPermissions: () => ['*'],
  getPermissionState: () => ({})
}));

const config = {
  title: 'Tags',
  description: 'Manage platform tags',
  columns: [{ key: 'name', label: 'Name', sortable: true }],
  fields: [{ key: 'name', label: 'Name', type: 'text' }],
  createLabel: 'Create Tag',
  load: vi.fn().mockResolvedValue({ data: [{ id: '1', name: 'Alpha' }], total: 1, totalPages: 1 }),
  create: vi.fn().mockResolvedValue({ ok: true }),
  update: vi.fn().mockResolvedValue({ ok: true }),
  remove: vi.fn().mockResolvedValue({ ok: true })
};

describe('EntityPage', () => {
  it('renders the title and calls load on mount', async () => {
    render(EntityPage, { config });
    expect(screen.getByText('Tags')).toBeTruthy();
    await waitFor(() => expect(config.load).toHaveBeenCalled());
  });

  it('renders rows from the load result', async () => {
    render(EntityPage, { config });
    await waitFor(() => expect(screen.getByText('Alpha')).toBeTruthy());
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd apps/admin && pnpm vitest run src/lib/components/entity/entity-page.test.ts`
Expected: FAIL — module not found.

- [ ] **Step 3: Write entity-page.svelte**

Compose: `<PageHeader>` (title/description + Refresh + Create button when `!readonly && canCreate`), error/success banners, `<Card>` wrapping `<DataTable serverSide {columns} {filters} {actions} {bulkActions} ...>`, the create/edit `<SlideOver>` (with `<EntityForm {fields} bind:value={formValue}>` or `formSnippet`), the view `<SlideOver>` (renders `viewSnippet({ row: viewTarget })` or a default grid of `columns` key/values), and the delete-confirm `<Modal>`. Wire the DataTable `actions` array: View → open view slide-over; Edit → open edit slide-over (seed `formValue` from row); Delete → open confirm modal. Wire `oncreate`/`onupdate`/`onremove` to call `config.create/update/remove` then close + reload. Wire `onfilterapply`/`onfilterclear`/`onsearch`/`onsortchange`/`onpagechange` to update state and let the `$effect` re-call `config.load`. Use `serverSide` mode (the scaffold always paginates on the server).

- [ ] **Step 4: Run test to verify it passes**

Run: `cd apps/admin && pnpm vitest run src/lib/components/entity/entity-page.test.ts`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add apps/admin/src/lib/components/entity/entity-page.svelte apps/admin/src/lib/components/entity/entity-page.test.ts apps/admin/src/lib/components/entity/index.ts
git commit -m "feat(admin): add EntityPage scaffold composing DataTable + SlideOver + confirm Modal"
```

---

### Task 9: Migrate reference page 1 — `/tags`

**Files:**
- Modify: `apps/admin/src/routes/tags/+page.svelte` (rewrite to use `<EntityPage>`)
- Test: `apps/admin/src/routes/tags/page.test.ts` (create)

**Goal:** Replace the 779-line page with an `<EntityPage config={...}>` of ~120 lines. Preserve all current behavior: list with sort/search/filter (min posts), View/Edit/Delete actions, bulk delete, Create, tag moderators sub-flow, logo/cover media, custom field definitions.

**Note:** The tag moderators sub-flow and the custom-field-definitions fetch are not covered by the basic `EntityPage` form. Use `formSnippet` to render the full existing form body (logo/cover pickers, custom fields) inside the slide-over, and add a custom "Manage Moderators" action via the `actions` array that opens a separate `<SlideOver>` (the scaffold's `actions` are row-level; the moderators slide-over is page-local state). This keeps the scaffold generic while letting tags keep its richer UX.

- [ ] **Step 1: Write a smoke test**

```ts
// tags/page.test.ts
import { describe, it, expect, vi, waitFor } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import Page from './+page.svelte';

vi.mock('$lib/api', () => ({
  tagsAdminApi: { list: vi.fn().mockResolvedValue({ ok: true, data: [] }) },
  tagsModeratorsAdminApi: { add: vi.fn(), remove: vi.fn() },
  api: vi.fn().mockResolvedValue({ ok: true, data: [] })
}));
vi.mock('$app/state', () => ({ page: { params: {}, url: new URL('http://localhost/') } }));
vi.mock('$app/navigation', () => ({ goto: vi.fn() }));
vi.mock('$lib/stores/permissions.svelte', () => ({ hasPermission: () => true, getPermissions: () => ['*'], getPermissionState: () => ({}) }));

describe('/tags', () => {
  it('renders the Tags heading', () => {
    render(Page);
    expect(screen.getByText('Tags')).toBeTruthy();
  });
});
```

- [ ] **Step 2: Run test to verify it fails (page still old shape — should still pass; this is a guard)**

Run: `cd apps/admin && pnpm vitest run src/routes/tags/page.test.ts`
Expected: PASS (old page already renders "Tags"). This test guards against the migration breaking the heading.

- [ ] **Step 3: Rewrite the page using EntityPage**

Build the `config` object: `columns` (logo/name/slug/posts/followers/created), `filters` (min posts), `actions` (View, Edit, Delete + Manage Moderators), `bulkActions` (Delete selected), `fields` left minimal (use `formSnippet` for the rich form), `load` wrapping `tagsAdminApi.list`, `create`/`update`/`remove` wrapping `tagsAdminApi.*`, `viewSnippet` rendering the existing view grid. Keep the moderators `<SlideOver>` as page-local state triggered from the action. Keep the custom-field-defs `$effect` and pass `tagFieldDefs` into the `formSnippet` closure.

- [ ] **Step 4: Run test to verify it still passes**

Run: `cd apps/admin && pnpm vitest run src/routes/tags/page.test.ts`
Expected: PASS.

- [ ] **Step 5: Manual smoke check**

Run: `cd apps/admin && pnpm dev` → `/tags` → confirm: list loads, search works, filter (min posts) works, sort works, pagination works, Create opens right slide-over, Edit opens right slide-over pre-filled, View opens right slide-over, Delete opens centered confirm modal, bulk select + Delete works, Manage Moderators opens its own slide-over.

- [ ] **Step 6: Commit**

```bash
git add apps/admin/src/routes/tags/+page.svelte apps/admin/src/routes/tags/page.test.ts
git commit -m "refactor(admin): migrate /tags to EntityPage scaffold + SlideOver"
```

---

### Task 10: Migrate reference page 2 — `/share-platforms` (simple)

**Files:**
- Modify: `apps/admin/src/routes/share-platforms/+page.svelte`
- Test: `apps/admin/src/routes/share-platforms/page.test.ts` (create)

**Goal:** Validate the scaffold on a simple entity (no media, no sub-flows). Should reduce to ~60 lines.

- [ ] **Step 1: Write smoke test** (same pattern as Task 9 Step 1, mock the share-platforms API).

- [ ] **Step 2: Rewrite using `<EntityPage>`** with `fields` (name/slug/icon class — text inputs), `actions` (View/Edit/Delete), `bulkActions` (Delete), `load`/`create`/`update`/`remove` from the existing share-platforms API client.

- [ ] **Step 3: Run test + manual smoke check** — confirm full CRUD + slide-over + confirm modal.

- [ ] **Step 4: Commit**

```bash
git add apps/admin/src/routes/share-platforms/+page.svelte apps/admin/src/routes/share-platforms/page.test.ts
git commit -m "refactor(admin): migrate /share-platforms to EntityPage scaffold"
```

---

### Task 11: Migrate reference page 3 — `/users` (state actions + external ActionCell)

**Files:**
- Modify: `apps/admin/src/routes/users/+page.svelte`
- Modify or delete: `apps/admin/src/routes/users/UserActionsCell.svelte`
- Test: `apps/admin/src/routes/users/page.test.ts` (extend existing)

**Goal:** Validate the scaffold on an entity with state actions (Suspend/Unsuspend, Ban/Unban, Verify) and a separate `UserActionsCell.svelte`. The state actions become conditional `ActionItem`s with `show: (row) => row.status === 'suspended'` etc. The external cell file is deleted (its logic moves into the `actions` config).

- [ ] **Step 1: Extend the existing users page test** to assert the heading still renders and that the action menu trigger appears per row.

- [ ] **Step 2: Rewrite `/users` using `<EntityPage>`** — `actions` includes View, Edit, Delete, Suspend (show when active), Unsuspend (show when suspended), Ban (show when not banned), Unban (show when banned), Verify (show when unverified). Each state action calls the existing users API client method. Use `formSnippet` if the user edit form is richer than `fields` covers (e.g. role assignment).

- [ ] **Step 3: Delete `UserActionsCell.svelte`** if its logic is fully absorbed into the `actions` config. (If any logic cannot move — e.g. a complex permission-gated submenu — keep it and pass it via a custom `actions` `componentSnippet` instead. Prefer deletion.)

- [ ] **Step 4: Run tests + manual smoke check** — confirm all state actions appear conditionally and work.

- [ ] **Step 5: Commit**

```bash
git add apps/admin/src/routes/users/+page.svelte apps/admin/src/routes/users/page.test.ts
git rm apps/admin/src/routes/users/UserActionsCell.svelte  # only if deleted
git commit -m "refactor(admin): migrate /users to EntityPage scaffold with state actions"
```

---

### Task 12: Document the shared-component conventions in AGENTS.md

**Files:**
- Modify or create: `apps/admin/AGENTS.md`

- [ ] **Step 1: Append a "Shared UI components" section** documenting:
  - `<EntityPage {config}>` is the mandatory scaffold for every entity list page. New pages MUST use it.
  - `<DataTable>` upgraded props: `serverSide`, `actions`, `filters`, `bulkActionsList`, `createLabel`/`onCreate`. Use `serverSide` for all server-paginated lists.
  - `<SlideOver>` replaces `<Modal>` for create/edit/view-detail. `<Modal>` is only for short confirmations.
  - `<ActionMenu>` (⋮) is the standard row-action UI; do not render inline action buttons.
  - `<FilterPanel>` (Accordion) is the standard filter UI; do not use `<FilterBar>` for new pages (legacy pages keep it until migrated).
  - Sidebar collapse: `Ctrl/Cmd+B` or the TopBar PanelLeft button; state persists to `localStorage`.
  - Migration recipe: (1) replace `<Modal>` create/edit with `<SlideOver>`; (2) replace `<FilterBar>` + external `<Pagination>` with `<DataTable serverSide filters bulkActionsList actions>`; (3) replace hand-rolled bulk bar with `<BulkBar>`/`bulkActionsList`; (4) replace inline action buttons with `<ActionMenu>` `actions` config; (5) where the page is a standard CRUD list, replace the whole page with `<EntityPage {config}>`.

- [ ] **Step 2: Commit**

```bash
git add apps/admin/AGENTS.md
git commit -m "docs(admin): document shared UI component conventions + migration recipe"
```

---

## Self-Review

**1. Spec coverage:**
- Single source of truth for similar BitsUI components → Tasks 3–8 (SlideOver, upgraded DataTable, ActionMenu, FilterPanel, BulkBar, EntityForm, EntityPage).
- Admin left nav expandable/collapsible → Tasks 1–2 (store + TopBar toggle + Ctrl/Cmd+B + localStorage).
- Settings/configuration/create/edit opens in right sidebar slide-over instead of modal → Tasks 3, 8, 9–11 (SlideOver + EntityPage uses it + reference migrations).
- DataTable inconsistencies (pagination/search/filter/bulk/actions) → Task 6 (server-side mode, built-in filter panel, bulk bar, action column) + Tasks 9–11 (migrations validate it).

**2. Placeholder scan:** No "TBD"/"implement later" — every step has concrete code or a concrete instruction. The `formSnippet`/`viewSnippet` escape hatches are explicit extension points, not placeholders.

**3. Type consistency:** `Column`/`ActionItem`/`FilterDef`/`BulkAction` defined once in `types.ts` (Task 4) and consumed by `ActionMenu` (Task 4), `FilterPanel`/`BulkBar` (Task 5), `DataTable` (Task 6), `EntityPage` (Task 8). `FieldDef` defined in `entity-form.svelte` (Task 7) and consumed by `EntityPage` (Task 8). Prop names match across tasks (`serverSide`, `actions`, `filters`, `bulkActionsList`, `filterValue`).

**Gaps acknowledged (intentional, out of scope for this foundation pass):**
- The remaining ~55 entity pages are NOT migrated here — that is the explicit "Foundation first" decision. The migration recipe in Task 12 + the 3 reference pages give the template for the incremental sweep.
- The 8 list pages not yet using DataTable (`guide-categories`, `seo`, `onboarding`, `university`, `custom-fields/entity-types`, `spaces/presets`, `(app)/rag`) are not migrated here — they follow the same recipe in the next pass.
- The 2 `window.confirm` pages (`blog/[slug]`, `settings`) are not touched — they are detail/settings pages, not entity lists, and will be cleaned up when those pages get their slide-over pass.
