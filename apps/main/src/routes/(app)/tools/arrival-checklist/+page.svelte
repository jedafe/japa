<script lang="ts">
	import { untrack } from 'svelte';
	import { replaceState } from '$app/navigation';
	import { t } from '$lib/i18n';
	import { page } from '$app/state';
	import {
		PlaneLanding,
		MapPin,
		CalendarDays,
		Flag,
		ExternalLink,
		ClipboardList,
		Lock,
		Square,
		CheckSquare,
		Loader2,
		Play
	, TriangleAlert } from 'lucide-svelte';
	import { EmptyState, LoadingSkeleton, SelectField } from '$lib/components/ui';
	import { api } from '$lib/api/client';
	import type {
		ArrivalChecklistSelectorEntry,
		ArrivalChecklistItem,
		GeoSpaceOption,
		UserArrivalChecklist,
		UserArrivalChecklistItem
	} from '$lib/types';
	import type { PageData } from './$types';

	/**
	 * Arrival Checklist tool — §92, ADR-088.
	 * Destination selector → optional visa category → checklist items grouped
	 * by phase. Authenticated users can start checklists, tick items, and add
	 * notes; those saves persist through the /me/arrival-checklists endpoints.
	 */
	let { data }: { data: PageData } = $props();

	const title = $derived(t('arrival_checklist.title', 'Arrival Checklist'));
	const description = $derived(
		t(
			'arrival_checklist.subtitle',
			'Everything you need to do after you land — phased by time period.'
		)
	);
	const disabled = $derived(data.disabled);
	const authed = $derived(data.authed);

	let selectorEntries = $state<ArrivalChecklistSelectorEntry[]>(
		untrack(() => data.selector)
	);
	let destinations = $state<GeoSpaceOption[]>(untrack(() => data.destinations));
	let items = $state<ArrivalChecklistItem[]>(untrack(() => data.items));
	let progressItems = $state<UserArrivalChecklistItem[]>([]);
	let myChecklists = $state<UserArrivalChecklist[]>(untrack(() => data.myChecklists ?? []));
	let selectedDestId = $state<string>(untrack(() => data.initialDestId ?? ''));
	let selectedVisaCategory = $state<string>('');
	let loadingItems = $state(false);
	let activeGuideId = $state<string>('');
	let starting = $state(false);
	let saving = $state<Record<string, boolean>>({});
	let notesDraft = $state<Record<string, string>>({});
	let editingNotes = $state<Record<string, boolean>>({});
	let error = $state<string | null>(untrack(() => data.loadError ?? null));

	// Visa categories available for the selected destination (from selector).
	const visaCategories = $derived(
		selectedDestId
			? selectorEntries
					.filter((e) => e.destination_space_id === selectedDestId && e.visa_category)
					.map((e) => e.visa_category as string)
			: []
	);

	const showVisaCategorySelector = $derived(visaCategories.length > 1);

	const activeChecklist = $derived(
		myChecklists.find((c) => c.guide_id === activeGuideId) ?? null
	);

	const usingProgress = $derived(progressItems.length > 0);

	// Group visible items by phase, sorted by sort_order within each phase.
	const phasedItems = $derived.by(() => {
		const list: { id: string; phase: string; title: string; description?: string | null; official_link?: string | null; sort_order: number; is_flagged?: boolean; completed?: boolean; notes?: string; item_id: string }[] = [];
		if (usingProgress) {
			for (const p of progressItems) {
				list.push({
					id: p.id,
					phase: p.phase,
					title: p.title,
					description: p.description,
					official_link: p.official_link,
					sort_order: p.sort_order,
					is_flagged: p.is_flagged,
					completed: p.completed,
					notes: p.notes,
					item_id: p.id
				});
			}
		} else {
			for (const i of items) {
				list.push({
					id: i.id,
					phase: i.phase,
					title: i.title,
					description: i.description,
					official_link: i.official_link,
					sort_order: i.sort_order,
					is_flagged: i.is_flagged,
					completed: false,
					notes: '',
					item_id: i.id
				});
			}
		}
		const phases = new Map<string, typeof list>();
		for (const item of list) {
			const arr = phases.get(item.phase) ?? [];
			arr.push(item);
			phases.set(item.phase, arr);
		}
		const result: { phase: string; items: typeof list }[] = [];
		for (const [phase, phaseItems] of phases) {
			result.push({
				phase,
				items: phaseItems.sort((a, b) => a.sort_order - b.sort_order)
			});
		}
		return result;
	});

	function getActiveEntry(): ArrivalChecklistSelectorEntry | undefined {
		return selectorEntries.find(
			(e) =>
				e.destination_space_id === selectedDestId &&
				(!selectedVisaCategory || e.visa_category === selectedVisaCategory)
		);
	}

	async function loadItems() {
		progressItems = [];
		activeGuideId = '';
		error = null;
		if (!selectedDestId) {
			items = [];
			return;
		}
		const entry = getActiveEntry();
		if (!entry?.guide_id) {
			items = [];
			return;
		}
		activeGuideId = entry.guide_id;
		loadingItems = true;
		const [publicRes, myRes] = await Promise.all([
			api.get<ArrivalChecklistItem[]>(`/arrival-checklist/${entry.guide_id}`),
			authed
				? api.get<UserArrivalChecklistItem[]>(`/me/arrival-checklists/${entry.guide_id}`)
				: Promise.resolve({ data: null, error: null, status: 0 })
		]);
		loadingItems = false;
		const errs: string[] = [];
		if (!publicRes.error) {
			items = publicRes.data ?? [];
		} else {
			items = [];
			errs.push(publicRes.error.message);
		}
		if (!myRes.error && myRes.data) {
			progressItems = myRes.data ?? [];
			for (const p of progressItems) notesDraft[p.id] = p.notes ?? '';
		} else {
			progressItems = [];
			if (myRes.error) errs.push(myRes.error.message);
		}
		if (errs.length > 0) error = errs.join(' ');
	}

	async function startChecklist() {
		if (!authed || !activeGuideId) return;
		starting = true;
		error = null;
		const res = await api.post<{ id: string; destination_space_name?: string }>('/me/arrival-checklists', {
			guide_id: activeGuideId
		});
		starting = false;
		if (res.error) {
			error = res.error.message;
			return;
		}
		await loadItems();
		// Refresh the my-checklists list.
		const listRes = await api.get<UserArrivalChecklist[]>('/me/arrival-checklists');
		if (listRes.error) {
			error = listRes.error.message;
		} else {
			myChecklists = listRes.data ?? [];
		}
	}

	async function toggleItem(itemId: string, completed: boolean) {
		if (!authed || !activeGuideId) return;
		saving[itemId] = true;
		error = null;
		const res = await api.patch(`/me/arrival-checklists/${activeGuideId}/items/${itemId}`, { completed });
		saving[itemId] = false;
		if (res.error) {
			error = res.error.message;
			return;
		}
		progressItems = progressItems.map((p) => (p.id === itemId ? { ...p, completed } : p));
	}

	function startEditNotes(itemId: string, current: string) {
		editingNotes[itemId] = true;
		notesDraft[itemId] = current ?? '';
	}

	async function saveNotes(itemId: string) {
		if (!authed || !activeGuideId) return;
		editingNotes[itemId] = false;
		saving[itemId] = true;
		error = null;
		const res = await api.patch(`/me/arrival-checklists/${activeGuideId}/items/${itemId}`, {
			notes: notesDraft[itemId]
		});
		saving[itemId] = false;
		if (res.error) {
			error = res.error.message;
			return;
		}
		progressItems = progressItems.map((p) =>
			p.id === itemId ? { ...p, notes: notesDraft[itemId] } : p
		);
	}

	function onDestChange(id: string) {
		selectedDestId = id;
		selectedVisaCategory = '';
		error = null;
		const url = new URL(page.url);
		if (id) url.searchParams.set('dest', id);
		else url.searchParams.delete('dest');
		replaceState(url, {});
		loadItems();
	}

	function onVisaCategoryChange(v: string) {
		selectedVisaCategory = v;
		error = null;
		loadItems();
	}

	async function resumeChecklist(guideId: string) {
		const entry = selectorEntries.find((e) => e.guide_id === guideId);
		if (!entry) return;
		selectedDestId = entry.destination_space_id;
		selectedVisaCategory = entry.visa_category ?? '';
		const url = new URL(page.url);
		url.searchParams.set('dest', selectedDestId);
		replaceState(url, {});
		await loadItems();
	}
</script>

<svelte:head>
	<title>{title}</title>
	<meta name="description" content={description} />
	<meta property="og:title" content={title} />
	<meta property="og:description" content={description} />
	<meta property="og:type" content="website" />
	<meta property="og:url" content={page.url.href} />
</svelte:head>

<div class="mx-auto w-full max-w-4xl">
	<div class="mx-4 border-b border-[var(--color-border)] px-4 py-3">
		<h1 class="flex items-center gap-2 text-lg font-semibold text-[var(--color-text)]">
			<PlaneLanding size={20} /> {title}
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
	{:else}
		<div class="flex flex-col gap-4 p-4 lg:flex-row">
			<!-- Main checklist -->
			<div class="flex-1">
				{#if error}
					<div class="mb-4 rounded-[var(--radius)] border border-red-200 bg-red-50 p-3 text-sm text-red-700 dark:border-red-900 dark:bg-red-950 dark:text-red-300">
						{error}
					</div>
				{/if}

				<!-- Destination + category selectors -->
				<div class="mb-4 flex flex-wrap items-end gap-3">
					<div class="w-full sm:w-auto">
						<label for="arr-dest" class="block text-xs font-medium uppercase tracking-wide text-[var(--color-muted)]">
							{t('arrival_checklist.destination', 'Destination')}
						</label>
						<SelectField
							id="arr-dest"
							value={selectedDestId}
							options={destinations.map((d) => ({ value: d.id, label: d.name }))}
							placeholder={t('arrival_checklist.select_destination', 'Select destination')}
							onValueChange={onDestChange}
						/>
					</div>
					{#if showVisaCategorySelector}
						<div class="w-full sm:w-auto">
							<label for="arr-visa" class="block text-xs font-medium uppercase tracking-wide text-[var(--color-muted)]">
								{t('arrival_checklist.visa_category', 'Visa category')}
							</label>
							<SelectField
								id="arr-visa"
								value={selectedVisaCategory}
								options={visaCategories.map((c) => ({ value: c, label: c }))}
								placeholder={t('arrival_checklist.select_category', 'Select category')}
								onValueChange={onVisaCategoryChange}
							/>
						</div>
					{/if}
				</div>

				{#if page.data.features?.['scam_alert.enabled'] && selectedDestId}
					<div class="mb-4 flex items-center gap-2 rounded-[var(--radius)] border border-amber-500/40 bg-amber-500/10 px-3 py-2 text-sm">
						<TriangleAlert size={15} class="shrink-0 text-amber-600" />
						<a href="/scam-alerts?dest={selectedDestId}" class="font-medium text-amber-700 hover:underline">
							{t('scam_alerts.checklist_link', 'Check confirmed scam alerts for this destination')}
						</a>
					</div>
				{/if}

				{#if loadingItems}
					<LoadingSkeleton variant="lines" />
				{:else if selectedDestId && phasedItems.length === 0}
					<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
						<EmptyState
							icon={PlaneLanding}
							title={t('arrival_checklist.no_items', 'No checklist items')}
							body={t('arrival_checklist.no_items_desc', 'Try a different destination or visa category.')}
						/>
					</div>
				{:else if selectedDestId}
					{#if authed && !usingProgress}
						<div class="mb-4 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
							<div class="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
								<div>
									<h2 class="text-sm font-semibold text-[var(--color-text)]">{t('arrival_checklist.track_progress', 'Track your progress')}</h2>
									<p class="text-xs text-[var(--color-muted)]">{t('arrival_checklist.track_progress_desc', 'Save this checklist and tick items off as you complete them.')}</p>
								</div>
								<button
									type="button"
									onclick={startChecklist}
									disabled={starting}
									class="inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-white hover:opacity-90 disabled:opacity-50"
								>
									{#if starting}
										<Loader2 size={16} class="animate-spin" />
									{:else}
										<Play size={16} />
									{/if}
									{t('arrival_checklist.start', 'Start checklist')}
								</button>
							</div>
						</div>
					{/if}

					{#if authed && activeChecklist}
						<p class="mb-3 text-sm text-[var(--color-text)]">
							{t('arrival_checklist.progress', '{completed} of {total} completed')
								.replace('{completed}', String(activeChecklist.completed_items))
								.replace('{total}', String(activeChecklist.total_items))}
						</p>
					{/if}

					<div class="space-y-4">
						{#each phasedItems as group (group.phase)}
							<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
								<h2 class="mb-3 flex items-center gap-2 text-sm font-semibold uppercase tracking-wide text-[var(--color-text)]">
									<CalendarDays size={16} />
									{group.phase.replace(/_/g, ' ')}
								</h2>
								<ul class="space-y-3">
									{#each group.items as item (item.id)}
										<li class="border-b border-[var(--color-border)] pb-3 last:border-0 last:pb-0">
											<div class="flex items-start gap-3">
												{#if authed && usingProgress}
													<button
														type="button"
														onclick={() => toggleItem(item.item_id, !item.completed)}
														disabled={saving[item.item_id]}
														class="mt-0.5 shrink-0 text-[var(--color-primary)] disabled:opacity-50"
														aria-label={item.completed ? t('common.mark_incomplete', 'Mark incomplete') : t('common.mark_complete', 'Mark complete')}
													>
														{#if item.completed}
															<CheckSquare size={18} />
														{:else}
															<Square size={18} />
														{/if}
													</button>
												{:else if item.is_flagged}
													<Flag size={16} class="mt-0.5 text-[var(--color-warning)]" />
												{/if}
												<div class="min-w-0 flex-1">
													<h3 class="text-sm font-medium text-[var(--color-text)] {item.completed ? 'line-through opacity-60' : ''}">{item.title}</h3>
													{#if item.description}
														<p class="mt-1 text-xs text-[var(--color-text-subtle)]">{item.description}</p>
													{/if}
													{#if item.official_link}
														<a
															href={item.official_link}
															target="_blank"
															rel="noopener noreferrer"
															class="mt-1 inline-flex items-center gap-1 text-xs text-[var(--color-primary)] hover:underline"
														>
															<ExternalLink class="rtl:-scale-x-100" size={12} />
															{t('arrival_checklist.official_link', 'Official info')}
														</a>
													{/if}

													{#if authed && usingProgress}
														<div class="mt-2">
															{#if editingNotes[item.item_id]}
																<div class="flex flex-col gap-2">
																	<textarea
																		bind:value={notesDraft[item.item_id]}
																		rows="2"
																		placeholder={t('arrival_checklist.notes_placeholder', 'Add notes...')}
																		class="w-full resize-none rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1 text-xs text-[var(--color-text)] focus:outline-none focus:ring-2 focus:ring-[var(--color-primary)]"
																	></textarea>
																	<div class="flex gap-2">
																		<button
																			type="button"
																			onclick={() => saveNotes(item.item_id)}
																			class="rounded-[var(--radius)] bg-[var(--color-primary)] px-2 py-1 text-xs font-medium text-white hover:opacity-90"
																		>
																			{t('common.save', 'Save')}
																		</button>
																		<button
																			type="button"
																			onclick={() => (editingNotes[item.item_id] = false)}
																			class="text-xs text-[var(--color-muted)] hover:text-[var(--color-text)]"
																		>
																			{t('common.cancel', 'Cancel')}
																		</button>
																	</div>
																</div>
															{:else}
																<div class="flex items-start justify-between gap-2">
																	<p class="text-xs text-[var(--color-text-subtle)]">{item.notes || t('arrival_checklist.no_notes', 'No notes')}</p>
																	<button
																		type="button"
																		onclick={() => startEditNotes(item.item_id, item.notes ?? '')}
																		class="shrink-0 text-xs text-[var(--color-muted)] hover:text-[var(--color-text)]"
																	>
																		{t('common.edit', 'Edit')}
																	</button>
																</div>
															{/if}
														</div>
													{/if}
												</div>
											</div>
										</li>
									{/each}
								</ul>
							</div>
						{/each}
					</div>
				{:else}
					<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
						<EmptyState
							icon={MapPin}
							title={t('arrival_checklist.select_destination_prompt', 'Select a destination')}
							body={t('arrival_checklist.select_destination_desc', 'Choose your destination to see the relevant arrival checklist.')}
						/>
					</div>
				{/if}
			</div>

			<!-- My checklists sidebar -->
			{#if authed}
				<aside class="w-full shrink-0 lg:w-64">
					<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
						<h2 class="mb-3 flex items-center gap-2 text-sm font-semibold text-[var(--color-text)]">
							<ClipboardList size={16} />
							{t('arrival_checklist.my_checklists', 'My checklists')}
						</h2>
						{#if myChecklists.length === 0}
							<p class="text-xs text-[var(--color-muted)]">{t('arrival_checklist.no_saved', 'No saved checklists yet.')}</p>
						{:else}
							<ul class="space-y-2">
								{#each myChecklists as c (c.guide_id)}
									<li>
										<button
											type="button"
											onclick={() => resumeChecklist(c.guide_id)}
											class="w-full rounded-[var(--radius)] p-2 text-start hover:bg-[var(--color-surface-subtle)] {activeGuideId === c.guide_id ? 'bg-[var(--color-primary)]/10' : ''}"
										>
											<div class="text-sm font-medium text-[var(--color-text)]">
												{c.destination_space_name ?? c.destination_space_code ?? c.guide_id}
											</div>
											<div class="text-xs text-[var(--color-muted)]">
												{c.completed_items} / {c.total_items} {t('arrival_checklist.done', 'done')}
											</div>
										</button>
									</li>
								{/each}
							</ul>
						{/if}
					</div>
				</aside>
			{/if}
		</div>
	{/if}
</div>
