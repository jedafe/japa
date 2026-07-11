<script lang="ts">
	import { ChevronLeft, ChevronRight } from 'lucide-svelte';

	let {
		columns,
		rows,
		page = 0,
		totalPages = 1,
		onPageChange
	}: {
		columns: { key: string; label: string; class?: string }[];
		rows: Record<string, unknown>[];
		page?: number;
		totalPages?: number;
		onPageChange?: (page: number) => void;
	} = $props();
</script>

<div class="overflow-x-auto rounded-lg border" style="border-color: var(--color-border)">
	<table class="w-full text-sm">
		<thead>
			<tr style="border-color: var(--color-border)">
				{#each columns as col}
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">
						{col.label}
					</th>
				{/each}
			</tr>
		</thead>
		<tbody>
			{#each rows as row, i}
				<tr class="border-t" style="border-color: var(--color-border)">
					{#each columns as col}
						<td class="px-4 py-3">
							{row[col.key] ?? '—'}
						</td>
					{/each}
				</tr>
			{:else}
				<tr>
					<td colspan={columns.length} class="px-4 py-8 text-center" style="color: var(--color-muted)">
						No data available
					</td>
				</tr>
			{/each}
		</tbody>
	</table>
</div>

{#if totalPages > 1}
	<div class="mt-3 flex items-center justify-between">
		<p class="text-xs" style="color: var(--color-muted)">Page {page + 1} of {totalPages}</p>
		<div class="flex gap-1">
			<button
				class="rounded p-1 hover:bg-black/5 dark:hover:bg-white/10 disabled:opacity-40"
				disabled={page === 0}
				onclick={() => onPageChange?.(page - 1)}
			>
				<ChevronLeft class="h-4 w-4" />
			</button>
			<button
				class="rounded p-1 hover:bg-black/5 dark:hover:bg-white/10 disabled:opacity-40"
				disabled={page >= totalPages - 1}
				onclick={() => onPageChange?.(page + 1)}
			>
				<ChevronRight class="h-4 w-4" />
			</button>
		</div>
	</div>
{/if}
