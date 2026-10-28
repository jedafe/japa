<script lang="ts">
	import { t } from '$lib/i18n';
	import { page } from '$app/state';
	import { replaceState } from '$app/navigation';
	import { untrack } from 'svelte';
	import { ClipboardCheck, Check, Lock, RotateCcw, MessageSquarePlus, ArrowLeft } from 'lucide-svelte';
	import { EmptyState, LoadingSkeleton, SelectField } from '$lib/components/ui';
	import AuthDialog from '$lib/components/auth/AuthDialog.svelte';
	import { api } from '$lib/api/client';
	import { destinationMarkets, originMarkets, pinMarketsFirst } from '$lib/utils/geoMarkets';
	import type { ChecklistGuide, ChecklistItem, ChecklistSelector } from '$lib/types';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	const title = $derived(t('tools.visa_checklist', 'Visa Checklist — {site}'));
	const description = $derived(
		t('tools.visa_checklist_desc', 'Build a document checklist for your visa route.')
	);

	const selector = $derived<ChecklistSelector | null>(data.selector);
	const disabled = $derived(data.disabled);
	const selectorFailed = $derived(data.selectorFailed);
	const authed = $derived(data.authed);

	// Selector state — 3 steps.
	let selectedOrigin = $state('');
	let selectedDestination = $state('');
	let selectedVisaType = $state('');
	let selectedGuideId = $state('');
	let guideTitle = $state('');

	// Checklist state.
	let items = $state<ChecklistItem[]>([]);
	let loadingItems = $state(false);
	let itemError = $state<string | null>(null);
	let savingKey = $state<string | null>(null);
	let resetting = $state(false);

	// §70 conversion: anonymous tick-off prompts registration.
	let authPromptOpen = $state(false);

	// ADR-064 curated markets (public siteSettings) — pinned first; every
	// country stays selectable (pin-first, not restricted). Falls back to the
	// selector's Geo Spaces when /geo/countries is unavailable.
	const siteSettings = $derived(
		(data as { siteSettings?: Record<string, unknown> }).siteSettings
	);
	const originOptions = $derived(
		(data.countries ?? []).length > 0
			? pinMarketsFirst(data.countries, originMarkets(siteSettings), (o) => o.code ?? '')
			: (selector?.origins ?? [])
	);
	// Destinations only include countries with published guides; pin curated
	// destination markets by mapping guide country name → ISO geo_code.
	const destCodeByName = $derived(
		new Map((data.countries ?? []).map((c) => [c.name.toLowerCase(), c.code]))
	);
	const destinations = $derived(
		pinMarketsFirst(
			selector?.destinations ?? [],
			destinationMarkets(siteSettings),
			(d) => destCodeByName.get(d.name.toLowerCase()) ?? ''
		)
	);
	const visaTypes = $derived(
		destinations.find((d) => d.id === selectedDestination)?.visa_types ?? []
	);

	const completedCount = $derived(items.filter((i) => i.completed).length);
	const progressPct = $derived(items.length > 0 ? Math.round((completedCount / items.length) * 100) : 0);

	// Items grouped by the API-provided category (§70 contract); uncategorised
	// items fall back to "General".
	const groupedItems = $derived.by((): [string, ChecklistItem[]][] => {
		const groups = new Map<string, ChecklistItem[]>();
		for (const item of items) {
			const cat = item.category?.trim() || 'General';
			const list = groups.get(cat) ?? [];
			list.push(item);
			groups.set(cat, list);
		}
		return [...groups.entries()];
	});

	// Merge a progress payload into a list of guide items.
	function applyProgress(
		loaded: ChecklistItem[],
		progItems: { item_key: string; completed: boolean; notes?: string }[] | undefined
	): ChecklistItem[] {
		if (!progItems || progItems.length === 0) return loaded;
		const progMap = new Map(progItems.map((p) => [p.item_key, p]));
		return loaded.map((it) => {
			const p = progMap.get(it.item_key);
			return p ? { ...it, completed: p.completed, notes: p.notes ?? it.notes } : it;
		});
	}

	// Hydrate from a ?guide_id= deep link: server already fetched the guide
	// (and progress when signed in).
	$effect(() => {
		if (data.guide && selectedGuideId !== data.guide.guide_id) {
			untrack(() => {
				selectedGuideId = data.guide!.guide_id;
				guideTitle = data.guide!.title;
				items = applyProgress(data.guide!.items, data.progressItems);
			});
		}
	});

	function syncGuideParam(guideId: string | null) {
		const url = new URL(page.url);
		if (guideId) url.searchParams.set('guide_id', guideId);
		else url.searchParams.delete('guide_id');
		replaceState(url, {});
	}

	async function loadChecklist(guideId: string) {
		loadingItems = true;
		itemError = null;
		items = [];
		const res = await api.get<ChecklistGuide>(`/checklist/${guideId}`);
		loadingItems = false;
		if (res.error || !res.data) {
			itemError = res.error?.message ?? t('checklist.load_error', 'Could not load checklist');
			return;
		}
		guideTitle = res.data.title ?? '';
		let loaded = res.data.items ?? [];

		// Merge saved progress for authenticated users (§70: GET /progress).
		if (authed) {
			const progRes = await api.get<{
				items?: { item_key: string; completed: boolean; notes?: string }[];
			}>(`/checklist/${guideId}/progress`);
			if (!progRes.error) loaded = applyProgress(loaded, progRes.data?.items);
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

	function resetRequestState() {
		requestError = null;
		requestSubmitted = false;
	}

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
		const originName = originOptions.find((o) => o.id === selectedOrigin)?.name ?? selectedOrigin;
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

	function onSelectOrigin() {
		// Step 1 changed — every downstream selection/result is for the old
		// route and must be cleared (previously left the UK checklist visible
		// under a different origin).
		selectedDestination = '';
		selectedVisaType = '';
		selectedGuideId = '';
		guideTitle = '';
		items = [];
		itemError = null;
		resetRequestState();
		syncGuideParam(null);
	}

	function onSelectDestination() {
		selectedVisaType = '';
		selectedGuideId = '';
		guideTitle = '';
		items = [];
		itemError = null;
		resetRequestState();
		syncGuideParam(null);
	}

	async function onSelectVisaType(guideId: string) {
		selectedGuideId = guideId;
		syncGuideParam(guideId);
		await loadChecklist(guideId);
	}

	function changeSelection() {
		selectedVisaType = '';
		selectedGuideId = '';
		guideTitle = '';
		items = [];
		itemError = null;
		syncGuideParam(null);
	}

	async function toggleItem(item: ChecklistItem) {
		// §70: anonymous browse is allowed, but saving progress requires an
		// account — the first tick-off opens the registration prompt.
		if (!authed) {
			authPromptOpen = true;
			return;
		}
		if (!selectedGuideId || savingKey === item.item_key) return;
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
			// Rollback on failure and surface the reason.
			items = items.map((i) =>
				i.item_key === item.item_key ? { ...i, completed: prev } : i
			);
			itemError = res.error.message;
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
		} else {
			itemError = res.error.message;
		}
	}

	function onNoteBlur(item: ChecklistItem, e: Event) {
		const value = (e.target as HTMLTextAreaElement).value;
		if (value !== item.notes) saveNote(item, value);
	}

	// HowTo structured data from the loaded checklist (SEO parity with the
	// tools surface page — §70, ADR-070).
	const jsonLd = $derived(
		JSON.stringify({
			'@context': 'https://schema.org',
			'@type': 'HowTo',
			name: guideTitle || 'Visa Document Checklist',
			description:
				'Personalized, checkable visa document list with progress tracking.',
			step: items.slice(0, 10).map((it, i) => ({
				'@type': 'HowToStep',
				position: i + 1,
				name: it.label,
				text: it.description ?? it.label
			}))
		})
	);
</script>

<svelte:head>
	<title>{title}</title>
	<meta name="description" content={description} />
	<link rel="canonical" href="{page.url.origin}/tools/visa-checklist" />
	<meta property="og:title" content={title} />
	<meta property="og:description" content={description} />
	<meta property="og:type" content="website" />
	<meta property="og:url" content={page.url.href} />
	<!-- eslint-disable-next-line svelte/no-at-html-tags -->
	{@html `<script type="application/ld+json">${jsonLd}</script>`}
</svelte:head>

<div class="mx-auto w-full max-w-3xl">
	<div class="mx-4 border-b border-[var(--color-border)] px-4 py-3">
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
			{#if selectorFailed}
				<EmptyState
					icon={ClipboardCheck}
					title={t('checklist.selector_failed', 'Could not load checklist options')}
					body={t('checklist.selector_failed_body', 'Please try again later.')}
				/>
			{:else}
				<LoadingSkeleton variant="lines" />
			{/if}
		</div>
	{:else}
		<!-- Step 1: Origin -->
		<div class="border-b border-[var(--color-border)] px-4 py-3">
			<label for="vc-origin" class="block text-xs font-medium uppercase tracking-wide text-[var(--color-muted)]">
				{t('checklist.step1', '1. Where are you coming from?')}
			</label>
			<p class="mt-1 text-xs text-[var(--color-muted)]">{t('tools.checklist.origin_hint', 'Where you are applying from.')}</p>
			<div class="mt-2">
				<SelectField
					id="vc-origin"
					value={selectedOrigin}
					options={originOptions.map((o) => ({ value: o.id, label: o.name }))}
					placeholder={t('checklist.select_origin', 'Select origin country')}
					onValueChange={(v) => {
						selectedOrigin = v;
						onSelectOrigin();
					}}
				/>
			</div>
		</div>

		<!-- Step 2: Destination -->
		{#if selectedOrigin}
			<div class="border-b border-[var(--color-border)] px-4 py-3">
				<label for="vc-dest" class="block text-xs font-medium uppercase tracking-wide text-[var(--color-muted)]">
					{t('checklist.step2', '2. Where are you going?')}
				</label>
				<p class="mt-1 text-xs text-[var(--color-muted)]">{t('tools.checklist.dest_hint', 'Where you are applying to.')}</p>
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
				<p class="mt-1 text-xs text-[var(--color-muted)]">{t('tools.checklist.visa_hint', 'The visa type this checklist is for.')}</p>
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
			</div>
		{/if}

		<!-- Request-a-guide CTA (§70): shown whenever the user's combination has
		     no guide — no destinations published at all, none for the chosen
		     destination, or the visa type they need is unlisted. -->
		{#if selectedOrigin && (destinations.length === 0 || (selectedDestination && visaTypes.length === 0))}
			<div class="px-4 pb-3">
				{#if requestSubmitted}
					<p class="text-sm text-[var(--color-primary)]">
						{t('checklist.request_success', 'Your request has been submitted. Our team will prioritize this route.')}
					</p>
				{:else}
					<button
						type="button"
						disabled={!selectedOrigin || !selectedDestination || requestLoading}
						onclick={submitGuideRequest}
						class="inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-1.5 text-sm text-[var(--color-text)] hover:border-[var(--color-primary)] hover:text-[var(--color-primary)] disabled:opacity-50"
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
				{#if guideTitle}
					<div class="mb-3 flex items-start justify-between gap-3">
						<h2 class="text-base font-semibold text-[var(--color-text)]">{guideTitle}</h2>
						<button
							type="button"
							onclick={changeSelection}
							class="inline-flex shrink-0 items-center gap-1 text-xs text-[var(--color-muted)] hover:text-[var(--color-text)]"
						>
							<ArrowLeft class="rtl:-scale-x-100" size={14} />
							{t('checklist.change_selection', 'Change selection')}
						</button>
					</div>
				{/if}

				{#if !authed}
					<div class="mb-3 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface-subtle)] px-3 py-2 text-xs text-[var(--color-muted)]">
						{t('checklist.anon_hint', 'Sign in to save your progress and notes.')}
					</div>
				{/if}

				{#if loadingItems}
					<LoadingSkeleton variant="lines" />
				{:else if itemError && items.length === 0}
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

					{#if itemError}
						<p class="mb-3 text-xs text-[var(--color-danger)]">{itemError}</p>
					{/if}

					{#each groupedItems as [category, catItems] (category)}
						{#if groupedItems.length > 1 || category !== 'General'}
							<h3 class="mb-2 text-xs font-semibold uppercase tracking-wide text-[var(--color-muted)]">
								{category === 'General' ? t('checklist.general_category', 'General') : category}
							</h3>
						{/if}
						<ul class="mb-4 space-y-2 last:mb-0">
							{#each catItems as item (item.item_key)}
								<li
									class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-3"
								>
									<div class="flex items-start gap-3">
										<button
											type="button"
											disabled={savingKey === item.item_key}
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
											<span class="flex flex-wrap items-center gap-2">
												<span
													class="text-sm {item.completed
													? 'text-[var(--color-muted)] line-through'
													: 'text-[var(--color-text)]'}"
												>{item.label}</span>
												{#if item.required}
													<span class="rounded bg-red-100 px-1.5 py-0.5 text-xs font-medium text-red-700 dark:bg-red-900/30 dark:text-red-400">
														{t('checklist.required', 'Required')}
													</span>
												{/if}
											</span>
											{#if item.description}
												<span class="mt-1 block text-xs text-[var(--color-muted)]">{item.description}</span>
											{/if}
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
					{/each}
				{/if}
			</div>
		{/if}
	{/if}
</div>

<!-- §70: registration prompt on first anonymous tick-off (conversion). -->
<AuthDialog bind:open={authPromptOpen} initialMode="signup" redirect={page.url.pathname + page.url.search} />
