<script lang="ts">
	/**
	 * Slim list-page header bar in the /blog style — a bordered, blurred strip
	 * hosting UrlTabs (or a compact title) on the left and page controls
	 * (FilterToggle, primary actions) on the right.
	 *
	 * Ref: ADR-058 (unified app shell center-column tab bar).
	 */
	import type { Snippet } from 'svelte';

	interface Props {
		/** Compact title rendered on the left when the page has no tab axis. */
		title?: string;
		/** Left-side content (typically <UrlTabs>); overrides `title`. */
		children?: Snippet;
		/** Right-side controls (FilterToggle, primary action buttons/links). */
		actions?: Snippet;
	}

	let { title = '', actions, children }: Props = $props();
</script>

	<!-- relative + z-20: backdrop-blur makes this bar a stacking context, so
	     dropdowns opened by bar controls (e.g. Autocomplete) are trapped at
	     its layer. Without an explicit z-index the bar paints in normal flow
	     and positioned content below (user cards, -mt-* overlap rows) covers
	     open dropdowns. -->
<div
	class="relative z-20 flex items-stretch border-b border-[var(--color-border)] bg-[color-mix(in_srgb,var(--color-bg)_90%,transparent)] backdrop-blur"
	style="margin-inline: var(--page-gutter, 0.75rem)"
>
	<div class="min-w-0 flex-1">
		{#if children}
			{@render children()}
		{:else if title}
			<h1
				class="flex items-center px-4 text-sm font-semibold text-[var(--color-text)]"
				style="min-height: var(--nav-height)"
			>
				{title}
			</h1>
		{/if}
	</div>
	{#if actions}
		<div class="flex min-w-0 items-center gap-2 overflow-x-auto px-3" style="min-height: var(--nav-height)">
			{@render actions()}
		</div>
	{/if}
</div>
