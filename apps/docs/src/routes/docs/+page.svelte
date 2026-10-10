<script lang="ts">
	import { untrack } from 'svelte';
	import AppShell from '$lib/components/shell/AppShell.svelte';
	import SearchPalette from '$lib/components/docs/SearchPalette.svelte';
	import { t } from '$lib/i18n';
	import { persistNavCollapse } from '$lib/nav-collapse';
	import { AUDIENCE_LABEL_KEY } from '$lib/types';
	import { AUDIENCE_ICON, AUDIENCE_DESC_KEY } from '$lib/audience-meta';
	import type { DocAudience, DocNavItem } from '$lib/types';
	import type { PageData } from './$types';
	import { ArrowRight, ArrowUpRight, FileText, FileJson, KeyRound, Play, Search } from 'lucide-svelte';
	import { safeJsonLdScript } from '$lib/utils/safeHtml';

	let { data }: { data: PageData } = $props();

	let collapsed = $state(untrack(() => data.navCollapsed));
	let searchOpen = $state(false);

	async function toggleCollapse() {
		collapsed = !collapsed;
		await persistNavCollapse(collapsed);
	}

	// Layout data (nav, accessibleAudiences, user, navCollapsed) comes via the
	// nearest layout — here the root +layout.svelte. We read it from $props.

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
			.slice(0, 6)
	);

	function audienceHref(a: DocAudience): string {
		return `/docs/${a}`;
	}

	// Nav items nest — count children recursively so the card total matches
	// what the nav tree actually shows.
	function countPages(audience: DocAudience): number {
		const node = data.nav.find((n) => n.audience === audience);
		const count = (items: DocNavItem[] | undefined): number =>
			(items ?? []).reduce((acc, i) => acc + 1 + count(i.children), 0);
		return count(node?.items);
	}

	// First page of an audience — the card's "start here" deep link.
	function firstPageSlug(audience: DocAudience): string | null {
		const node = data.nav.find((n) => n.audience === audience);
		return node?.items?.[0]?.slug ?? null;
	}

	// 'api' has no doc pages but its landing (endpoint browser) is always
	// worth linking — show the CTA even with an empty nav section.
	function showCta(audience: DocAudience): boolean {
		return audience === 'api' || firstPageSlug(audience) !== null;
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
	leftSidebarWidgets={data.leftSidebarWidgets}
	activeAudience={null}
	user={data.user}
	collapsed={collapsed}
	onToggleCollapse={toggleCollapse}
>
	<section class="landing">
		<header class="hero">
			<p class="hero-eyebrow">{t('app.tagline')}</p>
			<h1>{t('landing.title')}</h1>
			<p class="subtitle">{t('landing.subtitle')}</p>
			<button type="button" class="hero-search" onclick={() => (searchOpen = true)}>
				<Search size={18} />
				<span class="hero-search-label">{t('search.placeholder')}</span>
				<kbd class="hero-kbd">⌘K</kbd>
			</button>
		</header>

		<div class="audiences">
			<h2>{t('landing.audiences')}</h2>
			<div class="audience-grid">
				{#each data.audienceOrder.filter((a) => data.accessibleAudiences.includes(a)) as audience (audience)}
					{@const Icon = AUDIENCE_ICON[audience]}
					<a href={audienceHref(audience)} class="audience-card">
						<span class="audience-icon"><Icon size={20} /></span>
						<span class="audience-card-body">
							<span class="audience-name">{t(AUDIENCE_LABEL_KEY[audience], audience)}</span>
							<span class="audience-desc">{t(AUDIENCE_DESC_KEY[audience])}</span>
							<!-- 'api' lists OpenAPI endpoints, not doc pages — a 0-page
							     count would misread as empty. -->
							{#if audience !== 'api'}
								<span class="audience-count">
									{t('landing.pages', '{count} pages', { count: String(countPages(audience)) })}
								</span>
							{/if}
						</span>
						<span class="audience-cta">
							{#if showCta(audience)}
								{t('landing.explore')}
							{/if}
							<ArrowRight size={16} class="audience-arrow" />
						</span>
					</a>
				{/each}
			</div>
		</div>

		<div class="resources">
			<h2>{t('landing.resources')}</h2>
			<div class="resource-row">
				<a href="/docs/api/playground" class="resource-card">
					<span class="resource-icon"><Play size={16} /></span>
					<span class="resource-body">
						<span class="resource-title">{t('api.playground_title')}</span>
						<span class="resource-sub">{t('api.playground_subtitle')}</span>
					</span>
					<ArrowUpRight size={14} class="resource-arrow" />
				</a>
				<a href="/docs/api/keys" class="resource-card">
					<span class="resource-icon"><KeyRound size={16} /></span>
					<span class="resource-body">
						<span class="resource-title">{t('api.keys_title')}</span>
						<span class="resource-sub">{t('api.manage_keys')}</span>
					</span>
					<ArrowUpRight size={14} class="resource-arrow" />
				</a>
				<a href="/docs/openapi.json" class="resource-card" download>
					<span class="resource-icon"><FileJson size={16} /></span>
					<span class="resource-body">
						<span class="resource-title">{t('api.download_spec')}</span>
						<span class="resource-sub">{t('api.download_spec_sub')}</span>
					</span>
					<ArrowUpRight size={14} class="resource-arrow" />
				</a>
			</div>
		</div>

		{#if popular.length > 0}
			<div class="popular">
				<h2>{t('landing.popular')}</h2>
				<ul class="popular-list">
					{#each popular as p (`${p.audience}/${p.slug}`)}
						<li>
							<a href="/docs/{p.audience}/{p.slug}" class="popular-link">
								<FileText size={15} class="popular-icon" />
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

{#if searchOpen}
	<SearchPalette onClose={() => (searchOpen = false)} />
{/if}

<style>
	.landing {
		max-width: 960px;
		margin: 0 auto;
		padding: 3.5rem 1.5rem 4rem;
	}
	.hero { margin-bottom: 3rem; max-width: 640px; }
	.hero-eyebrow {
		font-size: 0.75rem;
		font-weight: 600;
		text-transform: uppercase;
		letter-spacing: 0.08em;
		color: var(--color-primary);
		margin: 0 0 0.75rem;
	}
	.hero h1 {
		font-family: var(--font-sans);
		font-size: 2.5rem;
		font-weight: 700;
		color: var(--color-text);
		margin: 0 0 0.75rem;
		letter-spacing: -0.03em;
		line-height: 1.15;
	}
	.subtitle {
		font-size: 1.0625rem;
		color: var(--color-text-subtle);
		line-height: 1.6;
		margin: 0 0 1.75rem;
		max-width: 56ch;
	}
	.hero-search {
		display: flex;
		align-items: center;
		gap: 0.625rem;
		width: 100%;
		max-width: 28rem;
		padding: 0.75rem 1rem;
		border: 1px solid var(--color-border);
		border-radius: var(--radius-card);
		background: var(--color-surface);
		color: var(--color-muted);
		font-size: 0.9375rem;
		cursor: pointer;
		text-align: left;
		transition: border-color 0.15s, box-shadow 0.15s;
	}
	.hero-search:hover {
		border-color: var(--color-primary);
		box-shadow: 0 0 0 3px color-mix(in srgb, var(--color-primary) 12%, transparent);
	}
	.hero-search-label { flex: 1; min-width: 0; }
	.hero-kbd {
		font-family: var(--font-mono);
		font-size: 0.6875rem;
		padding: 0.125rem 0.375rem;
		border: 1px solid var(--color-border);
		border-radius: 4px;
		color: var(--color-muted);
		background: var(--color-surface-subtle);
	}
	.audiences { margin-bottom: 2.5rem; }
	.resources { margin-bottom: 2.5rem; }
	.audiences h2, .popular h2, .resources h2 {
		font-size: 0.75rem;
		font-weight: 600;
		text-transform: uppercase;
		letter-spacing: 0.05em;
		color: var(--color-muted);
		margin: 0 0 1rem;
	}
	.audience-grid {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(280px, 1fr));
		gap: 1rem;
	}
	.audience-card {
		display: flex;
		flex-direction: column;
		gap: 0.75rem;
		padding: 1.25rem;
		border: 1px solid var(--color-border);
		border-radius: var(--radius-card);
		background: var(--color-surface);
		text-decoration: none;
		transition: border-color 0.15s, transform 0.15s, box-shadow 0.15s;
	}
	.audience-card:hover {
		border-color: var(--color-primary);
		transform: translateY(-2px);
		box-shadow: 0 4px 16px color-mix(in srgb, var(--color-text) 6%, transparent);
	}
	.audience-icon {
		display: grid;
		place-items: center;
		height: 2.5rem;
		width: 2.5rem;
		border-radius: var(--radius);
		background: color-mix(in srgb, var(--color-primary) 10%, transparent);
		color: var(--color-primary);
	}
	.audience-card-body { display: flex; flex-direction: column; gap: 0.25rem; flex: 1; }
	.audience-name { font-size: 1rem; font-weight: 600; color: var(--color-text); }
	.audience-desc {
		font-size: 0.8125rem;
		color: var(--color-text-subtle);
		line-height: 1.5;
		display: -webkit-box;
		-webkit-line-clamp: 2;
		line-clamp: 2;
		-webkit-box-orient: vertical;
		overflow: hidden;
	}
	.audience-count { font-size: 0.75rem; color: var(--color-muted); margin-top: 0.25rem; }
	.audience-cta {
		display: inline-flex;
		align-items: center;
		gap: 0.375rem;
		font-size: 0.8125rem;
		font-weight: 500;
		color: var(--color-primary);
	}
	:global(.audience-arrow) { color: var(--color-muted); flex-shrink: 0; transition: transform 0.15s; }
	.audience-card:hover :global(.audience-arrow) { color: var(--color-primary); transform: translateX(2px); }

	.resource-row {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(240px, 1fr));
		gap: 0.75rem;
	}
	.resource-card {
		display: flex;
		align-items: center;
		gap: 0.75rem;
		padding: 0.875rem 1rem;
		border: 1px solid var(--color-border);
		border-radius: var(--radius);
		background: var(--color-surface);
		text-decoration: none;
		transition: border-color 0.15s, background 0.15s;
	}
	.resource-card:hover {
		border-color: var(--color-primary);
		background: var(--color-surface-subtle);
	}
	.resource-icon {
		display: grid;
		place-items: center;
		height: 2rem;
		width: 2rem;
		border-radius: var(--radius);
		background: var(--color-surface-subtle);
		color: var(--color-text-subtle);
		flex-shrink: 0;
	}
	.resource-card:hover .resource-icon { color: var(--color-primary); }
	.resource-body { display: flex; flex-direction: column; gap: 0.0625rem; min-width: 0; flex: 1; }
	.resource-title { font-size: 0.875rem; font-weight: 600; color: var(--color-text); }
	.resource-sub {
		font-size: 0.75rem;
		color: var(--color-muted);
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	:global(.resource-arrow) { color: var(--color-muted); flex-shrink: 0; }

	.popular-list {
		list-style: none;
		padding: 0;
		margin: 0;
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(300px, 1fr));
		gap: 0.25rem 1rem;
	}
	.popular-link {
		display: flex;
		align-items: center;
		gap: 0.625rem;
		padding: 0.625rem 0.75rem;
		border-radius: var(--radius);
		text-decoration: none;
	}
	.popular-link:hover { background: var(--color-surface-subtle); }
	.popular-link :global(.popular-icon) { color: var(--color-muted); flex-shrink: 0; }
	.popular-title {
		font-size: 0.875rem;
		color: var(--color-text);
		font-weight: 500;
		flex: 1;
		min-width: 0;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.popular-link:hover .popular-title { color: var(--color-primary); }
	.popular-audience {
		font-size: 0.6875rem;
		text-transform: uppercase;
		letter-spacing: 0.05em;
		color: var(--color-primary);
		font-weight: 600;
		flex-shrink: 0;
	}
</style>
