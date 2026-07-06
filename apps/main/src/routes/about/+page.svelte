<script lang="ts">
	import { t } from '$lib/stores/i18n.svelte';
</script>

<svelte:head>
	<title>{t('about.title')}</title>
	<meta name="description" content="{t('about.mission_text_1')}" />
</svelte:head>

<main class="mx-auto max-w-3xl px-6 py-16">
	<h1 class="text-3xl font-bold" style="color: var(--color-foreground)">{t('about.title')}</h1>
	<div class="mt-6 space-y-4 text-base" style="color: var(--color-muted)">
		<p>
			{t('about.mission_text_1')}
		</p>
		<p>
			{t('about.mission_text_2')}
		</p>
		<h2 class="pt-4 text-xl font-semibold" style="color: var(--color-foreground)">{t('about.offer_heading')}</h2>
		<ul class="list-inside list-disc space-y-2">
			<li>{t('about.offer_1')}</li>
			<li>{t('about.offer_2')}</li>
			<li>{t('about.offer_3')}</li>
			<li>{t('about.offer_4')}</li>
			<li>{t('about.offer_5')}</li>
		</ul>
	</div>
</main>
