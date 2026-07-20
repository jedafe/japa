<script lang="ts">
	import { page } from '$app/state';
	import { t } from '$lib/stores/i18n.svelte';
</script>

<svelte:head>
	<title>{t('error.title')} — JapaControl</title>
</svelte:head>

<div class="flex min-h-[60vh] flex-col items-center justify-center px-4 text-center">
	<h1 class="text-foreground mb-2 text-6xl font-bold">
		{page.error?.code ?? t('error.generic')}
	</h1>
	<p class="text-muted-foreground mb-6 text-lg">
		{page.error?.message ?? t('error.something_wrong')}
	</p>
	<a
		href="/"
		class="bg-foreground text-background rounded-lg px-6 py-2 text-sm font-medium"
	>
		{t('error.back_to_dashboard')}
	</a>
</div>
