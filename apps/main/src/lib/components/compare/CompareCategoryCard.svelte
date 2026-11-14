<script lang="ts">
	/** Category tile for the /compare landing: tinted icon chip, name,
	 *  subcategory subtitle, optional comparison count, trailing chevron. */
	import { Check, ChevronRight } from 'lucide-svelte';
	import type { Component } from 'svelte';

	let {
		icon: Icon,
		tint,
		name,
		subtitle = '',
		count,
		href,
		selected = false,
		class: className = ''
	}: {
		icon: Component;
		tint: string;
		name: string;
		subtitle?: string;
		count?: number;
		href: string;
		selected?: boolean;
		class?: string;
	} = $props();

	// In the "All categories" dialog the tile for the workspace's current
	// category is marked selected — scroll it into view since the dialog's
	// grid overflows on narrow screens.
	function scrollSelected(node: HTMLElement) {
		if (!selected) return;
		requestAnimationFrame(() => node.scrollIntoView({ block: 'nearest' }));
	}
</script>

<a
	{href}
	aria-current={selected ? 'page' : undefined}
	{@attach scrollSelected}
	class="group flex items-center gap-3 rounded-[var(--radius-card)] border p-4 transition-colors {selected
		? 'border-[var(--color-primary)] bg-[var(--color-primary)]/5'
		: 'border-[var(--color-border)] bg-[var(--color-surface)] hover:bg-[var(--color-surface-subtle)]'} {className}"
>
	<span
		class="grid h-11 w-11 shrink-0 place-items-center rounded-[var(--radius)]"
		style="background: color-mix(in srgb, {tint} 14%, transparent); color: {tint}"
	>
		<Icon size={22} />
	</span>
	<span class="min-w-0 flex-1">
		<span class="block font-semibold text-[var(--color-text)]">
			{name}
			{#if count !== undefined}
				<span class="ms-1 text-xs font-normal text-[var(--color-muted)]">{count}</span>
			{/if}
		</span>
		{#if subtitle}
			<span class="mt-0.5 block truncate text-sm text-[var(--color-muted)]">{subtitle}</span>
		{/if}
	</span>
	{#if selected}
		<Check size={16} class="shrink-0 text-[var(--color-primary)]" />
	{:else}
		<ChevronRight size={16} class="shrink-0 text-[var(--color-muted)] transition-transform group-hover:translate-x-0.5 rtl:-scale-x-100" />
	{/if}
</a>
