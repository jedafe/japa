<script lang="ts">
	// MenuItem — standard row inside <Menu>. Wraps DropdownMenu.Item with
	// the app's shared item styling; use `destructive` for danger actions.
	import { DropdownMenu } from 'bits-ui';
	import type { Snippet } from 'svelte';
	import type { IconComponent } from './icon';

	interface Props {
		onSelect?: (event: Event) => void;
		/** Render as an anchor for navigation items. */
		href?: string;
		disabled?: boolean;
		/** Red danger styling for destructive actions. */
		destructive?: boolean;
		icon?: IconComponent;
		class?: string;
		children?: Snippet;
	}

	let {
		onSelect,
		href,
		disabled = false,
		destructive = false,
		icon: Icon,
		class: className = '',
		children
	}: Props = $props();

	const itemClass = $derived(
		`flex w-full cursor-pointer items-center gap-2 rounded px-3 py-2 text-sm transition data-[highlighted]:bg-[var(--color-surface-subtle,var(--muted))] disabled:opacity-50 ${
			destructive ? 'text-[var(--color-danger,#dc2626)]' : 'text-[var(--color-text,var(--foreground))]'
		} ${className}`
	);
</script>

{#if href}
	<DropdownMenu.Item {disabled} onSelect={(e) => onSelect?.(e)}>
		{#snippet child({ props })}
			<a {href} {...props} class={itemClass}>
				{#if Icon}
					<Icon class="h-4 w-4" />
				{/if}
				{@render children?.()}
			</a>
		{/snippet}
	</DropdownMenu.Item>
{:else}
	<DropdownMenu.Item {disabled} onSelect={(e) => onSelect?.(e)} class={itemClass}>
		{#if Icon}
			<Icon class="h-4 w-4" />
		{/if}
		{@render children?.()}
	</DropdownMenu.Item>
{/if}
