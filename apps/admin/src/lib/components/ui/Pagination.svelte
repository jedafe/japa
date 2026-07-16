<script lang="ts">
	import { ChevronLeft, ChevronRight, ChevronsLeft, ChevronsRight } from 'lucide-svelte';
	import { Select } from '$lib/components/ui/select';

	let {
		page = $bindable(0),
		totalPages,
		totalItems = 0,
		pageSize = $bindable(10),
		pageSizeOptions = [10, 25, 50, 100]
	}: {
		page: number;
		totalPages: number;
		totalItems?: number;
		pageSize?: number;
		pageSizeOptions?: number[];
	} = $props();

	function goTo(p: number) {
		page = Math.max(0, Math.min(p, totalPages - 1));
	}

	function handlePageSizeChange(val: string) {
		pageSize = Number(val);
		page = 0;
	}
</script>

<div class="flex flex-wrap items-center justify-between gap-3 border-t px-4 py-3" style="border-color: var(--color-border)">
	<div class="flex items-center gap-3 text-sm" style="color: var(--color-muted)">
		<span>{totalItems} item{totalItems !== 1 ? 's' : ''}</span>

		<div class="flex items-center gap-1.5">
			<span class="text-xs">Per page:</span>
			<Select
				options={pageSizeOptions.map(o => ({ value: String(o), label: String(o) }))}
				value={String(pageSize)}
				onchange={handlePageSizeChange}
				class="!h-7 !text-xs !w-20"
			/>
		</div>
	</div>

	<div class="flex items-center gap-1">
		<button
			class="flex h-8 w-8 items-center justify-center rounded-md transition-colors hover:bg-black/5 disabled:opacity-30 dark:hover:bg-white/10"
			disabled={page === 0}
			onclick={() => goTo(0)}
			aria-label="First page"
		>
			<ChevronsLeft class="h-4 w-4" />
		</button>
		<button
			class="flex h-8 w-8 items-center justify-center rounded-md transition-colors hover:bg-black/5 disabled:opacity-30 dark:hover:bg-white/10"
			disabled={page === 0}
			onclick={() => goTo(page - 1)}
			aria-label="Previous page"
		>
			<ChevronLeft class="h-4 w-4" />
		</button>

		<span class="px-2 text-xs font-medium" style="color: var(--color-foreground)">
			Page {page + 1} of {totalPages}
		</span>

		<button
			class="flex h-8 w-8 items-center justify-center rounded-md transition-colors hover:bg-black/5 disabled:opacity-30 dark:hover:bg-white/10"
			disabled={page >= totalPages - 1}
			onclick={() => goTo(page + 1)}
			aria-label="Next page"
		>
			<ChevronRight class="h-4 w-4" />
		</button>
		<button
			class="flex h-8 w-8 items-center justify-center rounded-md transition-colors hover:bg-black/5 disabled:opacity-30 dark:hover:bg-white/10"
			disabled={page >= totalPages - 1}
			onclick={() => goTo(totalPages - 1)}
			aria-label="Last page"
		>
			<ChevronsRight class="h-4 w-4" />
		</button>
	</div>
</div>
