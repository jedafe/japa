<script lang="ts">
	import { t } from '$lib/stores/i18n.svelte';
	import { ShieldAlert } from 'lucide-svelte';
	let { requiredPermission = '' }: { requiredPermission?: string } = $props();
</script>

<div class="flex flex-col items-center justify-center py-16 text-center">
	<ShieldAlert class="mb-4 h-12 w-12" style="color: var(--color-danger)" />
	<h2 class="text-xl font-semibold" style="color: var(--color-foreground)">{t('lib.components.AccessDenied.svelte.access_denied')}</h2>
	<p class="mt-2 text-sm" style="color: var(--color-muted)">
	{#if requiredPermission}
		{t('lib.components.AccessDenied.svelte.required_permission', 'You need the')} <code class="rounded bg-[var(--color-surface-subtle)] px-1 py-0.5 font-mono text-xs">{requiredPermission}</code> {t('lib.components.AccessDenied.svelte.permission_suffix', 'permission to access this page.')}
	{:else}
		{t('lib.components.AccessDenied.svelte.no_permission', 'You do not have permission to access this page.')}
	{/if}
	</p>
	<a href="/" class="mt-6 text-sm font-medium hover:underline" style="color: var(--color-primary)">
		{t('lib.components.AccessDenied.svelte.return_to_dashboard')}
	</a>
</div>
