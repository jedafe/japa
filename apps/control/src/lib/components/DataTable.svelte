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

<div class="border-border-card bg-background-alt shadow-card overflow-hidden rounded-xl">
	<table class="w-full text-sm">
		<thead>
			<tr class="border-border-card border-b">
				{#each columns as col}
					<th class="text-muted-foreground bg-muted px-4 py-3 text-left font-medium">
						{col.label}
					</th>
				{/each}
			</tr>
		</thead>
		<tbody>
			{#each rows as row, i}
				<tr class="border-border-card border-t">
					{#each columns as col}
						<td class="px-4 py-3">
							{row[col.key] ?? '—'}
						</td>
					{/each}
				</tr>
			{:else}
				<tr>
					<td colspan={columns.length} class="text-muted-foreground px-4 py-8 text-center">
						No data available
					</td>
				</tr>
			{/each}
		</tbody>
	</table>
</div>

{#if totalPages > 1}
	<div class="mt-3 flex items-center justify-between">
		<p class="text-muted-foreground text-xs">Page {page + 1} of {totalPages}</p>
		<div class="flex gap-1">
			<button
				class="border-border-input bg-background-alt hover:bg-muted text-foreground rounded p-1 transition-colors disabled:opacity-40"
				disabled={page === 0}
				onclick={() => onPageChange?.(page - 1)}
			>
				<ChevronLeft class="h-4 w-4" />
			</button>
			<button
				class="border-border-input bg-background-alt hover:bg-muted text-foreground rounded p-1 transition-colors disabled:opacity-40"
				disabled={page >= totalPages - 1}
				onclick={() => onPageChange?.(page + 1)}
			>
				<ChevronRight class="h-4 w-4" />
			</button>
		</div>
	</div>
{/if}
