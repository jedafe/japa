<script lang="ts">
	import { Inbox } from 'lucide-svelte';
	import { t } from '$lib/stores/i18n.svelte';

	let {
		icon = Inbox,
		title = undefined,
		message = '',
		actionLabel = undefined,
		onaction = undefined
	}: {
		icon?: any;
		title?: string;
		message?: string;
		actionLabel?: string;
		onaction?: () => void;
	} = $props();

	const IconComponent = $derived(icon);
	const displayTitle = $derived(title ?? $t('empty.title'));
</script>

<div class="flex flex-col items-center justify-center py-16 text-center">
	<IconComponent class="mb-4 h-12 w-12" style="color: var(--color-muted)" />
	<h2 class="text-xl font-semibold">{displayTitle}</h2>
	{#if message}
		<p class="mt-2 max-w-md" style="color: var(--color-muted)">{message}</p>
	{/if}
	{#if actionLabel && onaction}
		<button
			class="mt-6 rounded-md px-4 py-2 text-sm font-medium text-white transition-colors"
			style="background-color: var(--color-primary)"
			onclick={onaction}
		>
			{actionLabel}
		</button>
	{/if}
</div>
