<script lang="ts">
	/**
	 * Shared TOC link list — rendered inside TocBlock (inline card) and the
	 * TocFloat dialog. `onjump` overrides the default smooth-scroll so a
	 * dialog host can close first (Bits UI scroll-locks the body while open).
	 */
	import { scrollToTocItem, type TocItem } from '#lib/utils/toc.js';

	let {
		items,
		onjump
	}: {
		items: TocItem[];
		/** Custom click handler — defaults to smooth-scroll + hash sync. */
		onjump?: (e: MouseEvent, id: string) => void;
	} = $props();

	function handleClick(e: MouseEvent, id: string) {
		if (onjump) {
			onjump(e, id);
			return;
		}
		e.preventDefault();
		scrollToTocItem(id);
	}
</script>

<ul class="space-y-1.5">
	{#each items as item (item.id)}
		<li class={item.level >= 3 ? 'ms-4' : ''}>
			<a
				href="#{item.id}"
				onclick={(e) => handleClick(e, item.id)}
				class="block truncate text-sm text-[var(--color-text)] transition-colors hover:text-[var(--color-primary-text)]"
			>
				{item.text}
			</a>
		</li>
	{/each}
</ul>
