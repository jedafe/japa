<script lang="ts">
	// /tools/cost-of-living — stats/monitoring page for cost of living calculator usage.
	// The API only returns a stats object (not a list), so we use a custom card layout instead of EntityPage.
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import { costOfLivingAdminApi } from '$lib/api';

	const canManage = $derived(hasPermission('col_calculator.manage'));

	// ── State ───────────────────────────────────────────────────────────────
	let pageError = $state('');
	let loading = $state(true);
	let stats = $state<Record<string, any>>({});

	// Load stats on mount.
	$effect(() => {
		void loadStats();
	});

	async function loadStats() {
		loading = true;
		pageError = '';
		try {
			const result = await costOfLivingAdminApi.stats();
			if (result.ok) {
				stats = (result.data as Record<string, any>) ?? {};
			} else {
				pageError = result.error ?? t('common.error', 'An error occurred');
			}
		} catch {
			pageError = t('common.error', 'An error occurred');
		} finally {
			loading = false;
		}
	}

	// Stat card entries derived from the stats object.
	const statCards = $derived.by<Array<{ key: string; label: string; value: string }>>(() => {
		const entries: Array<{ key: string; label: string; value: string }> = [];
		const labels: Record<string, string> = {
			total_calculations: t('app.tools.col_total_calculations', 'Total Calculations'),
			unique_users: t('app.tools.col_unique_users', 'Unique Users'),
			most_compared_cities: t('app.tools.col_most_compared_cities', 'Most Compared Cities'),
			avg_calculations_per_user: t('app.tools.col_avg_per_user', 'Avg / User'),
			last_calculation_at: t('app.tools.col_last_calculation', 'Last Calculation')
		};
		for (const [key, value] of Object.entries(stats)) {
			entries.push({
				key,
				label: labels[key] ?? key.replace(/_/g, ' ').replace(/\b\w/g, (c) => c.toUpperCase()),
				value: formatStatValue(key, value)
			});
		}
		return entries;
	});

	function formatStatValue(key: string, value: any): string {
		if (value == null) return '—';
		if (key === 'last_calculation_at' && typeof value === 'string') {
			return new Date(value).toLocaleString();
		}
		if (typeof value === 'number') return value.toLocaleString();
		if (Array.isArray(value)) return value.join(', ');
		return String(value);
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
			{t('app.tools.cost_of_living_desc', 'Usage statistics for the cost of living comparison calculator')}
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
	{:else if statCards.length === 0}
		<div class="rounded-lg border p-8 text-center text-sm" style="border-color: var(--color-border); color: var(--color-muted)">
			{t('common.no_data', 'No data available')}
		</div>
	{:else}
		<div class="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
			{#each statCards as card (card.key)}
				<div
					class="rounded-lg border p-5"
					style="border-color: var(--color-border); background: var(--color-surface)"
				>
					<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
						{card.label}
					</p>
					<p class="mt-2 text-2xl font-bold" style="color: var(--color-foreground)">
						{card.value}
					</p>
				</div>
			{/each}
		</div>
	{/if}
</div>
