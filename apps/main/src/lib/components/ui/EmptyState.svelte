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
	const displayTitle = $derived(title ?? t('empty.title'));
</script>

<div class="animate-fade-in flex flex-col items-center justify-center py-20 text-center">
	<div
		class="mb-5 flex h-16 w-16 items-center justify-center rounded-2xl"
		style="background: color-mix(in srgb, var(--color-muted) 10%, transparent)"
	>
		<IconComponent class="h-8 w-8" style="color: var(--color-muted)" />
	</div>
	<h2 class="text-xl font-semibold tracking-tight" style="color: var(--color-foreground)">{displayTitle}</h2>
	{#if message}
		<p class="mt-2 max-w-md text-sm leading-relaxed" style="color: var(--color-muted)">{message}</p>
	{/if}
	{#if actionLabel && onaction}
		<button
			class="btn btn-primary mt-6"
			onclick={onaction}
		>
			{actionLabel}
		</button>
	{/if}
</div>
