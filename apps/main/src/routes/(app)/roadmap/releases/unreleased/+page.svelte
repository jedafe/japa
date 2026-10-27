<script lang="ts">
	import type { PageData } from './$types';
	import { t, siteName } from '$lib/i18n';
	import { ArrowLeft, GitCommitHorizontal } from 'lucide-svelte';
	import EmptyState from '$lib/components/ui/EmptyState.svelte';
	import type { RoadmapReleaseEntry } from '$lib/types';

	/**
	 * Unreleased queue — public-type changelog entries landed on main but
	 * not yet absorbed into a published release.
	 * Ref: japaconnect-engineering/sections/113-public-product-roadmap.md
	 */
	let { data } = $props<{ data: PageData }>();

	const entries = $derived((data.entries ?? []) as RoadmapReleaseEntry[]);

	const groups = $derived.by(() => {
		const order = ['feature', 'fix', 'ui', 'performance', 'security', 'translation', 'accessibility'];
		const m = new Map<string, RoadmapReleaseEntry[]>();
		for (const e of entries) {
			const list = m.get(e.commit_type) ?? [];
			list.push(e);
			m.set(e.commit_type, list);
		}
		return order.filter((k) => m.has(k)).map((k) => ({ type: k, entries: m.get(k)! }));
	});
</script>

<svelte:head>
	<title>{t('roadmap.unreleased.title', 'In development')} — {siteName()}</title>
</svelte:head>

<div class="wrap">
	<a class="back muted" href="/roadmap/releases">
		<ArrowLeft size={14} /> {t('roadmap.releases.back', 'All releases')}
	</a>
	<h1>{t('roadmap.unreleased.title', 'In development')}</h1>
	<p class="muted">
		{t('roadmap.unreleased.subtitle', 'Changes landed but not yet shipped in a release')}
	</p>

	{#if data.loadError}
		<EmptyState
			icon={GitCommitHorizontal}
			title={t('roadmap.releases.error_title', 'Could not load entries')}
			description={t('roadmap.releases.error_desc', 'Something went wrong. Please try again later.')}
		/>
	{:else if entries.length === 0}
		<EmptyState
			icon={GitCommitHorizontal}
			title={t('roadmap.unreleased.empty_title', 'Nothing queued')}
			description={t('roadmap.unreleased.empty_desc', 'New commits will appear here as they land.')}
		/>
	{:else}
		{#each groups as g}
			<section class="group">
				<h2>{t('roadmap.type_' + g.type, g.type)} <span class="n">({g.entries.length})</span></h2>
				<ul>
					{#each g.entries as e}
						<li>
							{#if e.is_breaking}
								<span class="breaking">{t('roadmap.breaking', 'BREAKING')}</span>
							{/if}
							{#if e.commit_url}
								<a href={e.commit_url} target="_blank" rel="noopener">{e.title}</a>
							{:else}
								{e.title}
							{/if}
							{#if e.scope}
								<span class="scope">{e.scope}</span>
							{/if}
						</li>
					{/each}
				</ul>
			</section>
		{/each}
	{/if}
</div>

<style>
	.wrap {
		max-width: 48rem;
		margin: 0 auto;
		padding: 2rem 1rem 4rem;
	}
	.back {
		display: inline-flex;
		align-items: center;
		gap: 0.3rem;
		font-size: 0.85rem;
		text-decoration: none;
	}
	.muted {
		color: var(--color-muted, #6b7280);
	}
	h1 {
		font-size: 1.5rem;
		font-weight: 700;
		margin: 0.75rem 0 0.25rem;
	}
	h2 {
		font-size: 1rem;
		font-weight: 700;
		margin: 1.5rem 0 0.5rem;
	}
	h2 .n {
		font-weight: 400;
		color: var(--color-muted, #6b7280);
	}
	ul {
		list-style: none;
		padding: 0;
		margin: 0;
		display: flex;
		flex-direction: column;
		gap: 0.4rem;
	}
	li {
		font-size: 0.9rem;
		display: flex;
		align-items: baseline;
		gap: 0.5rem;
		flex-wrap: wrap;
	}
	li a {
		color: var(--color-primary, #6d28d9);
		text-decoration: none;
	}
	li a:hover {
		text-decoration: underline;
	}
	.breaking {
		font-size: 0.65rem;
		font-weight: 700;
		text-transform: uppercase;
		background: color-mix(in srgb, #dc2626 15%, transparent);
		color: #dc2626;
		border-radius: 0.25rem;
		padding: 0.1rem 0.35rem;
	}
	.scope {
		font-size: 0.7rem;
		color: var(--color-muted, #6b7280);
		border: 1px solid var(--color-border, #e5e7eb);
		border-radius: 999px;
		padding: 0.05rem 0.5rem;
	}
</style>
