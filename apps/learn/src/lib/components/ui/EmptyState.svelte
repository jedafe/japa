<script lang="ts">
	import type { Snippet } from 'svelte';
	import { t } from '$lib/i18n';

	interface Props {
		title?: string;
		description?: string;
		/** Optional action snippet (e.g. a "Browse courses" button). */
		action?: Snippet;
	}
	let { title = t('common.empty'), description, action }: Props = $props();
</script>

<div class="flex flex-col items-center justify-center gap-3 px-6 py-16 text-center">
	<p class="text-base font-medium text-[var(--color-text)]">{title}</p>
	{#if description}
		<p class="max-w-md text-sm text-[var(--color-muted)]">{description}</p>
	{/if}
	{#if action}
		<div class="mt-2">{@render action()}</div>
	{/if}
</div>
