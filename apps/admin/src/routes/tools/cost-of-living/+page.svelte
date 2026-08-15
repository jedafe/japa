<script lang="ts">
	// /tools/cost-of-living — data completeness + staleness report for the Cost
	// of Living Calculator (Section 72, ADR-072). The backend returns a paginated
	// per-city report (AdminStatsResponse), not usage analytics.
	import { t } from '$lib/stores/i18n.svelte';
	import { costOfLivingAdminApi, type ColAdminStatsItem } from '$lib/api';

	// ── State ───────────────────────────────────────────────────────────────
	let pageError = $state('');
	let loading = $state(true);
	let cities = $state<ColAdminStatsItem[]>([]);
	let total = $state(0);
	let page = $state(1);
	let limit = $state(25);

	// Load stats on mount.
	$effect(() => {
		void loadStats();
	});

	async function loadStats() {
		loading = true;
		pageError = '';
		try {
			const result = await costOfLivingAdminApi.stats(`page=${page}&limit=${limit}`);
			if (result.ok) {
				cities = result.data.cities ?? [];
				total = result.data.total ?? 0;
			} else {
				pageError = result.error ?? t('common.error', 'An error occurred');
			}
		} catch {
			pageError = t('common.error', 'An error occurred');
		} finally {
			loading = false;
		}
	}

	const totalPages = $derived(Math.max(1, Math.ceil(total / limit)));

	function goTo(p: number) {
		if (p < 1 || p > totalPages || p === page) return;
		page = p;
		void loadStats();
	}

	function formatDate(s: string): string {
		if (!s) return '—';
		try {
			return new Date(s).toLocaleDateString();
		} catch {
			return s;
		}
	}

	function completenessColor(pct: number): string {
		if (pct >= 80) return 'var(--color-success, #16a34a)';
		if (pct >= 50) return 'var(--color-warning, #d97706)';
		return 'var(--color-danger, #dc2626)';
	}
</script>

<svelte:head>
	<title>{t('app.tools.cost_of_living_page_title', 'Cost of Living Stats — JapaConnect Admin')}</title>
</svelte:head>

<div class="space-y-6">
	<div>
		<h1 class="text-2xl font-bold" style="color: var(--color-foreground)">
			{t('app.tools.cost_of_living_heading', 'Cost of Living Calculator')}
		</h1>
		<p class="mt-1 text-sm" style="color: var(--color-muted)">
			{t('app.tools.cost_of_living_desc', 'Data completeness and staleness report per city')}
		</p>
	</div>

	{#if pageError}
		<div
			class="rounded-lg p-3 text-sm"
			style="background: color-mix(in srgb, var(--color-danger) 10%, transparent); color: var(--color-danger)"
		>
			{pageError}
		</div>
	{/if}

	{#if loading}
		<div class="rounded-lg border p-8 text-center text-sm" style="border-color: var(--color-border); color: var(--color-muted)">
			{t('common.loading', 'Loading...')}
		</div>
	{:else if cities.length === 0}
		<div class="rounded-lg border p-8 text-center text-sm" style="border-color: var(--color-border); color: var(--color-muted)">
			{t('common.no_data', 'No data available')}
		</div>
	{:else}
		<div class="overflow-x-auto rounded-lg border" style="border-color: var(--color-border)">
			<table class="w-full text-sm">
				<thead>
					<tr class="border-b text-left text-xs uppercase tracking-wider" style="border-color: var(--color-border); color: var(--color-muted)">
						<th class="px-4 py-3 font-medium">{t('app.tools.col_city', 'City')}</th>
						<th class="px-4 py-3 font-medium">{t('app.tools.col_completeness', 'Completeness')}</th>
						<th class="px-4 py-3 font-medium">{t('app.tools.col_filled', 'Filled / Total')}</th>
						<th class="px-4 py-3 font-medium">{t('app.tools.col_last_updated', 'Last Updated')}</th>
						<th class="px-4 py-3 font-medium">{t('app.tools.col_status', 'Status')}</th>
					</tr>
				</thead>
				<tbody>
					{#each cities as city (city.city_id)}
						<tr class="border-b last:border-0" style="border-color: var(--color-border)">
							<td class="px-4 py-3" style="color: var(--color-foreground)">
								{city.city_name}
								{#if city.is_baseline}
									<span class="ml-2 text-xs px-1.5 py-0.5 rounded" style="background: var(--color-surface-subtle, #f1f5f9); color: var(--color-muted)">{t('col.baseline_label', 'Baseline')}</span>
								{/if}
							</td>
							<td class="px-4 py-3">
								<div class="flex items-center gap-2">
									<div class="flex-1 max-w-[120px] h-2 rounded-full overflow-hidden" style="background: var(--color-surface-subtle, #e2e8f0)">
										<div class="h-full rounded-full" style="width: {city.completeness_pct}%; background: {completenessColor(city.completeness_pct)}"></div>
									</div>
									<span class="text-xs font-medium" style="color: var(--color-foreground)">{city.completeness_pct.toFixed(0)}%</span>
								</div>
							</td>
							<td class="px-4 py-3" style="color: var(--color-foreground)">
								{city.filled_dimensions} / {city.total_dimensions}
							</td>
							<td class="px-4 py-3" style="color: var(--color-muted)">
								{formatDate(city.last_updated)}
							</td>
							<td class="px-4 py-3">
								{#if city.is_stale}
									<span class="text-xs px-2 py-1 rounded-full" style="background: color-mix(in srgb, var(--color-warning, #d97706) 15%, transparent); color: var(--color-warning, #d97706)">
										{t('app.tools.col_stale', 'Stale')}
									</span>
								{:else}
									<span class="text-xs px-2 py-1 rounded-full" style="background: color-mix(in srgb, var(--color-success, #16a34a) 15%, transparent); color: var(--color-success, #16a34a)">
										{t('app.tools.col_fresh', 'Fresh')}
									</span>
								{/if}
							</td>
						</tr>
					{/each}
				</tbody>
			</table>
		</div>

		<!-- Pagination -->
		{#if totalPages > 1}
			<div class="flex items-center justify-between">
				<div class="flex items-center gap-3">
					<span class="text-sm" style="color: var(--color-muted)">
						{t('common.page', 'Page')} {page} / {totalPages} · {total} {t('common.total', 'total')}
					</span>
					<select
						value={limit}
						onchange={(e) => { limit = Number((e.currentTarget as HTMLSelectElement).value); page = 1; void loadStats(); }}
						class="rounded border px-2 py-1 text-sm"
						style="border-color: var(--color-border); color: var(--color-foreground); background: var(--color-surface)"
						aria-label={t('common.pageSize', 'Page size')}
					>
						<option value={10}>10</option>
						<option value={25}>25</option>
						<option value={50}>50</option>
						<option value={100}>100</option>
					</select>
				</div>
				<div class="flex gap-2">
					<button
						type="button"
						onclick={() => goTo(page - 1)}
						disabled={page <= 1}
						class="px-3 py-1.5 text-sm rounded border disabled:opacity-40"
						style="border-color: var(--color-border); color: var(--color-foreground)"
					>
						{t('common.prev', 'Previous')}
					</button>
					<button
						type="button"
						onclick={() => goTo(page + 1)}
						disabled={page >= totalPages}
						class="px-3 py-1.5 text-sm rounded border disabled:opacity-40"
						style="border-color: var(--color-border); color: var(--color-foreground)"
					>
						{t('common.next', 'Next')}
					</button>
				</div>
			</div>
		{/if}
	{/if}
</div>
