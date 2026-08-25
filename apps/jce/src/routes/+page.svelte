<script lang="ts">
	import { page } from '$app/stores';
	import { t } from '$lib/i18n/store.svelte';

	const user = $derived($page.data.user);
</script>

<svelte:head>
	<title>{t('jce.home.heading')}</title>
	<meta name="description" content={t('jce.home.meta_description')} />
</svelte:head>

<main class="mx-auto max-w-4xl p-8">
	<h1 class="text-3xl font-bold">{t('jce.home.heading')}</h1>
	<p class="mt-4 text-[var(--color-muted)]">
		{t('jce.home.tagline')}
	</p>
	{#if user}
		<p class="mt-4 text-sm text-[var(--color-muted)]">{t('jce.home.signed_in_as', undefined, { username: user.username })}</p>
	{:else}
		<p class="mt-4 text-sm text-[var(--color-muted)]">{t('jce.home.sign_in_prompt')}</p>
	{/if}

	<div class="mt-8 flex flex-wrap gap-3">
		<a href="/compare" class="inline-flex items-center rounded-[var(--radius-button)] bg-white px-4 py-2 text-sm font-medium text-black hover:bg-neutral-200">
			{t('jce.home.start_comparing')}
		</a>
		<a href="/comparisons" class="inline-flex items-center rounded-[var(--radius-button)] border border-white/20 px-4 py-2 text-sm font-medium hover:bg-white/10">
			{t('jce.home.browse_comparisons')}
		</a>
	</div>
</main>
