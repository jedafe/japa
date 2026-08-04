<script lang="ts">
	import { t } from '$lib/i18n';
	import { page } from '$app/state';
	import { Calculator, Lock, Share2, X } from 'lucide-svelte';
	import { EmptyState, LoadingSkeleton } from '$lib/components/ui';
	import { api } from '$lib/api/client';
	import type { ColCity, ColComparisonMatrix } from '$lib/types';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	const title = $derived(t('tools.cost_of_living', 'Cost of Living Calculator — JapaConnect'));
	const description = $derived(
		t('tools.cost_of_living_desc', 'Compare living costs between cities.')
	);

	const cities = $state<ColCity[]>(data.cities);
	const disabled = $state(data.disabled);

	// Up to 3 selected city IDs (baseline always included).
	let selectedIds = $state<string[]>(data.preselected.slice(0, 3));
	let matrix = $state<ColComparisonMatrix | null>(null);
	let loading = $state(false);
	let error = $state<string | null>(null);
	let copied = $state(false);

	const baselineCity = $derived(cities.find((c) => c.is_baseline));
	const selectedCities = $derived(
		selectedIds.map((id) => cities.find((c) => c.id === id)).filter(Boolean) as ColCity[]
	);

	// Ensure baseline is always in the selection.
	$effect(() => {
		if (baselineCity && !selectedIds.includes(baselineCity.id)) {
			selectedIds = [baselineCity.id, ...selectedIds.filter((id) => id !== baselineCity.id)].slice(0, 3);
		}
	});

	async function fetchComparison() {
		if (selectedIds.length < 2) {
			matrix = null;
			return;
		}
		loading = true;
		error = null;
		const ids = selectedIds.join(',');
		const res = await api.get<ColComparisonMatrix>(`/calculator/cost-of-living/compare?city_ids=${ids}`);
		loading = false;
		if (res.error) {
			error = res.error.message;
			matrix = null;
			return;
		}
		matrix = res.data;
	}

	function toggleCity(id: string) {
		if (selectedIds.includes(id)) {
			if (baselineCity && id === baselineCity.id) return; // can't remove baseline
			selectedIds = selectedIds.filter((sid) => sid !== id);
		} else {
			if (selectedIds.length >= 3) return;
			selectedIds = [...selectedIds, id];
		}
		fetchComparison();
	}

	async function shareUrl() {
		const url = `${page.url.origin}${page.url.pathname}?city_ids=${selectedIds.join(',')}`;
		try {
			await navigator.clipboard.writeText(url);
			copied = true;
			setTimeout(() => (copied = false), 2000);
		} catch {
			// Clipboard may be blocked; ignore.
		}
	}

	// Fetch comparison on mount if preselected.
	$effect(() => {
		if (data.preselected.length >= 2 && !matrix && !loading) {
			fetchComparison();
		}
	});

	function formatRatio(r: number): string {
		return r.toFixed(2) + '×';
	}
</script>

<svelte:head>
	<title>{title}</title>
	<meta name="description" content={description} />
	<meta property="og:title" content={title} />
	<meta property="og:description" content={description} />
	<meta property="og:type" content="website" />
	<meta property="og:url" content={page.url.href} />
</svelte:head>

<div class="mx-auto w-full max-w-4xl">
	<div class="border-b border-[var(--color-border)] px-4 py-3">
		<h1 class="flex items-center gap-2 text-lg font-semibold text-[var(--color-text)]">
			<Calculator size={20} /> {t('tools.cost_of_living', 'Cost of Living Calculator')}
		</h1>
		<p class="mt-1 text-sm text-[var(--color-muted)]">{description}</p>
	</div>

	{#if disabled}
		<div class="p-4">
			<EmptyState
				icon={Lock}
				title={t('tools.disabled_title', 'This tool is currently unavailable')}
				body={t('tools.disabled_body', 'Check back soon or contact support.')}
			/>
		</div>
	{:else}
		<!-- City selector -->
		<div class="border-b border-[var(--color-border)] px-4 py-3">
			<div class="mb-2 flex items-center justify-between">
				<span class="text-xs font-medium uppercase tracking-wide text-[var(--color-muted)]">
					{t('col.select_cities', 'Select up to 3 cities')}
				</span>
				{#if selectedIds.length >= 2}
					<button
						type="button"
						onclick={shareUrl}
						class="flex items-center gap-1 text-xs text-[var(--color-primary)] hover:underline"
					>
						<Share2 size={12} />
						{copied ? t('common.copied', 'Copied!') : t('common.share', 'Share')}
					</button>
				{/if}
			</div>

			<!-- Selected chips -->
			{#if selectedCities.length > 0}
				<div class="mb-2 flex flex-wrap gap-2">
					{#each selectedCities as city (city.id)}
						<span
							class="inline-flex items-center gap-1.5 rounded-full border border-[var(--color-border)] bg-[var(--color-surface)] px-2.5 py-1 text-xs text-[var(--color-text)]"
						>
							{city.title}
							{#if city.is_baseline}
								<span class="text-[var(--color-muted)]">({t('col.baseline', 'baseline')})</span>
							{:else}
								<button
									type="button"
									onclick={() => toggleCity(city.id)}
									class="text-[var(--color-muted)] hover:text-[var(--color-text)]"
									aria-label={t('common.remove', 'Remove')}
								>
									<X size={12} />
								</button>
							{/if}
						</span>
					{/each}
				</div>
			{/if}

			<!-- Available cities -->
			<div class="flex flex-wrap gap-2">
				{#each cities as city (city.id)}
					{#if !selectedIds.includes(city.id)}
						<button
							type="button"
							onclick={() => toggleCity(city.id)}
							disabled={selectedIds.length >= 3}
							class="rounded-full border border-[var(--color-border)] bg-[var(--color-surface)] px-2.5 py-1 text-xs text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)] disabled:opacity-40"
						>
							{city.title}
						</button>
					{/if}
				{/each}
			</div>
		</div>

		<!-- Comparison matrix -->
		<div class="p-4">
			{#if selectedIds.length < 2}
				<EmptyState
					icon={Calculator}
					title={t('col.select_more', 'Select at least 2 cities to compare')}
				/>
			{:else if loading}
				<LoadingSkeleton variant="lines" />
			{:else if error}
				<EmptyState icon={Calculator} title={t('col.load_error', 'Could not load comparison')} body={error} />
			{:else if matrix}
				<!-- Overall index -->
				<div class="mb-4 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
					<h2 class="mb-3 text-sm font-medium text-[var(--color-text)]">
						{t('col.overall_index', 'Overall Cost Index')}
					</h2>
					<div class="space-y-2">
						{#each matrix.cities as city (city.id)}
							<div class="flex items-center justify-between text-sm">
								<span class="text-[var(--color-text)]">
									{city.title}
									{#if city.is_baseline}
										<span class="text-[var(--color-muted)]">({t('col.baseline', 'baseline')})</span>
									{/if}
								</span>
								<span class="font-medium text-[var(--color-text)]">
									{formatRatio(matrix.overall_index[city.id] ?? 1)}
								</span>
							</div>
						{/each}
					</div>
				</div>

				<!-- Category breakdown table -->
				<div class="overflow-x-auto rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)]">
					<table class="w-full text-sm">
						<thead>
							<tr class="border-b border-[var(--color-border)] text-left text-xs text-[var(--color-muted)]">
								<th class="px-3 py-2 font-medium">{t('col.category', 'Category')}</th>
								{#each matrix.cities as city (city.id)}
									<th class="px-3 py-2 text-right font-medium">{city.title}</th>
								{/each}
							</tr>
						</thead>
						<tbody>
							{#each matrix.categories as cat (cat.key)}
								<tr class="border-b border-[var(--color-border)] last:border-0">
									<td class="px-3 py-2 text-[var(--color-text)]">{cat.label}</td>
									{#each matrix.cities as city (city.id)}
										<td class="px-3 py-2 text-right text-[var(--color-text)]">
											<div class="font-medium">{cat.values[city.id]?.toFixed(0) ?? '—'}</div>
											{#if !city.is_baseline && cat.ratios[city.id]}
												<div class="text-xs text-[var(--color-muted)]">{formatRatio(cat.ratios[city.id])}</div>
											{/if}
										</td>
									{/each}
								</tr>
							{/each}
						</tbody>
					</table>
				</div>
			{/if}
		</div>
	{/if}
</div>
