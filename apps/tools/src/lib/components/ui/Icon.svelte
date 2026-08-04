<script lang="ts">
	import * as LucideIcons from 'lucide-svelte';

	/**
	 * Icon component — renders a Lucide icon by name.
	 * Usage: <Icon name="file-check-2" size={24} />
	 * The name is a kebab-case Lucide icon name (e.g. "arrow-left-right").
	 *
	 * Uses lucide-svelte v1 (same as the Main, Learn, Admin, Control, and Docs
	 * apps). Icons are exported as PascalCase components; we convert kebab-case
	 * → PascalCase at runtime.
	 */
	let {
		name,
		size = 20,
		class: className = '',
		strokeWidth = 2,
		...rest
	}: {
		name: string;
		size?: number;
		class?: string;
		strokeWidth?: number;
	} & Record<string, unknown> = $props();

	// Lucide exports icons as PascalCase components. Convert kebab-case → PascalCase.
	const iconName = $derived(
		name
			.split('-')
			.map((part) => part.charAt(0).toUpperCase() + part.slice(1))
			.join('')
	);

	const IconComponent = $derived(
		(LucideIcons as Record<string, unknown>)[iconName] as
			| (typeof LucideIcons)[keyof typeof LucideIcons]
			| undefined
	);
</script>

{#if IconComponent}
	{@const Cmp = IconComponent as unknown as import('svelte').Component<Record<string, unknown>>}
	<Cmp {size} class={className} {strokeWidth} {...rest} />
{/if}
