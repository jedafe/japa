<script lang="ts">
  import { ChevronUp, ChevronDown, ChevronsUpDown } from "lucide-svelte";
  import { clsx, type ClassValue } from "clsx";
  import Pagination from "$lib/components/ui/Pagination.svelte";

  type Column = {
    key: string;
    label: string;
    sortable?: boolean;
    class?: string;
    render?: (value: any, row: any) => string;
  };

  let {
    columns,
    data = [],
    pageSize = $bindable(10),
    selectable = false,
    onrowclick,
    emptyMessage = "No data available",
    class: className
  }: {
    columns: Column[];
    data?: any[];
    pageSize?: number;
    selectable?: boolean;
    onrowclick?: (row: any) => void;
    emptyMessage?: string;
    class?: ClassValue;
  } = $props();

  let currentPage = $state(0);
  let sortColumn = $state<string | null>(null);
  let sortDirection = $state<"asc" | "desc">("asc");
  let selectedRows = $state(new Set<any>());

  let totalPages = $derived(Math.ceil(data.length / pageSize));

  let paginatedData = $derived.by(() => {
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
    const start = currentPage * pageSize;
    return sorted.slice(start, start + pageSize);
  });

  function handleSort(column: string) {
    if (sortColumn === column) {
      sortDirection = sortDirection === "asc" ? "desc" : "asc";
    } else {
      sortColumn = column;
      sortDirection = "asc";
    }
  }

  function toggleRow(row: any) {
    if (selectedRows.has(row.id)) {
      selectedRows.delete(row.id);
    } else {
      selectedRows.add(row.id);
    }
    selectedRows = selectedRows;
  }

  function toggleAll() {
    if (selectedRows.size === paginatedData.length) {
      selectedRows = new Set();
    } else {
      selectedRows = new Set(paginatedData.map(r => r.id));
    }
  }
</script>

<div class={clsx("w-full", className)}>
  <div class="overflow-x-auto rounded-lg border" style="border-color: var(--color-border)">
    <table class="w-full caption-bottom text-sm">
      <thead>
        <tr style="border-color: var(--color-border)">
          {#if selectable}
            <th class="h-12 w-12 px-4 text-left align-middle font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">
              <input
                type="checkbox"
                checked={selectedRows.size === paginatedData.length && paginatedData.length > 0}
                onchange={toggleAll}
                class="h-4 w-4 rounded border-gray-300"
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
        {#each paginatedData as row, i (row.id ?? i)}
          <tr
            class={clsx(
              "transition-colors hover:bg-black/[0.02] dark:hover:bg-white/[0.02]",
              onrowclick && "cursor-pointer",
              selectedRows.has(row.id) && "bg-primary-50 dark:bg-primary-950/30"
            )}
            style="border-color: var(--color-border)"
            onclick={() => onrowclick?.(row)}
          >
            {#if selectable}
              <td class="w-12 p-4 align-middle" style="border-color: var(--color-border)">
                <input
                  type="checkbox"
                  checked={selectedRows.has(row.id)}
                  onchange={() => toggleRow(row)}
                  class="h-4 w-4 rounded border-gray-300"
                  onclick={(e) => e.stopPropagation()}
                />
              </td>
            {/if}
            {#each columns as column}
              <td class="p-4 align-middle" style="border-color: var(--color-border)">
                {#if column.render}
                  {@html column.render(row[column.key], row)}
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

  {#if totalPages > 0}
    <Pagination
      bind:page={currentPage}
      {totalPages}
      totalItems={data.length}
      bind:pageSize
    />
  {/if}
</div>
