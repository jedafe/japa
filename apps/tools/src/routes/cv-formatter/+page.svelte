<script lang="ts">
	import { untrack, onMount, onDestroy } from 'svelte';
	import { api } from '$lib/api/client';
	import { buildSeoMeta, mainAppUrl } from '$lib/seo';
	import { t } from '$lib/i18n';
	import { page } from '$app/state';
	import Icon from '$lib/components/ui/Icon.svelte';
	import EmptyState from '$lib/components/ui/EmptyState.svelte';
	import LoadingSkeleton from '$lib/components/ui/LoadingSkeleton.svelte';
	import RegistrationPrompt from '$lib/components/ui/RegistrationPrompt.svelte';
	import ToolDisabled from '$lib/components/ui/ToolDisabled.svelte';
	import DestinationSelector from '$lib/components/tools/DestinationSelector.svelte';
	import type { GeoSpaceOption, CvStandard, CvFormatSession } from '$lib/types';

	/**
	 * CV Formatter tool — §95.
	 * Destination selector → CV standard display → input textarea →
	 * "Format CV" (auth, 202 Accepted) → poll session until complete →
	 * show output + download link.
	 */
	let { data } = $props();

	let destinations = $state<GeoSpaceOption[]>(untrack(() => data.destinations));
	let standards = $state<CvStandard[]>(untrack(() => data.standards));
	let user = $state(untrack(() => data.user));

	let selectedDest = $state('');
	let inputText = $state('');
	let outputFormat = $state('text');
	let loadingStandard = $state(false);
	let currentStandard = $state<CvStandard | null>(null);

	// Session state
	let session = $state<CvFormatSession | null>(null);
	let submitting = $state(false);
	let pollTimer: ReturnType<typeof setTimeout> | null = null;
	let showRegisterPrompt = $state(false);

	const seo = $derived(
		buildSeoMeta({
			title: t('cv_formatter.seo_title', 'CV Formatter — JapaConnect Tools'),
			description: t(
				'cv_formatter.seo_description',
				'Get destination-specific CV standards and AI-assisted formatting — photo policy, sections, tone, and common pitfalls.'
			),
			path: '/cv-formatter',
			origin: page.url.origin
		})
	);

	// Filter the destination dropdown to only show destinations that have a
	// CV standard available. Match by destination_space_id.
	const availableDestinations = $derived.by(() => {
		const standardDestIds = new Set(standards.map((s) => s.destination_space_id));
		return destinations.filter((d) => standardDestIds.has(d.id));
	});

	const isProcessing = $derived(
		session != null && (session.status === 'pending' || session.status === 'processing')
	);

	async function loadStandard(destId: string) {
		if (!destId) {
			currentStandard = null;
			return;
		}
		// Try the in-memory standards list first (already fetched in SSR).
		const cached = standards.find((s) => s.destination_space_id === destId);
		if (cached) {
			currentStandard = cached;
			return;
		}
		// Fallback: fetch the single standard for this destination.
		loadingStandard = true;
		const res = await api.get<CvStandard>(`/cv-formatter/standards/${destId}`);
		loadingStandard = false;
		if (!res.error && res.data) {
			currentStandard = res.data;
		} else {
			currentStandard = null;
		}
	}

	function onDestChange(id: string) {
		loadStandard(id);
		// Reset session when destination changes.
		session = null;
	}

	async function formatCv() {
		if (!selectedDest || !inputText.trim()) return;
		if (!user) {
			showRegisterPrompt = true;
			return;
		}
		submitting = true;
		session = null;
		const body: Record<string, unknown> = {
			destination_space_id: selectedDest,
			input_text: inputText.trim(),
			output_format: outputFormat
		};
		const res = await api.post<CvFormatSession>('/cv-formatter/format', body);
		submitting = false;
		if (!res.error && res.data) {
			session = res.data;
			pollSession(res.data.id);
		}
	}

	async function pollSession(sessionId: string) {
		if (pollTimer) clearTimeout(pollTimer);
		const res = await api.get<CvFormatSession>(`/cv-formatter/sessions/${sessionId}`);
		if (!res.error && res.data) {
			session = res.data;
			if (res.data.status === 'pending' || res.data.status === 'processing') {
				// Poll every 3 seconds until complete or failed.
				pollTimer = setTimeout(() => pollSession(sessionId), 3000);
			}
		}
	}

	function downloadUrl(sessionId: string): string {
		// The download endpoint returns a binary file — link directly.
		return `/cv-formatter/sessions/${sessionId}/download`;
	}

	onDestroy(() => {
		if (pollTimer) clearTimeout(pollTimer);
	});

	onMount(() => {
		// No auto-load — requires dest selection.
	});
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

<div class="mx-auto max-w-5xl px-4 py-8">
	{#if !data.enabled}
		<ToolDisabled tool={data.tool} />
	{:else}
		<div class="mb-8">
			<h1 class="text-2xl sm:text-3xl font-bold text-text">
				{t('cv_formatter.title', 'CV Formatter')}
			</h1>
			<p class="mt-2 text-text-subtle">
				{t(
					'cv_formatter.subtitle',
					'Get destination-specific CV standards and AI-assisted formatting — photo policy, sections, tone, and common pitfalls.'
				)}
			</p>
		</div>

		{#if destinations.length === 0}
			<EmptyState
				icon="file-text"
				title={t('cv_formatter.no_destinations', 'No destinations available')}
				description={t(
					'cv_formatter.no_destinations_desc',
					'Destination data has not been configured yet. Please check back later.'
				)}
			/>
		{:else if availableDestinations.length === 0}
			<EmptyState
				icon="file-text"
				title={t('cv_formatter.no_standards', 'No CV standards available')}
				description={t(
					'cv_formatter.no_standards_desc',
					'CV standards have not been configured for any destination yet. Please check back later.'
				)}
			/>
		{:else}
			<!-- Destination selector (filtered to destinations with a standard) -->
			<div class="mb-6">
				<DestinationSelector
					destinations={availableDestinations}
					bind:selectedId={selectedDest}
					onchange={onDestChange}
				/>
			</div>

			{#if selectedDest}
				<!-- CV Standard display -->
				{#if loadingStandard}
					<div class="space-y-2 mb-6">
						<LoadingSkeleton height="40px" />
						<LoadingSkeleton height="40px" />
						<LoadingSkeleton height="80px" />
					</div>
				{:else if currentStandard}
					<div class="mb-6 p-5 bg-surface border border-border rounded-lg space-y-4">
						<h2 class="text-sm font-semibold text-muted uppercase tracking-wide">
							{t('cv_formatter.standard_for', 'CV standard for')} {currentStandard.destination_space_name ?? ''}
						</h2>

						<div class="grid grid-cols-1 sm:grid-cols-3 gap-4">
							{#if currentStandard.recommended_length_pages}
								<div>
									<p class="text-xs text-muted mb-0.5">{t('cv_formatter.length', 'Recommended length')}</p>
									<p class="text-sm font-medium text-text">
										{currentStandard.recommended_length_pages}
									</p>
								</div>
							{/if}
							{#if currentStandard.photo_policy}
								<div>
									<p class="text-xs text-muted mb-0.5">{t('cv_formatter.photo_policy', 'Photo policy')}</p>
									<p class="text-sm font-medium text-text">
										{currentStandard.photo_policy}
									</p>
								</div>
							{/if}
							{#if currentStandard.date_format}
								<div>
									<p class="text-xs text-muted mb-0.5">{t('cv_formatter.date_format', 'Date format')}</p>
									<p class="text-sm font-medium text-text">
										{currentStandard.date_format}
									</p>
								</div>
							{/if}
						</div>

						{#if currentStandard.recommended_sections && currentStandard.recommended_sections.length > 0}
							<div>
								<p class="text-xs text-muted mb-1.5">{t('cv_formatter.recommended_sections', 'Recommended sections')}</p>
								<div class="flex flex-wrap gap-2">
									{#each currentStandard.recommended_sections as section (section)}
										<span class="px-2.5 py-1 bg-surface-subtle text-text text-xs rounded-full">
											{section}
										</span>
									{/each}
								</div>
							</div>
						{/if}

						{#if currentStandard.tone_guidance}
							<div>
								<p class="text-xs text-muted mb-1">{t('cv_formatter.tone_guidance', 'Tone guidance')}</p>
								<p class="text-sm text-text leading-relaxed">{currentStandard.tone_guidance}</p>
							</div>
						{/if}

						{#if currentStandard.common_pitfalls}
							<div class="p-3 bg-orange-500/5 border border-orange-500/20 rounded-lg">
								<p class="text-xs text-orange-600 font-medium mb-1 flex items-center gap-1">
									<Icon name="alert-triangle" size={14} />
									{t('cv_formatter.common_pitfalls', 'Common pitfalls')}
								</p>
								<p class="text-sm text-text leading-relaxed">{currentStandard.common_pitfalls}</p>
							</div>
						{/if}
					</div>
				{/if}

				<!-- CV input + format -->
				<div class="mb-6">
					<h2 class="text-sm font-semibold text-muted uppercase tracking-wide mb-3">
						{t('cv_formatter.input', 'Paste your CV')}
					</h2>

					<div class="space-y-3">
						<div class="flex items-center gap-3">
							<label for="output-format" class="text-sm font-medium text-text whitespace-nowrap">
								{t('cv_formatter.output_format', 'Format:')}
							</label>
							<select
								id="output-format"
								bind:value={outputFormat}
								class="px-3 py-1.5 bg-surface border border-border rounded-lg text-sm text-text focus:border-primary focus:outline-none"
							>
								<option value="text">{t('cv_formatter.format_text', 'Plain text')}</option>
								<option value="markdown">{t('cv_formatter.format_markdown', 'Markdown')}</option>
								<option value="html">{t('cv_formatter.format_html', 'HTML')}</option>
							</select>
						</div>

						<textarea
							bind:value={inputText}
							rows="10"
							placeholder={t(
								'cv_formatter.input_placeholder',
								'Paste your current CV/resume text here…'
							)}
							class="w-full px-4 py-3 bg-surface border border-border rounded-lg text-sm text-text focus:border-primary focus:outline-none resize-y font-mono"
						></textarea>

						<div class="flex items-center justify-between">
							<p class="text-xs text-muted">
								{inputText.length.toLocaleString()} {t('cv_formatter.characters', 'characters')}
							</p>
							<button
								type="button"
								onclick={formatCv}
								disabled={!inputText.trim() || submitting || isProcessing}
								class="inline-flex items-center gap-2 px-5 py-2 bg-[var(--tool-accent)] text-white rounded-lg text-sm font-medium hover:bg-[var(--tool-accent-hover)] transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
							>
								{#if submitting}
									<Icon name="loader-2" size={16} class="animate-spin" />
									{t('cv_formatter.submitting', 'Submitting…')}
								{:else if isProcessing}
									<Icon name="loader-2" size={16} class="animate-spin" />
									{t('cv_formatter.processing', 'Formatting…')}
								{:else}
									<Icon name="sparkles" size={16} />
									{t('cv_formatter.format_cv', 'Format CV')}
								{/if}
							</button>
						</div>
					</div>
				</div>

				<!-- Session output -->
				{#if session}
					<div class="space-y-4">
						{#if session.status === 'pending' || session.status === 'processing'}
							<div class="p-4 bg-surface border border-border rounded-lg flex items-center gap-3">
								<Icon name="loader-2" size={20} class="animate-spin text-primary" />
								<div>
									<p class="text-sm font-medium text-text">
										{t('cv_formatter.processing', 'Formatting your CV…')}
									</p>
									<p class="text-xs text-muted">
										{t('cv_formatter.processing_desc', 'This usually takes a few seconds. Please wait.')}
									</p>
								</div>
							</div>
						{:else if session.status === 'failed'}
							<div class="p-4 bg-red-500/5 border border-red-500/20 rounded-lg flex items-start gap-3">
								<Icon name="x-circle" size={20} class="text-red-500 flex-shrink-0 mt-0.5" />
								<div>
									<p class="text-sm font-medium text-text">
										{t('cv_formatter.failed', 'Formatting failed')}
									</p>
									<p class="text-xs text-muted mt-0.5">
										{t('cv_formatter.failed_desc', 'Something went wrong during formatting. Please try again.')}
									</p>
								</div>
							</div>
						{:else if session.status === 'complete'}
							<div class="p-4 bg-surface border border-border rounded-lg">
								<div class="flex items-center justify-between mb-3">
									<h2 class="text-sm font-semibold text-muted uppercase tracking-wide">
										{t('cv_formatter.output', 'Formatted CV')}
									</h2>
									<a
										href={downloadUrl(session.id)}
										class="inline-flex items-center gap-1.5 px-3 py-1.5 bg-primary text-primary-contrast rounded-lg text-sm font-medium hover:bg-primary-hover transition-colors"
									>
										<Icon name="download" size={14} />
										{t('cv_formatter.download', 'Download')}
									</a>
								</div>
								{#if session.output_text}
									<pre class="text-sm text-text whitespace-pre-wrap font-mono bg-surface-subtle p-4 rounded-lg max-h-96 overflow-y-auto">{session.output_text}</pre>
								{:else}
									<p class="text-sm text-muted">
										{t('cv_formatter.no_output', 'No output text was generated.')}
									</p>
								{/if}
								{#if session.tokens_used}
									<p class="text-xs text-muted mt-2">
										{t('cv_formatter.tokens_used', 'Tokens used:')} {session.tokens_used}
									</p>
								{/if}
							</div>
						{/if}
					</div>
				{/if}

				<!-- Auth hint for unauthed users -->
				{#if !user}
					<div class="mt-6 p-4 bg-surface border border-border rounded-lg text-center">
						<p class="text-sm text-text-subtle mb-2">
							{t('cv_formatter.auth_hint', 'Sign in to use the AI-assisted CV formatting feature.')}
						</p>
						<a
							href={mainAppUrl('/login')}
							class="inline-flex items-center gap-1.5 px-4 py-2 bg-primary text-primary-contrast rounded-lg text-sm font-medium hover:bg-primary-hover transition-colors"
						>
							{t('common.sign_in', 'Sign in')}
						</a>
					</div>
				{/if}
			{/if}
		{/if}
	{/if}

	{#if showRegisterPrompt}
		<RegistrationPrompt
			open={showRegisterPrompt}
			onClose={() => (showRegisterPrompt = false)}
			icon="sparkles"
			title={t('register.format_cv', 'Format your CV with AI')}
			description={t(
				'register.format_cv_desc',
				'Create a free account to use AI-assisted CV formatting tailored to your destination country.'
			)}
		/>
	{/if}
</div>
