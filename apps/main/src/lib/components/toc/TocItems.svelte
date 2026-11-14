<script lang="ts">
	/**
	 * Shared TOC link list — rail-and-marker styling (vertical line, dot per
	 * item, accent pill on the current section), rendered inside TocBlock
	 * (inline card) and the TocFloat modal. `onjump` overrides the default
	 * smooth-scroll so a dialog host can close first (Bits UI scroll-locks
	 * the body while open). Scroll position is tracked via watchTocSpy.
	 */
	import { scrollToTocItem, watchTocSpy, type TocItem } from '#lib/utils/toc.js';

	let {
		items,
		onjump
	}: {
		items: TocItem[];
		/** Custom click handler — defaults to smooth-scroll + hash sync. */
		onjump?: (e: MouseEvent, id: string) => void;
	} = $props();

	let activeId = $state('');
	$effect(() => watchTocSpy(items.map((i) => i.id), (id) => (activeId = id)));

	function handleClick(e: MouseEvent, id: string) {
		if (onjump) {
			onjump(e, id);
			return;
		}
		e.preventDefault();
		scrollToTocItem(id);
	}
</script>

<!-- Rail: markers center at 12px (link ps-1 + marker w-4), so the line
     runs at inset-inline-start 11.5px. Nested (h3+) rows indent, taking
     their markers off the rail — reads as hierarchy. -->
<ul
	class="relative space-y-1 before:absolute before:bottom-2 before:start-[11.5px] before:top-2 before:w-px before:bg-[var(--color-border)] before:content-['']"
>
	{#each items as item (item.id)}
		{@const active = item.id === activeId}
		<li class={item.level >= 3 ? 'ms-4' : ''}>
			<a
				href="#{item.id}"
				onclick={(e) => handleClick(e, item.id)}
				aria-current={active ? 'location' : undefined}
				class="flex items-center gap-3 rounded-full py-1.5 pe-4 ps-1 text-sm transition-colors {active
					? 'bg-[var(--color-primary)] font-medium text-[var(--color-primary-contrast)]'
					: 'text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]'}"
			>
				<span class="grid h-4 w-4 shrink-0 place-items-center" aria-hidden="true">
					<span
						class="rounded-full {active
							? 'h-3 w-3 border-2 border-current'
							: 'h-2 w-2 bg-[var(--color-primary)]'}"
					></span>
				</span>
				<span class="min-w-0 truncate">{item.text}</span>
			</a>
		</li>
	{/each}
</ul>
