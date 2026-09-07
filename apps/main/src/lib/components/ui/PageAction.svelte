<script lang="ts">
	/**
	 * Primary list-page action rendered inside the ListPageBar actions slot.
	 * On desktop it is a regular labelled button; on mobile it becomes a
	 * floating action button (icon only) so the slim bar stays uncluttered.
	 *
	 * Ref: ADR-058 (unified app shell list-page bar).
	 */
	import type { IconComponent } from './icons';

	interface Props {
		/** Link target — renders <a>. Omit to render a <button>. */
		href?: string;
		/** Click handler for the button variant. */
		onclick?: (e: MouseEvent) => void;
		icon: IconComponent;
		label: string;
		disabled?: boolean;
		/** Bordered secondary style for the desktop button (FAB stays primary). */
		secondary?: boolean;
		/**
		 * Stack order for pages with more than one PageAction — 0 (default)
		 * sits nearest the bottom nav; each step lifts the FAB 3.5rem.
		 */
		fabIndex?: number;
	}

	let {
		href,
		onclick,
		icon: Icon,
		label,
		disabled = false,
		secondary = false,
		fabIndex = 0
	}: Props = $props();

	const desktopClass = $derived(
		secondary
			? 'hidden items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-2 text-sm font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)] sm:inline-flex'
			: 'hidden items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-3 py-2 text-sm font-medium text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)] sm:inline-flex'
	);

	// The bar's backdrop-blur creates a containing block that would break
	// position:fixed, so the FAB is moved to <body>.
	function portal(node: HTMLElement) {
		node.ownerDocument.body.appendChild(node);
		return () => node.remove();
	}
</script>

{#if href}
	<a {href} class={desktopClass}>
		<Icon size={14} /> {label}
	</a>
{:else}
	<button type="button" {onclick} {disabled} class="{desktopClass} disabled:opacity-50">
		<Icon size={14} /> {label}
	</button>
{/if}

{#if href}
	<a
		{@attach portal}
		{href}
		aria-label={label}
		class="fixed right-4 z-40 grid h-12 w-12 place-items-center rounded-full bg-[var(--color-primary)] text-[var(--color-primary-contrast)] shadow-lg transition-colors hover:bg-[var(--color-primary-hover)] sm:hidden"
		style="bottom: calc(5rem + env(safe-area-inset-bottom, 0px) + {fabIndex * 3.5}rem)"
	>
		<Icon size={20} />
	</a>
{:else}
	<button
		{@attach portal}
		type="button"
		{onclick}
		{disabled}
		aria-label={label}
		class="fixed right-4 z-40 grid h-12 w-12 place-items-center rounded-full bg-[var(--color-primary)] text-[var(--color-primary-contrast)] shadow-lg transition-colors hover:bg-[var(--color-primary-hover)] disabled:opacity-50 sm:hidden"
		style="bottom: calc(5rem + env(safe-area-inset-bottom, 0px) + {fabIndex * 3.5}rem)"
	>
		<Icon size={20} />
	</button>
{/if}
