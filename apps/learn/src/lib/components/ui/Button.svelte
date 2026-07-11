<script lang="ts">
	import { Button } from 'bits-ui';
	import type { Snippet } from 'svelte';

	let {
		variant = 'primary',
		size = 'md',
		disabled = false,
		loading = false,
		type = 'button',
		class: className = '',
		onclick,
		children,
		...restProps
	}: {
		variant?: 'primary' | 'secondary' | 'ghost' | 'danger';
		size?: 'sm' | 'md' | 'lg';
		disabled?: boolean;
		loading?: boolean;
		type?: 'button' | 'submit' | 'reset';
		class?: string;
		onclick?: (e: MouseEvent) => void;
		children?: Snippet;
		[key: string]: any;
	} = $props();

	const variantClasses: Record<string, string> = {
		primary: 'bg-[var(--color-primary)] text-white hover:opacity-90',
		secondary: 'border border-[var(--color-border)] bg-[var(--color-surface)] text-[var(--color-foreground)] hover:bg-[var(--color-background)]',
		ghost: 'text-[var(--color-foreground)] hover:bg-[var(--color-surface)]',
		danger: 'bg-[var(--color-danger)] text-white hover:opacity-90'
	};

	const sizeClasses: Record<string, string> = {
		sm: 'h-8 px-3 text-xs',
		md: 'h-10 px-4 text-sm',
		lg: 'h-12 px-6 text-base'
	};
</script>

<Button.Root
	{type}
	{disabled}
	class="inline-flex items-center justify-center gap-2 rounded-lg font-medium transition-opacity disabled:opacity-50 {variantClasses[variant]} {sizeClasses[size]} {className}"
	{onclick}
	{...restProps}
>
	{#if loading}
		<svg class="h-4 w-4 animate-spin" viewBox="0 0 24 24" fill="none">
			<circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4" />
			<path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z" />
		</svg>
	{/if}
	{@render children?.()}
</Button.Root>
