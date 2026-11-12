<script lang="ts">
	/** Category tile for the /compare landing: tinted icon chip, name,
	 *  subcategory subtitle, optional comparison count, trailing chevron. */
	import { ChevronRight } from 'lucide-svelte';
	import type { Component } from 'svelte';

	let {
		icon: Icon,
		tint,
		name,
		subtitle = '',
		count,
		href
	}: {
		icon: Component;
		tint: string;
		name: string;
		subtitle?: string;
		count?: number;
		href: string;
	} = $props();
</script>

<a
	{href}
	class="group flex items-center gap-3 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4 transition-colors hover:bg-[var(--color-surface-subtle)]"
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
			<span class="mt-0.5 line-clamp-2 block text-sm text-[var(--color-muted)]">{subtitle}</span>
		{/if}
	</span>
	<ChevronRight size={16} class="shrink-0 text-[var(--color-muted)] transition-transform group-hover:translate-x-0.5 rtl:-scale-x-100" />
</a>
