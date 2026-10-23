<script lang="ts">
	import { untrack } from 'svelte';
	import { replaceState } from '$app/navigation';
	import { api } from '$lib/api/client';
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
	import type {
		ArrivalChecklistSelectorEntry,
		ArrivalChecklistItem,
		ArrivalChecklistUserItem,
		GeoSpaceOption,
		UserArrivalChecklist
	} from '$lib/types';

	/**
	 * Arrival Checklist tool — §92, ADR-088.
	 * Destination selector (built from the selector data) → optional visa
	 * category sub-selector → checklist items grouped by phase, sorted by
	 * sort_order. Viewing is anonymous; ticking an item prompts registration
	 * and persists progress via /me/arrival-checklists/* (ROADMAP-TOOLS 05.03).
	 */
	let { data } = $props();

	let selectorEntries = $state<ArrivalChecklistSelectorEntry[]>(
		untrack(() => data.selectorEntries)
	);
	let destinations = $state<GeoSpaceOption[]>(untrack(() => data.destinations));
	let items = $state<ArrivalChecklistItem[]>(untrack(() => data.items));
	let selectedDestId = $state<string>(untrack(() => data.initialDestId ?? ''));
	let selectedVisaCategory = $state<string>('');
	let loadingItems = $state(false);
	let user = $state(untrack(() => data.user));
	let myChecklists = $state<UserArrivalChecklist[]>(untrack(() => data.myChecklists));

	// Per-item progress state, keyed by item id (§92). Only populated for
	// authenticated users; anonymous ticks open the registration prompt.
	let progress = $state<Record<string, { completed: boolean; notes?: string | null }>>({});
	let activeGuideId = $state<string>('');
	// Guides already "started" via POST /me/arrival-checklists this session —
	// the POST registers the checklist in GET /me/arrival-checklists.
	let startedGuides = new Set<string>();
	let savingItemId = $state<string | null>(null);
	let showRegisterPrompt = $state(false);
	let editingNoteId = $state<string | null>(null);
	let noteDraft = $state('');

	const seo = $derived(
		buildSeoMeta({
			title: t('arrival_checklist.seo_title', 'Arrival Checklist — {site} Tools'),
			description: t(
				'arrival_checklist.seo_description',
				'A phased checklist of tasks to complete after arriving in your destination country — registration, banking, and more.'
			),
			path: '/arrival-checklist',
			origin: page.url.origin,
			ogImage: `${page.url.origin}/og/og-arrival-checklist.png`,
			jsonLd: {
				'@context': 'https://schema.org',
				'@type': 'HowTo',
				name: 'Arrival Checklist',
				description: 'Phased checklist of tasks to complete after arriving in your destination country.',
				step: items.slice(0, 10).map((it, i) => ({
					'@type': 'HowToStep',
					position: i + 1,
					name: it.title,
					text: it.description ?? it.title
				}))
			}
		})
	);

	// Visa categories available for the selected destination (from selector).
	// Only shown if there's more than one category for the destination.
	const visaCategories = $derived(
		selectedDestId
			? selectorEntries
					.filter((e) => e.destination_space_id === selectedDestId && e.visa_category)
					.map((e) => e.visa_category as string)
			: []
	);

	const showVisaCategorySelector = $derived(visaCategories.length > 1);

	// Group items by phase, sorted by sort_order within each phase.
	const phasedItems = $derived.by(() => {
		const phases = new Map<string, ArrivalChecklistItem[]>();
		for (const item of items) {
			const arr = phases.get(item.phase) ?? [];
			arr.push(item);
			phases.set(item.phase, arr);
		}
		// Sort items within each phase by sort_order
		const result: { phase: string; items: ArrivalChecklistItem[] }[] = [];
		for (const [phase, phaseItems] of phases) {
			result.push({
				phase,
				items: phaseItems.sort((a, b) => a.sort_order - b.sort_order)
			});
		}
		return result;
	});

	const completedCount = $derived(items.filter((i) => progress[i.id]?.completed).length);
	const progressPercent = $derived(items.length ? Math.round((completedCount / items.length) * 100) : 0);

	async function loadItems() {
		if (!selectedDestId) {
			items = [];
			return;
		}
		// Find the selector entry for this dest (+ optional visa category)
		// and use its guide_id to fetch items via the correct endpoint.
		const entry = selectorEntries.find(
			(e) =>
				e.destination_space_id === selectedDestId &&
				(!selectedVisaCategory || e.visa_category === selectedVisaCategory)
		);
		if (!entry?.guide_id) {
			items = [];
			progress = {};
			activeGuideId = '';
			return;
		}
		activeGuideId = entry.guide_id;
		loadingItems = true;
		// Authenticated users get items with their saved progress merged in
		// (GET /me/arrival-checklists/{guide_id}); guests get the public list.
		if (user) {
			const res = await api.get<ArrivalChecklistUserItem[]>(`/me/arrival-checklists/${entry.guide_id}`);
			loadingItems = false;
			if (!res.error && res.data) {
				items = res.data;
				const next: Record<string, { completed: boolean; notes?: string | null }> = {};
				for (const it of res.data) {
					next[it.id] = { completed: it.completed, notes: it.notes ?? null };
				}
				progress = next;
				// Any persisted progress row means the checklist is already
				// registered in the user's list — skip the POST start call.
				if (res.data.some((it) => it.completed || it.notes)) {
					startedGuides.add(entry.guide_id);
				}
				return;
			}
			// 404 (guide not started / not found) — fall through to public list.
		}
		const res = await api.get<ArrivalChecklistItem[]>(`/arrival-checklist/${entry.guide_id}`);
		loadingItems = false;
		if (!res.error) items = res.data ?? [];
		else items = [];
		progress = {};
	}

	/** Register the checklist in the user's saved list (idempotent). */
	async function ensureStarted() {
		if (!activeGuideId || startedGuides.has(activeGuideId)) return;
		startedGuides.add(activeGuideId);
		await api.post(`/me/arrival-checklists`, { guide_id: activeGuideId });
	}

	async function toggleItem(item: ArrivalChecklistItem) {
		if (!user) {
			showRegisterPrompt = true;
			return;
		}
		if (!activeGuideId || savingItemId === item.id) return;
		savingItemId = item.id;
		const next = !progress[item.id]?.completed;
		// Optimistic update.
		progress = { ...progress, [item.id]: { ...progress[item.id], completed: next } };
		await ensureStarted();
		const res = await api.patch(`/me/arrival-checklists/${activeGuideId}/items/${item.id}`, {
			completed: next
		});
		savingItemId = null;
		if (res.error) {
			// Roll back the optimistic toggle.
			progress = { ...progress, [item.id]: { ...progress[item.id], completed: !next } };
		} else {
			// Keep the "Your checklists" strip in sync (§92).
			myChecklists = myChecklists.map((cl) =>
				cl.guide_id === activeGuideId
					? { ...cl, completed_items: cl.completed_items + (next ? 1 : -1) }
					: cl
			);
		}
	}

	function startEditNote(item: ArrivalChecklistItem) {
		if (!user) {
			showRegisterPrompt = true;
			return;
		}
		editingNoteId = item.id;
		noteDraft = progress[item.id]?.notes ?? '';
	}

	async function saveNote(item: ArrivalChecklistItem) {
		if (!activeGuideId || savingItemId === item.id) return;
		savingItemId = item.id;
		editingNoteId = null;
		await ensureStarted();
		const res = await api.patch(`/me/arrival-checklists/${activeGuideId}/items/${item.id}`, {
			notes: noteDraft
		});
		savingItemId = null;
		if (!res.error) {
			progress = { ...progress, [item.id]: { ...progress[item.id], notes: noteDraft } };
		}
	}

	/**
	 * Resume a saved checklist — resolve the guide_id back to the selector
	 * entry (destination + visa category) and load it.
	 */
	function openChecklist(cl: UserArrivalChecklist) {
		const entry = selectorEntries.find((e) => e.guide_id === cl.guide_id);
		if (!entry) return;
		selectedDestId = entry.destination_space_id;
		selectedVisaCategory = entry.visa_category ?? '';
		const url = new URL(page.url);
		url.searchParams.set('dest', selectedDestId);
		replaceState(url, {});
		loadItems();
	}

	function onDestChange(id: string) {
		selectedDestId = id;
		selectedVisaCategory = '';
		const url = new URL(page.url);
		if (id) url.searchParams.set('dest', id);
		else url.searchParams.delete('dest');
		replaceState(url, {});
		loadItems();
	}

	function onVisaCategoryChange(value: string) {
		selectedVisaCategory = value;
		loadItems();
	}

	function fetchVisaCategories(q: string): Promise<AutocompleteItem[]> {
		const query = q.toLowerCase();
		const filtered = query === ''
			? visaCategories
			: visaCategories.filter((c) => c.toLowerCase().includes(query));
		return Promise.resolve(filtered.map((c) => ({ id: c, label: c })));
	}

	function fetchVisaCategoriesInitial(): Promise<AutocompleteItem[]> {
		return fetchVisaCategories('');
	}
</script>

<SeoHead seo={seo} ogType="website" twitterCard="summary" />

<div class="mx-auto max-w-4xl px-4 py-8">
	{#if !data.enabled}
		<ToolDisabled tool={data.tool} />
	{:else}
		<div class="mb-8">
			<h1 class="text-2xl sm:text-3xl font-bold text-text">
				{t('arrival_checklist.title', 'Arrival Checklist')}
			</h1>
			<p class="mt-2 text-text-subtle">
				{t('arrival_checklist.subtitle', 'Everything you need to do after you land — phased by time period.')}
			</p>
		</div>

		{#if selectorEntries.length === 0}
			<EmptyState
				icon="plane-landing"
				title={t('arrival_checklist.no_destinations', 'No checklists available')}
				description={t('arrival_checklist.no_destinations_desc', 'Arrival checklist guides haven’t been published yet. Please check back later.')}
			/>
		{:else}
			<!-- Your saved checklists (§92 — GET /me/arrival-checklists) -->
			{#if user && myChecklists.length > 0}
				<div class="mb-6">
					<h2 class="text-xs font-semibold uppercase tracking-wider text-muted mb-2">
						{t('arrival_checklist.your_checklists', 'Your checklists')}
					</h2>
					<div class="flex flex-wrap gap-2">
						{#each myChecklists as cl (cl.guide_id)}
							<button
								onclick={() => openChecklist(cl)}
								class="px-3 py-2 bg-surface border border-border rounded-lg text-sm text-start hover:border-[var(--tool-accent)] transition-colors {activeGuideId === cl.guide_id ? 'border-[var(--tool-accent)]' : ''}"
							>
								<span class="block font-medium text-text">
									{cl.destination_space_name ?? cl.destination_space_code ?? ''}{#if cl.visa_category}
										· {cl.visa_category}{/if}
								</span>
								<span class="block text-xs text-muted">
									{t('arrival_checklist.progress', '{done} of {total} done')
										.replace('{done}', String(cl.completed_items))
										.replace('{total}', String(cl.total_items))}
								</span>
							</button>
						{/each}
					</div>
				</div>
			{/if}

			<!-- Destination selector (full country list from GET /geo/countries) -->
			<div class="mb-4">
				<DestinationSelector {destinations} bind:selectedId={selectedDestId} onchange={onDestChange} />
			</div>

			{#if showVisaCategorySelector}
				<!-- Visa category sub-selector (only when multiple categories exist) -->
				<div class="mb-6 flex items-center gap-3">
					<label for="visa-cat" class="text-sm font-medium text-text whitespace-nowrap">
						{t('arrival_checklist.visa_category', 'Visa category:')}
					</label>
					<Autocomplete
						id="visa-cat"
						placeholder={t('arrival_checklist.all_categories', 'All categories')}
						emptyMessage={t('arrival_checklist.no_categories', 'No categories found')}
						fetcher={fetchVisaCategories}
						initialFetcher={fetchVisaCategoriesInitial}
						selectedId={selectedVisaCategory || null}
						selectedLabel={selectedVisaCategory || null}
						onSelect={(item) => onVisaCategoryChange(item.id)}
						onClear={() => onVisaCategoryChange('')}
					/>
				</div>
			{/if}

			{#if !selectedDestId}
				<EmptyState
					icon="map-pin"
					title={t('arrival_checklist.select_destination', 'Select a destination country')}
					description={t('arrival_checklist.select_destination_desc', 'Choose a country above to see your arrival checklist.')}
				/>
			{:else if loadingItems}
				<div class="space-y-4">
					<LoadingSkeleton height="40px" />
					<LoadingSkeleton height="100px" />
					<LoadingSkeleton height="100px" />
				</div>
			{:else if items.length === 0}
				<EmptyState
					icon="clipboard-list"
					title={t('arrival_checklist.no_items', 'No checklist items')}
					description={t('arrival_checklist.no_items_desc', 'No arrival checklist items have been published for this destination yet.')}
				/>
			{:else}
				<!-- Progress summary (§92 — persisted for authenticated users) -->
				{#if user}
					<div class="mb-6 p-4 bg-surface border border-border rounded-lg">
						<div class="flex items-center justify-between mb-2">
							<span class="text-sm font-medium text-text">
								{t('arrival_checklist.progress', '{done} of {total} done')
									.replace('{done}', String(completedCount))
									.replace('{total}', String(items.length))}
							</span>
							<span class="text-sm text-muted">{progressPercent}%</span>
						</div>
						<div class="h-2 rounded-full bg-surface-subtle overflow-hidden">
							<div
								class="h-full rounded-full bg-[var(--tool-accent)] transition-all duration-300"
								style="width: {progressPercent}%"
							></div>
						</div>
					</div>
				{/if}

				<!-- Phased checklist -->
				<div class="space-y-8">
					{#each phasedItems as group (group.phase)}
						<div>
							<div class="flex items-center gap-2 mb-3">
								<div
									class="flex items-center justify-center w-8 h-8 rounded-lg bg-[var(--tool-accent)]/10 text-[var(--tool-accent)]"
								>
									<Icon name="calendar-days" size={18} />
								</div>
								<h2 class="text-lg font-semibold text-text">{group.phase}</h2>
								<span class="text-xs text-muted">({group.items.length})</span>
							</div>
							<div class="space-y-2">
								{#each group.items as item (item.id)}
									{@const done = progress[item.id]?.completed ?? false}
									<div
										class="p-4 bg-surface border border-border rounded-lg {item.is_flagged ? 'border-yellow-300 bg-yellow-50/30' : ''}"
									>
										<div class="flex items-start gap-3">
											<button
												onclick={() => toggleItem(item)}
												disabled={savingItemId === item.id}
												class="flex-shrink-0 mt-0.5 w-5 h-5 rounded border flex items-center justify-center transition-colors {done ? 'bg-[var(--tool-accent)] border-[var(--tool-accent)] text-white' : 'border-border hover:border-[var(--tool-accent)]'}"
												aria-label={done
													? t('arrival_checklist.mark_incomplete', 'Mark as not done')
													: t('arrival_checklist.mark_complete', 'Mark as done')}
												aria-pressed={done}
											>
												{#if savingItemId === item.id}
													<Icon name="loader-2" size={12} class="animate-spin" />
												{:else if done}
													<Icon name="check" size={12} />
												{/if}
											</button>
											{#if item.is_flagged}
												<Icon name="flag" size={16} class="text-yellow-500 flex-shrink-0 mt-0.5" />
											{/if}
											<div class="min-w-0 flex-1">
												<h3 class="text-sm font-medium {done ? 'text-muted line-through' : 'text-text'}">{item.title}</h3>
												{#if item.description}
													<p class="mt-1 text-sm text-text-subtle">{item.description}</p>
												{/if}
												<div class="mt-2 flex items-center gap-3">
													{#if item.official_link}
														<a
															href={item.official_link}
															target="_blank"
															rel="noopener noreferrer"
															class="inline-flex items-center gap-1 text-xs text-[var(--tool-accent)] hover:underline"
														>
															<Icon name="external-link" size={12} />
															{t('arrival_checklist.official_link', 'Official guidance')}
														</a>
													{/if}
													<button
														onclick={() => startEditNote(item)}
														class="inline-flex items-center gap-1 text-xs text-muted hover:text-[var(--tool-accent)]"
													>
														<Icon name="pencil" size={12} />
														{progress[item.id]?.notes
															? t('arrival_checklist.edit_note', 'Edit note')
															: t('arrival_checklist.add_note', 'Add note')}
													</button>
												</div>
												{#if progress[item.id]?.notes && editingNoteId !== item.id}
													<p class="mt-1.5 text-xs text-muted italic">{progress[item.id]?.notes}</p>
												{/if}
												{#if editingNoteId === item.id}
													<div class="mt-2 flex gap-2">
														<textarea
															bind:value={noteDraft}
															rows="2"
															class="flex-1 px-2 py-1.5 text-sm bg-surface border border-border rounded-lg text-text resize-none focus:border-[var(--tool-accent)] focus:outline-none"
															placeholder={t('arrival_checklist.note_placeholder', 'Add a note…')}
														></textarea>
														<div class="flex flex-col gap-1">
															<button
																onclick={() => saveNote(item)}
																class="px-2.5 py-1 text-xs font-medium bg-[var(--tool-accent)] text-white rounded-md hover:bg-[var(--tool-accent-hover)]"
															>
																{t('common.save', 'Save')}
															</button>
															<button
																onclick={() => (editingNoteId = null)}
																class="px-2.5 py-1 text-xs text-muted hover:text-text"
															>
																{t('common.cancel', 'Cancel')}
															</button>
														</div>
													</div>
												{/if}
											</div>
										</div>
									</div>
								{/each}
							</div>
						</div>
					{/each}
				</div>
			{/if}
		{/if}
	{/if}
</div>

<RegistrationPrompt
	open={showRegisterPrompt}
	onClose={() => (showRegisterPrompt = false)}
	icon="user-plus"
	title={t('arrival_checklist.register_title', 'Save your arrival checklist')}
	description={t('arrival_checklist.register_desc', 'Create a free account to track your progress and pick up where you left off on any device.')}
/>
