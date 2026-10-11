<script lang="ts">
	import { untrack } from 'svelte';
	import AppShell from '$lib/components/shell/AppShell.svelte';
	import AutocompleteSelect from '$lib/components/ui/AutocompleteSelect.svelte';
	import { t } from '$lib/i18n';
	import { safeJsonLdScript } from '$lib/utils/safeHtml';
	import { persistNavCollapse } from '$lib/nav-collapse';
	import { groupNavItems } from '$lib/docs/grouping';
	import { page } from '$app/state';
	import { ArrowRight, ArrowUpRight, FileText, Play, Download, Plus, Search } from 'lucide-svelte';
	import type { PageData } from './$types';
	import type { DocNavItem } from '$lib/types';

	let { data }: { data: PageData } = $props();

	// Grouped page listing — same nav-tree-driven taxonomy as the other
	// audience landings (§44 amendment 2 parity).
	const navItems = $derived(
		data.nav.find((tree) => tree.audience === 'api')?.items ?? []
	);
	const grouped = $derived(groupNavItems(navItems));

	// Dynamic hreflang from enabled languages (copies [audience]/[slug] pattern).
	// NOTE: this page load has no `locale` field — read the default from the
	// request-scoped layout siteSettings, falling back to 'en' (Rule 16).
	const enabledLanguages = $derived(
		((page.data?.siteSettings?.['i18n.enabled_languages'] as string | undefined) ?? 'en')
			.split(',').map((l) => l.trim()).filter(Boolean)
	);
	const defaultLanguage = $derived(
		(page.data?.siteSettings?.['i18n.default_language'] as string | undefined) ?? 'en'
	);

	let collapsed = $state(untrack(() => data.navCollapsed));

	async function toggleCollapse() {
		collapsed = !collapsed;
		await persistNavCollapse(collapsed);
	}

	// Endpoint browser — client-side text + method filtering over the
	// OpenAPI-derived list, grouped by tag. Each row links into the
	// playground with method/path prefilled.
	let epQuery = $state('');
	let epMethod = $state('');

	const METHOD_ORDER = ['GET', 'POST', 'PUT', 'PATCH', 'DELETE', 'HEAD', 'OPTIONS'];
	let methodOptions = $derived(
		[
			{ value: '', label: t('api.all_methods', 'All methods') },
			...[...new Set(data.endpoints.map((e) => e.method))]
				.sort((a, b) => METHOD_ORDER.indexOf(a) - METHOD_ORDER.indexOf(b))
				.map((m) => ({ value: m, label: m }))
		]
	);

	let filteredEndpoints = $derived.by(() => {
		const needle = epQuery.trim().toLowerCase();
		return data.endpoints.filter(
			(ep) =>
				(!epMethod || ep.method === epMethod) &&
				(!needle ||
					ep.path.toLowerCase().includes(needle) ||
					ep.summary.toLowerCase().includes(needle))
		);
	});

	let groupedEndpoints = $derived.by(() => {
		const groups = new Map<string, typeof filteredEndpoints>();
		for (const ep of filteredEndpoints) {
			const list = groups.get(ep.tag) ?? [];
			list.push(ep);
			groups.set(ep.tag, list);
		}
		return [...groups.entries()].sort(([a], [b]) => a.localeCompare(b));
	});

	function playgroundHref(ep: { method: string; path: string }): string {
		return `/docs/api/playground?method=${encodeURIComponent(ep.method)}&path=${encodeURIComponent(ep.path)}`;
	}
</script>

<svelte:head>
	<title>{data.meta.title}</title>
	<meta name="description" content={data.meta.description} />
	<link rel="canonical" href={data.meta.ogUrl} />
	{#each enabledLanguages as lang (lang)}
		<link
			rel="alternate"
			hreflang={lang}
			href={lang === defaultLanguage
				? data.meta.ogUrl
				: data.meta.ogUrl.replace(/\/docs\//, `/${lang}/docs/`)}
		/>
	{/each}
	<link rel="alternate" hreflang="x-default" href={data.meta.ogUrl} />
	<meta property="og:title" content={data.meta.title} />
	<meta property="og:description" content={data.meta.description} />
	<meta property="og:type" content={data.meta.ogType} />
	<meta property="og:url" content={data.meta.ogUrl} />
	{#if data.meta.ogImage}
		<meta property="og:image" content={data.meta.ogImage} />
	{/if}
	<meta name="twitter:card" content={data.meta.twitterCard} />
	<meta name="twitter:title" content={data.meta.twitterTitle} />
	<meta name="twitter:description" content={data.meta.twitterDescription} />
	{#if data.meta.twitterImage}
		<meta name="twitter:image" content={data.meta.twitterImage} />
	{/if}
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
	activeAudience="api"
	user={data.user}
	collapsed={collapsed}
	onToggleCollapse={toggleCollapse}
>
	<section class="audience-page">
		<header class="audience-header">
			<a href="/docs" class="back-link">← {t('page.back_to_docs')}</a>
			<h1>{t('audience.api', 'API')}</h1>
			<p class="audience-desc">
				{t('api.landing_desc', 'REST API reference, authentication guides, and an interactive playground for testing endpoints.')}
			</p>
			{#if data.canEditDocs}
				<div class="audience-actions">
					<a href="/docs/api/new" class="new-page-link">
						<Plus size={15} />
						{t('audience.new_page', 'New page')}
					</a>
				</div>
			{/if}
		</header>

		<a href="/docs/api/playground" class="playground-card">
			<Play size={18} />
			<div>
				<span class="pg-title">{t('api.playground_title', 'API Playground')}</span>
				<span class="pg-sub">{t('api.playground_subtitle', 'Test live API requests')}</span>
			</div>
		</a>

		<a href="/docs/openapi.json" class="spec-download-card" download>
			<Download size={18} />
			<div>
				<span class="pg-title">{t('api.download_spec', 'Download OpenAPI Spec')}</span>
				<span class="pg-sub">{t('api.download_spec_sub', 'OpenAPI 3.x JSON')}</span>
			</div>
		</a>

		{#if data.pages.length === 0}
			{#if data.endpoints.length > 0}
				<div class="spec-endpoints">
					<h2 class="spec-heading">{t('api.endpoints_from_spec')}</h2>
					<p class="spec-hint">{t('api.spec_endpoints_hint')}</p>

					<div class="endpoint-filters">
						<div class="ep-search-wrap">
							<Search size={14} class="ep-search-icon" />
							<input
								type="text"
								bind:value={epQuery}
								placeholder={t('api.filter_endpoints', 'Filter endpoints…')}
								aria-label={t('api.filter_endpoints', 'Filter endpoints')}
								class="ep-search"
							/>
						</div>
						<AutocompleteSelect
							bind:value={epMethod}
							options={methodOptions}
							ariaLabel={t('api.method', 'Method')}
							class="ep-method-select"
						/>
					</div>

					{#if groupedEndpoints.length === 0}
						<p class="empty">{t('common.no_results', 'No results found')}</p>
					{:else}
						{#each groupedEndpoints as [tag, eps] (tag)}
							<div class="endpoint-group">
								<h3 class="endpoint-tag">{tag}</h3>
								<ul class="endpoint-list">
									{#each eps as ep (`${ep.method}${ep.path}`)}
										<li>
											<a href={playgroundHref(ep)} class="endpoint-item">
												<span class="method-badge method-{ep.method}">{ep.method}</span>
												<code class="endpoint-path">{ep.path}</code>
												{#if ep.summary}
													<span class="endpoint-summary">{ep.summary}</span>
												{/if}
												<span class="endpoint-cta">{t('api.try_it', 'Try it →')}</span>
											</a>
										</li>
									{/each}
								</ul>
							</div>
						{/each}
					{/if}
				</div>
			{:else}
				<p class="empty">{t('nav.empty', 'No pages yet')}</p>
			{/if}
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
		{/if}

		<!-- Editor-only: unpublished API docs (§44 amendment 2) -->
		{#if data.unpublishedPages.length > 0}
			<section class="cat-section unpublished-section">
				<h2 class="cat-title">
					{t('audience.unpublished', 'In progress')}
					<span class="unpublished-count">{data.unpublishedPages.length}</span>
				</h2>
				<div class="doc-grid">
					{#each data.unpublishedPages as item (item.id)}
						<a href="/docs/api/{item.slug}" class="doc-card doc-card-link doc-card-draft">
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
	</section>
</AppShell>

{#snippet docCard(item: DocNavItem)}
	{#if item.children && item.children.length > 0}
		<div class="doc-card doc-card-group">
			<a href="/docs/api/{item.slug}" class="doc-card-head">
				<span class="doc-card-title">{item.title}</span>
				<ArrowUpRight size={15} class="doc-card-arrow" />
			</a>
			<ul class="doc-card-children">
				{#each item.children as child (child.id)}
					<li>
						<a href="/docs/api/{child.slug}" class="doc-child-link">
							<span class="doc-child-title">{child.title}</span>
							<ArrowRight size={13} class="doc-child-arrow" />
						</a>
					</li>
				{/each}
			</ul>
		</div>
	{:else}
		<a href="/docs/api/{item.slug}" class="doc-card doc-card-link">
			<FileText size={16} class="doc-link-icon" />
			<span class="doc-card-title">{item.title}</span>
			<ArrowRight size={15} class="doc-card-arrow" />
		</a>
	{/if}
{/snippet}

<style>
	.audience-page {
		max-width: 760px;
		margin: 0 auto;
		padding: 2.5rem 1.5rem 4rem;
	}
	.audience-header { margin-bottom: 2rem; }
	.back-link {
		font-size: 0.8125rem;
		color: var(--color-muted);
		text-decoration: none;
	}
	.back-link:hover { color: var(--color-primary); }
	.audience-header h1 {
		font-family: var(--font-sans);
		font-size: 2rem;
		font-weight: 700;
		color: var(--color-text);
		margin: 0.5rem 0 0;
		letter-spacing: -0.02em;
		text-transform: capitalize;
	}
	.audience-desc {
		font-size: 0.9375rem;
		color: var(--color-text-subtle);
		margin: 0.5rem 0 0;
		line-height: 1.5;
	}
	.playground-card {
		display: flex;
		align-items: center;
		gap: 0.75rem;
		padding: 1rem 1.25rem;
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-card);
		background: var(--color-surface);
		text-decoration: none;
		margin-bottom: 1.5rem;
		transition: background 0.15s;
		color: var(--color-primary);
	}
	.playground-card:hover { background: var(--color-surface-subtle); }
	.spec-download-card {
		display: flex;
		align-items: center;
		gap: 0.75rem;
		padding: 1rem 1.25rem;
		border: 1px solid var(--color-border);
		border-radius: var(--radius-card);
		background: var(--color-surface);
		text-decoration: none;
		margin-bottom: 1.5rem;
		transition: background 0.15s, border-color 0.15s;
		color: var(--color-text);
	}
	.spec-download-card:hover {
		background: var(--color-surface-subtle);
		border-color: var(--color-primary);
	}
	.spec-download-card .pg-title {
		display: block;
		font-size: 0.9375rem;
		font-weight: 600;
		color: var(--color-text);
	}
	.spec-download-card .pg-sub {
		display: block;
		font-size: 0.8125rem;
		color: var(--color-muted);
		margin-top: 0.125rem;
	}
	.playground-card .pg-title {
		display: block;
		font-size: 0.9375rem;
		font-weight: 600;
		color: var(--color-text);
	}
	.playground-card .pg-sub {
		display: block;
		font-size: 0.8125rem;
		color: var(--color-muted);
		margin-top: 0.125rem;
	}
	.empty {
		padding: 2rem;
		text-align: center;
		color: var(--color-muted);
		font-size: 0.9375rem;
		border: 1px dashed var(--color-border);
		border-radius: var(--radius-card);
	}
	.spec-endpoints { margin-top: 1.5rem; }
	.spec-heading {
		font-size: 1rem;
		font-weight: 600;
		color: var(--color-text);
		margin: 0 0 0.5rem;
	}
	.spec-hint {
		font-size: 0.8125rem;
		color: var(--color-muted);
		margin: 0 0 1rem;
	}
	.endpoint-filters {
		display: flex;
		align-items: center;
		gap: 0.75rem;
		margin-bottom: 1.25rem;
	}
	.ep-search-wrap {
		position: relative;
		flex: 1;
		min-width: 0;
	}
	.ep-search-wrap :global(.ep-search-icon) {
		position: absolute;
		left: 0.625rem;
		top: 50%;
		transform: translateY(-50%);
		color: var(--color-muted);
		pointer-events: none;
	}
	.ep-search {
		width: 100%;
		padding: 0.5rem 0.75rem 0.5rem 2rem;
		border: 1px solid var(--color-border);
		border-radius: var(--radius);
		background: var(--color-surface);
		color: var(--color-text);
		font-size: 0.875rem;
		outline: none;
	}
	.ep-search:focus { border-color: var(--color-primary); }
	.endpoint-filters :global(.ep-method-select) {
		width: 9rem;
		flex-shrink: 0;
	}
	.endpoint-filters :global(.ep-method-select .autocomplete-input-wrapper) {
		height: 2.375rem;
	}
	.endpoint-group { margin-bottom: 1.25rem; }
	.endpoint-tag {
		font-size: 0.75rem;
		font-weight: 600;
		text-transform: uppercase;
		letter-spacing: 0.05em;
		color: var(--color-muted);
		margin: 0 0 0.375rem;
	}
	.endpoint-list {
		list-style: none;
		padding: 0;
		margin: 0;
		display: flex;
		flex-direction: column;
		gap: 0.375rem;
	}
	.endpoint-item {
		display: flex;
		align-items: center;
		gap: 0.625rem;
		padding: 0.5rem 0.75rem;
		border: 1px solid var(--color-border);
		border-radius: var(--radius);
		background: var(--color-surface);
		text-decoration: none;
		transition: border-color 0.15s;
	}
	.endpoint-item:hover { border-color: var(--color-primary); }
	.endpoint-cta {
		font-size: 0.75rem;
		font-weight: 500;
		color: var(--color-primary);
		white-space: nowrap;
		flex-shrink: 0;
	}
	.endpoint-item:not(:hover) .endpoint-cta {
		visibility: hidden;
	}
	.endpoint-path {
		font-family: var(--font-mono, monospace);
		font-size: 0.8125rem;
		color: var(--color-text);
	}
	.endpoint-summary {
		font-size: 0.8125rem;
		color: var(--color-muted);
		margin-left: auto;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.method-badge {
		font-size: 0.625rem;
		font-weight: 700;
		text-transform: uppercase;
		padding: 0.125rem 0.375rem;
		border-radius: 3px;
		flex-shrink: 0;
	}
	.method-GET { color: var(--method-get); background: var(--method-get-bg); }
	.method-POST { color: var(--method-post); background: var(--method-post-bg); }
	.method-PATCH { color: var(--method-patch); background: var(--method-patch-bg); }
	.method-PUT { color: var(--method-put); background: var(--method-put-bg); }
	.method-DELETE { color: var(--method-delete); background: var(--method-delete-bg); }
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
