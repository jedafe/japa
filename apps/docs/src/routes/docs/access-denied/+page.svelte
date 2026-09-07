<script lang="ts">
	import { untrack } from 'svelte';
	import AppShell from '$lib/components/shell/AppShell.svelte';
	import { t } from '$lib/i18n';
	import { persistNavCollapse } from '$lib/nav-collapse';
	import { AUDIENCE_LABEL_KEY } from '$lib/types';
	import { Lock, LogIn, BookOpen } from 'lucide-svelte';
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
	<meta name="robots" content="noindex" />
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
	<section class="denied">
		<div class="denied-icon"><Lock size={32} /></div>
		<h1>{t('audience.no_access_title')}</h1>
		<p class="denied-body">{t('audience.no_access_body')}</p>
		<div class="denied-actions">
			{#if !data.user}
				<a href={data.loginUrl} class="btn primary">
					<LogIn size={16} />
					{t('audience.no_access_signin')}
				</a>
			{/if}
			<a href="/docs/api" class="btn">
				<BookOpen size={16} />
				{t('audience.no_access_api_link')}
			</a>
		</div>

		{#if data.accessibleAudiences.length > 0}
			<div class="accessible">
				<h2>{t('audience.pick_audience')}</h2>
				<ul>
					{#each data.accessibleAudiences as audience (audience)}
						<li><a href="/docs/{audience}">{t(AUDIENCE_LABEL_KEY[audience], audience)}</a></li>
					{/each}
				</ul>
			</div>
		{/if}
	</section>
</AppShell>

<style>
	.denied {
		max-width: 560px;
		margin: 0 auto;
		padding: 4rem 1.5rem;
		text-align: center;
	}
	.denied-icon {
		display: grid;
		place-items: center;
		height: 4rem;
		width: 4rem;
		margin: 0 auto 1.5rem;
		border-radius: 9999px;
		background: var(--method-delete-bg);
		color: var(--method-delete);
	}
	.denied h1 {
		font-family: var(--font-sans);
		font-size: 1.75rem;
		font-weight: 700;
		color: var(--color-text);
		margin: 0 0 0.75rem;
	}
	.denied-body {
		font-size: 0.9375rem;
		color: var(--color-text-subtle);
		line-height: 1.6;
		margin: 0 0 1.5rem;
	}
	.denied-actions {
		display: flex;
		gap: 0.75rem;
		justify-content: center;
		flex-wrap: wrap;
	}
	.btn {
		display: inline-flex;
		align-items: center;
		gap: 0.375rem;
		padding: 0.5rem 1rem;
		border: 1px solid var(--color-border);
		border-radius: var(--radius);
		color: var(--color-text);
		text-decoration: none;
		font-size: 0.875rem;
		font-weight: 500;
	}
	.btn:hover { border-color: var(--color-border-input-hover); }
	.btn.primary {
		background: var(--color-primary);
		color: var(--color-primary-contrast);
		border-color: var(--color-primary);
	}
	.btn.primary:hover { background: var(--color-primary-hover); }
	.accessible {
		margin-top: 2.5rem;
		text-align: left;
	}
	.accessible h2 {
		font-size: 0.75rem;
		font-weight: 600;
		text-transform: uppercase;
		letter-spacing: 0.05em;
		color: var(--color-muted);
		margin: 0 0 0.75rem;
	}
	.accessible ul {
		list-style: none;
		padding: 0;
		margin: 0;
		display: flex;
		flex-direction: column;
		gap: 0.25rem;
	}
	.accessible a {
		display: block;
		padding: 0.5rem 0.75rem;
		border-radius: var(--radius);
		color: var(--color-text);
		text-decoration: none;
		font-size: 0.875rem;
		text-transform: capitalize;
	}
	.accessible a:hover { background: var(--color-surface-subtle); color: var(--color-primary); }
</style>
