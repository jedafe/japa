<script lang="ts">
	import { t } from '$lib/i18n';
	import { Filter } from 'lucide-svelte';

	interface Props {
		/** Whether the filter panel is currently open. */
		show?: boolean;
		/** Whether any filters are currently active (shows a dot). */
		active?: boolean;
		/** Accessible label / panel id for `aria-controls`. */
		controls?: string;
		/** Extra classes for the button. */
		class?: string;
	}

	let {
		show = $bindable(false),
		active = false,
		controls = '',
		class: className = ''
	}: Props = $props();
</script>

<button
	type="button"
	onclick={() => (show = !show)}
	class="inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)] {className}"
	aria-expanded={show}
	aria-controls={controls}
>
	<Filter size={16} />
	{t('common.filters', 'Filters')}
	{#if active}
		<span class="ms-0.5 h-2 w-2 rounded-full bg-[var(--color-primary)]" aria-hidden="true"></span>
	{/if}
</button>
