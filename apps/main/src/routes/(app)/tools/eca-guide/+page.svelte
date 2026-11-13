<script lang="ts">
	import SeoHead from '#lib/seo/SeoHead.svelte';
	import { toolOgCard } from '#lib/seo/og.js';
	import { untrack, onMount } from 'svelte';
	import { api } from '#lib/api/client.js';
	import { t } from '#lib/i18n/index.js';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { ChevronDown, ChevronUp, FileBadge, ExternalLink, Timer } from 'lucide-svelte';
	import { EmptyState, LoadingSkeleton, AutocompleteSelect } from '#lib/components/ui/index.js';
	import DestinationSelector from '#lib/components/tools/DestinationSelector.svelte';
	import { currencySymbol } from '#lib/utils/currency.js';
	import type { GeoSpaceOption, EcaProvider, EcaGuide, EcaGuideDetail } from '#lib/types/index.js';
	import type { PageData } from './$types';
	import ToolHeader from '#lib/components/tools/ToolHeader.svelte';

	/**
	 * ECA Evaluation Guide — §109, ADR-120.
	 * Destination → provider/purpose filters → guide list → detail with
	 * pathway steps, fee/timeline, and community updates. Auth users can
	 * submit fee/timeline reports. Cross-links to /tools/credential-checker.
	 *
	 * Ref: japaconnect-engineering/sections/109-eca-guide.md
	 */
	let { data }: { data: PageData } = $props();

	// Fee display — managed symbol prefix; skips the prefix when the fee
	// string already embeds the code (e.g. "200 USD").
	function feeText(currency: string | null | undefined, fee: string | null | undefined): string {
		if (!fee) return '';
		const c = currency?.trim().toUpperCase();
		if (c && fee.toUpperCase().includes(c)) return fee;
		const sym = currencySymbol(c, data.currencies) || c || '';
		return sym ? `${sym} ${fee}` : fee;
	}

	let destinations = $state<GeoSpaceOption[]>(untrack(() => data.destinations));
	let providers = $state<EcaProvider[]>(untrack(() => data.providers));
	let guides = $state<EcaGuide[]>(untrack(() => data.guides));
	const authed = $derived(data.authed);

	// Deep-link restore: ?dest=<space id> pre-selects the destination and
	// the SSR pre-fetch already populated `guides` (see +page.server.ts).
	// ?purpose=/ ?provider= restore the secondary filters too.
	let selectedDest = $state(untrack(() => data.initialDestId ?? ''));
	let selectedPurpose = $state(untrack(() => page.url.searchParams.get('purpose') ?? ''));
	let selectedProvider = $state(untrack(() => page.url.searchParams.get('provider') ?? ''));
	let loadingGuides = $state(false);
	let guidesError = $state('');
	let hasSearched = $state(untrack(() => !!data.initialDestId));

	// Provider comparison state (GET /eca/compare — §109 side-by-side).
	let compareSelected = $state<string[]>([]);
	let compareResults = $state<EcaGuide[]>([]);
	let comparing = $state(false);
	let compareError = $state('');

	// Detail state
	let expandedId = $state<string | null>(null);
	let detail = $state<EcaGuideDetail | null>(null);
	let loadingDetail = $state(false);

	// Community update submission
	let updateFee = $state('');
	let updateDays = $state<number | ''>('');
	let updateNote = $state('');
	let updateStep = $state<number | ''>('');
	let submittingUpdate = $state(false);
	let updateSuccess = $state(false);
	let updateError = $state('');
	let detailError = $state('');

	const purposeOptions = $derived([
		{ value: '', label: t('eca_guide.all_purposes', 'All purposes') },
		{ value: 'immigration', label: t('eca_guide.purpose_immigration', 'Immigration') },
		{ value: 'study', label: t('eca_guide.purpose_study', 'Study') },
		{
			value: 'employment',
			label: t('eca_guide.purpose_employment', 'Employment')
		},

		{
			value: 'professional_licensure',
			label: t('eca_guide.purpose_professional_licensure', 'Professional licensure')
		}
	]);

	// Human label for a purpose enum value — falls back to the raw value
	// so unknown backend values still render something readable.
	function purposeLabel(purpose: string): string {
		return purposeOptions.find((o) => o.value === purpose)?.label ?? purpose.replaceAll('_', ' ');
	}

	// documents_required is a plain string list per the API contract, but
	// legacy/admin-authored rows may carry {document, notes} objects —
	// render either shape without breaking.
	function docLabel(doc: unknown): string {
		if (typeof doc === 'string') return doc;
		if (doc && typeof doc === 'object') {
			const d = doc as { document?: string; notes?: string };
			return [d.document, d.notes].filter(Boolean).join(' — ');
		}
		return '';
	}

	const title = $derived(t('eca_guide.seo_title', 'ECA Evaluation Guide — WES, IQAS & More — {site}'));
	const description = $derived(
		t(
			'eca_guide.seo_description',
			'Compare credential evaluation providers, follow the step-by-step guide for your destination, and read real fee and timeline reports.'
		)
	);

	// Keep the shareable ?dest=&purpose=&provider= deep link in sync without
	// re-running the server load (replaceState does not trigger navigation).
	function syncUrl() {
		const url = new URL(page.url.href);

		if (selectedDest) url.searchParams.set('dest', selectedDest); else url.searchParams.delete('dest');
		if (selectedPurpose) url.searchParams.set('purpose', selectedPurpose); else url.searchParams.delete('purpose');
		if (selectedProvider) url.searchParams.set('provider', selectedProvider); else url.searchParams.delete('provider');

		goto(url, { shallow: true, replace: true });
	}

	async function loadGuides() {
		if (!selectedDest) return;
		hasSearched = true;
		expandedId = null;
		detail = null;
		guidesError = '';
		loadingGuides = true;
		const params = new URLSearchParams({ dest: selectedDest });
		if (selectedPurpose) params.set('purpose', selectedPurpose);
		if (selectedProvider) params.set('provider', selectedProvider);
		const res = await api.get<EcaGuide[]>(`/eca/guides?${params.toString()}`);
		loadingGuides = false;
		if (res.error) {
			guides = [];
			guidesError = res.error.message || t('eca_guide.load_failed', 'Could not load guides. Please try again.');
		} else {
			guides = res.data ?? [];
			guidesError = '';
		}
		syncUrl();
	}

	// runCompare fetches the side-by-side provider comparison for the selected
	// destination (GET /eca/compare?dest=&provider_ids=). Needs 2+ providers.
	async function runCompare() {
		if (!selectedDest || compareSelected.length < 2) return;
		comparing = true;
		compareError = '';
		compareResults = [];
		const res = await api.get<EcaGuide[]>(
			`/eca/compare?dest=${selectedDest}&provider_ids=${compareSelected.join(',')}`
		);
		comparing = false;
		if (res.error) {
			compareError = res.error.message || t('eca_guide.compare_failed', 'Could not load the comparison.');
		} else {
			compareResults = res.data ?? [];
		}
	}

	function toggleCompareProvider(id: string) {
		if (compareSelected.includes(id)) {
			compareSelected = compareSelected.filter((p) => p !== id);
		} else {
			compareSelected = [...compareSelected, id];
		}
	}

	// Compare rows are grouped per purpose — each compared guide contributes
	// one column so the grid stays aligned by provider.
	const comparePurposes = $derived.by(() => {
		const seen = new Map<string, EcaGuide[]>();
		for (const g of compareResults) {
			const list = seen.get(g.purpose) ?? [];
			list.push(g);
			seen.set(g.purpose, list);
		}
		return [...seen.entries()];
	});

	function onSearch(e: Event) {
		e.preventDefault();
		loadGuides();
	}

	// Destination is the primary data axis — the provider dropdown re-scopes
	// to providers with a published guide at the selected destination
	// (GET /eca/providers?dest=), so a stale provider selection is cleared.
	async function onDestChange() {
		guides = [];
		hasSearched = false;
		expandedId = null;
		detail = null;
		selectedProvider = '';
		compareSelected = [];
		compareResults = [];
		compareError = '';
		const res = await api.get<EcaProvider[]>(
			selectedDest ? `/eca/providers?dest=${selectedDest}` : '/eca/providers'
		);
		if (!res.error && res.data) providers = res.data;
		syncUrl();
	}

	async function toggleDetail(guide: EcaGuide) {
		if (expandedId === guide.id) {
			expandedId = null;
			detail = null;
			return;
		}
		expandedId = guide.id;
		loadingDetail = true;
		updateSuccess = false;
		updateError = '';
		detailError = '';
		const res = await api.get<EcaGuideDetail>(`/eca/guides/${guide.id}`);
		loadingDetail = false;
		if (!res.error && res.data) {
			detail = res.data;
		} else {
			detail = { guide, updates: [] };
			detailError = res.error?.message
				|| t('eca_guide.detail_failed', 'Could not load the guide details. Please try again.');
		}
	}

	async function submitUpdate() {
		if (!expandedId || !authed) return;
		submittingUpdate = true;
		updateError = '';
		const res = await api.post<{ id: string; status: string }>(`/eca/guides/${expandedId}/updates`, {
			fee_estimate: updateFee || undefined,
			turnaround_days: updateDays === '' ? undefined : updateDays,
			note: updateNote || undefined,
			step_index: updateStep === '' ? undefined : updateStep
		});
		submittingUpdate = false;
		if (!res.error) {
			updateSuccess = true;
			updateFee = '';
			updateDays = '';
			updateNote = '';
			updateStep = '';
		} else {
			updateError = res.error.message || t('eca_guide.update_failed', 'Could not submit your report.');
		}
	}

	const inputCls =
		'w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)]';
	const cardCls = 'rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)]';

	onMount(() => {
		// Deep link carried purpose/provider filters that SSR did not apply —
		// re-run the search so the initial list matches the shared URL.
		if (data.initialDestId && (selectedPurpose || selectedProvider)) {
			loadGuides();
		}
	});
</script>

<SeoHead title={title} description={description} ogTitle={title}
	ogImage={toolOgCard(page.url.origin, page.url.pathname)}
/>

<div class="mx-auto w-full max-w-4xl px-4 py-6">
	<ToolHeader icon={FileBadge} title={t('eca_guide.title', 'ECA Evaluation Guide')} description={t('eca_guide.subtitle', 'Compare credential evaluation providers and follow the step-by-step process for your destination.')} />

	<!-- Selector bar -->
	<form onsubmit={onSearch} class="{cardCls} p-4">
		<div class="grid grid-cols-1 gap-4 sm:grid-cols-3">
			<div>
				<label
					class="mb-1 block text-sm font-medium text-[var(--color-text)]"
					for="dest-selector"
				>{t('eca_guide.destination', 'Destination')}</label>

				<DestinationSelector
					destinations={destinations}
					bind:selectedId={selectedDest}
					onchange={onDestChange}
					hideLabel
				/>
			</div>
			<div>
				<label class="mb-1 block text-sm font-medium text-[var(--color-text)]" for="purpose">
					{t('eca_guide.purpose', 'Purpose')}
				</label>
				<AutocompleteSelect id="purpose" bind:value={selectedPurpose} options={purposeOptions} />
			</div>
			<div>
				<label class="mb-1 block text-sm font-medium text-[var(--color-text)]" for="provider">
					{t('eca_guide.provider', 'Provider')}
				</label>
				<AutocompleteSelect
					id="provider"
					bind:value={selectedProvider}
					options={[
						{ value: '', label: t('eca_guide.all_providers', 'All providers') },
						...providers.map((p) => ({ value: p.slug, label: p.name }))
					]}
				/>
			</div>
		</div>
		<div class="mt-4">
			<button
				type="submit"
				disabled={loadingGuides || !selectedDest}
				class="inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-[var(--color-primary-contrast)] hover:opacity-90 disabled:opacity-50"
			>
				{loadingGuides ? t('eca_guide.loading', 'Loading…') : t('eca_guide.find_guides', 'Find guides')}
			</button>
		</div>
	</form>

	<!-- Provider comparison (§109 side-by-side — GET /eca/compare) -->
	{#if selectedDest && providers.length >= 2}
		<div class="{cardCls} mt-4 p-4">
			<div class="mb-2 text-sm font-medium text-[var(--color-text)]">
				{t('eca_guide.compare_title', 'Compare providers')}
			</div>
			<p class="mb-3 text-xs text-[var(--color-muted)]">
				{t('eca_guide.compare_hint', 'Pick two or more providers to compare fees, timelines and purposes side by side.')}
			</p>
			<div class="flex flex-wrap gap-2">
				{#each providers as provider (provider.id)}
					<button
						type="button"
						onclick={() => toggleCompareProvider(provider.id)}
						aria-pressed={compareSelected.includes(provider.id)}
						class="rounded-full border px-3 py-1 text-xs transition-colors {compareSelected.includes(provider.id)
							? 'border-[var(--color-primary)] bg-[var(--color-primary)]/10 text-[var(--color-primary-text)]'
							: 'border-[var(--color-border)] text-[var(--color-muted)] hover:border-[var(--color-primary)]'}"
					>
						{provider.name}
					</button>
				{/each}
			</div>
			<button
				type="button"
				onclick={runCompare}
				disabled={comparing || compareSelected.length < 2}
				class="mt-3 inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-1.5 text-sm text-[var(--color-text)] hover:border-[var(--color-primary)] disabled:opacity-50"
			>
				{comparing ? t('eca_guide.loading', 'Loading…') : t('eca_guide.compare_action', 'Compare')}
			</button>
			{#if compareError}
				<p class="mt-2 text-xs text-[var(--color-danger)]">{compareError}</p>
			{/if}

			{#if compareResults.length > 0}
				<div class="mt-4 space-y-4">
					{#each comparePurposes as [purpose, guidesForPurpose] (purpose)}
						<div>
							<div class="mb-2 text-xs font-semibold uppercase tracking-wide text-[var(--color-muted)]">
								{purposeLabel(purpose)}
							</div>
							<div class="grid grid-cols-1 gap-2 sm:grid-cols-2 lg:grid-cols-4">
								{#each guidesForPurpose as g (g.id)}
									<div class="rounded-[var(--radius)] border border-[var(--color-border)] p-3">
										<div class="text-sm font-medium text-[var(--color-text)]">
											{g.provider_name ?? g.provider_slug}
										</div>
										<div class="mt-1 space-y-0.5 text-xs text-[var(--color-muted)]">
											{#if g.fee_estimate}
												<div>{feeText(g.currency, g.fee_estimate)}</div>
											{/if}
											{#if g.timeline_estimate}
												<div>{g.timeline_estimate}</div>
											{/if}
											{#if g.official_url}
												<a
													href={g.official_url}
													target="_blank"
													rel="noopener noreferrer"
													class="inline-flex items-center gap-1 underline text-[var(--color-primary-text)]"
												>
													<ExternalLink size={11} class="rtl:-scale-x-100" />
													{t('eca_guide.official', 'Official website')}
												</a>
											{/if}
										</div>
									</div>
								{/each}
							</div>
						</div>
					{/each}
				</div>
			{/if}
		</div>
	{/if}

	<!-- Provider directory (always visible before search) -->
	{#if providers.length > 0 && !hasSearched}
		<div class="mt-6">
			<h2 class="mb-3 text-sm font-semibold uppercase tracking-wide text-[var(--color-muted)]">
				{t('eca_guide.providers', 'ECA providers')}
			</h2>
			<div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
				{#each providers as provider (provider.id)}
					<div class="{cardCls} p-3">
						<div class="font-medium text-[var(--color-text)]">{provider.name}</div>
						<div class="mt-2 flex items-center gap-3 text-xs text-[var(--color-muted)]">
							{#if provider.website}
								<a href={provider.website} target="_blank" rel="noopener noreferrer" class="underline text-[var(--color-primary-text)]">{provider.website}</a>
							{/if}
						</div>
					</div>
				{/each}
			</div>
		</div>
	{/if}

	<!-- Guide list -->
	{#if guidesError}
		<p class="mt-6 text-sm text-[var(--color-danger)]">{guidesError}</p>
	{/if}
	{#if loadingGuides}
		<div class="mt-6"><LoadingSkeleton variant="lines" /></div>
	{:else if hasSearched && guides.length === 0 && !guidesError}
		<EmptyState
			icon={FileBadge}
			title={t('eca_guide.no_guides', 'No guides found')}
			body={t('eca_guide.no_guides_desc', 'Try a different destination or purpose — guides are added as they are reviewed.')}
		/>
	{:else if guides.length > 0}
		<div class="mt-6 space-y-3">
			{#each guides as guide (guide.id)}
				<div class={cardCls}>
					<button
						class="flex w-full items-center justify-between gap-4 p-4 text-start"
						onclick={() => toggleDetail(guide)}
						aria-expanded={expandedId === guide.id}
					>
						<div>
							<div class="font-medium text-[var(--color-text)]">
								{guide.provider_name ?? guide.provider_slug ?? 'Guide'} — {purposeLabel(guide.purpose)}
							</div>
							<div class="mt-1 flex items-center gap-3 text-xs text-[var(--color-muted)]">
								{#if guide.fee_estimate}
									<span>{feeText(guide.currency, guide.fee_estimate)}</span>
								{/if}
								{#if guide.timeline_estimate}
									<span>{guide.timeline_estimate}</span>
								{/if}
							</div>
						</div>
						{#if expandedId === guide.id}<ChevronUp size={18} />{:else}<ChevronDown size={18} />{/if}
					</button>

					{#if expandedId === guide.id}
						<div class="border-t border-[var(--color-border)] p-4">
							{#if loadingDetail}
								<LoadingSkeleton variant="lines" />
							{:else if detail}
								{#if detailError}
									<p class="mb-3 text-xs text-[var(--color-danger)]">{detailError}</p>
								{/if}
								<!-- Pathway steps -->
								{#if detail.guide.pathway_steps?.length}
									<div class="mb-4 space-y-3">
										{#each detail.guide.pathway_steps as step (step.step)}
											<div class="flex gap-3">
												<div class="flex h-6 w-6 shrink-0 items-center justify-center rounded-full bg-[var(--color-primary)]/10 text-xs font-semibold text-[var(--color-primary-text)]">
													{step.step}
												</div>
												<div>
													<div class="text-sm font-medium text-[var(--color-text)]">{step.title}</div>
													{#if step.detail}
														<div class="mt-0.5 text-xs text-[var(--color-muted)]">{step.detail}</div>
													{/if}
													{#if step.duration}
														<div class="mt-0.5 flex items-center gap-1 text-xs text-[var(--color-muted)]"><Timer size={11} class="rtl:-scale-x-100" />{step.duration}</div>
													{/if}
												</div>
											</div>
										{/each}
									</div>
								{/if}

								<!-- Documents required -->
								{#if detail.guide.documents_required?.length}
									<div class="mb-3">
										<div class="mb-1 text-xs font-semibold uppercase text-[var(--color-muted)]">
											{t('eca_guide.documents', 'Documents required')}
										</div>
										<div class="flex flex-wrap gap-1">
											{#each detail.guide.documents_required as doc, i (i)}
												<span class="rounded bg-[var(--color-surface-subtle)] px-2 py-0.5 text-xs text-[var(--color-text)]">{docLabel(doc)}</span>
											{/each}
										</div>
									</div>
								{/if}

								<!-- Accepted-for programs (§109: which programs this provider
								     is accepted for at the destination) -->
								{#if detail.guide.accepted_for?.length}
									<div class="mb-3">
										<div class="mb-1 text-xs font-semibold uppercase text-[var(--color-muted)]">
											{t('eca_guide.accepted_for', 'Accepted for')}
										</div>
										<div class="flex flex-wrap gap-1">
											{#each detail.guide.accepted_for as prog, i (i)}
												<span class="rounded bg-[var(--color-surface-subtle)] px-2 py-0.5 text-xs text-[var(--color-text)]">{prog}</span>
											{/each}
										</div>
									</div>
								{/if}

								{#if detail.guide.official_url}
									<a href={detail.guide.official_url} target="_blank" rel="noopener noreferrer" class="inline-flex items-center gap-1 text-xs underline text-[var(--color-primary-text)]">
										<ExternalLink size={11} class="rtl:-scale-x-100" />
										{t('eca_guide.official', 'Official website')}
									</a>
								{/if}

								<!-- Community updates -->
								{#if detail.updates.length > 0}
									<div class="mt-4 border-t border-[var(--color-border)] pt-3">
										<div class="mb-2 text-xs font-semibold uppercase text-[var(--color-muted)]">
											{t('eca_guide.community', 'Community reports')}
										</div>
										{#each detail.updates as update (update.id)}
											<div class="mb-2 text-xs text-[var(--color-text)]">
												<span class="font-medium">{update.user_name ?? t('eca_guide.anon_member', 'Member')}</span>
												{#if update.fee_estimate}
													· {t('eca_guide.paid_fmt', 'paid {fee}', { fee: update.fee_estimate })}
												{/if}
												{#if update.turnaround_days}
													· {t('eca_guide.turnaround_fmt', '{days}d turnaround', { days: update.turnaround_days })}
												{/if}
												{#if update.note}
													<p class="mt-0.5 text-[var(--color-muted)]">{update.note}</p>
												{/if}
											</div>
										{/each}
									</div>
								{/if}

								<!-- Submit update -->
								{#if authed}
									{#if updateSuccess}
										<p class="mt-3 text-xs text-[var(--color-success)]">
											{t('eca_guide.update_thanks', 'Thanks — your report is pending review.')}
										</p>
									{:else}
										<div class="mt-4 border-t border-[var(--color-border)] pt-3">
											<div class="mb-2 text-xs font-semibold uppercase text-[var(--color-muted)]">
												{t('eca_guide.share', 'Share your experience')}
											</div>
											<div class="grid grid-cols-1 gap-2 sm:grid-cols-4">
												<input type="text" bind:value={updateFee} placeholder={t('eca_guide.fee_placeholder', 'Fee paid (e.g. $230 CAD)')} class="{inputCls} text-sm" />
												<input type="number" bind:value={updateDays} placeholder={t('eca_guide.days_placeholder', 'Turnaround (days)')} class="{inputCls} text-sm" min="0" />
												<select bind:value={updateStep} class="{inputCls} text-sm" aria-label={t('eca_guide.step_label', 'Step (optional)')}>
													<option value="">{t('eca_guide.step_placeholder', 'Step (optional)')}</option>
													{#each detail.guide.pathway_steps ?? [] as step, i (i)}
														<option value={i}>{step.step}. {step.title}</option>
													{/each}
												</select>
												<input type="text" bind:value={updateNote} maxlength={500} placeholder={t('eca_guide.note_placeholder', 'Short note (optional)')} class="{inputCls} text-sm" />
											</div>
											<button
												class="mt-2 inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-1.5 text-sm text-[var(--color-text)] hover:border-[var(--color-primary)] disabled:opacity-50"
												onclick={submitUpdate}
												disabled={submittingUpdate || (!updateFee && updateDays === '' && !updateNote)}
											>
												{submittingUpdate ? t('eca_guide.submitting', 'Submitting…') : t('eca_guide.submit', 'Submit report')}
											</button>
											{#if updateError}
												<p class="mt-2 text-xs text-[var(--color-danger)]">{updateError}</p>
											{/if}
										</div>
									{/if}
								{:else}
									<a href="/login?redirect={encodeURIComponent(page.url.pathname)}" class="mt-3 inline-block text-xs underline text-[var(--color-primary-text)]">
										{t('eca_guide.signin_to_share', 'Sign in to share your experience')}
									</a>
								{/if}
							{/if}
						</div>
					{/if}
				</div>
			{/each}
		</div>
	{/if}

	<!-- Cross-link to credential checker -->
	<div class="mt-6 rounded-[var(--radius-card)] border border-dashed border-[var(--color-border)] p-4 text-center text-sm text-[var(--color-muted)]">
		{t('eca_guide.credential_hint', 'Need to check if your profession is regulated?')}
		<a href="/tools/credential-checker" class="font-medium underline text-[var(--color-primary-text)]">
			{t('eca_guide.credential_link', 'Credential Checker')}
		</a>
	</div>
</div>
