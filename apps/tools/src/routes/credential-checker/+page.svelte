<script lang="ts">
	import { untrack, onMount } from 'svelte';
	import { api } from '$lib/api/client';
	import { buildSeoMeta, mainAppUrl } from '$lib/seo';
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
	import type {
		GeoSpaceOption,
		CredentialProfession,
		CredentialPathwayWithAi,
		CredentialCommunityNote,
		CredentialPathwayDetail
	} from '$lib/types';

	/**
	 * Professional Credential Checker tool — §94.
	 * Destination → profession → optional issuing country → pathway cards.
	 * Click a card to expand detail (fetches /credentials/{id} which returns
	 * { pathway, notes }). Authed users can submit community notes.
	 */
	let { data } = $props();

	let destinations = $state<GeoSpaceOption[]>(untrack(() => data.destinations));
	let professions = $state<CredentialProfession[]>(untrack(() => data.professions));
	let pathways = $state<CredentialPathwayWithAi[]>(untrack(() => data.pathways));
	let user = $state(untrack(() => data.user));

	let selectedDest = $state('');
	let selectedProfession = $state('');
	let selectedIssuing = $state('');
	let loadingPathways = $state(false);
	let hasSearched = $state(false);

	// Expanded detail state
	let expandedId = $state<string | null>(null);
	let detailPathway = $state<CredentialPathwayWithAi | null>(null);
	let detailNotes = $state<CredentialCommunityNote[]>([]);
	let loadingDetail = $state(false);

	// Note submission state
	let noteText = $state('');
	let submittingNote = $state(false);
	let showRegisterPrompt = $state(false);

	const seo = $derived(
		buildSeoMeta({
			title: t(
				'credential_checker.seo_title',
				'Professional Credential Checker — {site} Tools'
			),
			description: t(
				'credential_checker.seo_description',
				'Look up credential recognition pathways for your profession — regulatory body, timeline, cost, and steps.'
			),
			path: '/credential-checker',
			origin: page.url.origin,
			ogImage: `${page.url.origin}/og/og-credential-checker.png`,
			jsonLd: {
				'@context': 'https://schema.org',
				'@type': 'HowTo',
				name: 'Professional Credential Recognition',
				description: 'Credential recognition pathways — regulatory body, timeline, cost, and steps.',
				step: pathways.slice(0, 10).map((p, i) => ({
					'@type': 'HowToStep',
					position: i + 1,
					name: `${p.profession_name} — ${p.destination_space_name ?? ''}`,
					text: p.pathway_description
				}))
			}
		})
	);

	// Group professions by category for the selector.
	const professionGroups = $derived.by(() => {
		const groups: Record<string, CredentialProfession[]> = {};
		for (const p of professions) {
			const cat = p.category ?? t('credential_checker.uncategorized', 'Other');
			if (!groups[cat]) groups[cat] = [];
			groups[cat].push(p);
		}
		return groups;
	});

	async function loadPathways() {
		if (!selectedDest || !selectedProfession) return;
		hasSearched = true;
		expandedId = null;
		detailPathway = null;
		detailNotes = [];
		loadingPathways = true;
		const params = new URLSearchParams({
			dest: selectedDest,
			profession: selectedProfession
		});
		if (selectedIssuing) params.set('issuing', selectedIssuing);
		const res = await api.get<CredentialPathwayWithAi[]>(
			`/credentials?${params.toString()}`
		);
		loadingPathways = false;
		if (!res.error) pathways = res.data ?? [];
		else pathways = [];
	}

	function onSearch(e: Event) {
		e.preventDefault();
		loadPathways();
	}

	function onDestChange() {
		pathways = [];
		hasSearched = false;
		expandedId = null;
	}

	async function toggleDetail(pathway: CredentialPathwayWithAi) {
		if (expandedId === pathway.id) {
			expandedId = null;
			detailPathway = null;
			detailNotes = [];
			return;
		}
		expandedId = pathway.id;
		loadingDetail = true;
		// /credentials/{id} returns { pathway, notes } — non-standard envelope.
		const res = await api.get<CredentialPathwayDetail>(`/credentials/${pathway.id}`);
		loadingDetail = false;
		if (!res.error && res.data) {
			detailPathway = res.data.pathway;
			detailNotes = res.data.notes ?? [];
		} else {
			// Fallback: use the list-row data if the detail fetch fails.
			detailPathway = pathway;
			detailNotes = [];
		}
	}

	async function submitNote() {
		if (!expandedId || !noteText.trim()) return;
		if (!user) {
			showRegisterPrompt = true;
			return;
		}
		submittingNote = true;
		const res = await api.post<CredentialCommunityNote>(
			`/credentials/${expandedId}/notes`,
			{ note: noteText.trim() }
		);
		submittingNote = false;
		if (!res.error && res.data) {
			detailNotes = [...detailNotes, res.data];
			noteText = '';
		}
	}

	function fmtDate(iso: string): string {
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

	onMount(() => {
		// If SSR pre-fetched pathways (deep link with ?dest=&profession=),
		// sync the selectors from the URL so the UI reflects the loaded state.
		const destParam = page.url.searchParams.get('dest');
		const professionParam = page.url.searchParams.get('profession');
		if (destParam) selectedDest = destParam;
		if (professionParam) selectedProfession = professionParam;
		if (pathways.length > 0) hasSearched = true;
	});

	function fetchProfessions(q: string): Promise<AutocompleteItem[]> {
		const query = q.toLowerCase();
		const filtered = query === ''
			? professions
			: professions.filter((p) => p.name.toLowerCase().includes(query));
		return Promise.resolve(
			filtered.map((p) => ({
				id: p.id,
				label: p.name,
				sublabel: p.category ?? undefined
			}))
		);
	}

	function fetchProfessionsInitial(): Promise<AutocompleteItem[]> {
		return fetchProfessions('');
	}

	function getProfessionLabel(id: string): string {
		return professions.find((p) => p.id === id)?.name ?? id;
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

<div class="mx-auto max-w-5xl px-4 py-8">
	{#if !data.enabled}
		<ToolDisabled tool={data.tool} />
	{:else}
		<div class="mb-8">
			<h1 class="text-2xl sm:text-3xl font-bold text-text">
				{t('credential_checker.title', 'Professional Credential Checker')}
			</h1>
			<p class="mt-2 text-text-subtle">
				{t(
					'credential_checker.subtitle',
					'Look up credential recognition pathways for your profession — regulatory body, timeline, cost, and steps.'
				)}
			</p>
		</div>

		{#if destinations.length === 0 || professions.length === 0}
			<EmptyState
				icon="badge-check"
				title={t('credential_checker.no_data', 'No data available')}
				description={t(
					'credential_checker.no_data_desc',
					'Credential data has not been configured yet. Please check back later.'
				)}
			/>
		{:else}
			<!-- Selectors -->
			<form onsubmit={onSearch} class="mb-6 space-y-4">
				<DestinationSelector
					{destinations}
					bind:selectedId={selectedDest}
					onchange={onDestChange}
				/>

				<div class="flex flex-wrap items-end gap-3">
					<div class="flex-1 min-w-[200px]">
						<label
							for="profession"
							class="block text-sm font-medium text-text mb-1"
						>
							{t('credential_checker.profession', 'Profession')}
						</label>
						<Autocomplete
							id="profession"
							placeholder={t('credential_checker.select_profession', 'Select a profession…')}
							emptyMessage={t('credential_checker.no_professions', 'No professions found')}
							fetcher={fetchProfessions}
							initialFetcher={fetchProfessionsInitial}
							selectedId={selectedProfession || null}
							selectedLabel={selectedProfession ? getProfessionLabel(selectedProfession) : null}
							onSelect={(item) => (selectedProfession = item.id)}
							onClear={() => (selectedProfession = '')}
						/>
					</div>

					<div class="flex-1 min-w-[200px]">
						<label
							for="issuing"
							class="block text-sm font-medium text-text mb-1"
						>
							{t('credential_checker.issuing_country', 'Issuing country (optional)')}
						</label>
						<Autocomplete
							id="issuing"
							placeholder={t('geo.select_destination', 'Select a country…')}
							emptyMessage={t('geo.no_destinations', 'No destinations found')}
							fetcher={fetchDestinations}
							initialFetcher={fetchDestinationsInitial}
							selectedId={selectedIssuing || null}
							selectedLabel={selectedIssuing ? getDestinationLabel(selectedIssuing) : null}
							onSelect={(item) => (selectedIssuing = item.id)}
							onClear={() => (selectedIssuing = '')}
						/>
					</div>

					<button
						type="submit"
						disabled={!selectedDest || !selectedProfession}
						class="px-5 py-2 bg-[var(--tool-accent)] text-white rounded-lg text-sm font-medium hover:bg-[var(--tool-accent-hover)] transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
					>
						{t('credential_checker.search', 'Search')}
					</button>
				</div>
			</form>

			<!-- Pathway list -->
			{#if loadingPathways}
				<div class="space-y-3">
					{#each Array(3) as _}
						<LoadingSkeleton height="120px" />
					{/each}
				</div>
			{:else if hasSearched && pathways.length === 0}
				<EmptyState
					icon="badge-check"
					title={t('credential_checker.no_pathways', 'No pathways found')}
					description={t(
						'credential_checker.no_pathways_desc',
						'No credential recognition pathways are available for this selection yet.'
					)}
				/>
			{:else if pathways.length > 0}
				<div class="space-y-3">
					{#each pathways as pathway (pathway.id)}
						<div class="bg-surface border border-border rounded-lg overflow-hidden">
							<!-- Card header (click to expand) -->
							<button
								type="button"
								onclick={() => toggleDetail(pathway)}
								class="w-full text-left p-4 hover:bg-surface-subtle transition-colors"
								aria-expanded={expandedId === pathway.id}
							>
								<div class="flex items-start justify-between gap-3">
									<div class="min-w-0">
										<h3 class="text-base font-semibold text-text">
											{pathway.profession_name}
										</h3>
										<p class="text-sm text-muted mt-0.5">
											{pathway.regulatory_body}
										</p>
										<div class="flex flex-wrap items-center gap-x-3 gap-y-1 mt-2 text-xs text-muted">
											<span class="flex items-center gap-1">
												<Icon name="route" size={12} />
												{pathway.pathway_type}
											</span>
											{#if pathway.timeline_estimate}
												<span class="flex items-center gap-1">
													<Icon name="clock" size={12} />
													{pathway.timeline_estimate}
												</span>
											{/if}
											{#if pathway.cost_estimate_usd}
												<span class="flex items-center gap-1">
													<Icon name="dollar-sign" size={12} />
													{pathway.cost_estimate_usd}
												</span>
											{/if}
											{#if pathway.issuing_space_name}
												<span class="flex items-center gap-1">
													<Icon name="map-pin" size={12} />
													{pathway.issuing_space_name}
												</span>
											{/if}
										</div>
									</div>
									<Icon
										name={expandedId === pathway.id ? 'chevron-up' : 'chevron-down'}
										size={20}
										class="text-muted flex-shrink-0 mt-1"
									/>
								</div>
							</button>

							<!-- Expanded detail -->
							{#if expandedId === pathway.id}
								<div class="border-t border-border p-4 space-y-4">
									{#if loadingDetail}
										<LoadingSkeleton height="80px" />
									{:else if detailPathway}
										<!-- Description -->
										{#if detailPathway.pathway_description}
											<div>
												<h4 class="text-xs font-semibold text-muted uppercase tracking-wide mb-1">
													{t('credential_checker.description', 'Pathway description')}
												</h4>
												<p class="text-sm text-text leading-relaxed">
													{detailPathway.pathway_description}
												</p>
											</div>
										{/if}

										<!-- Links -->
										<div class="flex flex-wrap gap-3">
											{#if detailPathway.official_application_url}
												<a
													href={detailPathway.official_application_url}
													target="_blank"
													rel="noopener noreferrer"
													class="inline-flex items-center gap-1.5 text-sm text-primary hover:underline"
												>
													<Icon name="external-link" size={14} />
													{t('credential_checker.official_application', 'Official application')}
												</a>
											{/if}
											{#if detailPathway.regulatory_body_url}
												<a
													href={detailPathway.regulatory_body_url}
													target="_blank"
													rel="noopener noreferrer"
													class="inline-flex items-center gap-1.5 text-sm text-primary hover:underline"
												>
													<Icon name="external-link" size={14} />
													{t('credential_checker.regulatory_body', 'Regulatory body')}
												</a>
											{/if}
										</div>

										{#if detailPathway.source_url_last_checked}
											<p class="text-xs text-muted">
												{t('credential_checker.last_checked', 'Last checked:')} {fmtDate(detailPathway.source_url_last_checked)}
											</p>
										{/if}

										<!-- Community notes -->
										<div class="pt-2">
											<h4 class="text-xs font-semibold text-muted uppercase tracking-wide mb-3">
												{t('credential_checker.community_notes', 'Community notes')}
											</h4>

											{#if detailNotes.length > 0}
												<div class="space-y-2 mb-4">
													{#each detailNotes as note (note.id)}
														<div class="p-3 bg-surface-subtle rounded-lg">
															<p class="text-sm text-text">{note.note}</p>
															<div class="flex items-center gap-2 mt-2 text-xs text-muted">
																{#if note.user_name}
																	<span>{note.user_name}</span>
																{/if}
																<span>· {fmtDate(note.created_at)}</span>
																<span class="flex items-center gap-0.5">
																	<Icon name="thumbs-up" size={12} />
																	{note.upvotes}
																</span>
															</div>
														</div>
													{/each}
												</div>
											{:else}
												<p class="text-sm text-muted mb-4">
													{t('credential_checker.no_notes', 'No community notes yet. Be the first to share your experience.')}
												</p>
											{/if}

											<!-- Note submission -->
											{#if user}
												<div class="space-y-2">
													<textarea
														bind:value={noteText}
														rows="3"
														placeholder={t(
															'credential_checker.note_placeholder',
															'Share your experience with this credential pathway…'
														)}
														class="w-full px-3 py-2 bg-surface border border-border rounded-lg text-sm text-text focus:border-primary focus:outline-none resize-y"
													></textarea>
													<button
														type="button"
														onclick={submitNote}
														disabled={!noteText.trim() || submittingNote}
														class="px-4 py-1.5 bg-[var(--tool-accent)] text-white rounded-lg text-sm font-medium hover:bg-[var(--tool-accent-hover)] transition-colors disabled:opacity-50"
													>
														{submittingNote
															? t('common.submitting', 'Submitting…')
															: t('credential_checker.submit_note', 'Submit note')}
													</button>
												</div>
											{:else}
												<div class="p-3 bg-surface border border-border rounded-lg text-center">
													<p class="text-sm text-text-subtle mb-2">
														{t('credential_checker.notes_signin', 'Sign in to share your experience and help others.')}
													</p>
													<button
														type="button"
														onclick={() => (showRegisterPrompt = true)}
														class="inline-flex items-center gap-1.5 px-4 py-2 bg-primary text-primary-contrast rounded-lg text-sm font-medium hover:bg-primary-hover transition-colors"
													>
														{t('common.sign_in', 'Sign in')}
													</button>
												</div>
											{/if}
										</div>
									{/if}
								</div>
							{/if}
						</div>
					{/each}
				</div>
			{:else if !hasSearched}
				<EmptyState
					icon="badge-check"
					title={t('credential_checker.select_prompt', 'Select a destination and profession')}
					description={t(
						'credential_checker.select_prompt_desc',
						'Choose your destination country and profession to view credential recognition pathways.'
					)}
				/>
			{/if}
		{/if}
	{/if}

	{#if showRegisterPrompt}
		<RegistrationPrompt
			open={showRegisterPrompt}
			onClose={() => (showRegisterPrompt = false)}
			icon="badge-check"
			title={t('register.credential_notes', 'Share credential notes')}
			description={t(
				'register.credential_notes_desc',
				'Create a free account to share your credential pathway experience and help others on their japa journey.'
			)}
		/>
	{/if}
</div>
