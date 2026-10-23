<script lang="ts">
	import { untrack } from 'svelte';
	import AppShell from '$lib/components/shell/AppShell.svelte';
	import SearchPalette from '$lib/components/docs/SearchPalette.svelte';
	import HeroGlobe from '$lib/components/landing/HeroGlobe.svelte';
	import { t } from '$lib/i18n';
	import { persistNavCollapse } from '$lib/nav-collapse';
	import { AUDIENCE_LABEL_KEY } from '$lib/types';
	import { AUDIENCE_ICON, AUDIENCE_DESC_KEY } from '$lib/audience-meta';
	import { categoryIcon, categoryDescKey, namedCategoryIcon } from '$lib/category-meta';
	import {
		collectCategories,
		collectChangelog,
		categoryHref,
		popularPages,
		audienceStats,
		parseCsvSetting,
		parseIntSetting,
		parseLinkList,
		parseLinkCards,
		parseManagedCategories,
		mergeManagedCategories
	} from '$lib/docs/catalog';
	import type { DocsResource } from '$lib/docs/catalog';
	import type { DocAudience } from '$lib/types';
	import type { PageData } from './$types';
	import {
		ArrowRight,
		ArrowUpRight,
		BookOpen,
		ChevronLeft,
		ChevronRight,
		CircleHelp,
		Code,
		FileJson,
		FilePen,
		FileText,
		Flame,
		KeyRound,
		Link2,
		Play,
		Search,
		Send,
		SquareTerminal
	} from 'lucide-svelte';
	import SeoHead from '$lib/seo/SeoHead.svelte';
	import { env } from '$env/dynamic/public';

	let { data }: { data: PageData } = $props();

	let collapsed = $state(untrack(() => data.navCollapsed));
	let searchOpen = $state(false);

	async function toggleCollapse() {
		collapsed = !collapsed;
		await persistNavCollapse(collapsed);
	}

	const mainOrigin = (env.PUBLIC_MAIN_ORIGIN ?? 'http://localhost:3000').replace(/\/$/, '');

	// Layout data (nav, accessibleAudiences, user, navCollapsed) comes via the
	// nearest layout — here the root +layout.svelte. We read it from $props.

	const siteSettings = $derived(data.siteSettings ?? {});

	// Hero "Popular:" chips — the leading page of each accessible audience.
	let heroPopular = $derived(popularPages(data.nav, 1, 5));

	// User-group cards — every accessible audience in canonical order.
	let groups = $derived(
		data.audienceOrder.filter((a) => data.accessibleAudiences.includes(a))
	);

	// Category cards — aggregated across all accessible audience trees.
	// Managed catalog (docs.categories) curates order/description/icon;
	// page-derived categories still fill in counts and uncategorized extras.
	let categories = $derived(
		mergeManagedCategories(
			collectCategories(data.nav),
			parseManagedCategories(siteSettings['docs.categories'])
		)
	);

	// "What's new" — changelog-category pages across accessible audiences,
	// newest first (docs.changelog_category / docs.changelog_new_days).
	let changelogCategories = $derived(
		parseCsvSetting(siteSettings['docs.changelog_category'], ['Changelog'])
	);
	let changelog = $derived(
		collectChangelog(
			data.summaries ?? [],
			changelogCategories,
			parseIntSetting(siteSettings['docs.changelog_new_days'], 14)
		).slice(0, 6)
	);
	let updatesHref = $derived(
		changelogCategories[0] ? categoryHref(changelogCategories[0]) : '/docs/categories'
	);

	// Popular documentation list — leading pages per audience.
	let popular = $derived(popularPages(data.nav, 2, 5));

	// Help / contribute links — repeater-driven (docs.help_links /
	// docs.contribute_links). An absent key falls back to the legacy
	// docs.*_url settings; a stored [] hides the links entirely.
	let helpCenterHref = $derived(
		(siteSettings['docs.help_center_url'] as string | undefined) || mainOrigin
	);
	let supportHref = $derived((siteSettings['docs.support_url'] as string | undefined) || '');
	let repoHref = $derived(((siteSettings['docs.repo_url'] as string | undefined) ?? '').replace(/\/$/, ''));
	let helpLinks = $derived.by(() => {
		const raw = siteSettings['docs.help_links'];
		if (raw !== undefined) return parseLinkList(raw);
		const out = [{ label: t('landing.help_center'), url: helpCenterHref }];
		if (supportHref) out.push({ label: t('landing.help_support'), url: supportHref });
		return out;
	});
	let contributeLinks = $derived.by(() => {
		const raw = siteSettings['docs.contribute_links'];
		if (raw !== undefined) return parseLinkList(raw);
		if (!repoHref) return [];
		return [
			{ label: t('landing.contribute_suggest'), url: repoHref },
			{ label: t('landing.contribute_issue'), url: `${repoHref}/issues` }
		];
	});

	// Developer resources — repeater-driven (docs.dev_resources). Absent key
	// renders the built-in defaults; a stored [] hides the section.
	let devResources = $derived.by((): DocsResource[] => {
		const raw = siteSettings['docs.dev_resources'];
		if (raw !== undefined) return parseLinkCards(raw);
		return [
			{ icon: 'play', label: t('api.playground_title'), url: '/docs/api/playground', description: t('api.playground_subtitle') },
			{ icon: 'key', label: t('api.keys_title'), url: '/docs/api/keys', description: t('api.manage_keys') },
			{ icon: 'file', label: t('api.download_spec'), url: '/docs/openapi.json', description: t('api.download_spec_sub') }
		];
	});
	const RESOURCE_ICONS: Record<string, typeof Link2> = {
		play: Play,
		key: KeyRound,
		file: FileJson,
		code: Code,
		book: BookOpen,
		terminal: SquareTerminal,
		link: Link2
	};

	// User-group slider — four cards per view, arrows page one card at a time.
	let groupTrack = $state<HTMLDivElement>();
	let canPrev = $state(false);
	let canNext = $state(false);
	function updateSlider() {
		const el = groupTrack;
		if (!el) return;
		canPrev = el.scrollLeft > 4;
		canNext = el.scrollLeft + el.clientWidth < el.scrollWidth - 4;
	}
	function slideBy(dir: -1 | 1) {
		const el = groupTrack;
		if (!el) return;
		const card = el.querySelector<HTMLElement>('.group-card');
		el.scrollBy({ left: dir * (card ? card.offsetWidth + 16 : el.clientWidth), behavior: 'smooth' });
	}
	$effect(() => {
		if (groupTrack) updateSlider();
	});

	function audienceHref(a: DocAudience): string {
		return `/docs/${a}`;
	}

	// 'api' has no doc pages but its landing (endpoint browser) is always
	// worth linking — and any audience with zero pages shows no count line
	// rather than "0 guides · 0 pages".
	function showStats(audience: DocAudience, pages: number): boolean {
		return audience !== 'api' && pages > 0;
	}

	// Accent rotation for card icon chips (chart tokens exist in both themes).
	const ACCENTS = ['var(--chart-1)', 'var(--chart-2)', 'var(--chart-3)', 'var(--chart-4)', 'var(--chart-5)'];
	function accent(i: number): string {
		return ACCENTS[i % ACCENTS.length];
	}

	function fmtDate(iso: string): string {
		const d = new Date(iso);
		if (Number.isNaN(d.getTime())) return '';
		return d.toLocaleDateString(data.locale ?? 'en', { month: 'short', day: 'numeric', year: 'numeric' });
	}

	// slug → excerpt lookup so the popular list can show the same preview
	// text the changelog column uses (both come from the summary payload).
	const excerptBySlug = $derived(
		new Map((data.summaries ?? []).map((s) => [`${s.audience}/${s.slug}`, s.excerpt ?? '']))
	);
</script>

<svelte:head>
	<SeoHead meta={data.meta} />
</svelte:head>

<svelte:window onresize={updateSlider} />

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
	hideRightSidebar
>
	<section class="landing">
		<header class="hero hero-card">
			<div class="hero-copy">
				<h1>{t('landing.hero_title')}</h1>
				<p class="subtitle">{t('landing.hero_subtitle')}</p>
				<div class="hero-searchbar">
					<button type="button" class="hero-search" onclick={() => (searchOpen = true)}>
						<Search size={18} />
						<span class="hero-search-label">{t('search.placeholder')}</span>
					</button>
					<button type="button" class="hero-search-btn" onclick={() => (searchOpen = true)}>
						{t('landing.search_cta')}
					</button>
				</div>
				{#if heroPopular.length > 0}
					<div class="hero-popular">
						<span class="hero-popular-label">{t('landing.popular_label')}</span>
						{#each heroPopular as p (`${p.audience}/${p.slug}`)}
							<a href="/docs/{p.audience}/{p.slug}" class="chip">{p.title}</a>
						{/each}
					</div>
				{/if}
			</div>
			<HeroGlobe />
		</header>

		{#if groups.length > 0}
			<section class="block">
				<div class="block-head row">
					<div>
						<h2>{t('landing.groups_title')}</h2>
						<p class="block-sub">{t('landing.groups_sub')}</p>
					</div>
					<div class="block-head-actions">
						<a href="/docs/groups" class="block-link">
							{t('landing.groups_all', 'See all user groups')} <ArrowRight class="rtl:-scale-x-100" size={14} />
						</a>
						<div class="slider-nav">
						<button
							type="button"
							class="slider-btn"
							disabled={!canPrev}
							onclick={() => slideBy(-1)}
							aria-label={t('landing.groups_prev', 'Previous groups')}
						>
							<ChevronLeft class="rtl:-scale-x-100" size={16} />
						</button>
						<button
							type="button"
							class="slider-btn"
							disabled={!canNext}
							onclick={() => slideBy(1)}
							aria-label={t('landing.groups_next', 'Next groups')}
						>
							<ChevronRight class="rtl:-scale-x-100" size={16} />
						</button>
						</div>
					</div>
				</div>
				<div class="group-track" bind:this={groupTrack} onscroll={updateSlider}>
					{#each groups as audience, i (audience)}
						{@const Icon = AUDIENCE_ICON[audience]}
						{@const stats = audienceStats(data.nav.find((n) => n.audience === audience))}
						<a href={audienceHref(audience)} class="group-card">
							<span class="card-icon" style="--accent:{accent(i)}"><Icon size={20} /></span>
							<span class="group-name">{t(AUDIENCE_LABEL_KEY[audience], audience)}</span>
							<span class="group-desc">{t(AUDIENCE_DESC_KEY[audience])}</span>
							<span class="group-foot">
								{#if showStats(audience, stats.pages)}
									<span class="group-count">
										{t('landing.guides_pages', '{guides} {guideUnit} · {pages} {pageUnit}', {
											guides: String(stats.guides),
											guideUnit: stats.guides === 1 ? t('landing.guide_one', 'guide') : t('landing.guide_other', 'guides'),
											pages: String(stats.pages),
											pageUnit: stats.pages === 1 ? t('landing.page_one', 'page') : t('landing.page_other', 'pages')
										})}
									</span>
								{:else}
									<span class="group-count"></span>
								{/if}
								<ArrowRight size={15} class="card-arrow rtl:-scale-x-100" />
							</span>
						</a>
					{/each}
				</div>
			</section>
		{/if}

		{#if categories.length > 0}
			<section class="block">
				<div class="block-head row">
					<div>
						<h2>{t('landing.categories_title')}</h2>
						<p class="block-sub">{t('landing.categories_sub')}</p>
					</div>
					<a href="/docs/categories" class="block-link">
						{t('categories.view_all', 'View all categories')} <ArrowRight class="rtl:-scale-x-100" size={14} />
					</a>
				</div>
				<div class="cat-grid">
					{#each categories.slice(0, 8) as cat, i (cat.slug)}
						{@const CatIcon = (cat.icon && namedCategoryIcon(cat.icon)) || categoryIcon(cat.name)}
						{@const descKey = cat.description ? null : categoryDescKey(cat.name)}
						<div class="cat-card">
							<div class="cat-head">
								<span class="card-icon sm" style="--accent:{accent(i + 1)}"><CatIcon size={16} /></span>
								<a href={categoryHref(cat.name)} class="cat-name">{cat.name}</a>
							</div>
							<span class="cat-desc">
								{cat.description ??
									(descKey
										? t(descKey)
										: t('landing.pages_count', '{count} pages', { count: String(cat.items.length) }))}
							</span>
							<ul class="cat-links">
								{#if cat.subcategories.length > 0}
									{#each cat.subcategories.slice(0, 5) as sub (sub)}
										<li><a href={categoryHref(cat.name)} class="cat-link">» {sub}</a></li>
									{/each}
								{:else}
									{#each cat.items.slice(0, 5) as item (`${item.audience}/${item.slug}`)}
										<li>
											<a href="/docs/{item.audience}/{item.slug}" class="cat-link">» {item.title}</a>
										</li>
									{/each}
								{/if}
							</ul>
							<a href={categoryHref(cat.name)} class="cat-all">
								{t('landing.view_all_count', 'View all ({count})', { count: String(cat.items.length) })}
								<ArrowRight class="rtl:-scale-x-100" size={13} />
							</a>
						</div>
					{/each}
				</div>
			</section>
		{/if}

		<div class="duo">
			{#if popular.length > 0}
				<section class="panel">
					<div class="panel-head">
						<h2 class="panel-title"><Flame size={17} class="panel-title-icon" />{t('landing.popular_docs')}</h2>
						<a href="/docs/search" class="block-link sm">
							{t('landing.popular_all')} <ArrowRight class="rtl:-scale-x-100" size={13} />
						</a>
					</div>
					<ul class="pop-list">
						{#each popular as p (`${p.audience}/${p.slug}`)}
							{@const excerpt = excerptBySlug.get(`${p.audience}/${p.slug}`)}
							{@const PopIcon = p.category ? categoryIcon(p.category) : AUDIENCE_ICON[p.audience]}
							<li>
								<a href="/docs/{p.audience}/{p.slug}" class="pop-link">
									<span class="pop-icon"><PopIcon size={15} /></span>
									<span class="pop-body">
										<span class="pop-title">{p.title}</span>
										{#if excerpt}
											<span class="pop-sub">{excerpt}</span>
										{:else if p.category}
											<span class="pop-sub">{p.category}</span>
										{/if}
									</span>
									<span class="pop-badge aud-{p.audience}">{t(AUDIENCE_LABEL_KEY[p.audience], p.audience)}</span>
									<ChevronRight size={15} class="pop-chevron rtl:-scale-x-100" />
								</a>
							</li>
						{/each}
					</ul>
				</section>
			{/if}

			{#if changelog.length > 0}
				<section class="panel">
					<div class="panel-head">
						<h2 class="panel-title"><Send size={16} class="panel-title-icon rtl:-scale-x-100" />{t('landing.whats_new')}</h2>
						<a href={updatesHref} class="block-link sm">
							{t('landing.updates_all')} <ArrowRight class="rtl:-scale-x-100" size={13} />
						</a>
					</div>
					<ul class="news-list">
						{#each changelog as entry (`${entry.audience}/${entry.slug}`)}
							<li>
								<a href="/docs/{entry.audience}/{entry.slug}" class="news-link">
									<span class="news-badge" class:is-new={entry.isNew}>
										{entry.isNew ? t('landing.badge_new') : t('landing.badge_update')}
									</span>
									<span class="news-body">
										<span class="news-title">{entry.title}</span>
										{#if entry.excerpt}
											<span class="news-sub">{entry.excerpt}</span>
										{/if}
									</span>
									<time class="news-date" datetime={entry.createdAt}>{fmtDate(entry.createdAt)}</time>
								</a>
							</li>
						{/each}
					</ul>
				</section>
			{/if}
		</div>

		<div class="duo">
			<section class="mini-card">
				<div class="mini-head">
					<span class="mini-icon"><CircleHelp size={18} /></span>
					<div>
						<h2 class="mini-title">{t('landing.help_title')}</h2>
						<p class="mini-sub">{t('landing.help_sub')}</p>
					</div>
				</div>
				{#if helpLinks.length > 0}
					<div class="mini-actions">
						{#each helpLinks as link (link.label + link.url)}
							<a href={link.url} class="mini-btn" target="_blank" rel="noopener noreferrer">
								{link.label} <ArrowRight class="rtl:-scale-x-100" size={14} />
							</a>
						{/each}
					</div>
				{/if}
			</section>
			{#if contributeLinks.length > 0}
				<section class="mini-card">
					<div class="mini-head">
						<span class="mini-icon"><FilePen size={18} /></span>
						<div>
							<h2 class="mini-title">{t('landing.contribute_title')}</h2>
							<p class="mini-sub">{t('landing.contribute_sub')}</p>
						</div>
					</div>
					<div class="mini-actions">
						{#each contributeLinks as link (link.label + link.url)}
							<a href={link.url} class="mini-btn" target="_blank" rel="noopener noreferrer">
								{link.label} <ArrowRight class="rtl:-scale-x-100" size={14} />
							</a>
						{/each}
					</div>
				</section>
			{/if}
		</div>

		{#if devResources.length > 0}
			<div class="resources">
				<h2 class="res-title">{t('landing.resources')}</h2>
				<div class="resource-row">
					{#each devResources as res (res.label + res.url)}
						{@const ResIcon = RESOURCE_ICONS[res.icon ?? 'link'] ?? Link2}
						<a
							href={res.url}
							class="resource-card"
							target={res.url.startsWith('http') ? '_blank' : undefined}
							rel={res.url.startsWith('http') ? 'noopener noreferrer' : undefined}
							data-sveltekit-reload={res.url.startsWith('http')
								? undefined
								: /\.[a-z0-9]+$/i.test(res.url.split('/').pop() ?? '') || undefined}
						>
							<span class="resource-icon"><ResIcon size={16} /></span>
							<span class="resource-body">
								<span class="resource-title">{res.label}</span>
								{#if res.description}
									<span class="resource-sub">{res.description}</span>
								{/if}
							</span>
							<ArrowUpRight size={14} class="resource-arrow rtl:-scale-x-100" />
						</a>
					{/each}
				</div>
			</div>
		{/if}
	</section>
</AppShell>

{#if searchOpen}
	<SearchPalette onClose={() => (searchOpen = false)} />
{/if}

<style>
	.landing {
		max-width: 1100px;
		margin: 0 auto;
		padding: 2.5rem 1.5rem 4rem;
	}

	/* ---- Hero ---- */
	.hero-card {
		position: relative;
		overflow: hidden;
		padding: 3rem 3.5rem;
		border: 1px solid var(--color-border);
		border-radius: var(--radius-card);
		background:
			radial-gradient(60% 120% at 85% 0%, color-mix(in srgb, var(--color-primary) 14%, transparent), transparent 70%),
			linear-gradient(135deg, color-mix(in srgb, var(--color-primary) 9%, var(--color-surface)) 0%, var(--color-surface) 55%);
	}
	.hero {
		display: grid;
		grid-template-columns: minmax(0, 1fr) auto;
		align-items: center;
		gap: 2.5rem;
		margin-bottom: 3.5rem;
	}
	.hero h1 {
		font-size: 2.5rem;
		font-weight: 700;
		color: var(--color-text);
		margin: 0 0 0.875rem;
		letter-spacing: -0.03em;
		line-height: 1.12;
	}
	.subtitle {
		font-size: 1.0625rem;
		color: var(--color-text-subtle);
		line-height: 1.6;
		margin: 0 0 1.75rem;
		max-width: 52ch;
	}
	.hero-searchbar {
		display: flex;
		gap: 0.625rem;
		max-width: 32rem;
		margin-bottom: 1.25rem;
	}
	.hero-search {
		display: flex;
		align-items: center;
		gap: 0.625rem;
		flex: 1;
		min-width: 0;
		padding: 0.75rem 1rem;
		border: 1px solid var(--color-border);
		border-radius: var(--radius-card);
		background: var(--color-surface);
		color: var(--color-muted);
		font-size: 0.9375rem;
		cursor: pointer;
		text-align: start;
		transition: border-color 0.15s, box-shadow 0.15s;
	}
	.hero-search:hover {
		border-color: var(--color-primary);
		box-shadow: 0 0 0 3px color-mix(in srgb, var(--color-primary) 12%, transparent);
	}
	.hero-search-label { flex: 1; min-width: 0; }
	.hero-search-btn {
		padding: 0 1.25rem;
		border: none;
		border-radius: var(--radius-card);
		background: var(--color-primary);
		color: var(--color-primary-contrast);
		font-size: 0.9375rem;
		font-weight: 600;
		cursor: pointer;
		white-space: nowrap;
	}
	.hero-search-btn:hover { background: var(--color-primary-hover); }
	.hero-popular {
		display: flex;
		align-items: center;
		flex-wrap: wrap;
		gap: 0.5rem;
	}
	.hero-popular-label {
		font-size: 0.8125rem;
		color: var(--color-muted);
		margin-inline-end: 0.125rem;
	}
	.chip {
		padding: 0.375rem 0.75rem;
		border: 1px solid var(--color-border);
		border-radius: 999px;
		background: var(--color-surface);
		font-size: 0.8125rem;
		color: var(--color-text-subtle);
		text-decoration: none;
		transition: border-color 0.15s, color 0.15s;
	}
	.chip:hover { border-color: var(--color-primary); color: var(--color-primary); }

	/* ---- Section scaffolding ---- */
	.block { margin-bottom: 3rem; }
	.block-head { margin-bottom: 1.25rem; }
	.block-head.row {
		display: flex;
		align-items: flex-end;
		justify-content: space-between;
		gap: 1rem;
	}
	.block-head-actions {
		display: flex;
		align-items: center;
		gap: 1.25rem;
	}
	.block-head h2, .panel-head h2 {
		font-size: 1.375rem;
		font-weight: 700;
		color: var(--color-text);
		letter-spacing: -0.02em;
		margin: 0 0 0.375rem;
	}
	.block-sub {
		font-size: 0.9375rem;
		color: var(--color-text-subtle);
		margin: 0;
	}
	.block-link {
		display: inline-flex;
		align-items: center;
		gap: 0.375rem;
		font-size: 0.875rem;
		font-weight: 500;
		color: var(--color-primary);
		text-decoration: none;
		white-space: nowrap;
	}
	.block-link:hover { text-decoration: underline; }
	.block-link.sm { font-size: 0.8125rem; }

	/* ---- User-group cards ---- */
	.group-track {
		display: flex;
		gap: 1rem;
		overflow-x: auto;
		scroll-snap-type: x mandatory;
		scrollbar-width: none;
	}
	.group-track::-webkit-scrollbar { display: none; }
	.group-card {
		flex: 0 0 calc((100% - 3rem) / 4);
		scroll-snap-align: start;
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
		padding: 1.25rem;
		border: 1px solid var(--color-border);
		border-radius: var(--radius-card);
		background: var(--color-surface);
		text-decoration: none;
		transition: border-color 0.15s, transform 0.15s, box-shadow 0.15s;
	}
	.group-card:hover {
		border-color: var(--color-primary);
		transform: translateY(-2px);
		box-shadow: 0 4px 16px color-mix(in srgb, var(--color-text) 6%, transparent);
	}
	.card-icon {
		display: grid;
		place-items: center;
		height: 2.75rem;
		width: 2.75rem;
		border-radius: 50%;
		background: color-mix(in srgb, var(--accent, var(--color-primary)) 14%, transparent);
		color: var(--accent, var(--color-primary));
		margin-bottom: 0.375rem;
	}
	.card-icon.sm {
		height: 2rem;
		width: 2rem;
		border-radius: var(--radius);
		margin-bottom: 0;
	}
	.group-name { font-size: 1rem; font-weight: 600; color: var(--color-text); }
	.group-desc {
		font-size: 0.8125rem;
		color: var(--color-text-subtle);
		line-height: 1.5;
		display: -webkit-box;
		-webkit-line-clamp: 2;
		line-clamp: 2;
		-webkit-box-orient: vertical;
		overflow: hidden;
		flex: 1;
	}
	.group-foot {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 0.5rem;
	}
	.group-count { font-size: 0.75rem; color: var(--color-muted); }
	:global(.card-arrow) { color: var(--color-muted); transition: transform 0.15s, color 0.15s; }
	.group-card:hover :global(.card-arrow) { color: var(--color-primary); transform: translateX(2px); }

	.slider-nav { display: flex; gap: 0.375rem; }
	.slider-btn {
		display: grid;
		place-items: center;
		height: 2rem;
		width: 2rem;
		border: 1px solid var(--color-border);
		border-radius: var(--radius);
		background: var(--color-surface);
		color: var(--color-text-subtle);
		cursor: pointer;
		transition: border-color 0.15s, color 0.15s, opacity 0.15s;
	}
	.slider-btn:hover:not(:disabled) { border-color: var(--color-primary); color: var(--color-primary); }
	.slider-btn:disabled { opacity: 0.4; cursor: default; }

	/* ---- Category cards ---- */
	.cat-grid {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(250px, 1fr));
		gap: 1rem;
	}
	.cat-card {
		display: flex;
		flex-direction: column;
		gap: 0.375rem;
		padding: 1.125rem;
		border: 1px solid var(--color-border);
		border-radius: var(--radius-card);
		background: var(--color-surface);
		transition: border-color 0.15s;
	}
	.cat-card:hover { border-color: var(--color-primary); }
	.cat-head {
		display: flex;
		align-items: center;
		gap: 0.625rem;
	}
	.cat-name {
		font-size: 0.9375rem;
		font-weight: 600;
		color: var(--color-text);
		text-decoration: none;
	}
	.cat-name:hover { color: var(--color-primary); }
	.cat-desc {
		font-size: 0.8125rem;
		color: var(--color-text-subtle);
		line-height: 1.5;
		display: -webkit-box;
		-webkit-line-clamp: 2;
		line-clamp: 2;
		-webkit-box-orient: vertical;
		overflow: hidden;
	}
	.cat-links {
		list-style: none;
		padding: 0.25rem 0;
		margin: 0;
		display: flex;
		flex-direction: column;
		gap: 0.125rem;
		flex: 1;
	}
	.cat-link {
		display: block;
		padding: 0.1875rem 0;
		font-size: 0.8125rem;
		color: var(--color-text-subtle);
		text-decoration: none;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.cat-link:hover { color: var(--color-primary); }
	.cat-all {
		display: inline-flex;
		align-items: center;
		gap: 0.375rem;
		font-size: 0.8125rem;
		font-weight: 500;
		color: var(--color-primary);
		text-decoration: none;
	}
	.cat-all:hover { text-decoration: underline; }

	/* ---- Popular + What's new panels ---- */
	.duo {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(300px, 1fr));
		gap: 1rem;
		margin-bottom: 1rem;
	}
	.panel {
		border: 1px solid var(--color-border);
		border-radius: var(--radius-card);
		background: var(--color-surface);
		padding: 1.25rem;
	}
	.panel-head {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 1rem;
		margin-bottom: 0.75rem;
	}
	.panel-head h2 { font-size: 1rem; }
	.panel-title {
		display: inline-flex;
		align-items: center;
		gap: 0.5rem;
	}
	.panel-title :global(.panel-title-icon) { color: var(--color-primary); }
	.pop-list, .news-list {
		list-style: none;
		padding: 0;
		margin: 0;
	}
	.pop-link, .news-link {
		display: flex;
		align-items: center;
		gap: 0.75rem;
		padding: 0.625rem 0.5rem;
		border-radius: var(--radius);
		text-decoration: none;
	}
	.pop-link:hover, .news-link:hover { background: var(--color-surface-subtle); }
	.pop-icon {
		display: grid;
		place-items: center;
		height: 2rem;
		width: 2rem;
		border-radius: var(--radius);
		background: var(--color-surface-subtle);
		color: var(--color-muted);
		flex-shrink: 0;
	}
	.pop-body { display: flex; flex-direction: column; min-width: 0; flex: 1; }
	.pop-title {
		font-size: 0.875rem;
		font-weight: 500;
		color: var(--color-text);
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.pop-link:hover .pop-title { color: var(--color-primary); }
	.pop-sub {
		font-size: 0.75rem;
		color: var(--color-muted);
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.pop-link :global(.pop-chevron) { color: var(--color-muted); flex-shrink: 0; }
	.pop-link:hover :global(.pop-chevron) { color: var(--color-primary); }
	.pop-badge {
		font-size: 0.625rem;
		font-weight: 700;
		text-transform: uppercase;
		letter-spacing: 0.05em;
		padding: 0.1875rem 0.5rem;
		border-radius: 4px;
		flex-shrink: 0;
		color: var(--color-primary);
		background: color-mix(in srgb, var(--color-primary) 10%, transparent);
	}
	.pop-badge.aud-users { color: var(--chart-3); background: color-mix(in srgb, var(--chart-3) 12%, transparent); }
	.pop-badge.aud-api { color: var(--chart-1); background: color-mix(in srgb, var(--chart-1) 10%, transparent); }
	.pop-badge.aud-premium { color: var(--chart-4); background: color-mix(in srgb, var(--chart-4) 14%, transparent); }
	.pop-badge.aud-admin, .pop-badge.aud-superadmin {
		color: var(--chart-5); background: color-mix(in srgb, var(--chart-5) 10%, transparent);
	}
	.news-badge {
		font-size: 0.625rem;
		font-weight: 700;
		text-transform: uppercase;
		letter-spacing: 0.05em;
		padding: 0.1875rem 0.5rem;
		border-radius: 4px;
		flex-shrink: 0;
		min-width: 3.5rem;
		text-align: center;
		color: var(--chart-2);
		background: color-mix(in srgb, var(--chart-2) 12%, transparent);
	}
	.news-badge.is-new {
		color: var(--chart-3);
		background: color-mix(in srgb, var(--chart-3) 12%, transparent);
	}
	.news-body { flex: 1; min-width: 0; }
	.news-title {
		font-size: 0.875rem;
		font-weight: 500;
		color: var(--color-text);
		display: block;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.news-sub {
		font-size: 0.75rem;
		color: var(--color-muted);
		display: block;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.news-link:hover .news-title { color: var(--color-primary); }
	.news-date {
		font-size: 0.75rem;
		color: var(--color-muted);
		flex-shrink: 0;
	}

	/* ---- Help / contribute cards ---- */
	.mini-card {
		border: 1px solid var(--color-border);
		border-radius: var(--radius-card);
		background: var(--color-surface);
		padding: 1.25rem;
	}
	.mini-head {
		display: flex;
		align-items: flex-start;
		gap: 0.75rem;
		margin-bottom: 1rem;
	}
	.mini-icon {
		display: grid;
		place-items: center;
		height: 2.25rem;
		width: 2.25rem;
		border-radius: 50%;
		background: color-mix(in srgb, var(--color-primary) 12%, transparent);
		color: var(--color-primary);
		flex-shrink: 0;
	}
	.mini-title {
		font-size: 1rem;
		font-weight: 600;
		color: var(--color-text);
		margin: 0 0 0.25rem;
	}
	.mini-sub {
		font-size: 0.8125rem;
		color: var(--color-text-subtle);
		margin: 0;
	}
	.mini-actions {
		margin-top: 1rem;
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
	}
	.mini-btn {
		display: flex;
		align-items: center;
		gap: 0.375rem;
		width: 100%;
		padding: 0.5rem 0.875rem;
		border: 1px solid var(--color-border);
		border-radius: var(--radius);
		background: var(--color-surface-subtle);
		font-size: 0.8125rem;
		font-weight: 500;
		color: var(--color-text);
		text-decoration: none;
		transition: border-color 0.15s, color 0.15s;
	}
	.mini-btn :global(svg) { margin-inline-start: auto; }
	.mini-btn:hover { border-color: var(--color-primary); color: var(--color-primary); }

	/* ---- Developer resources ---- */
	.resources { margin-top: 2rem; }
	.res-title {
		font-size: 0.75rem;
		font-weight: 600;
		text-transform: uppercase;
		letter-spacing: 0.05em;
		color: var(--color-muted);
		margin: 0 0 1rem;
	}
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

	@media (max-width: 900px) {
		.hero { grid-template-columns: 1fr; }
		.group-card { flex-basis: calc((100% - 1rem) / 2); }
	}
	@media (max-width: 640px) {
		.landing { padding-top: 1.5rem; }
		.hero-card { padding: 1.5rem; }
		.hero h1 { font-size: 1.875rem; }
		.hero-search-btn { display: none; }
		.group-card { flex-basis: 72%; }
		.block-head.row {
			flex-direction: column;
			align-items: flex-start;
			gap: 0.75rem;
		}
		.block-head-actions { align-self: stretch; justify-content: space-between; }
	}
</style>
