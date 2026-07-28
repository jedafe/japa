<script lang="ts">
	import { page } from '$app/state';
	import AppShell from '$lib/components/shell/AppShell.svelte';
	import { t } from '$lib/i18n';
	import type { LayoutData } from '../$types';

	let { data }: { data: LayoutData } = $props();
</script>

<AppShell
	nav={data.nav}
	accessibleAudiences={data.accessibleAudiences}
	activeAudience={null}
	user={data.user}
	collapsed={data.navCollapsed}
	onToggleCollapse={() => {}}
>
	<section class="not-found">
		<h1>{page.status}</h1>
		<p>{page.status === 404 ? t('page.not_found_body') : page.error?.message ?? t('page.not_found_body')}</p>
		<a href="/docs" class="back">{t('page.back_to_docs')}</a>
	</section>
</AppShell>

<style>
	.not-found {
		max-width: 560px;
		margin: 0 auto;
		padding: 4rem 1.5rem;
		text-align: center;
	}
	.not-found h1 {
		font-family: var(--font-sans);
		font-size: 3rem;
		font-weight: 700;
		color: var(--color-muted);
		margin: 0 0 0.5rem;
	}
	.not-found p {
		font-size: 0.9375rem;
		color: var(--color-text-subtle);
		margin: 0 0 1.5rem;
	}
	.back {
		display: inline-block;
		padding: 0.5rem 1rem;
		border: 1px solid var(--color-border);
		border-radius: var(--radius);
		color: var(--color-text);
		text-decoration: none;
		font-size: 0.875rem;
		font-weight: 500;
	}
	.back:hover { border-color: var(--color-primary); color: var(--color-primary); }
</style>
