<script lang="ts">
	import AppShell from '$lib/components/shell/AppShell.svelte';
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
	<section class="audience-page">
		<header class="audience-header">
			<a href="/docs" class="back-link">← {t('page.back_to_docs')}</a>
			<h1>{t(data.labelKey, data.audience)}</h1>
		</header>

		{#if data.pages.length === 0}
			<p class="empty">{t('nav.empty')}</p>
		{:else}
			<ul class="page-list">
				{#each data.pages as page (page.id)}
					<li>
						<a href="/docs/{data.audience}/{page.slug}" class="page-card">
							<span class="page-title">{page.title}</span>
							<span class="page-meta">
								{#if page.review_status === 'published'}
									<span class="badge published">published</span>
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
	.empty {
		padding: 2rem;
		text-align: center;
		color: var(--color-muted);
		font-size: 0.9375rem;
		border: 1px dashed var(--color-border);
		border-radius: var(--radius-card);
	}
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
