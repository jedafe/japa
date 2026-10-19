<script lang="ts">
	import { tick } from 'svelte';
	import { t } from '$lib/stores/i18n.svelte';
	import {
		ChevronLeft,
		ChevronRight,
		ChevronsLeft,
		ChevronsRight,
		ChevronUp,
		ChevronDown,
		ChevronsUpDown,
		Ellipsis,
		Inbox
	} from 'lucide-svelte';
	import { Menu, MenuItem } from '@japaconnect/ui';
	import AutocompleteSelect from '$lib/components/ui/AutocompleteSelect.svelte';
	import type { DataTableColumn, DataTableRowAction, DataTableBulkAction } from '$lib/api/types';
	import { sanitizeHtml } from '$lib/utils';

	let {
		columns = [],
		rows = [],
		totalRows = 0,
		page = $bindable(0),
		pageSize = $bindable(10),
		sortKey = $bindable(''),
		sortDirection = $bindable<'asc' | 'desc'>('asc'),
		selectedIds = $bindable(new Set<string>()),
		rowActions = [],
		bulkActions = [],
		loading = false,
		emptyMessage = t('common.no_data', 'No data available'),
		emptyIcon = undefined as any,
		onRowClick = undefined,
		getRowId = (row) => (row as Record<string, unknown>).id as string,
		selectable = true,
		pageSizeOptions = [10, 25, 50, 100]
	}: {
		columns?: DataTableColumn[];
		rows?: unknown[];
		totalRows?: number;
		page?: number;
		pageSize?: number;
		sortKey?: string;
		sortDirection?: 'asc' | 'desc';
		selectedIds?: Set<string>;
		rowActions?: DataTableRowAction[];
		bulkActions?: DataTableBulkAction[];
		loading?: boolean;
		emptyMessage?: string;
		emptyIcon?: any;
		onRowClick?: (row: unknown) => void;
		getRowId?: (row: unknown) => string;
		selectable?: boolean;
		pageSizeOptions?: number[];
	} = $props();

	let tableRef = $state<HTMLDivElement | null>(null);

	let totalPages = $derived(totalRows > 0 ? Math.ceil(totalRows / pageSize) : 1);

	// Client-side sort: when sortKey is set, sort a copy of the rows by that
	// column. When unset, preserve the original order.
	let sortedRows = $derived.by(() => {
		if (!sortKey) return rows;
		const key = sortKey;
		const dir = sortDirection === 'desc' ? -1 : 1;
		return [...rows].sort((a, b) => {
			let aVal = getNestedValue(a, key);
			let bVal = getNestedValue(b, key);
			if (Array.isArray(aVal)) aVal = aVal.join(', ');
			if (Array.isArray(bVal)) bVal = bVal.join(', ');
			if (aVal == null) aVal = '';
			if (bVal == null) bVal = '';
			const compare = typeof aVal === 'string'
				? String(aVal).localeCompare(String(bVal))
				: (aVal as number) - (bVal as number);
			return compare * dir;
		});
	});

	// Client-side pagination: slice the sorted rows to the current page.
	// When the parent passes fewer rows than totalRows, it's doing
	// server-side pagination — render rows as-is (don't double-slice).
	let paginatedRows = $derived(
		totalRows > rows.length
			? sortedRows
			: sortedRows.slice(page * pageSize, (page + 1) * pageSize)
	);

	// Reset to first page when the page size changes or data shrinks.
	$effect(() => {
		void pageSize;
		void totalRows;
		if (page > 0 && page >= totalPages) page = 0;
	});

	let visibleIds = $derived(paginatedRows.map((r) => getRowId(r as Record<string, unknown>)));

	let allSelected = $derived(
		visibleIds.length > 0 && visibleIds.every((id) => selectedIds.has(id))
	);

	let someSelected = $derived(
		visibleIds.some((id) => selectedIds.has(id)) && !allSelected
	);

	function toggleSort(key: string) {
		if (sortKey === key) {
			if (sortDirection === 'asc') {
				sortDirection = 'desc';
			} else {
				sortKey = '';
				sortDirection = 'asc';
			}
		} else {
			sortKey = key;
			sortDirection = 'asc';
		}
	}

	function toggleAll() {
		const next = new Set(selectedIds);
		if (allSelected) {
			for (const id of visibleIds) next.delete(id);
		} else {
			for (const id of visibleIds) next.add(id);
		}
		selectedIds = next;
	}

	function toggleRow(id: string) {
		const next = new Set(selectedIds);
		if (next.has(id)) {
			next.delete(id);
		} else {
			next.add(id);
		}
		selectedIds = next;
	}

	function goTo(p: number) {
		page = Math.max(0, Math.min(p, totalPages - 1));
		if (tableRef) {
			tableRef.scrollIntoView({ behavior: 'smooth', block: 'start' });
		}
	}

	function getNestedValue(row: unknown, key: string): unknown {
		const parts = key.split('.');
		let val: unknown = row;
		for (const part of parts) {
			if (val === null || val === undefined) return undefined;
			val = (val as Record<string, unknown>)[part];
		}
		return val;
	}

	function clearSelection() {
		selectedIds = new Set();
	}

	function handleBulkAction(action: DataTableBulkAction) {
		// Act on the full selection set, not just the ids visible on this
		// page — a user who selects rows across pages expects the action to
		// cover every checked row (previously off-page selections were
		// silently dropped).
		action.onClick([...selectedIds]);
		clearSelection();
	}
</script>

<div bind:this={tableRef} class="border-border-card bg-background-alt shadow-card overflow-hidden rounded-xl">
	<table class="w-full text-[14px]">
		<thead>
			<tr class="border-border-card border-b">
				{#if selectable}
					<th scope="col" class="text-muted-foreground bg-muted w-10 px-3 py-2.5">
						<label class="flex cursor-pointer items-center justify-center">
							<input
								type="checkbox"
								checked={allSelected}
								indeterminate={someSelected}
								onchange={toggleAll}
								aria-label={t('lib.components.DataTable.select_all', 'Select all rows')}
								class="border-border-input bg-background size-3.5 cursor-pointer rounded-sm accent-accent-foreground"
							/>
						</label>
					</th>
				{/if}
				{#each columns as col}
					<th
						scope="col"
						class="text-muted-foreground bg-muted px-4 py-2.5 text-left font-medium {col.class ?? ''}"
					>
						{#if col.sortable}
							<button
								type="button"
								class="flex items-center gap-1 transition-colors hover:text-foreground"
								onclick={() => toggleSort(col.key)}
							>
								{col.label}
								<span class="inline-flex">
									{#if sortKey === col.key}
										{#if sortDirection === 'asc'}
											<ChevronUp class="size-3.5" />
										{:else}
											<ChevronDown class="size-3.5" />
										{/if}
									{:else}
										<ChevronsUpDown class="size-3.5 opacity-40" />
									{/if}
								</span>
							</button>
						{:else}
							{col.label}
						{/if}
					</th>
				{/each}
				{#if rowActions.length > 0}
					<th class="text-muted-foreground bg-muted w-10 px-3 py-2.5"></th>
				{/if}
			</tr>
		</thead>
		<tbody>
			{#if loading}
				{#each Array(pageSize) as _, i}
					<tr class="border-border-card border-t">
						{#if selectable}
							<td class="px-3 py-2.5">
								<div class="bg-muted h-3.5 w-3.5 animate-pulse rounded-sm"></div>
							</td>
						{/if}
						{#each columns as col}
							<td class="px-4 py-2.5">
								<div class="bg-muted h-3.5 animate-pulse rounded" style="width: {40 + ((i * 37) % 40)}%"></div>
							</td>
						{/each}
						{#if rowActions.length > 0}
							<td class="px-3 py-2.5">
								<div class="bg-muted mx-auto h-3.5 w-3.5 animate-pulse rounded"></div>
							</td>
						{/if}
					</tr>
				{/each}
			{:else if rows.length === 0}
				<tr>
					<td
						colspan={columns.length + (selectable ? 1 : 0) + (rowActions.length > 0 ? 1 : 0)}
						class="px-4 py-12 text-center"
					>
						<div class="flex flex-col items-center gap-2">
							{#if emptyIcon}
								{#if typeof emptyIcon === 'string'}
									<span class="text-muted-foreground size-8 opacity-30">{emptyIcon}</span>
								{:else}
									<emptyIcon class="text-muted-foreground size-8 opacity-30"></emptyIcon>
								{/if}
							{:else}
								<Inbox class="text-muted-foreground size-8 opacity-30" />
							{/if}
							<p class="text-muted-foreground text-sm">{emptyMessage}</p>
						</div>
					</td>
				</tr>
			{:else}
				{#each paginatedRows as row, i (getRowId(row))}
					<tr
						class="border-border-card border-t transition-colors hover:bg-muted/30 {onRowClick ? 'cursor-pointer' : ''}"
						onclick={onRowClick ? () => onRowClick!(row) : undefined}
						role={onRowClick ? 'button' : undefined}
						tabindex={onRowClick ? 0 : undefined}
						onkeydown={onRowClick
							? (e) => {
									if (e.key === 'Enter' || e.key === ' ') {
										e.preventDefault();
										onRowClick!(row);
									}
								}
							: undefined}
					>
						{#if selectable}
							<td class="px-3 py-2.5">
								<input
									type="checkbox"
									checked={selectedIds.has(getRowId(row))}
									onchange={() => toggleRow(getRowId(row))}
									onclick={(e) => e.stopPropagation()}
									class="border-border-input bg-background size-3.5 cursor-pointer rounded-sm accent-accent-foreground"
								/>
							</td>
						{/if}
						{#each columns as col}
							<td
								class="px-4 py-2.5 {col.class ?? ''}"
							>
								{#if col.render}
									{@html sanitizeHtml(col.render(getNestedValue(row, col.key), row as Record<string, unknown>))}
								{:else}
									{getNestedValue(row, col.key) ?? '—'}
								{/if}
							</td>
						{/each}
						{#if rowActions.length > 0}
							<td class="px-3 py-2.5">
								<div role="presentation" onclick={(e) => e.stopPropagation()}>
									<Menu sideOffset={4} contentClass="min-w-[160px]">
										{#snippet trigger(props)}
											<button
												type="button"
												{...props}
												class="text-muted-foreground hover:bg-dark-04 flex h-7 w-7 items-center justify-center rounded-lg transition-colors outline-none"
												aria-label="Row actions"
											>
												<Ellipsis class="size-4" />
											</button>
										{/snippet}
										{#each rowActions.filter(a => !a.hidden?.(row as Record<string, unknown>)) as action}
											<MenuItem
												destructive={action.variant === 'destructive'}
												disabled={action.disabled?.(row as Record<string, unknown>)}
												icon={action.icon}
												onSelect={async () => {
													await tick();
													action.onClick(row as Record<string, unknown>);
												}}
											>
												{action.label}
											</MenuItem>
										{/each}
									</Menu>
								</div>
							</td>
						{/if}
					</tr>
				{/each}
			{/if}
		</tbody>
	</table>

	{#if totalPages > 1 || rows.length > 0}
		<div class="border-border-card flex flex-wrap items-center justify-between gap-3 border-t px-4 py-2.5">
			<div class="text-muted-foreground flex items-center gap-3 text-sm">
				<span>{totalRows} {totalRows !== 1 ? t('lib.components.DataTable.items', 'items') : t('lib.components.DataTable.item', 'item')}</span>
				<div class="flex items-center gap-1.5">
					<span class="text-xs">{t('lib.components.DataTable.per_page', 'Per page')}</span>
					<AutocompleteSelect
						value={String(pageSize)}
						onValueChange={(v) => {
							pageSize = Number(v);
							page = 0;
						}}
						options={pageSizeOptions.map((opt) => ({ value: String(opt), label: String(opt) }))}
						ariaLabel={t('lib.components.DataTable.per_page', 'Per page')}
						compact
					/>
				</div>
			</div>

			<div class="flex items-center gap-1">
				<button
					class="text-foreground hover:bg-dark-04 flex h-7 w-7 items-center justify-center rounded-lg transition-colors disabled:opacity-30"
					disabled={page === 0}
					onclick={() => goTo(0)}
					aria-label={t('lib.components.DataTable.first_page', 'First page')}
				>
					<ChevronsLeft class="size-4" />
				</button>
				<button
					class="text-foreground hover:bg-dark-04 flex h-7 w-7 items-center justify-center rounded-lg transition-colors disabled:opacity-30"
					disabled={page === 0}
					onclick={() => goTo(page - 1)}
					aria-label={t('lib.components.DataTable.prev_page', 'Previous page')}
				>
					<ChevronLeft class="size-4" />
				</button>
				<span class="text-foreground px-2 text-xs font-medium">
					{t('lib.components.DataTable.page_of', 'Page {p} of {t}').replace('{p}', String(page + 1)).replace('{t}', String(totalPages))}
				</span>
				<button
					class="text-foreground hover:bg-dark-04 flex h-7 w-7 items-center justify-center rounded-lg transition-colors disabled:opacity-30"
					disabled={page >= totalPages - 1}
					onclick={() => goTo(page + 1)}
					aria-label={t('lib.components.DataTable.next_page', 'Next page')}
				>
					<ChevronRight class="size-4" />
				</button>
				<button
					class="text-foreground hover:bg-dark-04 flex h-7 w-7 items-center justify-center rounded-lg transition-colors disabled:opacity-30"
					disabled={page >= totalPages - 1}
					onclick={() => goTo(totalPages - 1)}
					aria-label={t('lib.components.DataTable.last_page', 'Last page')}
				>
					<ChevronsRight class="size-4" />
				</button>
			</div>
		</div>
	{/if}
</div>

{#if selectedIds.size > 0}
	<div
		class="border-border-card bg-background-alt shadow-card mt-3 flex items-center justify-between rounded-xl border px-4 py-3"
	>
		<div class="text-foreground flex items-center gap-3 text-sm">
			<span class="font-medium">{t('lib.components.DataTable.selected_count', '{n} selected').replace('{n}', String(selectedIds.size))}</span>
			<button
				type="button"
				class="text-muted-foreground hover:text-foreground text-xs underline underline-offset-2 transition-colors"
				onclick={clearSelection}
			>
				{t('lib.components.DataTable.clear_selection', 'Clear')}
			</button>
		</div>
		<div class="flex items-center gap-2">
			{#each bulkActions as action}
				<button
					type="button"
					class="flex items-center gap-1.5 rounded-lg px-3 py-1.5 text-sm font-medium transition-colors {action.variant === 'destructive'
						? 'bg-destructive/10 text-destructive hover:bg-destructive/20'
						: 'bg-dark-04 text-foreground hover:bg-dark-10'}"
					onclick={() => handleBulkAction(action)}
				>
					{#if action.icon}
						<action.icon class="size-3.5" />
					{/if}
					{action.label}
				</button>
			{/each}
		</div>
	</div>
{/if}


