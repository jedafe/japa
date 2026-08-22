<script lang="ts">
	import { untrack, onMount } from 'svelte';
	import { replaceState } from '$app/navigation';
	import { api } from '$lib/api/client';
	import { buildSeoMeta } from '$lib/seo';
	import { t } from '$lib/i18n';
	import { page } from '$app/state';
	import Icon from '$lib/components/ui/Icon.svelte';
	import EmptyState from '$lib/components/ui/EmptyState.svelte';
	import LoadingSkeleton from '$lib/components/ui/LoadingSkeleton.svelte';
	import ToolDisabled from '$lib/components/ui/ToolDisabled.svelte';
	import DestinationSelector from '$lib/components/tools/DestinationSelector.svelte';
	import type { HousingGuideWithAi, GeoSpaceOption } from '$lib/types';

	/**
	 * Housing Guide tool — §96, ADR-088.
	 * Destination selector → list of housing guides as expandable cards.
	 * Each card shows city, accommodation type, and last-verified date.
	 * Expanding reveals search platforms, landlord requirements, rent ranges,
	 * scam categories, and tips.
	 */
	let { data } = $props();

	let destinations = $state<GeoSpaceOption[]>(untrack(() => data.destinations));
	let guides = $state<HousingGuideWithAi[]>(untrack(() => data.guides));
	let user = $state(untrack(() => data.user));

	let selectedDestId = $state<string>(untrack(() => data.initialDestId ?? ''));
	let loading = $state(false);
	let expandedId = $state<string | null>(null);

	const seo = $derived(
		buildSeoMeta({
			title: t('housing_guide.seo_title', 'Housing Guide — JapaConnect Tools'),
			description: t(
				'housing_guide.seo_description',
				'Destination-specific housing guides — platforms, landlord requirements, rent ranges, scam categories, and tips for finding safe housing abroad.'
			),
			path: '/housing-guide',
			origin: page.url.origin
		})
	);

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
		loadGuides();
		// Update URL for shareability (SvelteKit replaceState, not window.history)
		const url = new URL(page.url);
		if (id) url.searchParams.set('dest', id);
		else url.searchParams.delete('dest');
		replaceState(url, {});
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
</script>

<svelte:head>
	<title>{seo.title}</title>
	<meta name="description" content={seo.description} />
	<link rel="canonical" href={seo.canonical} />
	<meta property="og:title" content={seo.title} />
	<meta property="og:description" content={seo.description} />
	<meta property="og:url" content={seo.canonical} />
	<meta property="og:type" content="website" />
	<meta name="twitter:card" content="summary" />
	<meta name="twitter:title" content={seo.title} />
	<meta name="twitter:description" content={seo.description} />
</svelte:head>

<div class="mx-auto max-w-4xl px-4 py-8">
	{#if !data.enabled}
		<ToolDisabled tool={data.tool} />
	{:else}
	<div class="mb-8">
		<h1 class="text-2xl sm:text-3xl font-bold text-text">
			{t('housing_guide.title', 'Housing Guide')}
		</h1>
		<p class="mt-2 text-text-subtle">
			{t(
				'housing_guide.subtitle',
				'Find safe housing abroad — platforms, landlord requirements, rent ranges, and scam warnings.'
			)}
		</p>
	</div>

	{#if destinations.length === 0}
		<EmptyState
			icon="home"
			title={t('housing_guide.no_destinations', 'No destinations available')}
			description={t(
				'housing_guide.no_destinations_desc',
				"Housing guide data hasn't been configured yet. Please check back later."
			)}
		/>
	{:else}
		<!-- Destination selector (geo-aware pattern — first input) -->
		<div class="mb-6">
			<DestinationSelector
				{destinations}
				bind:selectedId={selectedDestId}
				onchange={onDestChange}
			/>
		</div>

		{#if loading}
			<div class="space-y-3">
				<LoadingSkeleton height="80px" />
				<LoadingSkeleton height="80px" />
				<LoadingSkeleton height="80px" />
			</div>
		{:else if !selectedDestId}
			<EmptyState
				icon="map-pin"
				title={t('housing_guide.select_destination', 'Select a destination')}
				description={t(
					'housing_guide.select_destination_desc',
					'Choose a country to see housing guides for that destination.'
				)}
			/>
		{:else if guides.length === 0}
			<EmptyState
				icon="home"
				title={t('housing_guide.no_guides', 'No housing guides')}
				description={t(
					'housing_guide.no_guides_desc',
					'No housing guides have been published for this destination yet.'
				)}
			/>
		{:else}
			<div class="space-y-3">
				{#each guides as guide (guide.id)}
					<div class="bg-surface border border-border rounded-lg overflow-hidden">
						<!-- Card header (click to expand) -->
						<button
							onclick={() => toggleExpand(guide.id)}
							class="w-full flex items-center justify-between p-4 text-left hover:bg-surface-subtle transition-colors"
							aria-expanded={expandedId === guide.id}
						>
							<div class="min-w-0">
								<div class="flex items-center gap-2 flex-wrap">
									{#if guide.city_space_name}
										<span class="text-sm font-semibold text-text">
											{guide.city_space_name}
										</span>
									{/if}
									{#if guide.accommodation_type}
										<span class="text-xs px-2 py-0.5 bg-[var(--tool-accent)]/10 text-[var(--tool-accent)] rounded-full font-medium">
											{guide.accommodation_type}
										</span>
									{/if}
								</div>
								{#if guide.last_verified_at}
									<p class="text-xs text-muted mt-1">
										{t('housing_guide.last_verified', 'Last verified:')}
										{formatDate(guide.last_verified_at)}
									</p>
								{/if}
							</div>
							<Icon
								name={expandedId === guide.id ? 'chevron-up' : 'chevron-down'}
								size={20}
								class="text-muted flex-shrink-0"
							/>
						</button>

						<!-- Expanded detail -->
						{#if expandedId === guide.id}
							<div class="border-t border-border p-4 space-y-4">
								<!-- Search platforms -->
								{#if guide.search_platforms}
									<div>
										<h3 class="text-sm font-semibold text-muted uppercase tracking-wide mb-2">
											{t('housing_guide.search_platforms', 'Search platforms')}
										</h3>
										{#if isPlatformArray(guide.search_platforms)}
											<ul class="space-y-1">
												{#each guide.search_platforms as platform (platform)}
													<li class="flex items-center gap-2 text-sm text-text">
														<Icon name="globe" size={14} class="text-muted flex-shrink-0" />
														{platform}
													</li>
												{/each}
											</ul>
										{:else}
											<dl class="space-y-1">
												{#each objectEntries(guide.search_platforms) as [key, val] (key)}
													<div class="flex items-start gap-2 text-sm">
														<dt class="font-medium text-text flex-shrink-0">{key}:</dt>
														<dd class="text-text-subtle">{formatValue(val)}</dd>
													</div>
												{/each}
											</dl>
										{/if}
									</div>
								{/if}

								<!-- Landlord requirements -->
								{#if guide.landlord_requirements}
									<div>
										<h3 class="text-sm font-semibold text-muted uppercase tracking-wide mb-2">
											{t('housing_guide.landlord_requirements', 'Landlord requirements')}
										</h3>
										<p class="text-sm text-text-subtle">{guide.landlord_requirements}</p>
									</div>
								{/if}

								<!-- Rent ranges -->
								{#if guide.rent_ranges}
									<div>
										<h3 class="text-sm font-semibold text-muted uppercase tracking-wide mb-2">
											{t('housing_guide.rent_ranges', 'Rent ranges')}
										</h3>
										<dl class="space-y-1">
											{#each objectEntries(guide.rent_ranges) as [key, val] (key)}
												<div class="flex items-start gap-2 text-sm">
													<dt class="font-medium text-text flex-shrink-0">{key}:</dt>
													<dd class="text-text-subtle">{formatValue(val)}</dd>
												</div>
											{/each}
										</dl>
									</div>
								{/if}

								<!-- Scam categories -->
								{#if guide.scam_categories && guide.scam_categories.length > 0}
									<div>
										<h3 class="text-sm font-semibold text-muted uppercase tracking-wide mb-2">
											{t('housing_guide.scam_categories', 'Scam categories')}
										</h3>
										<ul class="space-y-1">
											{#each guide.scam_categories as scam (scam)}
												<li class="flex items-start gap-2 text-sm text-text">
													<Icon name="alert-triangle" size={14} class="text-yellow-600 flex-shrink-0 mt-0.5" />
													{scam}
												</li>
											{/each}
										</ul>
									</div>
								{/if}

								<!-- Tips -->
								{#if guide.tips}
									<div>
										<h3 class="text-sm font-semibold text-muted uppercase tracking-wide mb-2">
											{t('housing_guide.tips', 'Tips')}
										</h3>
										<p class="text-sm text-text-subtle">{guide.tips}</p>
									</div>
								{/if}

								{#if guide.reviewer_name}
									<p class="text-xs text-muted pt-2 border-t border-border">
										{t('housing_guide.reviewed_by', 'Reviewed by')} {guide.reviewer_name}
									</p>
								{/if}

								<div class="pt-2">
									<a
										href={`/housing-guide/${guide.id}`}
										class="inline-flex items-center gap-1.5 text-sm text-[var(--tool-accent)] hover:underline"
									>
										<Icon name="external-link" size={16} />
										{t('housing_guide.view_guide', 'View guide')}
									</a>
								</div>
							</div>
						{/if}
					</div>
				{/each}
			</div>
		{/if}
	{/if}
	{/if}
</div>
