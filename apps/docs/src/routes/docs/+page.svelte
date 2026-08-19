<script lang="ts">
	import { untrack } from 'svelte';
	import AppShell from '$lib/components/shell/AppShell.svelte';
	import { t } from '$lib/i18n';
	import { persistNavCollapse } from '$lib/nav-collapse';
	import { AUDIENCE_LABEL_KEY, AUDIENCE_ORDER } from '$lib/types';
	import type { DocAudience } from '$lib/types';
	import type { PageData } from './$types';
	import { ArrowRight } from 'lucide-svelte';
	import { safeJsonLdScript } from '$lib/utils/safeHtml';

	let { data }: { data: PageData } = $props();

	let collapsed = $state(untrack(() => data.navCollapsed));

	async function toggleCollapse() {
		collapsed = !collapsed;
		await persistNavCollapse(collapsed);
	}

	// Layout data (nav, accessibleAudiences, user, navCollapsed) comes via the
	// nearest layout — here the root +layout.svelte. We read it from $props.
	let layout = $derived(data);

	// First page of each accessible audience as a "popular" entry.
	let popular = $derived(
		data.nav
			.flatMap((tree) =>
				tree.items.slice(0, 3).map((item) => ({
					audience: tree.audience,
					slug: item.slug,
					title: item.title
				}))
			)
			.slice(0, 8)
	);

	function audienceHref(a: DocAudience): string {
		return `/docs/${a}`;
	}
</script>

<svelte:head>
	<title>{data.meta.title}</title>
	<meta name="description" content={data.meta.description} />
	<link rel="canonical" href={data.meta.ogUrl} />
	<meta property="og:title" content={data.meta.title} />
	<meta property="og:description" content={data.meta.description} />
	<meta property="og:type" content={data.meta.ogType} />
	<meta property="og:url" content={data.meta.ogUrl} />
	<meta name="twitter:card" content={data.meta.twitterCard} />
	<meta name="twitter:title" content={data.meta.twitterTitle} />
	<meta name="twitter:description" content={data.meta.twitterDescription} />
	{#if data.meta.jsonLdString}
		{@html safeJsonLdScript(data.meta.jsonLdString)}
	{/if}
</svelte:head>

<AppShell
	nav={data.nav}
	accessibleAudiences={data.accessibleAudiences}
	logoUrl={(data.siteSettings?.["appearance.logo_url"] as string | undefined) ?? ""}
	footerMenu={data.footerMenu}
	rightSidebarWidgets={data.rightSidebarWidgets}
	activeAudience={null}
	user={data.user}
	collapsed={collapsed}
	onToggleCollapse={toggleCollapse}
>
	<section class="landing">
		<header class="hero">
			<h1>{t('landing.title')}</h1>
			<p class="subtitle">{t('landing.subtitle')}</p>
		</header>

		<div class="audiences">
			<h2>{t('landing.audiences')}</h2>
			<div class="audience-grid">
				{#each AUDIENCE_ORDER.filter((a) => data.accessibleAudiences.includes(a)) as audience (audience)}
					<a href={audienceHref(audience)} class="audience-card">
						<div class="audience-card-body">
							<span class="audience-name">{t(AUDIENCE_LABEL_KEY[audience], audience)}</span>
							<span class="audience-count">
								{data.nav.find((n) => n.audience === audience)?.items.length ?? 0} pages
							</span>
						</div>
						<ArrowRight size={18} class="audience-arrow" />
					</a>
				{/each}
			</div>
		</div>

		{#if popular.length > 0}
			<div class="popular">
				<h2>{t('landing.popular')}</h2>
				<ul class="popular-list">
					{#each popular as p (`${p.audience}/${p.slug}`)}
						<li>
							<a href="/docs/{p.audience}/{p.slug}" class="popular-link">
								<span class="popular-title">{p.title}</span>
								<span class="popular-audience">{t(AUDIENCE_LABEL_KEY[p.audience], p.audience)}</span>
							</a>
						</li>
					{/each}
				</ul>
			</div>
		{/if}
	</section>
</AppShell>

<style>
	.landing {
		max-width: 960px;
		margin: 0 auto;
		padding: 3rem 1.5rem 4rem;
	}
	.hero { margin-bottom: 3rem; }
	.hero h1 {
		font-family: var(--font-sans);
		font-size: 2.25rem;
		font-weight: 700;
		color: var(--color-text);
		margin: 0 0 0.75rem;
		letter-spacing: -0.02em;
	}
	.subtitle {
		font-size: 1.125rem;
		color: var(--color-text-subtle);
		line-height: 1.6;
		margin: 0;
		max-width: 60ch;
	}
	.audiences { margin-bottom: 3rem; }
	.audiences h2, .popular h2 {
		font-size: 0.75rem;
		font-weight: 600;
		text-transform: uppercase;
		letter-spacing: 0.05em;
		color: var(--color-muted);
		margin: 0 0 1rem;
	}
	.audience-grid {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(220px, 1fr));
		gap: 0.75rem;
	}
	.audience-card {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 0.5rem;
		padding: 1rem 1.25rem;
		border: 1px solid var(--color-border);
		border-radius: var(--radius-card);
		background: var(--color-surface);
		text-decoration: none;
		transition: border-color 0.15s, transform 0.15s;
	}
	.audience-card:hover {
		border-color: var(--color-primary);
		transform: translateY(-1px);
	}
	.audience-card-body { display: flex; flex-direction: column; gap: 0.125rem; }
	.audience-name { font-size: 1rem; font-weight: 600; color: var(--color-text); }
	.audience-count { font-size: 0.75rem; color: var(--color-muted); }
	:global(.audience-arrow) { color: var(--color-muted); flex-shrink: 0; }
	.audience-card:hover :global(.audience-arrow) { color: var(--color-primary); }

	.popular-list {
		list-style: none;
		padding: 0;
		margin: 0;
		display: flex;
		flex-direction: column;
		gap: 0.25rem;
	}
	.popular-link {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 0.75rem;
		padding: 0.625rem 0.75rem;
		border-radius: var(--radius);
		text-decoration: none;
	}
	.popular-link:hover { background: var(--color-surface-subtle); }
	.popular-title { font-size: 0.9375rem; color: var(--color-text); font-weight: 500; }
	.popular-audience {
		font-size: 0.6875rem;
		text-transform: uppercase;
		letter-spacing: 0.05em;
		color: var(--color-primary);
		font-weight: 600;
		flex-shrink: 0;
	}
</style>
