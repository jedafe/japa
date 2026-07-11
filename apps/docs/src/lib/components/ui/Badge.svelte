<script lang="ts">
	import type { Snippet } from 'svelte';

	let {
		variant = 'default',
		class: className = '',
		children
	}: {
		variant?: 'default' | 'success' | 'warning' | 'danger' | 'info';
		class?: string;
		children?: Snippet;
	} = $props();

	const variantClasses: Record<string, string> = {
		default: 'bg-[var(--color-surface)] text-[var(--color-muted)]',
		success: 'bg-[color-mix(in_srgb,var(--color-success)_15%,transparent)] text-[var(--color-success)]',
		warning: 'bg-[color-mix(in_srgb,var(--color-warning)_15%,transparent)] text-[var(--color-warning)]',
		danger: 'bg-[color-mix(in_srgb,var(--color-danger)_15%,transparent)] text-[var(--color-danger)]',
		info: 'bg-[color-mix(in srgb,var(--color-primary)_15%,transparent)] text-[var(--color-primary)]'
	};
</script>

<span class="inline-flex items-center rounded-full px-2.5 py-0.5 text-xs font-medium {variantClasses[variant]} {className}">
	{@render children?.()}
</span>
