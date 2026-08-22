<script lang="ts">
	import { untrack, onMount } from 'svelte';
	import { t } from '$lib/i18n';
	import { page } from '$app/state';
	import { Home, MapPin, ChevronDown, ChevronUp, Globe, AlertTriangle, Lock } from 'lucide-svelte';
	import { EmptyState, LoadingSkeleton } from '$lib/components/ui';
	import { api } from '$lib/api/client';
	import type { HousingGuideWithAi, GeoCountry } from '$lib/types';
	import type { PageData } from './$types';
	import Autocomplete from '$lib/components/create/Autocomplete.svelte';
	import type { AutocompleteItem } from '$lib/components/create/Autocomplete.svelte';

	/**
	 * Housing Guide tool — §96, ADR-088.
	 * Destination selector → list of housing guides as expandable cards.
	 * Each card shows city, accommodation type, and last-verified date.
	 * Expanding reveals search platforms, landlord requirements, rent ranges,
	 * scam categories, and tips.
	 */
	let { data }: { data: PageData } = $props();

	const title = $derived(t('housing_guide.title', 'Housing Guide'));
	const description = $derived(
		t(
			'housing_guide.subtitle',
			'Find safe housing abroad — platforms, landlord requirements, rent ranges, and scam warnings.'
		)
	);

	let destinations = $state<GeoCountry[]>(untrack(() => data.destinations));
	let guides = $state<HousingGuideWithAi[]>(untrack(() => data.guides));

	let selectedDestId = $state<string>(untrack(() => data.initialDestId ?? ''));
	let loading = $state(false);
	let expandedId = $state<string | null>(null);

	const disabled = $derived(data.disabled);
	const authed = $derived(data.authed);

	async function loadGuides() {
		if (!selectedDestId) {
			guides = [];
			return;
		}
		loading = true;
		const res = await api.get<HousingGuideWithAi[]>(
			`/housing-guide?dest=${selectedDestId}`
		);
		loading = false;
		if (!res.error) guides = res.data ?? [];
		else guides = [];
	}

	function onDestChange(id: string) {
		expandedId = null;
		selectedDestId = id;
		// Update URL for shareability (history.replaceState, not SvelteKit replaceState)
		const url = new URL(page.url);
		if (id) url.searchParams.set('dest', id);
		else url.searchParams.delete('dest');
		history.replaceState({}, '', url);
		loadGuides();
	}

	function toggleExpand(id: string) {
		expandedId = expandedId === id ? null : id;
	}

	/** Format an ISO date string as a readable date. */
	function formatDate(iso?: string | null): string {
		if (!iso) return '';
		try {
			return new Date(iso).toLocaleDateString(undefined, {
				year: 'numeric',
				month: 'short',
				day: 'numeric'
			});
		} catch {
			return iso;
		}
	}

	/** Render search_platforms as a list if it's an array, or key-value if object. */
	function isPlatformArray(
		platforms: string[] | Record<string, unknown> | null | undefined
	): platforms is string[] {
		return Array.isArray(platforms);
	}

	/** Get key-value entries from a JSON object field (rent_ranges, search_platforms object). */
	function objectEntries(val: Record<string, unknown> | null | undefined): [string, unknown][] {
		if (!val || typeof val !== 'object') return [];
		return Object.entries(val);
	}

	/** Format a value for display — stringify objects/arrays, return strings as-is. */
	function formatValue(val: unknown): string {
		if (val === null || val === undefined) return '';
		if (typeof val === 'string') return val;
		if (typeof val === 'number' || typeof val === 'boolean') return String(val);
		try {
			return JSON.stringify(val);
		} catch {
			return String(val);
		}
	}

	// On mount: if SSR already loaded guides for initialDestId, no action needed.
	// Otherwise nothing to do — the user picks a destination.
	onMount(() => {
		// SSR handles initial load; nothing to fetch here.
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
	<div class="border-b border-[var(--color-border)] px-4 py-3">
		<h1 class="flex items-center gap-2 text-lg font-semibold text-[var(--color-text)]">
			<Home size={20} /> {title}
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
	{:else if destinations.length === 0}
		<div class="p-4">
			<EmptyState
				icon={Home}
				title={t('housing_guide.no_destinations', 'No destinations available')}
				body={t(
					'housing_guide.no_destinations_desc',
					"Housing guide data hasn't been configured yet. Please check back later."
				)}
			/>
		</div>
	{:else}
		<!-- Destination selector (geo-aware pattern — first input) -->
		<div class="border-b border-[var(--color-border)] px-4 py-3">
			<label for="hg-dest" class="block text-xs font-medium uppercase tracking-wide text-[var(--color-muted)]">
				{t('housing_guide.destination', 'Destination country')}
			</label>
			<div class="mt-2">
				<Autocomplete
					placeholder={t('housing_guide.select_destination', 'Select a destination country')}
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

		{#if !authed}
			<div class="mx-4 mt-3 mb-3 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface-subtle)] px-3 py-2 text-xs text-[var(--color-muted)]">
				{t('housing_guide.anon_hint', 'Sign in to save your progress.')}
			</div>
		{/if}

		{#if loading}
			<div class="p-4 space-y-3">
				<LoadingSkeleton variant="card" count={3} />
			</div>
		{:else if !selectedDestId}
			<div class="p-4">
				<EmptyState
					icon={MapPin}
					title={t('housing_guide.select_destination', 'Select a destination')}
					body={t(
						'housing_guide.select_destination_desc',
						'Choose a country to see housing guides for that destination.'
					)}
				/>
			</div>
		{:else if guides.length === 0}
			<div class="p-4">
				<EmptyState
					icon={Home}
					title={t('housing_guide.no_guides', 'No housing guides')}
					body={t(
						'housing_guide.no_guides_desc',
						'No housing guides have been published for this destination yet.'
					)}
				/>
			</div>
		{:else}
			<div class="p-4 space-y-3">
				{#each guides as guide (guide.id)}
					<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] overflow-hidden">
						<!-- Card header (click to expand) -->
						<button
							type="button"
							onclick={() => toggleExpand(guide.id)}
							class="w-full flex items-center justify-between p-4 text-left hover:bg-[var(--color-surface-subtle)] transition-colors"
							aria-expanded={expandedId === guide.id}
						>
							<div class="min-w-0">
								<div class="flex items-center gap-2 flex-wrap">
									{#if guide.city_space_name}
										<span class="text-sm font-semibold text-[var(--color-text)]">
											{guide.city_space_name}
										</span>
									{/if}
									{#if guide.accommodation_type}
										<span class="text-xs px-2 py-0.5 bg-[var(--color-primary)]/10 text-[var(--color-primary)] rounded-full font-medium">
											{guide.accommodation_type}
										</span>
									{/if}
								</div>
								{#if guide.last_verified_at}
									<p class="text-xs text-[var(--color-muted)] mt-1">
										{t('housing_guide.last_verified', 'Last verified:')}
										{formatDate(guide.last_verified_at)}
									</p>
								{/if}
							</div>
							{#if expandedId === guide.id}
								<ChevronUp size={20} class="text-[var(--color-muted)] flex-shrink-0" />
							{:else}
								<ChevronDown size={20} class="text-[var(--color-muted)] flex-shrink-0" />
							{/if}
						</button>

						<!-- Expanded detail -->
						{#if expandedId === guide.id}
							<div class="border-t border-[var(--color-border)] p-4 space-y-4">
								<!-- Search platforms -->
								{#if guide.search_platforms}
									<div>
										<h3 class="text-sm font-semibold text-[var(--color-muted)] uppercase tracking-wide mb-2">
											{t('housing_guide.search_platforms', 'Search platforms')}
										</h3>
										{#if isPlatformArray(guide.search_platforms)}
											<ul class="space-y-1">
												{#each guide.search_platforms as platform (platform)}
													<li class="flex items-center gap-2 text-sm text-[var(--color-text)]">
														<Globe size={14} class="text-[var(--color-muted)] flex-shrink-0" />
														{platform}
													</li>
												{/each}
											</ul>
										{:else}
											<dl class="space-y-1">
												{#each objectEntries(guide.search_platforms) as [key, val] (key)}
													<div class="flex items-start gap-2 text-sm">
														<dt class="font-medium text-[var(--color-text)] flex-shrink-0">{key}:</dt>
														<dd class="text-[var(--color-text-subtle)]">{formatValue(val)}</dd>
													</div>
												{/each}
											</dl>
										{/if}
									</div>
								{/if}

								<!-- Landlord requirements -->
								{#if guide.landlord_requirements}
									<div>
										<h3 class="text-sm font-semibold text-[var(--color-muted)] uppercase tracking-wide mb-2">
											{t('housing_guide.landlord_requirements', 'Landlord requirements')}
										</h3>
										<p class="text-sm text-[var(--color-text-subtle)]">{guide.landlord_requirements}</p>
									</div>
								{/if}

								<!-- Rent ranges -->
								{#if guide.rent_ranges}
									<div>
										<h3 class="text-sm font-semibold text-[var(--color-muted)] uppercase tracking-wide mb-2">
											{t('housing_guide.rent_ranges', 'Rent ranges')}
										</h3>
										<dl class="space-y-1">
											{#each objectEntries(guide.rent_ranges) as [key, val] (key)}
												<div class="flex items-start gap-2 text-sm">
													<dt class="font-medium text-[var(--color-text)] flex-shrink-0">{key}:</dt>
													<dd class="text-[var(--color-text-subtle)]">{formatValue(val)}</dd>
												</div>
											{/each}
										</dl>
									</div>
								{/if}

								<!-- Scam categories -->
								{#if guide.scam_categories && guide.scam_categories.length > 0}
									<div>
										<h3 class="text-sm font-semibold text-[var(--color-muted)] uppercase tracking-wide mb-2">
											{t('housing_guide.scam_categories', 'Scam categories')}
										</h3>
										<ul class="space-y-1">
											{#each guide.scam_categories as scam (scam)}
												<li class="flex items-start gap-2 text-sm text-[var(--color-text)]">
													<AlertTriangle size={14} class="text-yellow-600 flex-shrink-0 mt-0.5" />
													{scam}
												</li>
											{/each}
										</ul>
									</div>
								{/if}

								<!-- Tips -->
								{#if guide.tips}
									<div>
										<h3 class="text-sm font-semibold text-[var(--color-muted)] uppercase tracking-wide mb-2">
											{t('housing_guide.tips', 'Tips')}
										</h3>
										<p class="text-sm text-[var(--color-text-subtle)]">{guide.tips}</p>
									</div>
								{/if}

								{#if guide.reviewer_name}
									<p class="text-xs text-[var(--color-muted)] pt-2 border-t border-[var(--color-border)]">
										{t('housing_guide.reviewed_by', 'Reviewed by')} {guide.reviewer_name}
									</p>
								{/if}
								<a
									href="/tools/housing-guide/{guide.id}"
									class="inline-flex items-center gap-1.5 pt-2 text-sm text-[var(--color-primary)] hover:underline"
								>
									{t('common.view_details', 'View details')}
								</a>
							</div>
						{/if}
					</div>
				{/each}
			</div>
		{/if}
	{/if}
</div>
