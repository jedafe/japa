<script lang="ts">
	import { ToolCard } from '$lib/components/ui';
	import EmptyState from '$lib/components/ui/EmptyState.svelte';
	import { getEnabledTools } from '$lib/tools/registry';
	import { buildSeoMeta, mainAppUrl } from '$lib/seo';
	import { t } from '$lib/i18n';
	import { page } from '$app/state';

	/**
	 * Tools hub — the landing page showing all identity tools as cards.
	 * Ref: §78 (Tools Web Surface), ROADMAP-TOOLS 02.01.
	 *
	 * The tool list comes from the registry filtered by the per-tool
	 * `system_settings` enabled toggles (e.g. `checklist.enabled`).
	 * Disabled tools are hidden from the hub grid. All dynamic tool data
	 * is fetched by each tool's own page from the backend API (Rule 16).
	 * Ref: conventions/system-settings-catalog.md ({tool}.enabled keys).
	 */
	let { data } = $props();

	const enabledTools = $derived(getEnabledTools(data.siteSettings ?? {}));

	const seo = $derived(
		buildSeoMeta({
			title: t('hub.seo_title', 'JapaConnect Tools — Identity tools for your japa journey'),
			description: t(
				'hub.seo_description',
				'Visa document checklist, readiness assessment, cost of living calculator, scholarship finder, remittance rate tracker, and JapaBot AI guide.'
			),
			path: '/',
			origin: page.url.origin,
			jsonLd: {
				'@context': 'https://schema.org',
				'@type': 'WebSite',
				name: 'JapaConnect Tools',
				url: `${page.url.origin}/`,
				description:
					'Identity tools for the Nigerian diaspora japa journey: visa checklist, readiness assessment, cost of living, scholarships, remittance tracker, and JapaBot.'
			}
		})
	);
</script>

<svelte:head>
	<title>{seo.title}</title>
	<meta name="description" content={seo.description} />
	<link rel="canonical" href={seo.canonical} />
	<meta property="og:title" content={seo.title} />
	<meta property="og:description" content={seo.description} />
	<meta property="og:url" content={seo.canonical} />
	<meta property="og:site_name" content={seo.siteName} />
	<meta property="og:type" content="website" />
	<meta name="twitter:card" content="summary_large_image" />
	<meta name="twitter:title" content={seo.title} />
	<meta name="twitter:description" content={seo.description} />
	{#if seo.jsonLdString}
		<script type="application/ld+json">{seo.jsonLdString}</script>
	{/if}
</svelte:head>

<div class="mx-auto max-w-[var(--max-width)] px-4 py-12 sm:py-16">
	<div class="text-center mb-12">
		<h1 class="text-3xl sm:text-4xl font-bold text-text tracking-tight">
			{t('hub.title', 'Tools for your japa journey')}
		</h1>
		<p class="mt-3 text-lg text-text-subtle max-w-2xl mx-auto">
			{t('hub.subtitle', 'Free identity tools to help you plan, prepare, and track your relocation from Nigeria. No account required to explore — sign in to save your progress.')}
		</p>
	</div>

	{#if enabledTools.length === 0}
		<EmptyState
			icon="wrench"
			title={t('hub.no_tools', 'No tools available')}
			description={t('hub.no_tools_desc', 'All tools are currently disabled. Please check back later.')}
			action={{ label: t('shell.back_to_japaconnect', 'Back to JapaConnect'), href: mainAppUrl('/') }}
		/>
	{:else}
		<div class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-5">
			{#each enabledTools as tool (tool.slug)}
				<ToolCard {tool} />
			{/each}
		</div>
	{/if}
</div>
