<script lang="ts">
	import { t } from '$lib/i18n';
</script>

<svelte:head>
	<title>{t('settings.danger.logged_out_title')}</title>
</svelte:head>

<div class="flex min-h-[60vh] flex-col items-center justify-center gap-4 px-6 text-center">
	<h1 class="text-2xl font-medium text-[var(--color-text)]">{t('settings.danger.logged_out_title')}</h1>
	<p class="max-w-md text-sm text-[var(--color-muted)]">{t('settings.danger.logged_out_body')}</p>
	<a
		href="/"
		class="mt-4 rounded-[var(--radius)] bg-[var(--color-primary)] px-6 py-2.5 text-sm font-medium text-white"
	>
		{t('settings.danger.logged_out_home')}
	</a>
</div>
