<script lang="ts">
	import { untrack } from 'svelte';
	import { api } from '$lib/api/client';
	import { buildSeoMeta } from '$lib/seo';
	import { t } from '$lib/i18n';
	import { page } from '$app/state';
	import { replaceState } from '$app/navigation';
	import Icon from '$lib/components/ui/Icon.svelte';
	import EmptyState from '$lib/components/ui/EmptyState.svelte';
	import RegistrationPrompt from '$lib/components/ui/RegistrationPrompt.svelte';
	import ToolDisabled from '$lib/components/ui/ToolDisabled.svelte';
	import DestinationSelector from '$lib/components/tools/DestinationSelector.svelte';
	import AutocompleteSelect from '$lib/components/ui/AutocompleteSelect.svelte';
	import { currencySymbol } from '$lib/utils/currency';
	import type {
		GeoSpaceOption,
		EcaProvider,
		EcaGuide,
		EcaGuideDetail,
		EcaCommunityUpdate
	} from '$lib/types';

	/**
	 * ECA Evaluation Guide — §109, ADR-120.
	 * Destination → provider/purpose filters → guide list → detail with
	 * pathway steps, fee/timeline, and community updates. Auth users can
	 * submit fee/timeline reports. Cross-links to /credential-checker.
	 */
	let { data } = $props();

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
	let user = $state(untrack(() => data.user));

	// Deep-link restore: ?dest=<space id> pre-selects the destination and
	// the SSR pre-fetch already populated `guides` (see +page.server.ts).
	let selectedDest = $state(untrack(() => data.initialDestId ?? ''));
	let selectedPurpose = $state('');
	let selectedProvider = $state('');
	let loadingGuides = $state(false);
	let hasSearched = $state(untrack(() => !!data.initialDestId));

	// Detail state
	let expandedId = $state<string | null>(null);
	let detail = $state<EcaGuideDetail | null>(null);
	let loadingDetail = $state(false);

	// Community update submission
	let updateFee = $state('');
	let updateDays = $state<number | ''>('');
	let updateNote = $state('');
	let submittingUpdate = $state(false);
	let showRegisterPrompt = $state(false);
	let updateSuccess = $state(false);
	let updateError = $state('');

	const purposeOptions = $derived([
		{ value: '', label: t('eca_guide.all_purposes', 'All purposes') },
		{ value: 'immigration', label: t('eca_guide.purpose_immigration', 'Immigration') },
		{ value: 'study', label: t('eca_guide.purpose_study', 'Study') },
		{ value: 'employment', label: t('eca_guide.purpose_employment', 'Employment') },
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

	const seo = $derived(
		buildSeoMeta({
			title: t('eca_guide.seo_title', 'ECA Evaluation Guide — WES, IQAS & More — JapaConnect Tools'),
			description: t(
				'eca_guide.seo_description',
				'Compare credential evaluation providers, follow the step-by-step guide for your destination, and read real fee and timeline reports.'
			),
			path: '/eca-guide',
			origin: page.url.origin,
			jsonLd: {
				'@context': 'https://schema.org',
				'@type': 'HowTo',
				name: 'How to get an Educational Credential Assessment',
				description:
					'Step-by-step guide to getting your educational credentials evaluated for immigration, study, or employment.',
				step:
					guides.length > 0
						? (guides[0].pathway_steps ?? []).map((s) => ({
								'@type': 'HowToStep',
								position: s.step,
								name: s.title,
								text: s.detail
							}))
						: undefined
			}
		})
	);

	// Keep the shareable ?dest=&purpose=&provider= deep link in sync without
	// re-running the server load (replaceState does not trigger navigation).
	function syncUrl() {
		const url = new URL(page.url);
		if (selectedDest) url.searchParams.set('dest', selectedDest);
		else url.searchParams.delete('dest');
		if (selectedPurpose) url.searchParams.set('purpose', selectedPurpose);
		else url.searchParams.delete('purpose');
		if (selectedProvider) url.searchParams.set('provider', selectedProvider);
		else url.searchParams.delete('provider');
		replaceState(url, {});
	}

	async function loadGuides() {
		if (!selectedDest) return;
		hasSearched = true;
		expandedId = null;
		detail = null;
		loadingGuides = true;
		const params = new URLSearchParams({ dest: selectedDest });
		if (selectedPurpose) params.set('purpose', selectedPurpose);
		if (selectedProvider) params.set('provider', selectedProvider);
		const res = await api.get<EcaGuide[]>(`/eca/guides?${params.toString()}`);
		loadingGuides = false;
		guides = res.error ? [] : (res.data ?? []);
		syncUrl();
	}

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
		const res = await api.get<EcaGuideDetail>(`/eca/guides/${guide.id}`);
		loadingDetail = false;
		if (!res.error && res.data) {
			detail = res.data;
		} else {
			detail = { guide, updates: [] };
		}
	}

	async function submitUpdate() {
		if (!expandedId) return;
		if (!user) {
			showRegisterPrompt = true;
			return;
		}
		submittingUpdate = true;
		const res = await api.post<{ id: string; status: string }>(
			`/eca/guides/${expandedId}/updates`,
			{
				fee_estimate: updateFee || undefined,
				turnaround_days: updateDays === '' ? undefined : updateDays,
				note: updateNote || undefined
			}
		);
		submittingUpdate = false;
		if (!res.error) {
			updateSuccess = true;
			updateFee = '';
			updateDays = '';
			updateNote = '';
		} else {
			updateError = res.error.message || t('eca_guide.update_failed', 'Could not submit your report.');
		}
	}
</script>

<svelte:head>
	<title>{seo.title}</title>
	<meta name="description" content={seo.description} />
	<link rel="canonical" href={seo.canonical} />
	<meta property="og:title" content={seo.title} />
	<meta property="og:description" content={seo.description} />
	{#if seo.jsonLdString}
		{@html `<script type="application/ld+json">${seo.jsonLdString}<\/script>`}
	{/if}
</svelte:head>

{#if !data.enabled}
	<ToolDisabled tool={data.tool} />
{:else}
	<div class="mx-auto max-w-4xl px-4 py-8">
		<div class="mb-8">
			<h1 class="text-3xl font-bold tracking-tight">
				{t('eca_guide.title', 'ECA Evaluation Guide')}
			</h1>
			<p class="mt-2 text-muted">
				{t(
					'eca_guide.subtitle',
					'Compare credential evaluation providers and follow the step-by-step process for your destination.'
				)}
			</p>
		</div>

		<!-- Selector bar -->
		<form onsubmit={onSearch} class="rounded-xl border border-border bg-surface p-6">
			<div class="grid gap-4 sm:grid-cols-3">
				<div>
					<label class="block text-sm font-medium mb-1" for="dest">
						{t('eca_guide.destination', 'Destination')}
					</label>
					<DestinationSelector
						{destinations}
						bind:selectedId={selectedDest}
						onchange={onDestChange}
					/>
				</div>
				<div>
					<label class="block text-sm font-medium mb-1" for="purpose">
						{t('eca_guide.purpose', 'Purpose')}
					</label>
					<AutocompleteSelect
						id="purpose"
						bind:value={selectedPurpose}
						options={purposeOptions}
						class="w-full"
					/>
				</div>
				<div>
					<label class="block text-sm font-medium mb-1" for="provider">
						{t('eca_guide.provider', 'Provider')}
					</label>
					<AutocompleteSelect
						id="provider"
						bind:value={selectedProvider}
						options={[
							{ value: '', label: t('eca_guide.all_providers', 'All providers') },
							...providers.map((p) => ({ value: p.slug, label: p.name }))
						]}
						class="w-full"
					/>
				</div>
			</div>
			<div class="mt-4">
				<button type="submit" class="btn-primary" disabled={loadingGuides || !selectedDest}>
					{loadingGuides ? t('eca_guide.loading', 'Loading…') : t('eca_guide.find_guides', 'Find guides')}
				</button>
			</div>
		</form>

		<!-- Provider directory (always visible) -->
		{#if providers.length > 0 && !hasSearched}
			<div class="mt-8">
				<h2 class="text-sm font-semibold text-muted uppercase tracking-wide mb-3">
					{t('eca_guide.providers', 'ECA providers')}
				</h2>
				<div class="grid gap-3 sm:grid-cols-2">
					{#each providers as provider (provider.id)}
						<div class="rounded-lg border border-border bg-surface p-4">
							<div class="font-medium">{provider.name}</div>
							<div class="mt-2 flex items-center gap-3 text-xs text-muted">
								{#if provider.website}
									<a href={provider.website} target="_blank" rel="noopener" class="underline">{provider.website}</a>
								{/if}
							</div>
						</div>
					{/each}
				</div>
			</div>
		{/if}

		<!-- Guide list -->
		{#if loadingGuides}
			<div class="mt-6 space-y-3">
				<div class="h-20 animate-pulse rounded-lg bg-surface"></div>
				<div class="h-20 animate-pulse rounded-lg bg-surface"></div>
			</div>
		{:else if hasSearched && guides.length === 0}
			<EmptyState
				icon="file-badge"
				title={t('eca_guide.no_guides', 'No guides found')}
				description={t('eca_guide.no_guides_desc', 'Try a different destination or purpose — guides are added as they are reviewed.')}
			/>
		{:else if guides.length > 0}
			<div class="mt-6 space-y-3">
				{#each guides as guide (guide.id)}
					<div class="rounded-lg border border-border bg-surface">
						<button
							class="w-full text-left p-4 flex items-center justify-between gap-4"
							onclick={() => toggleDetail(guide)}
						>
							<div>
								<div class="font-medium">
									{guide.provider_name ?? guide.provider_slug ?? 'Guide'} — {purposeLabel(guide.purpose)}
								</div>
								<div class="mt-1 flex items-center gap-3 text-xs text-muted">
									{#if guide.fee_estimate}
										<span>{feeText(guide.currency, guide.fee_estimate)}</span>
									{/if}
									{#if guide.timeline_estimate}
										<span>{guide.timeline_estimate}</span>
									{/if}
								</div>
							</div>
							<Icon name={expandedId === guide.id ? 'chevron-up' : 'chevron-down'} size={18} />
						</button>

						{#if expandedId === guide.id}
							<div class="border-t border-border p-4">
								{#if loadingDetail}
									<div class="h-16 animate-pulse rounded bg-surface"></div>
								{:else if detail}
									<!-- Pathway steps -->
									{#if detail.guide.pathway_steps?.length}
										<div class="space-y-3 mb-4">
											{#each detail.guide.pathway_steps as step (step.step)}
												<div class="flex gap-3">
													<div class="flex h-6 w-6 shrink-0 items-center justify-center rounded-full bg-accent/10 text-xs font-semibold text-accent">
														{step.step}
													</div>
													<div>
														<div class="text-sm font-medium">{step.title}</div>
														{#if step.detail}
															<div class="text-xs text-muted mt-0.5">{step.detail}</div>
														{/if}
														{#if step.duration}
															<div class="text-xs text-muted mt-0.5">⏱ {step.duration}</div>
														{/if}
													</div>
												</div>
											{/each}
										</div>
									{/if}

									<!-- Documents + accepted-for -->
									{#if detail.guide.documents_required?.length}
										<div class="mb-3">
											<div class="text-xs font-semibold text-muted uppercase mb-1">
												{t('eca_guide.documents', 'Documents required')}
											</div>
											<div class="flex flex-wrap gap-1">
												{#each detail.guide.documents_required as doc, i (i)}
													<span class="rounded bg-surface px-2 py-0.5 text-xs">{docLabel(doc)}</span>
												{/each}
											</div>
										</div>
									{/if}

									<!-- Accepted-for programs (§109: which programs this provider
									     is accepted for at the destination) -->
									{#if detail.guide.accepted_for?.length}
										<div class="mb-3">
											<div class="text-xs font-semibold text-muted uppercase mb-1">
												{t('eca_guide.accepted_for', 'Accepted for')}
											</div>
											<div class="flex flex-wrap gap-1">
												{#each detail.guide.accepted_for as prog, i (i)}
													<span class="rounded bg-surface px-2 py-0.5 text-xs">{prog}</span>
												{/each}
											</div>
										</div>
									{/if}

									{#if detail.guide.official_url}
										<a href={detail.guide.official_url} target="_blank" rel="noopener" class="text-xs underline">
											{t('eca_guide.official', 'Official website')} ↗
										</a>
									{/if}

									<!-- Community updates -->
									{#if detail.updates.length > 0}
										<div class="mt-4 border-t border-border pt-3">
											<div class="text-xs font-semibold text-muted uppercase mb-2">
												{t('eca_guide.community', 'Community reports')}
											</div>
											{#each detail.updates as update (update.id)}
												<div class="mb-2 text-xs">
													<span class="font-medium">{update.user_name ?? t('eca_guide.anon_member', 'Member')}</span>
													{#if update.fee_estimate}
														· {t('eca_guide.paid_fmt', 'paid {fee}', { fee: update.fee_estimate })}
													{/if}
													{#if update.turnaround_days}
														· {t('eca_guide.turnaround_fmt', '{days}d turnaround', { days: update.turnaround_days })}
													{/if}
													{#if update.note}
														<p class="text-muted mt-0.5">{update.note}</p>
													{/if}
												</div>
											{/each}
										</div>
									{/if}

									<!-- Submit update -->
									{#if user}
										{#if updateSuccess}
											<p class="mt-3 text-xs text-green-600">
												{t('eca_guide.update_thanks', 'Thanks — your report is pending review.')}
											</p>
										{:else}
											<div class="mt-4 border-t border-border pt-3">
												<div class="text-xs font-semibold text-muted uppercase mb-2">
													{t('eca_guide.share', 'Share your experience')}
												</div>
												<div class="grid gap-2 sm:grid-cols-3">
													<input
														type="text"
														bind:value={updateFee}
														placeholder={t('eca_guide.fee_placeholder', 'Fee paid (e.g. $230 CAD)')}
														class="input text-sm"
													/>
													<input
														type="number"
														bind:value={updateDays}
														placeholder={t('eca_guide.days_placeholder', 'Turnaround (days)')}
														class="input text-sm"
														min="0"
													/>
													<input
														type="text"
														bind:value={updateNote}
														placeholder={t('eca_guide.note_placeholder', 'Short note (optional)')}
														class="input text-sm"
													/>
												</div>
												<button
													class="btn-secondary mt-2 text-sm"
													onclick={submitUpdate}
													disabled={submittingUpdate || (!updateFee && updateDays === '' && !updateNote)}
												>
													{submittingUpdate ? t('eca_guide.submitting', 'Submitting…') : t('eca_guide.submit', 'Submit report')}
												</button>
												{#if updateError}
													<p class="mt-2 text-xs text-red-600">{updateError}</p>
												{/if}
											</div>
										{/if}
									{:else}
										<button class="mt-3 text-xs underline" onclick={() => (showRegisterPrompt = true)}>
											{t('eca_guide.signin_to_share', 'Sign in to share your experience')}
										</button>
									{/if}
								{/if}
							</div>
						{/if}
					</div>
				{/each}
			</div>
		{/if}

		<!-- Cross-link to credential checker -->
		<div class="mt-8 rounded-xl border border-dashed border-border p-4 text-center text-sm">
			{t('eca_guide.credential_hint', 'Need to check if your profession is regulated?')}
			<a href="/credential-checker" class="font-medium underline">
				{t('eca_guide.credential_link', 'Credential Checker')}
			</a>
		</div>
	</div>

	<RegistrationPrompt
		open={showRegisterPrompt}
		onClose={() => (showRegisterPrompt = false)}
		icon="file-badge"
		title={t('register.eca_updates', 'Share your ECA experience')}
		description={t('register.eca_updates_desc', 'Sign in to report your fee and turnaround time — it helps other members.')}
	/>
{/if}
