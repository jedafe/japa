<script lang="ts">
	import HintIcon from '$lib/components/ui/HintIcon.svelte';
	import { untrack, onMount, onDestroy } from 'svelte';
	import { api } from '$lib/api/client';
	import { t } from '$lib/i18n';
	import { page } from '$app/state';
	import { PenLine, Trash2 } from 'lucide-svelte';
	import DestinationSelector from '$lib/components/tools/DestinationSelector.svelte';
	import Autocomplete from '$lib/components/create/Autocomplete.svelte';
	import type { AutocompleteItem } from '$lib/components/create/Autocomplete.svelte';
	import type { GeoSpaceOption, SopTemplate, SopTemplateSection, SopSample, SopDraft, SopQuota } from '$lib/types';
	import type { PageData } from './$types';

	/**
	 * AI SOP Generator — §110, ADR-121.
	 * Public: template + sample browsing. Auth: quota display, generation
	 * form (async — 202 + draft_id, polled via GET /sop/drafts/:id), draft
	 * editor with version save.
	 *
	 * The destination selector is the first input and filters the template
	 * and sample lists via ?dest= (geo-aware pattern). Purpose filters
	 * templates; program level + field of study filter samples.
	 *
	 * Ref: japaconnect-engineering/sections/110-sop-generator.md
	 */
	let { data }: { data: PageData } = $props();

	let destinations = $state<GeoSpaceOption[]>(untrack(() => data.destinations));
	let templates = $state<SopTemplate[]>(untrack(() => data.templates));
	let samples = $state<SopSample[]>(untrack(() => data.samples));
	let quota = $state<SopQuota | null>(untrack(() => data.quota));
	let drafts = $state<SopDraft[]>(untrack(() => data.drafts));
	const authed = $derived(data.authed);

	// Generate form — destination doubles as the library filter axis.
	let genDest = $state('');
	let genPurpose = $state('');
	let genProgram = $state('');
	let genInstitution = $state('');
	let genBackground = $state('');
	let generating = $state(false);
	let genError = $state('');

	// Sample library filters (GET /sop/samples?level=&field=).
	let filterLevel = $state('');
	let filterField = $state('');
	// Field-of-study options come from the curated sop_fields list
	// (GET /sop/fields — admin/MCP-managed). Stray field values on returned
	// samples are still appended so an existing value never becomes
	// unfilterable.
	let knownFields = $state<string[]>(
		untrack(() => [
			...new Set([
				...data.fields.map((f) => f.name),
				...data.samples
					.map((s) => s.field_of_study)
					.filter((f): f is string => Boolean(f))
			])
		])
	);
	const fieldItems = $derived(knownFields.map((f) => ({ id: f, label: f })));

	// Active draft being edited
	let activeDraft = $state<SopDraft | null>(null);
	let editText = $state('');
	let editTitle = $state('');
	let savingDraft = $state(false);
	let draftMsg = $state('');

	// Poll timer for queued drafts
	let pollTimer: ReturnType<typeof setInterval> | null = null;
	let pollAttempts = 0;
	let pollErrors = 0;
	const POLL_MAX_ATTEMPTS = 40; // ~2 minutes at 3s
	const POLL_MAX_ERRORS = 3;

	const title = $derived(t('sop_generator.seo_title', 'AI SOP Generator — {site}'));
	const description = $derived(
		t('sop_generator.seo_description', 'Generate a tailored statement of purpose draft from proven templates — browse samples, refine, and export.')
	);

	// §110 purpose enum — labels translated, values fixed by the API contract.
	const PURPOSE_VALUES = ['study_permit', 'student_visa', 'work_visa', 'visitor_visa'] as const;
	const PURPOSE_LABELS: Record<string, string> = {
		study_permit: 'Study permit',
		student_visa: 'Student visa',
		work_visa: 'Work visa',
		visitor_visa: 'Visitor visa'
	};
	const purposeItems = $derived(
		PURPOSE_VALUES.map((v) => ({ id: v, label: t(`sop_generator.purpose_${v}`, PURPOSE_LABELS[v]) }))
	);

	// §110 program_level enum.
	const LEVEL_VALUES = ['diploma', 'bachelors', 'masters', 'mba', 'phd'] as const;
	const LEVEL_LABELS: Record<string, string> = {
		diploma: 'Diploma',
		bachelors: "Bachelor's",
		masters: "Master's",
		mba: 'MBA',
		phd: 'PhD'
	};
	const levelItems = $derived(
		LEVEL_VALUES.map((v) => ({ id: v, label: t(`sop_generator.level_${v}`, LEVEL_LABELS[v]) }))
	);

	const canGenerate = $derived(quota === null || quota.remaining > 0);

	function filterByQuery(items: AutocompleteItem[]) {
		return (q: string): Promise<AutocompleteItem[]> => {
			const needle = q.trim().toLowerCase();
			return Promise.resolve(needle ? items.filter((i) => i.label.toLowerCase().includes(needle)) : items);
		};
	}

	function selectedLabel(items: AutocompleteItem[], id: string): string | null {
		return items.find((i) => i.id === id)?.label ?? null;
	}

	// Normalize the template structure jsonb: spec shape is
	// [{section, prompt_hint, max_words}] but AI drafts and older rows may
	// carry {sections:[...]} or legacy keys (heading/guidance/min_words).
	function templateSections(tpl: SopTemplate): SopTemplateSection[] {
		const s = tpl.structure as { sections?: unknown[] } | unknown[] | null | undefined;
		const arr = Array.isArray(s) ? s : Array.isArray(s?.sections) ? s.sections : [];
		return arr
			.map((it) => {
				if (typeof it === 'string') return { section: it };
				const o = it as Record<string, unknown>;
				return {
					section: String(o.section ?? o.heading ?? o.name ?? ''),
					prompt_hint: (o.prompt_hint ?? o.guidance) as string | undefined,
					max_words: (o.max_words ?? o.min_words) as number | undefined
				};
			})
			.filter((it) => it.section);
	}

	function draftMeta(d: SopDraft): string {
		return [d.program, d.institution, d.status].filter(Boolean).join(' · ');
	}

	// Library fetches — destination + purpose from the generation form
	// drive template/sample filtering (§110 Geo Awareness).
	async function loadTemplates() {
		const params = new URLSearchParams();
		if (genDest) params.set('dest', genDest);
		if (genPurpose) params.set('purpose', genPurpose);
		const qs = params.toString();
		const res = await api.get<SopTemplate[]>(`/sop/templates${qs ? `?${qs}` : ''}`);
		if (!res.error && res.data) templates = res.data;
	}

	async function loadSamples() {
		const params = new URLSearchParams();
		if (genDest) params.set('dest', genDest);
		if (filterLevel) params.set('level', filterLevel);
		if (filterField) params.set('field', filterField);
		const qs = params.toString();
		const res = await api.get<SopSample[]>(`/sop/samples${qs ? `?${qs}` : ''}`);
		if (!res.error && res.data) {
			samples = res.data;
			for (const s of res.data) {
				if (s.field_of_study && !knownFields.includes(s.field_of_study)) {
					knownFields = [...knownFields, s.field_of_study];
				}
			}
		}
	}

	function applyFilters() {
		loadTemplates();
		loadSamples();
	}

	async function generate(e: Event) {
		e.preventDefault();
		if (!authed || !genPurpose) return;
		generating = true;
		genError = '';
		const res = await api.post<{ draft_id: string }>('/sop/generate', {
			destination_space_id: genDest || undefined,
			purpose: genPurpose,
			program: genProgram || undefined,
			institution: genInstitution || undefined,
			inputs: { background: genBackground }
		});
		generating = false;
		if (res.error) {
			genError = res.error.message;
			return;
		}
		if (res.data?.draft_id) {
			await refreshDrafts();
			const q = await api.get<SopQuota>('/sop/quota');
			if (!q.error && q.data) quota = q.data;
			startPolling(res.data.draft_id);
		}
	}

	async function refreshDrafts() {
		const res = await api.get<SopDraft[]>('/sop/drafts');
		if (!res.error && res.data) drafts = res.data;
	}

	function stopPolling() {
		if (pollTimer) {
			clearInterval(pollTimer);
			pollTimer = null;
		}
	}

	function startPolling(draftId: string) {
		stopPolling();
		pollAttempts = 0;
		pollErrors = 0;
		pollTimer = setInterval(async () => {
			pollAttempts++;
			const res = await api.get<SopDraft>(`/sop/drafts/${draftId}`);
			if (res.error || !res.data) {
				// Stop on repeated failures or after the overall cap — a dead
				// draft must not poll forever.
				if (++pollErrors >= POLL_MAX_ERRORS || pollAttempts >= POLL_MAX_ATTEMPTS) stopPolling();
				return;
			}
			pollErrors = 0;
			if (res.data.status !== 'queued') {
				stopPolling();
				await refreshDrafts();
				openDraft(res.data);
			} else if (pollAttempts >= POLL_MAX_ATTEMPTS) {
				stopPolling();
			}
		}, 3000);
	}

	function openDraft(d: SopDraft) {
		activeDraft = d;
		editText = d.generated_text ?? '';
		editTitle = d.title ?? '';
		draftMsg = '';
	}

	async function saveDraft() {
		if (!activeDraft) return;
		savingDraft = true;
		draftMsg = '';
		const res = await api.patch<SopDraft>(`/sop/drafts/${activeDraft.id}`, {
			title: editTitle || undefined,
			generated_text: editText || undefined
		});
		savingDraft = false;
		if (res.error) {
			draftMsg = res.error.message;
			return;
		}
		draftMsg = t('sop_generator.saved', 'Saved');
		// PATCH returns the full updated draft (status, version, timestamps).
		if (res.data?.status) {
			activeDraft = res.data;
		} else {
			await refreshDraft();
		}
		await refreshDrafts();
	}

	// Defensive refetch if a stale backend ever returns a partial PATCH body.
	async function refreshDraft() {
		if (!activeDraft) return;
		const res = await api.get<SopDraft>(`/sop/drafts/${activeDraft.id}`);
		if (!res.error && res.data) activeDraft = res.data;
	}

	async function deleteDraft(id: string) {
		if (!confirm(t('sop_generator.delete_confirm', 'Delete this draft? This cannot be undone.'))) return;
		const res = await api.delete(`/sop/drafts/${id}`);
		if (!res.error) {
			drafts = drafts.filter((d) => d.id !== id);
			if (activeDraft?.id === id) {
				activeDraft = null;
				editText = '';
				editTitle = '';
			}
		}
	}

	onMount(() => {
		const queued = drafts.find((d) => d.status === 'queued');
		if (queued) startPolling(queued.id);
	});

	onDestroy(() => stopPolling());

	const inputCls =
		'w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)]';
	const labelCls = 'mb-1 block text-sm font-medium text-[var(--color-text)]';
	const cardCls = 'rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)]';
	const sectionCls = 'mb-3 text-sm font-semibold uppercase tracking-wide text-[var(--color-muted)]';
	const filterLabelCls = 'mb-1 block text-xs font-medium text-[var(--color-muted)]';
</script>

<svelte:head>
	<title>{title}</title>
	<meta name="description" content={description} />
	<meta property="og:title" content={title} />
	<meta property="og:description" content={description} />
	<meta property="og:type" content="website" />
	<meta property="og:url" content={page.url.href} />
</svelte:head>

<div class="mx-auto w-full max-w-4xl px-4 py-6">
	<div class="mb-6">
		<h1 class="flex items-center gap-2 text-lg font-semibold text-[var(--color-text)]">
			<PenLine size={20} /> {t('sop_generator.title', 'AI SOP Generator')}
		</h1>
		<p class="mt-1 text-sm text-[var(--color-muted)]">
			{t('sop_generator.subtitle', 'Generate a tailored statement of purpose from proven templates — browse samples, refine, and export.')}
		</p>
	</div>

	<!-- Quota (auth) -->
	{#if authed && quota}
		<div class="mb-4 flex items-center justify-between rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-4 py-3 text-sm">
			<span class="text-[var(--color-muted)]">
				{t('sop_generator.quota', 'Monthly quota')}: {quota.used}/{quota.limit} {t('sop_generator.used', 'used')}
			</span>
			<span class="font-medium {quota.remaining > 0 ? 'text-green-600' : 'text-[var(--color-danger)]'}">
				{quota.remaining} {t('sop_generator.remaining', 'remaining')}
			</span>
		</div>
	{/if}

	<!-- Generation form -->
	<form onsubmit={generate} class="{cardCls} p-4">
		<h2 class="mb-4 text-base font-semibold text-[var(--color-text)]">{t('sop_generator.generate_title', 'Generate a draft')}</h2>
		<!-- Destination is the first input and also filters the template +
		     sample lists below (geo-aware pattern — §110). It renders its own
		     label. -->
		<div class="mb-4">
			<DestinationSelector {destinations} bind:selectedId={genDest} onchange={applyFilters} />
		</div>
		<div class="grid grid-cols-1 gap-4 sm:grid-cols-2">
			<div>
				<label class={labelCls} for="genPurpose">{t('sop_generator.purpose', 'Purpose')}
				<HintIcon text={t('tools.sop.purpose_hint', 'What the statement is for.')}  />
</label>
				<Autocomplete
					id="genPurpose"
					placeholder={t('sop_generator.purpose_placeholder', 'Select a purpose…')}
					emptyMessage={t('common.no_results', 'No results found')}
					fetcher={filterByQuery(purposeItems)}
					initialFetcher={() => Promise.resolve(purposeItems)}
					selectedId={genPurpose || null}
					selectedLabel={selectedLabel(purposeItems, genPurpose)}
					onSelect={(item) => {
						genPurpose = item.id;
						applyFilters();
					}}
					onClear={() => {
						genPurpose = '';
						applyFilters();
					}}
				/>
			</div>
			<div>
				<label class={labelCls} for="genProgram">{t('sop_generator.program', 'Program')}
				<HintIcon text={t('tools.sop.program_hint', 'The program you are applying to.')}  />
</label>
				<input id="genProgram" type="text" bind:value={genProgram} placeholder="e.g. MSc Computer Science" class={inputCls} />
			</div>
			<div>
				<label class={labelCls} for="genInstitution">{t('sop_generator.institution', 'Institution')}
				<HintIcon text={t('tools.sop.institution_hint', 'The school or institution name.')}  />
</label>
				<input id="genInstitution" type="text" bind:value={genInstitution} placeholder="e.g. University of Toronto" class={inputCls} />
			</div>
		</div>
		<div class="mt-4">
			<label class={labelCls} for="genBackground">{t('sop_generator.background', 'Brief background')}
			<HintIcon text={t('tools.sop.background_hint', 'Summarize your background — the generator expands it.')}  />
</label>
			<textarea
				id="genBackground"
				bind:value={genBackground}
				rows="3"
				placeholder={t('sop_generator.background_placeholder', 'e.g. 5 years software engineering, BSc from UNILAG, first-class honours…')}
				class={inputCls}
			></textarea>
		</div>
		<div class="mt-4">
			<button
				type="submit"
				disabled={generating || !canGenerate || !authed || !genPurpose}
				class="inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-white hover:opacity-90 disabled:opacity-50"
			>
				{generating ? t('sop_generator.generating', 'Generating…') : t('sop_generator.generate', 'Generate draft')}
			</button>
			{#if !authed}
				<span class="ms-3 text-xs text-[var(--color-muted)]">
					{t('sop_generator.signin_required', 'Sign in to generate')}
				</span>
			{:else if !canGenerate}
				<span class="ms-3 text-xs text-[var(--color-danger)]">
					{t('sop_generator.quota_exhausted', 'Monthly quota exhausted')}
				</span>
			{/if}
		</div>
	</form>

	{#if genError}
		<div class="mt-4 rounded-[var(--radius)] border border-[var(--color-danger)]/30 bg-[var(--color-danger)]/10 p-3 text-sm text-[var(--color-danger)]">
			{genError}
		</div>
	{/if}

	<!-- Draft editor -->
	{#if activeDraft}
		<div class="mt-4 {cardCls} p-4">
			<div class="mb-3 flex items-center justify-between">
				<h2 class="text-base font-semibold text-[var(--color-text)]">{t('sop_generator.edit_draft', 'Edit draft')}</h2>
				<div class="flex items-center gap-2 text-xs text-[var(--color-muted)]">
					<span>v{activeDraft.version}</span>
					<span>·</span>
					<span>{activeDraft.status}</span>
					{#if activeDraft.ai_model}
						<span>· {activeDraft.ai_model}</span>
					{/if}
				</div>
			</div>
			{#if activeDraft.status === 'failed'}
				<div class="mb-3 rounded-[var(--radius)] border border-[var(--color-danger)]/30 bg-[var(--color-danger)]/10 p-3 text-sm text-[var(--color-danger)]">
					{t('sop_generator.draft_failed', 'Generation failed — you can edit or delete this draft, or generate a new one.')}
				</div>
			{:else if activeDraft.status === 'queued'}
				<div class="mb-3 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface-2,var(--color-surface))] p-3 text-sm text-[var(--color-muted)]">
					{t('sop_generator.draft_queued', 'Generation in progress — this draft updates automatically.')}
				</div>
			{/if}
			<input type="text" bind:value={editTitle} placeholder={t('sop_generator.draft_title', 'Draft title')} class="{inputCls} mb-3" />
			<textarea bind:value={editText} rows="16" class="{inputCls} font-mono text-sm"></textarea>
			<div class="mt-3 flex items-center gap-3">
				<button
					class="inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-white hover:opacity-90 disabled:opacity-50"
					onclick={saveDraft}
					disabled={savingDraft}
				>
					{savingDraft ? t('sop_generator.saving', 'Saving…') : t('sop_generator.save', 'Save draft')}
				</button>
				<button
					class="inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-1.5 text-sm text-[var(--color-danger)]"
					onclick={() => activeDraft && deleteDraft(activeDraft.id)}
				>
					{t('sop_generator.delete', 'Delete')}
				</button>
				{#if draftMsg}
					<span class="text-xs text-green-600">{draftMsg}</span>
				{/if}
			</div>
		</div>
	{/if}

	<!-- Draft list (auth) -->
	{#if authed && drafts.length > 0}
		<div class="mt-6">
			<h2 class={sectionCls}>{t('sop_generator.my_drafts', 'My drafts')}</h2>
			<div class="space-y-2">
				{#each drafts as draft (draft.id)}
					<div class="flex items-center justify-between rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-4 py-3">
						<button class="flex-1 text-start" onclick={() => openDraft(draft)}>
							<div class="text-sm font-medium text-[var(--color-text)]">{draft.title || draft.purpose}</div>
							<div class="mt-0.5 text-xs text-[var(--color-muted)]">
								{draftMeta(draft)}
							</div>
						</button>
						<button class="ms-3 text-[var(--color-muted)] hover:text-[var(--color-danger)]" onclick={() => deleteDraft(draft.id)} aria-label={t('sop_generator.delete_draft', 'Delete draft')}>
							<Trash2 size={14} />
						</button>
					</div>
				{/each}
			</div>
		</div>
	{/if}

	<!-- Templates (public, filtered by destination + purpose) -->
	<div class="mt-6">
		<h2 class={sectionCls}>{t('sop_generator.templates', 'Templates')}</h2>
		{#if templates.length > 0}
			<div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
				{#each templates as tpl (tpl.id)}
					{@const sections = templateSections(tpl)}
					<div class="{cardCls} p-3">
						<div class="text-sm font-medium text-[var(--color-text)]">{tpl.title}</div>
						<div class="mt-0.5 text-xs text-[var(--color-muted)]">{selectedLabel(purposeItems, tpl.purpose) ?? tpl.purpose}</div>
						{#if sections.length}
							<div class="mt-2 space-y-1">
								{#each sections as section (section.section)}
									<div class="text-xs text-[var(--color-text)]">
										<span class="font-medium">{section.section}</span>
										{#if section.max_words}
											<span class="text-[var(--color-muted)]">· ~{section.max_words}w</span>
										{/if}
									</div>
								{/each}
							</div>
						{/if}
					</div>
				{/each}
			</div>
		{:else}
			<p class="text-xs text-[var(--color-muted)]">{t('sop_generator.no_templates', 'No templates match the selected destination and purpose.')}</p>
		{/if}
	</div>

	<!-- Samples (public, filtered by destination + level + field) -->
	<div class="mt-6">
		<h2 class={sectionCls}>{t('sop_generator.samples', 'Sample SOPs')}</h2>
		<div class="mb-3 grid grid-cols-1 gap-3 sm:grid-cols-2">
			<div>
				<label class={filterLabelCls} for="sampleLevel">{t('sop_generator.filter_level', 'Program level')}</label>
				<Autocomplete
					id="sampleLevel"
					placeholder={t('sop_generator.all_levels', 'All levels')}
					emptyMessage={t('common.no_results', 'No results found')}
					fetcher={filterByQuery(levelItems)}
					initialFetcher={() => Promise.resolve(levelItems)}
					selectedId={filterLevel || null}
					selectedLabel={selectedLabel(levelItems, filterLevel)}
					onSelect={(item) => {
						filterLevel = item.id;
						loadSamples();
					}}
					onClear={() => {
						filterLevel = '';
						loadSamples();
					}}
					openUp
				/>
			</div>
			<div>
				<label class={filterLabelCls} for="sampleField">{t('sop_generator.filter_field', 'Field of study')}</label>
				<Autocomplete
					id="sampleField"
					placeholder={t('sop_generator.all_fields', 'All fields')}
					emptyMessage={t('common.no_results', 'No results found')}
					fetcher={filterByQuery(fieldItems)}
					initialFetcher={() => Promise.resolve(fieldItems)}
					selectedId={filterField || null}
					selectedLabel={selectedLabel(fieldItems, filterField)}
					onSelect={(item) => {
						filterField = item.id;
						loadSamples();
					}}
					onClear={() => {
						filterField = '';
						loadSamples();
					}}
					openUp
				/>
			</div>
		</div>
		{#if samples.length > 0}
			<div class="space-y-3">
				{#each samples as sample (sample.id)}
					<details class="rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)]">
						<summary class="cursor-pointer px-4 py-3 text-sm font-medium text-[var(--color-text)]">
							{sample.title}
							{#if sample.destination_space_name}
								<span class="text-xs font-normal text-[var(--color-muted)]">· {sample.destination_space_name}</span>
							{/if}
							{#if sample.word_count}
								<span class="text-xs font-normal text-[var(--color-muted)]">· {sample.word_count}w</span>
							{/if}
						</summary>
						<div class="whitespace-pre-wrap border-t border-[var(--color-border)] px-4 py-3 text-sm text-[var(--color-text)]">
							{sample.body}
						</div>
					</details>
				{/each}
			</div>
		{:else}
			<p class="text-xs text-[var(--color-muted)]">{t('sop_generator.no_samples', 'No samples match the selected filters.')}</p>
		{/if}
	</div>
</div>
