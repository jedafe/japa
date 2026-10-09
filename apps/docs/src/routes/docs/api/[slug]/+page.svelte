<script lang="ts">
	import { untrack } from 'svelte';
	import AppShell from '$lib/components/shell/AppShell.svelte';
	import TableOfContents from '$lib/components/shell/TableOfContents.svelte';
	import DocContent from '$lib/components/docs/DocContent.svelte';
	import ApiRefTabs from '$lib/components/docs/ApiRefTabs.svelte';
	import MethodBadge from '$lib/components/docs/MethodBadge.svelte';
	import { t } from '$lib/i18n';
	import { safeJsonLdScript } from '$lib/utils/safeHtml';
	import { persistNavCollapse } from '$lib/nav-collapse';
	import { page } from '$app/state';
	import { env } from '$env/dynamic/public';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

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

	// OpenAPI paths are relative to the spec's server URL (…/api/v1) — prepend
	// the configured API base so the sample is copy-paste runnable. Join with
	// PUBLIC_MAIN_ORIGIN when the API base is a relative path.
	const apiBaseUrl = env.PUBLIC_JC_API_URL ?? '/api/v1';
	const mainOrigin = (env.PUBLIC_MAIN_ORIGIN ?? '').replace(/\/$/, '');
	const curlBaseUrl = apiBaseUrl.startsWith('http')
		? apiBaseUrl.replace(/\/$/, '')
		: mainOrigin
			? `${mainOrigin}${apiBaseUrl}`
			: apiBaseUrl;

	// Derive code-pane samples from the matched operation (best-effort).
	let method = $derived(
		data.operation ? (data.operation.method as 'GET' | 'POST' | 'PATCH' | 'PUT' | 'DELETE') : 'GET'
	);
	let path = $derived(data.operation?.path ?? '');
	// DOC-005: public API key auth uses the X-API-Key header.
	let requestSample = $derived(
		data.operation
			? `curl -X ${method} "${curlBaseUrl}${path}" \\\n  -H "X-API-Key: $JC_API_KEY"${method !== 'GET' && method !== 'DELETE' ? ' \\\n  -H "Content-Type: application/json"' : ''}`
			: ''
	);
	let responseSample = $derived(
		data.operation
			? `// 200 OK\n{\n  "data": { ... }\n}`
			: ''
	);
	let exampleSample = $derived(requestSample);
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
	variant="api"
>
	{#snippet apiPane()}
		<div class="api-pane">
			{#if data.operation}
				<div class="api-pane-head">
					<MethodBadge method={method} />
					<code class="api-pane-path">{path}</code>
				</div>
				<p class="api-pane-summary">{data.operation.summary ?? ''}</p>
				<ApiRefTabs
					method={method}
					path={path}
					{requestSample}
					{responseSample}
					{exampleSample}
				/>
			{:else}
				<TableOfContents headings={data.toc} />
			{/if}
		</div>
	{/snippet}

	<article class="doc-article">
		<header class="doc-header">
			<a href="/docs/api" class="back-link">← {t('audience.api')}</a>
			<h1>{data.page.title}</h1>
			{#if data.operation}
				<div class="op-line">
					<MethodBadge method={method} />
					<code class="op-path">{path}</code>
				</div>
			{/if}
		</header>

		<div class="prose">
			<DocContent
				pageId={data.page.id}
				audience={data.page.audience}
				slug={data.page.slug}
				html={data.html}
				lastReviewedAt={data.page.last_reviewed_at}
				lastReviewedBy={data.page.last_reviewed_by}
			/>
		</div>
	</article>
</AppShell>

<style>
	.doc-article {
		max-width: 820px;
		margin: 0 auto;
		padding: 2.5rem 1.5rem 4rem;
	}
	.doc-header { margin-bottom: 1.5rem; }
	.back-link {
		font-size: 0.8125rem;
		color: var(--color-muted);
		text-decoration: none;
	}
	.back-link:hover { color: var(--color-primary); }
	.doc-header h1 {
		font-family: var(--font-sans);
		font-size: 2.25rem;
		font-weight: 700;
		color: var(--color-text);
		margin: 0.5rem 0 0;
		letter-spacing: -0.02em;
	}
	.op-line {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		margin-top: 0.75rem;
		flex-wrap: wrap;
	}
	.op-path {
		font-family: var(--font-mono);
		font-size: 0.875rem;
		color: var(--color-text);
		background: var(--color-surface-subtle);
		padding: 0.25rem 0.5rem;
		border-radius: 4px;
	}
	.api-pane { display: flex; flex-direction: column; gap: 0.75rem; }
	.api-pane-head {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		flex-wrap: wrap;
	}
	.api-pane-path {
		font-family: var(--font-mono);
		font-size: 0.75rem;
		color: var(--color-text);
		background: var(--color-surface-subtle);
		padding: 0.125rem 0.375rem;
		border-radius: 4px;
		word-break: break-all;
	}
	.api-pane-summary {
		font-size: 0.8125rem;
		color: var(--color-text-subtle);
		margin: 0;
	}
</style>
