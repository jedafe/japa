<script lang="ts">
	import type { PageData } from './$types';
	import { t, siteName } from '$lib/i18n';
	import { Package, Rss, ArrowRight } from 'lucide-svelte';
	import EmptyState from '$lib/components/ui/EmptyState.svelte';
	import type { RoadmapRelease } from '$lib/types';

	/**
	 * Releases timeline — Discourse-style release bands + per-release cards.
	 * Band width/offset computed from released_at → superseded_at (falling
	 * back to eol_at, then a 1-month minimum band).
	 * Ref: japaconnect-engineering/sections/113-public-product-roadmap.md
	 */
	let { data } = $props<{ data: PageData }>();

	const release = $derived((data.releases ?? []) as RoadmapRelease[]);
	const bySurface = $derived.by(() => {
		const m = new Map<string, RoadmapRelease[]>();
		for (const r of release) {
			const list = m.get(r.surface) ?? [];
			list.push(r);
			m.set(r.surface, list);
		}
		return [...m.entries()];
	});

	// Band math — the chart domain runs from the oldest release to now +
	// one month of headroom so the "latest" band lands inside the track.
	const MONTH = 30.44 * 24 * 3600 * 1000;
	const domain = $derived.by(() => {
		const now = Date.now();
		const starts = release
			.map((r) => (r.released_at ? Date.parse(r.released_at) : NaN))
			.filter((v) => !Number.isNaN(v));
		const min = starts.length ? Math.min(...starts) : now - 6 * MONTH;
		const max = Math.max(now + MONTH, ...starts);
		return { min, max };
	});

	function bandPct(r: RoadmapRelease): { left: string; width: string } {
		const start = r.released_at ? Date.parse(r.released_at) : domain.min;
		const end = r.superseded_at
			? Date.parse(r.superseded_at)
			: r.eol_at && r.support_state !== 'latest'
				? Date.parse(r.eol_at)
				: domain.max;
		const span = domain.max - domain.min || 1;
		const left = ((start - domain.min) / span) * 100;
		const width = Math.max(((end - start) / span) * 100, 2);
		return { left: `${left.toFixed(1)}%`, width: `${width.toFixed(1)}%` };
	}

	function stateClass(r: RoadmapRelease): string {
		return `band-${r.support_state}`;
	}
</script>

<svelte:head>
	<title>{t('roadmap.releases.title', 'Releases')} — {siteName()}</title>
	<link
		rel="alternate"
		type="application/rss+xml"
		title={t('roadmap.releases.rss_title', '{site} Releases')}
		href="/roadmap/releases/feed.xml"
	/>
</svelte:head>

<div class="wrap">
	<header class="page-head">
		<div>
			<h1>{t('roadmap.releases.title', 'Releases')}</h1>
			<p class="muted">{t('roadmap.releases.subtitle', 'Versioned updates across the platform')}</p>
		</div>
		<div class="head-actions">
			{#if data.unreleased_count > 0}
				<a class="unreleased-pill" href="/roadmap/releases/unreleased">
					{t('roadmap.releases.in_development', 'In development')}
					<span class="count">{data.unreleased_count}</span>
					<ArrowRight size={14} />
				</a>
			{/if}
			<a class="rss-link" href="/roadmap/releases/feed.xml" title="RSS">
				<Rss size={16} />
			</a>
		</div>
	</header>

	{#if data.loadError}
		<EmptyState
			icon={Package}
			title={t('roadmap.releases.error_title', 'Could not load releases')}
			description={t('roadmap.releases.error_desc', 'Something went wrong. Please try again later.')}
		/>
	{:else if release.length === 0}
		<EmptyState
			icon={Package}
			title={t('roadmap.releases.empty_title', 'No releases yet')}
			description={t('roadmap.releases.empty_desc', 'Published releases will show up here.')}
		/>
	{:else}
		<!-- Timeline bands, one track per surface -->
		<section class="timeline" aria-label={t('roadmap.releases.timeline_aria', 'Release timeline')}>
			{#each bySurface as [surface, rels]}
				<div class="track">
					<span class="track-label">{t('roadmap.releases.surface_' + surface, surface)}</span>
					<div class="track-rail">
						{#each rels as r}
							<a
								href="/roadmap/releases/{r.slug}"
								class="band {stateClass(r)}"
								style:left={bandPct(r).left}
								style:width={bandPct(r).width}
								title="{r.version} · {r.support_state}"
							>
								<span class="band-label">{r.version}</span>
							</a>
						{/each}
					</div>
				</div>
			{/each}
		</section>

		<!-- Release cards -->
		<section class="cards">
			{#each release as r}
				<a class="card" href="/roadmap/releases/{r.slug}">
					<div class="card-top">
						<span class="version">{r.version}</span>
						<span class="state state-{r.support_state}">
							{t('roadmap.releases.state_' + r.support_state, r.support_state)}
						</span>
					</div>
					{#if r.title}
						<div class="card-title">{r.title}</div>
					{/if}
					{#if r.summary}
						<p class="card-summary">{r.summary}</p>
					{/if}
					<div class="card-meta muted">
						{#if r.released_at}
							<span>{new Date(r.released_at).toLocaleDateString()}</span>
						{/if}
						{#if r.item_count > 0}
							<span>· {r.item_count} {t('roadmap.releases.features', 'features')}</span>
						{/if}
						{#if r.eol_at}
							<span>
								· {t('roadmap.releases.eol_prefix', 'EOL')}
								{new Date(r.eol_at).toLocaleDateString()}
							</span>
						{/if}
					</div>
				</a>
			{/each}
		</section>
	{/if}
</div>

<style>
	.wrap {
		max-width: 64rem;
		margin: 0 auto;
		padding: 2rem 1rem 4rem;
	}
	.page-head {
		display: flex;
		align-items: flex-start;
		justify-content: space-between;
		gap: 1rem;
		margin-bottom: 1.5rem;
	}
	h1 {
		font-size: 1.5rem;
		font-weight: 700;
		margin: 0;
	}
	.muted {
		color: var(--color-muted, #6b7280);
	}
	.page-head .muted {
		margin: 0.25rem 0 0;
		font-size: 0.9rem;
	}
	.head-actions {
		display: flex;
		align-items: center;
		gap: 0.5rem;
	}
	.unreleased-pill {
		display: inline-flex;
		align-items: center;
		gap: 0.4rem;
		font-size: 0.82rem;
		padding: 0.35rem 0.75rem;
		border-radius: 999px;
		border: 1px solid var(--color-border, #e5e7eb);
		color: var(--color-text, #111827);
		text-decoration: none;
	}
	.unreleased-pill:hover {
		background: var(--color-surface-hover, #f3f4f6);
	}
	.unreleased-pill .count {
		background: var(--color-primary, #6d28d9);
		color: #fff;
		border-radius: 999px;
		font-size: 0.7rem;
		padding: 0 0.45rem;
		line-height: 1.3;
	}
	.rss-link {
		color: var(--color-muted, #6b7280);
		padding: 0.4rem;
	}
	.rss-link:hover {
		color: var(--color-primary, #6d28d9);
	}
	.timeline {
		margin-bottom: 2rem;
		display: flex;
		flex-direction: column;
		gap: 0.6rem;
	}
	.track {
		display: grid;
		grid-template-columns: 5rem 1fr;
		align-items: center;
		gap: 0.75rem;
	}
	.track-label {
		font-size: 0.75rem;
		font-weight: 600;
		text-transform: capitalize;
		color: var(--color-muted, #6b7280);
	}
	.track-rail {
		position: relative;
		height: 2rem;
		background: var(--color-surface, #f9fafb);
		border: 1px solid var(--color-border, #e5e7eb);
		border-radius: 0.5rem;
		overflow: hidden;
	}
	.band {
		position: absolute;
		top: 0;
		bottom: 0;
		display: flex;
		align-items: center;
		padding: 0 0.5rem;
		overflow: hidden;
		text-decoration: none;
		border-left: 1px solid var(--color-border, #e5e7eb);
	}
	.band-label {
		font-size: 0.7rem;
		font-weight: 600;
		white-space: nowrap;
		text-overflow: ellipsis;
		overflow: hidden;
	}
	.band-latest {
		background: color-mix(in srgb, var(--color-primary, #6d28d9) 18%, transparent);
		color: var(--color-primary, #6d28d9);
	}
	.band-supported {
		background: color-mix(in srgb, var(--color-muted, #6b7280) 12%, transparent);
		color: var(--color-muted, #6b7280);
	}
	.band-esr {
		background: color-mix(in srgb, #2563eb 15%, transparent);
		color: #2563eb;
	}
	.band-eol {
		background: color-mix(in srgb, var(--color-muted, #6b7280) 6%, transparent);
		color: var(--color-muted, #6b7280);
	}
	.band-upcoming {
		background: repeating-linear-gradient(
			-45deg,
			transparent,
			transparent 4px,
			color-mix(in srgb, var(--color-primary, #6d28d9) 10%, transparent) 4px,
			color-mix(in srgb, var(--color-primary, #6d28d9) 10%, transparent) 8px
		);
		color: var(--color-primary, #6d28d9);
	}
	.band-yanked {
		background: color-mix(in srgb, #dc2626 10%, transparent);
		color: #dc2626;
	}
	.cards {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(17rem, 1fr));
		gap: 1rem;
	}
	.card {
		border: 1px solid var(--color-border, #e5e7eb);
		border-radius: var(--radius-card, 0.75rem);
		background: var(--color-surface, #fff);
		padding: 1rem;
		text-decoration: none;
		color: var(--color-text, #111827);
		display: flex;
		flex-direction: column;
		gap: 0.4rem;
	}
	.card:hover {
		border-color: var(--color-primary, #6d28d9);
	}
	.card-top {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 0.5rem;
	}
	.version {
		font-weight: 700;
		font-size: 1.05rem;
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
	.card-title {
		font-weight: 600;
	}
	.card-summary {
		font-size: 0.85rem;
		color: var(--color-muted, #6b7280);
		margin: 0;
	}
	.card-meta {
		font-size: 0.78rem;
		margin-top: auto;
		display: flex;
		flex-wrap: wrap;
		gap: 0.35rem;
	}
</style>
