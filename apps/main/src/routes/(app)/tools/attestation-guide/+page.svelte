<script lang="ts">
	import { untrack } from 'svelte';
	import { goto } from '$app/navigation';
	import { api } from '#lib/api/client.js';
	import BodyField from '#lib/components/ui/BodyField.svelte';
	import { t, siteName, i18n } from '#lib/i18n/index.js';
	import { page } from '$app/state';
	import {
		Stamp,
		MapPin,
		ChevronUp,
		ChevronDown,
		Download,
		MessageSquarePlus,
		Lock,
		BookOpen
	} from 'lucide-svelte';
	import { EmptyState, LoadingSkeleton } from '#lib/components/ui/index.js';
	import type { AttestationPathwayWithAi, AttestationCommunityUpdate, GeoCountry } from '#lib/types/index.js';
	import { DOC_TYPES, docTypeLabel, isStepObject, stepFields, stepHeadline, stepValueText } from '#lib/attestation/steps.js';
	import type { PageData } from './$types';
	import Autocomplete from '#lib/components/create/Autocomplete.svelte';
	import type { AutocompleteItem } from '#lib/components/create/Autocomplete.svelte';
	import ToolHeader from '#lib/components/tools/ToolHeader.svelte';
	import SeoHead from '#lib/seo/SeoHead.svelte';
	import { toolOgCard } from '#lib/seo/og.js';

	/**
	 * Attestation Guide tool — §97, ADR-088.
	 * Destination selector → optional issuing country + doc_type filters →
	 * list of pathways as expandable cards. Each card shows document_type,
	 * issuing→destination, and apostille_required badge. Expanding reveals
	 * pathway_steps as an ordered list. Authed users can submit community
	 * updates; a PDF export link is shown.
	 */
	let { data }: { data: PageData } = $props();

	const title = $derived(t('attestation_guide.title', 'Attestation Guide'));
	const description = $derived(
		t(
			'attestation_guide.subtitle',
			'Step-by-step document attestation pathways — apostille requirements, steps, fees, and community-verified updates.'
		)
	);
	const disabled = $derived(data.disabled);
	const authed = $derived(data.authed);

	let destinations = $state<GeoCountry[]>(untrack(() => data.destinations));
	let pathways = $state<AttestationPathwayWithAi[]>(untrack(() => data.pathways));
	let pathwaysTotal = $state<number>(untrack(() => data.pathwaysTotal));

	let selectedDestId = $state<string>(untrack(() => data.initialDestId ?? ''));
	let selectedIssuingId = $state<string>(untrack(() => data.initialIssuingId ?? ''));
	let docTypeFilter = $state<string>(untrack(() => data.initialDocType ?? ''));
	let loading = $state(false);
	let loadingMore = $state(false);
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
	let updateSuccessId = $state<string | null>(null);
	let loadFailed = $state(false);

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
		loadFailed = false;
		const res = await api.getEnvelope<AttestationPathwayWithAi[]>(
			`/attestation?${pathwaysQuery(1)}`
		);
		loading = false;
		if (!res.error) {
			pathways = res.data ?? [];
			currentPage = 1;
			pathwaysTotal = readTotal(res.envelope) ?? pathways.length;
		} else {
			pathways = [];
			pathwaysTotal = 0;
			loadFailed = true;
		}
	}

	async function loadMorePathways() {
		if (!selectedDestId || loadingMore || !hasMore) return;
		loadingMore = true;
		const nextPage = currentPage + 1;
		const res = await api.getEnvelope<AttestationPathwayWithAi[]>(
			`/attestation?${pathwaysQuery(nextPage)}`
		);
		loadingMore = false;
		if (!res.error) {
			pathways = [...pathways, ...res.data ?? []];
			currentPage = nextPage;
			pathwaysTotal = readTotal(res.envelope) ?? pathwaysTotal;
		}
	}

	function readTotal(envelope: Record<string, unknown> | null): number | null {
		const p = envelope?.pagination as { total?: number } | undefined;
		return typeof p?.total === 'number' ? p.total : null;
	}

	function onDestChange(id: string) {
		selectedDestId = id;
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

	function onDocTypeChange() {
		loadPathways();
		updateUrl();
	}

	function updateUrl() {
		const url = new URL(page.url.href);

		if (selectedDestId) url.searchParams.set('dest', selectedDestId); else url.searchParams.delete('dest');
		if (selectedIssuingId) url.searchParams.set('issuing', selectedIssuingId); else url.searchParams.delete('issuing');
		if (docTypeFilter.trim()) url.searchParams.set('doc_type', docTypeFilter.trim()); else url.searchParams.delete('doc_type');

		goto(url, { shallow: true, replace: true });
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
		if (!authed) {
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
		if (!updatePathwayId) return;
		// Client-side mirror of the backend rule: a submission must carry at
		// least one substantive field (a step selection alone is a locator).
		if (!updateFeeEstimate.trim() && !updateTurnaroundDays && !updateNote.trim()) {
			updateError = t(
				'attestation_guide.update_required',
				'Add a fee, turnaround, or note before submitting.'
			);
			return;
		}
		submittingUpdate = true;
		updateError = null;
		const body: Record<string, unknown> = {};
		if (updateStepIndex !== '') body.step_index = parseInt(updateStepIndex, 10);
		if (updateFeeEstimate.trim()) body.fee_estimate = updateFeeEstimate.trim();
		if (updateTurnaroundDays !== '') body.turnaround_days = parseInt(updateTurnaroundDays, 10);
		if (updateNote.trim()) body.note = updateNote.trim();
		const res = await api.post<AttestationCommunityUpdate>(
			`/attestation/${updatePathwayId}/updates`,
			body
		);
		submittingUpdate = false;
		if (res.error) {
			updateError = res.error.message;
			return;
		}
		updateSuccessId = updatePathwayId;
		closeUpdateForm();
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
	const selectedIssuingLabel = $derived(
		selectedIssuingId ? destinations.find((d) => d.id === selectedIssuingId)?.name ?? null : null
	);
</script>

<SeoHead
	title="{title} — {siteName()} Tools"
	description={t(
		'attestation_guide.seo_description',
		'Step-by-step document attestation pathways — apostille requirements, steps, fees, and community-verified updates.'
	)}
	ogImage={toolOgCard(page.url.origin, page.url.pathname)}
/>

<div class="mx-auto w-full max-w-3xl">
	<div class="px-4 pt-4">
		<ToolHeader icon={Stamp} title={title} description={description} />
		<!-- Cross-link to the editorial attestation guides (§97 editorial surface). -->
		<a
			href="/guides?category=attestation"
			class="mt-3 inline-flex items-center gap-1.5 text-sm font-medium text-[var(--color-primary-text)] hover:underline"
		>
			<BookOpen size={16} />
			{t('attestation_guide.read_editorial', 'Read in-depth attestation guides')}
		</a>
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
				icon={Stamp}
				title={t('attestation_guide.no_destinations', 'No destinations available')}
				body={t(
					'attestation_guide.no_destinations_desc',
					"Attestation guide data hasn't been configured yet. Please check back later."
				)}
			/>
		</div>
	{:else}
		<!-- Destination selector (geo-aware pattern — first input) -->
		<div class="border-b border-[var(--color-border)] px-4 py-3">
			<label
				for="ag-dest"
				class="block text-xs font-medium uppercase tracking-wide text-[var(--color-muted)]"
			>
				{t('geo.destination', 'Destination:')}
			</label>
			<div class="mt-2">
				<Autocomplete
					placeholder={t('geo.select_destination', 'Select a country…')}
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

		{#if selectedDestId}
			<!-- Optional filters: issuing country + doc type -->
			<div class="border-b border-[var(--color-border)] px-4 py-3 flex flex-wrap items-end gap-4">
				<div class="flex flex-col gap-1.5">
					<label for="issuing-selector" class="text-sm font-medium text-[var(--color-text)]">
						{t('attestation_guide.issuing_country', 'Issuing country:')}
					</label>
					<Autocomplete
						id="issuing-selector"
						placeholder={t('attestation_guide.any_issuing', 'Any')}
						emptyMessage={t('common.no_results', 'No results found')}
						fetcher={fetchDestinations}
						initialFetcher={fetchDestinationsInitial}
						selectedId={selectedIssuingId || null}
						selectedLabel={selectedIssuingLabel}
						onSelect={(item) => onIssuingChange(item.id)}
						onClear={() => onIssuingChange('')}
					/>
				</div>
				<div class="flex flex-col gap-1.5">
					<label for="doc-type" class="text-sm font-medium text-[var(--color-text)]">
						{t('attestation_guide.doc_type', 'Document type:')}
					</label>
					<select
						id="doc-type"
						bind:value={docTypeFilter}
						onchange={onDocTypeChange}
						class="px-4 py-2 bg-[var(--color-surface)] border border-[var(--color-border)] rounded-[var(--radius)] text-[var(--color-text)] focus:border-[var(--color-primary)] focus:outline-none"
					>
						<option value="">{t('attestation_guide.any_doc_type', 'Any')}</option>
						{#each DOC_TYPES as dt (dt.value)}
							<option value={dt.value}>{t(`attestation_guide.doc_type_${dt.value}`, dt.label)}</option>
						{/each}
					</select>
				</div>
			</div>
		{/if}

		<div class="p-4">
			{#if loading}
				<div class="space-y-3">
					<LoadingSkeleton variant="card" count={3} />
				</div>
			{:else if !selectedDestId}
				<EmptyState
					icon={MapPin}
					title={t('attestation_guide.select_destination', 'Select a destination')}
					body={t(
						'attestation_guide.select_destination_desc',
						'Choose a country to see attestation pathways for that destination.'
					)}
				/>
			{:else if pathways.length === 0}
				<EmptyState
					icon={Stamp}
					title={loadFailed
						? t('attestation_guide.load_failed', 'Couldn\u2019t load pathways')
						: t('attestation_guide.no_pathways', 'No attestation pathways')}
					body={loadFailed
						? t('attestation_guide.load_failed_desc', 'Something went wrong fetching pathways — please try again.')
						: t(
							'attestation_guide.no_pathways_desc',
							'No attestation pathways have been published for this destination yet.'
						)}
				/>
			{:else}
				<div class="space-y-3">
					{#each pathways as pathway (pathway.id)}
						<div class="bg-[var(--color-surface)] border border-[var(--color-border)] rounded-[var(--radius-card)] overflow-hidden">
							<!-- Card header (click to expand) -->
							<button
								type="button"
								onclick={() => toggleExpand(pathway.id)}
								class="w-full flex items-center justify-between p-4 text-start hover:bg-[var(--color-surface-subtle)] transition-colors"
								aria-expanded={expandedId === pathway.id}
							>
								<div class="min-w-0">
									<div class="flex items-center gap-2 flex-wrap">
										<span class="text-sm font-semibold text-[var(--color-text)]">
											{docTypeLabel(pathway.document_type)}
										</span>
										{#if pathway.apostille_required}
											<span class="text-xs px-2 py-0.5 rounded-full font-medium" style="background: color-mix(in srgb, var(--color-primary) 15%, transparent); color: var(--color-primary-text)">
												{t('attestation_guide.apostille_required', 'Apostille required')}
											</span>
										{:else}
											<span class="text-xs px-2 py-0.5 rounded-full font-medium" style="background: color-mix(in srgb, var(--color-muted) 15%, transparent); color: var(--color-muted)">
												{t('attestation_guide.no_apostille', 'No apostille')}
											</span>
										{/if}
									</div>
									<p class="text-xs text-[var(--color-muted)] mt-1">
										{pathway.issuing_space_name ?? t('attestation_guide.unknown_issuing', 'Unknown')}
										→
										{pathway.destination_space_name ?? t('attestation_guide.unknown_dest', 'Unknown')}
									</p>
								</div>
								{#if expandedId === pathway.id}
									<ChevronUp size={20} class="text-[var(--color-muted)] flex-shrink-0" />
								{:else}
									<ChevronDown size={20} class="text-[var(--color-muted)] flex-shrink-0" />
								{/if}
							</button>

							<!-- Expanded detail -->
							{#if expandedId === pathway.id}
								<div class="border-t border-[var(--color-border)] p-4 space-y-4">
									<!-- Pathway steps -->
									{#if Array.isArray(pathway.pathway_steps) && pathway.pathway_steps.length > 0}
										<div>
											<h3 class="text-sm font-semibold text-[var(--color-muted)] uppercase tracking-wide mb-2">
												{t('attestation_guide.pathway_steps', 'Pathway steps')}
											</h3>
											<ol class="space-y-3">
												{#each pathway.pathway_steps as step, i (i)}
													<li class="flex items-start gap-3">
														<span class="flex-shrink-0 w-6 h-6 rounded-full bg-[var(--color-primary)]/10 text-[var(--color-primary-text)] text-xs font-semibold flex items-center justify-center">
															{i + 1}
														</span>
														<div class="text-sm text-[var(--color-text)] min-w-0">
															{#if isStepObject(step)}
																<p class="font-medium text-[var(--color-text)]">
																	{stepHeadline(step, i)}
																</p>
																{#each stepFields(step) as field (field.label)}
																	<div class="flex items-start gap-1.5 mt-0.5">
																		<span class="text-xs font-medium text-[var(--color-muted)] uppercase tracking-wide">{field.label}:</span>
																		{#if field.url}
																			<a
																				href={field.url}
																				target="_blank"
																				rel="noopener noreferrer"
																				class="text-[var(--color-primary-text)] hover:underline break-all"
																			>
																				{field.value}
																			</a>
																		{:else}
																			<span class="text-[var(--color-text-subtle)]">{field.value}</span>
																		{/if}
																	</div>
																{/each}
															{:else}
																<span class="text-[var(--color-text-subtle)]">{stepValueText(step)}</span>
															{/if}
														</div>
													</li>
												{/each}
											</ol>
										</div>
									{/if}

									<!-- Community last verified + staleness warning (ADR-105) -->
									{#if pathway.community_last_verified}
										<p class="text-xs text-[var(--color-muted)]">
											{t('attestation_guide.community_last_verified', 'Community last verified:')}
											{formatDate(pathway.community_last_verified)}
										</p>
									{/if}
									{#if isStale(pathway.community_last_verified)}
										<p
											class="text-xs px-3 py-2 rounded-[var(--radius)]"
											style="background: color-mix(in srgb, var(--color-warning, #f59e0b) 10%, transparent); color: var(--color-warning, #f59e0b)"
										>
											{t(
												'attestation_guide.stale_warning',
												'This pathway has not been community-verified recently. Details may be outdated — please confirm with official sources.'
											)}
										</p>
									{/if}

									<!-- Actions: view detail + PDF export + community update -->
									<div class="flex flex-wrap items-center gap-3 pt-2 border-t border-[var(--color-border)]">
										<a
											href="/tools/attestation-guide/{pathway.id}"
										class="inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-1.5 text-sm font-medium text-[var(--color-text)] hover:border-[var(--color-primary)] hover:text-[var(--color-primary-text)] transition-colors"
									>
										{t('common.view_details', 'View details')}
									</a>
									<a
										href="/tools/attestation-guide/{pathway.id}?print=1"
											class="inline-flex items-center gap-1.5 px-3 py-1.5 text-sm font-medium text-[var(--color-text)] bg-[var(--color-surface)] border border-[var(--color-border)] rounded-[var(--radius)] hover:border-[var(--color-primary)] hover:text-[var(--color-primary-text)] transition-colors"
										>
											<Download size={16} />
											{t('attestation_guide.download_pdf', 'Export PDF')}
										</a>
										<button
											type="button"
											onclick={() => openUpdateForm(pathway.id)}
											class="inline-flex items-center gap-1.5 px-3 py-1.5 text-sm font-medium text-[var(--color-primary-contrast)] bg-[var(--color-primary)] rounded-[var(--radius)] hover:bg-[var(--color-primary-hover)] transition-colors"
										>
											<MessageSquarePlus size={16} />
											{t('attestation_guide.add_update', 'Add community update')}
										</button>
									</div>

									{#if updateSuccessId === pathway.id}
										<p class="text-xs" style="color: var(--color-success, #16a34a)">
											{t(
												'attestation_guide.update_submitted',
												'Update submitted — it will appear after moderator review.'
											)}
										</p>
									{/if}

									<!-- Community update form -->
									{#if updatePathwayId === pathway.id}
										<div class="p-4 bg-[var(--color-surface-subtle)] border border-[var(--color-border)] rounded-[var(--radius)] space-y-3">
											<h4 class="text-sm font-semibold text-[var(--color-text)]">
												{t('attestation_guide.update_title', 'Submit a community update')}
											</h4>
											{#if updateError}
												<p class="text-sm" style="color: var(--color-danger)">{updateError}</p>
											{/if}
											<div class="grid grid-cols-1 sm:grid-cols-3 gap-3">
												<div>
													<label for="upd-step" class="block text-xs font-medium text-[var(--color-muted)] mb-1">
														{t('attestation_guide.step_index', 'Which step?')}
													</label>
													<select
														id="upd-step"
														bind:value={updateStepIndex}
														class="w-full px-3 py-2 text-sm bg-[var(--color-surface)] border border-[var(--color-border)] rounded-[var(--radius)] focus:border-[var(--color-primary)] focus:outline-none"
													>
														<option value="">{t('attestation_guide.step_general', 'General note')}</option>
														{#each (Array.isArray(pathway.pathway_steps) ? pathway.pathway_steps : []) as _s, i (i)}
															<option value={String(i)}>
																{t('attestation_guide.step_n', 'Step')} {i + 1}
															</option>
														{/each}
													</select>
												</div>
												<div>
													<label for="upd-fee" class="block text-xs font-medium text-[var(--color-muted)] mb-1">
														{t('attestation_guide.fee_estimate', 'Fee estimate')}
													</label>
													<input
														id="upd-fee"
														type="text"
														bind:value={updateFeeEstimate}
														placeholder="e.g. ₦15,000"
														class="w-full px-3 py-2 text-sm bg-[var(--color-surface)] border border-[var(--color-border)] rounded-[var(--radius)] focus:border-[var(--color-primary)] focus:outline-none"
													/>
												</div>
												<div>
													<label for="upd-turnaround" class="block text-xs font-medium text-[var(--color-muted)] mb-1">
														{t('attestation_guide.turnaround_days', 'Turnaround (days)')}
													</label>
													<input
														id="upd-turnaround"
														type="number"
														bind:value={updateTurnaroundDays}
														min="0"
														class="w-full px-3 py-2 text-sm bg-[var(--color-surface)] border border-[var(--color-border)] rounded-[var(--radius)] focus:border-[var(--color-primary)] focus:outline-none"
													/>
												</div>
											</div>
											<div>
												<BodyField
													id="upd-note"
													label={t('attestation_guide.note', 'Note')}
													body={updateNote}
													onBody={(v) => updateNote = v}
													placeholder={t('attestation_guide.note_placeholder', 'Share your experience with this step...')}
													rows={3}
												/>
											</div>
											<div class="flex items-center gap-2">
												<button
													type="button"
													onclick={submitUpdate}
													disabled={submittingUpdate}
													class="px-4 py-2 text-sm font-medium text-[var(--color-primary-contrast)] bg-[var(--color-primary)] rounded-[var(--radius)] hover:bg-[var(--color-primary-hover)] transition-colors disabled:opacity-50"
												>
													{submittingUpdate
														? t('common.submitting', 'Submitting...')
														: t('common.submit', 'Submit')}
												</button>
												<button
													type="button"
													onclick={closeUpdateForm}
													class="px-4 py-2 text-sm text-[var(--color-muted)] hover:text-[var(--color-text)] transition-colors"
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
						<p class="text-xs text-[var(--color-muted)]">
							{t('common.showing_of', 'Showing {n} of {total}')
								.replace('{n}', String(pathways.length))
								.replace('{total}', String(pathwaysTotal))}
						</p>
						{#if hasMore}
							<button
								type="button"
								onclick={loadMorePathways}
								disabled={loadingMore}
								class="px-4 py-2 text-sm font-medium text-[var(--color-text)] bg-[var(--color-surface)] border border-[var(--color-border)] rounded-[var(--radius)] hover:border-[var(--color-primary)] hover:text-[var(--color-primary-text)] transition-colors disabled:opacity-50"
							>
								{loadingMore ? t('common.loading', 'Loading...') : t('common.load_more', 'Load more')}
							</button>
						{/if}
					</div>
				</div>
			{/if}
		</div>
	{/if}

	{#if showRegisterPrompt}
		<div class="p-4">
			<div class="mb-3 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface-subtle)] px-3 py-2 text-xs text-[var(--color-muted)]">
				{t('register.add_update_desc', 'Create a free account to share your attestation experience and help others navigate the process.')}
				<a href="/login" class="ms-1 text-[var(--color-primary-text)] hover:underline">{t('common.sign_in', 'Sign in')}</a>.
			</div>
		</div>
	{/if}
</div>
