<script lang="ts">
	import { untrack } from 'svelte';
	import AppShell from '$lib/components/shell/AppShell.svelte';
	import { t } from '$lib/i18n';
	import SeoHead from '$lib/seo/SeoHead.svelte';
	import { persistNavCollapse } from '$lib/nav-collapse';
	import { categoryIcon, namedCategoryIcon } from '$lib/category-meta';
	import { categoryHref } from '$lib/docs/catalog';
	import { ArrowRight, ChevronRight } from 'lucide-svelte';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	let collapsed = $state(untrack(() => data.navCollapsed));

	async function toggleCollapse() {
		collapsed = !collapsed;
		await persistNavCollapse(collapsed);
	}
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
	<section class="categories-page">
		<header class="page-header">
			<nav class="breadcrumb" aria-label={t('a11y.breadcrumb', 'Breadcrumb')}>
				<a href="/docs" class="crumb">{t('nav.home', 'Home')}</a>
				<ChevronRight size={12} class="crumb-sep rtl:-scale-x-100" />
				<span class="crumb-current">{t('categories.title', 'Documentation categories')}</span>
			</nav>
			<h1>{t('categories.title', 'Documentation categories')}</h1>
			<p class="page-sub">{t('categories.sub', 'Browse all documentation by category and sub-category.')}</p>
		</header>

		{#if data.categories.length === 0}
			<p class="empty">{t('nav.empty', 'No documentation available.')}</p>
		{:else}
			<div class="cat-grid">
				{#each data.categories as cat (cat.slug)}
					{@const CatIcon = (cat.icon && namedCategoryIcon(cat.icon)) || categoryIcon(cat.name)}
					<a href={categoryHref(cat.name)} class="cat-card">
						<span class="card-icon"><CatIcon size={20} /></span>
						<span class="cat-body">
							<span class="cat-name">{cat.name}</span>
							<span class="cat-count">
								{cat.description ??
									t('landing.pages_count', '{count} {pageUnit}', {
										count: String(cat.items.length),
										pageUnit: cat.items.length === 1 ? t('landing.page_one', 'page') : t('landing.page_other', 'pages')
									})}
							</span>
							{#if cat.subcategories.length > 0}
								<span class="cat-subs">{cat.subcategories.slice(0, 4).join(' · ')}</span>
							{/if}
						</span>
						<ArrowRight size={16} class="cat-arrow rtl:-scale-x-100" />
					</a>
				{/each}
			</div>
		{/if}
	</section>
</AppShell>

<style>
	.categories-page {
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
	.cat-grid {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(280px, 1fr));
		gap: 0.875rem;
	}
	.cat-card {
		display: flex;
		align-items: center;
		gap: 0.875rem;
		padding: 1rem 1.125rem;
		border: 1px solid var(--color-border);
		border-radius: var(--radius-card);
		background: var(--color-surface);
		text-decoration: none;
		transition: border-color 0.15s, box-shadow 0.15s;
	}
	.cat-card:hover {
		border-color: var(--color-primary);
		box-shadow: 0 2px 10px color-mix(in srgb, var(--color-text) 5%, transparent);
	}
	.card-icon {
		display: grid;
		place-items: center;
		height: 2.5rem;
		width: 2.5rem;
		border-radius: var(--radius);
		background: color-mix(in srgb, var(--color-primary) 10%, transparent);
		color: var(--color-primary);
		flex-shrink: 0;
	}
	.cat-body { display: flex; flex-direction: column; gap: 0.125rem; min-width: 0; flex: 1; }
	.cat-name {
		font-size: 0.9375rem;
		font-weight: 600;
		color: var(--color-text);
	}
	.cat-card:hover .cat-name { color: var(--color-primary); }
	.cat-count { font-size: 0.75rem; color: var(--color-muted); }
	.cat-subs {
		font-size: 0.75rem;
		color: var(--color-text-subtle);
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	:global(.cat-arrow) { color: var(--color-muted); flex-shrink: 0; }
	.cat-card:hover :global(.cat-arrow) { color: var(--color-primary); }
	.empty {
		padding: 2rem;
		text-align: center;
		color: var(--color-muted);
		border: 1px dashed var(--color-border);
		border-radius: var(--radius-card);
	}
</style>
