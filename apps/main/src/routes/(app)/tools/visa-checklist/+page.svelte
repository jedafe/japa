<script lang="ts">
	import SeoHead from '#lib/seo/SeoHead.svelte';
	import { toolOgCard } from '#lib/seo/og.js';
	import { t } from '#lib/i18n/index.js';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { untrack } from 'svelte';
	import { ClipboardCheck, Check, Lock, RotateCcw, MessageSquarePlus, ArrowLeft, Route } from 'lucide-svelte';
	import { EmptyState, LoadingSkeleton, SelectField } from '#lib/components/ui/index.js';
	import AuthDialog from '#lib/components/auth/AuthDialog.svelte';
	import { api } from '#lib/api/client.js';
	import { destinationMarkets, originMarkets, pinMarketsFirst } from '#lib/utils/geoMarkets.js';
	import type { ChecklistGuide, ChecklistItem, ChecklistSelector } from '#lib/types/index.js';
	import type { PageData } from './$types';
	import ToolHeader from '#lib/components/tools/ToolHeader.svelte';

	let { data }: { data: PageData } = $props();

	const title = $derived(t('tools.visa_checklist', 'Visa Checklist — {site}'));
	const description = $derived(
		t('tools.visa_checklist_desc', 'Build a document checklist for your visa route.')
	);

	const selector = $derived<ChecklistSelector | null>(data.selector);
	const disabled = $derived(data.disabled);
	const selectorFailed = $derived(data.selectorFailed);
	const authed = $derived(data.authed);
	const sourceRoute = $derived(data.sourceRoute);

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
		: selector?.origins ?? []);

	// Destinations only include countries with published guides; pin curated
	// destination markets by mapping guide country name → ISO geo_code.
	const destCodeByName = $derived(new Map((data.countries ?? []).map((c) => [c.name.toLowerCase(), c.code])));

	const destinations = $derived(pinMarketsFirst(selector?.destinations ?? [], destinationMarkets(siteSettings), (d) => destCodeByName.get(d.name.toLowerCase()) ?? ''));
	const visaTypes = $derived(destinations.find((d) => d.id === selectedDestination)?.visa_types ?? []);
	// Stale-link notice — SSR flags a ?guide_id= that didn't resolve, but
	// the flag must track the live URL: a shallow goto() drops the param
	// on selection change and must not resurface the notice.
	const guideNoticeVisible = $derived(
		data.guideUnavailable && page.url.searchParams.has('guide_id') && !selectedGuideId
	);
	// Transient fetch failure — same URL-tracking as the stale notice but
	// a different message: the guide may be fine, the load just failed.
	const guideFailureVisible = $derived(
		data.guideFailed && page.url.searchParams.has('guide_id') && !selectedGuideId
	);
	// Request-a-guide CTA: visible whenever the picked combination has no
	// guide — an empty destination corpus, a destination with no visa types,
	// or a needed visa type that isn't listed (destination picked, no guide
	// loaded). Once a guide renders, the request form is moot.
	const showRequestCta = $derived(
		!!selectedOrigin &&
			(destinations.length === 0 || (selectedDestination !== '' && selectedGuideId === ''))
	);
	const completedCount = $derived(items.filter((i) => i.completed).length);
	const progressPct = $derived(items.length > 0 ? Math.round(completedCount / items.length * 100) : 0);

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
	// (and progress when signed in). Track which guide_id was hydrated — NOT
	// selectedGuideId — so clearing the selection never re-fires hydration
	// (previously caused a snap-back that trapped users on deep links).
	let hydratedGuideId = $state<string | null>(null);

	$effect(() => {
		const g = data.guide;
		if (!g) {
			hydratedGuideId = null;
			return;
		}
		if (g.guide_id === hydratedGuideId) return;
		hydratedGuideId = g.guide_id;
		untrack(() => {
			selectedGuideId = g.guide_id;
			guideTitle = g.title;
			items = applyProgress(g.items, data.progressItems);
			if (data.progressFailed) {
				itemError = t(
					'checklist.progress_unavailable',
					'Could not load your saved progress — items show unticked.'
				);
			}
			// Backfill the 3-step selector (destination + visa type) by
			// scanning the selector corpus for this guide_id, so a
			// deep-linked user sees — and can change — the route that
			// produced the checklist. Origin stays empty: the guide can't
			// know it, and picking one re-filters items.
			backfillSelector(g.guide_id);
		});
	});

	// Resolve a guide's destination option id + visa type value so the
	// selector selects render filled on deep links.
	function backfillSelector(guideId: string) {
		for (const d of selector?.destinations ?? []) {
			const vt = (d.visa_types ?? []).find((v) => v.guide_id === guideId);
			if (vt) {
				selectedDestination = d.id;
				selectedVisaType = vt.value;
				return;
			}
		}
		// Guide not in selector corpus (unpublished since the link was
		// shared, or field edited) — nothing to backfill; the checklist
		// itself is still rendered.
	}

	// §90 integration: a route_id with no resolvable guide_id still
	// backfills the selector — destination by name, visa type by category —
	// so the user lands one step from the checklist (or the request form).
	$effect(() => {
		const sr = data.sourceRoute;
		if (!sr?.destination || data.guide) return;
		untrack(() => {
			if (selectedDestination) return;
			const d = destinations.find(
				(x) => x.name.toLowerCase() === sr.destination!.toLowerCase()
			);
			if (d) {
				selectedDestination = d.id;
				if (sr.visaCategory) {
					const vt = (d.visa_types ?? []).find(
						(v) => v.value.toLowerCase() === sr.visaCategory!.toLowerCase()
					);
					if (vt) {
						selectedVisaType = vt.value;
						void onSelectVisaType();
					}
				}
			}
		});
	});

	function syncGuideParam(guideId: string | null) {
		const url = new URL(page.url.href);

		if (guideId) url.searchParams.set('guide_id', guideId); else url.searchParams.delete('guide_id');
		if (!guideId) url.searchParams.delete('route_id');

		goto(url, { shallow: true, replace: true });
	}

	// Origin filter params shared by every checklist API call — ADR-070 §2
	// narrows the item set to origin-specific requirements. Slug + ISO code
	// are both sent; the backend matches either against per-item origins.
	function originQuery(): string {
		const o = originOptions.find((x) => x.id === selectedOrigin);
		if (!o) return '';
		const code = 'code' in o ? (o as { code?: string }).code ?? '' : '';
		let qs = `?origin=${encodeURIComponent(o.slug)}`;
		if (code) qs += `&origin_code=${encodeURIComponent(code)}`;
		return qs;
	}

	// Stale-response guard: rapid selection/origin changes can overlap
	// checklist fetches — only the latest request may write results.
	let loadSeq = 0;

	async function loadChecklist(guideId: string) {
		const seq = ++loadSeq;
		loadingItems = true;
		itemError = null;
		items = [];
		const oq = originQuery();
		const res = await api.get<ChecklistGuide>(`/checklist/${guideId}${oq}`);
		if (seq !== loadSeq) return;
		loadingItems = false;
		if (res.error || !res.data) {
			itemError = res.error?.message ?? t('checklist.load_error', 'Could not load checklist');
			return;
		}
		guideTitle = res.data.title ?? '';
		let loaded = res.data.items ?? [];

		// Merge saved progress for authenticated users (§70: GET /progress).
		// A failed progress load must surface — silently rendering everything
		// unticked looks like lost data.
		if (authed) {
			const progRes = await api.get<{
				items?: { item_key: string; completed: boolean; notes?: string }[];
			}>(`/checklist/${guideId}/progress${oq}`);
			if (seq !== loadSeq) return;
			if (!progRes.error) {
				loaded = applyProgress(loaded, progRes.data?.items);
			} else {
				itemError = progRes.error.message;
			}
		}
		items = loaded;
	}

	/** Reset all progress for the current guide (§70: DELETE /progress). */
	async function resetProgress() {
		if (!authed || !selectedGuideId || resetting) return;
		if (!confirm(t('checklist.reset_confirm', 'Reset all progress for this checklist? This cannot be undone.'))) return;
		resetting = true;
		const res = await api.delete(`/checklist/${selectedGuideId}/progress${originQuery()}`);
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
	// The request UI is rendered from the live embed form definition — the
	// visa_type select options come from the seeded form schema (Rule 16, no
	// hardcoded option lists) and destination is a free-text field so the
	// request still works when no destination is published yet.
	interface RequestFormField {
		key: string;
		label: string;
		type: string;
		required?: boolean;
		option_defs?: { value: string; label: string }[];
		options?: string[];
		placeholder?: string | null;
	}
	interface RequestFormDef {
		id: string;
		submit_label?: string;
		fields?: RequestFormField[];
	}

	let requestForm = $state<RequestFormDef | null>(null);
	let requestLoading = $state(false);
	let requestError = $state<string | null>(null);
	let requestSubmitted = $state(false);
	let requestDestination = $state('');
	let requestVisaType = $state('other');
	let requestNotes = $state('');

	function resetRequestState() {
		requestError = null;
		requestSubmitted = false;
	}

	async function ensureRequestForm(): Promise<RequestFormDef | null> {
		if (requestForm) return requestForm;
		const res = await api.get<RequestFormDef>(`/forms/visa-guide-request/embed`);
		if (res.error || !res.data?.id) return null;
		requestForm = res.data;
		return requestForm;
	}

	// Preload the form definition the moment the CTA becomes visible so the
	// fields render instantly when the user expands it.
	$effect(() => {
		if (showRequestCta && !requestForm) void ensureRequestForm();
	});

	const requestVisaTypeOptions = $derived.by(() => {
		const f = requestForm?.fields?.find((x) => x.key === 'visa_type');
		return f?.option_defs ?? [];
	});

	// Sync the requested destination with the selector when one is picked —
	// free text otherwise (covers the empty-corpus case).
	$effect(() => {
		const name = destinations.find((d) => d.id === selectedDestination)?.name;
		if (name) requestDestination = name;
	});

	async function submitGuideRequest() {
		if (!selectedOrigin || !requestDestination.trim() || requestLoading) return;
		requestLoading = true;
		requestError = null;
		requestSubmitted = false;
		const form = await ensureRequestForm();
		if (!form) {
			requestError = t('checklist.request_unavailable', 'Request form unavailable. Please try again later.');
			requestLoading = false;
			return;
		}
		const originName = originOptions.find((o) => o.id === selectedOrigin)?.name ?? selectedOrigin;
		const res = await api.post(`/forms/${form.id}/submissions`, {
			field_values: {
				origin_country: originName,
				destination_country: requestDestination.trim(),
				visa_type: requestVisaType || 'other',
				notes: requestNotes.trim()
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
		// Origin narrows the displayed item set to origin-specific
		// requirements (ADR-070 §2). With a guide loaded, refetch its items
		// under the new origin — the guide itself is destination+type-scoped
		// and stays selected. Before a guide is picked, downstream selects
		// are origin-independent and stay as-is.
		resetRequestState();
		if (selectedGuideId) void loadChecklist(selectedGuideId);
	}

	function onSelectDestination() {
		loadSeq++; // discard any in-flight checklist fetch
		selectedVisaType = '';
		selectedGuideId = '';
		guideTitle = '';
		items = [];
		itemError = null;
		resetRequestState();
		syncGuideParam(null);
	}

	async function onSelectVisaType() {
		// The select binds the visa-type value; resolve it back to the
		// guide_id (selectedVisaType storing a guide_id conflated the two).
		const vt = visaTypes.find((v) => v.value === selectedVisaType);
		if (!vt) return;
		selectedGuideId = vt.guide_id;
		syncGuideParam(vt.guide_id);
		await loadChecklist(vt.guide_id);
	}

	function changeSelection() {
		loadSeq++; // discard any in-flight checklist fetch
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
			`/checklist/${selectedGuideId}/progress/${item.item_key}${originQuery()}`,
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
			`/checklist/${selectedGuideId}/progress/${item.item_key}${originQuery()}`,
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

<!-- Faceted canonical: ?guide_id= names a distinct checklist (indexable),
     ?route_id= only carries back-context — collapse it to the guide URL. -->
<SeoHead title={title} description={description} ogTitle={title} jsonLd={jsonLd}
	canonical={page.url.origin + '/tools/visa-checklist' + (page.url.searchParams.get('guide_id') ? `?guide_id=${encodeURIComponent(page.url.searchParams.get('guide_id')!)}` : '')}
	ogImage={toolOgCard(page.url.origin, page.url.pathname)}
/>

<div class="mx-auto w-full max-w-3xl">
	<div class="px-4 pt-4">
		<ToolHeader icon={ClipboardCheck} title={t('tools.visa_checklist', 'Visa Checklist')} description={description} />
	</div>

	{#if sourceRoute}
		<!-- §90 integration: user arrived from a specific comparison route —
		     give them a way back to exactly that context. -->
		<div class="px-4 pt-2">
			<a
				href={`/tools/visa-routes/${encodeURIComponent(sourceRoute.id)}`}
				class="inline-flex items-center gap-1.5 rounded-full border border-[var(--color-border)] bg-[var(--color-surface-subtle)] px-3 py-1 text-xs text-[var(--color-text)] hover:border-[var(--color-primary)]"
			>
				<ArrowLeft class="rtl:-scale-x-100" size={12} />
				{sourceRoute.title
					? t('checklist.back_to_route', 'Back to {route}').replace('{route}', sourceRoute.title)
					: t('checklist.back_to_comparison', 'Back to route comparison')}
			</a>
		</div>
	{/if}

	{#if !disabled}
		<!-- §90 reverse link: the comparison tool and this checklist are
		     two halves of the same flow — once a destination is picked,
		     deep-link straight into its filtered route list. -->
		<div class="px-4 pt-2">
			<a
				href={selectedDestination ? `/tools/visa-routes?dest=${encodeURIComponent(selectedDestination)}` : '/tools/visa-routes'}
				class="inline-flex items-center gap-1.5 text-sm text-[var(--color-primary-text)] hover:underline"
			>
				<Route size={14} />
				{t('checklist.compare_routes', 'Compare visa routes')}
			</a>
		</div>
	{/if}

	{#if guideNoticeVisible}
		<div class="px-4 pt-3">
			<div class="rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface-subtle)] px-3 py-2 text-sm text-[var(--color-muted)]">
				{t('checklist.guide_unavailable', 'The linked checklist is no longer available — pick your route below.')}
			</div>
		</div>
	{/if}

	{#if guideFailureVisible}
		<div class="px-4 pt-3">
			<div class="rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface-subtle)] px-3 py-2 text-sm text-[var(--color-muted)]">
				{t('checklist.guide_failed', 'Could not load the linked checklist — please try again.')}
			</div>
		</div>
	{/if}

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
						options={visaTypes.map((vt) => ({ value: vt.value, label: vt.label }))}
						placeholder={t('checklist.select_visa', 'Select visa type')}
						onValueChange={(v) => {
							selectedVisaType = v;
							onSelectVisaType();
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

		<!-- Request-a-guide (§70): visible whenever the picked combination has
		     no guide — no destinations published, none for the destination,
		     or the needed visa type isn't listed. Fields are rendered from
		     the live form schema; destination is free-text so the request
		     works even with an empty destination corpus. -->
		{#if showRequestCta}
			<div class="px-4 pb-3">
				{#if requestSubmitted}
					<p class="text-sm text-[var(--color-primary-text)]">
						{t('checklist.request_success', 'Your request has been submitted. Our team will prioritize this route.')}
					</p>
				{:else}
					<div class="rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface-subtle)] p-3">
						<p class="mb-2 text-sm font-medium text-[var(--color-text)]">
							{t('checklist.request_heading', 'No guide for this route yet — request one')}
						</p>
						<div class="mb-2">
							<label for="vc-req-dest" class="sr-only">{t('checklist.request_destination', 'Destination country')}</label>
							<input
								id="vc-req-dest"
								type="text"
								bind:value={requestDestination}
								placeholder={t('checklist.request_destination_ph', 'Destination country')}
								class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1.5 text-sm text-[var(--color-text)] focus:outline-none focus:ring-1 focus:ring-[var(--color-primary)]"
							/>
						</div>
						{#if requestVisaTypeOptions.length > 0}
							<div class="mb-2">
								<SelectField
									id="vc-req-type"
									value={requestVisaType}
									options={requestVisaTypeOptions.map((o) => ({ value: o.value, label: o.label }))}
									placeholder={t('checklist.request_visa_type', 'Visa type')}
									onValueChange={(v) => (requestVisaType = v)}
								/>
							</div>
						{/if}
						<textarea
							rows="2"
							bind:value={requestNotes}
							placeholder={t('checklist.request_notes_ph', 'Anything else we should know? (optional)')}
							class="mb-2 w-full resize-y rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1.5 text-sm text-[var(--color-text)] focus:outline-none focus:ring-1 focus:ring-[var(--color-primary)]"
						></textarea>
						<button
							type="button"
							disabled={!selectedOrigin || !requestDestination.trim() || requestLoading}
							onclick={submitGuideRequest}
							class="inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-1.5 text-sm text-[var(--color-text)] hover:border-[var(--color-primary)] hover:text-[var(--color-primary-text)] disabled:opacity-50"
						>
							<MessageSquarePlus size={16} />
							{requestLoading
								? t('checklist.request_submitting', 'Submitting…')
								: t('checklist.request_guide', 'Request a guide for this route')}
						</button>
						{#if requestError}
							<p class="mt-2 text-xs text-[var(--color-danger)]">{requestError}</p>
						{/if}
					</div>
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
										class="inline-flex items-center gap-1 text-[var(--color-muted)] hover:text-[var(--color-danger)] disabled:opacity-50"
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
													<span class="rounded bg-[var(--color-danger)]/10 px-1.5 py-0.5 text-xs font-medium text-[var(--color-danger)]">
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
