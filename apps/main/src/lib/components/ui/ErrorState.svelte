<script lang="ts">
	import { AlertTriangle } from 'lucide-svelte';
	import { t } from '$lib/stores/i18n.svelte';

	let {
		title = undefined,
		message = undefined,
		onretry = undefined
	}: {
		title?: string;
		message?: string;
		onretry?: () => void;
	} = $props();

	const displayTitle = $derived(title ?? $t('error.title'));
	const displayMessage = $derived(message ?? $t('error.message'));
</script>

<div class="flex flex-col items-center justify-center py-16 text-center">
	<AlertTriangle class="mb-4 h-12 w-12" style="color: var(--color-muted)" />
	<h2 class="text-xl font-semibold">{displayTitle}</h2>
	<p class="mt-2 max-w-md" style="color: var(--color-muted)">{displayMessage}</p>
	{#if onretry}
		<button
			class="mt-6 rounded-md px-4 py-2 text-sm font-medium text-white transition-colors"
			style="background-color: var(--color-primary)"
			onclick={onretry}
		>
			{$t('error.try_again')}
		</button>
	{/if}
</div>
