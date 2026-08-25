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
	let mode = $state<'generate' | 'improve' | 'extend' | 'shorten'>('generate');
	let framework = $state<'aida' | 'pas' | 'none'>('none');
	let tone = $state('professional');
	let humanizeLevel = $state('medium');
	let minWords = $state('');
	let paragraphCount = $state('');
	let headingCount = $state('');
	let minParagraphsPerHeading = $state('');
	let externalLinks = $state('');
	let internalLinks = $state('');
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

	$effect(() => {
		if (open && panelEl) {
			panelEl.focus();
		}
	});

	$effect(() => {
		if (!open) return;
		(async () => {
			const modelRes = await api.get<{ id: string; label: string }[]>('/ai/models?category=content_generation');
			console.log('AI models response', modelRes);
			if (modelRes.data && modelRes.data.length > 0) {
				models = [{ id: '', label: t('ai.body.model_default', 'Default model') }, ...modelRes.data];
			}
			const voicesRes = await api.get<{ id: string; name: string }[]>('/ai/voices?page_size=100');
			console.log('AI voices response', voicesRes);
			if (voicesRes.data) {
				voices = [
					{ id: '', label: t('ai.body.voice_default', 'Default voice') },
					...voicesRes.data.map((v) => ({ id: v.id, label: v.name }))
				];
			}
		})();
	});

	$effect(() => {
		if (open && framework === 'none') {
			const defaultFramework = DEFAULT_FRAMEWORK_BY_CONTENT_TYPE[contentType];
			if (defaultFramework) {
				framework = defaultFramework;
			}
		}
	});

	const TONES = $derived([
		{ value: 'professional', label: t('ai.body.tone_professional', 'Professional') },
		{ value: 'friendly', label: t('ai.body.tone_friendly', 'Friendly') },
		{ value: 'concise', label: t('ai.body.tone_concise', 'Concise') },
		{ value: 'persuasive', label: t('ai.body.tone_persuasive', 'Persuasive') },
		{ value: 'academic', label: t('ai.body.tone_academic', 'Academic') },
		{ value: 'travel_blogger', label: t('ai.body.tone_travel_blogger', 'Travel Blogger') },
		{ value: 'visa_guide', label: t('ai.body.tone_visa_guide', 'Visa Guide') },
		{ value: 'student_advisor', label: t('ai.body.tone_student_advisor', 'Student Advisor') },
		{ value: 'community_storyteller', label: t('ai.body.tone_community_storyteller', 'Community Storyteller') },
		{ value: 'immigration_consultant', label: t('ai.body.tone_immigration_consultant', 'Immigration Consultant') },
		{ value: 'practical', label: t('ai.body.tone_practical', 'Practical How-To') },
		{ value: 'empathetic', label: t('ai.body.tone_empathetic', 'Empathetic') },
		{ value: 'motivational', label: t('ai.body.tone_motivational', 'Motivational') },
		{ value: 'casual', label: t('ai.body.tone_casual', 'Casual') }
	]);

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
		forum_thread: 'aida'
	};

	const frameworkHint = $derived(() => {
		if (framework === 'none') return '';
		const template = t(
			'ai.body.framework_hint',
			'Using the {framework} copywriting framework to structure this {contentType}.'
		);
		return template
			.replaceAll('{framework}', framework.toUpperCase())
			.replaceAll('{contentType}', contentType);
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
		if (!isNaN(hc) && hc >= 0) opts.heading_count = hc;
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
		return Object.keys(opts).length > 1 ? opts : undefined;
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
				error = res.error.message;
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
		class="fixed right-0 top-0 z-50 h-full w-full max-w-md border-l border-[var(--color-border)] bg-[var(--color-surface)] p-4 shadow-xl"
		role="dialog"
		tabindex="-1"
		aria-modal="true"
		aria-label={t('ai.body.panel_title')}
		onkeydown={onKeydown}
	>
		<div class="flex h-full flex-col gap-3">
			<div class="flex items-center justify-between">
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

			<textarea
				bind:value={prompt}
				rows={3}
				placeholder={t('ai.body.placeholder')}
				class="w-full resize-none rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-background)] p-2 text-sm text-[var(--color-text)] placeholder:text-[var(--color-muted)] outline-none focus:border-[var(--color-primary)]"
			></textarea>

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
					<label class="text-xs text-[var(--color-muted)]" for="ai-voice">{t('ai.body.voice_label', 'Voice / Tone')}</label>
					<select
						id="ai-voice"
						bind:value={selectedVoice}
						class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] outline-none transition focus:border-[var(--color-primary)] focus:ring-2 focus:ring-[var(--color-primary)]/20"
					>
						{#each voices as option (option.id)}
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
						class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] outline-none transition focus:border-[var(--color-primary)] focus:ring-2 focus:ring-[var(--color-primary)]/20"
					/>
				</div>
				<div class="flex flex-col gap-1">
					<label class="text-xs text-[var(--color-muted)]" for="ai-headings">{t('ai.body.headings', 'Headings')}</label>
					<input
						id="ai-headings"
						type="number"
						min="0"
						bind:value={headingCount}
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
						class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] outline-none transition focus:border-[var(--color-primary)] focus:ring-2 focus:ring-[var(--color-primary)]/20"
					/>
				</div>
				<div class="flex flex-col gap-1">
					<label class="text-xs text-[var(--color-muted)]" for="ai-external-links">{t('ai.body.external_links', 'External links')}</label>
					<input
						id="ai-external-links"
						type="number"
						min="0"
						bind:value={externalLinks}
						class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] outline-none transition focus:border-[var(--color-primary)] focus:ring-2 focus:ring-[var(--color-primary)]/20"
					/>
				</div>
				<div class="flex flex-col gap-1">
					<label class="text-xs text-[var(--color-muted)]" for="ai-internal-links">{t('ai.body.internal_links', 'Internal links')}</label>
					<input
						id="ai-internal-links"
						type="number"
						min="0"
						bind:value={internalLinks}
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

			{#if error}
				<p class="text-xs text-[var(--color-danger)]">{error}</p>
			{/if}

			{#if draft}
				<div class="flex-1 overflow-y-auto rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-background)] p-3 text-sm">
					{@html draft.body_html}
				</div>

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
{/if}
