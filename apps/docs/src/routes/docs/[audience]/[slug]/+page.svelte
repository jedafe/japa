<script lang="ts">
	import AppShell from '$lib/components/shell/AppShell.svelte';
	import TableOfContents from '$lib/components/shell/TableOfContents.svelte';
	import DocContent from '$lib/components/docs/DocContent.svelte';
	import { t } from '$lib/i18n';
	import { persistNavCollapse } from '$lib/nav-collapse';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	let collapsed = $state(data.navCollapsed);

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
		<script type="application/ld+json">{data.meta.jsonLdString}</script>
	{/if}
</svelte:head>

<AppShell
	nav={data.nav}
	accessibleAudiences={data.accessibleAudiences}
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
			<a href="/docs/{data.audience}" class="back-link">
				← {t('page.back_to_audience', '', { audience: t(data.audienceLabelKey, data.audience) })}
			</a>
			<h1>{data.page.title}</h1>
			{#if data.page.ai_generated}
				<span class="ai-flag">AI-assisted draft</span>
			{/if}
		</header>

		<div class="prose">
			<DocContent
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
</style>
