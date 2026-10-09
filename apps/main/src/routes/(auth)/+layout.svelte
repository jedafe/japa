<script lang="ts">
	// ADR-058 4th exception: split-screen auth layout (not AppShell).
	// brandSlides comes from the root layout's siteSettings fetch
	// (GET /site/settings/public → appearance.auth_brand_slides).
	import AuthShell from '$lib/components/auth/AuthShell.svelte';
	import { parseBrandSlides } from '$lib/auth/brandSlides';
	import type { Snippet } from 'svelte';
	import type { LayoutData } from './$types';

	let { data, children }: { data: LayoutData; children: Snippet } = $props();

	// Parse the JSON array of carousel slides from the public settings.
	let brandSlides = $derived(parseBrandSlides(data.siteSettings));
</script>

<svelte:head>
	<!-- Auth pages are private — not indexed. -->
	<meta name="robots" content="noindex, nofollow" />
</svelte:head>

<AuthShell
	{brandSlides}
	logoUrl={data.siteSettings?.['appearance.logo_url'] as string | undefined}
	logoDarkUrl={data.siteSettings?.['appearance.logo_url_dark'] as string | undefined}
>
	{@render children()}
</AuthShell>
