<script lang="ts">
	import type { Snippet } from 'svelte';
	import { t } from '$lib/i18n';

	interface Props {
		titleKey: string;
		titleFallback?: string;
		actions?: Snippet;
		children: Snippet;
	}
	let { titleKey, titleFallback, actions, children }: Props = $props();
</script>

<section class="rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
	<header class="mb-3 flex items-center justify-between">
		<h2 class="text-sm font-medium text-[var(--color-text)]">{t(titleKey, titleFallback)}</h2>
		{#if actions}
			<div class="flex items-center gap-2">{@render actions()}</div>
		{/if}
	</header>
	<div>{@render children()}</div>
</section>
