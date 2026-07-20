<script lang="ts">
	import { t } from '$lib/stores/i18n.svelte';
	import { Select } from 'bits-ui';
	import { ChevronLeft, ChevronRight, ChevronsLeft, ChevronsRight } from 'lucide-svelte';

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
</script>

<div class="border-border-card flex flex-wrap items-center justify-between gap-3 border-t px-4 py-3">
	<div class="text-muted-foreground flex items-center gap-3 text-sm">
		<span>{totalItems} item{totalItems !== 1 ? 's' : ''}</span>

		<div class="flex items-center gap-1.5">
			<span class="text-xs">{t('lib.components.ui.Pagination.svelte.per_page')}</span>
			<Select.Root
				type="single"
				value={String(pageSize)}
				onValueChange={(v) => {
					if (v !== undefined) {
						pageSize = Number(v);
						page = 0;
					}
				}}
			>
				<Select.Trigger
					class="border-border-input bg-background text-foreground h-7 rounded-lg border px-2 text-xs focus:outline-none focus:ring-2 focus:ring-foreground/20"
				>
					<Select.Value />
				</Select.Trigger>
				<Select.Portal>
					<Select.Content class="border-border-input bg-background shadow-popover z-50 rounded-lg border">
						<Select.Viewport class="p-1">
							{#each pageSizeOptions as opt}
								<Select.Item
									class="data-highlighted:bg-accent rounded-lg px-3 py-1.5 text-sm outline-none"
									value={String(opt)}
									label={String(opt)}
								>
									{opt}
								</Select.Item>
							{/each}
						</Select.Viewport>
					</Select.Content>
				</Select.Portal>
			</Select.Root>
		</div>
	</div>

	<div class="flex items-center gap-1">
		<button
			class="text-foreground hover:bg-dark-04 flex h-8 w-8 items-center justify-center rounded-lg transition-colors disabled:opacity-30"
			disabled={page === 0}
			onclick={() => goTo(0)}
			aria-label="First page"
		>
			<ChevronsLeft class="size-4" />
		</button>
		<button
			class="text-foreground hover:bg-dark-04 flex h-8 w-8 items-center justify-center rounded-lg transition-colors disabled:opacity-30"
			disabled={page === 0}
			onclick={() => goTo(page - 1)}
			aria-label="Previous page"
		>
			<ChevronLeft class="size-4" />
		</button>

		<span class="text-foreground px-2 text-xs font-medium">
			Page {page + 1} of {totalPages}
		</span>

		<button
			class="text-foreground hover:bg-dark-04 flex h-8 w-8 items-center justify-center rounded-lg transition-colors disabled:opacity-30"
			disabled={page >= totalPages - 1}
			onclick={() => goTo(page + 1)}
			aria-label="Next page"
		>
			<ChevronRight class="size-4" />
		</button>
		<button
			class="text-foreground hover:bg-dark-04 flex h-8 w-8 items-center justify-center rounded-lg transition-colors disabled:opacity-30"
			disabled={page >= totalPages - 1}
			onclick={() => goTo(totalPages - 1)}
			aria-label="Last page"
		>
			<ChevronsRight class="size-4" />
		</button>
	</div>
</div>
