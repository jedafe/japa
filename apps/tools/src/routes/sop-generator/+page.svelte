<script lang="ts">
	import { untrack, onMount, onDestroy } from 'svelte';
	import { api } from '$lib/api/client';
	import { buildSeoMeta } from '$lib/seo';
	import SeoHead from '$lib/components/seo/SeoHead.svelte';
	import { t } from '$lib/i18n';
	import { page } from '$app/state';
	import Icon from '$lib/components/ui/Icon.svelte';
	import RegistrationPrompt from '$lib/components/ui/RegistrationPrompt.svelte';
	import ToolDisabled from '$lib/components/ui/ToolDisabled.svelte';
	import DestinationSelector from '$lib/components/tools/DestinationSelector.svelte';
	import Autocomplete from '$lib/components/ui/Autocomplete.svelte';
	import type { AutocompleteItem } from '$lib/components/ui/Autocomplete.svelte';
	import type {
		GeoSpaceOption,
		SopTemplate,
		SopTemplateSection,
		SopSample,
		SopField,
		SopDraft,
		SopQuota
	} from '$lib/types';

	/**
	 * AI SOP Generator — §110, ADR-121.
	 * Public: template + sample browsing. Auth: quota display, generation
	 * form (async — 202 + draft_id, polled via GET /sop/drafts/:id), draft
	 * editor with version save.
	 *
	 * The destination selector is the first input and filters the template
	 * and sample lists via ?dest= (geo-aware pattern). Purpose filters
	 * templates; program level + field of study filter samples.
	 */
	let { data } = $props();

	let destinations = $state<GeoSpaceOption[]>(untrack(() => data.destinations));
	let templates = $state<SopTemplate[]>(untrack(() => data.templates));
	let samples = $state<SopSample[]>(untrack(() => data.samples));
	let quota = $state<SopQuota | null>(untrack(() => data.quota));
	let drafts = $state<SopDraft[]>(untrack(() => data.drafts));
	let user = $state(untrack(() => data.user));

	// Generate form — destination doubles as the library filter axis.
	let genDest = $state('');
	let genPurpose = $state('');
	let genProgram = $state('');
	let genInstitution = $state('');
	let genBackground = $state('');
	let generating = $state(false);
	let genError = $state('');
	let showRegisterPrompt = $state(false);

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
				...data.fields.map((f: SopField) => f.name),
				...data.samples
					.map((s: SopSample) => s.field_of_study)
					.filter((f: string | undefined): f is string => Boolean(f))
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

	const seo = $derived(
		buildSeoMeta({
			title: t('sop_generator.seo_title', 'AI SOP Generator — {site} Tools'),
			description: t(
				'sop_generator.seo_description',
				'Generate a tailored statement of purpose draft from proven templates — browse samples, refine, and export.'
			),
			path: '/sop-generator',
			origin: page.url.origin,
			ogImage: `${page.url.origin}/og/og-sop-generator.png`,
			jsonLd: {
				'@context': 'https://schema.org',
				'@graph': [
					{
						'@type': 'SoftwareApplication',
						name: 'AI SOP Generator',
						applicationCategory: 'UtilitiesApplication',
						operatingSystem: 'Web',
						offers: { '@type': 'Offer', price: '0', priceCurrency: 'USD' },
						description:
							'Generate a tailored statement of purpose draft for visa and study applications.'
					},
					{
						'@type': 'HowTo',
						name: 'How to write a statement of purpose',
						description:
							'Generate a tailored statement of purpose draft from proven templates — browse samples, refine, and export.',
						step: templates.slice(0, 10).map((tpl, i) => ({
							'@type': 'HowToStep',
							position: i + 1,
							name: tpl.title ?? `Template ${i + 1}`,
							text: tpl.title ?? ''
						}))
					}
				]
			}
		})
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
		if (!user) {
			showRegisterPrompt = true;
			return;
		}
		if (!genPurpose) return;
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
			// Refresh drafts + quota, then poll the new draft.
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
		// Resume polling for any draft still queued.
		const queued = drafts.find((d) => d.status === 'queued');
		if (queued) startPolling(queued.id);
	});

	onDestroy(() => stopPolling());
</script>

<SeoHead seo={seo} ogType="website" twitterCard="summary" />

{#if !data.enabled}
	<ToolDisabled tool={data.tool} />
{:else}
	<div class="mx-auto max-w-4xl px-4 py-8">
		<div class="mb-8">
			<h1 class="text-3xl font-bold tracking-tight">
				{t('sop_generator.title', 'AI SOP Generator')}
			</h1>
			<p class="mt-2 text-muted">
				{t(
					'sop_generator.subtitle',
					'Generate a tailored statement of purpose from proven templates — browse samples, refine, and export.'
				)}
			</p>
		</div>

		<!-- Quota (auth) -->
		{#if user && quota}
			<div class="mb-6 flex items-center justify-between rounded-lg border border-border bg-surface px-4 py-3 text-sm">
				<span class="text-muted">
					{t('sop_generator.quota', 'Monthly quota')}: {quota.used}/{quota.limit} {t('sop_generator.used', 'used')}
				</span>
				<span class="font-medium {quota.remaining > 0 ? 'text-green-600' : 'text-red-600'}">
					{quota.remaining} {t('sop_generator.remaining', 'remaining')}
				</span>
			</div>
		{/if}

		<!-- Generation form -->
		<form onsubmit={generate} class="rounded-xl border border-border bg-surface p-6">
			<h2 class="text-lg font-semibold mb-4">{t('sop_generator.generate_title', 'Generate a draft')}</h2>
			<!-- Destination is the first input and also filters the template +
			     sample lists below (geo-aware pattern — §110). It renders its
			     own label. -->
			<div class="mb-4">
				<DestinationSelector {destinations} bind:selectedId={genDest} onchange={applyFilters} />
			</div>
			<div class="grid gap-4 sm:grid-cols-2">
				<div>
					<label class="block text-sm font-medium mb-1" for="genPurpose">
						{t('sop_generator.purpose', 'Purpose')}
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
						class="w-full"
					/>
				</div>
				<div>
					<label class="block text-sm font-medium mb-1" for="genProgram">
						{t('sop_generator.program', 'Program')}
					</label>
					<input id="genProgram" type="text" bind:value={genProgram} placeholder="e.g. MSc Computer Science" class="input w-full" />
				</div>
				<div>
					<label class="block text-sm font-medium mb-1" for="genInstitution">
						{t('sop_generator.institution', 'Institution')}
					</label>
					<input id="genInstitution" type="text" bind:value={genInstitution} placeholder="e.g. University of Toronto" class="input w-full" />
				</div>
			</div>
			<div class="mt-4">
				<label class="block text-sm font-medium mb-1" for="genBackground">
					{t('sop_generator.background', 'Brief background')}
				</label>
				<textarea
					id="genBackground"
					bind:value={genBackground}
					rows="3"
					placeholder={t('sop_generator.background_placeholder', 'e.g. 5 years software engineering, BSc from UNILAG, first-class honours…')}
					class="input w-full"
				></textarea>
			</div>
			<div class="mt-4">
				<button
					type="submit"
					class="btn-primary"
					disabled={generating || !canGenerate || !user || !genPurpose}
				>
					{generating ? t('sop_generator.generating', 'Generating…') : t('sop_generator.generate', 'Generate draft')}
				</button>
				{#if !user}
					<span class="ms-3 text-xs text-muted">
						{t('sop_generator.signin_required', 'Sign in to generate')}
					</span>
				{:else if !canGenerate}
					<span class="ms-3 text-xs text-red-600">
						{t('sop_generator.quota_exhausted', 'Monthly quota exhausted')}
					</span>
				{/if}
			</div>
		</form>

		{#if genError}
			<div class="mt-4 rounded-lg border border-red-500/30 bg-red-500/10 p-3 text-sm text-red-600">
				{genError}
			</div>
		{/if}

		<!-- Draft editor -->
		{#if activeDraft}
			<div class="mt-6 rounded-xl border border-border bg-surface p-6">
				<div class="flex items-center justify-between mb-3">
					<h2 class="text-lg font-semibold">{t('sop_generator.edit_draft', 'Edit draft')}</h2>
					<div class="flex items-center gap-2 text-xs text-muted">
						<span>v{activeDraft.version}</span>
						<span>·</span>
						<span>{activeDraft.status}</span>
						{#if activeDraft.ai_model}
							<span>· {activeDraft.ai_model}</span>
						{/if}
					</div>
				</div>
				{#if activeDraft.status === 'failed'}
					<div class="mb-3 rounded-lg border border-red-500/30 bg-red-500/10 p-3 text-sm text-red-600">
						{t('sop_generator.draft_failed', 'Generation failed — you can edit or delete this draft, or generate a new one.')}
					</div>
				{:else if activeDraft.status === 'queued'}
					<div class="mb-3 rounded-lg border border-border bg-surface p-3 text-sm text-muted">
						{t('sop_generator.draft_queued', 'Generation in progress — this draft updates automatically.')}
					</div>
				{/if}
				<input
					type="text"
					bind:value={editTitle}
					placeholder={t('sop_generator.draft_title', 'Draft title')}
					class="input w-full mb-3"
				/>
				<textarea
					bind:value={editText}
					rows="16"
					class="input w-full font-mono text-sm"
				></textarea>
				<div class="mt-3 flex items-center gap-3">
					<button class="btn-primary" onclick={saveDraft} disabled={savingDraft}>
						{savingDraft ? t('sop_generator.saving', 'Saving…') : t('sop_generator.save', 'Save draft')}
					</button>
					<button
						class="btn-secondary text-red-600"
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
		{#if user && drafts.length > 0}
			<div class="mt-8">
				<h2 class="text-sm font-semibold text-muted uppercase tracking-wide mb-3">
					{t('sop_generator.my_drafts', 'My drafts')}
				</h2>
				<div class="space-y-2">
					{#each drafts as draft (draft.id)}
						<div class="flex items-center justify-between rounded-lg border border-border bg-surface px-4 py-3">
							<button class="text-start flex-1" onclick={() => openDraft(draft)}>
								<div class="font-medium text-sm">{draft.title || draft.purpose}</div>
								<div class="text-xs text-muted mt-0.5">
									{draftMeta(draft)}
								</div>
							</button>
							<button
								class="text-xs text-muted hover:text-red-600 ms-3"
								onclick={() => deleteDraft(draft.id)}
								aria-label={t('sop_generator.delete_draft', 'Delete draft')}
							>
								<Icon name="trash-2" size={14} />
							</button>
						</div>
					{/each}
				</div>
			</div>
		{/if}

		<!-- Templates (public, filtered by destination + purpose) -->
		<div class="mt-8">
			<h2 class="text-sm font-semibold text-muted uppercase tracking-wide mb-3">
				{t('sop_generator.templates', 'Templates')}
			</h2>
			{#if templates.length > 0}
				<div class="grid gap-3 sm:grid-cols-2">
					{#each templates as tpl (tpl.id)}
						{@const sections = templateSections(tpl)}
						<div class="rounded-lg border border-border bg-surface p-4">
							<div class="font-medium text-sm">{tpl.title}</div>
							<div class="text-xs text-muted mt-0.5">{selectedLabel(purposeItems, tpl.purpose) ?? tpl.purpose}</div>
							{#if sections.length}
								<div class="mt-2 space-y-1">
									{#each sections as section (section.section)}
										<div class="text-xs">
											<span class="font-medium">{section.section}</span>
											{#if section.max_words}
												<span class="text-muted">· ~{section.max_words}w</span>
											{/if}
										</div>
									{/each}
								</div>
							{/if}
						</div>
					{/each}
				</div>
			{:else}
				<p class="text-xs text-muted">{t('sop_generator.no_templates', 'No templates match the selected destination and purpose.')}</p>
			{/if}
		</div>

		<!-- Samples (public, filtered by destination + level + field) -->
		<div class="mt-8">
			<h2 class="text-sm font-semibold text-muted uppercase tracking-wide mb-3">
				{t('sop_generator.samples', 'Sample SOPs')}
			</h2>
			<div class="mb-3 grid gap-3 sm:grid-cols-2">
				<div>
					<label class="block text-xs font-medium text-muted mb-1" for="sampleLevel">
						{t('sop_generator.filter_level', 'Program level')}
					</label>
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
						class="w-full"
					/>
				</div>
				<div>
					<label class="block text-xs font-medium text-muted mb-1" for="sampleField">
						{t('sop_generator.filter_field', 'Field of study')}
					</label>
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
						class="w-full"
					/>
				</div>
			</div>
			{#if samples.length > 0}
				<div class="space-y-3">
					{#each samples as sample (sample.id)}
						<details class="rounded-lg border border-border bg-surface">
							<summary class="cursor-pointer px-4 py-3 font-medium text-sm">
								{sample.title}
								{#if sample.destination_space_name}
									<span class="text-xs text-muted font-normal">· {sample.destination_space_name}</span>
								{/if}
								{#if sample.word_count}
									<span class="text-xs text-muted font-normal">· {sample.word_count}w</span>
								{/if}
							</summary>
							<div class="border-t border-border px-4 py-3 text-sm whitespace-pre-wrap">
								{sample.body}
							</div>
						</details>
					{/each}
				</div>
			{:else}
				<p class="text-xs text-muted">{t('sop_generator.no_samples', 'No samples match the selected filters.')}</p>
			{/if}
		</div>
	</div>

	<RegistrationPrompt
		open={showRegisterPrompt}
		onClose={() => (showRegisterPrompt = false)}
		icon="pen-line"
		title={t('register.sop_generate', 'Generate your SOP')}
		description={t('register.sop_generate_desc', 'Sign in to generate a tailored statement of purpose draft.')}
	/>
{/if}
