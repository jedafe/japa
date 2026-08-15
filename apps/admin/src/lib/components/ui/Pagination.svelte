<script lang="ts">
	import { t } from '$lib/stores/i18n.svelte';
	import { ChevronLeft, ChevronRight, ChevronsLeft, ChevronsRight } from 'lucide-svelte';
	import { Select } from '$lib/components/ui/select';
	import { page as sveltekitPage } from '$app/state';

	let {
		page = $bindable(0),
		totalPages,
		totalItems = 0,
		pageSize = $bindable(10),
		pageSizeOptions = [10, 25, 50, 100],
		urlSync = true
	}: {
		page: number;
		totalPages: number;
		totalItems?: number;
		pageSize?: number;
		pageSizeOptions?: number[];
		urlSync?: boolean;
	} = $props();

	function syncToUrl() {
		if (!urlSync || typeof window === 'undefined') return;
		const params = new URLSearchParams(sveltekitPage.url.searchParams.toString());
		if (page > 0) params.set('page', String(page + 1));
		else params.delete('page');
		if (pageSize !== 10) params.set('pageSize', String(pageSize));
		else params.delete('pageSize');
		const qs = params.toString();
		const url = `${sveltekitPage.url.pathname}${qs ? `?${qs}` : ''}`;
		try {
			history.replaceState({}, '', url);
		} catch {
			// Fallback for SSR or edge cases
		}
	}

	function goTo(p: number) {
		page = Math.max(0, Math.min(p, totalPages - 1));
		syncToUrl();
	}

	function handlePageSizeChange(val: string) {
		pageSize = Number(val);
		page = 0;
		syncToUrl();
	}
</script>

<div class="flex flex-wrap items-center justify-between gap-3 border-t px-4 py-3" style="border-color: var(--color-border)">
	<div class="flex items-center gap-3 text-sm" style="color: var(--color-muted)">
		<span>{t('pagination.items', '{n} items').replace('{n}', String(totalItems))}</span>

		<div class="flex items-center gap-1.5">
			<span class="text-xs">{t('lib.components.ui.Pagination.svelte.per_page')}</span>
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
			{t('pagination.page_of', 'Page {current} of {total}').replace('{current}', String(page + 1)).replace('{total}', String(totalPages))}
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
