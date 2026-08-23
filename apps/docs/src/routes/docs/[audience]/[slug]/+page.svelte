<script lang="ts">
	import { untrack } from 'svelte';
	import AppShell from '$lib/components/shell/AppShell.svelte';
	import TableOfContents from '$lib/components/shell/TableOfContents.svelte';
	import DocContent from '$lib/components/docs/DocContent.svelte';
	import { t } from '$lib/i18n';
	import { page } from '$app/state';
	import { dev } from '$app/environment';
	import { env } from '$env/dynamic/public';
	import { persistNavCollapse } from '$lib/nav-collapse';
	import { ArrowLeft, ArrowRight, Pencil } from 'lucide-svelte';
	import { safeJsonLdScript } from '$lib/utils/safeHtml';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	let collapsed = $state(untrack(() => data.navCollapsed));

	async function toggleCollapse() {
		collapsed = !collapsed;
		await persistNavCollapse(collapsed);
	}

	// "Edit this page" link — only visible to users with docs.manage permission
	// (Ref: §44 ROADMAP 02.10). The admin panel doc editor URL is
	// {admin_origin}/docs?id={id}.
	let canEdit = $derived(data.canEditDocs === true);
	const adminOrigin = $derived((dev ? 'http://localhost:3001' : (env.PUBLIC_ADMIN_ORIGIN ?? '')).replace(/\/$/, ''));
	const editUrl = $derived(adminOrigin ? `${adminOrigin}/docs?id=${data.page.id}` : '');

	// Dynamic hreflang from enabled languages (§18, ADR-013). The canonical
	// list lives in the public site settings (i18n.enabled_languages). If the
	// setting is not available, fall back to the active locale rather than a
	// hardcoded list (Rule 16).
	const enabledLanguages = $derived(
		((page.data?.siteSettings?.['i18n.enabled_languages'] as string | undefined) ?? data.locale)
			.split(',').map((l) => l.trim()).filter(Boolean)
	);
	const defaultLanguage = $derived(
		(page.data?.siteSettings?.['i18n.default_language'] as string | undefined) ?? 'en'
	);
</script>

<svelte:head>
	<title>{data.meta.title}</title>
	<meta name="description" content={data.meta.description} />
	<link rel="canonical" href={data.meta.ogUrl} />
	{#if data.isGated}
		<meta name="robots" content="noindex, nofollow" />
	{:else}
		<meta name="robots" content="index, follow" />
		<!-- hreflang for public audiences (Ref: §18 ROADMAP 07.05) -->
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
	{/if}
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
	activeAudience={data.audience}
	user={data.user}
	collapsed={collapsed}
	onToggleCollapse={toggleCollapse}
>
	{#snippet sidebar()}
		<TableOfContents headings={data.toc} />
	{/snippet}

	<article class="doc-article">
		<header class="doc-header">
			<!-- Breadcrumb (Ref: §44 ROADMAP 02.03) -->
			{#if data.page.breadcrumb && data.page.breadcrumb.length > 0}
				<nav class="breadcrumb" aria-label="Breadcrumb">
					<a href="/docs/{data.audience}" class="crumb">
						{t(data.audienceLabelKey, data.audience)}
					</a>
					{#each data.page.breadcrumb as crumb}
						<span class="crumb-sep">/</span>
						<a href="/docs/{data.audience}/{crumb.slug}" class="crumb">{crumb.title}</a>
					{/each}
				</nav>
			{:else}
				<a href="/docs/{data.audience}" class="back-link">
					← {t('page.back_to_audience', '', { audience: t(data.audienceLabelKey, data.audience) })}
				</a>
			{/if}
			<h1>{data.page.title}</h1>
			{#if data.page.ai_generated}
				<span class="ai-flag">{t('page.ai_assisted_draft')}</span>
			{/if}
			{#if data.locale !== 'en' && data.page.content_lang === 'en'}
				<div class="translation-notice">
					{t('page.not_translated', 'This page is not yet translated. Showing the English version.')}
				</div>
			{/if}
		</header>

		<div class="prose">
			<DocContent
				pageId={data.page.id}
				audience={data.audience}
				slug={data.page.slug}
				html={data.html}
				lastReviewedAt={data.page.last_reviewed_at}
				lastReviewedBy={data.page.last_reviewed_by}
			/>
		</div>

		<!-- Prev/Next navigation (Ref: §44 ROADMAP 02.05) -->
		{#if data.page.prev || data.page.next}
			<nav class="prev-next" aria-label="Page navigation">
				{#if data.page.prev}
					<a href="/docs/{data.audience}/{data.page.prev.slug}" class="pn-card pn-prev">
						<span class="pn-label"><ArrowLeft size={14} /> {t('page.previous', 'Previous')}</span>
						<span class="pn-title">{data.page.prev.title}</span>
					</a>
				{:else}
					<span class="pn-card pn-placeholder"></span>
				{/if}
				{#if data.page.next}
					<a href="/docs/{data.audience}/{data.page.next.slug}" class="pn-card pn-next">
						<span class="pn-label">{t('page.next', 'Next')} <ArrowRight size={14} /></span>
						<span class="pn-title">{data.page.next.title}</span>
					</a>
				{:else}
					<span class="pn-card pn-placeholder"></span>
				{/if}
			</nav>
		{/if}

		<!-- Edit this page link (Ref: §44 ROADMAP 02.10 — admin/superadmin only) -->
		{#if canEdit && editUrl}
			<div class="edit-link-row">
				<a href={editUrl} class="edit-link" target="_blank" rel="noopener">
					<Pencil size={14} />
					{t('page.edit_this_page', 'Edit this page')}
				</a>
			</div>
		{/if}
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
	.ai-flag {
		display: inline-block;
		margin-top: 0.75rem;
		font-size: 0.6875rem;
		text-transform: uppercase;
		letter-spacing: 0.05em;
		color: var(--method-patch);
		background: var(--method-patch-bg);
		padding: 0.125rem 0.5rem;
		border-radius: 4px;
		font-weight: 600;
	}
	.translation-notice {
		margin-top: 0.75rem;
		padding: 0.5rem 0.75rem;
		border: 1px solid var(--method-patch-bg);
		background: var(--method-patch-bg);
		color: var(--method-patch);
		border-radius: var(--radius);
		font-size: 0.8125rem;
	}
	.breadcrumb {
		display: flex;
		align-items: center;
		flex-wrap: wrap;
		gap: 0.25rem;
		font-size: 0.8125rem;
		margin-bottom: 0.5rem;
	}
	.crumb {
		color: var(--color-muted);
		text-decoration: none;
	}
	.crumb:hover { color: var(--color-primary); }
	.crumb-sep {
		color: var(--color-muted);
		font-size: 0.75rem;
	}
	.prev-next {
		display: grid;
		grid-template-columns: 1fr 1fr;
		gap: 1rem;
		margin-top: 3rem;
		padding-top: 2rem;
		border-top: 1px solid var(--color-border);
	}
	.pn-card {
		display: flex;
		flex-direction: column;
		gap: 0.25rem;
		padding: 1rem 1.25rem;
		border: 1px solid var(--color-border);
		border-radius: var(--radius-card);
		background: var(--color-surface);
		text-decoration: none;
		transition: border-color 0.15s;
	}
	.pn-card:hover { border-color: var(--color-primary); }
	.pn-placeholder { visibility: hidden; }
	.pn-next { text-align: right; }
	.pn-label {
		display: inline-flex;
		align-items: center;
		gap: 0.25rem;
		font-size: 0.75rem;
		color: var(--color-muted);
		text-transform: uppercase;
		letter-spacing: 0.05em;
	}
	.pn-next .pn-label { justify-content: flex-end; }
	.pn-title {
		font-size: 0.9375rem;
		font-weight: 600;
		color: var(--color-text);
	}
	.edit-link-row {
		margin-top: 1.5rem;
		padding-top: 1.5rem;
		border-top: 1px solid var(--color-border);
	}
	.edit-link {
		display: inline-flex;
		align-items: center;
		gap: 0.375rem;
		font-size: 0.8125rem;
		color: var(--color-muted);
		text-decoration: none;
	}
	.edit-link:hover { color: var(--color-primary); }
</style>
