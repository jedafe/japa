<script lang="ts">
	import SeoHead from '#lib/seo/SeoHead.svelte';
	import { toolOgCard } from '#lib/seo/og.js';
	import { untrack, onMount } from 'svelte';
	import { t, i18n } from '#lib/i18n/index.js';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';

	import {
		Home,
		MapPin,
		ChevronDown,
		ChevronUp,
		Globe,
		AlertTriangle,
		Lock
	} from 'lucide-svelte';

	import { EmptyState, LoadingSkeleton } from '#lib/components/ui/index.js';
	import { api } from '#lib/api/client.js';
	import {
		housingPlatforms,
		housingRentTiers,
		rentTierLabel,
		housingTypeLabel,
		isStaleVerification,
		HOUSING_ACCOMMODATION_TYPES
	} from '#lib/utils/housing.js';
	import type { HousingGuideWithAi, GeoCountry } from '#lib/types/index.js';
	import type { PageData } from './$types';
	import Autocomplete from '#lib/components/create/Autocomplete.svelte';
	import type { AutocompleteItem } from '#lib/components/create/Autocomplete.svelte';
	import ToolHeader from '#lib/components/tools/ToolHeader.svelte';

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
	let selectedType = $state<string>(untrack(() => data.initialType ?? ''));
	let loading = $state(false);
	let expandedId = $state<string | null>(null);

	const disabled = $derived(data.disabled);
	const authed = $derived(data.authed);

	// §96 accommodation_type enum — shared with the detail page and the
	// Studio field options via HOUSING_ACCOMMODATION_TYPES.
	const accommodationTypes = HOUSING_ACCOMMODATION_TYPES;

	async function loadGuides() {
		if (!selectedDestId) {
			guides = [];
			return;
		}
		loading = true;
		const qs = new URLSearchParams({ dest: selectedDestId });
		if (selectedType) qs.set('type', selectedType);
		const res = await api.get<HousingGuideWithAi[]>(`/housing-guide?${qs}`);
		loading = false;
		if (!res.error) guides = res.data ?? [];
		else guides = [];
	}

	function syncUrl() {
		const url = new URL(page.url.href);
		if (selectedDestId) url.searchParams.set('dest', selectedDestId); else url.searchParams.delete('dest');
		if (selectedType) url.searchParams.set('type', selectedType); else url.searchParams.delete('type');
		goto(url, { shallow: true, replace: true });
	}

	function onDestChange(id: string) {
		expandedId = null;
		selectedDestId = id;
		// Update URL for shareability (shallow — SvelteKit replaceState keeps page.url in sync)
		syncUrl();
		loadGuides();
	}

	function onTypeChange(e: Event) {
		expandedId = null;
		selectedType = (e.target as HTMLSelectElement).value;
		syncUrl();
		loadGuides();
	}

	function toggleExpand(id: string) {
		expandedId = expandedId === id ? null : id;
	}

	/** Format an ISO date string as a readable date. */
	function formatDate(iso?: string | null): string {
		if (!iso) return '';
		try {
			return new Date(iso).toLocaleDateString(i18n.locale, {
				year: 'numeric',
				month: 'short',
				day: 'numeric'
			});
		} catch {
			return iso;
		}
	}

	/** ADR-104 staleness — row not re-verified within 90 days. */
	function isStale(guide: HousingGuideWithAi): boolean {
		return isStaleVerification(guide.last_verified_at);
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

<SeoHead title={title} description={description} ogTitle={title}
	ogImage={toolOgCard(page.url.origin, page.url.pathname)}
/>

<div class="mx-auto w-full max-w-3xl">
	<div class="px-4 pt-4">
		<ToolHeader icon={Home} title={title} description={description} />
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
			<label for="hg-type" class="mt-3 block text-xs font-medium uppercase tracking-wide text-[var(--color-muted)]">
				{t('housing_guide.accommodation_type', 'Accommodation type')}
			</label>
			<select
				id="hg-type"
				value={selectedType}
				onchange={onTypeChange}
				class="mt-2 w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)]"
			>
				<option value="">{t('housing_guide.any_type', 'Any type')}</option>
				{#each accommodationTypes as at (at.value)}
					<option value={at.value}>{t(at.labelKey, at.label)}</option>
				{/each}
			</select>
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
							class="w-full flex items-center justify-between p-4 text-start hover:bg-[var(--color-surface-subtle)] transition-colors"
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
										<span class="text-xs px-2 py-0.5 bg-[var(--color-primary)]/10 text-[var(--color-primary-text)] rounded-full font-medium">
											{housingTypeLabel(guide.accommodation_type, t)}
										</span>
									{/if}
								</div>
								{#if guide.last_verified_at}
									<p class="text-xs text-[var(--color-muted)] mt-1">
										{t('housing_guide.last_verified', 'Last verified:')}
										{formatDate(guide.last_verified_at)}
									</p>
									{#if isStale(guide)}
										<p class="mt-1 inline-flex items-center gap-1 rounded-full bg-yellow-500/15 px-2 py-0.5 text-xs font-medium text-yellow-700 dark:text-yellow-400">
											<AlertTriangle size={12} />
											{t('housing_guide.stale_warning', 'May be outdated — not verified in 90+ days')}
										</p>
									{/if}
								{:else}
									<p class="text-xs text-[var(--color-muted)] mt-1">
										{t('housing_guide.not_verified', 'Not yet verified')}
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
								<!-- Search platforms — §96 [{name, url, description}] -->
								{#if guide.search_platforms}
									{@const platforms = housingPlatforms(guide.search_platforms)}
								{#if platforms.length > 0}
									<div>
										<h3 class="text-sm font-semibold text-[var(--color-muted)] uppercase tracking-wide mb-2">
											{t('housing_guide.search_platforms', 'Search platforms')}
										</h3>
										<ul class="space-y-2">
											{#each platforms as platform (platform.name + (platform.url ?? ''))}
												<li class="text-sm">
													<span class="flex items-center gap-2 text-[var(--color-text)]">
														<Globe size={14} class="text-[var(--color-muted)] flex-shrink-0" />
														{#if platform.url}
															<a
																href={platform.url}
																target="_blank"
																rel="noopener noreferrer"
																class="font-medium text-[var(--color-primary-text)] hover:underline"
															>
																{platform.name}
															</a>
														{:else}
															<span class="font-medium">{platform.name}</span>
														{/if}
													</span>
													{#if platform.description}
														<p class="ms-6 text-xs text-[var(--color-muted)]">{platform.description}</p>
													{/if}
												</li>
											{/each}
										</ul>
									</div>
								{/if}
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

								<!-- Rent ranges — §96 {tier: {studio, 1bed, 2bed, currency}} -->
								{#if guide.rent_ranges}
									{@const rentTiers = housingRentTiers(guide.rent_ranges)}
								{#if rentTiers.length > 0}
									<div>
										<h3 class="text-sm font-semibold text-[var(--color-muted)] uppercase tracking-wide mb-2">
											{t('housing_guide.rent_ranges', 'Rent ranges')}
										</h3>
										<div class="space-y-2">
											{#each rentTiers as row (row.tier)}
												<div class="text-sm">
													<p class="font-medium text-[var(--color-text)] capitalize">
														{rentTierLabel(row.tier)}
														{#if row.currency}
															<span class="ms-1 text-xs font-normal text-[var(--color-muted)]">({row.currency})</span>
														{/if}
													</p>
													{#if row.raw}
														<p class="text-[var(--color-text-subtle)]">{row.raw}</p>
													{:else}
														<dl class="space-y-0.5">
															{#if row.studio}
																<div class="flex gap-2"><dt class="text-[var(--color-muted)]">{t('housing_guide.unit_studio', 'Studio')}:</dt><dd class="text-[var(--color-text-subtle)]">{row.studio}</dd></div>
															{/if}
															{#if row.oneBed}
																<div class="flex gap-2"><dt class="text-[var(--color-muted)]">{t('housing_guide.unit_1bed', '1-bed')}:</dt><dd class="text-[var(--color-text-subtle)]">{row.oneBed}</dd></div>
															{/if}
															{#if row.twoBed}
																<div class="flex gap-2"><dt class="text-[var(--color-muted)]">{t('housing_guide.unit_2bed', '2-bed')}:</dt><dd class="text-[var(--color-text-subtle)]">{row.twoBed}</dd></div>
															{/if}
														</dl>
													{/if}
												</div>
											{/each}
										</div>
									</div>
								{/if}
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
										{#if page.data.features?.['scam_alert.enabled']}
											<a
												href="/scam-alerts?scam_type=housing_fraud&dest={guide.destination_space_id}"
												class="mt-2 inline-flex items-center gap-1 text-xs font-medium text-[var(--color-primary-text)] hover:underline"
											>
												{t('scam_alerts.see_alerts', 'See housing scam alerts for this destination')} →
											</a>
										{/if}
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
								<div class="flex flex-wrap items-center gap-4 pt-2">
									<a
										href="/tools/housing-guide/{guide.id}"
										class="inline-flex items-center gap-1.5 text-sm text-[var(--color-primary-text)] hover:underline"
									>
										{t('common.view_details', 'View details')}
									</a>
									<!-- §96 editorial cross-link — the in-depth guide that
									     claims this row (guide_slug on the detail DTO). -->
									{#if guide.guide_slug}
										<a
											href="/guide/{guide.guide_slug}"
											class="inline-flex items-center gap-1.5 text-sm text-[var(--color-primary-text)] hover:underline"
										>
											{t('housing_guide.read_guide', 'Read the in-depth guide')}
										</a>
									{/if}
								</div>
							</div>
						{/if}
					</div>
				{/each}
			</div>
		{/if}
	{/if}
</div>
