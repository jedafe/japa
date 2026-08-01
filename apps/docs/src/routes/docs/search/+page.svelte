<script lang="ts">
	import { goto } from '$app/navigation';
	import AppShell from '$lib/components/shell/AppShell.svelte';
	import { Search } from 'lucide-svelte';
	import { t } from '$lib/i18n';
	import { AUDIENCE_LABEL_KEY, AUDIENCE_ORDER } from '$lib/types';
	import type { DocAudience } from '$lib/types';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	let query = $state('');
	let selectedAudience = $state<DocAudience | null>(null);

	// Keep the input + audience filter in sync with the URL params.
	$effect(() => {
		query = data.q;
		selectedAudience = data.audience;
	});

	function submit(e: SubmitEvent) {
		e.preventDefault();
		const q = query.trim();
		const params = new URLSearchParams();
		if (q) params.set('q', q);
		if (selectedAudience) params.set('audience', selectedAudience);
		goto(params.toString() ? `/docs/search?${params}` : '/docs/search');
	}

	function pickAudience(a: DocAudience | null) {
		selectedAudience = a;
		const q = query.trim();
		const params = new URLSearchParams();
		if (q) params.set('q', q);
		if (a) params.set('audience', a);
		goto(params.toString() ? `/docs/search?${params}` : '/docs/search');
	}
</script>

<svelte:head>
	<title>{data.meta.title}</title>
	<meta name="description" content={data.meta.description} />
	<link rel="canonical" href={data.meta.ogUrl} />
	{#if data.q}<meta name="robots" content="noindex" />{/if}
</svelte:head>

<AppShell
	nav={data.nav}
	accessibleAudiences={data.accessibleAudiences}
	logoUrl={(data.siteSettings?.["appearance.logo_url"] as string | undefined) ?? ""}
	footerMenu={data.footerMenu}
	rightSidebarWidgets={data.rightSidebarWidgets}
	activeAudience={null}
	user={data.user}
	collapsed={data.navCollapsed}
	onToggleCollapse={() => {}}
>
	<section class="search-page">
		<h1>{t('search.title')}</h1>

		<form class="search-form" onsubmit={submit}>
			<div class="input-wrap">
				<Search size={18} class="search-icon" />
				<input
					bind:value={query}
					type="text"
					placeholder={t('search.placeholder')}
					aria-label={t('search.placeholder')}
				/>
			</div>
			<button type="submit" class="submit-btn">{t('search.title')}</button>
		</form>

		<div class="audience-filters" role="group" aria-label={t('search.filter_audience')}>
			<button
				type="button"
				class="chip"
				class:active={selectedAudience === null}
				onclick={() => pickAudience(null)}
			>
				{t('search.all_audiences')}
			</button>
			{#each AUDIENCE_ORDER.filter((a) => data.accessibleAudiences.includes(a)) as audience (audience)}
				<button
					type="button"
					class="chip"
					class:active={selectedAudience === audience}
					onclick={() => pickAudience(audience)}
				>
					{t(AUDIENCE_LABEL_KEY[audience], audience)}
				</button>
			{/each}
		</div>

		{#if !data.q}
			<p class="hint">{t('search.start_typing')}</p>
		{:else if data.results.length === 0}
			<p class="hint">{t('search.empty')}</p>
		{:else}
			<p class="results-for">
				{t('search.results_for', '', { q: data.q })}
				{#if data.audience} · {t(AUDIENCE_LABEL_KEY[data.audience], data.audience)}{/if}
			</p>
			<ul class="results">
				{#each data.results as r (r.id)}
					<li>
						<a href="/docs/{r.audience}/{r.slug}" class="result">
							<span class="result-title">{r.title}</span>
							<span class="result-meta">
								<span class="result-audience">{t(AUDIENCE_LABEL_KEY[r.audience], r.audience)}</span>
								{#if r.excerpt}<span class="result-excerpt">{r.excerpt}</span>{/if}
							</span>
						</a>
					</li>
				{/each}
			</ul>
		{/if}
	</section>
</AppShell>

<style>
	.search-page {
		max-width: 760px;
		margin: 0 auto;
		padding: 2.5rem 1.5rem 4rem;
	}
	h1 {
		font-family: var(--font-sans);
		font-size: 1.75rem;
		font-weight: 700;
		color: var(--color-text);
		margin: 0 0 1.5rem;
	}
	.search-form {
		display: flex;
		gap: 0.5rem;
		margin-bottom: 1.5rem;
	}
	.input-wrap {
		position: relative;
		flex: 1;
		display: flex;
		align-items: center;
	}
	:global(.search-icon) {
		position: absolute;
		left: 0.75rem;
		color: var(--color-muted);
		pointer-events: none;
	}
	.input-wrap input {
		width: 100%;
		padding: 0.625rem 0.75rem 0.625rem 2.5rem;
		border: 1px solid var(--color-border);
		border-radius: var(--radius);
		background: var(--color-surface);
		color: var(--color-text);
		font-size: 0.9375rem;
		outline: none;
	}
	.input-wrap input:focus { border-color: var(--color-primary); }
	.submit-btn {
		padding: 0.625rem 1.25rem;
		border: 1px solid var(--color-border);
		border-radius: var(--radius);
		background: var(--color-surface);
		color: var(--color-text);
		font-size: 0.875rem;
		font-weight: 500;
		cursor: pointer;
	}
	.submit-btn:hover { border-color: var(--color-primary); color: var(--color-primary); }

	.audience-filters {
		display: flex;
		flex-wrap: wrap;
		gap: 0.375rem;
		margin-bottom: 1.5rem;
	}
	.chip {
		padding: 0.25rem 0.625rem;
		border: 1px solid var(--color-border);
		border-radius: 9999px;
		background: var(--color-surface);
		color: var(--color-muted);
		font-size: 0.75rem;
		font-weight: 500;
		cursor: pointer;
		text-transform: capitalize;
	}
	.chip:hover { border-color: var(--color-border-input-hover); color: var(--color-text); }
	.chip.active {
		background: var(--color-primary);
		color: var(--color-primary-contrast);
		border-color: var(--color-primary);
	}

	.hint {
		padding: 2rem;
		text-align: center;
		color: var(--color-muted);
		font-size: 0.9375rem;
		border: 1px dashed var(--color-border);
		border-radius: var(--radius-card);
	}
	.results-for {
		font-size: 0.875rem;
		color: var(--color-muted);
		margin: 0 0 1rem;
	}
	.results {
		list-style: none;
		padding: 0;
		margin: 0;
		display: flex;
		flex-direction: column;
		gap: 0.375rem;
	}
	.result {
		display: flex;
		flex-direction: column;
		gap: 0.125rem;
		padding: 0.75rem 0.875rem;
		border: 1px solid var(--color-border);
		border-radius: var(--radius);
		background: var(--color-surface);
		text-decoration: none;
	}
	.result:hover { border-color: var(--color-primary); }
	.result-title { font-size: 0.9375rem; font-weight: 500; color: var(--color-text); }
	.result-meta { display: flex; align-items: center; gap: 0.5rem; }
	.result-audience {
		font-size: 0.6875rem;
		text-transform: uppercase;
		letter-spacing: 0.05em;
		color: var(--color-primary);
		font-weight: 600;
	}
	.result-excerpt {
		font-size: 0.75rem;
		color: var(--color-muted);
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
</style>
