<script lang="ts">
	import { untrack } from 'svelte';
	import AppShell from '$lib/components/shell/AppShell.svelte';
	import { t } from '$lib/i18n';
	import SeoHead from '$lib/seo/SeoHead.svelte';
	import { persistNavCollapse } from '$lib/nav-collapse';
	import { AUDIENCE_LABEL_KEY } from '$lib/types';
	import { AUDIENCE_ICON, AUDIENCE_DESC_KEY } from '$lib/audience-meta';
	import { ArrowRight, ChevronRight } from 'lucide-svelte';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	let collapsed = $state(untrack(() => data.navCollapsed));

	async function toggleCollapse() {
		collapsed = !collapsed;
		await persistNavCollapse(collapsed);
	}

	const ACCENTS = ['var(--chart-1)', 'var(--chart-2)', 'var(--chart-3)', 'var(--chart-4)', 'var(--chart-5)'];
</script>

<svelte:head>
	<SeoHead meta={data.meta} />
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
	<section class="groups-page">
		<header class="page-header">
			<nav class="breadcrumb" aria-label={t('a11y.breadcrumb', 'Breadcrumb')}>
				<a href="/docs" class="crumb">{t('nav.home', 'Home')}</a>
				<ChevronRight size={12} class="crumb-sep rtl:-scale-x-100" />
				<span class="crumb-current">{t('groups.title', 'User groups')}</span>
			</nav>
			<h1>{t('landing.groups_title')}</h1>
			<p class="page-sub">{t('landing.groups_sub')}</p>
		</header>

		<div class="group-grid">
			{#each data.groups as g, i (g.audience)}
				{@const Icon = AUDIENCE_ICON[g.audience]}
				<a href="/docs/{g.audience}" class="group-card">
					<span class="card-icon" style="--accent:{ACCENTS[i % ACCENTS.length]}"><Icon size={20} /></span>
					<span class="group-name">{t(AUDIENCE_LABEL_KEY[g.audience], g.audience)}</span>
					<span class="group-desc">{t(AUDIENCE_DESC_KEY[g.audience])}</span>
					<span class="group-foot">
						{#if g.audience !== 'api' && g.stats.pages > 0}
							<span class="group-count">
								{t('landing.guides_pages', '{guides} {guideUnit} · {pages} {pageUnit}', {
									guides: String(g.stats.guides),
									guideUnit: g.stats.guides === 1 ? t('landing.guide_one', 'guide') : t('landing.guide_other', 'guides'),
									pages: String(g.stats.pages),
									pageUnit: g.stats.pages === 1 ? t('landing.page_one', 'page') : t('landing.page_other', 'pages')
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
</AppShell>

<style>
	.groups-page {
		max-width: 960px;
		margin: 0 auto;
		padding: 2.5rem 1.5rem 4rem;
	}
	.page-header { margin-bottom: 2rem; }
	.breadcrumb {
		display: flex;
		align-items: center;
		gap: 0.375rem;
		font-size: 0.8125rem;
		margin-bottom: 1.25rem;
	}
	.crumb {
		color: var(--color-muted);
		text-decoration: none;
	}
	.crumb:hover { color: var(--color-primary); }
	.breadcrumb :global(.crumb-sep) { color: var(--color-muted); flex-shrink: 0; }
	.crumb-current { color: var(--color-text-subtle); font-weight: 500; }
	h1 {
		font-size: 1.875rem;
		font-weight: 700;
		color: var(--color-text);
		margin: 0 0 0.5rem;
		letter-spacing: -0.02em;
	}
	.page-sub {
		font-size: 0.9375rem;
		color: var(--color-text-subtle);
		margin: 0;
	}
	.group-grid {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(220px, 1fr));
		gap: 1rem;
	}
	.group-card {
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
		background: color-mix(in srgb, var(--accent) 14%, transparent);
		color: var(--accent);
	}
	.group-name {
		font-size: 1rem;
		font-weight: 600;
		color: var(--color-text);
	}
	.group-card:hover .group-name { color: var(--color-primary); }
	.group-desc {
		font-size: 0.8125rem;
		color: var(--color-text-subtle);
		line-height: 1.5;
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
</style>
