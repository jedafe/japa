<script lang="ts">
	import '../app.css';
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { theme } from '$lib/stores/theme.svelte';
	import { i18n } from '$lib/i18n/store.svelte';
	import type { Snippet } from 'svelte';
	import type { LayoutData } from './$types';

	let { data, children }: { data: LayoutData; children: Snippet } = $props();

	// Sync the client theme store with the server-resolved value once, after
	// hydration. The initial attribute is already on <html>, so there is no flash.
	onMount(() => theme.init(data.theme));

	// Sync the i18n locale with the server-resolved cookie value. Updates
	// reactively when the locale cookie changes (e.g. after visiting /fr).
	$effect(() => {
		i18n.setLocale(data.locale);
	});

	// Canonical URLs for hreflang — pathname only, host is implicit.
	let pathname = $derived(page.url.pathname);
	let canonicalEn = $derived(`/en${pathname === '/' ? '' : pathname}`);
	let canonicalFr = $derived(`/fr${pathname === '/' ? '' : pathname}`);
</script>

<svelte:head>
	<link rel="alternate" hreflang="en" href={canonicalEn} />
	<link rel="alternate" hreflang="fr" href={canonicalFr} />
	<link rel="alternate" hreflang="x-default" href={canonicalEn} />
	<link rel="alternate" type="application/rss+xml" title="JapaConnect Blog" href="/blog/rss.xml" />
	<link
		rel="alternate"
		type="application/rss+xml"
		title="JapaConnect Forum"
		href="/forum/rss.xml"
	/>
</svelte:head>

<a
	href="#main-content"
	class="sr-only focus:not-sr-only focus:absolute focus:top-2 focus:left-2 focus:z-[100] focus:rounded-[var(--radius)] focus:bg-[var(--color-primary)] focus:px-3 focus:py-1.5 focus:text-white"
>
	Skip to content
</a>

<div id="main-content">
	{@render children()}
</div>
