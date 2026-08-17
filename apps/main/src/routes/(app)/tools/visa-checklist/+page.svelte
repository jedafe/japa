<script lang="ts">
	import { t } from '$lib/i18n';
	import { page } from '$app/state';
	import { ClipboardCheck, Check, Lock, RotateCcw, MessageSquarePlus } from 'lucide-svelte';
	import { EmptyState, LoadingSkeleton, SelectField } from '$lib/components/ui';
	import { api } from '$lib/api/client';
	import type { ChecklistItem, ChecklistSelector } from '$lib/types';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	const title = $derived(t('tools.visa_checklist', 'Visa Checklist — JapaConnect'));
	const description = $derived(
		t('tools.visa_checklist_desc', 'Build a document checklist for your visa route.')
	);

	const selector = $derived<ChecklistSelector | null>(data.selector);
	const disabled = $derived(data.disabled);
	const authed = $derived(data.authed);

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
	let resetting = $state(false);

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
		let loaded: ChecklistItem[] = Array.isArray(raw) ? raw : (raw.items ?? []);

		// Merge saved progress for authenticated users (§70: GET /progress).
		// Without this, authenticated users see all items unchecked on every
		// load and their persisted notes/completions are invisible.
		if (authed) {
			const progRes = await api.get<{
				items?: { item_key: string; completed: boolean; notes?: string }[];
			}>(`/checklist/${guideId}/progress`);
			if (!progRes.error && progRes.data?.items) {
				const progMap = new Map(
					progRes.data.items.map((p) => [p.item_key, p])
				);
				loaded = loaded.map((it) => {
					const p = progMap.get(it.item_key);
					return p
						? { ...it, completed: p.completed, notes: p.notes ?? it.notes }
						: it;
				});
			}
		}
		items = loaded;
	}

	/** Reset all progress for the current guide (§70: DELETE /progress). */
	async function resetProgress() {
		if (!authed || !selectedGuideId || resetting) return;
		if (!confirm(t('checklist.reset_confirm', 'Reset all progress for this checklist? This cannot be undone.'))) return;
		resetting = true;
		const res = await api.delete(`/checklist/${selectedGuideId}/progress`);
		resetting = false;
		if (res.error) {
			itemError = res.error.message;
			return;
		}
		// Clear local completed/notes state (items remain, just unchecked).
		items = items.map((i) => ({ ...i, completed: false, notes: '' }));
		itemError = null;
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
			<div class="mt-2">
				<SelectField
					id="vc-origin"
					value={selectedOrigin}
					options={selector.origins.map((o) => ({ value: o.id, label: o.name }))}
					placeholder={t('checklist.select_origin', 'Select origin country')}
					onValueChange={(v) => (selectedOrigin = v)}
				/>
			</div>
		</div>

		<!-- Step 2: Destination -->
		{#if selectedOrigin}
			<div class="border-b border-[var(--color-border)] px-4 py-3">
				<label for="vc-dest" class="block text-xs font-medium uppercase tracking-wide text-[var(--color-muted)]">
					{t('checklist.step2', '2. Where are you going?')}
				</label>
				<div class="mt-2">
					<SelectField
						id="vc-dest"
						value={selectedDestination}
						options={destinations.map((d) => ({ value: d.id, label: d.name }))}
						placeholder={t('checklist.select_destination', 'Select destination country')}
						onValueChange={(v) => {
							selectedDestination = v;
							onSelectDestination();
						}}
					/>
				</div>
			</div>
		{/if}

		<!-- Step 3: Visa type -->
		{#if selectedDestination && visaTypes.length > 0}
			<div class="border-b border-[var(--color-border)] px-4 py-3">
				<label for="vc-visa" class="block text-xs font-medium uppercase tracking-wide text-[var(--color-muted)]">
					{t('checklist.step3', '3. Visa type')}
				</label>
				<div class="mt-2">
					<SelectField
						id="vc-visa"
						value={selectedVisaType}
						options={visaTypes.map((vt) => ({ value: vt.guide_id, label: vt.label }))}
						placeholder={t('checklist.select_visa', 'Select visa type')}
						onValueChange={(v) => {
							selectedVisaType = v;
							onSelectVisaType(v);
						}}
					/>
				</div>
			</div>
		{:else if selectedDestination && visaTypes.length === 0}
			<div class="px-4 py-3">
				<p class="text-sm text-[var(--color-muted)]">
					{t('checklist.no_visa_types', 'No visa guides published for this destination yet.')}
				</p>
				<!-- Request-a-guide CTA (§70: form_submission via §06, slug 'visa-guide-request') -->
				{#if requestSubmitted}
					<p class="mt-2 text-sm text-[var(--color-primary)]">
						{t('checklist.request_success', 'Your request has been submitted. Our team will prioritize this route.')}
					</p>
				{:else}
					<button
						type="button"
						disabled={!selectedOrigin || !selectedDestination || requestLoading}
						onclick={submitGuideRequest}
						class="mt-3 inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-1.5 text-sm text-[var(--color-text)] hover:border-[var(--color-primary)] hover:text-[var(--color-primary)] disabled:opacity-50"
					>
						<MessageSquarePlus size={16} />
						{requestLoading
							? t('checklist.request_submitting', 'Submitting…')
							: t('checklist.request_guide', 'Request a guide for this route')}
					</button>
					{#if requestError}
						<p class="mt-2 text-xs text-[var(--color-danger)]">{requestError}</p>
					{/if}
				{/if}
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
							<span class="flex items-center gap-3">
								<span>{completedCount}/{items.length} ({progressPct}%)</span>
								{#if authed && completedCount > 0}
									<button
										type="button"
										disabled={resetting}
										onclick={resetProgress}
										class="inline-flex items-center gap-1 text-[var(--color-muted)] hover:text-red-500 disabled:opacity-50"
									>
										<RotateCcw size={14} />
										{t('checklist.reset_progress', 'Reset progress')}
									</button>
								{/if}
							</span>
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
