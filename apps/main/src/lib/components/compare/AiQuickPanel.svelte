<script lang="ts">
	import { t } from '$lib/i18n';
	import type { CompareEntitiesResult } from '$lib/ai/compareApi';
	import { X } from 'lucide-svelte';

	interface Props {
		loading: boolean;
		result: CompareEntitiesResult | null;
		error: string;
		onRun: (opts: { focus: string; fields: string; createPost: boolean }) => void;
		onClose: () => void;
	}

	let { loading, result, error, onRun, onClose }: Props = $props();

	let focus = $state('');
	let fields = $state('');
	let createPost = $state(false);
</script>

<div
	class="max-h-[60vh] space-y-3 overflow-y-auto rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-3 text-sm text-[var(--color-text)]"
	role="region"
	aria-label={t('jce.compare.ai_quick.button')}
>
	<div class="flex items-center justify-between gap-2">
		<span class="text-xs font-medium text-[var(--color-muted)]">
			{t('jce.compare.ai_quick.button')}
		</span>
		<button
			type="button"
			onclick={onClose}
			class="rounded-full p-0.5 text-[var(--color-muted)] transition-colors hover:text-[var(--color-text)]"
			aria-label={t('jce.common.close')}
		>
			<X size={14} />
		</button>
	</div>
	<div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
		<label class="space-y-1">
			<span class="text-xs font-medium text-[var(--color-muted)]">
				{t('jce.compare.ai_quick.focus_label')}
			</span>
			<input
				type="text"
				bind:value={focus}
				placeholder={t('jce.compare.ai_quick.focus_placeholder')}
				class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-bg)] px-3 py-1.5 text-sm text-[var(--color-text)] placeholder:text-[var(--color-muted)] focus:border-[var(--color-primary)] focus:outline-none"
			/>
		</label>
		<label class="space-y-1">
			<span class="text-xs font-medium text-[var(--color-muted)]">
				{t('jce.compare.ai_quick.fields_label')}
			</span>
			<input
				type="text"
				bind:value={fields}
				placeholder={t('jce.compare.ai_quick.fields_placeholder')}
				class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-bg)] px-3 py-1.5 text-sm text-[var(--color-text)] placeholder:text-[var(--color-muted)] focus:border-[var(--color-primary)] focus:outline-none"
			/>
		</label>
	</div>
	<div class="flex flex-wrap items-center gap-3">
		<label class="inline-flex items-center gap-2 text-xs text-[var(--color-text)]">
			<input type="checkbox" bind:checked={createPost} />
			{t('jce.compare.ai_quick.create_post')}
		</label>
		<button
			type="button"
			disabled={loading}
			onclick={() => onRun({ focus, fields, createPost })}
			class="inline-flex items-center gap-2 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface-subtle)] px-4 py-1.5 text-sm font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-border)] disabled:opacity-50"
		>
			{loading ? t('jce.compare.ai_quick.running') : t('jce.compare.ai_quick.run')}
		</button>
	</div>

	{#if error}
		<p class="text-xs text-[var(--color-danger)]">{error}</p>
	{/if}

	{#if result}
		<div class="space-y-2">
			<div class="flex flex-wrap items-center gap-2">
				<span
					class="inline-flex items-center rounded-full border border-[var(--color-border)] px-2 py-0.5 text-xs font-medium text-[var(--color-text)]"
				>
					{t(`jce.compare.ai_quick.tier.${result.tier}`, result.tier)}
				</span>
				{#if result.type_name}
					<span class="text-xs text-[var(--color-muted)]">{result.type_name}</span>
				{/if}
			</div>

			{#if result.existing_dimensions?.length}
				<div class="flex flex-wrap gap-1.5">
					{#each result.existing_dimensions as dim (dim)}
						<span
							class="rounded-full bg-[var(--color-surface-subtle)] px-2 py-0.5 text-xs text-[var(--color-text)]"
						>{dim}</span>
					{/each}
				</div>
			{/if}

			{#if result.missing_dimensions?.length}
				<p class="text-xs text-[var(--color-muted)]">
					{t('jce.compare.ai_quick.missing_dimensions')}:
					{result.missing_dimensions.join(', ')}
				</p>
			{/if}

			{#if result.presearch_level || result.presearch_summary}
				<p class="text-xs text-[var(--color-muted)]">
					{t('jce.compare.ai_quick.presearch')}:
					{result.presearch_summary ?? result.presearch_level}
				</p>
			{/if}

			{#if result.content_id}
				<p class="text-xs text-[var(--color-text)]">
					{t('jce.compare.ai_quick.draft_created')}
					{#if result.comparison?.comparison?.slug}
						<a
							href="/compare/{result.comparison.comparison.slug}"
							class="text-[var(--color-primary)] underline"
						>
							{result.comparison.comparison.title}
						</a>
					{:else}
						<code>{result.content_id}</code>
					{/if}
				</p>
			{/if}
		</div>
	{/if}
</div>
