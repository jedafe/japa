<script lang="ts">
	import { t } from '$lib/stores/i18n.svelte';
  import { page } from '$app/state';
  import { goto } from '$app/navigation';
  import { ChevronUp, ChevronDown, ChevronsUpDown } from "lucide-svelte";
  import { clsx, type ClassValue } from "clsx";
  import type { Component, Snippet } from "svelte";
  import { Button } from "$lib/components/ui/button";

  function sanitizeHtml(html: string): string {
    // 1. Escape all HTML metacharacters so no raw markup can execute.
    const escaped = html
      .replace(/&/g, '&amp;')
      .replace(/</g, '&lt;')
      .replace(/>/g, '&gt;')
      .replace(/"/g, '&quot;')
      .replace(/'/g, '&#39;');
    // 2. Re-allow a safe subset of inline tags, permitting only a `class` attribute.
    return escaped
      .replace(/&lt;(b|i|strong|em|code|span)\b((?:\s+class=&quot;[^&]*&quot;)?)\s*&gt;/gi, '<$1$2>')
      .replace(/&lt;\/(b|i|strong|em|code|span)&gt;/gi, '</$1>');
  }

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
    componentProps?: Record<string, any>;
  };

   let {
     columns,
     data = [],
     selectable = false,
     selectedIds = $bindable(new Set<string>()),
     onrowclick,
     emptyMessage = "No data available",
     class: className,
     showPagination = true,
     loading = false,
     pageSize,
      bulkActions,
      urlSync = false,
      defaultBulkActions = false,
      onbulkclear,
      onsortchange,
      // Optional URL-persisted search box (opt-in via `searchable`).
      searchable = false,
      searchPlaceholder = t("common.search", "Search"),
      searchParam = "q",
      onsearch,
      // Optional URL-persisted single filter value (opt-in via `filterParam`).
      filterParam = "",
      filterValue = $bindable(""),
      onfilterchange
    }: {
      columns: Column[];
      data?: any[];
      selectable?: boolean;
      selectedIds?: Set<string>;
      onrowclick?: (row: any) => void;
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
      filterValue?: string;
      onfilterchange?: (v: string) => void;
    } = $props();

   // Bulk actions can be provided either as a `bulkActions` snippet prop or as a
   // child `{#snippet bulkActions()}`. Normalise to a renderable snippet.
   const bulkActionsSnippet = $derived(bulkActions);

  // URL-driven sort (opt-in). Reads initial ?sort=/?dir= and writes back via goto.
  const urlParams = $derived(page.url.searchParams);
  let sortColumn = $state<string | null>(null);
  let sortDirection = $state<"asc" | "desc">("asc");

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
        const aVal = a[sortColumn!];
        const bVal = b[sortColumn!];
        const compare = typeof aVal === "string"
          ? aVal.localeCompare(bVal)
          : (aVal ?? 0) - (bVal ?? 0);
        return sortDirection === "asc" ? compare : -compare;
      });
    }
    return sorted;
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
      const params = new URLSearchParams(page.url.searchParams.toString());
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
       searchInput = page.url.searchParams.get(searchParam) ?? "";
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
     const params = new URLSearchParams(page.url.searchParams.toString());
     if (q) params.set(searchParam, q); else params.delete(searchParam);
     goto(`?${params.toString()}`, { keepFocus: true, noScroll: true });
     onsearch?.(q);
   }
   function clearSearch() {
     searchInput = "";
     if (searchTimer) clearTimeout(searchTimer);
     applySearch();
   }

   // Optional URL-persisted filter value — seeded from and written to `?{filterParam}=`.
   let filterSeeded = false;
   $effect(() => {
     if (filterParam && !filterSeeded) {
       const v = page.url.searchParams.get(filterParam) ?? "";
       if (v) filterValue = v;
       filterSeeded = true;
     }
   });
   let filterPersistReady = false;
   $effect(() => {
     // Persist filterValue to the URL on change (opt-in); skip the initial pass.
     if (!filterParam) return;
     const v = filterValue; // track reactivity
     if (!filterPersistReady) { filterPersistReady = true; return; }
     const params = new URLSearchParams(page.url.searchParams.toString());
     if (v) params.set(filterParam, v); else params.delete(filterParam);
     goto(`?${params.toString()}`, { keepFocus: true, noScroll: true });
     onfilterchange?.(v);
   });
 </script>

<div class={clsx("w-full", className)} aria-live="polite" aria-busy={loading}>
  {#if searchable}
    <!-- Optional debounced search box that persists the query to the URL. -->
    <div class="relative mb-3">
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
              style="color: var(--color-muted); background-color: var(--color-surface)"
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
        </tr>
      </thead>
      <tbody>
        {#each sortedData as row, i (row.id ?? i)}
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
                {#if column.component}
                  <column.component value={row[column.key]} {row} {...column.componentProps ?? {}} />
                {:else if column.render}
                  {@html sanitizeHtml(column.render(row[column.key], row))}
                {:else}
                  {row[column.key] ?? "—"}
                {/if}
              </td>
            {/each}
          </tr>
        {:else}
          <tr>
            <td colspan={columns.length + (selectable ? 1 : 0)} class="h-24 text-center">
              <div class="flex flex-col items-center justify-center" style="color: var(--color-muted)">
                <p>{emptyMessage}</p>
              </div>
            </td>
          </tr>
        {/each}
      </tbody>
    </table>
  </div>
  {#if selectable && selectedIds.size > 0 && (bulkActionsSnippet || defaultBulkActions)}
    <div class="mt-3 flex flex-wrap items-center gap-3 rounded-lg p-3" style="background: color-mix(in srgb, var(--color-primary) 12%, transparent)">
      <span class="text-sm font-medium" style="color: var(--color-foreground)">{selectedIds.size} selected</span>
      {#if bulkActionsSnippet}
        {@render bulkActionsSnippet()}
      {/if}
      <Button variant="ghost" size="sm" onclick={clearSelection} style="margin-left: auto">{t('common.clear', 'Clear')}</Button>
    </div>
  {/if}
</div>
