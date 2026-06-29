<script lang="ts">
	import PageHeader from '$lib/components/PageHeader.svelte';
	import { t } from '$lib/stores/i18n.svelte';
</script>

<PageHeader title={t('queue.title')} description={t('queue.description')} />

<div class="rounded-lg border p-8 text-center" style="border-color: var(--color-border); background-color: var(--color-surface)">
	<p class="text-sm font-medium" style="color: var(--color-muted)">{t('queue.placeholder')}</p>
	<p class="mt-1 text-xs" style="color: var(--color-muted)">{t('queue.placeholder_detail')}</p>
</div>
