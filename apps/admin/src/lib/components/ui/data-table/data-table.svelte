<script lang="ts">
	import { t } from '$lib/stores/i18n.svelte';
  import { page as sveltekitPage } from '$app/state';
  import { replaceState } from '$app/navigation';
  import { ChevronUp, ChevronDown, ChevronsUpDown, Plus, Columns3, Download } from "lucide-svelte";
  import { clsx, type ClassValue } from "clsx";
  import { untrack } from "svelte";
  import type { Component, Snippet } from "svelte";
  import { DropdownMenu } from "bits-ui";
  import { Button } from "$lib/components/ui/button";
  import Pagination from "$lib/components/ui/Pagination.svelte";
  import { sanitizeHtml } from "$lib/utils/html";
  import ActionMenu from "./action-menu.svelte";
  import FilterPanel from "./filter-panel.svelte";
  import BulkBar from "./bulk-bar.svelte";
  import type { ActionItem, FilterDef, BulkAction } from "./types";

  function extractId(row: any): string {
    return String(row?.id ?? '').split('::')[0] || String(row ?? '');
  }

  // Write query params to the URL without triggering a SvelteKit navigation.
  // window.location is the source of truth for query params because
  // $app/state's page.url can lag behind replaceState, causing urlSync to
  // clobber the sort state with stale (empty) params.
  function replaceUrl(params: URLSearchParams) {
    if (typeof window === 'undefined') return;
    const qs = params.toString();
    const url = `${window.location.pathname}${qs ? `?${qs}` : ''}`;
    try {
      replaceState(url, {});
    } catch {
      // ignore URL sync failures in non-browser contexts
    }
  }

  type Column = {
    key: string;
    label: string;
    sortable?: boolean;
    /** Optional API sort key. When present, DataTable sends this value to the
     *  backend (URL `?sort=` and `onsortchange`) while still using `key` to
     *  read the cell value and client-side sorting. */
    sortKey?: string;
    class?: string;
    render?: (value: any, row: any) => string;
    component?: Component<any> | ((...args: any[]) => any);
    componentSnippet?: Snippet<[{ value: any; row: any; [key: string]: any }]>;
    componentProps?: Record<string, any>;
    resizable?: boolean;
    hideable?: boolean;
    width?: number;
    defaultHidden?: boolean;
  };

   let {
     columns,
     data = [],
     selectable = false,
     selectedIds = $bindable(new Set<string>()),
     onrowclick,
     rowKey,
     emptyMessage = t("common.no_data", "No data available"),
     class: className,
     showPagination = true,
     loading = false,
     pageSize = $bindable(10),
      bulkActions,
      urlSync = true,
      defaultBulkActions = false,
      onbulkclear,
      onsortchange,
      // Optional URL-persisted search box (opt-in via `searchable`).
      searchable = false,
      searchPlaceholder = t("common.search", "Search"),
      searchParam = "q",
      onsearch,
      // Optional URL-persisted single filter value (opt-in via `filterParam`); legacy single-filter string state is kept internally so `filterParam`/`onfilterchange` keep working, while the bindable `filterValue` below is the NEW multi-filter Record for the built-in <FilterPanel>.
      filterParam = "",
      onfilterchange,
      // ── NEW props (all optional, additive) ──────────────────────────────
      // Server-side pagination mode.
      serverSide = false,
      totalRows,
      totalPages,
      page = $bindable(0),
      onpagechange,
      onpagesizechange,
      pageSizeOptions = [10, 25, 50, 100],
      defaultPageSize = 10,
      // Built-in action column.
      actions,
      actionsLabel = t("common.actions", "Actions"),
      // Built-in filter panel (multi-filter).
      filters,
      filterValue = $bindable({} as Record<string, string>),
      onfilterapply,
      onfilterclear,
      // Typed bulk actions.
      bulkActionsList,
      // Built-in "Export Selected" bulk action — on by default for every
      // table that doesn't already define its own bulk actions. Opt out with
      // bulkExport={false} (e.g. inline/detail tables where selection is
      // meaningless).
      bulkExport = true,
      exportFilename = "export-selected",
      // Create button slot.
      createLabel,
      onCreate
    }: {
      columns: Column[];
      data?: any[];
      selectable?: boolean;
      selectedIds?: Set<string>;
      onrowclick?: (row: any) => void;
      rowKey?: (row: any) => string;
      emptyMessage?: string;
      class?: ClassValue;
      showPagination?: boolean;
      loading?: boolean;
      pageSize?: number;
      bulkActions?: Snippet;
      urlSync?: boolean;
      defaultBulkActions?: boolean;
      onbulkclear?: () => void;
      onsortchange?: (sort: { key: string | null; dir: "asc" | "desc" }) => void;
      searchable?: boolean;
      searchPlaceholder?: string;
      searchParam?: string;
      onsearch?: (q: string) => void;
      filterParam?: string;
      onfilterchange?: (v: string) => void;
      // NEW
      serverSide?: boolean;
      totalRows?: number;
      totalPages?: number;
      page?: number;
      onpagechange?: (page: number) => void;
      onpagesizechange?: (size: number) => void;
      pageSizeOptions?: number[];
      defaultPageSize?: number;
      actions?: ActionItem[];
      actionsLabel?: string;
      filters?: FilterDef[];
      filterValue?: Record<string, string>;
      onfilterapply?: () => void;
      onfilterclear?: () => void;
      bulkActionsList?: BulkAction[];
      bulkExport?: boolean;
      exportFilename?: string;
      createLabel?: string;
      onCreate?: () => void;
    } = $props();

   // Bulk actions can be provided either as a `bulkActions` snippet prop or as a
   // child `{#snippet bulkActions()}`. Normalise to a renderable snippet.
   const bulkActionsSnippet = $derived(bulkActions);

  // ── Column widths (resizable columns) ───────────────────────────────────
  // Seeded from each column's `width` field at init; updated by dragging resize handles.
  // Double-click a resize handle to reset a column to auto-width.
  let columnWidths = $state<Record<string, number>>({});

  function startResize(e: MouseEvent, colKey: string) {
    e.preventDefault();
    e.stopPropagation();
    const th = (e.currentTarget as HTMLElement).parentElement as HTMLElement;
    const startX = e.clientX;
    const startWidth = th.offsetWidth;
    function onMove(ev: MouseEvent) {
      const delta = ev.clientX - startX;
      columnWidths = { ...columnWidths, [colKey]: Math.max(60, startWidth + delta) };
    }
    function onUp() {
      document.removeEventListener('mousemove', onMove);
      document.removeEventListener('mouseup', onUp);
      document.body.style.cursor = '';
      document.body.style.userSelect = '';
    }
    document.addEventListener('mousemove', onMove);
    document.addEventListener('mouseup', onUp);
    document.body.style.cursor = 'col-resize';
    document.body.style.userSelect = 'none';
  }

  // ── Column visibility (hide/unhide) ─────────────────────────────────────
  // Seeded from `defaultHidden` at init; user toggles modify the set thereafter.
  // untrack() makes the "initial value only" intent explicit (columns may be a
  // reactive prop, but we deliberately only read it once here).
  let hiddenColumns = $state<Set<string>>(
    untrack(() => new Set(columns.filter((c) => c.defaultHidden).map((c) => c.key)))
  );

  const hideableColumns = $derived(columns.filter((c) => c.hideable !== false));
  const visibleColumns = $derived(columns.filter((c) => !hiddenColumns.has(c.key)));

  function toggleColumn(key: string) {
    const next = new Set(hiddenColumns);
    if (next.has(key)) next.delete(key);
    else next.add(key);
    hiddenColumns = next;
  }

  // URL-driven sort (opt-in). Reads initial ?sort=/?dir= and writes back via replaceState.
  let sortColumn = $state<string | null>(null);
  let sortDirection = $state<"asc" | "desc">("asc");

  // Client-side pagination state (used only when serverSide is false).
  let currentPage = $state(0);

  $effect(() => {
    if (urlSync) {
      // Track SvelteKit page state for popstate/back/forward, but read the
      // actual URL (which replaceState updates immediately) so we don't restore
      // stale/empty params and clobber the current sort state.
      void sveltekitPage.url.searchParams;
      const searchParams = new URL(window.location.href).searchParams;
      const urlSort = searchParams.get("sort");
      // Map an API sort key back to the column's data `key` so the UI and
      // client-side sorting use the correct row field.
      sortColumn = urlSort
        ? (columns.find((c) => (c.sortKey ?? c.key) === urlSort)?.key ?? urlSort)
        : null;
      sortDirection = (searchParams.get("dir") as "asc" | "desc") ?? "asc";
    }
  });

  let sortedData = $derived.by(() => {
    let sorted = [...data];
    if (sortColumn) {
      sorted.sort((a, b) => {
        let aVal = a[sortColumn!];
        let bVal = b[sortColumn!];
        // Handle arrays (e.g. groups) by joining for comparison
        if (Array.isArray(aVal)) aVal = aVal.join(', ');
        if (Array.isArray(bVal)) bVal = bVal.join(', ');
        const compare = typeof aVal === "string"
          ? aVal.localeCompare(bVal)
          : (aVal ?? 0) - (bVal ?? 0);
        return sortDirection === "asc" ? compare : -compare;
      });
    }
    return sorted;
  });

  // Client-side totals (renamed to avoid shadowing the new totalRows/totalPages props).
  const clientTotalRows = $derived(sortedData.length);
  const clientTotalPages = $derived(Math.max(1, Math.ceil(clientTotalRows / pageSize)));

  // Effective totals: server-side uses the API-provided props; client-side uses the sorted data length.
  const effectiveTotalRows = $derived(serverSide ? (totalRows ?? 0) : clientTotalRows);
  const effectiveTotalPages = $derived(serverSide ? (totalPages ?? 1) : clientTotalPages);

  // Paginated data: server-side renders rows as-is (the API already returned the page); client-side slices the sorted data.
  const paginatedData = $derived(
    serverSide
      ? data
      : showPagination
        ? sortedData.slice(currentPage * pageSize, (currentPage + 1) * pageSize)
        : sortedData
  );

  // Reset to first page when data or page size changes (client-side only).
  $effect(() => {
    if (serverSide) return;
    void clientTotalRows;
    void pageSize;
    if (currentPage >= clientTotalPages) currentPage = 0;
  });

  // Server-side: emit onpagechange when the bound `page` prop changes (skip the initial run so we don't fire on mount).
  let pageChangeReady = false;
  $effect(() => {
    if (!serverSide) return;
    const p = page; // track reactivity
    if (!pageChangeReady) { pageChangeReady = true; return; }
    onpagechange?.(p);
  });

  function handleSort(column: Column) {
    const dataKey = column.key;
    const apiKey = column.sortKey ?? dataKey;
    if (sortColumn === dataKey) {
      sortDirection = sortDirection === "asc" ? "desc" : "asc";
    } else {
      sortColumn = dataKey;
      sortDirection = "asc";
    }
    // Notify parent and URL using the API sort key, while keeping the
    // internal sort state on the data key.
    onsortchange?.({ key: apiKey, dir: sortDirection });
    if (urlSync) {
      const params = new URLSearchParams(window.location.search);
      if (sortColumn) {
        params.set("sort", apiKey);
        params.set("dir", sortDirection);
      } else {
        params.delete("sort");
        params.delete("dir");
      }
      replaceUrl(params);
    }
  }

  function toggleRow(row: any) {
    const id = extractId(row);
    const next = new Set(selectedIds);
    if (next.has(id)) {
      next.delete(id);
    } else {
      next.add(id);
    }
    selectedIds = next;
  }

  // IDs visible on the current page — used for page-scoped select-all.
  const visibleIds = $derived(paginatedData.map(r => extractId(r)));
  const allSelected = $derived(visibleIds.length > 0 && visibleIds.every((id) => selectedIds.has(id)));
  const someSelected = $derived(visibleIds.some((id) => selectedIds.has(id)) && !allSelected);

  function toggleAll() {
    const next = new Set(selectedIds);
    if (allSelected) {
      for (const id of visibleIds) next.delete(id);
    } else {
      for (const id of visibleIds) next.add(id);
    }
    selectedIds = next;
  }

   function clearSelection() {
     selectedIds = new Set();
     onbulkclear?.();
   }

   // Optional debounced search box — writes `?{searchParam}=` to the URL.
   let searchInput = $state("");
   let searchSeeded = false;
   let searchTimer: ReturnType<typeof setTimeout> | undefined;
   $effect(() => {
     // Seed the search box from the URL once so a reload restores the query.
     if (searchable && !searchSeeded) {
       searchInput = new URLSearchParams(window.location.search).get(searchParam) ?? "";
       searchSeeded = true;
     }
   });
   // Clear any pending search debounce when the table unmounts.
   $effect(() => {
     return () => {
       if (searchTimer) clearTimeout(searchTimer);
     };
   });
   function handleSearchInput(e: Event) {
     searchInput = (e.currentTarget as HTMLInputElement).value;
     if (searchTimer) clearTimeout(searchTimer);
     searchTimer = setTimeout(applySearch, 300); // debounce URL writes + refetch
   }
   function applySearch() {
     const q = searchInput.trim();
     if (urlSync) {
       const params = new URLSearchParams(window.location.search);
       if (q) params.set(searchParam, q); else params.delete(searchParam);
       replaceUrl(params);
     }
     onsearch?.(q);
   }
   function clearSearch() {
     searchInput = "";
     if (searchTimer) clearTimeout(searchTimer);
     applySearch();
   }

   // Optional URL-persisted single filter value — seeded from and written to `?{filterParam}=`; uses internal state so the bindable `filterValue` prop (Record, for the built-in FilterPanel) is not disturbed.
   let singleFilterValue = $state("");
   let filterSeeded = false;
   $effect(() => {
     if (filterParam && !filterSeeded) {
       const v = new URLSearchParams(window.location.search).get(filterParam) ?? "";
       if (v) singleFilterValue = v;
       filterSeeded = true;
     }
   });
   let filterPersistReady = false;
   $effect(() => {
     // Persist singleFilterValue to the URL on change (opt-in); skip initial pass.
     if (!filterParam) return;
     const v = singleFilterValue; // track reactivity
     if (!filterPersistReady) { filterPersistReady = true; return; }
     if (urlSync) {
       const params = new URLSearchParams(window.location.search);
       if (v) params.set(filterParam, v); else params.delete(filterParam);
       replaceUrl(params);
     }
     onfilterchange?.(v);
   });

   // Server-side page-size change handler — emits onpagesizechange after the Pagination component updates the bound pageSize.
   function handlePageSizeChange() {
     if (serverSide) onpagesizechange?.(pageSize);
   }
   let lastPageSize = $state(pageSize);
   $effect(() => {
     if (!serverSide) return;
     const s = pageSize; // track reactivity
     if (s !== lastPageSize) {
       lastPageSize = s;
       handlePageSizeChange();
     }
   });

   // ── Bulk selection ────────────────────────────────────────────────────
   // CSV export of the currently selected rows — the universal built-in bulk
   // action. Skips label-less columns (avatar stacks, action cells).
   function csvEscape(v: any): string {
     let s = v == null ? "" : String(v);
     // Formula neutralization: prefix =, +, -, @, tab, pipe, and DDE
     // payloads with a single quote so spreadsheets render text, not formulas.
     if (/^[=+\-@\t|]/.test(s) || /^cmd\|/i.test(s)) s = `'${s}`;
     return /[",\n]/.test(s) ? `"${s.replace(/"/g, '""')}"` : s;
   }
   function exportSelectedCsv(ids: Set<string>) {
     const selected = sortedData.filter((r) => ids.has(extractId(r)));
     if (!selected.length) return;
     const cols = visibleColumns.filter((c) => c.key && c.label);
     const header = cols.map((c) => csvEscape(c.label)).join(",");
     const lines = selected.map((row) => cols.map((c) => csvEscape(row[c.key])).join(","));
     const csv = [header, ...lines].join("\n");
     const blob = new Blob([csv], { type: "text/csv;charset=utf-8" });
     const url = URL.createObjectURL(blob);
     const a = document.createElement("a");
     a.href = url;
     a.download = `${exportFilename}.csv`;
     document.body.appendChild(a);
     a.click();
     document.body.removeChild(a);
     URL.revokeObjectURL(url);
   }
   const exportSelectedAction: BulkAction = {
     label: t("common.exportSelected", "Export Selected"),
     icon: Download,
     onclick: exportSelectedCsv,
   };
   // Tables using the legacy snippet path (bulkActions snippet / defaultBulkActions)
   // keep their own bar — the built-in export is not injected for them. For the
   // typed path, the built-in "Export Selected" is prepended unless the caller
   // already supplies an action with the same label or opts out via bulkExport.
   const resolvedBulkActions = $derived.by<BulkAction[]>(() => {
     if (bulkActionsSnippet || defaultBulkActions) return bulkActionsList ?? [];
     const custom = bulkActionsList ?? [];
     const hasExport = custom.some((a) => a.label === exportSelectedAction.label);
     return bulkExport && !hasExport ? [exportSelectedAction, ...custom] : custom;
   });
   // Row selection is on when explicitly requested or any bulk action exists.
   const canSelect = $derived(selectable || resolvedBulkActions.length > 0);
   // Whether to show the built-in BulkBar (typed bulkActionsList path).
   const showBulkBar = $derived(resolvedBulkActions.length > 0);
   // Whether to show the legacy inline bulk bar (snippet path).
   const showLegacyBulkBar = $derived(selectable && !bulkActionsList && (bulkActionsSnippet || defaultBulkActions));
   // Rows matching the current selection — passed to BulkBar for disabled predicates.
   const selectedRows = $derived(sortedData.filter((r) => selectedIds.has(extractId(r))));
 </script>

<div class={clsx("w-full", className)} aria-live="polite" aria-busy={loading}>
  {#if filters}
    <div class="mb-3">
      <FilterPanel {filters} bind:value={filterValue} onapply={onfilterapply} onclear={onfilterclear} />
    </div>
  {/if}
  {#if searchable || (createLabel && onCreate) || hideableColumns.length > 0}
    <div class="mb-3 flex items-center gap-3">
      {#if searchable}
        <!-- Optional debounced search box that persists the query to the URL. -->
        <div class="relative flex-1">
          <input
            type="search"
            placeholder={searchPlaceholder}
            bind:value={searchInput}
            oninput={handleSearchInput}
            aria-label={searchPlaceholder}
            class="w-full rounded-lg border p-2 pr-10 text-sm outline-none focus:ring-2"
            style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground); --tw-ring-color: var(--color-primary)"
          />
          {#if searchInput}
            <button
              type="button"
              onclick={clearSearch}
              aria-label={t("common.clearSelection", "Clear search")}
              class="absolute right-2 top-1/2 -translate-y-1/2 rounded p-1"
              style="color: var(--color-muted)"
            >
              ✕
            </button>
          {/if}
        </div>
      {/if}
      {#if hideableColumns.length > 0}
        <DropdownMenu.Root>
          <DropdownMenu.Trigger
            class="inline-flex h-9 items-center gap-1.5 rounded-lg border px-3 text-sm font-medium transition-colors hover:bg-black/5 dark:hover:bg-white/10"
            style="border-color: var(--color-border); color: var(--color-foreground)"
            aria-label={t('dataTable.columns', 'Columns')}
          >
            <Columns3 class="h-4 w-4" />
            <span class="hidden sm:inline">{t('dataTable.columns', 'Columns')}</span>
          </DropdownMenu.Trigger>
          <DropdownMenu.Portal>
            <DropdownMenu.Content
              class="z-50 min-w-[180px] rounded-md border p-1 shadow-lg"
              style="border-color: var(--color-border); background-color: var(--color-surface)"
              sideOffset={4}
            >
              {#each hideableColumns as col (col.key)}
                <DropdownMenu.Item
                  class="flex cursor-pointer items-center gap-2 rounded px-2 py-1.5 text-sm outline-none transition-colors hover:bg-black/5 dark:hover:bg-white/10"
                  style="color: var(--color-foreground)"
                  onSelect={(e) => { e.preventDefault(); toggleColumn(col.key); }}
                >
                  <input
                    type="checkbox"
                    checked={!hiddenColumns.has(col.key)}
                    readonly
                    class="h-4 w-4 rounded border-gray-300 pointer-events-none"
                    aria-label={col.label}
                  />
                  {col.label}
                </DropdownMenu.Item>
              {/each}
            </DropdownMenu.Content>
          </DropdownMenu.Portal>
        </DropdownMenu.Root>
      {/if}
      {#if createLabel && onCreate}
        <Button onclick={onCreate}>
          <Plus class="mr-1 h-4 w-4" />
          {createLabel}
        </Button>
      {/if}
    </div>
  {/if}
  <div class="overflow-x-auto rounded-lg border" style="border-color: var(--color-border)">
    <table class="w-full caption-bottom text-sm">
      <thead>
        <tr style="border-color: var(--color-border)">
          {#if canSelect}
            <th class="h-12 w-12 px-4 text-left align-middle font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">
              <input
                type="checkbox"
                checked={allSelected}
                indeterminate={someSelected}
                onchange={toggleAll}
                class="h-4 w-4 rounded border-gray-300"
                aria-label={t('dataTable.selectAll', 'Select all rows on this page')}
              />
            </th>
          {/if}
          {#each visibleColumns as column (column.key)}
            <th
              class={clsx(
                "relative h-12 px-4 text-left align-middle font-medium",
                column.sortable && "cursor-pointer select-none",
                column.class
              )}
              style={`color: var(--color-foreground-alt); background-color: var(--color-surface)${columnWidths[column.key] ? `; width: ${columnWidths[column.key]}px; min-width: ${columnWidths[column.key]}px` : ''}`}
              scope="col"
              aria-sort={column.sortable ? (sortColumn === column.key ? (sortDirection === "asc" ? "ascending" : "descending") : "none") : undefined}
              onclick={() => column.sortable && handleSort(column)}
            >
              <div class="flex items-center gap-2 pr-2">
                {column.label}
                {#if column.sortable}
                  {#if sortColumn === column.key}
                    {#if sortDirection === "asc"}
                      <ChevronUp class="h-4 w-4" />
                    {:else}
                      <ChevronDown class="h-4 w-4" />
                    {/if}
                  {:else}
                    <ChevronsUpDown class="h-4 w-4" style="color: var(--color-muted); opacity: 0.5" />
                  {/if}
                {/if}
              </div>
              {#if column.resizable !== false}
                <button
                  type="button"
                  class="absolute right-0 top-0 h-full w-1.5 cursor-col-resize touch-none border-none bg-transparent"
                  onmousedown={(e) => startResize(e, column.key)}
                  onclick={(e) => e.stopPropagation()}
                  ondblclick={(e) => { e.stopPropagation(); if (columnWidths[column.key]) { const next = { ...columnWidths }; delete next[column.key]; columnWidths = next; } }}
                  aria-label={t('dataTable.resizeColumn', 'Resize {label}').replace('{label}', column.label)}
                ></button>
              {/if}
            </th>
          {/each}
          {#if actions}
            <th class="h-12 w-12 px-4 text-left align-middle font-medium" style="color: var(--color-foreground-alt); background-color: var(--color-surface)" scope="col">
              {actionsLabel}
            </th>
          {/if}
        </tr>
      </thead>
      <tbody>
        {#each paginatedData as row, i (rowKey ? rowKey(row) : (row.id ?? i))}
          <tr
            class={clsx(
              "transition-colors hover:bg-black/[0.02] dark:hover:bg-white/[0.02]",
              onrowclick && "cursor-pointer",
              selectedIds.has(extractId(row)) && "bg-primary-50 dark:bg-primary-950/30"
            )}
            style="border-color: var(--color-border)"
            onclick={() => onrowclick?.(row)}
          >
            {#if canSelect}
              <td class="w-12 p-4 align-middle" style="border-color: var(--color-border)">
                <input
                  type="checkbox"
                  checked={selectedIds.has(extractId(row))}
                  onchange={() => toggleRow(row)}
                  class="h-4 w-4 rounded border-gray-300"
                  onclick={(e) => e.stopPropagation()}
                  aria-label={t('dataTable.selectRow', 'Select row {id}').replace('{id}', extractId(row))}
                />
              </td>
            {/if}
            {#each visibleColumns as column (column.key)}
              <td class="p-4 align-middle" style="border-color: var(--color-border)">
                {#if column.componentSnippet}
                  {@render column.componentSnippet({ value: row[column.key], row, ...column.componentProps })}
                {:else if column.component}
                  {@const CellComponent = column.component}
                  <CellComponent value={row[column.key]} {row} {...column.componentProps ?? {}} />
                {:else if column.render}
                  {@html sanitizeHtml(column.render(row[column.key], row))}
                {:else}
                  {row[column.key] ?? "—"}
                {/if}
              </td>
            {/each}
            {#if actions}
              <td class="w-12 p-4 align-middle" style="border-color: var(--color-border)" onclick={(e) => e.stopPropagation()}>
                <ActionMenu {actions} {row} />
              </td>
            {/if}
          </tr>
        {:else}
          <tr>
            <td colspan={visibleColumns.length + (canSelect ? 1 : 0) + (actions ? 1 : 0)} class="h-24 text-center">
              <div class="flex flex-col items-center justify-center" style="color: var(--color-muted)">
                <p>{emptyMessage}</p>
              </div>
            </td>
          </tr>
        {/each}
      </tbody>
    </table>
  </div>
  {#if showPagination && effectiveTotalRows > 0}
    {#if serverSide}
      <Pagination
        bind:page
        totalPages={effectiveTotalPages}
        totalItems={effectiveTotalRows}
        bind:pageSize
        {pageSizeOptions}
        {urlSync}
        {defaultPageSize}
      />
    {:else}
      <Pagination
        bind:page={currentPage}
        totalPages={effectiveTotalPages}
        totalItems={effectiveTotalRows}
        bind:pageSize
        {pageSizeOptions}
        {urlSync}
        {defaultPageSize}
      />
    {/if}
  {/if}
  {#if showBulkBar}
    <BulkBar {selectedIds} {selectedRows} actions={resolvedBulkActions} onclear={clearSelection} />
  {/if}
  {#if showLegacyBulkBar}
    <div class="mt-3 flex flex-wrap items-center gap-3 rounded-lg p-3" style="background: color-mix(in srgb, var(--color-primary) 12%, transparent); opacity: {selectedIds.size > 0 ? '1' : '0.5'}">
      <span class="text-sm font-medium" style="color: var(--color-foreground)">{t('common.selected', '{count} selected').replace('{count}', String(selectedIds.size))}</span>
      {#if bulkActionsSnippet}
        {@render bulkActionsSnippet()}
      {/if}
      <Button variant="ghost" size="sm" onclick={clearSelection} disabled={selectedIds.size === 0} style="margin-left: auto">{t('common.clear', 'Clear')}</Button>
    </div>
  {/if}
</div>
