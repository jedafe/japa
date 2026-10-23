<script lang="ts">
	// EmptyState — shared empty-state block for all JapaConnect apps.
	// Union of the previous per-app variants:
	// - `icon` accepts a Svelte component (lucide) or a kebab-case Lucide name.
	// - `description`/`body` are aliases — either renders the muted subtext.
	// - `action` accepts a {label, href?, onclick?} object (renders a link or
	//   button) or a Snippet (rendered verbatim).
	import { Inbox } from 'lucide-svelte';
	import type { Snippet } from 'svelte';
	import { resolveLucideIcon, type IconComponent } from './icon';

	interface ActionLink {
		label: string;
		href?: string;
		onclick?: () => void;
	}

	interface Props {
		/** Lucide icon component or kebab-case icon name; defaults to an inbox. */
		icon?: IconComponent | string;
		/** Extra classes applied to the icon (e.g. `rtl:-scale-x-100`). */
		iconClass?: string;
		title?: string;
		/** Muted subtext — `body` is accepted as an alias. */
		description?: string;
		body?: string;
		/** CTA: an action descriptor or a custom snippet. */
		action?: ActionLink | Snippet;
		/** Extra content rendered below the action. */
		children?: Snippet;
		class?: string;
	}

	let {
		icon = Inbox,
		iconClass = '',
		title,
		description,
		body,
		action,
		children,
		class: className = ''
	}: Props = $props();

	const Icon = $derived(typeof icon === 'string' ? resolveLucideIcon(icon) : icon);
	const text = $derived(description ?? body);
</script>

<div class="flex flex-col items-center justify-center gap-3 px-6 py-16 text-center {className}">
	{#if Icon}
		<div
			class="grid h-14 w-14 place-items-center rounded-full bg-[var(--color-surface-subtle,var(--color-muted))]/30 text-[var(--color-muted-foreground,var(--muted-foreground,var(--color-muted)))]"
		>
			<Icon size={26} strokeWidth={1.5} class={iconClass} />
		</div>
	{/if}
	{#if title}
		<h2 class="text-lg font-medium text-[var(--color-text,var(--color-foreground))]">{title}</h2>
	{/if}
	{#if text}
		<p class="max-w-sm text-sm text-[var(--color-text-subtle,var(--color-muted-foreground,var(--color-muted)))]">{text}</p>
	{/if}
	{#if action}
		{#if typeof action === 'function'}
			{@render action()}
		{:else if action.href}
			<a
				href={action.href}
				class="mt-2 rounded-[var(--radius,var(--radius-md,8px))] bg-[var(--color-primary,var(--color-accent))] px-4 py-2 text-sm font-medium text-[var(--color-primary-contrast,var(--color-accent-foreground,#fff))] transition-colors hover:bg-[var(--color-primary-hover,var(--primary-hover,var(--color-accent)))]"
			>
				{action.label}
			</a>
		{:else}
			<button
				type="button"
				onclick={action.onclick}
				class="mt-2 rounded-[var(--radius,var(--radius-md,8px))] bg-[var(--color-primary,var(--color-accent))] px-4 py-2 text-sm font-medium text-[var(--color-primary-contrast,var(--color-accent-foreground,#fff))] transition-colors hover:bg-[var(--color-primary-hover,var(--primary-hover,var(--color-accent)))]"
			>
				{action.label}
			</button>
		{/if}
	{/if}
	{#if children}
		{@render children()}
	{/if}
</div>
