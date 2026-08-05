<script lang="ts">
	import { t } from '$lib/stores/i18n.svelte';
  import { page as sveltekitPage } from '$app/state';
  import { goto } from '$app/navigation';
  import { ChevronUp, ChevronDown, ChevronsUpDown, Plus } from "lucide-svelte";
  import { clsx, type ClassValue } from "clsx";
  import type { Component, Snippet } from "svelte";
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

  type Column = {
    key: string;
    label: string;
    sortable?: boolean;
    class?: string;
    render?: (value: any, row: any) => string;
    component?: Component<any> | ((...args: any[]) => any);
    componentSnippet?: Snippet<[{ value: any; row: any; [key: string]: any }]>;
    componentProps?: Record<string, any>;
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
      actions?: ActionItem[];
      actionsLabel?: string;
      filters?: FilterDef[];
      filterValue?: Record<string, string>;
      onfilterapply?: () => void;
      onfilterclear?: () => void;
      bulkActionsList?: BulkAction[];
      createLabel?: string;
      onCreate?: () => void;
    } = $props();

   // Bulk actions can be provided either as a `bulkActions` snippet prop or as a
   // child `{#snippet bulkActions()}`. Normalise to a renderable snippet.
   const bulkActionsSnippet = $derived(bulkActions);

  // URL-driven sort (opt-in). Reads initial ?sort=/?dir= and writes back via goto.
  const urlParams = $derived(sveltekitPage.url.searchParams);
  let sortColumn = $state<string | null>(null);
  let sortDirection = $state<"asc" | "desc">("asc");

  // Client-side pagination state (used only when serverSide is false).
  let currentPage = $state(0);

  $effect(() => {
    if (urlSync) {
      sortColumn = urlParams.get("sort") ?? null;
      sortDirection = (urlParams.get("dir") as "asc" | "desc") ?? "asc";
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

  function handleSort(column: string) {
    if (sortColumn === column) {
      sortDirection = sortDirection === "asc" ? "desc" : "asc";
    } else {
      sortColumn = column;
      sortDirection = "asc";
    }
    onsortchange?.({ key: sortColumn, dir: sortDirection });
    if (urlSync) {
      const params = new URLSearchParams(sveltekitPage.url.searchParams.toString());
      if (sortColumn) {
        params.set("sort", sortColumn);
        params.set("dir", sortDirection);
      } else {
        params.delete("sort");
        params.delete("dir");
      }
      goto(`?${params.toString()}`, { keepFocus: true, noScroll: true });
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

  function toggleAll() {
    if (selectedIds.size === sortedData.length && sortedData.length > 0) {
      selectedIds = new Set();
    } else {
      selectedIds = new Set(sortedData.map(r => extractId(r)));
    }
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
       searchInput = sveltekitPage.url.searchParams.get(searchParam) ?? "";
       searchSeeded = true;
     }
   });
   function handleSearchInput(e: Event) {
     searchInput = (e.currentTarget as HTMLInputElement).value;
     if (searchTimer) clearTimeout(searchTimer);
     searchTimer = setTimeout(applySearch, 300); // debounce URL writes + refetch
   }
   function applySearch() {
     const q = searchInput.trim();
     const params = new URLSearchParams(sveltekitPage.url.searchParams.toString());
     if (q) params.set(searchParam, q); else params.delete(searchParam);
     goto(`?${params.toString()}`, { keepFocus: true, noScroll: true });
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
       const v = sveltekitPage.url.searchParams.get(filterParam) ?? "";
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
     const params = new URLSearchParams(sveltekitPage.url.searchParams.toString());
     if (v) params.set(filterParam, v); else params.delete(filterParam);
     goto(`?${params.toString()}`, { keepFocus: true, noScroll: true });
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

   // Whether to show the built-in BulkBar (typed bulkActionsList path).
   const showBulkBar = $derived(!!bulkActionsList && selectable);
   // Whether to show the legacy inline bulk bar (snippet path).
   const showLegacyBulkBar = $derived(selectable && !bulkActionsList && (bulkActionsSnippet || defaultBulkActions));
 </script>

<div class={clsx("w-full", className)} aria-live="polite" aria-busy={loading}>
  {#if filters}
    <div class="mb-3">
      <FilterPanel {filters} bind:value={filterValue} onapply={onfilterapply} onclear={onfilterclear} />
    </div>
  {/if}
  {#if searchable || (createLabel && onCreate)}
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
          {#if selectable}
            <th class="h-12 w-12 px-4 text-left align-middle font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">
              <input
                type="checkbox"
                checked={selectedIds.size === sortedData.length && sortedData.length > 0}
                onchange={toggleAll}
                class="h-4 w-4 rounded border-gray-300"
                aria-label="Select all rows"
              />
            </th>
          {/if}
          {#each columns as column}
            <th
              class={clsx(
                "h-12 px-4 text-left align-middle font-medium",
                column.sortable && "cursor-pointer select-none",
                column.class
              )}
              style="color: var(--color-foreground-alt); background-color: var(--color-surface)"
              scope="col"
              aria-sort={column.sortable ? (sortColumn === column.key ? (sortDirection === "asc" ? "ascending" : "descending") : "none") : undefined}
              onclick={() => column.sortable && handleSort(column.key)}
            >
              <div class="flex items-center gap-2">
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
            {#if selectable}
              <td class="w-12 p-4 align-middle" style="border-color: var(--color-border)">
                <input
                  type="checkbox"
                  checked={selectedIds.has(extractId(row))}
                  onchange={() => toggleRow(row)}
                  class="h-4 w-4 rounded border-gray-300"
                  onclick={(e) => e.stopPropagation()}
                  aria-label={`Select row ${extractId(row)}`}
                />
              </td>
            {/if}
            {#each columns as column}
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
            <td colspan={columns.length + (selectable ? 1 : 0) + (actions ? 1 : 0)} class="h-24 text-center">
              <div class="flex flex-col items-center justify-center" style="color: var(--color-muted)">
                <p>{emptyMessage}</p>
              </div>
            </td>
          </tr>
        {/each}
      </tbody>
    </table>
  </div>
  {#if showPagination && effectiveTotalRows > pageSize}
    {#if serverSide}
      <Pagination
        bind:page
        totalPages={effectiveTotalPages}
        totalItems={effectiveTotalRows}
        bind:pageSize
        {pageSizeOptions}
        urlSync={false}
      />
    {:else}
      <Pagination
        bind:page={currentPage}
        totalPages={effectiveTotalPages}
        totalItems={effectiveTotalRows}
        bind:pageSize
        {pageSizeOptions}
        urlSync={false}
      />
    {/if}
  {/if}
  {#if showBulkBar}
    <BulkBar {selectedIds} actions={bulkActionsList ?? []} onclear={clearSelection} />
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
