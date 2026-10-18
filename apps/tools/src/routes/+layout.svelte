<script lang="ts">
	import { untrack, onMount, type Snippet } from 'svelte';
	import { env } from '$env/dynamic/public';
	import { page } from '$app/state';
	import { beforeNavigate } from '$app/navigation';
	import { Sparkles } from 'lucide-svelte';
	import HubHeader from '$lib/components/shell/HubHeader.svelte';
	import SingleToolHeader from '$lib/components/shell/SingleToolHeader.svelte';
	import Footer from '$lib/components/shell/Footer.svelte';
	import { theme } from '$lib/stores/theme.svelte';
	import { seedCsrfToken } from '$lib/api/csrf.svelte';
	import { getTool } from '$lib/tools/registry';
	import { t } from '$lib/i18n';
	import { AiPanel, setAiPageContext } from '@japaconnect/ai-panel';
	import { ensureAiPanelDeps } from '$lib/ai/panel';
	import { resolveAiPageContext } from '$lib/ai/pageContext';
	import type { ToolSurface } from '$lib/types';
	import type { LayoutData } from './$types';
	import '../app.css';

	let { data, children }: { data: LayoutData; children: Snippet } = $props();

	// Seed the client stores from the SSR-resolved values once, after hydration.
	// untrack: theme.init reads reactive store state — without it the effect
	// re-fires on every user toggle and snaps the theme back to the SSR value.
	$effect(() => {
		const serverTheme = data.theme;
		untrack(() => theme.init(serverTheme));
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

	// Admin appearance overrides are injected server-side by hooks.server.ts
	// into the static <style id="jc-theme-overrides"> tag in app.html.

	// Custom favicon from appearance.favicon_url — overrides the default
	// favicon links in app.html when set.
	// Ref: conventions/system-settings-catalog.md (appearance.favicon_url)
	let faviconUrl = $derived(
		((theme.resolved === 'dark'
			? (data.siteSettings?.['appearance.favicon_url_dark'] as string | undefined)
			: (data.siteSettings?.['appearance.favicon_url_light'] as string | undefined)) ??
			(data.siteSettings?.['appearance.favicon_url'] as string | undefined) ?? '')
	);

	// Umami analytics — Tools surface (§40, §78). The script is injected only
	// when both the Umami URL and the tools-specific site ID are configured.
	const umamiEnabled = $derived(
		!!env.PUBLIC_JC_UMAMI_URL && !!env.PUBLIC_JC_UMAMI_SITE_ID_TOOLS
	);

	// §115 AI Quick Panel: FAB toggles the page-aware slide-in panel
	// (chat streams to the shared backend; "Open in Japa AI" links to the
	// main app's /ai). Deps init on every page.
	const showChatFab = $derived(data.features?.['feature.japabot'] === true);
	let aiPanelOpen = $state(false);

	$effect(() => {
		setAiPageContext(resolveAiPageContext(page.url.pathname, page.data));
	});
	beforeNavigate(() => setAiPageContext(null));

	onMount(() => ensureAiPanelDeps());
</script>

<svelte:head>
	{#if faviconUrl}
		<link rel="icon" href={faviconUrl} type="image/png" />
	{/if}
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
		<HubHeader user={data.user} logoUrl={data.logoUrl} logoDarkUrl={data.logoDarkUrl} />
		<main class="flex-1">
			{@render children()}
		</main>
		<Footer mode="hub" />
	</div>
{/if}

{#if showChatFab}
	<!-- §115 Japa AI FAB — opens the page-aware quick panel. -->
	<button
		type="button"
		onclick={() => (aiPanelOpen = !aiPanelOpen)}
		aria-label={t('ai.panel.title', 'Japa AI')}
		title={t('ai.panel.title', 'Japa AI')}
		aria-expanded={aiPanelOpen}
		class="fixed bottom-5 right-4 z-40 grid h-12 w-12 place-items-center rounded-full bg-[var(--color-primary)] text-[var(--color-primary-contrast)] shadow-lg transition-transform hover:scale-105"
	>
		<Sparkles size={22} />
	</button>
	<AiPanel open={aiPanelOpen} user={data.user ?? null} onclose={() => (aiPanelOpen = false)} />
{/if}
