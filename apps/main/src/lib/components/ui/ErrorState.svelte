<script lang="ts">
	import { AlertTriangle } from 'lucide-svelte';

	let {
		title = 'Something went wrong',
		message = 'An unexpected error occurred. Please try again.',
		onretry = undefined
	}: {
		title?: string;
		message?: string;
		onretry?: () => void;
	} = $props();
</script>

<div class="flex flex-col items-center justify-center py-16 text-center">
	<AlertTriangle class="mb-4 h-12 w-12" style="color: var(--color-muted)" />
	<h2 class="text-xl font-semibold">{title}</h2>
	<p class="mt-2 max-w-md" style="color: var(--color-muted)">{message}</p>
	{#if onretry}
		<button
			class="mt-6 rounded-md px-4 py-2 text-sm font-medium text-white transition-colors"
			style="background-color: var(--color-primary)"
			onclick={onretry}
		>
			Try again
		</button>
	{/if}
</div>
