<script lang="ts">
	import { untrack } from 'svelte';
	import { api } from '$lib/api/client';
	import { t } from '$lib/i18n';
	import { page } from '$app/state';
	import { replaceState } from '$app/navigation';
	import { ChevronDown, ChevronUp, FileBadge } from 'lucide-svelte';
	import { EmptyState, LoadingSkeleton, AutocompleteSelect } from '$lib/components/ui';
	import DestinationSelector from '$lib/components/tools/DestinationSelector.svelte';
	import { currencySymbol } from '$lib/utils/currency';
	import type { GeoSpaceOption, EcaProvider, EcaGuide, EcaGuideDetail } from '$lib/types';
	import type { PageData } from './$types';

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
		if (!expandedId || !authed) return;
		submittingUpdate = true;
		updateError = '';
		const res = await api.post<{ id: string; status: string }>(`/eca/guides/${expandedId}/updates`, {
			fee_estimate: updateFee || undefined,
			turnaround_days: updateDays === '' ? undefined : updateDays,
			note: updateNote || undefined
		});
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

	const inputCls =
		'w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)]';
	const cardCls = 'rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)]';
</script>

<svelte:head>
	<title>{title}</title>
	<meta name="description" content={description} />
	<link rel="canonical" href={page.url.href} />
	<meta property="og:title" content={title} />
	<meta property="og:description" content={description} />
	<meta property="og:type" content="website" />
	<meta property="og:url" content={page.url.href} />
</svelte:head>

<div class="mx-auto w-full max-w-4xl px-4 py-6">
	<div class="mb-6">
		<h1 class="flex items-center gap-2 text-lg font-semibold text-[var(--color-text)]">
			<FileBadge size={20} /> {t('eca_guide.title', 'ECA Evaluation Guide')}
		</h1>
		<p class="mt-1 text-sm text-[var(--color-muted)]">
			{t('eca_guide.subtitle', 'Compare credential evaluation providers and follow the step-by-step process for your destination.')}
		</p>
	</div>

	<!-- Selector bar -->
	<form onsubmit={onSearch} class="{cardCls} p-4">
		<div class="grid grid-cols-1 gap-4 sm:grid-cols-3">
			<div>
				<label class="mb-1 block text-sm font-medium text-[var(--color-text)]" for="dest">
					{t('eca_guide.destination', 'Destination')}
				</label>
				<DestinationSelector {destinations} bind:selectedId={selectedDest} onchange={onDestChange} />
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
				class="inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-white hover:opacity-90 disabled:opacity-50"
			>
				{loadingGuides ? t('eca_guide.loading', 'Loading…') : t('eca_guide.find_guides', 'Find guides')}
			</button>
		</div>
	</form>

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
								<a href={provider.website} target="_blank" rel="noopener" class="underline text-[var(--color-primary)]">{provider.website}</a>
							{/if}
						</div>
					</div>
				{/each}
			</div>
		</div>
	{/if}

	<!-- Guide list -->
	{#if loadingGuides}
		<div class="mt-6"><LoadingSkeleton variant="lines" /></div>
	{:else if hasSearched && guides.length === 0}
		<EmptyState
			icon={FileBadge}
			title={t('eca_guide.no_guides', 'No guides found')}
			body={t('eca_guide.no_guides_desc', 'Try a different destination or purpose — guides are added as they are reviewed.')}
		/>
	{:else if guides.length > 0}
		<div class="mt-6 space-y-3">
			{#each guides as guide (guide.id)}
				<div class="{cardCls}">
					<button class="flex w-full items-center justify-between gap-4 p-4 text-start" onclick={() => toggleDetail(guide)}>
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
								<!-- Pathway steps -->
								{#if detail.guide.pathway_steps?.length}
									<div class="mb-4 space-y-3">
										{#each detail.guide.pathway_steps as step (step.step)}
											<div class="flex gap-3">
												<div class="flex h-6 w-6 shrink-0 items-center justify-center rounded-full bg-[var(--color-primary)]/10 text-xs font-semibold text-[var(--color-primary)]">
													{step.step}
												</div>
												<div>
													<div class="text-sm font-medium text-[var(--color-text)]">{step.title}</div>
													{#if step.detail}
														<div class="mt-0.5 text-xs text-[var(--color-muted)]">{step.detail}</div>
													{/if}
													{#if step.duration}
														<div class="mt-0.5 text-xs text-[var(--color-muted)]">⏱ {step.duration}</div>
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
									<a href={detail.guide.official_url} target="_blank" rel="noopener" class="text-xs underline text-[var(--color-primary)]">
										{t('eca_guide.official', 'Official website')} ↗
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
										<p class="mt-3 text-xs text-green-600">
											{t('eca_guide.update_thanks', 'Thanks — your report is pending review.')}
										</p>
									{:else}
										<div class="mt-4 border-t border-[var(--color-border)] pt-3">
											<div class="mb-2 text-xs font-semibold uppercase text-[var(--color-muted)]">
												{t('eca_guide.share', 'Share your experience')}
											</div>
											<div class="grid grid-cols-1 gap-2 sm:grid-cols-3">
												<input type="text" bind:value={updateFee} placeholder={t('eca_guide.fee_placeholder', 'Fee paid (e.g. $230 CAD)')} class="{inputCls} text-sm" />
												<input type="number" bind:value={updateDays} placeholder={t('eca_guide.days_placeholder', 'Turnaround (days)')} class="{inputCls} text-sm" min="0" />
												<input type="text" bind:value={updateNote} placeholder={t('eca_guide.note_placeholder', 'Short note (optional)')} class="{inputCls} text-sm" />
											</div>
											<button
												class="mt-2 inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-1.5 text-sm text-[var(--color-text)] hover:border-[var(--color-primary)] disabled:opacity-50"
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
									<a href="/login?redirect={encodeURIComponent(page.url.pathname)}" class="mt-3 inline-block text-xs underline text-[var(--color-primary)]">
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
		<a href="/tools/credential-checker" class="font-medium underline text-[var(--color-primary)]">
			{t('eca_guide.credential_link', 'Credential Checker')}
		</a>
	</div>
</div>
