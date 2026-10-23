<script lang="ts">
	// /tools/cost-of-living — data completeness + staleness report for the Cost
	// of Living Calculator (Section 72, ADR-072). The backend returns a paginated
	// per-city report (AdminStatsResponse) rendered through the shared DataTable.
	import { t } from '$lib/stores/i18n.svelte';
	import { DataTable } from '$lib/components/ui/data-table';
	import { costOfLivingAdminApi, type ColAdminStatsItem } from '$lib/api';
	import type { Column } from '$lib/components/ui/data-table/types';

	// ── State ───────────────────────────────────────────────────────────────
	let pageError = $state('');
	let loading = $state(true);
	let cities = $state<ColAdminStatsItem[]>([]);
	let total = $state(0);
	let page = $state(0);
	let pageSize = $state(25);

	const totalPages = $derived(Math.max(1, Math.ceil(total / pageSize)));

	async function loadStats() {
		loading = true;
		pageError = '';
		try {
			const result = await costOfLivingAdminApi.stats(`page=${page + 1}&limit=${pageSize}`);
			if (result.ok) {
				const data = result.data as { cities?: ColAdminStatsItem[]; total?: number };
				cities = data.cities ?? [];
				total = data.total ?? 0;
			} else {
				pageError = result.error ?? t('common.error', 'An error occurred');
			}
		} catch {
			pageError = t('common.error', 'An error occurred');
		} finally {
			loading = false;
		}
	}

	$effect(() => {
		void loadStats();
	});

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

	const columns = $derived<Column[]>([
		{
			key: 'city_name',
			label: t('app.tools.col_city', 'City'),
			sortable: true,
			render: (value: string, row: ColAdminStatsItem) => {
				let html = `<span class="text-sm" style="color: var(--color-foreground)">${value}</span>`;
				if (row.is_baseline) {
					html += ` <span class="ms-2 text-xs px-1.5 py-0.5 rounded" style="background: var(--color-surface-subtle, #f1f5f9); color: var(--color-muted)">${t('col.baseline_label', 'Baseline')}</span>`;
				}
				return html;
			}
		},
		{
			key: 'completeness_pct',
			label: t('app.tools.col_completeness', 'Completeness'),
			sortable: true,
			render: (value: number) =>
				`<div class="flex items-center gap-2">` +
				`<div class="flex-1 max-w-[120px] h-2 rounded-full overflow-hidden" style="background: var(--color-surface-subtle, #e2e8f0)">` +
				`<div class="h-full rounded-full" style="width: ${value}%; background: ${completenessColor(value)}"></div>` +
				`</div>` +
				`<span class="text-xs font-medium" style="color: var(--color-foreground)">${value.toFixed(0)}%</span>` +
				`</div>`
		},
		{
			key: 'filled_dimensions',
			label: t('app.tools.col_filled', 'Filled / Total'),
			sortable: true,
			render: (_: number, row: ColAdminStatsItem) =>
				`<span class="text-sm" style="color: var(--color-foreground)">${row.filled_dimensions} / ${row.total_dimensions}</span>`
		},
		{
			key: 'last_updated',
			label: t('app.tools.col_last_updated', 'Last Updated'),
			sortable: true,
			render: (value: string) =>
				`<span class="text-xs" style="color: var(--color-muted)">${formatDate(value)}</span>`
		},
		{
			key: 'is_stale',
			label: t('app.tools.col_status', 'Status'),
			sortable: true,
			render: (value: boolean) => value
				? `<span class="text-xs px-2 py-1 rounded-full" style="background: color-mix(in srgb, var(--color-warning, #d97706) 15%, transparent); color: var(--color-warning, #d97706)">${t('app.tools.col_stale', 'Stale')}</span>`
				: `<span class="text-xs px-2 py-1 rounded-full" style="background: color-mix(in srgb, var(--color-success, #16a34a) 15%, transparent); color: var(--color-success, #16a34a)">${t('app.tools.col_fresh', 'Fresh')}</span>`
		}
	]);
</script>

<svelte:head>
	<title>{t('app.tools.cost_of_living_page_title', 'Cost of Living Stats — {site} Admin')}</title>
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

	<DataTable
		{columns}
		data={cities}
		serverSide={true}
		totalRows={total}
		totalPages={totalPages}
		bind:page
		bind:pageSize
		pageSizeOptions={[10, 25, 50, 100]}
		loading={loading}
		emptyMessage={t('common.no_data', 'No data available')}
		onpagechange={() => void loadStats()}
		onpagesizechange={() => { page = 0; void loadStats(); }}
	/>
</div>
