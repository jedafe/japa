<script lang="ts">
	import { untrack } from 'svelte';
	import AppShell from '$lib/components/shell/AppShell.svelte';
	import { t } from '$lib/i18n';
	import { persistNavCollapse } from '$lib/nav-collapse';
	import SeoHead from '$lib/seo/SeoHead.svelte';
	import { categoryIcon, namedCategoryIcon } from '$lib/category-meta';
	import { ArrowRight, ChevronRight, FileText } from 'lucide-svelte';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	let collapsed = $state(untrack(() => data.navCollapsed));

	async function toggleCollapse() {
		collapsed = !collapsed;
		await persistNavCollapse(collapsed);
	}

	const CatIcon = $derived(
		(data.category.icon && namedCategoryIcon(data.category.icon)) ||
			categoryIcon(data.category.name)
	);
	const totalPages = $derived(data.category.items.length);
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
	<section class="category-page">
		<header class="category-header">
			<nav class="breadcrumb" aria-label={t('a11y.breadcrumb', 'Breadcrumb')}>
				<a href="/docs" class="crumb">{t('nav.home', 'Home')}</a>
				<ChevronRight size={12} class="crumb-sep rtl:-scale-x-100" />
				<a href="/docs/categories" class="crumb">{t('category.title', 'Category')}</a>
				<ChevronRight size={12} class="crumb-sep rtl:-scale-x-100" />
				<span class="crumb-current">{data.category.name}</span>
			</nav>
			<div class="category-title-row">
				<span class="category-icon"><CatIcon size={22} /></span>
				<div class="category-title-body">
					<h1>{data.category.name}</h1>
					<p class="category-count">
						{data.category.description ??
							t('landing.pages_count', '{count} {pageUnit}', {
								count: String(totalPages),
								pageUnit: totalPages === 1 ? t('landing.page_one', 'page') : t('landing.page_other', 'pages')
							})}
					</p>
				</div>
			</div>
		</header>

		{#if data.groups.length === 0}
			<p class="empty-category">{t('category.empty', 'No pages in this category yet.')}</p>
		{/if}
		{#each data.groups as group (group.audience)}
			<section class="audience-section">
				<h2 class="audience-title">{t(group.labelKey, group.audience)}</h2>
				{#each group.subgroups as sub (sub.name)}
					{#if sub.name}
						<h3 class="sub-title">{sub.name}</h3>
					{/if}
					<div class="doc-grid">
						{#each sub.items as item (`${item.audience}/${item.slug}`)}
							<a href="/docs/{item.audience}/{item.slug}" class="doc-card">
								<FileText size={16} class="doc-icon" />
								<span class="doc-title">{item.title}</span>
								<ArrowRight size={15} class="doc-arrow rtl:-scale-x-100" />
							</a>
						{/each}
					</div>
				{/each}
			</section>
		{/each}
	</section>
</AppShell>

<style>
	.category-page {
		max-width: 860px;
		margin: 0 auto;
		padding: 2.5rem 1.5rem 4rem;
	}
	.category-header { margin-bottom: 2rem; }
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
	.category-title-row {
		display: flex;
		align-items: center;
		gap: 0.875rem;
	}
	.category-icon {
		display: grid;
		place-items: center;
		height: 2.75rem;
		width: 2.75rem;
		border-radius: var(--radius-card);
		background: color-mix(in srgb, var(--color-primary) 10%, transparent);
		color: var(--color-primary);
		flex-shrink: 0;
	}
	.category-title-body h1 {
		font-size: 1.75rem;
		font-weight: 700;
		color: var(--color-text);
		margin: 0;
		letter-spacing: -0.02em;
		line-height: 1.2;
	}
	.category-count {
		font-size: 0.75rem;
		color: var(--color-muted);
		margin: 0.375rem 0 0;
	}
	.empty-category {
		color: var(--color-muted);
		font-size: 0.875rem;
		border: 1px dashed var(--color-border);
		border-radius: var(--radius-md, 8px);
		padding: 1.5rem;
		text-align: center;
	}
	.audience-section { margin-top: 2rem; }
	.audience-title {
		font-size: 1.125rem;
		font-weight: 700;
		color: var(--color-text);
		margin: 0 0 0.875rem;
		letter-spacing: -0.01em;
	}
	.sub-title {
		font-size: 0.8125rem;
		font-weight: 600;
		text-transform: uppercase;
		letter-spacing: 0.05em;
		color: var(--color-muted);
		margin: 1.25rem 0 0.625rem;
	}
	.doc-grid {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(300px, 1fr));
		gap: 0.875rem;
		align-items: start;
	}
	.doc-card {
		display: flex;
		align-items: center;
		gap: 0.625rem;
		padding: 1rem 1.125rem;
		border: 1px solid var(--color-border);
		border-radius: var(--radius-card);
		background: var(--color-surface);
		text-decoration: none;
		transition: border-color 0.15s, box-shadow 0.15s;
	}
	.doc-card:hover {
		border-color: var(--color-primary);
		box-shadow: 0 2px 10px color-mix(in srgb, var(--color-text) 5%, transparent);
	}
	.doc-card :global(.doc-icon) { color: var(--color-muted); flex-shrink: 0; }
	.doc-card:hover :global(.doc-icon) { color: var(--color-primary); }
	.doc-title {
		font-size: 0.9375rem;
		font-weight: 600;
		color: var(--color-text);
		flex: 1;
		min-width: 0;
	}
	.doc-card:hover .doc-title { color: var(--color-primary); }
	:global(.doc-arrow) { color: var(--color-muted); flex-shrink: 0; }
	.doc-card:hover :global(.doc-arrow) { color: var(--color-primary); }
</style>
