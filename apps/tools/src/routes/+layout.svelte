<script lang="ts">
	import { type Snippet } from 'svelte';
	import { env } from '$env/dynamic/public';
	import HubHeader from '$lib/components/shell/HubHeader.svelte';
	import SingleToolHeader from '$lib/components/shell/SingleToolHeader.svelte';
	import Footer from '$lib/components/shell/Footer.svelte';
	import { theme } from '$lib/stores/theme.svelte';
	import { seedCsrfToken } from '$lib/api/csrf.svelte';
	import { getTool } from '$lib/tools/registry';
	import type { ToolSurface } from '$lib/types';
	import type { LayoutData } from './$types';
	import '../app.css';

	let { data, children }: { data: LayoutData; children: Snippet } = $props();

	// Seed the client stores from the SSR-resolved values once, after hydration.
	$effect(() => {
		theme.init(data.theme);
	});

	// Seed the i18n locale from the SSR-resolved value (jc_locale cookie →
	// i18n.default_language setting). The store derives locale from
	// `page.data.locale` — no manual setLocale() call needed (SSR-safe).
	// Ref: §7 (i18n), ADR-013.

	// Seed the browser-side CSRF token from server-rendered page data so the
	// first client mutation already carries X-CSRF-Token without a boot fetch.
	$effect(() => {
		seedCsrfToken(data.csrfToken ?? null);
	});

	// In single-tool shell mode, the active tool is determined by the surface.
	const activeTool = $derived(
		data.toolSurface !== 'hub' ? getTool(data.toolSurface as ToolSurface) : undefined
	);
	const isSingleTool = $derived(activeTool !== undefined);

	// Umami analytics — Tools surface (§40, §78). The script is injected only
	// when both the Umami URL and the tools-specific site ID are configured.
	const umamiEnabled = $derived(
		!!env.PUBLIC_JC_UMAMI_URL && !!env.PUBLIC_JC_UMAMI_SITE_ID_TOOLS
	);
</script>

<svelte:head>
	{#if umamiEnabled}
		<script
			defer
			src="{env.PUBLIC_JC_UMAMI_URL}/script.js"
			data-website-id={env.PUBLIC_JC_UMAMI_SITE_ID_TOOLS}
		></script>
	{/if}
</svelte:head>

{#if isSingleTool && activeTool}
	<div
		class="flex flex-col min-h-screen"
		style="--tool-accent: {activeTool.accent}; --tool-accent-hover: {activeTool.accentHover};"
	>
		<SingleToolHeader tool={activeTool} />
		<main class="flex-1">
			{@render children()}
		</main>
		<Footer mode="single" tool={activeTool} />
	</div>
{:else}
	<div class="flex flex-col min-h-screen">
		<HubHeader user={data.user} logoUrl={data.logoUrl} />
		<main class="flex-1">
			{@render children()}
		</main>
		<Footer mode="hub" />
	</div>
{/if}
