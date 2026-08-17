<script lang="ts">
	import { untrack } from 'svelte';
	import { replaceState } from '$app/navigation';
	import { api } from '$lib/api/client';
	import { buildSeoMeta } from '$lib/seo';
	import { t } from '$lib/i18n';
	import { page } from '$app/state';
	import Icon from '$lib/components/ui/Icon.svelte';
	import EmptyState from '$lib/components/ui/EmptyState.svelte';
	import LoadingSkeleton from '$lib/components/ui/LoadingSkeleton.svelte';
	import RegistrationPrompt from '$lib/components/ui/RegistrationPrompt.svelte';
	import ToolDisabled from '$lib/components/ui/ToolDisabled.svelte';
	import Autocomplete from '$lib/components/ui/Autocomplete.svelte';
	import type { AutocompleteItem } from '$lib/components/ui/Autocomplete.svelte';
	import type {
		ChecklistSelector,
		ChecklistGuide,
		ChecklistProgress,
		ChecklistCountryOption,
		ChecklistItem
	} from '$lib/types';

	/**
	 * Visa Document Checklist tool — §70, ADR-070.
	 * Three-step selector: origin → destination → visa type.
	 * Then renders checkable document items with per-item notes.
	 * Anonymous users see the checklist but cannot save progress (registration
	 * prompt on first tick = conversion opportunity).
	 */
	let { data } = $props();

	let selector = $state<ChecklistSelector | null>(untrack(() => data.selector));
	let guide = $state<ChecklistGuide | null>(untrack(() => data.guide));
	let progress = $state<ChecklistProgress | null>(untrack(() => data.progress));
	let user = $state(untrack(() => data.user));

	// Selector state
	let selectedOrigin = $state<string>('');
	let selectedDestination = $state<string>('');
	let selectedVisaType = $state<string>('');

	let loadingGuide = $state(false);
	let error = $state<string | null>(null);

	// Per-item note editing
	let editingNote = $state<string | null>(null);
	let noteDraft = $state<string>('');

	// In-flight guards (prevent double-submit)
	let savingKey = $state<string | null>(null);
	let resetting = $state(false);

	// Registration prompt
	let showRegisterPrompt = $state(false);

	const destinations = $derived(
		selector?.destinations ?? []
	);
	const selectedDestOption = $derived(
		destinations.find((d) => d.id === selectedDestination)
	);
	const visaTypes = $derived(selectedDestOption?.visa_types ?? []);

	const seo = $derived(
		buildSeoMeta({
			title: t('checklist.seo_title', 'Visa Document Checklist — JapaConnect Tools'),
			description:
				t('checklist.seo_description', 'Select your origin, destination, and visa type to get a personalized, checkable document list with progress tracking.'),
			path: '/visa-checklist',
			origin: page.url.origin
		})
	);

	async function loadGuide(guideId: string) {
		loadingGuide = true;
		error = null;
		const res = await api.get<ChecklistGuide>(`/checklist/${guideId}`);
		loadingGuide = false;
		if (res.error) {
			error = res.error.message;
			return;
		}
		guide = res.data;
		// Load progress if authenticated
		if (user) {
			const progRes = await api.get<ChecklistProgress>(`/checklist/${guideId}/progress`);
			if (!progRes.error) progress = progRes.data;
		}
		// Update URL for shareability (SvelteKit replaceState, not window.history)
		const url = new URL(page.url);
		url.searchParams.set('guide_id', guideId);
		replaceState(url, {});
	}

	function onSelectVisaType(guideId: string, visaValue: string) {
		selectedVisaType = visaValue;
		loadGuide(guideId);
	}

	function getProgressItem(itemKey: string) {
		return progress?.items.find((i) => i.item_key === itemKey);
	}

	function isCompleted(itemKey: string): boolean {
		return getProgressItem(itemKey)?.completed ?? false;
	}

	async function toggleItem(item: ChecklistItem) {
		if (!guide) return;
		if (!user) {
			showRegisterPrompt = true;
			return;
		}
		const itemKey = item.item_key;
		if (savingKey === itemKey) return; // prevent double-submit
		savingKey = itemKey;
		const currentlyCompleted = isCompleted(itemKey);
		// Optimistic update
		if (progress) {
			const existing = progress.items.find((i) => i.item_key === itemKey);
			if (existing) {
				existing.completed = !currentlyCompleted;
				existing.completed_at = !currentlyCompleted ? new Date().toISOString() : null;
			} else {
				progress.items.push({
					item_key: itemKey,
					completed: !currentlyCompleted,
					completed_at: !currentlyCompleted ? new Date().toISOString() : null
				});
			}
			progress = { ...progress };
		}
		const res = await api.patch(
			`/checklist/${guide.guide_id}/progress/${itemKey}`,
			{ completed: !currentlyCompleted }
		);
		savingKey = null;
		if (res.error) {
			// Rollback
			if (progress) {
				const item2 = progress.items.find((i) => i.item_key === itemKey);
				if (item2) {
					item2.completed = currentlyCompleted;
					item2.completed_at = currentlyCompleted ? new Date().toISOString() : null;
				}
				progress = { ...progress };
			}
		}
	}

	function startEditNote(itemKey: string) {
		editingNote = itemKey;
		noteDraft = getProgressItem(itemKey)?.notes ?? '';
	}

	async function saveNote(itemKey: string) {
		if (!guide || !user) return;
		if (savingKey === itemKey) return; // prevent double-submit
		savingKey = itemKey;
		editingNote = null;
		const res = await api.patch(
			`/checklist/${guide.guide_id}/progress/${itemKey}`,
			{ notes: noteDraft }
		);
		savingKey = null;
		if (res.error) return;
		if (progress) {
			const existing = progress.items.find((i) => i.item_key === itemKey);
			if (existing) {
				existing.notes = noteDraft;
			} else {
				progress.items.push({ item_key: itemKey, completed: false, notes: noteDraft });
			}
			progress = { ...progress };
		}
	}

	/** Reset all progress for the current guide (§70: DELETE /checklist/:guide_id/progress). */
	async function resetProgress() {
		if (!guide || !user) return;
		if (resetting) return; // prevent double-submit
		if (!confirm(t('checklist.reset_confirm', 'Reset all progress for this checklist? This cannot be undone.'))) return;
		resetting = true;
		const res = await api.delete(`/checklist/${guide.guide_id}/progress`);
		resetting = false;
		if (res.error) {
			error = res.error.message;
			return;
		}
		progress = null;
		error = null;
	}

	// ── Request-a-guide (§70: POST /forms/{id}/submissions via §06) ────────
	// When no visa guide exists for the selected origin + destination, the
	// user can submit a request. The form slug is fixed ('visa-guide-request',
	// seeded by migration 000509); the form ID is resolved via the embed
	// endpoint and cached for the session.
	let requestFormId = $state<string | null>(null);
	let requestLoading = $state(false);
	let requestError = $state<string | null>(null);
	let requestSubmitted = $state(false);

	async function ensureRequestFormId(): Promise<string | null> {
		if (requestFormId) return requestFormId;
		const res = await api.get<{ id: string }>(`/forms/visa-guide-request/embed`);
		if (res.error || !res.data?.id) return null;
		requestFormId = res.data.id;
		return requestFormId;
	}

	async function submitGuideRequest() {
		if (!selectedOrigin || !selectedDestination || requestLoading) return;
		requestLoading = true;
		requestError = null;
		requestSubmitted = false;
		const formId = await ensureRequestFormId();
		if (!formId) {
			requestError = t('checklist.request_unavailable', 'Request form unavailable. Please try again later.');
			requestLoading = false;
			return;
		}
		const originName = selector?.origins.find((o) => o.id === selectedOrigin)?.name ?? selectedOrigin;
		const destName = destinations.find((d) => d.id === selectedDestination)?.name ?? selectedDestination;
		const res = await api.post(`/forms/${formId}/submissions`, {
			field_values: {
				origin_country: originName,
				destination_country: destName,
				visa_type: 'other',
				notes: ''
			}
		});
		requestLoading = false;
		if (res.error) {
			requestError = res.error.message;
			return;
		}
		requestSubmitted = true;
	}

	// Group items by category
	const groupedItems = $derived.by(() => {
		if (!guide?.items) return [];
		const groups: Record<string, ChecklistItem[]> = {};
		for (const item of guide.items) {
			const cat = item.category ?? t('checklist.general_category', 'General');
			if (!groups[cat]) groups[cat] = [];
			groups[cat].push(item);
		}
		return Object.entries(groups);
	});

	const completedCount = $derived(
		guide?.items.filter((i) => isCompleted(i.item_key)).length ?? 0
	);
	const totalCount = $derived(guide?.items.length ?? 0);
	const progressPct = $derived(totalCount > 0 ? Math.round((completedCount / totalCount) * 100) : 0);

	function fetchOrigins(q: string): Promise<AutocompleteItem[]> {
		const opts = selector?.origins ?? [];
		const query = q.toLowerCase();
		const filtered = query === ''
			? opts
			: opts.filter((o) => o.name.toLowerCase().includes(query));
		return Promise.resolve(filtered.map((o) => ({ id: o.id, label: o.name })));
	}

	function fetchOriginsInitial(): Promise<AutocompleteItem[]> {
		return fetchOrigins('');
	}

	function getOriginLabel(id: string): string {
		return selector?.origins.find((o) => o.id === id)?.name ?? id;
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

<svelte:head>
	<title>{seo.title}</title>
	<meta name="description" content={seo.description} />
	<link rel="canonical" href={seo.canonical} />
	<meta property="og:title" content={seo.title} />
	<meta property="og:description" content={seo.description} />
	<meta property="og:url" content={seo.canonical} />
	<meta property="og:type" content="website" />
	<meta name="twitter:card" content="summary" />
	<meta name="twitter:title" content={seo.title} />
	<meta name="twitter:description" content={seo.description} />
</svelte:head>

<div class="mx-auto max-w-3xl px-4 py-8">
	{#if !data.enabled}
		<ToolDisabled tool={data.tool} />
	{:else}
	<div class="mb-8">
		<h1 class="text-2xl sm:text-3xl font-bold text-text">{t('checklist.title', 'Visa Document Checklist')}</h1>
		<p class="mt-2 text-text-subtle">
			{t('checklist.subtitle', 'Select your origin, destination, and visa type to get a personalized document list.')}
		</p>
	</div>

	{#if !selector}
		<EmptyState
			icon="file-question"
			title={t('checklist.unavailable', 'Checklist unavailable')}
			description={t('checklist.unavailable_desc', "The visa checklist service isn't configured yet. Please check back later.")}
		/>
	{:else if !guide}
		<!-- Three-step selector -->
		<div class="space-y-6">
			<!-- Step 1: Origin -->
			<div>
				<label for="origin" class="block text-sm font-medium text-text mb-2">
					<span class="text-muted">1.</span> {t('checklist.step1_label', 'Your country (origin)')}
				</label>
				<Autocomplete
					id="origin"
					placeholder={t('checklist.step1_placeholder', 'Select your country')}
					emptyMessage={t('geo.no_destinations', 'No destinations found')}
					fetcher={fetchOrigins}
					initialFetcher={fetchOriginsInitial}
					selectedId={selectedOrigin || null}
					selectedLabel={selectedOrigin ? getOriginLabel(selectedOrigin) : null}
					onSelect={(item) => (selectedOrigin = item.id)}
					onClear={() => (selectedOrigin = '')}
				/>
			</div>

			<!-- Step 2: Destination -->
			<div>
				<label for="dest" class="block text-sm font-medium text-text mb-2">
					<span class="text-muted">2.</span> {t('checklist.step2_label', 'Destination country')}
				</label>
				<Autocomplete
					id="dest"
					placeholder={t('checklist.step2_placeholder', 'Select destination')}
					emptyMessage={t('geo.no_destinations', 'No destinations found')}
					fetcher={fetchDestinations}
					initialFetcher={fetchDestinationsInitial}
					selectedId={selectedDestination || null}
					selectedLabel={selectedDestination ? getDestinationLabel(selectedDestination) : null}
					onSelect={(item) => (selectedDestination = item.id)}
					onClear={() => (selectedDestination = '')}
				/>
			</div>

			<!-- Step 3: Visa type -->
			{#if visaTypes.length > 0}
				<div>
					<span class="block text-sm font-medium text-text mb-2">
						<span class="text-muted">3.</span> {t('checklist.step3_label', 'Visa type')}
					</span>
					<div class="grid grid-cols-2 sm:grid-cols-3 gap-3">
						{#each visaTypes as vt (vt.value)}
							<button
								onclick={() => onSelectVisaType(vt.guide_id, vt.value)}
								class="px-4 py-3 text-sm font-medium text-text bg-surface border border-border rounded-lg hover:border-primary hover:text-primary transition-colors text-center"
							>
								{vt.label}
							</button>
						{/each}
					</div>
				</div>
			{/if}

			{#if selectedOrigin && destinations.length === 0}
				<p class="text-sm text-muted">{t('checklist.no_destinations', 'No visa guides have been published yet. Please check back later.')}</p>
			{:else if selectedDestination && visaTypes.length === 0}
				<p class="text-sm text-muted">{t('checklist.no_guides', 'No visa guides found for this destination.')}</p>
			{/if}

			<!-- Request-a-guide CTA (§70: form_submission via §06, slug 'visa-guide-request') -->
			{#if (selectedOrigin && destinations.length === 0) || (selectedDestination && visaTypes.length === 0)}
				{#if requestSubmitted}
					<p class="text-sm text-primary">
						{t('checklist.request_success', 'Your request has been submitted. Our team will prioritize this route.')}
					</p>
				{:else}
					<button
						type="button"
						disabled={!selectedOrigin || !selectedDestination || requestLoading}
						onclick={submitGuideRequest}
						class="inline-flex items-center gap-1.5 px-3 py-1.5 text-sm bg-surface border border-border rounded hover:border-primary hover:text-primary transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
					>
						<Icon name="message-square-plus" size={16} />
						{requestLoading
							? t('checklist.request_submitting', 'Submitting…')
							: t('checklist.request_guide', 'Request a guide for this route')}
					</button>
					{#if requestError}
						<p class="mt-2 text-xs text-red-500">{requestError}</p>
					{/if}
				{/if}
			{/if}
		</div>
	{:else}
		<!-- Checklist view -->
		<div class="space-y-6">
			<div class="flex items-center justify-between">
				<div>
					<h2 class="text-xl font-semibold text-text">{guide.title}</h2>
					{#if totalCount > 0}
						<p class="text-sm text-muted mt-1">
							{t('checklist.progress_label', '{completed} of {total} documents ready ({pct}%)').replace('{completed}', String(completedCount)).replace('{total}', String(totalCount)).replace('{pct}', String(progressPct))}
						</p>
					{/if}
				</div>
				<div class="flex items-center gap-3">
					{#if user && progress && completedCount > 0}
						<button
							onclick={resetProgress}
							disabled={resetting}
							class="inline-flex items-center gap-1.5 px-3 py-1.5 text-sm text-muted hover:text-red-500 transition-colors"
						>
							<Icon name="rotate-ccw" size={16} />
							{t('checklist.reset_progress', 'Reset progress')}
						</button>
					{/if}
					<button
						onclick={() => { guide = null; progress = null; selectedVisaType = ''; }}
						class="inline-flex items-center gap-1.5 px-3 py-1.5 text-sm text-muted hover:text-text transition-colors"
					>
						<Icon name="arrow-left" size={16} />
						{t('checklist.change_selection', 'Change selection')}
					</button>
				</div>
			</div>

			{#if totalCount > 0}
				<div class="w-full h-2 bg-surface-subtle rounded-full overflow-hidden">
					<div
						class="h-full bg-[var(--tool-accent)] transition-all duration-300"
						style="width: {progressPct}%;"
					></div>
				</div>
			{/if}

			{#if loadingGuide}
				<div class="space-y-3">
					<LoadingSkeleton height="60px" />
					<LoadingSkeleton height="60px" />
					<LoadingSkeleton height="60px" />
				</div>
			{:else if groupedItems.length === 0}
				<EmptyState
					icon="file-question"
					title={t('checklist.no_documents', 'No documents found')}
					description={t('checklist.no_documents_desc', "This visa guide doesn't have any checklist items configured yet.")}
				/>
			{:else}
				<div class="space-y-6">
					{#each groupedItems as [category, items] (category)}
						<div>
							<h3 class="text-sm font-semibold text-muted uppercase tracking-wide mb-3">
								{category === 'General' ? t('checklist.general_category', 'General') : category}
							</h3>
							<div class="space-y-2">
								{#each items as item (item.item_key)}
									<div
										class="flex items-start gap-3 p-4 bg-surface border border-border rounded-lg"
									>
										<button
											onclick={() => toggleItem(item)}
											disabled={savingKey === item.item_key}
											class="flex-shrink-0 mt-0.5 w-5 h-5 rounded border-2 flex items-center justify-center transition-colors {isCompleted(item.item_key) ? 'bg-[var(--tool-accent)] border-[var(--tool-accent)]' : 'border-border hover:border-[var(--tool-accent)]'}"
											aria-label={isCompleted(item.item_key) ? t('checklist.mark_incomplete', 'Mark incomplete') : t('checklist.mark_complete', 'Mark complete')}
										>
											{#if isCompleted(item.item_key)}
												<Icon name="check" size={14} class="text-white" />
											{/if}
										</button>
										<div class="flex-1 min-w-0">
											<div class="flex items-center gap-2">
												<span class="text-sm font-medium text-text {isCompleted(item.item_key) ? 'line-through text-muted' : ''}">
													{item.label}
												</span>
												{#if item.required}
													<span class="text-xs px-1.5 py-0.5 bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-400 rounded font-medium">
														{t('checklist.required', 'Required')}
													</span>
												{/if}
											</div>
											{#if item.description}
												<p class="text-sm text-text-subtle mt-1">{item.description}</p>
											{/if}
											{#if getProgressItem(item.item_key)?.notes}
												<p class="text-sm text-muted mt-1 italic">
													{t('checklist.note_label', 'Note:')} {getProgressItem(item.item_key)?.notes}
												</p>
											{/if}
											{#if editingNote === item.item_key}
												<div class="mt-2 flex gap-2">
													<input
														type="text"
														bind:value={noteDraft}
														placeholder={t('checklist.note_placeholder', 'Add a note...')}
														class="flex-1 px-3 py-1.5 text-sm bg-surface border border-border rounded focus:border-primary focus:outline-none"
													/>
													<button
														onclick={() => saveNote(item.item_key)}
														disabled={savingKey === item.item_key}
														class="px-3 py-1.5 text-sm bg-primary text-primary-contrast rounded hover:bg-primary-hover disabled:opacity-50 disabled:cursor-not-allowed"
													>
														{t('common.save', 'Save')}
													</button>
												</div>
											{:else if user}
												<button
													onclick={() => startEditNote(item.item_key)}
													class="mt-1 text-xs text-muted hover:text-text transition-colors"
												>
													{t('checklist.add_note', '+ Add note')}
												</button>
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

	{#if showRegisterPrompt}
		<RegistrationPrompt
			open={showRegisterPrompt}
			onClose={() => (showRegisterPrompt = false)}
			icon="user-plus"
			title={t('register.save_progress', 'Save your progress')}
			description={t('common.create_account_desc', 'Create a free account to save your checklist progress, add notes, and track your documents across devices.')}
		/>
	{/if}
	{/if}
</div>
