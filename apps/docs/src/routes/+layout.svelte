<script lang="ts">
	import '../app.css';
	import { onMount } from 'svelte';
	import { theme } from '$lib/stores/theme.svelte';
	import { i18n } from '$lib/i18n/store.svelte';
	import type { Snippet } from 'svelte';
	import type { LayoutData } from './$types';

	let { data, children }: { data: LayoutData; children: Snippet } = $props();

	// Sync the client theme store with the server-resolved value once, after
	// hydration. The initial attribute is already on <html>, so there is no flash.
	onMount(() => theme.init(data.theme));

	// Sync the i18n locale with the server-resolved cookie value.
	$effect(() => {
		i18n.setLocale(data.locale);
	});

	// Apply the primary brand color from the control panel appearance settings.
	let primaryColor = $derived(
		(data.siteSettings?.['appearance.primary_color'] as string | undefined) ?? ''
	);
	$effect(() => {
		if (primaryColor && /^#[0-9a-fA-F]{3,8}$/.test(primaryColor)) {
			document.documentElement.style.setProperty('--color-primary', primaryColor);
		}
	});
</script>

<a
	href="#main-content"
	class="sr-only focus:not-sr-only focus:absolute focus:top-2 focus:left-2 focus:z-[100] focus:rounded-[var(--radius)] focus:bg-[var(--color-primary)] focus:px-3 focus:py-1.5 focus:text-white"
>
	Skip to content
</a>

<svelte:head>
	<!-- Sitemap link (Ref: §26 ROADMAP 07.04) — Go backend serves /docs-sitemap.xml -->
	<link rel="sitemap" href="/docs-sitemap.xml" />
</svelte:head>

<div id="main-content">
	{@render children()}
</div>
