<script lang="ts">
	import PageHeader from '$lib/components/PageHeader.svelte';
	import ConfirmModal from '$lib/components/ui/ConfirmModal.svelte';
	import { api } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';

	let flushing = $state(false);
	let message = $state('');
	let messageType = $state<'success' | 'error' | ''>('');
	let showFlushConfirm = $state(false);

	async function flush() {
		flushing = true;
		message = '';
		messageType = '';
		try {
			const result = await api('/control/cache/flush', { method: 'POST' });
			if (result.ok) {
				message = t('cache.flushed');
				messageType = 'success';
			} else {
				message = result.error || t('cache.flush_error');
				messageType = 'error';
			}
		} catch {
			message = t('cache.flush_error');
			messageType = 'error';
		} finally {
			flushing = false;
		}
	}
</script>

<PageHeader title={t('cache.title')} description={t('cache.description')} />

<div class="rounded-lg border p-6" style="border-color: var(--color-border); background-color: var(--color-surface)">
	<h3 class="text-sm font-semibold">{t('cache.flush_all')}</h3>
	<p class="mt-1 text-xs" style="color: var(--color-muted)">{t('cache.flush_description')}</p>
	<div class="mt-4 flex items-center gap-3">
		<button
			class="rounded-md border px-4 py-2 text-sm font-medium transition-colors hover:bg-black/5 dark:hover:bg-white/10 disabled:opacity-50"
			style="border-color: var(--color-border)"
			disabled={flushing}
			onclick={() => (showFlushConfirm = true)}
		>
			{flushing ? t('cache.flushing') : t('cache.flush_all')}
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

<ConfirmModal
	bind:open={showFlushConfirm}
	title="Flush All Caches"
	consequence="This will clear all cached data across all namespaces."
	confirmText="FLUSH"
	onConfirm={flush}
	onCancel={() => {}}
/>
