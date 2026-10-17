<script lang="ts">
	import { untrack, onMount } from 'svelte';
	import { api } from '$lib/api/client';
	import { mainAppUrl, buildSeoMeta } from '$lib/seo';
	import SeoHead from '$lib/components/seo/SeoHead.svelte';
	import { t } from '$lib/i18n';
	import { page } from '$app/state';
	import { replaceState } from '$app/navigation';
	import Icon from '$lib/components/ui/Icon.svelte';
	import EmptyState from '$lib/components/ui/EmptyState.svelte';
	import LoadingSkeleton from '$lib/components/ui/LoadingSkeleton.svelte';
	import ToolDisabled from '$lib/components/ui/ToolDisabled.svelte';
	import DestinationSelector from '$lib/components/tools/DestinationSelector.svelte';
	import type { VisaRouteWithAi, GeoSpaceOption } from '$lib/types';

	/**
	 * Visa Route Comparison tool — §90, ADR-088.
	 * Destination selector → list of visa route cards with checkboxes →
	 * side-by-side comparison table (up to 4 routes) fetched from
	 * /visa-routes/compare?ids=...
	 */
	let { data } = $props();

	let destinations = $state<GeoSpaceOption[]>(untrack(() => data.destinations));
	let routes = $state<VisaRouteWithAi[]>(untrack(() => data.routes));
	let selectedDestId = $state<string>(untrack(() => data.initialDestId ?? ''));
	let loadingRoutes = $state(false);

	// Comparison state
	let selectedRouteIds = $state<Set<string>>(new Set());
	let comparisonData = $state<VisaRouteWithAi[]>([]);
	let loadingComparison = $state(false);
	let showComparison = $state(false);

	const seo = $derived(
		buildSeoMeta({
			title: t('visa_routes.seo_title', 'Visa Route Comparison — {site} Tools'),
			description: t(
				'visa_routes.seo_description',
				'Compare visa routes for your destination side by side — eligibility, fees, path to PR, and renewal terms.'
			),
			path: '/visa-routes',
			origin: page.url.origin,
			ogImage: `${page.url.origin}/og/og-visa-routes.png`,
			jsonLd: {
				'@context': 'https://schema.org',
				'@type': 'ItemList',
				name: 'Visa Route Comparison',
				description: 'Compare visa routes side by side — eligibility, fees, path to PR, and renewal terms.',
				itemListElement: routes.slice(0, 10).map((r, i) => ({
					'@type': 'ListItem',
					position: i + 1,
					name: r.name,
					url: `${page.url.origin}/visa-routes/${r.id}`
				}))
			}
		})
	);

	const selectedRoutes = $derived(
		routes.filter((r) => selectedRouteIds.has(r.id))
	);

	const canCompare = $derived(selectedRouteIds.size >= 2 && selectedRouteIds.size <= 4);

	async function loadRoutes() {
		if (!selectedDestId) {
			routes = [];
			return;
		}
		loadingRoutes = true;
		const res = await api.getList<VisaRouteWithAi>(`/visa-routes?dest=${selectedDestId}`);
		loadingRoutes = false;
		if (!res.error) routes = res.data ?? [];
		else routes = [];
		// Reset selection when destination changes
		selectedRouteIds = new Set();
		showComparison = false;
		comparisonData = [];
	}

	function toggleRouteSelection(id: string) {
		const next = new Set(selectedRouteIds);
		if (next.has(id)) {
			next.delete(id);
		} else {
			if (next.size >= 4) return; // max 4
			next.add(id);
		}
		selectedRouteIds = next;
		// Hide stale comparison when selection changes
		showComparison = false;
		comparisonData = [];
	}

	async function runComparison() {
		if (!canCompare) return;
		loadingComparison = true;
		showComparison = true;
		const ids = Array.from(selectedRouteIds).join(',');
		const res = await api.getEnvelope<VisaRouteWithAi[]>(`/visa-routes/compare?ids=${ids}`);
		loadingComparison = false;
		// Compare may return 207 multi-status with missing_ids; render the
		// resolved routes and drop the missing ones (logged for diagnosis).
		const missing = (res.envelope?.missing_ids as string[] | undefined) ?? [];
		if (missing.length > 0) console.warn('visa compare: missing routes', missing);
		if (!res.error) comparisonData = res.data ?? [];
		else comparisonData = [];
	}

	function onDestChange(id: string) {
		selectedDestId = id;
		// Update URL ?dest= for shareable state
		const url = new URL(page.url);
		if (id) url.searchParams.set('dest', id);
		else url.searchParams.delete('dest');
		replaceState(url, {});
		loadRoutes();
	}

	function formatFee(fee: number | null | undefined): string {
		if (fee == null) return '—';
		return new Intl.NumberFormat('en-US', {
			style: 'currency',
			currency: 'USD',
			maximumFractionDigits: 0
		}).format(fee);
	}

	function orDash(value: string | null | undefined): string {
		return value && value.trim() ? value : '—';
	}

	// On mount: if SSR already loaded routes for the initial dest, no need to
	// re-fetch. If no dest was selected, do nothing (user picks one).
	onMount(() => {
		// Routes are already populated from SSR when initialDestId was set.
		// Nothing to do here unless we want to sync URL — already handled.
	});
</script>

<SeoHead seo={seo} ogType="website" twitterCard="summary" />

<div class="mx-auto max-w-5xl px-4 py-8">
	{#if !data.enabled}
		<ToolDisabled tool={data.tool} />
	{:else}
		<div class="mb-8">
			<h1 class="text-2xl sm:text-3xl font-bold text-text">
				{t('visa_routes.title', 'Visa Route Comparison')}
			</h1>
			<p class="mt-2 text-text-subtle">
				{t('visa_routes.subtitle', 'Compare visa routes side by side — eligibility, fees, path to PR, and renewal terms.')}
			</p>
		</div>

		{#if destinations.length === 0}
			<EmptyState
				icon="route"
				title={t('visa_routes.no_destinations', 'No destinations available')}
				description={t('visa_routes.no_destinations_desc', 'Visa route data hasn’t been configured yet. Please check back later.')}
			/>
		{:else}
			<!-- Destination selector (geo-aware pattern — first input) -->
			<div class="mb-6">
				<DestinationSelector {destinations} bind:selectedId={selectedDestId} onchange={onDestChange} />
			</div>

			{#if !selectedDestId}
				<EmptyState
					icon="map-pin"
					title={t('visa_routes.select_destination', 'Select a destination country')}
					description={t('visa_routes.select_destination_desc', 'Choose a country above to see available visa routes.')}
				/>
			{:else if loadingRoutes}
				<div class="space-y-3">
					<LoadingSkeleton height="120px" />
					<LoadingSkeleton height="120px" />
					<LoadingSkeleton height="120px" />
				</div>
			{:else if routes.length === 0}
				<EmptyState
					icon="route"
					title={t('visa_routes.no_routes', 'No visa routes available')}
					description={t('visa_routes.no_routes_desc', 'No visa routes have been published for this destination yet.')}
				/>
			{:else}
				<!-- Selection counter + compare button -->
				<div class="mb-4 flex items-center justify-between flex-wrap gap-3">
					<p class="text-sm text-muted">
						{t('visa_routes.selected', 'Selected')}: {selectedRouteIds.size} / 4
					</p>
					<button
						onclick={runComparison}
						disabled={!canCompare}
						class="inline-flex items-center gap-2 px-4 py-2 bg-[var(--tool-accent)] text-white rounded-lg text-sm font-medium hover:bg-[var(--tool-accent-hover)] transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
					>
						<Icon name="git-compare" size={16} />
						{t('visa_routes.compare', 'Compare')}
					</button>
				</div>

				<!-- Route cards -->
				<div class="grid grid-cols-1 md:grid-cols-2 gap-4 mb-8">
					{#each routes as route (route.id)}
						<label
							class="p-4 bg-surface border border-border rounded-lg cursor-pointer transition-colors hover:border-[var(--tool-accent)] {selectedRouteIds.has(route.id) ? 'border-[var(--tool-accent)] ring-1 ring-[var(--tool-accent)]' : ''}"
						>
							<div class="flex items-start gap-3">
								<input
									type="checkbox"
									checked={selectedRouteIds.has(route.id)}
									onchange={() => toggleRouteSelection(route.id)}
									disabled={selectedRouteIds.size >= 4 && !selectedRouteIds.has(route.id)}
									class="mt-1 accent-[var(--tool-accent)]"
								/>
								<div class="min-w-0 flex-1">
									<div class="flex items-center gap-2">
										<h3 class="text-sm font-semibold text-text">{route.name}</h3>
										<span class="text-xs px-2 py-0.5 bg-surface-subtle text-muted rounded-full">
											{route.visa_category}
										</span>
									</div>
									{#if route.eligibility_summary}
										<p class="mt-1.5 text-sm text-text-subtle line-clamp-2">{route.eligibility_summary}</p>
									{/if}
									<div class="mt-3 flex items-center gap-4 text-xs text-muted">
										{#if route.typical_fee_usd != null}
											<span class="flex items-center gap-1">
												<Icon name="dollar-sign" size={12} />
												{formatFee(route.typical_fee_usd)}
											</span>
										{/if}
										{#if route.path_to_pr}
											<span class="flex items-center gap-1">
												<Icon name="map" size={12} />
												{t('visa_routes.path_to_pr', 'PR path')}
											</span>
										{/if}
										{#if route.gov_source_url}
											<a
												href={route.gov_source_url}
												target="_blank"
												rel="noopener noreferrer"
												class="flex items-center gap-1 text-muted hover:text-text"
												onclick={(e) => e.stopPropagation()}
											>
												<Icon name="external-link" size={12} />
												{t('visa_routes.source', 'Source')}
											</a>
										{/if}
										<a
											href={`/visa-routes/${route.id}`}
											class="flex items-center gap-1 text-[var(--tool-accent)] hover:text-text"
											onclick={(e) => e.stopPropagation()}
										>
											<Icon name="external-link" size={12} />
											{t('visa_routes.view_route', 'View route')}
										</a>
									</div>
								</div>
							</div>
						</label>
					{/each}
				</div>

				<!-- Comparison table -->
				{#if showComparison}
					<div class="mt-8">
						<h2 class="text-sm font-semibold text-muted uppercase tracking-wide mb-3">
							{t('visa_routes.comparison', 'Comparison')}
						</h2>
						{#if loadingComparison}
							<LoadingSkeleton height="200px" />
						{:else if comparisonData.length === 0}
							<EmptyState
								icon="git-compare"
								title={t('visa_routes.no_comparison', 'Could not load comparison')}
								description={t('visa_routes.no_comparison_desc', 'An error occurred while fetching the comparison data.')}
							/>
						{:else}
							<div class="overflow-x-auto bg-surface border border-border rounded-lg">
								<table class="w-full text-sm">
									<thead>
										<tr class="border-b border-border">
											<th class="text-left p-3 font-semibold text-muted uppercase tracking-wide text-xs w-32">
												{t('visa_routes.attribute', 'Attribute')}
											</th>
											{#each comparisonData as route (route.id)}
												<th class="text-left p-3 font-semibold text-text align-top">
													{route.name}
													<span class="block text-xs font-normal text-muted mt-0.5">{route.visa_category}</span>
												</th>
											{/each}
										</tr>
									</thead>
									<tbody class="divide-y divide-border">
										<tr>
											<td class="p-3 font-medium text-muted">{t('visa_routes.fee', 'Typical fee')}</td>
											{#each comparisonData as route (route.id)}
												<td class="p-3 text-text">{formatFee(route.typical_fee_usd)}</td>
											{/each}
										</tr>
										<tr>
											<td class="p-3 font-medium text-muted">{t('visa_routes.path_to_pr', 'Path to PR')}</td>
											{#each comparisonData as route (route.id)}
												<td class="p-3 text-text">{orDash(route.path_to_pr)}</td>
											{/each}
										</tr>
										<tr>
											<td class="p-3 font-medium text-muted">{t('visa_routes.renewal_terms', 'Renewal terms')}</td>
											{#each comparisonData as route (route.id)}
												<td class="p-3 text-text">{orDash(route.renewal_terms)}</td>
											{/each}
										</tr>
										<tr>
											<td class="p-3 font-medium text-muted">{t('visa_routes.restrictions', 'Restrictions')}</td>
											{#each comparisonData as route (route.id)}
												<td class="p-3 text-text">{orDash(route.restrictions)}</td>
											{/each}
										</tr>
										<tr>
											<td class="p-3 font-medium text-muted">{t('visa_routes.gov_source', 'Official source')}</td>
											{#each comparisonData as route (route.id)}
												<td class="p-3">
													{#if route.gov_source_url}
														<a
															href={route.gov_source_url}
															target="_blank"
															rel="noopener noreferrer"
															class="inline-flex items-center gap-1 text-[var(--tool-accent)] hover:underline"
														>
															<Icon name="external-link" size={12} />
															{t('visa_routes.view_source', 'View')}
														</a>
													{:else}
														<span class="text-muted">—</span>
													{/if}
												</td>
											{/each}
										</tr>
									</tbody>
								</table>
							</div>
						{/if}
					</div>
				{/if}
			{/if}
		{/if}
	{/if}
</div>
