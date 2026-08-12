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
	let taxConsultants = $state<Array<{ id: string; name: string; slug?: string; description?: string }>>([]);
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
		taxConsultants = [];

		// Build query params for tax guides
		const params = new URLSearchParams();
		params.set('dest', selectedDestId);
		params.set('origin', selectedOriginId);
		if (employmentStatus) params.set('employment', employmentStatus);

		// Fetch tax guides, treaty info, and service directory tax consultants
		// (§100: result page surfaces verified tax consultants from §98
		// filtered by service_category='finance' and destination_country_id).
		const [guidesRes, treatyRes, consultantsRes] = await Promise.all([
			api.get<TaxGuideWithAi[]>(`/tax-calculator?${params.toString()}`),
			api.get<TaxTreaty>(
				`/tax-calculator/treaty?dest=${selectedDestId}&origin=${selectedOriginId}`
			),
			api.get<Array<{ id: string; name: string; slug?: string; description?: string }>>(
				`/service-directory?dest=${selectedDestId}&category=finance&limit=5`
			)
		]);

		loading = false;
		if (!guidesRes.error) guides = guidesRes.data ?? [];
		else guides = [];
		if (!treatyRes.error) treaty = treatyRes.data;
		else treaty = null;
		if (!consultantsRes.error) taxConsultants = consultantsRes.data ?? [];
		else taxConsultants = [];
	}

	function toggleExpand(id: string) {
		expandedId = expandedId === id ? null : id;
	}

	/** Render action_checklist items — each item is {action, deadline_description, official_url}. */
	function checklistItems(
		val: unknown[] | Record<string, unknown> | null | undefined
	): Array<{ action: string; deadline?: string; url?: string }> {
		if (!Array.isArray(val)) return [];
		return val.map((v) => {
			if (typeof v === 'string') return { action: v };
			if (v && typeof v === 'object') {
				const obj = v as Record<string, unknown>;
				return {
					action: String(obj.action ?? obj.title ?? ''),
					deadline: obj.deadline_description ? String(obj.deadline_description) : undefined,
					url: obj.official_url ? String(obj.official_url) : undefined
				};
			}
			return { action: String(v) };
		}).filter((item) => item.action.length > 0);
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

			<!-- Employment status (select — backend validates enum) -->
			<div class="flex items-center gap-3">
				<label for="employment" class="text-sm font-medium text-text whitespace-nowrap">
					{t('tax_calculator.employment_status', 'Employment status:')}
				</label>
				<select
					id="employment"
					value={employmentStatus}
					onchange={(e) => (employmentStatus = (e.currentTarget as HTMLSelectElement).value)}
					class="flex-1 px-4 py-2 bg-surface border border-border rounded-lg text-text focus:border-primary focus:outline-none"
				>
					<option value="">{t('tax_calculator.employment_any', 'Any (optional)')}</option>
					<option value="employed">{t('tax_calculator.employed', 'Employed')}</option>
					<option value="self_employed">{t('tax_calculator.self_employed', 'Self-employed')}</option>
					<option value="contractor">{t('tax_calculator.contractor', 'Contractor')}</option>
					<option value="unemployed">{t('tax_calculator.unemployed', 'Unemployed')}</option>
				</select>
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
																	<span>
																		{item.action}
																		{#if item.deadline}
																			<span class="text-muted">— {item.deadline}</span>
																		{/if}
																		{#if item.url}
																			<a href={item.url} target="_blank" rel="noopener noreferrer" class="ml-1 text-primary hover:underline inline-flex items-center gap-0.5">
																				<Icon name="external-link" size={12} />
																			</a>
																		{/if}
																	</span>
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

				<!-- Service Directory: verified tax consultants (§100 Integration) -->
				{#if taxConsultants.length > 0}
					<div>
						<h2 class="text-sm font-semibold text-muted uppercase tracking-wide mb-3">
							{t('tax_calculator.tax_consultants', 'Verified tax consultants')}
						</h2>
						<div class="space-y-2">
							{#each taxConsultants as consultant (consultant.id)}
								<a
									href={`/service-directory/${consultant.slug ?? consultant.id}`}
									class="block p-3 bg-surface border border-border rounded-lg hover:bg-surface-subtle transition-colors"
								>
									<p class="text-sm font-medium text-text">{consultant.name}</p>
									{#if consultant.description}
										<p class="text-xs text-muted mt-1 line-clamp-2">{consultant.description}</p>
									{/if}
								</a>
							{/each}
						</div>
					</div>
				{/if}

				<!-- HITL disclosure (§100/ADR-108: mandatory on every output page) -->
				<div class="mt-6 p-3 bg-surface-subtle border border-border rounded-lg text-xs text-muted">
					{t(
						'tax_calculator.hitl_disclosure',
						'All tax content on this page has been reviewed by a qualified tax professional before publication. This tool provides general guidance only — consult a qualified tax advisor for your specific situation.'
					)}
				</div>
			</div>
		{/if}
	{/if}
	{/if}
</div>
