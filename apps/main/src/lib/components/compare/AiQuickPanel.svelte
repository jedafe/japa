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
	class="max-h-[60vh] overflow-y-auto rounded-[var(--radius-card)] border p-3 space-y-3 text-sm"
	style="background-color: var(--color-surface); border-color: var(--color-border); color: var(--color-text)"
	role="region"
	aria-label={t('jce.compare.ai_quick.button')}
>
	<div class="flex items-center justify-between gap-2">
		<span class="text-xs font-medium" style="color: var(--color-muted)">
			{t('jce.compare.ai_quick.button')}
		</span>
		<button
			type="button"
			onclick={onClose}
			class="rounded-full p-0.5"
			style="color: var(--color-muted)"
			aria-label={t('jce.common.close')}
		>
			<X size={14} />
		</button>
	</div>
	<div class="grid gap-3 sm:grid-cols-2">
		<label class="space-y-1">
			<span class="text-xs font-medium" style="color: var(--color-muted)">
				{t('jce.compare.ai_quick.focus_label')}
			</span>
			<input
				type="text"
				bind:value={focus}
				placeholder={t('jce.compare.ai_quick.focus_placeholder')}
				class="w-full rounded-[var(--radius-button)] border px-3 py-1.5 text-sm"
				style="border-color: var(--color-border); background-color: var(--color-bg); color: var(--color-text)"
			/>
		</label>
		<label class="space-y-1">
			<span class="text-xs font-medium" style="color: var(--color-muted)">
				{t('jce.compare.ai_quick.fields_label')}
			</span>
			<input
				type="text"
				bind:value={fields}
				placeholder={t('jce.compare.ai_quick.fields_placeholder')}
				class="w-full rounded-[var(--radius-button)] border px-3 py-1.5 text-sm"
				style="border-color: var(--color-border); background-color: var(--color-bg); color: var(--color-text)"
			/>
		</label>
	</div>
	<div class="flex items-center gap-3 flex-wrap">
		<label class="inline-flex items-center gap-2 text-xs" style="color: var(--color-text)">
			<input type="checkbox" bind:checked={createPost} />
			{t('jce.compare.ai_quick.create_post')}
		</label>
		<button
			type="button"
			disabled={loading}
			onclick={() => onRun({ focus, fields, createPost })}
			class="inline-flex items-center gap-2 px-4 py-1.5 rounded-[var(--radius-button)] text-sm font-medium disabled:opacity-50"
			style="background-color: var(--color-bg); border: 1px solid var(--color-border); color: var(--color-text)"
		>
			{loading ? t('jce.compare.ai_quick.running') : t('jce.compare.ai_quick.run')}
		</button>
	</div>

	{#if error}
		<p class="text-xs" style="color: #ef4444">{error}</p>
	{/if}

	{#if result}
		<div class="space-y-2">
			<div class="flex items-center gap-2 flex-wrap">
				<span
					class="inline-flex items-center rounded-full border px-2 py-0.5 text-xs font-medium"
					style="border-color: var(--color-border); color: var(--color-text)"
				>
					{t(`jce.compare.ai_quick.tier.${result.tier}`, result.tier)}
				</span>
				{#if result.type_name}
					<span class="text-xs" style="color: var(--color-muted)">{result.type_name}</span>
				{/if}
			</div>

			{#if result.existing_dimensions?.length}
				<div class="flex flex-wrap gap-1.5">
					{#each result.existing_dimensions as dim (dim)}
						<span
							class="rounded-full px-2 py-0.5 text-xs"
							style="background-color: var(--color-bg); color: var(--color-text)"
						>{dim}</span>
					{/each}
				</div>
			{/if}

			{#if result.missing_dimensions?.length}
				<p class="text-xs" style="color: var(--color-muted)">
					{t('jce.compare.ai_quick.missing_dimensions')}:
					{result.missing_dimensions.join(', ')}
				</p>
			{/if}

			{#if result.presearch_level || result.presearch_summary}
				<p class="text-xs" style="color: var(--color-muted)">
					{t('jce.compare.ai_quick.presearch')}:
					{result.presearch_summary ?? result.presearch_level}
				</p>
			{/if}

			{#if result.content_id}
				<p class="text-xs" style="color: var(--color-text)">
					{t('jce.compare.ai_quick.draft_created')}
					{#if result.comparison?.comparison?.slug}
						<a
							href="/compare/{result.comparison.comparison.slug}"
							class="underline"
							style="color: var(--color-accent, currentColor)"
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
