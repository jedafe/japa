<script lang="ts">
	import { Sparkles, X, Loader2, RefreshCw, Check } from 'lucide-svelte';
	import { t } from '$lib/i18n';
	import { draftContent } from './draftApi';
	import type { DraftRequest, DraftResponse, FieldSchema } from './types';

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
	let tone = $state('professional');
	let humanizeLevel = $state('medium');
	let draft = $state<DraftResponse | null>(null);
	let loading = $state(false);
	let error = $state('');
	let panelEl = $state<HTMLDivElement | null>(null);

	$effect(() => {
		if (open && panelEl) {
			panelEl.focus();
		}
	});

	const TONES = $derived([
		{ value: 'professional', label: t('ai.body.tone_professional', 'Professional') },
		{ value: 'friendly', label: t('ai.body.tone_friendly', 'Friendly') },
		{ value: 'concise', label: t('ai.body.tone_concise', 'Concise') },
		{ value: 'persuasive', label: t('ai.body.tone_persuasive', 'Persuasive') },
		{ value: 'academic', label: t('ai.body.tone_academic', 'Academic') },
	]);

	const HUMANIZE_LEVELS = $derived([
		{ value: 'off', label: t('ai.body.humanize_off', 'Off') },
		{ value: 'light', label: t('ai.body.humanize_light', 'Light') },
		{ value: 'medium', label: t('ai.body.humanize_medium', 'Medium') },
		{ value: 'heavy', label: t('ai.body.humanize_heavy', 'Heavy') },
	]);

	const humanize = $derived(humanizeLevel !== 'off');

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

			<button
				type="button"
				disabled={loading}
				onclick={generate}
				class="inline-flex w-full items-center justify-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)] disabled:opacity-50"
			>
				{#if loading}<Loader2 size={14} class="animate-spin" />{/if}
				{t('ai.body.regenerate', 'Generate')}
			</button>

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
						<RefreshCw size={14} /> {t('ai.body.regenerate')}
					</button>
				</div>
			{/if}
		</div>
	</div>
{/if}
