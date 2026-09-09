<script lang="ts">
	import { Sparkles } from 'lucide-svelte';
	import { t } from '$lib/i18n';

	interface Props {
		disabled?: boolean;
		onclick?: () => void;
		class?: string;
		/** Render as an icon-only floating action button. */
		floating?: boolean;
	}

	let { disabled = false, onclick, class: klass = '', floating = false }: Props = $props();
</script>

{#if floating}
	<button
		type="button"
		{disabled}
		onclick={() => onclick?.()}
		class="grid h-12 w-12 place-items-center rounded-full border border-[var(--color-border)] bg-[var(--color-surface)] text-[var(--color-text)] shadow-lg transition-all hover:bg-[var(--color-surface-subtle)] hover:shadow-xl hover:scale-105 active:scale-95 disabled:opacity-50 {klass}"
		aria-label={t('ai.body.button_label', 'japa AI')}
		title={t('ai.body.button_label', 'japa AI')}
	>
		<Sparkles size={22} class="text-[var(--color-primary)]" />
	</button>
{:else}
	<button
		type="button"
		{disabled}
		onclick={() => onclick?.()}
		class="inline-flex min-h-11 min-w-11 items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)] disabled:opacity-50 {klass}"
	>
		<Sparkles size={16} class="text-[var(--color-primary)]" />
		{t('ai.body.button_label', 'japa AI')}
	</button>
{/if}
