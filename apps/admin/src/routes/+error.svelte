<script lang="ts">
	import { page } from '$app/state';
	import { t } from '$lib/stores/i18n.svelte';
</script>

<svelte:head>
	<title>{t('error.title', 'Error')} — JapaConnect Admin</title>
</svelte:head>

<div class="flex min-h-[60vh] flex-col items-center justify-center px-4 text-center">
	<h1 class="mb-2 text-6xl font-bold">
		{page.error?.code ?? t('error.title', 'Error')}
	</h1>
	<p class="mb-6 text-lg" style="color: var(--color-muted)">
		{page.error?.message ?? t('error.something_went_wrong', 'Something went wrong.')}
	</p>
	<a
		href="/"
		class="rounded-lg px-6 py-2 text-sm font-medium text-white"
		style="background-color: var(--color-primary)"
	>
		{t('error.back_to_dashboard', 'Back to Dashboard')}
	</a>
</div>
