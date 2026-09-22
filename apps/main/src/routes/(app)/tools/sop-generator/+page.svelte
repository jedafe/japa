<script lang="ts">
	import { untrack, onMount, onDestroy } from 'svelte';
	import { api } from '$lib/api/client';
	import { t } from '$lib/i18n';
	import { page } from '$app/state';
	import { PenLine, Trash2 } from 'lucide-svelte';
	import DestinationSelector from '$lib/components/tools/DestinationSelector.svelte';
	import type { GeoSpaceOption, SopTemplate, SopSample, SopDraft, SopQuota } from '$lib/types';
	import type { PageData } from './$types';

	/**
	 * AI SOP Generator — §110, ADR-121.
	 * Public: template + sample browsing. Auth: quota display, generation
	 * form (async — 202 + draft_id, polled via GET /sop/drafts/:id), draft
	 * editor with version save.
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

	// Generate form
	let genDest = $state('');
	let genPurpose = $state('study_permit');
	let genProgram = $state('');
	let genInstitution = $state('');
	let genBackground = $state('');
	let generating = $state(false);
	let genError = $state('');

	// Active draft being edited
	let activeDraft = $state<SopDraft | null>(null);
	let editText = $state('');
	let editTitle = $state('');
	let savingDraft = $state(false);
	let draftMsg = $state('');

	// Poll timer for queued drafts
	let pollTimer: ReturnType<typeof setInterval> | null = null;

	const title = $derived(t('sop_generator.seo_title', 'AI SOP Generator — JapaConnect'));
	const description = $derived(
		t('sop_generator.seo_description', 'Generate a tailored statement of purpose draft from proven templates — browse samples, refine, and export.')
	);

	const purposeOptions = [
		{ value: 'study_permit', label: 'Study permit' },
		{ value: 'student_visa', label: 'Student visa' },
		{ value: 'work_visa', label: 'Work visa' },
		{ value: 'visitor_visa', label: 'Visitor visa' }
	];

	const canGenerate = $derived(quota === null || quota.remaining > 0);

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

	function startPolling(draftId: string) {
		if (pollTimer) clearInterval(pollTimer);
		pollTimer = setInterval(async () => {
			const res = await api.get<SopDraft>(`/sop/drafts/${draftId}`);
			if (res.error || !res.data) return;
			if (res.data.status !== 'queued') {
				if (pollTimer) clearInterval(pollTimer);
				pollTimer = null;
				await refreshDrafts();
				activeDraft = res.data;
				editText = res.data.generated_text ?? '';
				editTitle = res.data.title ?? '';
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
		activeDraft = res.data;
		await refreshDrafts();
	}

	async function deleteDraft(id: string) {
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

	onDestroy(() => {
		if (pollTimer) clearInterval(pollTimer);
	});

	const inputCls =
		'w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)]';
	const labelCls = 'mb-1 block text-sm font-medium text-[var(--color-text)]';
	const cardCls = 'rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)]';
	const sectionCls = 'mb-3 text-sm font-semibold uppercase tracking-wide text-[var(--color-muted)]';
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
				{t('sop_generator.quota', 'Monthly quota')}: {quota.used}/{quota.limit} used
			</span>
			<span class="font-medium {quota.remaining > 0 ? 'text-green-600' : 'text-[var(--color-danger)]'}">
				{quota.remaining} {t('sop_generator.remaining', 'remaining')}
			</span>
		</div>
	{/if}

	<!-- Generation form -->
	<form onsubmit={generate} class="{cardCls} p-4">
		<h2 class="mb-4 text-base font-semibold text-[var(--color-text)]">{t('sop_generator.generate_title', 'Generate a draft')}</h2>
		<div class="grid gap-4 sm:grid-cols-2">
			<div>
				<label class={labelCls} for="genDest">{t('sop_generator.destination', 'Destination')}</label>
				<DestinationSelector {destinations} bind:selectedId={genDest} />
			</div>
			<div>
				<label class={labelCls} for="genPurpose">{t('sop_generator.purpose', 'Purpose')}</label>
				<select id="genPurpose" bind:value={genPurpose} class={inputCls}>
					{#each purposeOptions as opt (opt.value)}
						<option value={opt.value}>{opt.label}</option>
					{/each}
				</select>
			</div>
			<div>
				<label class={labelCls} for="genProgram">{t('sop_generator.program', 'Program')}</label>
				<input id="genProgram" type="text" bind:value={genProgram} placeholder="e.g. MSc Computer Science" class={inputCls} />
			</div>
			<div>
				<label class={labelCls} for="genInstitution">{t('sop_generator.institution', 'Institution')}</label>
				<input id="genInstitution" type="text" bind:value={genInstitution} placeholder="e.g. University of Toronto" class={inputCls} />
			</div>
		</div>
		<div class="mt-4">
			<label class={labelCls} for="genBackground">{t('sop_generator.background', 'Brief background')}</label>
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
				disabled={generating || !canGenerate || !authed}
				class="inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-white hover:opacity-90 disabled:opacity-50"
			>
				{generating ? t('sop_generator.generating', 'Generating…') : t('sop_generator.generate', 'Generate draft')}
			</button>
			{#if !authed}
				<span class="ml-3 text-xs text-[var(--color-muted)]">
					{t('sop_generator.signin_required', 'Sign in to generate')}
				</span>
			{:else if !canGenerate}
				<span class="ml-3 text-xs text-[var(--color-danger)]">
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
						<button class="flex-1 text-left" onclick={() => openDraft(draft)}>
							<div class="text-sm font-medium text-[var(--color-text)]">{draft.title || draft.purpose}</div>
							<div class="mt-0.5 text-xs text-[var(--color-muted)]">
								{draft.program ?? ''} {draft.institution ? `· ${draft.institution}` : ''} · {draft.status}
							</div>
						</button>
						<button class="ml-3 text-[var(--color-muted)] hover:text-[var(--color-danger)]" onclick={() => deleteDraft(draft.id)} aria-label="Delete draft">
							<Trash2 size={14} />
						</button>
					</div>
				{/each}
			</div>
		</div>
	{/if}

	<!-- Templates (public) -->
	{#if templates.length > 0}
		<div class="mt-6">
			<h2 class={sectionCls}>{t('sop_generator.templates', 'Templates')}</h2>
			<div class="grid gap-3 sm:grid-cols-2">
				{#each templates as tpl (tpl.id)}
					<div class="{cardCls} p-3">
						<div class="text-sm font-medium text-[var(--color-text)]">{tpl.title}</div>
						<div class="mt-0.5 text-xs text-[var(--color-muted)]">{tpl.purpose}</div>
						{#if tpl.structure?.length}
							<div class="mt-2 space-y-1">
								{#each tpl.structure as section (section.section)}
									<div class="text-xs text-[var(--color-text)]">
										<span class="font-medium">{section.section}</span>
										{#if section.min_words}
											<span class="text-[var(--color-muted)]">· ~{section.min_words}w</span>
										{/if}
									</div>
								{/each}
							</div>
						{/if}
					</div>
				{/each}
			</div>
		</div>
	{/if}

	<!-- Samples (public) -->
	{#if samples.length > 0}
		<div class="mt-6">
			<h2 class={sectionCls}>{t('sop_generator.samples', 'Sample SOPs')}</h2>
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
		</div>
	{/if}
</div>
