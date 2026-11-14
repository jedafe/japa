<script lang="ts">
	/**
	 * Inline collapsible "On this page" card — sits at the top of article
	 * bodies. Collapsed by default; the TocFloat FAB takes over once the
	 * reader scrolls past it.
	 */
	import { List, ChevronDown } from 'lucide-svelte';
	import { t } from '#lib/i18n/index.js';
	import TocItems from './TocItems.svelte';
	import type { TocItem } from '#lib/utils/toc.js';

	let { items }: { items: TocItem[] } = $props();

	let open = $state(false);
</script>

<nav
	class="rounded-[12px] border border-[var(--color-border)] bg-[var(--color-surface)]"
	aria-label={t('toc.on_this_page', 'On this page')}
>
	<button
		type="button"
		class="flex w-full items-center justify-between gap-2 px-4 py-2.5 text-start"
		aria-expanded={open}
		onclick={() => (open = !open)}
	>
		<span class="flex items-center gap-2 text-xs font-semibold uppercase tracking-wide text-[var(--color-muted)]">
			<List size={14} class="text-[var(--color-primary-text)]" aria-hidden="true" />
			{t('toc.on_this_page', 'On this page')}
		</span>
		<ChevronDown
			size={14}
			class="shrink-0 text-[var(--color-muted)] transition-transform duration-200 {open ? 'rotate-180' : ''}"
			aria-hidden="true"
		/>
	</button>
	{#if open}
		<div class="border-t border-[var(--color-border)] px-4 py-3">
			<TocItems {items} />
		</div>
	{/if}
</nav>
