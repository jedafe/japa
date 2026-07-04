<script lang="ts">
	import { api } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { Pin, PinOff, Loader2 } from 'lucide-svelte';

	let { contentId, initiallyPinned = false }: { contentId: string; initiallyPinned?: boolean } = $props();

	// svelte-ignore state_referenced_locally
	let pinned = $state(initiallyPinned);
	let loading = $state(false);
	let permissionError = $state(false);

	async function togglePin() {
		loading = true;
		permissionError = false;
		try {
			if (pinned) {
				await api.unpinContent(contentId);
				pinned = false;
			} else {
				await api.pinContent(contentId);
				pinned = true;
			}
		} catch (err) {
			if (err instanceof Error && err.message.includes('forbidden')) {
				permissionError = true;
			}
		} finally {
			loading = false;
		}
	}
</script>

<div class="relative inline-block">
	<button
		onclick={togglePin}
		disabled={loading}
		class="inline-flex items-center gap-1.5 rounded-lg px-3 py-1.5 text-xs font-medium transition-all hover:opacity-80 disabled:opacity-50"
		style="background-color: {pinned ? 'var(--color-primary)' : 'var(--color-surface)'}; color: {pinned ? 'white' : 'var(--color-foreground)'}; border: 1px solid var(--color-border);"
		title={pinned ? t('pin.unpin') : t('pin.title')}
	>
		{#if loading}
			<Loader2 class="h-3.5 w-3.5 animate-spin" />
		{:else if pinned}
			<Pin class="h-3.5 w-3.5" />
			<span>{t('pin.pinned')}</span>
		{:else}
			<PinOff class="h-3.5 w-3.5" />
			<span>{t('pin.title')}</span>
		{/if}
	</button>

	{#if permissionError}
		<div
			class="absolute left-0 top-full z-50 mt-1 whitespace-nowrap rounded-lg border px-3 py-1.5 text-xs shadow-lg"
			style="border-color: var(--color-danger); background: var(--color-surface); color: var(--color-danger)"
		>
			{t('pin.no_permission')}
		</div>
	{/if}
</div>
