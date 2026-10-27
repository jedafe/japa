<script lang="ts">
	import type { PageData } from './$types';
	import { t, siteName } from '$lib/i18n';
	import { ArrowLeft, Package } from 'lucide-svelte';
	import EmptyState from '$lib/components/ui/EmptyState.svelte';
	import { markdownToHtml } from '$lib/studio/markdown-html';
	import type { RoadmapReleaseEntry } from '$lib/types';

	/**
	 * Release detail — version header with support-state chip, editorial
	 * body, shipped roadmap items, and commit entries grouped by type.
	 * ?scope= filters items + entries by extension.
	 * Ref: japaconnect-engineering/sections/113-public-product-roadmap.md
	 */
	let { data } = $props<{ data: PageData }>();

	const rel = $derived(data.release);
	const scopes = $derived.by(() => {
		const s = new Set<string>();
		for (const e of rel?.entries ?? []) {
			if (e.scope) s.add(e.scope);
		}
		for (const i of rel?.items ?? []) {
			if (i.feature_flag_key) s.add(i.feature_flag_key);
		}
		return [...s].sort();
	});

	const groups = $derived.by(() => {
		const order = ['feature', 'fix', 'ui', 'performance', 'security', 'translation', 'accessibility'];
		const m = new Map<string, RoadmapReleaseEntry[]>();
		for (const e of rel?.entries ?? []) {
			const list = m.get(e.commit_type) ?? [];
			list.push(e);
			m.set(e.commit_type, list);
		}
		return order.filter((k) => m.has(k)).map((k) => ({ type: k, entries: m.get(k)! }));
	});
</script>

<svelte:head>
	<title>{rel?.version ?? t('roadmap.releases.title', 'Release')} — {siteName()}</title>
</svelte:head>

<div class="wrap">
	<a class="back muted" href="/roadmap/releases">
		<ArrowLeft size={14} /> {t('roadmap.releases.back', 'All releases')}
	</a>

	{#if data.loadError}
		<EmptyState
			icon={Package}
			title={t('roadmap.releases.error_title', 'Could not load this release')}
			description={t('roadmap.releases.error_desc', 'Something went wrong. Please try again later.')}
		/>
	{:else if rel}
		<header class="rel-head">
			<div class="rel-top">
				<h1>{rel.version}</h1>
				<span class="state state-{rel.support_state}">
					{t('roadmap.releases.state_' + rel.support_state, rel.support_state)}
				</span>
				<span class="surface">{t('roadmap.releases.surface_' + rel.surface, rel.surface)}</span>
			</div>
			{#if rel.title}
				<p class="title-line">{rel.title}</p>
			{/if}
			<div class="meta muted">
				{#if rel.released_at}
					<span>{t('roadmap.releases.released_on', 'Released')} {new Date(rel.released_at).toLocaleDateString()}</span>
				{/if}
				{#if rel.eol_at}
					<span>· {t('roadmap.releases.eol_prefix', 'EOL')} {new Date(rel.eol_at).toLocaleDateString()}</span>
				{/if}
				{#if rel.is_esr}
					<span>· ESR</span>
				{/if}
			</div>
			{#if scopes.length > 0}
				<div class="scope-filters">
					<a class="scope {data.scope === '' ? 'active' : ''}" href="/roadmap/releases/{rel.slug}">
						{t('roadmap.scope_all', 'All')}
					</a>
					{#each scopes as s}
						<a
							class="scope {data.scope === s ? 'active' : ''}"
							href="/roadmap/releases/{rel.slug}?scope={encodeURIComponent(s)}"
						>
							{s}
						</a>
					{/each}
				</div>
			{/if}
		</header>

		{#if rel.body}
			<section class="notes">
				{@html markdownToHtml(rel.body, { sanitize: true })}
			</section>
		{/if}

		{#if rel.items.length > 0}
			<section>
				<h2>{t('roadmap.releases.headline_features', 'Headline features')}</h2>
				<ul class="items">
					{#each rel.items as item}
						<li>
							<a href="/roadmap/{item.slug}" class="item-title">{item.title}</a>
							{#if item.category_name}
								<span
									class="cat"
									style:background={item.category_color ?? 'var(--color-muted, #6b7280)'}
								>
									{item.category_name}
								</span>
							{/if}
							{#if item.summary}
								<span class="muted summary">{item.summary}</span>
							{/if}
						</li>
					{/each}
				</ul>
			</section>
		{/if}

		{#each groups as g}
			<section>
				<h2>{t('roadmap.type_' + g.type, g.type)} <span class="n">({g.entries.length})</span></h2>
				<ul class="entries">
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
								<span class="scope-tag">{e.scope}</span>
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
	.rel-head {
		margin: 0.75rem 0 1.5rem;
	}
	.rel-top {
		display: flex;
		align-items: center;
		gap: 0.6rem;
		flex-wrap: wrap;
	}
	h1 {
		font-size: 1.5rem;
		font-weight: 700;
		margin: 0;
	}
	.title-line {
		margin: 0.3rem 0 0;
		font-size: 1.05rem;
		font-weight: 600;
	}
	.meta {
		font-size: 0.85rem;
		margin-top: 0.4rem;
		display: flex;
		gap: 0.4rem;
		flex-wrap: wrap;
	}
	.state {
		font-size: 0.7rem;
		font-weight: 600;
		text-transform: uppercase;
		letter-spacing: 0.04em;
		padding: 0.15rem 0.5rem;
		border-radius: 999px;
	}
	.state-latest {
		background: color-mix(in srgb, var(--color-primary, #6d28d9) 15%, transparent);
		color: var(--color-primary, #6d28d9);
	}
	.state-supported {
		background: color-mix(in srgb, var(--color-muted, #6b7280) 15%, transparent);
		color: var(--color-muted, #6b7280);
	}
	.state-esr {
		background: color-mix(in srgb, #2563eb 15%, transparent);
		color: #2563eb;
	}
	.state-eol {
		background: color-mix(in srgb, #b45309 15%, transparent);
		color: #b45309;
	}
	.state-upcoming {
		background: color-mix(in srgb, #059669 15%, transparent);
		color: #059669;
	}
	.state-yanked {
		background: color-mix(in srgb, #dc2626 15%, transparent);
		color: #dc2626;
	}
	.surface {
		font-size: 0.72rem;
		color: var(--color-muted, #6b7280);
		text-transform: capitalize;
	}
	.scope-filters {
		display: flex;
		flex-wrap: wrap;
		gap: 0.4rem;
		margin-top: 0.75rem;
	}
	.scope {
		font-size: 0.75rem;
		border: 1px solid var(--color-border, #e5e7eb);
		border-radius: 999px;
		padding: 0.2rem 0.7rem;
		text-decoration: none;
		color: var(--color-text, #111827);
	}
	.scope.active {
		background: var(--color-primary, #6d28d9);
		border-color: var(--color-primary, #6d28d9);
		color: #fff;
	}
	.notes {
		border: 1px solid var(--color-border, #e5e7eb);
		border-radius: var(--radius-card, 0.75rem);
		background: var(--color-surface, #fff);
		padding: 1rem;
		margin-bottom: 1.5rem;
	}
	.notes {
		font-size: 0.9rem;
		line-height: 1.6;
	}
	.notes :global(p) {
		margin: 0.4rem 0;
	}
	.notes :global(pre) {
		overflow-x: auto;
		font-size: 0.82rem;
	}
	.notes :global(img) {
		border-radius: 0.5rem;
	}
	.notes :global(a) {
		color: var(--color-primary, #6d28d9);
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
	}
	.items li {
		display: flex;
		align-items: baseline;
		gap: 0.5rem;
		flex-wrap: wrap;
		padding: 0.4rem 0;
	}
	.item-title {
		font-weight: 600;
		color: var(--color-primary, #6d28d9);
		text-decoration: none;
	}
	.item-title:hover {
		text-decoration: underline;
	}
	.cat {
		font-size: 0.65rem;
		font-weight: 600;
		color: #fff;
		border-radius: 999px;
		padding: 0.1rem 0.5rem;
	}
	.summary {
		font-size: 0.82rem;
		flex-basis: 100%;
	}
	.entries li {
		font-size: 0.9rem;
		display: flex;
		align-items: baseline;
		gap: 0.5rem;
		flex-wrap: wrap;
		padding: 0.25rem 0;
	}
	.entries a {
		color: var(--color-primary, #6d28d9);
		text-decoration: none;
	}
	.entries a:hover {
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
	.scope-tag {
		font-size: 0.7rem;
		color: var(--color-muted, #6b7280);
		border: 1px solid var(--color-border, #e5e7eb);
		border-radius: 999px;
		padding: 0.05rem 0.5rem;
	}
</style>
