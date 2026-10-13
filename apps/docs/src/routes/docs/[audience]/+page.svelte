<script lang="ts">
	import { untrack } from 'svelte';
	import { onMount } from 'svelte';
	import AppShell from '$lib/components/shell/AppShell.svelte';
	import { t } from '$lib/i18n';
	import { page } from '$app/state';
	import { persistNavCollapse } from '$lib/nav-collapse';
	import SeoHead from '$lib/seo/SeoHead.svelte';
	import { AUDIENCE_ICON, AUDIENCE_DESC_KEY } from '$lib/audience-meta';
	import { groupNavItems } from '$lib/docs/grouping';
	import { ArrowRight, ArrowUpRight, ChevronRight, FileText, Plus } from 'lucide-svelte';
	import type { PageData } from './$types';
	import type { DocNavItem } from '$lib/types';

	let { data }: { data: PageData } = $props();

	let collapsed = $state(untrack(() => data.navCollapsed));

	async function toggleCollapse() {
		collapsed = !collapsed;
		await persistNavCollapse(collapsed);
	}


	// The nav tree (layout data) carries parent→children hierarchy — the flat
	// pages list doesn't. Build the section cards from it (Rule 16: same API
	// payload the sidebar renders).
	const navItems = $derived(
		data.nav.find((tree) => tree.audience === data.audience)?.items ?? []
	);
	// Category → subcategory grouping (§44 taxonomy): ungrouped pages first,
	// then category sections in first-appearance order.
	const grouped = $derived(groupNavItems(navItems));
	const AudienceIcon = $derived(AUDIENCE_ICON[data.audience]);

	// Post-create/save notices (?created=<status> / ?saved=<status>) —
	// one-shot: strip the params after mount so a refresh doesn't resurrect
	// the banner (§44 amendment 2).
	let noticeStatus = $state<string | null>(null);
	onMount(() => {
		noticeStatus =
			page.url.searchParams.get('created') ?? page.url.searchParams.get('saved');
		if (noticeStatus) {
			const url = new URL(page.url);
			url.searchParams.delete('created');
			url.searchParams.delete('saved');
			history.replaceState(history.state, '', url);
		}
	});
	const noticeText = $derived.by(() => {
		switch (noticeStatus) {
			case 'published':
				return t('notice.saved_published', 'Published — this page is now live.');
			case 'approved':
				return t('notice.saved_approved', 'Approved — ready to publish.');
			case 'in_review':
				return t('notice.saved_in_review', 'Submitted for review. Only editors can see this page.');
			case 'draft':
				return t('notice.saved_draft', 'Saved as draft. Only editors can see this page.');
			default:
				return noticeStatus ? t('notice.saved', 'Changes saved.') : null;
		}
	});

	function pageHref(slug: string): string {
		return `/docs/${data.audience}/${slug}`;
	}
</script>

<svelte:head>
	<SeoHead meta={data.meta} gated={data.isGated} />
</svelte:head>

<AppShell
	nav={data.nav}
	accessibleAudiences={data.accessibleAudiences}
	logoUrl={(data.siteSettings?.["appearance.logo_url"] as string | undefined) ?? ""}
	footerMenu={data.footerMenu}
	rightSidebarWidgets={data.rightSidebarWidgets}
	leftSidebarWidgets={data.leftSidebarWidgets}
	activeAudience={data.audience}
	user={data.user}
	collapsed={collapsed}
	onToggleCollapse={toggleCollapse}
>
	<section class="audience-page">
		<header class="audience-header">
			<nav class="breadcrumb" aria-label="Breadcrumb">
				<a href="/docs" class="crumb">{t('audience.overview')}</a>
				<ChevronRight size={12} class="crumb-sep" />
				<span class="crumb-current">{t(data.labelKey, data.audience)}</span>
			</nav>
			<div class="audience-title-row">
				<span class="audience-icon"><AudienceIcon size={22} /></span>
				<div class="audience-title-body">
					<h1>{t(data.labelKey, data.audience)}</h1>
					<p class="audience-desc">{t(AUDIENCE_DESC_KEY[data.audience])}</p>
				</div>
			</div>
			{#if navItems.length > 0}
				<p class="audience-count">
					{t('landing.pages', '{count} pages', { count: String(data.pages.length) })}
				</p>
			{/if}
			{#if data.canEditDocs}
				<div class="audience-actions">
					<a href="/docs/{data.audience}/new" class="new-page-link">
						<Plus size={15} />
						{t('audience.new_page', 'New page')}
					</a>
				</div>
			{/if}
		</header>

		{#if noticeText}
			<p class="created-notice" role="status">{noticeText}</p>
		{/if}

		{#if navItems.length === 0 && data.unpublishedPages.length === 0}
			<p class="empty">{t('nav.empty')}</p>
		{:else}
			{#if grouped.ungrouped.length > 0}
				<div class="doc-grid">
					{#each grouped.ungrouped as item (item.id)}
						{@render docCard(item)}
					{/each}
				</div>
			{/if}
			{#each grouped.categories as cat (cat.name)}
				<section class="cat-section">
					<h2 class="cat-title">{cat.name}</h2>
					{#each cat.subcategories as sub (sub.name)}
						{#if sub.name}
							<h3 class="subcat-title">{sub.name}</h3>
						{/if}
						<div class="doc-grid">
							{#each sub.items as item (item.id)}
								{@render docCard(item)}
							{/each}
						</div>
					{/each}
				</section>
			{/each}

			<!-- Editor-only: unpublished pages (draft / in_review / approved) so
			     in-flight docs stay reachable + reviewable in-app (§44 amendment 2). -->
			{#if data.unpublishedPages.length > 0}
				<section class="cat-section unpublished-section">
					<h2 class="cat-title">
						{t('audience.unpublished', 'In progress')}
						<span class="unpublished-count">{data.unpublishedPages.length}</span>
					</h2>
					<div class="doc-grid">
						{#each data.unpublishedPages as item (item.id)}
							<a href={pageHref(item.slug)} class="doc-card doc-card-link doc-card-draft">
								<FileText size={16} class="doc-link-icon" />
								<span class="doc-card-title">{item.title}</span>
								<span class="status-badge status-{item.review_status}">
									{t(`new_page.status.${item.review_status}`, item.review_status ?? '')}
								</span>
								<ArrowRight size={15} class="doc-card-arrow" />
							</a>
						{/each}
					</div>
				</section>
			{/if}
		{/if}
	</section>
</AppShell>

{#snippet docCard(item: DocNavItem)}
	{#if item.children && item.children.length > 0}
		<div class="doc-card doc-card-group">
			<a href={pageHref(item.slug)} class="doc-card-head">
				<span class="doc-card-title">{item.title}</span>
				<ArrowUpRight size={15} class="doc-card-arrow" />
			</a>
			<ul class="doc-card-children">
				{#each item.children as child (child.id)}
					<li>
						<a href={pageHref(child.slug)} class="doc-child-link">
							<span class="doc-child-title">{child.title}</span>
							<ArrowRight size={13} class="doc-child-arrow" />
						</a>
					</li>
				{/each}
			</ul>
		</div>
	{:else}
		<a href={pageHref(item.slug)} class="doc-card doc-card-link">
			<FileText size={16} class="doc-link-icon" />
			<span class="doc-card-title">{item.title}</span>
			<ArrowRight size={15} class="doc-card-arrow" />
		</a>
	{/if}
{/snippet}

<style>
	.audience-page {
		max-width: 860px;
		margin: 0 auto;
		padding: 2.5rem 1.5rem 4rem;
	}
	.audience-header { margin-bottom: 2rem; }
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
	.audience-title-row {
		display: flex;
		align-items: flex-start;
		gap: 0.875rem;
	}
	.audience-icon {
		display: grid;
		place-items: center;
		height: 2.75rem;
		width: 2.75rem;
		border-radius: var(--radius-card);
		background: color-mix(in srgb, var(--color-primary) 10%, transparent);
		color: var(--color-primary);
		flex-shrink: 0;
	}
	.audience-title-body { min-width: 0; }
	.audience-title-body h1 {
		font-family: var(--font-sans);
		font-size: 1.75rem;
		font-weight: 700;
		color: var(--color-text);
		margin: 0;
		letter-spacing: -0.02em;
		line-height: 1.2;
	}
	.audience-desc {
		font-size: 0.9375rem;
		color: var(--color-text-subtle);
		margin: 0.375rem 0 0;
		line-height: 1.55;
		max-width: 60ch;
	}
	.audience-count {
		font-size: 0.75rem;
		color: var(--color-muted);
		margin: 0.75rem 0 0;
	}
	.audience-actions { margin-top: 1rem; }
	.new-page-link {
		display: inline-flex;
		align-items: center;
		gap: 0.375rem;
		padding: 0.5rem 0.875rem;
		border-radius: var(--radius);
		border: 1px solid var(--color-border);
		background: var(--color-surface);
		color: var(--color-primary);
		font-size: 0.8125rem;
		font-weight: 600;
		text-decoration: none;
		transition: border-color 0.15s;
	}
	.new-page-link:hover { border-color: var(--color-primary); }
	.created-notice {
		margin: 0 0 1.5rem;
		padding: 0.75rem 1rem;
		border-radius: var(--radius);
		border: 1px solid color-mix(in srgb, var(--color-primary) 35%, var(--color-border));
		background: color-mix(in srgb, var(--color-primary) 7%, transparent);
		color: var(--color-text);
		font-size: 0.875rem;
	}
	.cat-section { margin-top: 2rem; }
	.cat-title {
		font-family: var(--font-sans);
		font-size: 1.125rem;
		font-weight: 700;
		color: var(--color-text);
		margin: 0 0 0.875rem;
		letter-spacing: -0.01em;
	}
	.subcat-title {
		font-family: var(--font-sans);
		font-size: 0.8125rem;
		font-weight: 600;
		text-transform: uppercase;
		letter-spacing: 0.05em;
		color: var(--color-muted);
		margin: 1.25rem 0 0.625rem;
	}
	.empty {
		padding: 2rem;
		text-align: center;
		color: var(--color-muted);
		font-size: 0.9375rem;
		border: 1px dashed var(--color-border);
		border-radius: var(--radius-card);
	}
	.doc-grid {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(320px, 1fr));
		gap: 0.875rem;
		align-items: start;
	}
	.doc-card {
		border: 1px solid var(--color-border);
		border-radius: var(--radius-card);
		background: var(--color-surface);
		transition: border-color 0.15s, box-shadow 0.15s;
		overflow: hidden;
	}
	.doc-card-link {
		display: flex;
		align-items: center;
		gap: 0.625rem;
		padding: 1rem 1.125rem;
		text-decoration: none;
	}
	.doc-card-link:hover {
		border-color: var(--color-primary);
		box-shadow: 0 2px 10px color-mix(in srgb, var(--color-text) 5%, transparent);
	}
	.doc-card-link :global(.doc-link-icon) { color: var(--color-muted); flex-shrink: 0; }
	.doc-card-link:hover :global(.doc-link-icon) { color: var(--color-primary); }
	.doc-card-title {
		font-size: 0.9375rem;
		font-weight: 600;
		color: var(--color-text);
		flex: 1;
		min-width: 0;
	}
	.doc-card-group { display: flex; flex-direction: column; }
	.doc-card-group:hover { border-color: var(--color-primary); }
	.doc-card-head {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 0.5rem;
		padding: 1rem 1.125rem;
		text-decoration: none;
	}
	.doc-card-head:hover .doc-card-title { color: var(--color-primary); }
	.doc-card-children {
		list-style: none;
		padding: 0.25rem 0.375rem 0.625rem;
		margin: 0;
		border-top: 1px solid var(--color-border);
	}
	.doc-child-link {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 0.5rem;
		padding: 0.4375rem 0.75rem;
		border-radius: var(--radius);
		text-decoration: none;
	}
	.doc-child-link:hover { background: var(--color-surface-subtle); }
	.doc-child-title {
		font-size: 0.8125rem;
		color: var(--color-text-subtle);
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.doc-child-link:hover .doc-child-title { color: var(--color-text); }
	:global(.doc-card-arrow), :global(.doc-child-arrow) {
		color: var(--color-muted);
		flex-shrink: 0;
	}
	.doc-card-head:hover :global(.doc-card-arrow),
	.doc-card-link:hover :global(.doc-card-arrow),
	.doc-child-link:hover :global(.doc-child-arrow) {
		color: var(--color-primary);
	}
	.unpublished-section .cat-title {
		display: flex;
		align-items: center;
		gap: 0.5rem;
	}
	.unpublished-count {
		font-size: 0.6875rem;
		font-weight: 700;
		background: var(--color-surface-subtle);
		color: var(--color-text-subtle);
		border: 1px solid var(--color-border);
		border-radius: 999px;
		padding: 0.0625rem 0.5rem;
		font-variant-numeric: tabular-nums;
	}
	.doc-card-draft { border-style: dashed; }
	.status-badge {
		display: inline-flex;
		align-items: center;
		font-size: 0.625rem;
		text-transform: uppercase;
		letter-spacing: 0.05em;
		font-weight: 700;
		padding: 0.125rem 0.4375rem;
		border-radius: 4px;
		background: var(--color-surface-subtle);
		color: var(--color-text-subtle);
		border: 1px solid var(--color-border);
		flex-shrink: 0;
	}
	.status-badge.status-in_review,
	.status-badge.status-approved {
		color: var(--method-patch);
		background: var(--method-patch-bg);
		border-color: transparent;
	}
</style>
