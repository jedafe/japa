<script lang="ts">
	import { untrack } from 'svelte';
	import { replaceState } from '$app/navigation';
	import { api } from '$lib/api/client';
	import { PUBLIC_JC_API_URL } from '$env/static/public';
	import { buildApiUrl } from '@japaconnect/api-url';
	import { buildSeoMeta } from '$lib/seo';
	import SeoHead from '$lib/components/seo/SeoHead.svelte';
	import { t } from '$lib/i18n';
	import { page } from '$app/state';
	import Icon from '$lib/components/ui/Icon.svelte';
	import EmptyState from '$lib/components/ui/EmptyState.svelte';
	import LoadingSkeleton from '$lib/components/ui/LoadingSkeleton.svelte';
	import RegistrationPrompt from '$lib/components/ui/RegistrationPrompt.svelte';
	import ToolDisabled from '$lib/components/ui/ToolDisabled.svelte';
	import DestinationSelector from '$lib/components/tools/DestinationSelector.svelte';
	import Autocomplete from '$lib/components/ui/Autocomplete.svelte';
	import type { AutocompleteItem } from '$lib/components/ui/Autocomplete.svelte';
	import type { AttestationPathwayWithAi, AttestationCommunityUpdate, GeoSpaceOption } from '$lib/types';

	/**
	 * Attestation Guide tool — §97, ADR-088.
	 * Destination selector → optional issuing country + doc_type filters →
	 * list of pathways as expandable cards. Each card shows document_type,
	 * issuing→destination, and apostille_required badge. Expanding reveals
	 * pathway_steps as an ordered list. Authed users can submit community
	 * updates; a PDF export link is shown.
	 */
	let { data } = $props();

	let destinations = $state<GeoSpaceOption[]>(untrack(() => data.destinations));
	let pathways = $state<AttestationPathwayWithAi[]>(untrack(() => data.pathways));
	let user = $state(untrack(() => data.user));

	let selectedDestId = $state<string>(untrack(() => data.initialDestId ?? ''));
	let selectedIssuingId = $state<string>(untrack(() => data.initialIssuingId ?? ''));
	let docTypeFilter = $state<string>(untrack(() => data.initialDocType ?? ''));
	let loading = $state(false);
	let loadingMore = $state(false);
	let pathwaysTotal = $state<number>(untrack(() => data.pathwaysTotal ?? 0));
	let currentPage = $state(1); // 1-based UI page; API takes a row offset via page_offset
	let expandedId = $state<string | null>(null);
	const PAGE_SIZE = 25;
	const hasMore = $derived(pathways.length < pathwaysTotal);

	// Community update form state
	let showRegisterPrompt = $state(false);
	let updatePathwayId = $state<string | null>(null);
	let updateStepIndex = $state<string>('');
	let updateFeeEstimate = $state<string>('');
	let updateTurnaroundDays = $state<string>('');
	let updateNote = $state<string>('');
	let submittingUpdate = $state(false);
	let updateError = $state<string | null>(null);

	const seo = $derived(
		buildSeoMeta({
			title: t('attestation_guide.seo_title', 'Attestation Guide — {site} Tools'),
			description: t(
				'attestation_guide.seo_description',
				'Step-by-step document attestation pathways — apostille requirements, steps, fees, and community-verified updates.'
			),
			path: '/attestation-guide',
			origin: page.url.origin,
			ogImage: `${page.url.origin}/og/og-attestation-guide.png`,
			jsonLd: {
				'@context': 'https://schema.org',
				'@type': 'HowTo',
				name: 'Document Attestation Guide',
				description: 'Step-by-step document attestation pathways — apostille requirements, steps, and fees.',
				step: pathways.slice(0, 1).flatMap((p) =>
					(Array.isArray(p.pathway_steps) ? p.pathway_steps : []).slice(0, 10).map((s, i) => ({
						'@type': 'HowToStep',
						position: i + 1,
						name: typeof s === 'object' && s !== null ? (s as Record<string, unknown>).title as string ?? `Step ${i + 1}` : `Step ${i + 1}`,
						text: typeof s === 'string' ? s : JSON.stringify(s)
					}))
				)
			}
		})
	);

	function pathwaysQuery(pageNum: number): string {
		const params = new URLSearchParams();
		params.set('dest', selectedDestId);
		params.set('page_offset', String((pageNum - 1) * PAGE_SIZE));
		params.set('page_size', String(PAGE_SIZE));
		if (selectedIssuingId) params.set('issuing', selectedIssuingId);
		if (docTypeFilter.trim()) params.set('doc_type', docTypeFilter.trim());
		return params.toString();
	}

	async function loadPathways() {
		if (!selectedDestId) {
			pathways = [];
			pathwaysTotal = 0;
			currentPage = 1;
			return;
		}
		loading = true;
		const res = await api.get<AttestationPathwayWithAi[]>(
			`/attestation?${pathwaysQuery(1)}`
		);
		loading = false;
		if (!res.error) {
			pathways = res.data ?? [];
			currentPage = 1;
			pathwaysTotal = res.meta?.total ?? pathways.length;
		} else {
			pathways = [];
			pathwaysTotal = 0;
		}
	}

	async function loadMorePathways() {
		if (!selectedDestId || loadingMore || !hasMore) return;
		loadingMore = true;
		const nextPage = currentPage + 1;
		const res = await api.get<AttestationPathwayWithAi[]>(
			`/attestation?${pathwaysQuery(nextPage)}`
		);
		loadingMore = false;
		if (!res.error) {
			pathways = [...pathways, ...(res.data ?? [])];
			currentPage = nextPage;
			pathwaysTotal = res.meta?.total ?? pathwaysTotal;
		}
	}

	function onDestChange() {
		// selectedDestId is already updated via bind:selectedId.
		expandedId = null;
		selectedIssuingId = '';
		loadPathways();
		updateUrl();
	}

	function onIssuingChange(id: string) {
		selectedIssuingId = id;
		expandedId = null;
		loadPathways();
		updateUrl();
	}

	let docTypeDebounce: ReturnType<typeof setTimeout> | null = null;

	function onDocTypeInput() {
		// Debounce the server-side filter so we don't fire a request per keystroke.
		if (docTypeDebounce) clearTimeout(docTypeDebounce);
		docTypeDebounce = setTimeout(() => {
			loadPathways();
			updateUrl();
			docTypeDebounce = null;
		}, 300);
	}

	function updateUrl() {
		const url = new URL(page.url);
		if (selectedDestId) url.searchParams.set('dest', selectedDestId);
		else url.searchParams.delete('dest');
		if (selectedIssuingId) url.searchParams.set('issuing', selectedIssuingId);
		else url.searchParams.delete('issuing');
		if (docTypeFilter.trim()) url.searchParams.set('doc_type', docTypeFilter.trim());
		else url.searchParams.delete('doc_type');
		replaceState(url, {});
	}

	function toggleExpand(id: string) {
		expandedId = expandedId === id ? null : id;
	}

	/** Render pathway_steps as an ordered list. Each step is a JSON object that
	 *  may have title, description, url — rendered generically. */
	function pathwayStepEntries(step: unknown): [string, unknown][] {
		if (!step || typeof step !== 'object') return [];
		return Object.entries(step as Record<string, unknown>);
	}

	function isStepObject(step: unknown): step is Record<string, unknown> {
		return step !== null && typeof step === 'object' && !Array.isArray(step);
	}

	/** Pathway step values may be nested objects — render them as readable text. */
	function stepValueText(val: unknown): string {
		if (val === null || val === undefined) return '';
		if (typeof val === 'object') return JSON.stringify(val);
		return String(val);
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

	/** ADR-105: community_last_verified older than 180 days (or NULL) is stale. */
	const STALENESS_THRESHOLD_DAYS = 180;

	function isStale(iso?: string | null): boolean {
		if (!iso) return true;
		try {
			const verified = new Date(iso);
			const daysOld = (Date.now() - verified.getTime()) / (1000 * 60 * 60 * 24);
			return daysOld > STALENESS_THRESHOLD_DAYS;
		} catch {
			return true;
		}
	}

	function openUpdateForm(pathwayId: string) {
		if (!user) {
			showRegisterPrompt = true;
			return;
		}
		updatePathwayId = pathwayId;
		updateStepIndex = '';
		updateFeeEstimate = '';
		updateTurnaroundDays = '';
		updateNote = '';
		updateError = null;
	}

	function closeUpdateForm() {
		updatePathwayId = null;
		updateError = null;
	}

	async function submitUpdate() {
		if (!user) {
			showRegisterPrompt = true;
			return;
		}
		if (!updatePathwayId) return;
		submittingUpdate = true;
		updateError = null;
		const body: Record<string, unknown> = {};
		if (updateStepIndex) body.step_index = parseInt(updateStepIndex, 10);
		if (updateFeeEstimate) body.fee_estimate = updateFeeEstimate;
		if (updateTurnaroundDays) body.turnaround_days = parseInt(updateTurnaroundDays, 10);
		if (updateNote) body.note = updateNote;
		const res = await api.post<AttestationCommunityUpdate>(
			`/attestation/${updatePathwayId}/updates`,
			body
		);
		submittingUpdate = false;
		if (res.error) {
			updateError = res.error.message;
			return;
		}
		closeUpdateForm();
	}

	function fetchDestinations(q: string): Promise<AutocompleteItem[]> {
		const query = q.toLowerCase();
		const filtered = query === ''
			? destinations
			: destinations.filter((d) => d.name.toLowerCase().includes(query));
		return Promise.resolve(filtered.map((d) => ({ id: d.id, label: d.name })));
	}

	function fetchDestinationsInitial(): Promise<AutocompleteItem[]> {
		return fetchDestinations('');
	}

	function getDestinationLabel(id: string): string {
		return destinations.find((d) => d.id === id)?.name ?? id;
	}
</script>

<SeoHead seo={seo} ogType="website" twitterCard="summary" />

<div class="mx-auto max-w-4xl px-4 py-8">
	{#if !data.enabled}
		<ToolDisabled tool={data.tool} />
	{:else}
	<div class="mb-8">
		<h1 class="text-2xl sm:text-3xl font-bold text-text">
			{t('attestation_guide.title', 'Attestation Guide')}
		</h1>
		<p class="mt-2 text-text-subtle">
			{t(
				'attestation_guide.subtitle',
				'Step-by-step document attestation pathways — apostille requirements, steps, fees, and community-verified updates.'
			)}
		</p>
	</div>

	{#if destinations.length === 0}
		<EmptyState
			icon="stamp"
			title={t('attestation_guide.no_destinations', 'No destinations available')}
			description={t(
				'attestation_guide.no_destinations_desc',
				"Attestation guide data hasn't been configured yet. Please check back later."
			)}
		/>
	{:else}
		<!-- Destination selector (geo-aware pattern — first input) -->
		<div class="mb-4">
			<DestinationSelector
				{destinations}
				bind:selectedId={selectedDestId}
				onchange={onDestChange}
			/>
		</div>

		{#if selectedDestId}
			<!-- Optional filters: issuing country + doc type -->
			<div class="mb-6 flex flex-wrap items-end gap-4">
				<div class="flex items-center gap-3">
					<label for="issuing-selector" class="text-sm font-medium text-text whitespace-nowrap">
						{t('attestation_guide.issuing_country', 'Issuing country:')}
					</label>
					<Autocomplete
						id="issuing-selector"
						placeholder={t('attestation_guide.any_issuing', 'Any')}
						emptyMessage={t('geo.no_destinations', 'No destinations found')}
						fetcher={fetchDestinations}
						initialFetcher={fetchDestinationsInitial}
						selectedId={selectedIssuingId || null}
						selectedLabel={selectedIssuingId ? getDestinationLabel(selectedIssuingId) : null}
						onSelect={(item) => onIssuingChange(item.id)}
						onClear={() => onIssuingChange('')}
					/>
				</div>
				<div class="flex items-center gap-3">
					<label for="doc-type" class="text-sm font-medium text-text whitespace-nowrap">
						{t('attestation_guide.doc_type', 'Document type:')}
					</label>
					<input
						id="doc-type"
						type="text"
						bind:value={docTypeFilter}
						oninput={onDocTypeInput}
						placeholder={t('attestation_guide.doc_type_placeholder', 'e.g. birth certificate')}
						class="px-4 py-2 bg-surface border border-border rounded-lg text-text focus:border-primary focus:outline-none"
					/>
				</div>
			</div>
		{/if}

		{#if loading}
			<div class="space-y-3">
				<LoadingSkeleton height="80px" />
				<LoadingSkeleton height="80px" />
				<LoadingSkeleton height="80px" />
			</div>
		{:else if !selectedDestId}
			<EmptyState
				icon="map-pin"
				title={t('attestation_guide.select_destination', 'Select a destination')}
				description={t(
					'attestation_guide.select_destination_desc',
					'Choose a country to see attestation pathways for that destination.'
				)}
			/>
		{:else if pathways.length === 0}
			<EmptyState
				icon="stamp"
				title={t('attestation_guide.no_pathways', 'No attestation pathways')}
				description={t(
					'attestation_guide.no_pathways_desc',
					'No attestation pathways have been published for this destination yet.'
				)}
			/>
		{:else}
			<div class="space-y-3">
				{#each pathways as pathway (pathway.id)}
					<div class="bg-surface border border-border rounded-lg overflow-hidden">
						<!-- Card header (click to expand) -->
						<button
							onclick={() => toggleExpand(pathway.id)}
							class="w-full flex items-center justify-between p-4 text-left hover:bg-surface-subtle transition-colors"
							aria-expanded={expandedId === pathway.id}
						>
							<div class="min-w-0">
								<div class="flex items-center gap-2 flex-wrap">
									<span class="text-sm font-semibold text-text">
										{pathway.document_type}
									</span>
									{#if pathway.apostille_required}
										<span class="text-xs px-2 py-0.5 rounded-full font-medium" style="background: color-mix(in srgb, var(--color-primary) 15%, transparent); color: var(--color-primary)">
											{t('attestation_guide.apostille_required', 'Apostille required')}
										</span>
									{:else}
										<span class="text-xs px-2 py-0.5 rounded-full font-medium" style="background: color-mix(in srgb, var(--color-muted) 15%, transparent); color: var(--color-muted)">
											{t('attestation_guide.no_apostille', 'No apostille')}
										</span>
									{/if}
								</div>
								<p class="text-xs text-muted mt-1">
									{pathway.issuing_space_name ?? t('attestation_guide.unknown_issuing', 'Unknown')}
									→
									{pathway.destination_space_name ?? t('attestation_guide.unknown_dest', 'Unknown')}
								</p>
							</div>
							<Icon
								name={expandedId === pathway.id ? 'chevron-up' : 'chevron-down'}
								size={20}
								class="text-muted flex-shrink-0"
							/>
						</button>

						<!-- Expanded detail -->
						{#if expandedId === pathway.id}
							<div class="border-t border-border p-4 space-y-4">
								<!-- Pathway steps -->
								{#if Array.isArray(pathway.pathway_steps) && pathway.pathway_steps.length > 0}
									<div>
										<h3 class="text-sm font-semibold text-muted uppercase tracking-wide mb-2">
											{t('attestation_guide.pathway_steps', 'Pathway steps')}
										</h3>
										<ol class="space-y-3">
											{#each pathway.pathway_steps as step, i (i)}
												<li class="flex items-start gap-3">
													<span class="flex-shrink-0 w-6 h-6 rounded-full bg-[var(--tool-accent)]/10 text-[var(--tool-accent)] text-xs font-semibold flex items-center justify-center">
														{i + 1}
													</span>
													<div class="text-sm text-text min-w-0">
														{#if isStepObject(step)}
															{#each pathwayStepEntries(step) as [key, val] (key)}
																<div class="flex items-start gap-1.5">
																	<span class="font-medium text-text">{key}:</span>
																	{#if key === 'url' && typeof val === 'string'}
																		<a
																			href={val}
																			target="_blank"
																			rel="noopener noreferrer"
																			class="text-primary hover:underline break-all"
																		>
																			{val}
																		</a>
																	{:else}
																		<span class="text-text-subtle">{stepValueText(val)}</span>
																	{/if}
																</div>
															{/each}
														{:else}
															<span class="text-text-subtle">{stepValueText(step)}</span>
														{/if}
													</div>
												</li>
											{/each}
										</ol>
									</div>
								{/if}

								<!-- Community last verified + staleness warning (ADR-105) -->
								{#if pathway.community_last_verified}
									<p class="text-xs text-muted">
										{t('attestation_guide.community_last_verified', 'Community last verified:')}
										{formatDate(pathway.community_last_verified)}
									</p>
								{/if}
								{#if isStale(pathway.community_last_verified)}
									<p
										class="text-xs px-3 py-2 rounded-lg"
										style="background: color-mix(in srgb, var(--color-warning, #f59e0b) 10%, transparent); color: var(--color-warning, #f59e0b)"
									>
										{t(
											'attestation_guide.stale_warning',
											'This pathway has not been community-verified recently. Details may be outdated — please confirm with official sources.'
										)}
									</p>
								{/if}

								<!-- Actions: PDF export + community update + view detail -->
								<div class="flex flex-wrap items-center gap-3 pt-2 border-t border-border">
									<a
										href={`/attestation-guide/${pathway.id}`}
										class="inline-flex items-center gap-1.5 px-3 py-1.5 text-sm font-medium text-[var(--tool-accent)] bg-surface border border-border rounded-lg hover:border-[var(--tool-accent)] transition-colors"
									>
										<Icon name="external-link" size={16} />
										{t('attestation_guide.view_pathway', 'View pathway')}
									</a>
									<a
										href={buildApiUrl(`/attestation/${pathway.id}/export/pdf`, PUBLIC_JC_API_URL)}
										target="_blank"
										rel="noopener noreferrer"
										class="inline-flex items-center gap-1.5 px-3 py-1.5 text-sm font-medium text-text bg-surface border border-border rounded-lg hover:border-primary hover:text-primary transition-colors"
									>
										<Icon name="download" size={16} />
										{t('attestation_guide.download_pdf', 'Export pathway')}
									</a>
									<button
										onclick={() => openUpdateForm(pathway.id)}
										class="inline-flex items-center gap-1.5 px-3 py-1.5 text-sm font-medium text-white bg-[var(--tool-accent)] rounded-lg hover:bg-[var(--tool-accent-hover)] transition-colors"
									>
										<Icon name="message-square-plus" size={16} />
										{t('attestation_guide.add_update', 'Add community update')}
									</button>
								</div>

								<!-- Community update form -->
								{#if updatePathwayId === pathway.id}
									<div class="p-4 bg-surface-subtle border border-border rounded-lg space-y-3">
										<h4 class="text-sm font-semibold text-text">
											{t('attestation_guide.update_title', 'Submit a community update')}
										</h4>
										{#if updateError}
											<p class="text-sm" style="color: var(--color-danger)">{updateError}</p>
										{/if}
										<div class="grid grid-cols-1 sm:grid-cols-3 gap-3">
											<div>
												<label for="upd-step" class="block text-xs font-medium text-muted mb-1">
													{t('attestation_guide.step_index', 'Step index')}
												</label>
												<input
													id="upd-step"
													type="number"
													bind:value={updateStepIndex}
													min="0"
													class="w-full px-3 py-2 text-sm bg-surface border border-border rounded focus:border-primary focus:outline-none"
												/>
											</div>
											<div>
												<label for="upd-fee" class="block text-xs font-medium text-muted mb-1">
													{t('attestation_guide.fee_estimate', 'Fee estimate')}
												</label>
												<input
													id="upd-fee"
													type="text"
													bind:value={updateFeeEstimate}
													placeholder="e.g. ₦15,000"
													class="w-full px-3 py-2 text-sm bg-surface border border-border rounded focus:border-primary focus:outline-none"
												/>
											</div>
											<div>
												<label for="upd-turnaround" class="block text-xs font-medium text-muted mb-1">
													{t('attestation_guide.turnaround_days', 'Turnaround (days)')}
												</label>
												<input
													id="upd-turnaround"
													type="number"
													bind:value={updateTurnaroundDays}
													min="0"
													class="w-full px-3 py-2 text-sm bg-surface border border-border rounded focus:border-primary focus:outline-none"
												/>
											</div>
										</div>
										<div>
											<label for="upd-note" class="block text-xs font-medium text-muted mb-1">
												{t('attestation_guide.note', 'Note')}
											</label>
											<textarea
												id="upd-note"
												bind:value={updateNote}
												rows="3"
												placeholder={t('attestation_guide.note_placeholder', 'Share your experience with this step...')}
												class="w-full px-3 py-2 text-sm bg-surface border border-border rounded focus:border-primary focus:outline-none"
											></textarea>
										</div>
										<div class="flex items-center gap-2">
											<button
												onclick={submitUpdate}
												disabled={submittingUpdate}
												class="px-4 py-2 text-sm font-medium text-white bg-[var(--tool-accent)] rounded-lg hover:bg-[var(--tool-accent-hover)] transition-colors disabled:opacity-50"
											>
												{submittingUpdate
													? t('common.submitting', 'Submitting...')
													: t('common.submit', 'Submit')}
											</button>
											<button
												onclick={closeUpdateForm}
												class="px-4 py-2 text-sm text-muted hover:text-text transition-colors"
											>
												{t('common.cancel', 'Cancel')}
											</button>
										</div>
									</div>
								{/if}
							</div>
						{/if}
					</div>
				{/each}

				<!-- Pagination — consumes the endpoint's page/page_size contract -->
				<div class="flex items-center justify-between pt-2">
					<p class="text-xs text-muted">
						{t('common.showing_of', 'Showing {n} of {total}')
							.replace('{n}', String(pathways.length))
							.replace('{total}', String(pathwaysTotal))}
					</p>
					{#if hasMore}
						<button
							type="button"
							onclick={loadMorePathways}
							disabled={loadingMore}
							class="px-4 py-2 text-sm font-medium text-text bg-surface border border-border rounded-lg hover:border-primary hover:text-primary transition-colors disabled:opacity-50"
						>
							{loadingMore ? t('common.loading', 'Loading...') : t('common.load_more', 'Load more')}
						</button>
					{/if}
				</div>
			</div>
		{/if}
	{/if}

	{#if showRegisterPrompt}
		<RegistrationPrompt
			open={showRegisterPrompt}
			onClose={() => (showRegisterPrompt = false)}
			icon="message-square-plus"
			title={t('register.add_update', 'Add a community update')}
			description={t(
				'register.add_update_desc',
				'Create a free account to share your attestation experience and help others navigate the process.'
			)}
		/>
	{/if}
	{/if}
</div>
