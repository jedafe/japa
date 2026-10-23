<script lang="ts">
	import { t, siteName} from '$lib/i18n';
	import RecommendationsWidget from '$lib/components/hub/dashboard/RecommendationsWidget.svelte';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	const resolvedCountry = $derived(data.recommendations?.context?.content_country ?? null);
</script>

<svelte:head>
	<title>{t('dashboard.overview.recommendations')} — {siteName()}</title>
</svelte:head>

<div class="mx-auto w-full max-w-3xl px-4 py-6">
	<header class="mb-4">
		<h1 class="text-xl font-semibold text-[var(--color-text)]">
			{t('dashboard.overview.recommendations')}
		</h1>
		<p class="mt-1 text-sm text-[var(--color-muted)]">
			{#if resolvedCountry}
				{t('dashboard.overview.recommendations_personalized_for', 'Personalized for {country}', { country: resolvedCountry })}
			{:else}
				{t('dashboard.overview.recommendations_personalized_geo', 'Personalized to your location. Add ?geo=XX to preview another country.')}
			{/if}
		</p>
	</header>

	<RecommendationsWidget data={data.recommendations} />

	{#if !data.recommendations}
		<p class="mt-4 text-sm text-[var(--color-muted)]">
			{t('dashboard.overview.recommendations_unavailable', 'Recommendations are unavailable right now. Please check back soon.')}
		</p>
	{/if}
</div>
