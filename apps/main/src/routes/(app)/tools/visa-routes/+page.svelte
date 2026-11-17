<script lang="ts">
	import SeoHead from '#lib/seo/SeoHead.svelte';
	import { toolOgCard } from '#lib/seo/og.js';
	import { untrack, onMount } from 'svelte';
	import { t } from '#lib/i18n/index.js';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { enhance } from '$app/forms';
	import { Route, MapPin, GitCompare, DollarSign, Map, ExternalLink, Lock, Loader2, Clock, ListChecks } from 'lucide-svelte';
	import { EmptyState, LoadingSkeleton } from '#lib/components/ui/index.js';
	import { api } from '#lib/api/client.js';
	import type { VisaRouteWithAi, GeoCountry } from '#lib/types/index.js';
	import type { PageData, ActionData } from './$types';
	import Autocomplete from '#lib/components/create/Autocomplete.svelte';
	import type { AutocompleteItem } from '#lib/components/create/Autocomplete.svelte';
	import ToolHeader from '#lib/components/tools/ToolHeader.svelte';

	/**
	 * Visa Route Comparison tool — §90, ADR-088.
	 * Destination selector + profile filters → list of visa route cards with
	 * checkboxes → side-by-side comparison table (up to 4 routes) fetched from
	 * /visa-routes/compare?ids=... Selection state is shareable via
	 * ?dest={id}&ids={a,b,c}.
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

	// Comparison state — ?ids= preselects routes from the shared link.
	let selectedRouteIds = $state<Set<string>>(
		new Set(untrack(() => data.initialRouteIds ?? []).filter((id) => routes.some((r) => r.id === id)))
	);
	let comparisonData = $state<VisaRouteWithAi[]>([]);
	let comparisonMissing = $state<string[]>([]);
	let loadingComparison = $state(false);
	let showComparison = $state(false);
	let savingJCE = $state(false);

	// Profile filters (§90 — rule-based eligibility matching against
	// requirements_json). Empty = no constraint.
	let profileProfession = $state('');
	let profileEducation = $state('');
	let profileExperience = $state('');
	let profileAge = $state('');
	let profileOriginId = $state('');

	const educationLevels = [
		{ value: 'high_school', key: 'visa_routes.edu_high_school', fallback: 'High school' },
		{ value: 'diploma', key: 'visa_routes.edu_diploma', fallback: 'Diploma' },
		{ value: 'bachelor', key: 'visa_routes.edu_bachelor', fallback: "Bachelor's degree" },
		{ value: 'master', key: 'visa_routes.edu_master', fallback: "Master's degree" },
		{ value: 'phd', key: 'visa_routes.edu_phd', fallback: 'Doctorate / PhD' }
	];

	const hasProfileFilters = $derived(
		profileProfession.trim() !== '' ||
			profileEducation !== '' ||
			profileExperience.trim() !== '' ||
			profileAge.trim() !== '' ||
			profileOriginId !== ''
	);

	const selectedRoutes = $derived(routes.filter((r) => selectedRouteIds.has(r.id)));

	const canCompare = $derived(selectedRouteIds.size >= 2 && selectedRouteIds.size <= 4);

	// syncUrl writes ?dest=&ids= back so the comparison is shareable.
	function syncUrl() {
		const url = new URL(page.url.href);
		if (selectedDestId) url.searchParams.set('dest', selectedDestId);
		else url.searchParams.delete('dest');
		if (selectedRouteIds.size > 0) url.searchParams.set('ids', Array.from(selectedRouteIds).join(','));
		else url.searchParams.delete('ids');
		goto(url, { shallow: true, replace: true });
	}

	async function loadRoutes() {
		if (!selectedDestId) {
			routes = [];
			return;
		}
		loadingRoutes = true;
		const params = new URLSearchParams({ dest: selectedDestId });
		if (profileProfession.trim()) params.set('profession', profileProfession.trim());
		if (profileEducation) params.set('education', profileEducation);
		if (profileExperience.trim()) params.set('experience_years', profileExperience.trim());
		if (profileAge.trim()) params.set('age', profileAge.trim());
		if (profileOriginId) params.set('origin', profileOriginId);
		const res = await api.getEnvelope<VisaRouteWithAi[]>(`/visa-routes?${params}`);
		loadingRoutes = false;
		if (!res.error) routes = res.data ?? [];
		else routes = [];
		// Drop selections that no longer resolve against the filtered set.
		selectedRouteIds = new Set([...selectedRouteIds].filter((id) => routes.some((r) => r.id === id)));
		showComparison = false;
		comparisonData = [];
		syncUrl();
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
		syncUrl();
	}

	async function runComparison() {
		if (!canCompare) return;
		loadingComparison = true;
		showComparison = true;
		const ids = Array.from(selectedRouteIds).join(',');
		const res = await api.getEnvelope<VisaRouteWithAi[]>(`/visa-routes/compare?dest=${selectedDestId}&ids=${ids}`);
		// 207 multi-status: surface missing_ids instead of silently dropping routes.
		const missing = res.envelope?.missing_ids as string[] | undefined ?? [];
		loadingComparison = false;
		if (!res.error) comparisonData = res.data ?? [];
		else comparisonData = [];
		comparisonMissing = missing;
	}

	function onDestChange(id: string) {
		selectedDestId = id;
		selectedRouteIds = new Set();
		loadRoutes();
	}

	function applyProfileFilters() {
		loadRoutes();
	}

	function formatProcessingDays(route: VisaRouteWithAi): string {
		if (route.typical_processing_days == null) return '—';
		const days = Math.round(route.typical_processing_days);
		return `${days} ${t('visa_routes.days', 'days')}`;
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

	// Shared-link deep state: a ?ids= selection lands with the comparison
	// already run so the recipient sees the table, not just checked boxes.
	onMount(() => {
		if (canCompare) void runComparison();
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
	const selectedOriginLabel = $derived(
		profileOriginId ? destinations.find((d) => d.id === profileOriginId)?.name ?? null : null
	);
</script>

<SeoHead title={title} description={description} ogTitle={title}
	ogImage={toolOgCard(page.url.origin, page.url.pathname)}
/>

<div class="mx-auto w-full max-w-3xl">
	<div class="px-4 pt-4">
		<ToolHeader icon={Route} title={title} description={description} />
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

		{#if selectedDestId}
			<!-- Profile filters (§90 eligibility matching — rule-based,
			     matched against requirements_json server-side) -->
			<details class="border-b border-[var(--color-border)] px-4 py-3 group">
				<summary class="flex items-center justify-between cursor-pointer text-xs font-medium uppercase tracking-wide text-[var(--color-muted)] select-none">
					<span>{t('visa_routes.profile_filters', 'Your profile')}</span>
					{#if hasProfileFilters}
						<span class="normal-case px-2 py-0.5 bg-[var(--color-primary-soft)] text-[var(--color-primary-text)] rounded-full">
							{t('visa_routes.filters_active', 'Filters active')}
						</span>
					{/if}
				</summary>
				<form
					class="mt-3 grid grid-cols-1 sm:grid-cols-2 gap-3"
					onsubmit={(e) => { e.preventDefault(); applyProfileFilters(); }}
				>
					<div>
						<label for="vr-prof" class="block text-xs text-[var(--color-muted)]">{t('visa_routes.profession', 'Profession')}</label>
						<input
							id="vr-prof"
							type="text"
							bind:value={profileProfession}
							placeholder={t('visa_routes.profession_placeholder', 'e.g. Software engineer, nurse')}
							class="mt-1 w-full px-3 py-2 text-sm border border-[var(--color-border)] rounded-[var(--radius)] bg-[var(--color-surface)] text-[var(--color-text)]"
						/>
					</div>
					<div>
						<label for="vr-edu" class="block text-xs text-[var(--color-muted)]">{t('visa_routes.education', 'Education')}</label>
						<select
							id="vr-edu"
							bind:value={profileEducation}
							class="mt-1 w-full px-3 py-2 text-sm border border-[var(--color-border)] rounded-[var(--radius)] bg-[var(--color-surface)] text-[var(--color-text)]"
						>
							<option value="">{t('visa_routes.any_education', 'Any level')}</option>
							{#each educationLevels as lvl (lvl.value)}
								<option value={lvl.value}>{t(lvl.key, lvl.fallback)}</option>
							{/each}
						</select>
					</div>
					<div>
						<label for="vr-exp" class="block text-xs text-[var(--color-muted)]">{t('visa_routes.experience_years', 'Years of experience')}</label>
						<input
							id="vr-exp"
							type="number"
							min="0"
							max="60"
							bind:value={profileExperience}
							placeholder="5"
							class="mt-1 w-full px-3 py-2 text-sm border border-[var(--color-border)] rounded-[var(--radius)] bg-[var(--color-surface)] text-[var(--color-text)]"
						/>
					</div>
					<div>
						<label for="vr-age" class="block text-xs text-[var(--color-muted)]">{t('visa_routes.age', 'Age')}</label>
						<input
							id="vr-age"
							type="number"
							min="16"
							max="99"
							bind:value={profileAge}
							placeholder="30"
							class="mt-1 w-full px-3 py-2 text-sm border border-[var(--color-border)] rounded-[var(--radius)] bg-[var(--color-surface)] text-[var(--color-text)]"
						/>
					</div>
					<div class="sm:col-span-2">
						<label for="vr-origin" class="block text-xs text-[var(--color-muted)]">{t('visa_routes.origin', 'Nationality / origin country')}</label>
						<div class="mt-1">
							<Autocomplete
								placeholder={t('visa_routes.origin_placeholder', 'Your nationality (optional)')}
								emptyMessage={t('common.no_results', 'No results found')}
								fetcher={fetchDestinations}
								initialFetcher={fetchDestinationsInitial}
								selectedId={profileOriginId || null}
								selectedLabel={selectedOriginLabel}
								onSelect={(item) => { profileOriginId = item.id; }}
								onClear={() => { profileOriginId = ''; }}
							/>
						</div>
					</div>
					<div class="sm:col-span-2 flex items-center gap-2">
						<button
							type="submit"
							class="px-4 py-2 bg-[var(--color-primary)] text-[var(--color-primary-contrast)] rounded-[var(--radius)] text-sm font-medium hover:bg-[var(--color-primary-hover)] transition-colors"
						>
							{t('visa_routes.match_routes', 'Match my routes')}
						</button>
						{#if hasProfileFilters}
							<button
								type="button"
								onclick={() => { profileProfession = ''; profileEducation = ''; profileExperience = ''; profileAge = ''; profileOriginId = ''; loadRoutes(); }}
								class="px-3 py-2 text-sm text-[var(--color-muted)] hover:text-[var(--color-text)]"
							>
								{t('common.clear', 'Clear')}
							</button>
						{/if}
					</div>
				</form>
			</details>
		{/if}

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

				<!-- Route cards: a <label> can't legally nest links, so the card
				     is a div and the checkbox gets an explicit accessible label
				     (a11y — the old markup silently broke click delegation). -->
				<div class="grid grid-cols-1 md:grid-cols-2 gap-4 mb-8" role="group" aria-label={t('visa_routes.routes_label', 'Available visa routes')}>
					{#each routes as route (route.id)}
						<div
							class="p-4 bg-[var(--color-surface)] border border-[var(--color-border)] rounded-[var(--radius-card)] transition-colors {selectedRouteIds.has(route.id) ? 'border-[var(--color-primary)] ring-1 ring-[var(--color-primary)]' : ''}"
						>
							<div class="flex items-start gap-3">
								<input
									id="vr-select-{route.id}"
									type="checkbox"
									checked={selectedRouteIds.has(route.id)}
									onchange={() => toggleRouteSelection(route.id)}
									disabled={selectedRouteIds.size >= 4 && !selectedRouteIds.has(route.id)}
									aria-label={t('visa_routes.select_route', 'Select {name} for comparison').replace('{name}', route.name)}
									class="mt-1 accent-[var(--color-primary)]"
								/>
								<div class="min-w-0 flex-1">
									<div class="flex items-center gap-2">
										<label for="vr-select-{route.id}" class="text-sm font-semibold text-[var(--color-text)] cursor-pointer">{route.name}</label>
										<span class="text-xs px-2 py-0.5 bg-[var(--color-surface-subtle)] text-[var(--color-muted)] rounded-full">
											{route.visa_category}
										</span>
									</div>
									{#if route.eligibility_summary}
										<p class="mt-1.5 text-sm text-[var(--color-text-subtle)] line-clamp-2">{route.eligibility_summary}</p>
									{/if}
									<div class="mt-3 flex flex-wrap items-center gap-x-4 gap-y-1.5 text-xs text-[var(--color-muted)]">
										{#if route.typical_fee_usd != null}
											<span class="flex items-center gap-1">
												<DollarSign size={12} />
												{formatFee(route.typical_fee_usd)}
											</span>
										{/if}
										{#if route.typical_processing_days != null}
											<span class="flex items-center gap-1" title={t('visa_routes.processing_hint', 'Median reported processing time')}>
												<Clock size={12} />
												{formatProcessingDays(route)}
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
											>
												<ExternalLink class="rtl:-scale-x-100" size={12} />
												{t('visa_routes.source', 'Source')}
											</a>
										{/if}
										{#if route.guide_id}
											<a
												href="/tools/visa-checklist?guide_id={route.guide_id}"
												class="flex items-center gap-1 text-[var(--color-muted)] hover:text-[var(--color-text)]"
											>
												<ListChecks size={12} />
												{t('visa_routes.checklist', 'Checklist')}
											</a>
										{/if}
										<a
											href="/tools/visa-routes/{route.id}"
											class="text-[var(--color-primary-text)] hover:underline"
										>
											{t('common.view', 'View')}
										</a>
									</div>
								</div>
							</div>
						</div>
					{/each}
				</div>

				<!-- Comparison table -->
				{#if showComparison}
					<div class="mt-8">
						<h2 class="text-sm font-semibold text-[var(--color-muted)] uppercase tracking-wide mb-3">
							{t('visa_routes.comparison', 'Comparison')}
						</h2>
						{#if comparisonMissing.length > 0}
							<p class="mb-2 text-xs text-[var(--color-muted)]">
								{t('visa_routes.partial_comparison', '{count} selected route(s) are no longer available.').replace('{count}', String(comparisonMissing.length))}
							</p>
						{/if}
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
											<th class="text-start p-3 font-semibold text-[var(--color-muted)] uppercase tracking-wide text-xs w-32">
												{t('visa_routes.attribute', 'Attribute')}
											</th>
											{#each comparisonData as route (route.id)}
												<th class="text-start p-3 font-semibold text-[var(--color-text)] align-top">
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
											<!-- §83 "typical processing time" column (median reported) -->
											<td class="p-3 font-medium text-[var(--color-muted)]">{t('visa_routes.processing_time', 'Processing time')}</td>
											{#each comparisonData as route (route.id)}
												<td class="p-3 text-[var(--color-text)]">
													{formatProcessingDays(route)}
													{#if route.processing_sample_count != null && route.processing_sample_count > 0}
														<span class="text-xs text-[var(--color-muted)]">({route.processing_sample_count} {t('visa_routes.reports', 'reports')})</span>
													{/if}
												</td>
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
															class="inline-flex items-center gap-1 text-[var(--color-primary-text)] hover:underline"
														>
															<ExternalLink class="rtl:-scale-x-100" size={12} />
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
