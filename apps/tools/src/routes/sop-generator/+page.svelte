<script lang="ts">
	import { untrack, onMount, onDestroy } from 'svelte';
	import { api } from '$lib/api/client';
	import { buildSeoMeta } from '$lib/seo';
	import { t } from '$lib/i18n';
	import { page } from '$app/state';
	import Icon from '$lib/components/ui/Icon.svelte';
	import EmptyState from '$lib/components/ui/EmptyState.svelte';
	import RegistrationPrompt from '$lib/components/ui/RegistrationPrompt.svelte';
	import ToolDisabled from '$lib/components/ui/ToolDisabled.svelte';
	import DestinationSelector from '$lib/components/tools/DestinationSelector.svelte';
	import AutocompleteSelect from '$lib/components/ui/AutocompleteSelect.svelte';
	import type {
		GeoSpaceOption,
		SopTemplate,
		SopSample,
		SopDraft,
		SopQuota
	} from '$lib/types';

	/**
	 * AI SOP Generator — §110, ADR-121.
	 * Public: template + sample browsing. Auth: quota display, generation
	 * form (async — 202 + draft_id, polled via GET /sop/drafts/:id), draft
	 * editor with version save.
	 */
	let { data } = $props();

	let destinations = $state<GeoSpaceOption[]>(untrack(() => data.destinations));
	let templates = $state<SopTemplate[]>(untrack(() => data.templates));
	let samples = $state<SopSample[]>(untrack(() => data.samples));
	let quota = $state<SopQuota | null>(untrack(() => data.quota));
	let drafts = $state<SopDraft[]>(untrack(() => data.drafts));
	let user = $state(untrack(() => data.user));

	// Generate form
	let genDest = $state('');
	let genPurpose = $state('study_permit');
	let genProgram = $state('');
	let genInstitution = $state('');
	let genBackground = $state('');
	let generating = $state(false);
	let genError = $state('');
	let showRegisterPrompt = $state(false);

	// Active draft being edited
	let activeDraft = $state<SopDraft | null>(null);
	let editText = $state('');
	let editTitle = $state('');
	let savingDraft = $state(false);
	let draftMsg = $state('');

	// Poll timer for queued drafts
	let pollTimer: ReturnType<typeof setInterval> | null = null;

	const seo = $derived(
		buildSeoMeta({
			title: t('sop_generator.seo_title', 'AI SOP Generator — JapaConnect Tools'),
			description: t(
				'sop_generator.seo_description',
				'Generate a tailored statement of purpose draft from proven templates — browse samples, refine, and export.'
			),
			path: '/sop-generator',
			origin: page.url.origin,
			jsonLd: {
				'@context': 'https://schema.org',
				'@type': 'SoftwareApplication',
				name: 'AI SOP Generator',
				applicationCategory: 'UtilitiesApplication',
				operatingSystem: 'Web',
				offers: { '@type': 'Offer', price: '0', priceCurrency: 'USD' },
				description: 'Generate a tailored statement of purpose draft for visa and study applications.'
			}
		})
	);

	const purposeOptions = [
		{ value: 'study_permit', label: 'Study permit' },
		{ value: 'student_visa', label: 'Student visa' },
		{ value: 'work_visa', label: 'Work visa' },
		{ value: 'visitor_visa', label: 'Visitor visa' }
	];

	const canGenerate = $derived(
		quota === null || quota.remaining > 0
	);

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
		// Resume polling for any draft still queued.
		const queued = drafts.find((d) => d.status === 'queued');
		if (queued) startPolling(queued.id);
	});

	onDestroy(() => {
		if (pollTimer) clearInterval(pollTimer);
	});
</script>

<svelte:head>
	<title>{seo.title}</title>
	<meta name="description" content={seo.description} />
	<link rel="canonical" href={seo.canonical} />
	<meta property="og:title" content={seo.title} />
	<meta property="og:description" content={seo.description} />
	{#if seo.jsonLdString}
		{@html `<script type="application/ld+json">${seo.jsonLdString}<\/script>`}
	{/if}
</svelte:head>

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
					{t('sop_generator.quota', 'Monthly quota')}: {quota.used}/{quota.limit} used
				</span>
				<span class="font-medium {quota.remaining > 0 ? 'text-green-600' : 'text-red-600'}">
					{quota.remaining} {t('sop_generator.remaining', 'remaining')}
				</span>
			</div>
		{/if}

		<!-- Generation form -->
		<form onsubmit={generate} class="rounded-xl border border-border bg-surface p-6">
			<h2 class="text-lg font-semibold mb-4">{t('sop_generator.generate_title', 'Generate a draft')}</h2>
			<div class="grid gap-4 sm:grid-cols-2">
				<div>
					<label class="block text-sm font-medium mb-1" for="genDest">
						{t('sop_generator.destination', 'Destination')}
					</label>
					<DestinationSelector {destinations} bind:selectedId={genDest} />
				</div>
				<div>
					<label class="block text-sm font-medium mb-1" for="genPurpose">
						{t('sop_generator.purpose', 'Purpose')}
					</label>
					<AutocompleteSelect
						id="genPurpose"
						bind:value={genPurpose}
						options={purposeOptions}
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
					disabled={generating || !canGenerate || !user}
				>
					{generating ? t('sop_generator.generating', 'Generating…') : t('sop_generator.generate', 'Generate draft')}
				</button>
				{#if !user}
					<span class="ml-3 text-xs text-muted">
						{t('sop_generator.signin_required', 'Sign in to generate')}
					</span>
				{:else if !canGenerate}
					<span class="ml-3 text-xs text-red-600">
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
							<button class="text-left flex-1" onclick={() => openDraft(draft)}>
								<div class="font-medium text-sm">{draft.title || draft.purpose}</div>
								<div class="text-xs text-muted mt-0.5">
									{draft.program ?? ''} {draft.institution ? `· ${draft.institution}` : ''} · {draft.status}
								</div>
							</button>
							<button class="text-xs text-muted hover:text-red-600 ml-3" onclick={() => deleteDraft(draft.id)}>
								<Icon name="trash-2" size={14} />
							</button>
						</div>
					{/each}
				</div>
			</div>
		{/if}

		<!-- Templates (public) -->
		{#if templates.length > 0}
			<div class="mt-8">
				<h2 class="text-sm font-semibold text-muted uppercase tracking-wide mb-3">
					{t('sop_generator.templates', 'Templates')}
				</h2>
				<div class="grid gap-3 sm:grid-cols-2">
					{#each templates as tpl (tpl.id)}
						<div class="rounded-lg border border-border bg-surface p-4">
							<div class="font-medium text-sm">{tpl.title}</div>
							<div class="text-xs text-muted mt-0.5">{tpl.purpose}</div>
							{#if tpl.structure?.length}
								<div class="mt-2 space-y-1">
									{#each tpl.structure as section (section.section)}
										<div class="text-xs">
											<span class="font-medium">{section.section}</span>
											{#if section.min_words}
												<span class="text-muted">· ~{section.min_words}w</span>
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
			<div class="mt-8">
				<h2 class="text-sm font-semibold text-muted uppercase tracking-wide mb-3">
					{t('sop_generator.samples', 'Sample SOPs')}
				</h2>
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
			</div>
		{/if}
	</div>

	<RegistrationPrompt
		open={showRegisterPrompt}
		onClose={() => (showRegisterPrompt = false)}
		icon="pen-line"
		title={t('register.sop_generate', 'Generate your SOP')}
		description={t('register.sop_generate_desc', 'Sign in to generate a tailored statement of purpose draft.')}
	/>
{/if}
