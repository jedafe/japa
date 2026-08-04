<script lang="ts">
	import { api } from '$lib/api/client';
	import { mainAppUrl } from '$lib/seo';
	import { buildSeoMeta } from '$lib/seo';
	import { page } from '$app/state';
	import Icon from '$lib/components/ui/Icon.svelte';
	import EmptyState from '$lib/components/ui/EmptyState.svelte';
	import LoadingSkeleton from '$lib/components/ui/LoadingSkeleton.svelte';
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

	let selector = $state<ChecklistSelector | null>(data.selector);
	let guide = $state<ChecklistGuide | null>(data.guide);
	let progress = $state<ChecklistProgress | null>(data.progress);
	let user = $state(data.user);

	// Selector state
	let selectedOrigin = $state<string>('');
	let selectedDestination = $state<string>('');
	let selectedVisaType = $state<string>('');

	let loadingGuide = $state(false);
	let error = $state<string | null>(null);

	// Per-item note editing
	let editingNote = $state<string | null>(null);
	let noteDraft = $state<string>('');

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
			title: 'Visa Document Checklist — JapaConnect Tools',
			description:
				'Select your origin, destination, and visa type to get a personalized, checkable document list with progress tracking.',
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
		// Update URL for shareability
		const url = new URL(window.location.href);
		url.searchParams.set('guide_id', guideId);
		window.history.replaceState({}, '', url);
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
		editingNote = null;
		const res = await api.patch(
			`/checklist/${guide.guide_id}/progress/${itemKey}`,
			{ notes: noteDraft }
		);
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

	// Group items by category
	const groupedItems = $derived.by(() => {
		if (!guide?.items) return [];
		const groups: Record<string, ChecklistItem[]> = {};
		for (const item of guide.items) {
			const cat = item.category ?? 'General';
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
	<div class="mb-8">
		<h1 class="text-2xl sm:text-3xl font-bold text-text">Visa Document Checklist</h1>
		<p class="mt-2 text-text-subtle">
			Select your origin, destination, and visa type to get a personalized document list.
		</p>
	</div>

	{#if !selector}
		<div class="space-y-3">
			<LoadingSkeleton height="48px" />
			<LoadingSkeleton height="48px" />
			<LoadingSkeleton height="48px" />
		</div>
	{:else if !guide}
		<!-- Three-step selector -->
		<div class="space-y-6">
			<!-- Step 1: Origin -->
			<div>
				<label for="origin" class="block text-sm font-medium text-text mb-2">
					<span class="text-muted">1.</span> Your country (origin)
				</label>
				<select
					id="origin"
					bind:value={selectedOrigin}
					class="w-full px-4 py-3 bg-surface border border-border rounded-lg text-text focus:border-primary focus:outline-none"
				>
					<option value="">Select your country</option>
					{#each selector.origins as opt (opt.id)}
						<option value={opt.id}>{opt.name}</option>
					{/each}
				</select>
			</div>

			<!-- Step 2: Destination -->
			<div>
				<label for="dest" class="block text-sm font-medium text-text mb-2">
					<span class="text-muted">2.</span> Destination country
				</label>
				<select
					id="dest"
					bind:value={selectedDestination}
					disabled={!selectedOrigin}
					class="w-full px-4 py-3 bg-surface border border-border rounded-lg text-text focus:border-primary focus:outline-none disabled:opacity-50"
				>
					<option value="">Select destination</option>
					{#each destinations as opt (opt.id)}
						<option value={opt.id}>{opt.name}</option>
					{/each}
				</select>
			</div>

			<!-- Step 3: Visa type -->
			{#if visaTypes.length > 0}
				<div>
					<span class="block text-sm font-medium text-text mb-2">
						<span class="text-muted">3.</span> Visa type
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

			{#if selectedDestination && visaTypes.length === 0}
				<p class="text-sm text-muted">No visa guides found for this destination.</p>
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
							{completedCount} of {totalCount} documents ready ({progressPct}%)
						</p>
					{/if}
				</div>
				<button
					onclick={() => { guide = null; progress = null; selectedVisaType = ''; }}
					class="inline-flex items-center gap-1.5 px-3 py-1.5 text-sm text-muted hover:text-text transition-colors"
				>
					<Icon name="arrow-left" size={16} />
					Change selection
				</button>
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
					title="No documents found"
					description="This visa guide doesn't have any checklist items configured yet."
				/>
			{:else}
				<div class="space-y-6">
					{#each groupedItems as [category, items] (category)}
						<div>
							<h3 class="text-sm font-semibold text-muted uppercase tracking-wide mb-3">
								{category}
							</h3>
							<div class="space-y-2">
								{#each items as item (item.item_key)}
									<div
										class="flex items-start gap-3 p-4 bg-surface border border-border rounded-lg"
									>
										<button
											onclick={() => toggleItem(item)}
											class="flex-shrink-0 mt-0.5 w-5 h-5 rounded border-2 flex items-center justify-center transition-colors {isCompleted(item.item_key) ? 'bg-[var(--tool-accent)] border-[var(--tool-accent)]' : 'border-border hover:border-[var(--tool-accent)]'}"
											aria-label={isCompleted(item.item_key) ? 'Mark incomplete' : 'Mark complete'}
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
														Required
													</span>
												{/if}
											</div>
											{#if item.description}
												<p class="text-sm text-text-subtle mt-1">{item.description}</p>
											{/if}
											{#if getProgressItem(item.item_key)?.notes}
												<p class="text-sm text-muted mt-1 italic">
													Note: {getProgressItem(item.item_key)?.notes}
												</p>
											{/if}
											{#if editingNote === item.item_key}
												<div class="mt-2 flex gap-2">
													<input
														type="text"
														bind:value={noteDraft}
														placeholder="Add a note..."
														class="flex-1 px-3 py-1.5 text-sm bg-surface border border-border rounded focus:border-primary focus:outline-none"
													/>
													<button
														onclick={() => saveNote(item.item_key)}
														class="px-3 py-1.5 text-sm bg-primary text-primary-contrast rounded hover:bg-primary-hover"
													>
														Save
													</button>
												</div>
											{:else if user}
												<button
													onclick={() => startEditNote(item.item_key)}
													class="mt-1 text-xs text-muted hover:text-text transition-colors"
												>
													+ Add note
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
		<div
			class="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4"
			onclick={() => (showRegisterPrompt = false)}
			onkeydown={(e) => e.key === 'Escape' && (showRegisterPrompt = false)}
			role="button"
			tabindex="0"
		>
			<div
				class="bg-surface rounded-[var(--radius-card)] p-6 max-w-sm w-full text-center"
				onclick={(e) => e.stopPropagation()}
				onkeydown={(e) => e.stopPropagation()}
				role="button"
				tabindex="0"
			>
				<div class="flex justify-center mb-4">
					<div class="w-12 h-12 rounded-full bg-primary/10 flex items-center justify-center">
						<Icon name="user-plus" size={24} class="text-primary" />
					</div>
				</div>
				<h3 class="text-lg font-semibold text-text">Save your progress</h3>
				<p class="mt-2 text-sm text-text-subtle">
					Create a free account to save your checklist progress, add notes, and track
					your documents across devices.
				</p>
				<div class="mt-5 flex flex-col gap-2">
					<a
						href={mainAppUrl('/signup')}
						class="px-4 py-2.5 bg-primary text-primary-contrast rounded-lg text-sm font-medium hover:bg-primary-hover transition-colors"
					>
						Create free account
					</a>
					<a
						href={mainAppUrl('/login')}
						class="px-4 py-2.5 text-sm font-medium text-text hover:text-primary transition-colors"
					>
						Sign in
					</a>
				</div>
			</div>
		</div>
	{/if}
</div>
