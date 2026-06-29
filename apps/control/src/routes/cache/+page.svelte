<script lang="ts">
	import PageHeader from '$lib/components/PageHeader.svelte';
	import { api } from '$lib/api';

	let flushing = $state(false);
	let message = $state('');
	let messageType = $state<'success' | 'error' | ''>('');

	async function flush() {
		flushing = true;
		message = '';
		messageType = '';
		try {
			const result = await api('/admin/analytics/cache/flush', { method: 'POST' });
			if (result.ok) {
				message = 'All caches flushed successfully';
				messageType = 'success';
			} else {
				message = result.error || 'Failed to flush caches';
				messageType = 'error';
			}
		} catch {
			message = 'Failed to flush caches';
			messageType = 'error';
		} finally {
			flushing = false;
		}
	}
</script>

<PageHeader title="Cache Management" description="Flush all cached layers" />

<div class="rounded-lg border p-6" style="border-color: var(--color-border); background-color: var(--color-surface)">
	<h3 class="text-sm font-semibold">Flush All Caches</h3>
	<p class="mt-1 text-xs" style="color: var(--color-muted)">Clear every cached layer including pages, feeds, i18n, and search results.</p>
	<div class="mt-4 flex items-center gap-3">
		<button
			class="rounded-md border px-4 py-2 text-sm font-medium transition-colors hover:bg-black/5 dark:hover:bg-white/10 disabled:opacity-50"
			style="border-color: var(--color-border)"
			disabled={flushing}
			onclick={flush}
		>
			{flushing ? 'Flushing...' : 'Flush All Caches'}
		</button>
		{#if message}
			<span
				class="text-xs font-medium"
				style="color: {messageType === 'success' ? 'var(--color-success)' : 'var(--color-danger)'}"
			>
				{message}
			</span>
		{/if}
	</div>
</div>
