<script lang="ts">
	import { untrack, onMount } from 'svelte';
	import { t } from '$lib/i18n';
	import { page } from '$app/state';
	import { enhance } from '$app/forms';
	import { Route, MapPin, GitCompare, DollarSign, Map, ExternalLink, Lock, Loader2 } from 'lucide-svelte';
	import { EmptyState, LoadingSkeleton } from '$lib/components/ui';
	import { api } from '$lib/api/client';
	import type { VisaRouteWithAi, GeoCountry } from '$lib/types';
	import type { PageData, ActionData } from './$types';
	import Autocomplete from '$lib/components/create/Autocomplete.svelte';
	import type { AutocompleteItem } from '$lib/components/create/Autocomplete.svelte';

	/**
	 * Visa Route Comparison tool — §90, ADR-088.
	 * Destination selector → list of visa route cards with checkboxes →
	 * side-by-side comparison table (up to 4 routes) fetched from
	 * /visa-routes/compare?ids=...
	 */
	let { data, form }: { data: PageData; form?: ActionData } = $props();

	const title = $derived(t('visa_routes.title', 'Visa Route Comparison'));
	const description = $derived(
		t(
			'visa_routes.subtitle',
			'Compare visa routes side by side — eligibility, fees, path to PR, and renewal terms.'
		)
	);

	let destinations = $state<GeoCountry[]>(untrack(() => data.destinations));
	let routes = $state<VisaRouteWithAi[]>(untrack(() => data.routes));
	let selectedDestId = $state<string>(untrack(() => data.initialDestId ?? ''));
	let loadingRoutes = $state(false);

	// Comparison state
	let selectedRouteIds = $state<Set<string>>(new Set());
	let comparisonData = $state<VisaRouteWithAi[]>([]);
	let loadingComparison = $state(false);
	let showComparison = $state(false);
	let savingJCE = $state(false);

	const selectedRoutes = $derived(routes.filter((r) => selectedRouteIds.has(r.id)));

	const canCompare = $derived(selectedRouteIds.size >= 2 && selectedRouteIds.size <= 4);

	async function loadRoutes() {
		if (!selectedDestId) {
			routes = [];
			return;
		}
		loadingRoutes = true;
		const res = await api.get<VisaRouteWithAi[]>(`/visa-routes?dest=${selectedDestId}`);
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
		const res = await api.get<VisaRouteWithAi[]>(`/visa-routes/compare?dest=${selectedDestId}&ids=${ids}`);
		loadingComparison = false;
		if (!res.error) comparisonData = res.data ?? [];
		else comparisonData = [];
	}

	function onDestChange(id: string) {
		selectedDestId = id;
		// Update URL ?dest= for shareable state
		const url = new URL(page.url);
		if (id) url.searchParams.set('dest', id);
		else url.searchParams.delete('dest');
		history.replaceState({}, '', url);
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

	// Autocomplete fetchers — filter the already-loaded destinations client-side.
	function fetchDestinations(q: string): Promise<AutocompleteItem[]> {
		const query = q.toLowerCase();
		const filtered = query === ''
			? destinations
			: destinations.filter((d) => d.name.toLowerCase().includes(query) || d.geo_code.toLowerCase().includes(query));
		return Promise.resolve(filtered.map((d) => ({ id: d.id, label: d.name, sublabel: d.geo_code })));
	}
	function fetchDestinationsInitial(): Promise<AutocompleteItem[]> {
		return fetchDestinations('');
	}
	const selectedDestLabel = $derived(
		selectedDestId ? destinations.find((d) => d.id === selectedDestId)?.name ?? null : null
	);
</script>

<svelte:head>
	<title>{title}</title>
	<meta name="description" content={description} />
	<link rel="canonical" href={page.url.href} />
	<meta property="og:title" content={title} />
	<meta property="og:description" content={description} />
	<meta property="og:url" content={page.url.href} />
	<meta property="og:type" content="website" />
	<meta name="twitter:card" content="summary" />
	<meta name="twitter:title" content={title} />
	<meta name="twitter:description" content={description} />
</svelte:head>

<div class="mx-auto w-full max-w-3xl">
	<div class="mx-4 border-b border-[var(--color-border)] px-4 py-3">
		<h1 class="flex items-center gap-2 text-lg font-semibold text-[var(--color-text)]">
			<Route size={20} /> {title}
		</h1>
		<p class="mt-1 text-sm text-[var(--color-muted)]">{description}</p>
	</div>

	{#if data.disabled}
		<div class="p-4">
			<EmptyState
				icon={Lock}
				title={t('tools.disabled_title', 'This tool is currently unavailable')}
				body={t('tools.disabled_body', 'Check back soon or contact support.')}
			/>
		</div>
	{:else if destinations.length === 0}
		<div class="p-4">
			<EmptyState
				icon={Route}
				title={t('visa_routes.no_destinations', 'No destinations available')}
				body={t(
					'visa_routes.no_destinations_desc',
					'Visa route data hasn’t been configured yet. Please check back later.'
				)}
			/>
		</div>
	{:else}
		<!-- Destination selector (geo-aware pattern — first input) -->
		<div class="border-b border-[var(--color-border)] px-4 py-3">
			<label for="vr-dest" class="block text-xs font-medium uppercase tracking-wide text-[var(--color-muted)]">
				{t('visa_routes.destination', 'Destination country')}
			</label>
			<div class="mt-2">
				<Autocomplete
					placeholder={t('visa_routes.select_destination', 'Select a destination country')}
					emptyMessage={t('common.no_results', 'No results found')}
					fetcher={fetchDestinations}
					initialFetcher={fetchDestinationsInitial}
					selectedId={selectedDestId || null}
					selectedLabel={selectedDestLabel}
					onSelect={(item) => onDestChange(item.id)}
					onClear={() => onDestChange('')}
				/>
			</div>
		</div>

		{#if !selectedDestId}
			<div class="p-4">
				<EmptyState
					icon={MapPin}
					title={t('visa_routes.select_destination', 'Select a destination country')}
					body={t(
						'visa_routes.select_destination_desc',
						'Choose a country above to see available visa routes.'
					)}
				/>
			</div>
		{:else if loadingRoutes}
			<div class="p-4 space-y-3">
				<LoadingSkeleton variant="card" count={3} />
			</div>
		{:else if routes.length === 0}
			<div class="p-4">
				<EmptyState
					icon={Route}
					title={t('visa_routes.no_routes', 'No visa routes available')}
					body={t(
						'visa_routes.no_routes_desc',
						'No visa routes have been published for this destination yet.'
					)}
				/>
			</div>
		{:else}
			<div class="p-4">
				<!-- Selection counter + compare buttons -->
				<div class="mb-4 flex items-center justify-between flex-wrap gap-3">
					<p class="text-sm text-[var(--color-muted)]">
						{t('visa_routes.selected', 'Selected')}: {selectedRouteIds.size} / 4
					</p>
					<div class="flex flex-wrap items-center gap-2">
						<button
							type="button"
							onclick={runComparison}
							disabled={!canCompare}
							class="inline-flex items-center gap-2 px-4 py-2 border border-[var(--color-border)] text-[var(--color-text)] rounded-[var(--radius)] text-sm font-medium hover:bg-[var(--color-surface-subtle)] transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
						>
							<GitCompare size={16} />
							{t('visa_routes.compare', 'Compare')}
						</button>

						<form
							method="POST"
							use:enhance={() => {
								savingJCE = true;
								return async ({ update }) => {
									savingJCE = false;
									await update();
								};
							}}
							class="contents"
						>
							<input type="hidden" name="dest_id" value={selectedDestId} />
							{#each selectedRoutes as route (route.id)}
								<input type="hidden" name="route_id" value={route.id} />
							{/each}
							<button
								type="submit"
								disabled={!canCompare || !data.authed}
								title={data.authed
									? t('visa_routes.open_in_jce_hint', 'Open in JCE comparison')
									: t('visa_routes.sign_in_to_compare', 'Sign in to compare')}
								class="inline-flex items-center gap-2 px-4 py-2 bg-[var(--color-primary)] text-[var(--color-primary-contrast)] rounded-[var(--radius)] text-sm font-medium hover:bg-[var(--color-primary-hover)] transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
							>
								{#if savingJCE}
									<Loader2 size={16} class="animate-spin" />
								{:else}
									<GitCompare size={16} />
								{/if}
								{t('visa_routes.open_in_jce', 'Compare in JCE')}
							</button>
						</form>
					</div>
				</div>

				{#if form?.error}
					<div class="mb-4 rounded-[var(--radius)] border border-red-200 bg-red-50 p-3 text-sm text-red-700 dark:border-red-900 dark:bg-red-950 dark:text-red-300">
						{form.message || t('visa_routes.jce_error', 'Could not create comparison')}
					</div>
				{/if}

				<!-- Route cards -->
				<div class="grid grid-cols-1 md:grid-cols-2 gap-4 mb-8">
					{#each routes as route (route.id)}
						<label
							class="p-4 bg-[var(--color-surface)] border border-[var(--color-border)] rounded-[var(--radius-card)] cursor-pointer transition-colors hover:border-[var(--color-primary)] {selectedRouteIds.has(route.id) ? 'border-[var(--color-primary)] ring-1 ring-[var(--color-primary)]' : ''}"
						>
							<div class="flex items-start gap-3">
								<input
									type="checkbox"
									checked={selectedRouteIds.has(route.id)}
									onchange={() => toggleRouteSelection(route.id)}
									disabled={selectedRouteIds.size >= 4 && !selectedRouteIds.has(route.id)}
									class="mt-1 accent-[var(--color-primary)]"
								/>
								<div class="min-w-0 flex-1">
									<div class="flex items-center gap-2">
										<h3 class="text-sm font-semibold text-[var(--color-text)]">{route.name}</h3>
										<span class="text-xs px-2 py-0.5 bg-[var(--color-surface-subtle)] text-[var(--color-muted)] rounded-full">
											{route.visa_category}
										</span>
									</div>
									{#if route.eligibility_summary}
										<p class="mt-1.5 text-sm text-[var(--color-text-subtle)] line-clamp-2">{route.eligibility_summary}</p>
									{/if}
									<div class="mt-3 flex items-center gap-4 text-xs text-[var(--color-muted)]">
										{#if route.typical_fee_usd != null}
											<span class="flex items-center gap-1">
												<DollarSign size={12} />
												{formatFee(route.typical_fee_usd)}
											</span>
										{/if}
										{#if route.path_to_pr}
											<span class="flex items-center gap-1">
												<Map size={12} />
												{t('visa_routes.path_to_pr', 'PR path')}
											</span>
										{/if}
										{#if route.gov_source_url}
											<a
												href={route.gov_source_url}
												target="_blank"
												rel="noopener noreferrer"
												class="flex items-center gap-1 text-[var(--color-muted)] hover:text-[var(--color-text)]"
												onclick={(e) => e.stopPropagation()}
											>
												<ExternalLink size={12} />
												{t('visa_routes.source', 'Source')}</a>
									<a
										href="/tools/visa-routes/{route.id}"
										class="text-[var(--color-primary)] hover:underline"
										onclick={(e) => e.stopPropagation()}
									>
										{t('common.view', 'View')}
											</a>
										{/if}
									</div>
								</div>
							</div>
						</label>
					{/each}
				</div>

				<!-- Comparison table -->
				{#if showComparison}
					<div class="mt-8">
						<h2 class="text-sm font-semibold text-[var(--color-muted)] uppercase tracking-wide mb-3">
							{t('visa_routes.comparison', 'Comparison')}
						</h2>
						{#if loadingComparison}
							<LoadingSkeleton variant="lines" count={5} />
						{:else if comparisonData.length === 0}
							<EmptyState
								icon={GitCompare}
								title={t('visa_routes.no_comparison', 'Could not load comparison')}
								body={t(
									'visa_routes.no_comparison_desc',
									'An error occurred while fetching the comparison data.'
								)}
							/>
						{:else}
							<div class="overflow-x-auto bg-[var(--color-surface)] border border-[var(--color-border)] rounded-[var(--radius-card)]">
								<table class="w-full text-sm">
									<thead>
										<tr class="border-b border-[var(--color-border)]">
											<th class="text-left p-3 font-semibold text-[var(--color-muted)] uppercase tracking-wide text-xs w-32">
												{t('visa_routes.attribute', 'Attribute')}
											</th>
											{#each comparisonData as route (route.id)}
												<th class="text-left p-3 font-semibold text-[var(--color-text)] align-top">
													{route.name}
													<span class="block text-xs font-normal text-[var(--color-muted)] mt-0.5">{route.visa_category}</span>
												</th>
											{/each}
										</tr>
									</thead>
									<tbody class="divide-y divide-[var(--color-border)]">
										<tr>
											<td class="p-3 font-medium text-[var(--color-muted)]">{t('visa_routes.fee', 'Typical fee')}</td>
											{#each comparisonData as route (route.id)}
												<td class="p-3 text-[var(--color-text)]">{formatFee(route.typical_fee_usd)}</td>
											{/each}
										</tr>
										<tr>
											<td class="p-3 font-medium text-[var(--color-muted)]">{t('visa_routes.path_to_pr', 'Path to PR')}</td>
											{#each comparisonData as route (route.id)}
												<td class="p-3 text-[var(--color-text)]">{orDash(route.path_to_pr)}</td>
											{/each}
										</tr>
										<tr>
											<td class="p-3 font-medium text-[var(--color-muted)]">{t('visa_routes.renewal_terms', 'Renewal terms')}</td>
											{#each comparisonData as route (route.id)}
												<td class="p-3 text-[var(--color-text)]">{orDash(route.renewal_terms)}</td>
											{/each}
										</tr>
										<tr>
											<td class="p-3 font-medium text-[var(--color-muted)]">{t('visa_routes.restrictions', 'Restrictions')}</td>
											{#each comparisonData as route (route.id)}
												<td class="p-3 text-[var(--color-text)]">{orDash(route.restrictions)}</td>
											{/each}
										</tr>
										<tr>
											<td class="p-3 font-medium text-[var(--color-muted)]">{t('visa_routes.gov_source', 'Official source')}</td>
											{#each comparisonData as route (route.id)}
												<td class="p-3">
													{#if route.gov_source_url}
														<a
															href={route.gov_source_url}
															target="_blank"
															rel="noopener noreferrer"
															class="inline-flex items-center gap-1 text-[var(--color-primary)] hover:underline"
														>
															<ExternalLink size={12} />
															{t('visa_routes.view_source', 'View')}
														</a>
													{:else}
														<span class="text-[var(--color-muted)]">—</span>
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
			</div>
		{/if}
	{/if}
</div>
