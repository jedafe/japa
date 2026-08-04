<script lang="ts">
	import { t } from '$lib/i18n';
	import { page } from '$app/state';
	import { ClipboardCheck, Check, Lock } from 'lucide-svelte';
	import { EmptyState, LoadingSkeleton } from '$lib/components/ui';
	import { api } from '$lib/api/client';
	import type { ChecklistItem, ChecklistSelector } from '$lib/types';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	const title = $derived(t('tools.visa_checklist', 'Visa Checklist — JapaConnect'));
	const description = $derived(
		t('tools.visa_checklist_desc', 'Build a document checklist for your visa route.')
	);

	const selector = $state<ChecklistSelector | null>(data.selector);
	const disabled = $state(data.disabled);
	const authed = $state(data.authed);

	// Selector state — 3 steps.
	let selectedOrigin = $state('');
	let selectedDestination = $state('');
	let selectedVisaType = $state('');
	let selectedGuideId = $state('');

	// Checklist state.
	let items = $state<ChecklistItem[]>([]);
	let loadingItems = $state(false);
	let itemError = $state<string | null>(null);
	let savingKey = $state<string | null>(null);

	const destinations = $derived(selector?.destinations ?? []);
	const visaTypes = $derived(
		destinations.find((d) => d.id === selectedDestination)?.visa_types ?? []
	);

	const completedCount = $derived(items.filter((i) => i.completed).length);
	const progressPct = $derived(items.length > 0 ? Math.round((completedCount / items.length) * 100) : 0);

	async function loadChecklist(guideId: string) {
		loadingItems = true;
		itemError = null;
		items = [];
		const res = await api.get<{ items: ChecklistItem[] } | ChecklistItem[]>(`/checklist/${guideId}`);
		loadingItems = false;
		if (res.error) {
			itemError = res.error.message;
			return;
		}
		// Backend returns either { items: [...] } or a bare array.
		const raw = res.data as { items?: ChecklistItem[] } | ChecklistItem[];
		items = Array.isArray(raw) ? raw : (raw.items ?? []);
	}

	function onSelectDestination() {
		selectedVisaType = '';
		selectedGuideId = '';
		items = [];
	}

	async function onSelectVisaType(guideId: string) {
		selectedGuideId = guideId;
		await loadChecklist(guideId);
	}

	async function toggleItem(item: ChecklistItem) {
		if (!authed || !selectedGuideId) return;
		savingKey = item.item_key;
		const prev = item.completed;
		// Optimistic update.
		items = items.map((i) =>
			i.item_key === item.item_key ? { ...i, completed: !prev } : i
		);
		const res = await api.patch(
			`/checklist/${selectedGuideId}/progress/${item.item_key}`,
			{ completed: !prev }
		);
		savingKey = null;
		if (res.error) {
			// Rollback on failure.
			items = items.map((i) =>
				i.item_key === item.item_key ? { ...i, completed: prev } : i
			);
		}
	}

	async function saveNote(item: ChecklistItem, notes: string) {
		if (!authed || !selectedGuideId) return;
		savingKey = item.item_key;
		const res = await api.patch(
			`/checklist/${selectedGuideId}/progress/${item.item_key}`,
			{ notes }
		);
		savingKey = null;
		if (!res.error) {
			items = items.map((i) =>
				i.item_key === item.item_key ? { ...i, notes } : i
			);
		}
	}

	function onNoteBlur(item: ChecklistItem, e: Event) {
		const value = (e.target as HTMLTextAreaElement).value;
		if (value !== item.notes) saveNote(item, value);
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

<div class="mx-auto w-full max-w-3xl">
	<div class="border-b border-[var(--color-border)] px-4 py-3">
		<h1 class="flex items-center gap-2 text-lg font-semibold text-[var(--color-text)]">
			<ClipboardCheck size={20} /> {t('tools.visa_checklist', 'Visa Checklist')}
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
	{:else if !selector}
		<div class="p-4">
			<LoadingSkeleton variant="lines" />
		</div>
	{:else}
		<!-- Step 1: Origin -->
		<div class="border-b border-[var(--color-border)] px-4 py-3">
			<label for="vc-origin" class="block text-xs font-medium uppercase tracking-wide text-[var(--color-muted)]">
				{t('checklist.step1', '1. Where are you coming from?')}
			</label>
			<select
				id="vc-origin"
				bind:value={selectedOrigin}
				class="mt-2 w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] focus:outline-none focus:ring-2 focus:ring-[var(--color-primary)]"
			>
				<option value="">{t('checklist.select_origin', 'Select origin country')}</option>
				{#each selector.origins as opt (opt.id)}
					<option value={opt.id}>{opt.name}</option>
				{/each}
			</select>
		</div>

		<!-- Step 2: Destination -->
		{#if selectedOrigin}
			<div class="border-b border-[var(--color-border)] px-4 py-3">
				<label for="vc-dest" class="block text-xs font-medium uppercase tracking-wide text-[var(--color-muted)]">
					{t('checklist.step2', '2. Where are you going?')}
				</label>
				<select
					id="vc-dest"
					bind:value={selectedDestination}
					onchange={onSelectDestination}
					class="mt-2 w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] focus:outline-none focus:ring-2 focus:ring-[var(--color-primary)]"
				>
					<option value="">{t('checklist.select_destination', 'Select destination country')}</option>
					{#each destinations as opt (opt.id)}
						<option value={opt.id}>{opt.name}</option>
					{/each}
				</select>
			</div>
		{/if}

		<!-- Step 3: Visa type -->
		{#if selectedDestination && visaTypes.length > 0}
			<div class="border-b border-[var(--color-border)] px-4 py-3">
				<label for="vc-visa" class="block text-xs font-medium uppercase tracking-wide text-[var(--color-muted)]">
					{t('checklist.step3', '3. Visa type')}
				</label>
				<select
					id="vc-visa"
					bind:value={selectedVisaType}
					onchange={(e) => onSelectVisaType((e.target as HTMLSelectElement).value)}
					class="mt-2 w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] focus:outline-none focus:ring-2 focus:ring-[var(--color-primary)]"
				>
					<option value="">{t('checklist.select_visa', 'Select visa type')}</option>
					{#each visaTypes as vt (vt.value)}
						<option value={vt.guide_id}>{vt.label}</option>
					{/each}
				</select>
			</div>
		{:else if selectedDestination && visaTypes.length === 0}
			<div class="px-4 py-3 text-sm text-[var(--color-muted)]">
				{t('checklist.no_visa_types', 'No visa guides published for this destination yet.')}
			</div>
		{/if}

		<!-- Checklist -->
		{#if selectedGuideId}
			<div class="p-4">
				{#if !authed}
					<div class="mb-3 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface-subtle)] px-3 py-2 text-xs text-[var(--color-muted)]">
						{t('checklist.anon_hint', 'Sign in to save your progress and notes.')}
					</div>
				{/if}

				{#if loadingItems}
					<LoadingSkeleton variant="lines" />
				{:else if itemError}
					<EmptyState icon={ClipboardCheck} title={t('checklist.load_error', 'Could not load checklist')} body={itemError} />
				{:else if items.length === 0}
					<EmptyState icon={ClipboardCheck} title={t('checklist.empty', 'No checklist items for this guide')} />
				{:else}
					<!-- Progress bar -->
					<div class="mb-4">
						<div class="flex items-center justify-between text-xs text-[var(--color-muted)]">
							<span>{t('checklist.progress', 'Progress')}</span>
							<span>{completedCount}/{items.length} ({progressPct}%)</span>
						</div>
						<div class="mt-1 h-2 overflow-hidden rounded-full bg-[var(--color-surface-subtle)]">
							<div
								class="h-full bg-[var(--color-primary)] transition-all"
								style="width: {progressPct}%"
							></div>
						</div>
					</div>

					<ul class="space-y-2">
						{#each items as item (item.item_key)}
							<li
								class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-3"
							>
								<div class="flex items-start gap-3">
									<button
										type="button"
										disabled={!authed || savingKey === item.item_key}
										onclick={() => toggleItem(item)}
										class="mt-0.5 grid h-5 w-5 shrink-0 place-items-center rounded border transition-colors disabled:opacity-50
											{item.completed
											? 'border-[var(--color-primary)] bg-[var(--color-primary)] text-white'
											: 'border-[var(--color-border)] bg-[var(--color-surface)] text-transparent hover:border-[var(--color-primary)]'}"
										aria-label={item.label}
										aria-pressed={item.completed}
									>
										<Check size={14} />
									</button>
									<span class="min-w-0 flex-1">
										<span
											class="text-sm {item.completed
											? 'text-[var(--color-muted)] line-through'
											: 'text-[var(--color-text)]'}"
										>{item.label}</span>
										{#if authed}
											<textarea
												rows="1"
												placeholder={t('checklist.notes_placeholder', 'Add a note…')}
												onblur={(e) => onNoteBlur(item, e)}
												class="mt-1 w-full resize-y rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1 text-xs text-[var(--color-text)] focus:outline-none focus:ring-1 focus:ring-[var(--color-primary)]"
											>{item.notes}</textarea>
										{:else if item.notes}
											<span class="mt-1 block text-xs text-[var(--color-muted)]">{item.notes}</span>
										{/if}
									</span>
								</div>
							</li>
						{/each}
					</ul>
				{/if}
			</div>
		{/if}
	{/if}
</div>
