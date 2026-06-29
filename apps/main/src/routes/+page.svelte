<script lang="ts">
	import type { HealthStatus } from '$lib/types';
	import { MetadataHead, JsonLd, createWebsiteSchema } from '$lib/seo';
	import { t } from '$lib/stores/i18n.svelte';

	let { data }: { data: HealthStatus } = $props();

	const status = $derived(data.healthy ? $t('landing.operational') : $t('landing.degraded'));
	const label = $derived(data.healthy ? 'ok' : 'warn');

	const metadata = $derived({
		title: $t('nav.home'),
		description: $t('landing.tagline'),
		content_type: 'website'
	});

	const websiteSchema = $derived(createWebsiteSchema());
</script>

<MetadataHead {metadata} />
<JsonLd schema={websiteSchema} />

<main class="mx-auto max-w-3xl px-6 py-16">
	<h1 class="text-4xl font-bold tracking-tight">JapaConnect</h1>
	<p class="mt-3 text-lg" style="color: var(--color-muted)">
		{$t('landing.tagline')}
	</p>

	<section class="mt-10 rounded-lg border p-6" style="border-color: var(--color-muted)">
		<h2 class="text-xl font-semibold">{$t('landing.health')}</h2>
		<p class="mt-2" data-testid="health-status">
			<span data-testid="health-badge">{label}</span> &mdash; {status}
		</p>
	</section>
</main>