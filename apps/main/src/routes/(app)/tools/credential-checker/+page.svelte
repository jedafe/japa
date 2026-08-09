<script lang="ts">
	import { untrack, onMount } from 'svelte';
	import { t } from '$lib/i18n';
	import { page } from '$app/state';
	import {
		BadgeCheck,
		Route,
		Clock,
		DollarSign,
		MapPin,
		ChevronDown,
		ChevronUp,
		ExternalLink,
		ThumbsUp,
		Lock
	} from 'lucide-svelte';
	import { EmptyState, LoadingSkeleton, SelectField } from '$lib/components/ui';
	import { api } from '$lib/api/client';
	import type {
		GeoSpaceOption,
		CredentialProfession,
		CredentialPathwayWithAi,
		CredentialCommunityNote,
		CredentialPathwayDetail
	} from '$lib/types';
	import type { PageData } from './$types';

	/**
	 * Professional Credential Checker tool — §94.
	 * Destination → profession → optional issuing country → pathway cards.
	 * Click a card to expand detail (fetches /credentials/{id} which returns
	 * { pathway, notes }). Authed users can submit community notes.
	 */
	let { data }: { data: PageData } = $props();

	const title = $derived(t('credential_checker.title', 'Professional Credential Checker'));
	const description = $derived(
		t(
			'credential_checker.subtitle',
			'Look up credential recognition pathways for your profession — regulatory body, timeline, cost, and steps.'
		)
	);

	const destinations = $derived<GeoSpaceOption[]>(data.destinations);
	const professions = $derived<CredentialProfession[]>(data.professions);
	let pathways = $state<CredentialPathwayWithAi[]>(untrack(() => data.pathways));
	const authed = $derived(data.authed);

	let selectedDest = $state(untrack(() => data.initialDestId ?? ''));
	let selectedProfession = $state(untrack(() => data.initialProfessionId ?? ''));
	let selectedIssuing = $state('');
	let loadingPathways = $state(false);
	let hasSearched = $state(untrack(() => data.pathways.length > 0));

	// Expanded detail state
	let expandedId = $state<string | null>(null);
	let detailPathway = $state<CredentialPathwayWithAi | null>(null);
	let detailNotes = $state<CredentialCommunityNote[]>([]);
	let loadingDetail = $state(false);

	// Note submission state
	let noteText = $state('');
	let submittingNote = $state(false);

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

	// Flatten grouped professions into SelectField options (optgroup not
	// supported by Bits UI Select; category is prefixed instead).
	const professionOptions = $derived(
		Object.entries(professionGroups).flatMap(([category, items]) =>
			items.map((p) => ({ value: p.id, label: `${category} — ${p.name}` }))
		)
	);

	const destinationOptions = $derived(
		destinations.map((d) => ({ value: d.id, label: d.name }))
	);

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
		if (!authed) return;
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
		// the selectors are already synced from data.initial*; nothing to do.
	});
</script>

<svelte:head>
	<title>{title}</title>
	<meta name="description" content={description} />
	<meta property="og:title" content={title} />
	<meta property="og:description" content={description} />
	<meta property="og:type" content="website" />
	<meta property="og:url" content={page.url.href} />
	<meta name="twitter:card" content="summary" />
	<meta name="twitter:title" content={title} />
	<meta name="twitter:description" content={description} />
</svelte:head>

<div class="mx-auto w-full max-w-3xl">
	<div class="border-b border-[var(--color-border)] px-4 py-3">
		<h1 class="flex items-center gap-2 text-lg font-semibold text-[var(--color-text)]">
			<BadgeCheck size={20} /> {title}
		</h1>
		<p class="mt-1 text-sm text-[var(--color-muted)]">{description}</p>
	</div>

	{#if data.disabled}
		<div class="p-4">
			<EmptyState
				icon={Lock}
				title={t('tools.disabled_title', 'This tool is currently unavailable')}
				body={t('tools.disabled_body', 'Check back soon or contact support.')}
			/>
		</div>
	{:else if destinations.length === 0 || professions.length === 0}
		<div class="p-4">
			<EmptyState
				icon={BadgeCheck}
				title={t('credential_checker.no_data', 'No data available')}
				body={t(
					'credential_checker.no_data_desc',
					'Credential data has not been configured yet. Please check back later.'
				)}
			/>
		</div>
	{:else}
		<!-- Selectors -->
		<form onsubmit={onSearch} class="space-y-4 px-4 py-3">
			<!-- Destination selector (geo-aware, first axis) -->
			<div>
				<label
					for="cc-dest"
					class="block text-xs font-medium uppercase tracking-wide text-[var(--color-muted)]"
				>
					{t('geo.destination', 'Destination')}
				</label>
				<div class="mt-2">
					<SelectField
						id="cc-dest"
						value={selectedDest}
						options={destinationOptions}
						placeholder={t('geo.select_destination', 'Select a country…')}
						onValueChange={(v) => {
							selectedDest = v;
							onDestChange();
						}}
					/>
				</div>
			</div>

			<!-- Profession + issuing country -->
			<div class="flex flex-wrap items-end gap-3">
				<div class="flex-1 min-w-[200px]">
					<label
						for="cc-profession"
						class="block text-xs font-medium uppercase tracking-wide text-[var(--color-muted)]"
					>
						{t('credential_checker.profession', 'Profession')}
					</label>
					<div class="mt-2">
						<SelectField
							id="cc-profession"
							value={selectedProfession}
							options={professionOptions}
							placeholder={t('credential_checker.select_profession', 'Select a profession…')}
							onValueChange={(v) => (selectedProfession = v)}
						/>
					</div>
				</div>

				<div class="flex-1 min-w-[200px]">
					<label
						for="cc-issuing"
						class="block text-xs font-medium uppercase tracking-wide text-[var(--color-muted)]"
					>
						{t('credential_checker.issuing_country', 'Issuing country (optional)')}
					</label>
					<div class="mt-2">
						<SelectField
							id="cc-issuing"
							value={selectedIssuing}
							options={destinationOptions}
							placeholder={t('geo.select_destination', 'Select a country…')}
							onValueChange={(v) => (selectedIssuing = v)}
						/>
					</div>
				</div>

				<button
					type="submit"
					disabled={!selectedDest || !selectedProfession}
					class="rounded-[var(--radius)] bg-[var(--color-primary)] px-5 py-2 text-sm font-medium text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)] disabled:cursor-not-allowed disabled:opacity-50"
				>
					{t('credential_checker.search', 'Search')}
				</button>
			</div>
		</form>

		<!-- Pathway list -->
		<div class="p-4">
			{#if loadingPathways}
				<div class="space-y-3">
					{#each Array(3) as _}
						<LoadingSkeleton variant="card" count={1} />
					{/each}
				</div>
			{:else if hasSearched && pathways.length === 0}
				<EmptyState
					icon={BadgeCheck}
					title={t('credential_checker.no_pathways', 'No pathways found')}
					body={t(
						'credential_checker.no_pathways_desc',
						'No credential recognition pathways are available for this selection yet.'
					)}
				/>
			{:else if pathways.length > 0}
				<div class="space-y-3">
					{#each pathways as pathway (pathway.id)}
						<div class="overflow-hidden rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)]">
							<!-- Card header (click to expand) -->
							<button
								type="button"
								onclick={() => toggleDetail(pathway)}
								class="w-full p-4 text-left transition-colors hover:bg-[var(--color-surface-subtle)]"
								aria-expanded={expandedId === pathway.id}
							>
								<div class="flex items-start justify-between gap-3">
									<div class="min-w-0">
										<h3 class="text-base font-semibold text-[var(--color-text)]">
											{pathway.profession_name}
										</h3>
										<p class="mt-0.5 text-sm text-[var(--color-muted)]">
											{pathway.regulatory_body}
										</p>
										<div class="mt-2 flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-[var(--color-muted)]">
											<span class="flex items-center gap-1">
												<Route size={12} />
												{pathway.pathway_type}
											</span>
											{#if pathway.timeline_estimate}
												<span class="flex items-center gap-1">
													<Clock size={12} />
													{pathway.timeline_estimate}
												</span>
											{/if}
											{#if pathway.cost_estimate_usd}
												<span class="flex items-center gap-1">
													<DollarSign size={12} />
													{pathway.cost_estimate_usd}
												</span>
											{/if}
											{#if pathway.issuing_space_name}
												<span class="flex items-center gap-1">
													<MapPin size={12} />
													{pathway.issuing_space_name}
												</span>
											{/if}
										</div>
									</div>
									{#if expandedId === pathway.id}
										<ChevronUp size={20} class="mt-1 flex-shrink-0 text-[var(--color-muted)]" />
									{:else}
										<ChevronDown size={20} class="mt-1 flex-shrink-0 text-[var(--color-muted)]" />
									{/if}
								</div>
							</button>

							<!-- Expanded detail -->
							{#if expandedId === pathway.id}
								<div class="space-y-4 border-t border-[var(--color-border)] p-4">
									{#if loadingDetail}
										<LoadingSkeleton variant="lines" />
									{:else if detailPathway}
										<!-- Description -->
										{#if detailPathway.pathway_description}
											<div>
												<h4 class="mb-1 text-xs font-semibold uppercase tracking-wide text-[var(--color-muted)]">
													{t('credential_checker.description', 'Pathway description')}
												</h4>
												<p class="text-sm leading-relaxed text-[var(--color-text)]">
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
													class="inline-flex items-center gap-1.5 text-sm text-[var(--color-primary)] hover:underline"
												>
													<ExternalLink size={14} />
													{t('credential_checker.official_application', 'Official application')}
												</a>
											{/if}
											{#if detailPathway.regulatory_body_url}
												<a
													href={detailPathway.regulatory_body_url}
													target="_blank"
													rel="noopener noreferrer"
													class="inline-flex items-center gap-1.5 text-sm text-[var(--color-primary)] hover:underline"
												>
													<ExternalLink size={14} />
													{t('credential_checker.regulatory_body', 'Regulatory body')}
												</a>
											{/if}
										</div>

										{#if detailPathway.source_url_last_checked}
											<p class="text-xs text-[var(--color-muted)]">
												{t('credential_checker.last_checked', 'Last checked:')} {fmtDate(detailPathway.source_url_last_checked)}
											</p>
										{/if}

										<!-- Community notes -->
										<div class="pt-2">
											<h4 class="mb-3 text-xs font-semibold uppercase tracking-wide text-[var(--color-muted)]">
												{t('credential_checker.community_notes', 'Community notes')}
											</h4>

											{#if detailNotes.length > 0}
												<div class="mb-4 space-y-2">
													{#each detailNotes as note (note.id)}
														<div class="rounded-[var(--radius)] bg-[var(--color-surface-subtle)] p-3">
															<p class="text-sm text-[var(--color-text)]">{note.note}</p>
															<div class="mt-2 flex items-center gap-2 text-xs text-[var(--color-muted)]">
																{#if note.user_name}
																	<span>{note.user_name}</span>
																{/if}
																<span>· {fmtDate(note.created_at)}</span>
																<span class="flex items-center gap-0.5">
																	<ThumbsUp size={12} />
																	{note.upvotes}
																</span>
															</div>
														</div>
													{/each}
												</div>
											{:else}
												<p class="mb-4 text-sm text-[var(--color-muted)]">
													{t('credential_checker.no_notes', 'No community notes yet. Be the first to share your experience.')}
												</p>
											{/if}

											<!-- Note submission -->
											{#if authed}
												<div class="space-y-2">
													<textarea
														bind:value={noteText}
														rows="3"
														placeholder={t(
															'credential_checker.note_placeholder',
															'Share your experience with this credential pathway…'
														)}
														class="w-full resize-y rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] focus:border-[var(--color-primary)] focus:outline-none"
													></textarea>
													<button
														type="button"
														onclick={submitNote}
														disabled={!noteText.trim() || submittingNote}
														class="rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-1.5 text-sm font-medium text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)] disabled:opacity-50"
													>
														{submittingNote
															? t('common.submitting', 'Submitting…')
															: t('credential_checker.submit_note', 'Submit note')}
													</button>
												</div>
											{:else}
												<div class="mb-3 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface-subtle)] px-3 py-2 text-xs text-[var(--color-muted)]">
													{t('credential_checker.notes_signin', 'Sign in to save your progress.')}
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
					icon={BadgeCheck}
					title={t('credential_checker.select_prompt', 'Select a destination and profession')}
					body={t(
						'credential_checker.select_prompt_desc',
						'Choose your destination country and profession to view credential recognition pathways.'
					)}
				/>
			{/if}
		</div>
	{/if}
</div>
