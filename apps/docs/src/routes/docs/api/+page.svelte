<script lang="ts">
	import { untrack } from 'svelte';
	import AppShell from '$lib/components/shell/AppShell.svelte';
	import { t } from '$lib/i18n';
	import { safeJsonLdScript } from '$lib/utils/safeHtml';
	import { persistNavCollapse } from '$lib/nav-collapse';
	import { Play, Download } from 'lucide-svelte';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	let collapsed = $state(untrack(() => data.navCollapsed));

	async function toggleCollapse() {
		collapsed = !collapsed;
		await persistNavCollapse(collapsed);
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
					<ul class="endpoint-list">
						{#each data.endpoints as ep (`${ep.method}${ep.path}`)}
							<li class="endpoint-item">
								<span class="method-badge method-{ep.method}">{ep.method}</span>
								<code class="endpoint-path">{ep.path}</code>
								{#if ep.summary}
									<span class="endpoint-summary">{ep.summary}</span>
								{/if}
							</li>
						{/each}
					</ul>
				</div>
			{:else}
				<p class="empty">{t('nav.empty', 'No pages yet')}</p>
			{/if}
		{:else}
			<ul class="page-list">
				{#each data.pages as page (page.id)}
					<li>
						<a href="/docs/api/{page.slug}" class="page-card">
							<span class="page-title">{page.title}</span>
							<span class="page-meta">
								{#if page.review_status === 'published'}
									<span class="badge published">{t('page.status_published')}</span>
								{:else}
									<span class="badge">{page.review_status}</span>
								{/if}
							</span>
						</a>
					</li>
				{/each}
			</ul>
		{/if}
	</section>
</AppShell>

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
	.page-list {
		list-style: none;
		padding: 0;
		margin: 0;
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
	}
	.page-card {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 0.75rem;
		padding: 0.875rem 1rem;
		border: 1px solid var(--color-border);
		border-radius: var(--radius);
		background: var(--color-surface);
		text-decoration: none;
		transition: border-color 0.15s;
	}
	.page-card:hover { border-color: var(--color-primary); }
	.page-title { font-size: 0.9375rem; font-weight: 500; color: var(--color-text); }
	.badge {
		font-size: 0.6875rem;
		text-transform: uppercase;
		letter-spacing: 0.05em;
		color: var(--color-muted);
		padding: 0.125rem 0.5rem;
		border-radius: 4px;
		background: var(--color-surface-subtle);
		flex-shrink: 0;
	}
	.badge.published { color: var(--method-get); background: var(--method-get-bg); }
</style>
