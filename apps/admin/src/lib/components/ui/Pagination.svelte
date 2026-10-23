<script lang="ts">
	import { t } from '$lib/stores/i18n.svelte';
	import { ChevronLeft, ChevronRight, ChevronsLeft, ChevronsRight } from 'lucide-svelte';
	import Autocomplete from '$lib/components/ui/Autocomplete.svelte';
	import { replaceState } from '$app/navigation';

	let {
		page = $bindable(0),
		totalPages,
		totalItems = 0,
		pageSize = $bindable(10),
		pageSizeOptions = [10, 25, 50, 100],
		urlSync = true,
		defaultPageSize = 10
	}: {
		page: number;
		totalPages: number;
		totalItems?: number;
		pageSize?: number;
		pageSizeOptions?: number[];
		urlSync?: boolean;
		// Page size treated as the app's default — the ?pageSize param is only
		// written when the selection differs from it (matches EntityPage's
		// pageSize omission logic so the two writers agree).
		defaultPageSize?: number;
	} = $props();

	function syncToUrl() {
		if (!urlSync || typeof window === 'undefined') return;
		// Read the live URL, because $app/state can lag behind replaceState updates
		// made by other controls (DataTable sort/search) and would clobber them.
		const params = new URLSearchParams(window.location.search);
		if (page > 0) params.set('page', String(page + 1));
		else params.delete('page');
		if (pageSize !== defaultPageSize) params.set('pageSize', String(pageSize));
		else params.delete('pageSize');
		const qs = params.toString();
		const url = `${window.location.pathname}${qs ? `?${qs}` : ''}`;
		try {
			replaceState(url, {});
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
			<!-- Page-size is a dropdown field — rendered through the shared
			     Autocomplete like every other dropdown in the app. -->
			<div class="w-20">
				<Autocomplete
					placeholder={String(pageSize)}
					emptyMessage={t('common.no_results', 'No results found')}
					inputClass="!h-7 !text-xs"
					fetcher={async (q: string) =>
						pageSizeOptions
							.filter((o) => String(o).includes(q.trim()))
							.map((o) => ({ id: String(o), label: String(o) }))}
					initialFetcher={async () => pageSizeOptions.map((o) => ({ id: String(o), label: String(o) }))}
					selectedId={String(pageSize)}
					selectedLabel={String(pageSize)}
					onSelect={(item) => handlePageSizeChange(item.id)}
					onClear={() => handlePageSizeChange(String(defaultPageSize))}
				/>
			</div>
		</div>
	</div>

	<div class="flex items-center gap-1">
		<button
			class="flex h-8 w-8 items-center justify-center rounded-md transition-colors hover:bg-black/5 disabled:opacity-30 dark:hover:bg-white/10"
			disabled={page === 0}
			onclick={() => goTo(0)}
			aria-label={t('pagination.first', 'First page')}
		>
			<ChevronsLeft class="h-4 w-4 rtl:-scale-x-100" />
		</button>
		<button
			class="flex h-8 w-8 items-center justify-center rounded-md transition-colors hover:bg-black/5 disabled:opacity-30 dark:hover:bg-white/10"
			disabled={page === 0}
			onclick={() => goTo(page - 1)}
			aria-label={t('pagination.previous', 'Previous page')}
		>
			<ChevronLeft class="h-4 w-4 rtl:-scale-x-100" />
		</button>

		<span class="px-2 text-xs font-medium" style="color: var(--color-foreground)">
			{t('pagination.page_of', 'Page {current} of {total}').replace('{current}', String(page + 1)).replace('{total}', String(totalPages))}
		</span>

		<button
			class="flex h-8 w-8 items-center justify-center rounded-md transition-colors hover:bg-black/5 disabled:opacity-30 dark:hover:bg-white/10"
			disabled={page >= totalPages - 1}
			onclick={() => goTo(page + 1)}
			aria-label={t('pagination.next', 'Next page')}
		>
			<ChevronRight class="h-4 w-4 rtl:-scale-x-100" />
		</button>
		<button
			class="flex h-8 w-8 items-center justify-center rounded-md transition-colors hover:bg-black/5 disabled:opacity-30 dark:hover:bg-white/10"
			disabled={page >= totalPages - 1}
			onclick={() => goTo(totalPages - 1)}
			aria-label={t('pagination.last', 'Last page')}
		>
			<ChevronsRight class="h-4 w-4 rtl:-scale-x-100" />
		</button>
	</div>
</div>
