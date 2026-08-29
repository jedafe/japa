<script lang="ts">
	import { Sparkles, X, Loader2, RefreshCw, Check } from 'lucide-svelte';
	import { t } from '$lib/i18n';
	import { api } from '$lib/api/client';
	import { draftContent } from './draftApi';
	import type { DraftRequest, DraftResponse, FieldSchema, DraftOptions } from './types';

	interface Props {
		open: boolean;
		contentType: string;
		title: string;
		body: string;
		context?: Record<string, unknown>;
		fieldSchema?: FieldSchema[];
		onApply: (payload: {
			body: string;
			title?: string;
			fieldValues?: Record<string, unknown>;
		}) => void;
		onClose: () => void;
	}

	let { open, contentType, title, body, context, fieldSchema, onApply, onClose }: Props = $props();

	let prompt = $state('');
	let promptEl = $state<HTMLTextAreaElement | null>(null);
	let mode = $state<'generate' | 'improve' | 'extend' | 'shorten'>('generate');
	let framework = $state<'aida' | 'pas' | 'none'>('none');
	let frameworkManuallySet = $state(false);
	let tone = $state('professional');
	let humanizeLevel = $state('medium');
	let minWords = $state('');
	let paragraphCount = $state('');
	let headingCount = $state('0');
	let minParagraphsPerHeading = $state('');
	let externalLinks = $state('0');
	let internalLinks = $state('0');
	let includeIntro = $state(false);
	let includeConclusion = $state(false);
	let includeFAQs = $state(false);
	let includeInlineImage = $state(false);
	let includeSteps = $state(false);
	let includeKeyTakeaways = $state(false);
	let draft = $state<DraftResponse | null>(null);
	let loading = $state(false);
	let error = $state('');
	let panelEl = $state<HTMLDivElement | null>(null);
	let selectedModel = $state('');
	let selectedVoice = $state('');
	let models = $state<{ id: string; label: string }[]>([{ id: '', label: t('ai.body.model_default', 'Default model') }]);
	let voices = $state<{ id: string; label: string }[]>([{ id: '', label: t('ai.body.voice_default', 'Default voice') }]);

	function resizePrompt() {
		if (!promptEl) return;
		promptEl.style.height = 'auto';
		promptEl.style.height = promptEl.scrollHeight + 'px';
	}

	function clampToMin(value: string, min: number, fallback = ''): string {
		if (value === '') return fallback;
		const n = parseInt(value, 10);
		if (isNaN(n)) return fallback;
		return n < min ? min.toString() : value;
	}

	$effect(() => {
		if (open && panelEl) {
			panelEl.focus();
		}
	});

	$effect(() => {
		// Grow/shrink the prompt textarea as the user types.
		prompt;
		resizePrompt();
	});

	$effect(() => {
		if (!open) return;
		(async () => {
			const [modelRes, voicesRes] = await Promise.all([
				api.get<{ id: string; label: string }[]>('/ai/models?category=content_generation'),
				api.get<{ id: string; name: string }[]>('/ai/voices?page_size=5000')
			]);
			if (modelRes.data) {
				// The backend already injects a default entry with an empty id;
				// replace it with the translated default to avoid duplicate keys.
				const registered = modelRes.data.filter((m) => m.id !== '');
				models = [{ id: '', label: t('ai.body.model_default', 'Default model') }, ...registered];
			}
			if (voicesRes.data) {
				const fetched = voicesRes.data
					.filter((v) => v.id !== '')
					.map((v) => ({ id: v.id, label: v.name }));
				voices = [{ id: '', label: t('ai.body.voice_default', 'Default voice') }, ...fetched];
			}
		})();
	});

	// Reset the manual framework flag when the panel opens so the current
	// content type's best-fit default is preselected.
	$effect(() => {
		if (open) {
			frameworkManuallySet = false;
		}
	});

	// Preselect a best-fit framework for the current content type whenever the
	// panel opens or the content type changes, unless the user has manually
	// picked a framework in the current panel session.
	$effect(() => {
		if (open && !frameworkManuallySet) {
			const defaultFramework = DEFAULT_FRAMEWORK_BY_CONTENT_TYPE[contentType];
			framework = defaultFramework ?? 'none';
		}
	});

	const TONES = $derived([
		{ value: 'natural', label: t('ai.body.tone_natural', 'Natural') },
		{ value: 'friendly', label: t('ai.body.tone_friendly', 'Friendly') },
		{ value: 'witty', label: t('ai.body.tone_witty', 'Witty') },
		{ value: 'empathetic', label: t('ai.body.tone_empathetic', 'Empathetic') },
		{ value: 'direct', label: t('ai.body.tone_direct', 'Direct') },
		{ value: 'eloquent', label: t('ai.body.tone_eloquent', 'Eloquent') },
		{ value: 'professional', label: t('ai.body.tone_professional', 'Professional') },
		{ value: 'persuasive', label: t('ai.body.tone_persuasive', 'Persuasive') },
		{ value: 'motivational', label: t('ai.body.tone_motivational', 'Motivational') },
		{ value: 'promoter', label: t('ai.body.tone_promoter', 'Promoter') },
		{ value: 'influencer', label: t('ai.body.tone_influencer', 'Influencer') },
		{ value: 'marketer', label: t('ai.body.tone_marketer', 'Marketer') },
		{ value: 'concise', label: t('ai.body.tone_concise', 'Concise') },
		{ value: 'academic', label: t('ai.body.tone_academic', 'Academic') },
		{ value: 'casual', label: t('ai.body.tone_casual', 'Casual') }
	]);

	const TONE_VALUES = $derived(TONES.map((t) => t.value));

	function voiceToneFromLabel(label: string): string {
		const lower = label.toLowerCase();
		for (const value of TONE_VALUES) {
			if (value === 'natural') continue;
			if (lower.startsWith(value + ' ')) {
				return value;
			}
		}
		return 'natural';
	}

	const filteredVoices = $derived(
		[
			{ id: '', label: t('ai.body.voice_default', 'Default voice') },
			...voices
				.filter((v) => v.id !== '' && voiceToneFromLabel(v.label) === tone)
				.sort((a, b) => a.label.localeCompare(b.label))
		]
	);

	$effect(() => {
		const available = new Set(filteredVoices.map((v) => v.id));
		if (selectedVoice !== '' && !available.has(selectedVoice)) {
			selectedVoice = '';
		}
	});

	const HUMANIZE_LEVELS = $derived([
		{ value: 'off', label: t('ai.body.humanize_off', 'Off') },
		{ value: 'light', label: t('ai.body.humanize_light', 'Light') },
		{ value: 'medium', label: t('ai.body.humanize_medium', 'Medium') },
		{ value: 'heavy', label: t('ai.body.humanize_heavy', 'Heavy') },
	]);

	const FRAMEWORKS = $derived([
		{ value: 'none', label: t('ai.body.framework_none', 'No framework') },
		{ value: 'aida', label: t('ai.body.framework_aida', 'AIDA') },
		{ value: 'pas', label: t('ai.body.framework_pas', 'PAS') }
	]);

	// Default marketing/copywriting framework by content type. When the panel
	// opens for a listed type and no framework has been picked yet, the best-fit
	// framework is preselected (Rule 3: configurable defaults from code/JSON).
	const DEFAULT_FRAMEWORK_BY_CONTENT_TYPE: Record<string, 'aida' | 'pas'> = {
		blog_post: 'aida',
		comparison: 'aida',
		discussion: 'aida',
		question: 'pas',
		forum_thread: 'aida',
		guide: 'aida',
		page: 'aida',
		story: 'aida'
	};

	const frameworkHint = $derived(() => {
		if (framework === 'none') return '';
		const template = t(
			'ai.body.framework_hint',
			'Using the {framework} copywriting framework to structure this {contentType}.'
		);
		const contentTypeLabel = t('content_type.' + contentType, contentType);
		return template
			.replaceAll('{framework}', framework.toUpperCase())
			.replaceAll('{contentType}', contentTypeLabel);
	});

	const MODES = $derived([
		{ value: 'generate', label: t('ai.body.mode_generate', 'Generate') },
		{ value: 'improve', label: t('ai.body.mode_improve', 'Improve') },
		{ value: 'extend', label: t('ai.body.mode_extend', 'Extend') },
		{ value: 'shorten', label: t('ai.body.mode_shorten', 'Shorten') },
	]);

	const humanize = $derived(humanizeLevel !== 'off');
	const actionLabel = $derived(() => {
		switch (mode) {
			case 'improve':
				return t('ai.body.improve', 'Improve');
			case 'extend':
				return t('ai.body.extend', 'Extend');
			case 'shorten':
				return t('ai.body.shorten', 'Shorten');
			default:
				return t('ai.body.generate', 'Generate');
		}
	});

	function buildOptions(): DraftOptions | undefined {
		const opts: DraftOptions = { mode };
		if (framework && framework !== 'none') opts.framework = framework;
		const mw = parseInt(minWords, 10);
		if (!isNaN(mw) && mw > 0) opts.min_words = mw;
		const pc = parseInt(paragraphCount, 10);
		if (!isNaN(pc) && pc > 0) opts.paragraph_count = pc;
		const hc = parseInt(headingCount, 10);
		if (!isNaN(hc) && hc > 0) opts.heading_count = hc;
		const mph = parseInt(minParagraphsPerHeading, 10);
		if (!isNaN(mph) && mph > 0) opts.min_paragraphs_per_heading = mph;
		const el = parseInt(externalLinks, 10);
		if (!isNaN(el) && el > 0) opts.external_links = el;
		const il = parseInt(internalLinks, 10);
		if (!isNaN(il) && il > 0) opts.internal_links = il;
		if (includeIntro) opts.include_intro = true;
		if (includeConclusion) opts.include_conclusion = true;
		if (includeFAQs) opts.include_faqs = true;
		if (includeInlineImage) opts.include_inline_image = true;
		if (includeSteps) opts.include_steps = true;
		if (includeKeyTakeaways) opts.include_key_takeaways = true;
		return opts;
	}

	function errorMessageFromCode(code: string | undefined, fallback: string): string {
		switch (code) {
			case 'ai_error':
				return t('ai.body.error_provider', 'The AI provider failed to respond. Please try again in a moment.');
			case 'request_failed':
				return t('ai.body.error_request_failed', 'The server did not respond. Please try again.');
			case 'network_error':
				return t('ai.body.error_network', 'Connection lost. Check your network and try again.');
			case 'invalid_token':
			case 'token_revoked':
				return t('ai.body.error_session', 'Your session has expired. Please log in again.');
			case 'invalid_model':
				return t('ai.body.error_model', 'The selected model is not available. Choose a different model.');
			case 'invalid_voice':
				return t('ai.body.error_voice', 'The selected voice is not available. Choose a different voice.');
			case 'validation_error':
				return t('ai.body.error_validation', 'Please check the prompt and options and try again.');
			case 'csrf_token_invalid':
				return t('ai.body.error_csrf', 'Session expired. Refresh the page and try again.');
			default:
				return fallback;
		}
	}

	async function generate() {
		if (loading) return;
		loading = true;
		error = '';
		try {
			const req: DraftRequest = {
				content_type: contentType,
				title,
				existing_body: body,
				instructions: prompt,
				tone,
				humanize,
				humanize_level: humanizeLevel,
				field_schema: fieldSchema,
				context,
				options: buildOptions(),
				model: selectedModel || undefined,
				voice_id: selectedVoice || undefined
			};
			const res = await draftContent(req);
			if (res.error) {
				error = errorMessageFromCode(res.error.code, res.error.message || t('ai.body.error'));
			} else if (res.data) {
				draft = res.data;
			} else {
				error = t('ai.body.error');
			}
		} catch (e) {
			error = e instanceof Error ? e.message : t('ai.body.error');
		} finally {
			loading = false;
		}
	}

	function apply() {
		if (!draft) return;
		onApply({
			body: draft.body_html,
			title: draft.title,
			fieldValues: draft.field_values,
		});
		onClose();
	}

	function onKeydown(e: KeyboardEvent) {
		if (e.key === 'Escape') {
			e.preventDefault();
			onClose();
		} else if ((e.metaKey || e.ctrlKey) && e.key === 'Enter') {
			e.preventDefault();
			generate();
		}
	}
</script>

{#if open}
	<div
		class="fixed inset-0 z-40 bg-black/20"
		onclick={onClose}
		role="presentation"
		aria-hidden="true"
	></div>
	<div
		bind:this={panelEl}
		class="fixed right-0 top-0 z-50 h-full w-full max-w-md border-l border-[var(--color-border)] bg-[var(--color-surface)] shadow-xl"
		role="dialog"
		tabindex="-1"
		aria-modal="true"
		aria-label={t('ai.body.panel_title')}
		onkeydown={onKeydown}
	>
		<div class="flex h-full flex-col">
			<div class="flex shrink-0 items-center justify-between p-4">
				<h2 class="flex items-center gap-2 text-sm font-semibold text-[var(--color-text)]">
					<Sparkles size={16} class="text-[var(--color-primary)]" />
					{t('ai.body.panel_title')}
				</h2>
				<button
					type="button"
					onclick={onClose}
					class="grid h-8 w-8 place-items-center rounded-[var(--radius)] text-[var(--color-muted)] hover:bg-[var(--color-surface-subtle)]"
					aria-label={t('common.close', 'Close')}
				>
					<X size={18} />
				</button>
			</div>

			<div class="px-4">
				<textarea
					bind:this={promptEl}
					bind:value={prompt}
					rows={3}
					placeholder={t('ai.body.placeholder')}
					class="max-h-40 w-full resize-none overflow-y-auto rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-background)] p-2 text-sm text-[var(--color-text)] placeholder:text-[var(--color-muted)] outline-none focus:border-[var(--color-primary)]"
				></textarea>
			</div>

			<div class="flex min-h-0 flex-1 flex-col gap-3 overflow-y-auto p-4">
				<div class="grid grid-cols-2 gap-2">
					<div class="flex flex-col gap-1">
						<label class="text-xs text-[var(--color-muted)]" for="ai-model">{t('ai.body.model_label', 'Model')}</label>
						<select
							id="ai-model"
							bind:value={selectedModel}
							class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] outline-none transition focus:border-[var(--color-primary)] focus:ring-2 focus:ring-[var(--color-primary)]/20"
						>
							{#each models as option (option.id)}
								<option value={option.id}>{option.label}</option>
							{/each}
						</select>
					</div>
					<div class="flex flex-col gap-1">
						<label class="text-xs text-[var(--color-muted)]" for="ai-humanize">{t('ai.body.humanize_label')}</label>
						<select
							id="ai-humanize"
							bind:value={humanizeLevel}
							class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] outline-none transition focus:border-[var(--color-primary)] focus:ring-2 focus:ring-[var(--color-primary)]/20"
						>
							{#each HUMANIZE_LEVELS as option (option.value)}
								<option value={option.value}>{option.label}</option>
							{/each}
						</select>
					</div>
				</div>

				<div class="grid grid-cols-2 gap-2">
					<div class="flex flex-col gap-1">
						<label class="text-xs text-[var(--color-muted)]" for="ai-tone">{t('ai.body.tone_label')}</label>
						<select
							id="ai-tone"
							bind:value={tone}
							class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] outline-none transition focus:border-[var(--color-primary)] focus:ring-2 focus:ring-[var(--color-primary)]/20"
						>
							{#each TONES as option (option.value)}
								<option value={option.value}>{option.label}</option>
							{/each}
						</select>
					</div>
					<div class="flex flex-col gap-1">
						<label class="text-xs text-[var(--color-muted)]" for="ai-voice">{t('ai.body.voice_label', 'Voice / Tone')}</label>
						<select
							id="ai-voice"
							bind:value={selectedVoice}
							class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] outline-none transition focus:border-[var(--color-primary)] focus:ring-2 focus:ring-[var(--color-primary)]/20"
						>
							{#each filteredVoices as option (option.id)}
								<option value={option.id}>{option.label}</option>
							{/each}
						</select>
					</div>
				</div>

				<div class="flex flex-col gap-1">
					<label class="text-xs text-[var(--color-muted)]" for="ai-mode">{t('ai.body.mode_label', 'Mode')}</label>
					<select
						id="ai-mode"
						bind:value={mode}
						class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] outline-none transition focus:border-[var(--color-primary)] focus:ring-2 focus:ring-[var(--color-primary)]/20"
					>
						{#each MODES as option (option.value)}
							<option value={option.value}>{option.label}</option>
						{/each}
					</select>
				</div>

				<div class="flex flex-col gap-1">
					<label class="text-xs text-[var(--color-muted)]" for="ai-framework">{t('ai.body.framework_label', 'Marketing framework')}</label>
					<select
						id="ai-framework"
						bind:value={framework}
						onchange={() => (frameworkManuallySet = true)}
						class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] outline-none transition focus:border-[var(--color-primary)] focus:ring-2 focus:ring-[var(--color-primary)]/20"
					>
						{#each FRAMEWORKS as option (option.value)}
							<option value={option.value}>{option.label}</option>
						{/each}
					</select>
					{#if frameworkHint()}
						<p class="text-[10px] text-[var(--color-muted)]">{frameworkHint()}</p>
					{/if}
				</div>

				<div class="grid grid-cols-2 gap-2">
					<div class="flex flex-col gap-1">
						<label class="text-xs text-[var(--color-muted)]" for="ai-min-words">{t('ai.body.min_words', 'Min words')}</label>
						<input
							id="ai-min-words"
							type="number"
							min="0"
							bind:value={minWords}
							onchange={() => (minWords = clampToMin(minWords, 0))}
							class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] outline-none transition focus:border-[var(--color-primary)] focus:ring-2 focus:ring-[var(--color-primary)]/20"
						/>
					</div>
					<div class="flex flex-col gap-1">
						<label class="text-xs text-[var(--color-muted)]" for="ai-paragraphs">{t('ai.body.paragraphs', 'Paragraphs')}</label>
						<input
							id="ai-paragraphs"
							type="number"
							min="0"
							bind:value={paragraphCount}
							onchange={() => (paragraphCount = clampToMin(paragraphCount, 0))}
							class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] outline-none transition focus:border-[var(--color-primary)] focus:ring-2 focus:ring-[var(--color-primary)]/20"
						/>
					</div>
					<div class="flex flex-col gap-1">
						<label class="text-xs text-[var(--color-muted)]" for="ai-headings">{t('ai.body.headings', 'Min headings')}</label>
						<input
							id="ai-headings"
							type="number"
							min="0"
							bind:value={headingCount}
							onchange={() => (headingCount = clampToMin(headingCount, 0, '0'))}
							class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] outline-none transition focus:border-[var(--color-primary)] focus:ring-2 focus:ring-[var(--color-primary)]/20"
						/>
					</div>
					<div class="flex flex-col gap-1">
						<label class="text-xs text-[var(--color-muted)]" for="ai-min-paragraphs-heading">{t('ai.body.min_paragraphs_per_heading', 'Min paragraphs per heading')}</label>
						<input
							id="ai-min-paragraphs-heading"
							type="number"
							min="0"
							bind:value={minParagraphsPerHeading}
							onchange={() => (minParagraphsPerHeading = clampToMin(minParagraphsPerHeading, 0))}
							class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] outline-none transition focus:border-[var(--color-primary)] focus:ring-2 focus:ring-[var(--color-primary)]/20"
						/>
					</div>
					<div class="flex flex-col gap-1">
						<label class="text-xs text-[var(--color-muted)]" for="ai-external-links">{t('ai.body.external_links', 'Min external links')}</label>
						<input
							id="ai-external-links"
							type="number"
							min="0"
							bind:value={externalLinks}
							onchange={() => (externalLinks = clampToMin(externalLinks, 0, '0'))}
							class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] outline-none transition focus:border-[var(--color-primary)] focus:ring-2 focus:ring-[var(--color-primary)]/20"
						/>
					</div>
					<div class="flex flex-col gap-1">
						<label class="text-xs text-[var(--color-muted)]" for="ai-internal-links">{t('ai.body.internal_links', 'Min internal links')}</label>
						<input
							id="ai-internal-links"
							type="number"
							min="0"
							bind:value={internalLinks}
							onchange={() => (internalLinks = clampToMin(internalLinks, 0, '0'))}
							class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] outline-none transition focus:border-[var(--color-primary)] focus:ring-2 focus:ring-[var(--color-primary)]/20"
						/>
					</div>
				</div>

				<p class="text-[10px] text-[var(--color-muted)]">
					{t('ai.body.headings_note', 'Headings count excludes introduction, conclusion, FAQs, and key takeaways.')}
				</p>

				<div class="grid grid-cols-2 gap-x-3 gap-y-2 text-sm text-[var(--color-text)]">
					<label class="flex items-center gap-2">
						<input type="checkbox" bind:checked={includeIntro} class="rounded border-[var(--color-border)]" />
						{t('ai.body.include_intro', 'Introduction')}
					</label>
					<label class="flex items-center gap-2">
						<input type="checkbox" bind:checked={includeConclusion} class="rounded border-[var(--color-border)]" />
						{t('ai.body.include_conclusion', 'Conclusion')}
					</label>
					<label class="flex items-center gap-2">
						<input type="checkbox" bind:checked={includeFAQs} class="rounded border-[var(--color-border)]" />
						{t('ai.body.include_faqs', 'FAQs')}
					</label>
					<label class="flex items-center gap-2">
						<input type="checkbox" bind:checked={includeInlineImage} class="rounded border-[var(--color-border)]" />
						{t('ai.body.include_inline_image', 'Inline image')}
					</label>
					<label class="flex items-center gap-2">
						<input type="checkbox" bind:checked={includeSteps} class="rounded border-[var(--color-border)]" />
						{t('ai.body.include_steps', 'Steps')}
					</label>
					<label class="flex items-center gap-2">
						<input type="checkbox" bind:checked={includeKeyTakeaways} class="rounded border-[var(--color-border)]" />
						{t('ai.body.include_key_takeaways', 'Key takeaways')}
					</label>
				</div>

				{#if draft}
					<div class="min-h-[12rem] rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-background)] p-3 text-sm">
						{@html draft.body_html}
					</div>
				{/if}
			</div>

			<div class="shrink-0 border-t border-[var(--color-border)] bg-[var(--color-surface)] p-4">
				<div class="flex flex-col gap-2">
					{#if error}
						<p class="text-xs text-[var(--color-danger)]">{error}</p>
					{/if}

					{#if !draft}
						<button
							type="button"
							disabled={loading}
							onclick={generate}
							class="inline-flex w-full items-center justify-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)] disabled:opacity-50"
						>
							{#if loading}<Loader2 size={14} class="animate-spin" />{/if}
							{actionLabel()}
						</button>
					{/if}

					{#if draft}
						<div class="flex gap-2">
							<button
								type="button"
								disabled={loading}
								onclick={apply}
								class="inline-flex flex-1 items-center justify-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)] disabled:opacity-50"
							>
								<Check size={14} /> {t('ai.body.apply')}
							</button>
							<button
								type="button"
								disabled={loading}
								onclick={generate}
								class="inline-flex flex-1 items-center justify-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-4 py-2 text-sm font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)] disabled:opacity-50"
							>
								<RefreshCw size={14} /> {t('ai.body.regenerate', 'Regenerate')}
							</button>
						</div>
					{/if}
				</div>
			</div>
		</div>
	</div>
{/if}
