<script lang="ts">
	import { untrack } from 'svelte';
	import { api } from '$lib/api/client';
	import { buildSeoMeta } from '$lib/seo';
	import { t } from '$lib/i18n';
	import { page } from '$app/state';
	import Icon from '$lib/components/ui/Icon.svelte';
	import EmptyState from '$lib/components/ui/EmptyState.svelte';
	import LoadingSkeleton from '$lib/components/ui/LoadingSkeleton.svelte';
	import ToolDisabled from '$lib/components/ui/ToolDisabled.svelte';
	import type { TaxGuideWithAi, TaxTreaty, GeoSpaceOption } from '$lib/types';

	/**
	 * Tax Calculator tool — §100, ADR-088.
	 * Two destination selectors (origin + destination) + optional employment
	 * status text input. "Search" button fetches tax guides and treaty info.
	 * Guides are displayed as cards; treaty info is shown in a separate card.
	 * No initial data — requires both origin + destination selection (Rule 16).
	 */
	let { data } = $props();

	let destinations = $state<GeoSpaceOption[]>(untrack(() => data.destinations));
	let user = $state(untrack(() => data.user));

	let selectedOriginId = $state<string>('');
	let selectedDestId = $state<string>('');
	let employmentStatus = $state<string>('');

	let guides = $state<TaxGuideWithAi[]>([]);
	let treaty = $state<TaxTreaty | null>(null);
	let loading = $state(false);
	let hasSearched = $state(false);
	let expandedId = $state<string | null>(null);

	const seo = $derived(
		buildSeoMeta({
			title: t('tax_calculator.seo_title', 'Tax Calculator — JapaConnect Tools'),
			description: t(
				'tax_calculator.seo_description',
				'Destination-specific tax guides — residency triggers, worldwide income rules, double taxation treaties, and checklists.'
			),
			path: '/tax-calculator',
			origin: page.url.origin
		})
	);

	const canSearch = $derived(Boolean(selectedOriginId && selectedDestId));

	async function search() {
		if (!canSearch) return;
		loading = true;
		hasSearched = true;
		expandedId = null;
		guides = [];
		treaty = null;

		// Build query params for tax guides
		const params = new URLSearchParams();
		params.set('dest', selectedDestId);
		params.set('origin', selectedOriginId);
		if (employmentStatus) params.set('employment', employmentStatus);

		// Fetch tax guides and treaty info in parallel
		const [guidesRes, treatyRes] = await Promise.all([
			api.get<TaxGuideWithAi[]>(`/tax-calculator?${params.toString()}`),
			api.get<TaxTreaty>(
				`/tax-calculator/treaty?dest=${selectedDestId}&origin=${selectedOriginId}`
			)
		]);

		loading = false;
		if (!guidesRes.error) guides = guidesRes.data ?? [];
		else guides = [];
		if (!treatyRes.error) treaty = treatyRes.data;
		else treaty = null;
	}

	function toggleExpand(id: string) {
		expandedId = expandedId === id ? null : id;
	}

	/** Render action_checklist as a list if it's an array. */
	function checklistItems(
		val: unknown[] | Record<string, unknown> | null | undefined
	): string[] {
		if (Array.isArray(val)) return val.map((v) => String(v));
		return [];
	}

	function onOriginChange(e: Event) {
		selectedOriginId = (e.currentTarget as HTMLSelectElement).value;
	}

	function onDestChange(e: Event) {
		selectedDestId = (e.currentTarget as HTMLSelectElement).value;
	}
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
			{t('tax_calculator.title', 'Tax Calculator')}
		</h1>
		<p class="mt-2 text-text-subtle">
			{t(
				'tax_calculator.subtitle',
				'Understand your tax obligations — residency triggers, worldwide income rules, double taxation treaties, and action checklists.'
			)}
		</p>
	</div>

	{#if destinations.length === 0}
		<EmptyState
			icon="receipt"
			title={t('tax_calculator.no_destinations', 'No destinations available')}
			description={t(
				'tax_calculator.no_destinations_desc',
				"Tax guide data hasn't been configured yet. Please check back later."
			)}
		/>
	{:else}
		<!-- Origin + Destination selectors + employment status -->
		<div class="mb-6 space-y-4">
			<div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
				<!-- Origin selector -->
				<div class="flex items-center gap-3">
					<label for="origin-selector" class="text-sm font-medium text-text whitespace-nowrap">
						{t('tax_calculator.origin', 'Origin:')}
					</label>
					<select
						id="origin-selector"
						value={selectedOriginId}
						onchange={onOriginChange}
						class="flex-1 px-4 py-2 bg-surface border border-border rounded-lg text-text focus:border-primary focus:outline-none"
					>
						<option value="">{t('tax_calculator.select_origin', 'Select origin country…')}</option>
						{#each destinations as dest (dest.id)}
							<option value={dest.id}>{dest.name}</option>
						{/each}
					</select>
				</div>

				<!-- Destination selector -->
				<div class="flex items-center gap-3">
					<label for="dest-selector" class="text-sm font-medium text-text whitespace-nowrap">
						{t('tax_calculator.destination', 'Destination:')}
					</label>
					<select
						id="dest-selector"
						value={selectedDestId}
						onchange={onDestChange}
						class="flex-1 px-4 py-2 bg-surface border border-border rounded-lg text-text focus:border-primary focus:outline-none"
					>
						<option value="">{t('geo.select_destination', 'Select a country…')}</option>
						{#each destinations as dest (dest.id)}
							<option value={dest.id}>{dest.name}</option>
						{/each}
					</select>
				</div>
			</div>

			<!-- Employment status (text input — no endpoint for employment types) -->
			<div class="flex items-center gap-3">
				<label for="employment" class="text-sm font-medium text-text whitespace-nowrap">
					{t('tax_calculator.employment_status', 'Employment status:')}
				</label>
				<input
					id="employment"
					type="text"
					bind:value={employmentStatus}
					placeholder={t('tax_calculator.employment_placeholder', 'e.g. employed, self-employed, student (optional)')}
					class="flex-1 px-4 py-2 bg-surface border border-border rounded-lg text-text focus:border-primary focus:outline-none"
				/>
				<button
					onclick={search}
					disabled={!canSearch || loading}
					class="px-5 py-2 bg-[var(--tool-accent)] text-white rounded-lg text-sm font-medium hover:bg-[var(--tool-accent-hover)] transition-colors disabled:opacity-50"
				>
					{loading
						? t('common.searching', 'Searching...')
						: t('tax_calculator.search', 'Search')}
				</button>
			</div>
		</div>

		{#if loading}
			<div class="space-y-3">
				<LoadingSkeleton height="100px" />
				<LoadingSkeleton height="100px" />
				<LoadingSkeleton height="80px" />
			</div>
		{:else if !hasSearched}
			<EmptyState
				icon="arrow-right-left"
				title={t('tax_calculator.select_pair', 'Select origin and destination')}
				description={t(
					'tax_calculator.select_pair_desc',
					'Choose your origin and destination countries to see tax guides and treaty information.'
				)}
			/>
		{:else if guides.length === 0 && !treaty}
			<EmptyState
				icon="receipt"
				title={t('tax_calculator.no_guides', 'No tax guides')}
				description={t(
					'tax_calculator.no_guides_desc',
					'No tax guides or treaty information have been published for this origin→destination pair yet.'
				)}
			/>
		{:else}
			<div class="space-y-6">
				<!-- Tax guides -->
				{#if guides.length > 0}
					<div>
						<h2 class="text-sm font-semibold text-muted uppercase tracking-wide mb-3">
							{t('tax_calculator.tax_guides', 'Tax guides')}
						</h2>
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
												<span class="text-sm font-semibold text-text">
													{guide.employment_status}
												</span>
												{#if guide.worldwide_income_taxed === true}
													<span class="text-xs px-2 py-0.5 bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-400 rounded-full font-medium">
														{t('tax_calculator.worldwide_taxed', 'Worldwide income taxed')}
													</span>
												{:else if guide.worldwide_income_taxed === false}
													<span class="text-xs px-2 py-0.5 bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-400 rounded-full font-medium">
														{t('tax_calculator.worldwide_not_taxed', 'Worldwide income not taxed')}
													</span>
												{/if}
											</div>
											<p class="text-xs text-muted mt-1">
												{guide.origin_space_name ?? t('tax_calculator.unknown', 'Unknown')}
												→
												{guide.destination_space_name ?? t('tax_calculator.unknown', 'Unknown')}
											</p>
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
											{#if guide.residency_trigger}
												<div>
													<h3 class="text-sm font-semibold text-muted uppercase tracking-wide mb-1">
														{t('tax_calculator.residency_trigger', 'Residency trigger')}
													</h3>
													<p class="text-sm text-text-subtle">{guide.residency_trigger}</p>
												</div>
											{/if}

											{#if guide.first_year_obligations}
												<div>
													<h3 class="text-sm font-semibold text-muted uppercase tracking-wide mb-1">
														{t('tax_calculator.first_year_obligations', 'First year obligations')}
													</h3>
													<p class="text-sm text-text-subtle">{guide.first_year_obligations}</p>
												</div>
											{/if}

											{#if guide.action_checklist}
												{@const items = checklistItems(guide.action_checklist)}
												{#if items.length > 0}
													<div>
														<h3 class="text-sm font-semibold text-muted uppercase tracking-wide mb-2">
															{t('tax_calculator.action_checklist', 'Action checklist')}
														</h3>
														<ul class="space-y-1">
															{#each items as item, i (i)}
																<li class="flex items-start gap-2 text-sm text-text">
																	<Icon name="check-circle" size={14} class="text-[var(--tool-accent)] flex-shrink-0 mt-0.5" />
																	{item}
																</li>
															{/each}
														</ul>
													</div>
												{/if}
											{/if}
										</div>
									{/if}
								</div>
							{/each}
						</div>
					</div>
				{/if}

				<!-- Treaty info -->
				{#if treaty}
					<div>
						<h2 class="text-sm font-semibold text-muted uppercase tracking-wide mb-3">
							{t('tax_calculator.treaty_info', 'Double taxation treaty')}
						</h2>
						<div class="p-4 bg-surface border border-border rounded-lg">
							<div class="flex items-center gap-2 mb-3">
								{#if treaty.treaty_exists}
									<span class="text-xs px-2 py-0.5 bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-400 rounded-full font-medium">
										{t('tax_calculator.treaty_exists', 'Treaty exists')}
									</span>
								{:else}
									<span class="text-xs px-2 py-0.5 bg-yellow-100 text-yellow-700 dark:bg-yellow-900/30 dark:text-yellow-400 rounded-full font-medium">
										{t('tax_calculator.no_treaty', 'No treaty')}
									</span>
								{/if}
								<span class="text-xs text-muted">
									{treaty.origin_space_name ?? t('tax_calculator.unknown', 'Unknown')}
									↔
									{treaty.destination_space_name ?? t('tax_calculator.unknown', 'Unknown')}
								</span>
							</div>
							{#if treaty.treaty_summary}
								<p class="text-sm text-text-subtle">{treaty.treaty_summary}</p>
							{/if}
							{#if treaty.treaty_source_url}
								<a
									href={treaty.treaty_source_url}
									target="_blank"
									rel="noopener noreferrer"
									class="mt-3 inline-flex items-center gap-1.5 text-sm text-primary hover:underline"
								>
									<Icon name="external-link" size={14} />
									{t('tax_calculator.treaty_source', 'Treaty source')}
								</a>
							{/if}
						</div>
					</div>
				{/if}
			</div>
		{/if}
	{/if}
	{/if}
</div>
