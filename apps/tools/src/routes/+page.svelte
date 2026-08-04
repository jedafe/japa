<script lang="ts">
	import { ToolCard } from '$lib/components/ui';
	import { TOOLS } from '$lib/tools/registry';
	import { buildSeoMeta } from '$lib/seo';
	import { page } from '$app/state';

	/**
	 * Tools hub — the landing page showing all identity tools as cards.
	 * Ref: §78 (Tools Web Surface), ROADMAP-TOOLS 02.01.
	 *
	 * The tool list comes from the registry (structural metadata — §78 defines
	 * which tools exist). All dynamic tool data is fetched by each tool's own
	 * page from the backend API (Rule 16).
	 */
	const seo = $derived(
		buildSeoMeta({
			title: 'JapaConnect Tools — Identity tools for your japa journey',
			description:
				'Visa document checklist, readiness assessment, cost of living calculator, scholarship finder, remittance rate tracker, and JapaBot AI guide.',
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
			Tools for your japa journey
		</h1>
		<p class="mt-3 text-lg text-text-subtle max-w-2xl mx-auto">
			Free identity tools to help you plan, prepare, and track your relocation from Nigeria.
			No account required to explore — sign in to save your progress.
		</p>
	</div>

	<div class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-5">
		{#each TOOLS as tool (tool.slug)}
			<ToolCard {tool} />
		{/each}
	</div>
</div>
