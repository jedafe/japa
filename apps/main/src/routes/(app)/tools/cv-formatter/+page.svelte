<script lang="ts">
	import { untrack, onDestroy } from 'svelte';
	import { t } from '$lib/i18n';
	import { page } from '$app/state';
	import { FileText, Lock, Sparkles, Loader2, AlertTriangle, XCircle, Download, Upload } from 'lucide-svelte';
	import { EmptyState, LoadingSkeleton } from '$lib/components/ui';
	import { api } from '$lib/api/client';
	import type { GeoSpaceOption, CvStandard, CvFormatSession } from '$lib/types';
	import type { PageData } from './$types';
	import Autocomplete from '$lib/components/create/Autocomplete.svelte';
	import type { AutocompleteItem } from '$lib/components/create/Autocomplete.svelte';

	/**
	 * CV Formatter tool — §95.
	 * Destination selector → CV standard display → input textarea →
	 * "Format CV" (auth, 202 Accepted) → poll session until complete →
	 * show output + download link.
	 */
	let { data }: { data: PageData } = $props();

	const title = $derived(t('cv_formatter.title', 'CV Formatter'));
	const description = $derived(
		t(
			'cv_formatter.subtitle',
			'Get destination-specific CV standards and AI-assisted formatting — photo policy, sections, tone, and common pitfalls.'
		)
	);
	const disabled = $derived(data.disabled);
	const authed = $derived(data.authed);

	let destinations = $state<GeoSpaceOption[]>(untrack(() => data.destinations));
	let standards = $state<CvStandard[]>(untrack(() => data.standards));

	let selectedDest = $state('');
	let inputText = $state('');
	let outputFormat = $state('docx');
	let loadingStandard = $state(false);
	let currentStandard = $state<CvStandard | null>(null);

	// Input mode: 'text' (paste) or 'file' (upload)
	let inputMode = $state<'text' | 'file'>('text');
	let uploadedFile = $state<File | null>(null);
	let fileError = $state('');

	// Session state
	let session = $state<CvFormatSession | null>(null);
	let submitting = $state(false);
	let pollTimer: ReturnType<typeof setTimeout> | null = null;
	let showRegisterPrompt = $state(false);

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
		if (!selectedDest) return;
		if (inputMode === 'text' && !inputText.trim()) return;
		if (inputMode === 'file' && !uploadedFile) return;
		if (!authed) {
			showRegisterPrompt = true;
			return;
		}
		submitting = true;
		session = null;

		let res;
		if (inputMode === 'file' && uploadedFile) {
			const formData = new FormData();
			formData.append('file', uploadedFile);
			formData.append('destination_space_id', selectedDest);
			formData.append('output_format', outputFormat);
			res = await api.postMultipart<CvFormatSession>('/cv-formatter/format', formData);
		} else {
			const body: Record<string, unknown> = {
				destination_space_id: selectedDest,
				input_text: inputText.trim(),
				output_format: outputFormat
			};
			res = await api.post<CvFormatSession>('/cv-formatter/format', body);
		}
		submitting = false;
		if (!res.error && res.data) {
			session = res.data;
			pollSession(res.data.id);
		}
	}

	function onFileSelect(e: Event) {
		const input = e.target as HTMLInputElement;
		fileError = '';
		if (!input.files || input.files.length === 0) {
			uploadedFile = null;
			return;
		}
		const file = input.files[0];
		// Validate file type.
		const allowedTypes = [
			'application/pdf',
			'application/vnd.openxmlformats-officedocument.wordprocessingml.document'
		];
		const allowedExts = ['.pdf', '.docx'];
		const ext = file.name.toLowerCase().slice(file.name.lastIndexOf('.'));
		if (!allowedTypes.includes(file.type) && !allowedExts.includes(ext)) {
			fileError = t('cv_formatter.file_type_error', 'Only PDF and DOCX files are accepted.');
			uploadedFile = null;
			return;
		}
		// Validate file size (10 MB).
		if (file.size > 10 * 1024 * 1024) {
			fileError = t('cv_formatter.file_size_error', 'File exceeds 10 MB limit.');
			uploadedFile = null;
			return;
		}
		uploadedFile = file;
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

	// Autocomplete fetchers — filter the available destinations client-side.
	function fetchDestinations(q: string): Promise<AutocompleteItem[]> {
		const query = q.toLowerCase();
		const filtered = query === ''
			? availableDestinations
			: availableDestinations.filter((d) => d.name.toLowerCase().includes(query));
		return Promise.resolve(filtered.map((d) => ({ id: d.id, label: d.name })));
	}
	function fetchDestinationsInitial(): Promise<AutocompleteItem[]> {
		return fetchDestinations('');
	}
	const selectedDestLabel = $derived(
		selectedDest ? availableDestinations.find((d) => d.id === selectedDest)?.name ?? null : null
	);
</script>

<svelte:head>
	<title>{title}</title>
	<meta name="description" content={description} />
	<meta property="og:title" content={title} />
	<meta property="og:description" content={description} />
	<meta property="og:type" content="website" />
	<meta property="og:url" content={page.url.href} />
	<meta name="twitter:card" content="summary" />
	<meta name="twitter:title" content={title} />
	<meta name="twitter:description" content={description} />
</svelte:head>

<div class="mx-auto w-full max-w-3xl">
	<div class="border-b border-[var(--color-border)] px-4 py-3">
		<h1 class="flex items-center gap-2 text-lg font-semibold text-[var(--color-text)]">
			<FileText size={20} /> {title}
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
	{:else if destinations.length === 0}
		<div class="p-4">
			<EmptyState
				icon={FileText}
				title={t('cv_formatter.no_destinations', 'No destinations available')}
				body={t(
					'cv_formatter.no_destinations_desc',
					'Destination data has not been configured yet. Please check back later.'
				)}
			/>
		</div>
	{:else if availableDestinations.length === 0}
		<div class="p-4">
			<EmptyState
				icon={FileText}
				title={t('cv_formatter.no_standards', 'No CV standards available')}
				body={t(
					'cv_formatter.no_standards_desc',
					'CV standards have not been configured for any destination yet. Please check back later.'
				)}
			/>
		</div>
	{:else}
		<!-- Destination selector (filtered to destinations with a standard) -->
		<div class="border-b border-[var(--color-border)] px-4 py-3">
			<label for="cvf-dest" class="block text-xs font-medium uppercase tracking-wide text-[var(--color-muted)]">
				{t('cv_formatter.select_destination', 'Destination')}
			</label>
			<div class="mt-2">
				<Autocomplete
					placeholder={t('cv_formatter.select_destination_placeholder', 'Select destination country')}
					emptyMessage={t('common.no_results', 'No results found')}
					fetcher={fetchDestinations}
					initialFetcher={fetchDestinationsInitial}
					selectedId={selectedDest || null}
					selectedLabel={selectedDestLabel}
					onSelect={(item) => {
						selectedDest = item.id;
						onDestChange(item.id);
					}}
					onClear={() => {
						selectedDest = '';
						onDestChange('');
					}}
				/>
			</div>
		</div>

		{#if selectedDest}
			<!-- CV Standard display -->
			{#if loadingStandard}
				<div class="p-4">
					<LoadingSkeleton variant="lines" count={3} />
				</div>
			{:else if currentStandard}
				<div class="border-b border-[var(--color-border)] px-4 py-4 space-y-4">
					<h2 class="text-xs font-medium uppercase tracking-wide text-[var(--color-muted)]">
						{t('cv_formatter.standard_for', 'CV standard for')} {currentStandard.destination_space_name ?? ''}
					</h2>

					<div class="grid grid-cols-1 sm:grid-cols-3 gap-4">
						{#if currentStandard.recommended_length_pages}
							<div>
								<p class="text-xs text-[var(--color-muted)] mb-0.5">{t('cv_formatter.length', 'Recommended length')}</p>
								<p class="text-sm font-medium text-[var(--color-text)]">
									{currentStandard.recommended_length_pages}
								</p>
							</div>
						{/if}
						{#if currentStandard.photo_policy}
							<div>
								<p class="text-xs text-[var(--color-muted)] mb-0.5">{t('cv_formatter.photo_policy', 'Photo policy')}</p>
								<p class="text-sm font-medium text-[var(--color-text)]">
									{currentStandard.photo_policy}
								</p>
							</div>
						{/if}
						{#if currentStandard.date_format}
							<div>
								<p class="text-xs text-[var(--color-muted)] mb-0.5">{t('cv_formatter.date_format', 'Date format')}</p>
								<p class="text-sm font-medium text-[var(--color-text)]">
									{currentStandard.date_format}
								</p>
							</div>
						{/if}
					</div>

					{#if currentStandard.recommended_sections && currentStandard.recommended_sections.length > 0}
						<div>
							<p class="text-xs text-[var(--color-muted)] mb-1.5">{t('cv_formatter.recommended_sections', 'Recommended sections')}</p>
							<div class="flex flex-wrap gap-2">
								{#each currentStandard.recommended_sections as section (section)}
									<span class="px-2.5 py-1 bg-[var(--color-surface-subtle)] text-[var(--color-text)] text-xs rounded-full">
										{section}
									</span>
								{/each}
							</div>
						</div>
					{/if}

					{#if currentStandard.tone_guidance}
						<div>
							<p class="text-xs text-[var(--color-muted)] mb-1">{t('cv_formatter.tone_guidance', 'Tone guidance')}</p>
							<p class="text-sm text-[var(--color-text)] leading-relaxed">{currentStandard.tone_guidance}</p>
						</div>
					{/if}

					{#if currentStandard.common_pitfalls}
						<div class="rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface-subtle)] p-3">
							<p class="text-xs font-medium mb-1 flex items-center gap-1 text-[var(--color-text)]">
								<AlertTriangle size={14} />
								{t('cv_formatter.common_pitfalls', 'Common pitfalls')}
							</p>
							<p class="text-sm text-[var(--color-text)] leading-relaxed">{currentStandard.common_pitfalls}</p>
						</div>
					{/if}
				</div>
			{/if}

			<!-- CV input + format -->
			<div class="border-b border-[var(--color-border)] px-4 py-4">
				<h2 class="text-xs font-medium uppercase tracking-wide text-[var(--color-muted)] mb-3">
					{t('cv_formatter.input', 'Paste your CV')}
				</h2>

				<div class="space-y-3">
					<div class="flex items-center gap-3">
						<label for="output-format" class="text-sm font-medium text-[var(--color-text)] whitespace-nowrap">
							{t('cv_formatter.output_format', 'Format:')}
						</label>
						<select
							id="output-format"
							bind:value={outputFormat}
							class="px-3 py-1.5 bg-[var(--color-surface)] border border-[var(--color-border)] rounded-[var(--radius)] text-sm text-[var(--color-text)] focus:border-[var(--color-primary)] focus:outline-none"
						>
							<option value="docx">{t('cv_formatter.format_docx', 'DOCX')}</option>
							<option value="pdf">{t('cv_formatter.format_pdf', 'PDF')}</option>
						</select>
					</div>

					<!-- Input mode toggle: paste text or upload file -->
					<div class="flex items-center gap-2 mb-3">
						<button
							type="button"
							onclick={() => (inputMode = 'text')}
							class="px-3 py-1.5 text-sm rounded-[var(--radius)] transition-colors {inputMode === 'text'
								? 'bg-[var(--color-primary)] text-[var(--color-primary-contrast)]'
								: 'bg-[var(--color-surface)] text-[var(--color-text)] border border-[var(--color-border)]'}"
						>
							{t('cv_formatter.mode_text', 'Paste Text')}
						</button>
						<button
							type="button"
							onclick={() => (inputMode = 'file')}
							class="px-3 py-1.5 text-sm rounded-[var(--radius)] transition-colors {inputMode === 'file'
								? 'bg-[var(--color-primary)] text-[var(--color-primary-contrast)]'
								: 'bg-[var(--color-surface)] text-[var(--color-text)] border border-[var(--color-border)]'}"
						>
							<Upload size={14} class="inline mr-1" />
							{t('cv_formatter.mode_file', 'Upload File')}
						</button>
					</div>

					{#if inputMode === 'text'}
						<textarea
							bind:value={inputText}
							rows="10"
							placeholder={t(
								'cv_formatter.input_placeholder',
								'Paste your current CV/resume text here…'
							)}
							class="w-full px-4 py-3 bg-[var(--color-surface)] border border-[var(--color-border)] rounded-[var(--radius)] text-sm text-[var(--color-text)] focus:border-[var(--color-primary)] focus:outline-none resize-y font-mono"
						></textarea>
					{:else}
						<div class="w-full px-4 py-6 bg-[var(--color-surface)] border border-[var(--color-border)] rounded-[var(--radius)] text-center">
							<Upload size={32} class="mx-auto text-[var(--color-muted)] mb-2" />
							<label for="cvf-file" class="cursor-pointer text-sm text-[var(--color-primary)] hover:underline">
								{t('cv_formatter.choose_file', 'Choose a PDF or DOCX file')}
							</label>
							<input
								id="cvf-file"
								type="file"
								accept=".pdf,.docx,application/pdf,application/vnd.openxmlformats-officedocument.wordprocessingml.document"
								onchange={onFileSelect}
								class="hidden"
							/>
							{#if uploadedFile}
								<p class="mt-2 text-sm text-[var(--color-text)]">
									<FileText size={14} class="inline mr-1" />
									{uploadedFile.name}
									<span class="text-[var(--color-muted)]">
										({(uploadedFile.size / 1024).toFixed(0)} KB)
									</span>
								</p>
							{/if}
							{#if fileError}
								<p class="mt-2 text-sm text-[var(--color-error)]">{fileError}</p>
							{/if}
							<p class="mt-2 text-xs text-[var(--color-muted)]">
								{t('cv_formatter.file_hint', 'Max 10 MB. Text is extracted on the server.')}
							</p>
						</div>
					{/if}

					<div class="flex items-center justify-between">
						<p class="text-xs text-[var(--color-muted)]">
							{#if inputMode === 'text'}
								{inputText.length.toLocaleString()} {t('cv_formatter.characters', 'characters')}
							{:else if uploadedFile}
								{uploadedFile.name}
							{/if}
						</p>
						<button
							type="button"
							onclick={formatCv}
							disabled={(inputMode === 'text' ? !inputText.trim() : !uploadedFile) || submitting || isProcessing}
							class="inline-flex items-center gap-2 px-5 py-2 bg-[var(--color-primary)] text-[var(--color-primary-contrast)] rounded-[var(--radius)] text-sm font-medium hover:bg-[var(--color-primary-hover)] transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
						>
							{#if submitting}
								<Loader2 size={16} class="animate-spin" />
								{t('cv_formatter.submitting', 'Submitting…')}
							{:else if isProcessing}
								<Loader2 size={16} class="animate-spin" />
								{t('cv_formatter.processing', 'Formatting…')}
							{:else}
								<Sparkles size={16} />
								{t('cv_formatter.format_cv', 'Format CV')}
							{/if}
						</button>
					</div>
				</div>
			</div>

			<!-- Session output -->
			{#if session}
				<div class="px-4 py-4 space-y-4">
					{#if session.status === 'pending' || session.status === 'processing'}
						<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4 flex items-center gap-3">
							<Loader2 size={20} class="animate-spin text-[var(--color-primary)]" />
							<div>
								<p class="text-sm font-medium text-[var(--color-text)]">
									{t('cv_formatter.processing', 'Formatting your CV…')}
								</p>
								<p class="text-xs text-[var(--color-muted)]">
									{t('cv_formatter.processing_desc', 'This usually takes a few seconds. Please wait.')}
								</p>
							</div>
						</div>
					{:else if session.status === 'failed'}
						<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4 flex items-start gap-3">
							<XCircle size={20} class="text-[var(--color-primary)] flex-shrink-0 mt-0.5" />
							<div>
								<p class="text-sm font-medium text-[var(--color-text)]">
									{t('cv_formatter.failed', 'Formatting failed')}
								</p>
								<p class="text-xs text-[var(--color-muted)] mt-0.5">
									{t('cv_formatter.failed_desc', 'Something went wrong during formatting. Please try again.')}
								</p>
							</div>
						</div>
					{:else if session.status === 'complete'}
						<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
							<div class="flex items-center justify-between mb-3">
								<h2 class="text-xs font-medium uppercase tracking-wide text-[var(--color-muted)]">
									{t('cv_formatter.output', 'Formatted CV')}
								</h2>
								<a
									href={downloadUrl(session.id)}
									class="inline-flex items-center gap-1.5 px-3 py-1.5 bg-[var(--color-primary)] text-[var(--color-primary-contrast)] rounded-[var(--radius)] text-sm font-medium hover:bg-[var(--color-primary-hover)] transition-colors"
								>
									<Download size={14} />
									{t('cv_formatter.download', 'Download')}
								</a>
							</div>
							{#if session.output_text}
								<pre class="text-sm text-[var(--color-text)] whitespace-pre-wrap font-mono bg-[var(--color-surface-subtle)] p-4 rounded-[var(--radius)] max-h-96 overflow-y-auto">{session.output_text}</pre>
							{:else}
								<p class="text-sm text-[var(--color-muted)]">
									{t('cv_formatter.no_output', 'No output text was generated.')}
								</p>
							{/if}
							{#if session.tokens_used}
								<p class="text-xs text-[var(--color-muted)] mt-2">
									{t('cv_formatter.tokens_used', 'Tokens used:')} {session.tokens_used}
								</p>
							{/if}
							<!-- HITL disclosure (§95, ADR-088): AI-generated output
							     must be reviewed by the user before use. -->
							<div class="mt-3 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface-subtle)] p-3 flex items-start gap-2">
								<AlertTriangle size={14} class="text-amber-600 flex-shrink-0 mt-0.5" />
								<p class="text-xs text-[var(--color-text)] leading-relaxed">
									{t('cv_formatter.hitl_disclosure', 'AI-generated output should be reviewed by a human before submission. Verify all facts, dates, and personal information for accuracy.')}
								</p>
							</div>
						</div>
					{/if}
				</div>
			{/if}

			<!-- Auth hint for unauthed users -->
			{#if !authed}
				<div class="px-4 py-4">
					<div class="mb-3 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface-subtle)] px-3 py-2 text-xs text-[var(--color-muted)]">
						{t('cv_formatter.anon_hint', 'Sign in to save your progress.')}
					</div>
					<a
						href="/login"
						class="inline-flex items-center gap-1.5 px-4 py-2 bg-[var(--color-primary)] text-[var(--color-primary-contrast)] rounded-[var(--radius)] text-sm font-medium hover:bg-[var(--color-primary-hover)] transition-colors"
					>
						{t('common.sign_in', 'Sign in')}
					</a>
				</div>
			{/if}
		{/if}
	{/if}

	{#if showRegisterPrompt}
		<div class="px-4 py-4">
			<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4 text-center">
				<div class="mx-auto mb-2 grid h-10 w-10 place-items-center rounded-full bg-[var(--color-surface-subtle)] text-[var(--color-primary)]">
					<Sparkles size={20} />
				</div>
				<p class="text-sm font-medium text-[var(--color-text)]">
					{t('register.format_cv', 'Format your CV with AI')}
				</p>
				<p class="mt-1 text-xs text-[var(--color-muted)]">
					{t(
						'register.format_cv_desc',
						'Create a free account to use AI-assisted CV formatting tailored to your destination country.'
					)}
				</p>
				<a
					href="/login"
					class="mt-3 inline-flex items-center gap-1.5 px-4 py-2 bg-[var(--color-primary)] text-[var(--color-primary-contrast)] rounded-[var(--radius)] text-sm font-medium hover:bg-[var(--color-primary-hover)] transition-colors"
				>
					{t('common.sign_in', 'Sign in')}
				</a>
			</div>
		</div>
	{/if}
</div>
