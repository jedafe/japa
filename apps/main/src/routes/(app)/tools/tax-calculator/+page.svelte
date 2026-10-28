<script lang="ts">
	import HintIcon from '$lib/components/ui/HintIcon.svelte';
	import { untrack } from 'svelte';
	import { t } from '$lib/i18n';
	import { page } from '$app/state';
	import {
		Receipt,
		ArrowRightLeft,
		ChevronUp,
		ChevronDown,
		CheckCircle2,
		ExternalLink,
		Lock
	} from 'lucide-svelte';
	import { EmptyState, LoadingSkeleton } from '$lib/components/ui';
	import { api } from '$lib/api/client';
	import type { TaxGuideWithAi, TaxTreaty, GeoCountry, CatalogOption } from '$lib/types';
	import type { PageData } from './$types';
	import Autocomplete from '$lib/components/create/Autocomplete.svelte';
	import AutocompleteSelect from '$lib/components/ui/AutocompleteSelect.svelte';
	import type { AutocompleteItem } from '$lib/components/create/Autocomplete.svelte';

	/**
	 * Tax Calculator tool — §100, ADR-088.
	 * Two destination selectors (origin + destination) + optional employment
	 * status text input. "Search" button fetches tax guides and treaty info.
	 * Guides are displayed as cards; treaty info is shown in a separate card.
	 * No initial data — requires both origin + destination selection (Rule 16).
	 */
	let { data }: { data: PageData } = $props();

	let destinations = $state<GeoCountry[]>(untrack(() => data.destinations));

	let selectedOriginId = $state<string>('');
	let selectedDestId = $state<string>('');
	let employmentStatus = $state<string>('');
	const employmentOptions = $derived<CatalogOption[]>(data.employmentOptions ?? []);

	let guides = $state<TaxGuideWithAi[]>([]);
	let treaty = $state<TaxTreaty | null>(null);
	let taxConsultants = $state<Array<{ id: string; name: string; slug?: string; description?: string }>>([]);
	let loading = $state(false);
	let hasSearched = $state(false);
	let expandedId = $state<string | null>(null);

	const title = $derived(t('tax_calculator.title', 'Tax Calculator'));
	const description = $derived(
		t(
			'tax_calculator.subtitle',
			'Understand your tax obligations — residency triggers, worldwide income rules, double taxation treaties, and action checklists.'
		)
	);
	const disabled = $derived(data.disabled);
	const authed = $derived(data.authed);

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
		// Backend route is /services (api-routes-catalog.md L2123).
		// §100: service directory integration uses service_category='finance'.
		const [guidesRes, treatyRes, consultantsRes] = await Promise.all([
			api.get<TaxGuideWithAi[]>(`/tax-calculator?${params.toString()}`),
			api.get<TaxTreaty>(
				`/tax-calculator/treaty?dest=${selectedDestId}&origin=${selectedOriginId}`
			),
			api.get<Array<{ id: string; name: string; slug?: string; description?: string }>>(
				`/services?dest=${selectedDestId}&category=finance`
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
		selectedOriginId ? destinations.find((d) => d.id === selectedOriginId)?.name ?? null : null
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
			<Receipt size={20} /> {title}
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
				icon={Receipt}
				title={t('tax_calculator.no_destinations', 'No destinations available')}
				body={t(
					'tax_calculator.no_destinations_desc',
					"Tax guide data hasn't been configured yet. Please check back later."
				)}
			/>
		</div>
	{:else}
		{#if !authed}
			<div class="mb-3 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface-subtle)] px-3 py-2 text-xs text-[var(--color-muted)]">
				{t('tax_calculator.anon_hint', 'Sign in to save your progress.')}
			</div>
		{/if}

		<!-- Destination + Origin selectors + employment status -->
		<!-- Destination selector is first per the geo-aware tool pattern
		     (geo-aware-tool-pattern.md: "destination country selector is
		     always the first input"). -->
		<div class="border-b border-[var(--color-border)] px-4 py-3 space-y-4">
			<div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
				<!-- Destination selector (first — geo-aware pattern) -->
				<div>
					<label for="tc-dest" class="block text-xs font-medium uppercase tracking-wide text-[var(--color-muted)]">
						{t('tax_calculator.destination', 'Destination:')}
					
					<HintIcon text={t('tools.tax.dest_hint', 'The country you will pay tax in.')}  />
</label>
					<div class="mt-2">
						<Autocomplete
							placeholder={t('geo.select_destination', 'Select a country…')}
							emptyMessage={t('common.no_results', 'No results found')}
							fetcher={fetchDestinations}
							initialFetcher={fetchDestinationsInitial}
							selectedId={selectedDestId || null}
							selectedLabel={selectedDestLabel}
							onSelect={(item) => (selectedDestId = item.id)}
							onClear={() => (selectedDestId = '')}
						/>
					</div>
				</div>

				<!-- Origin selector (secondary — geo-aware pattern) -->
				<div>
					<label for="tc-origin" class="block text-xs font-medium uppercase tracking-wide text-[var(--color-muted)]">
						{t('tax_calculator.origin', 'Origin:')}
					
					<HintIcon text={t('tools.tax.origin_hint', 'Where you are coming from — affects treaties.')}  />
</label>
					<div class="mt-2">
						<Autocomplete
							placeholder={t('tax_calculator.select_origin', 'Select origin country…')}
							emptyMessage={t('common.no_results', 'No results found')}
							fetcher={fetchDestinations}
							initialFetcher={fetchDestinationsInitial}
							selectedId={selectedOriginId || null}
							selectedLabel={selectedOriginLabel}
							onSelect={(item) => (selectedOriginId = item.id)}
							onClear={() => (selectedOriginId = '')}
						/>
					</div>
				</div>
			</div>

			<!-- Employment status (select — backend validates enum) -->
			<div class="flex items-center gap-3">
				<label for="employment" class="text-sm font-medium text-[var(--color-text)] whitespace-nowrap">
					{t('tax_calculator.employment_status', 'Employment status:')}
				
				<HintIcon text={t('tools.tax.employment_hint', 'Your employment situation.')}  />
</label>
				<AutocompleteSelect
					id="employment"
					value={employmentStatus}
					options={employmentOptions.length > 0
						? employmentOptions
						: [{ value: '', label: t('tax_calculator.employment_any', 'Any (optional)') }]}
					onValueChange={(v) => (employmentStatus = v)}
				/>
				<button
					onclick={search}
					disabled={!canSearch || loading}
					class="px-5 py-2 bg-[var(--color-primary)] text-[var(--color-primary-contrast)] rounded-[var(--radius)] text-sm font-medium hover:bg-[var(--color-primary-hover)] transition-colors disabled:opacity-50"
				>
					{loading
						? t('common.searching', 'Searching...')
						: t('tax_calculator.search', 'Search')}
				</button>
			</div>
		</div>

		<div class="p-4">
			{#if loading}
				<div class="space-y-3">
					<LoadingSkeleton variant="card" count={2} />
					<LoadingSkeleton variant="lines" count={2} />
				</div>
			{:else if !hasSearched}
				<EmptyState
					icon={ArrowRightLeft}
					title={t('tax_calculator.select_pair', 'Select origin and destination')}
					body={t(
						'tax_calculator.select_pair_desc',
						'Choose your origin and destination countries to see tax guides and treaty information.'
					)}
				/>
			{:else if guides.length === 0 && !treaty}
				<EmptyState
					icon={Receipt}
					title={t('tax_calculator.no_guides', 'No tax guides')}
					body={t(
						'tax_calculator.no_guides_desc',
						'No tax guides or treaty information have been published for this origin→destination pair yet.'
					)}
				/>
			{:else}
				<div class="space-y-6">
					<!-- Tax guides -->
					{#if guides.length > 0}
						<div>
							<h2 class="text-sm font-semibold text-[var(--color-muted)] uppercase tracking-wide mb-3">
								{t('tax_calculator.tax_guides', 'Tax guides')}
							</h2>
							<div class="space-y-3">
								{#each guides as guide (guide.id)}
									<div class="bg-[var(--color-surface)] border border-[var(--color-border)] rounded-[var(--radius-card)] overflow-hidden">
										<!-- Card header (click to expand) -->
										<button
											onclick={() => toggleExpand(guide.id)}
											class="w-full flex items-center justify-between p-4 text-start hover:bg-[var(--color-surface-subtle)] transition-colors"
											aria-expanded={expandedId === guide.id}
										>
											<div class="min-w-0">
												<div class="flex items-center gap-2 flex-wrap">
													<span class="text-sm font-semibold text-[var(--color-text)]">
														{guide.employment_status}
													</span>
													{#if guide.worldwide_income_taxed === true}
														<span class="text-xs px-2 py-0.5 bg-[var(--color-surface-subtle)] text-[var(--color-text)] rounded-full font-medium">
															{t('tax_calculator.worldwide_taxed', 'Worldwide income taxed')}
														</span>
													{:else if guide.worldwide_income_taxed === false}
														<span class="text-xs px-2 py-0.5 bg-[var(--color-surface-subtle)] text-[var(--color-muted)] rounded-full font-medium">
															{t('tax_calculator.worldwide_not_taxed', 'Worldwide income not taxed')}
														</span>
													{/if}
												</div>
												<p class="text-xs text-[var(--color-muted)] mt-1">
													{guide.origin_space_name ?? t('tax_calculator.unknown', 'Unknown')}
													→
													{guide.destination_space_name ?? t('tax_calculator.unknown', 'Unknown')}
												</p>
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
												{#if guide.residency_trigger}
													<div>
														<h3 class="text-sm font-semibold text-[var(--color-muted)] uppercase tracking-wide mb-1">
															{t('tax_calculator.residency_trigger', 'Residency trigger')}
														</h3>
														<p class="text-sm text-[var(--color-text)]">{guide.residency_trigger}</p>
													</div>
												{/if}

												{#if guide.first_year_obligations}
													<div>
														<h3 class="text-sm font-semibold text-[var(--color-muted)] uppercase tracking-wide mb-1">
															{t('tax_calculator.first_year_obligations', 'First year obligations')}
														</h3>
														<p class="text-sm text-[var(--color-text)]">{guide.first_year_obligations}</p>
													</div>
												{/if}

												{#if guide.action_checklist}
													{@const items = checklistItems(guide.action_checklist)}
													{#if items.length > 0}
														<div>
															<h3 class="text-sm font-semibold text-[var(--color-muted)] uppercase tracking-wide mb-2">
																{t('tax_calculator.action_checklist', 'Action checklist')}
															</h3>
															<ul class="space-y-1">
																{#each items as item, i (i)}
																	<li class="flex items-start gap-2 text-sm text-[var(--color-text)]">
																		<CheckCircle2 size={14} class="text-[var(--color-primary)] flex-shrink-0 mt-0.5" />
																		<span>
																			{item.action}
																			{#if item.deadline}
																				<span class="text-[var(--color-muted)]">— {item.deadline}</span>
																			{/if}
																			{#if item.url}
																				<a href={item.url} target="_blank" rel="noopener noreferrer" class="ms-1 text-[var(--color-primary)] hover:underline inline-flex items-center gap-0.5">
																					<ExternalLink class="rtl:-scale-x-100" size={12} />
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
							<h2 class="text-sm font-semibold text-[var(--color-muted)] uppercase tracking-wide mb-3">
								{t('tax_calculator.treaty_info', 'Double taxation treaty')}
							</h2>
							<div class="p-4 bg-[var(--color-surface)] border border-[var(--color-border)] rounded-[var(--radius-card)]">
								<div class="flex items-center gap-2 mb-3">
									{#if treaty.treaty_exists}
										<span class="text-xs px-2 py-0.5 bg-[var(--color-surface-subtle)] text-[var(--color-text)] rounded-full font-medium">
											{t('tax_calculator.treaty_exists', 'Treaty exists')}
										</span>
									{:else}
										<span class="text-xs px-2 py-0.5 bg-[var(--color-surface-subtle)] text-[var(--color-muted)] rounded-full font-medium">
											{t('tax_calculator.no_treaty', 'No treaty')}
										</span>
									{/if}
									<span class="text-xs text-[var(--color-muted)]">
										{treaty.origin_space_name ?? t('tax_calculator.unknown', 'Unknown')}
										↔
										{treaty.destination_space_name ?? t('tax_calculator.unknown', 'Unknown')}
									</span>
								</div>
								{#if treaty.treaty_summary}
									<p class="text-sm text-[var(--color-text)]">{treaty.treaty_summary}</p>
								{/if}
								{#if treaty.treaty_source_url}
									<a
										href={treaty.treaty_source_url}
										target="_blank"
										rel="noopener noreferrer"
										class="mt-3 inline-flex items-center gap-1.5 text-sm text-[var(--color-primary)] hover:underline"
									>
										<ExternalLink class="rtl:-scale-x-100" size={14} />
										{t('tax_calculator.treaty_source', 'Treaty source')}
									</a>
								{/if}
							</div>
						</div>
					{/if}

				<!-- Service Directory: verified tax consultants (§100 Integration) -->
				{#if taxConsultants.length > 0}
					<div>
						<h2 class="text-sm font-semibold text-[var(--color-muted)] uppercase tracking-wide mb-3">
							{t('tax_calculator.tax_consultants', 'Verified tax consultants')}
						</h2>
						<div class="space-y-2">
							{#each taxConsultants as consultant (consultant.id)}
								{#if consultant.slug}
									<a
										href={`/services/${consultant.slug}`}
										class="block p-3 bg-[var(--color-surface)] border border-[var(--color-border)] rounded-[var(--radius-card)] hover:bg-[var(--color-surface-subtle)] transition-colors"
									>
										<p class="text-sm font-medium text-[var(--color-text)]">{consultant.name}</p>
										{#if consultant.description}
											<p class="text-xs text-[var(--color-muted)] mt-1 line-clamp-2">{consultant.description}</p>
										{/if}
									</a>
								{:else}
									<div class="block p-3 bg-[var(--color-surface)] border border-[var(--color-border)] rounded-[var(--radius-card)]">
										<p class="text-sm font-medium text-[var(--color-text)]">{consultant.name}</p>
										{#if consultant.description}
											<p class="text-xs text-[var(--color-muted)] mt-1 line-clamp-2">{consultant.description}</p>
										{/if}
									</div>
								{/if}
							{/each}
						</div>
					</div>
				{/if}

				<!-- HITL disclosure (§100/ADR-108: mandatory on every output page) -->
				<div class="mt-6 p-3 bg-[var(--color-surface-subtle)] border border-[var(--color-border)] rounded-[var(--radius)] text-xs text-[var(--color-muted)]">
					{t(
						'tax_calculator.hitl_disclosure',
						'All tax content on this page has been reviewed by a qualified tax professional before publication. This tool provides general guidance only — consult a qualified tax advisor for your specific situation.'
					)}
				</div>
				</div>
			{/if}
		</div>
	{/if}
</div>
