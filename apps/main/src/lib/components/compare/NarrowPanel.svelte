<script lang="ts">
	import ScopeChain from './ScopeChain.svelte';
	import { t } from '$lib/i18n';

	interface Props {
		chain: string[];
		selections: Record<string, string>;
		labels?: Record<string, string>;
		required: boolean;
		requiredStepLabel?: string;
		requiredItemLabel?: string;
		onChange: (step: string, value: string) => void;
		onSkip: () => void;
		onDone: () => void;
		onClear: () => void;
	}

	let {
		chain,
		selections,
		labels = $bindable({}),
		required,
		requiredStepLabel = '',
		requiredItemLabel = '',
		onChange,
		onSkip,
		onDone,
		onClear
	}: Props = $props();

	let hasSelection = $derived(chain.some((step) => selections[step]));
</script>

<section
	class="space-y-3 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4"
	aria-label={t('jce.compare.scope.title')}
>
	<div class="flex items-center justify-between gap-3">
		<h2 class="text-sm font-semibold text-[var(--color-text)]">
			{#if required && requiredStepLabel}
				{t('jce.compare.narrow.required_prompt', 'Choose a {step} to see {label}.', {
					step: requiredStepLabel,
					label: requiredItemLabel || t('jce.compare.items_heading')
				})}
			{:else}
				{t('jce.compare.scope.title')}
			{/if}
		</h2>
		<div class="flex items-center gap-3">
			{#if hasSelection}
				<button type="button" onclick={onClear} class="text-xs text-[var(--color-muted)] transition-colors hover:text-[var(--color-text)]">
					{t('jce.common.clear')}
				</button>
			{/if}
			{#if !required}
				<button type="button" onclick={onSkip} class="text-xs text-[var(--color-muted)] transition-colors hover:text-[var(--color-text)]">
					{t('jce.compare.narrow.skip')}
				</button>
			{/if}
		</div>
	</div>

	<ScopeChain {chain} {selections} bind:labels onChange={(k, v) => onChange(k, v)} />

	{#if hasSelection}
		<div class="flex justify-end">
			<button
				type="button"
				onclick={onDone}
				class="rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface-subtle)] px-4 py-1.5 text-sm font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-border)]"
			>
				{t('jce.compare.narrow.done')}
			</button>
		</div>
	{/if}
</section>
