<script lang="ts">
	import { untrack } from 'svelte';
	import { replaceState } from '$app/navigation';
	import { api } from '$lib/api/client';
	import AiBodyField from '$lib/ai/AiBodyField.svelte';
	import { PUBLIC_JC_API_URL } from '$env/static/public';
	import { t } from '$lib/i18n';
	import { page } from '$app/state';
	import {
		Stamp,
		MapPin,
		ChevronUp,
		ChevronDown,
		Download,
		MessageSquarePlus,
		Lock
	} from 'lucide-svelte';
	import { EmptyState, LoadingSkeleton } from '$lib/components/ui';
	import type { AttestationPathwayWithAi, AttestationCommunityUpdate, GeoCountry } from '$lib/types';
	import type { PageData } from './$types';
	import Autocomplete from '$lib/components/create/Autocomplete.svelte';
	import type { AutocompleteItem } from '$lib/components/create/Autocomplete.svelte';

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

	let selectedDestId = $state<string>(untrack(() => data.initialDestId ?? ''));
	let selectedIssuingId = $state<string>('');
	let docTypeFilter = $state<string>('');
	let loading = $state(false);
	let expandedId = $state<string | null>(null);

	// Community update form state
	let showRegisterPrompt = $state(false);
	let updatePathwayId = $state<string | null>(null);
	let updateStepIndex = $state<string>('');
	let updateFeeEstimate = $state<string>('');
	let updateTurnaroundDays = $state<string>('');
	let updateNote = $state<string>('');
	let submittingUpdate = $state(false);
	let updateError = $state<string | null>(null);

	async function loadPathways() {
		if (!selectedDestId) {
			pathways = [];
			return;
		}
		loading = true;
		const params = new URLSearchParams();
		params.set('dest', selectedDestId);
		if (selectedIssuingId) params.set('issuing', selectedIssuingId);
		if (docTypeFilter) params.set('doc_type', docTypeFilter);
		const res = await api.get<AttestationPathwayWithAi[]>(
			`/attestation?${params.toString()}`
		);
		loading = false;
		if (!res.error) pathways = res.data ?? [];
		else pathways = [];
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

	let docTypeDebounce: ReturnType<typeof setTimeout> | null = null;

	function onDocTypeInput() {
		// Debounce the server-side filter so we don't fire a request per keystroke.
		if (docTypeDebounce) clearTimeout(docTypeDebounce);
		docTypeDebounce = setTimeout(() => {
			loadPathways();
			docTypeDebounce = null;
		}, 300);
	}

	function updateUrl() {
		const url = new URL(page.url);
		if (selectedDestId) url.searchParams.set('dest', selectedDestId);
		else url.searchParams.delete('dest');
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

<svelte:head>
	<title>{title} — JapaConnect Tools</title>
	<meta
		name="description"
		content={t(
			'attestation_guide.seo_description',
			'Step-by-step document attestation pathways — apostille requirements, steps, fees, and community-verified updates.'
		)}
	/>
	<link rel="canonical" href={page.url.href} />
	<meta property="og:title" content="{title} — JapaConnect Tools" />
	<meta
		property="og:description"
		content={t(
			'attestation_guide.seo_description',
			'Step-by-step document attestation pathways — apostille requirements, steps, fees, and community-verified updates.'
		)}
	/>
	<meta property="og:url" content={page.url.href} />
	<meta property="og:type" content="website" />
	<meta name="twitter:card" content="summary" />
	<meta name="twitter:title" content="{title} — JapaConnect Tools" />
	<meta
		name="twitter:description"
		content={t(
			'attestation_guide.seo_description',
			'Step-by-step document attestation pathways — apostille requirements, steps, fees, and community-verified updates.'
		)}
	/>
</svelte:head>

<div class="mx-auto w-full max-w-3xl">
	<div class="border-b border-[var(--color-border)] px-4 py-3">
		<h1 class="flex items-center gap-2 text-lg font-semibold text-[var(--color-text)]">
			<Stamp size={20} /> {title}
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
				<div class="flex items-center gap-3">
					<label for="issuing-selector" class="text-sm font-medium text-[var(--color-text)] whitespace-nowrap">
						{t('attestation_guide.issuing_country', 'Issuing country:')}
					</label>
					<select
						id="issuing-selector"
						value={selectedIssuingId}
						onchange={(e) => onIssuingChange((e.currentTarget as HTMLSelectElement).value)}
						class="px-4 py-2 bg-[var(--color-surface)] border border-[var(--color-border)] rounded-[var(--radius)] text-[var(--color-text)] focus:border-[var(--color-primary)] focus:outline-none"
					>
						<option value="">{t('attestation_guide.any_issuing', 'Any')}</option>
						{#each destinations as dest (dest.id)}
							<option value={dest.id}>{dest.name}</option>
						{/each}
					</select>
				</div>
				<div class="flex items-center gap-3">
					<label for="doc-type" class="text-sm font-medium text-[var(--color-text)] whitespace-nowrap">
						{t('attestation_guide.doc_type', 'Document type:')}
					</label>
					<input
						id="doc-type"
						type="text"
						bind:value={docTypeFilter}
						oninput={onDocTypeInput}
						placeholder={t('attestation_guide.doc_type_placeholder', 'e.g. birth certificate')}
						class="px-4 py-2 bg-[var(--color-surface)] border border-[var(--color-border)] rounded-[var(--radius)] text-[var(--color-text)] focus:border-[var(--color-primary)] focus:outline-none"
					/>
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
					title={t('attestation_guide.no_pathways', 'No attestation pathways')}
					body={t(
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
								class="w-full flex items-center justify-between p-4 text-left hover:bg-[var(--color-surface-subtle)] transition-colors"
								aria-expanded={expandedId === pathway.id}
							>
								<div class="min-w-0">
									<div class="flex items-center gap-2 flex-wrap">
										<span class="text-sm font-semibold text-[var(--color-text)]">
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
														<span class="flex-shrink-0 w-6 h-6 rounded-full bg-[var(--color-primary)]/10 text-[var(--color-primary)] text-xs font-semibold flex items-center justify-center">
															{i + 1}
														</span>
														<div class="text-sm text-[var(--color-text)] min-w-0">
															{#if isStepObject(step)}
																{#each pathwayStepEntries(step) as [key, val] (key)}
																	<div class="flex items-start gap-1.5">
																		<span class="font-medium text-[var(--color-text)]">{key}:</span>
																		{#if key === 'url' && typeof val === 'string'}
																			<a
																				href={val}
																				target="_blank"
																				rel="noopener noreferrer"
																				class="text-[var(--color-primary)] hover:underline break-all"
																			>
																				{val}
																			</a>
																		{:else}
																			<span class="text-[var(--color-text-subtle)]">{String(val)}</span>
																		{/if}
																	</div>
																{/each}
															{:else}
																<span class="text-[var(--color-text-subtle)]">{String(step)}</span>
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
										class="inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-1.5 text-sm font-medium text-[var(--color-text)] hover:border-[var(--color-primary)] hover:text-[var(--color-primary)] transition-colors"
									>
										{t('common.view_details', 'View details')}
									</a>
									<a
										href={`${PUBLIC_JC_API_URL}/attestation/${pathway.id}/export/pdf`}
											target="_blank"
											rel="noopener noreferrer"
											class="inline-flex items-center gap-1.5 px-3 py-1.5 text-sm font-medium text-[var(--color-text)] bg-[var(--color-surface)] border border-[var(--color-border)] rounded-[var(--radius)] hover:border-[var(--color-primary)] hover:text-[var(--color-primary)] transition-colors"
										>
											<Download size={16} />
											{t('attestation_guide.download_pdf', 'Export pathway')}
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
														{t('attestation_guide.step_index', 'Step index')}
													</label>
													<input
														id="upd-step"
														type="number"
														bind:value={updateStepIndex}
														min="0"
														class="w-full px-3 py-2 text-sm bg-[var(--color-surface)] border border-[var(--color-border)] rounded-[var(--radius)] focus:border-[var(--color-primary)] focus:outline-none"
													/>
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
												<AiBodyField
													id="upd-note"
													label={t('attestation_guide.note', 'Note')}
													contentType="attestation_guide"
													title={pathway.document_type}
													body={updateNote}
													onBody={(v) => (updateNote = v)}
													placeholder={t('attestation_guide.note_placeholder', 'Share your experience with this step...')}
													rows={3}
													user={data.user}
													features={data.features ?? {}}
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
				</div>
			{/if}
		</div>
	{/if}

	{#if showRegisterPrompt}
		<div class="p-4">
			<div class="mb-3 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface-subtle)] px-3 py-2 text-xs text-[var(--color-muted)]">
				{t('register.add_update_desc', 'Create a free account to share your attestation experience and help others navigate the process.')}
				<a href="/login" class="ml-1 text-[var(--color-primary)] hover:underline">{t('common.sign_in', 'Sign in')}</a>.
			</div>
		</div>
	{/if}
</div>
