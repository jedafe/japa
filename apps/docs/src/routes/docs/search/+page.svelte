<script lang="ts">
	import { untrack } from 'svelte';
	import SeoHead from '$lib/seo/SeoHead.svelte';
	import { goto } from '$app/navigation';
	import { navigating } from '$app/state';
	import AppShell from '$lib/components/shell/AppShell.svelte';
	import AutocompleteSelect from '$lib/components/ui/AutocompleteSelect.svelte';
	import { Search } from 'lucide-svelte';
	import { t } from '$lib/i18n';
	import { persistNavCollapse } from '$lib/nav-collapse';
	import { AUDIENCE_LABEL_KEY } from '$lib/types';
	import type { DocAudience } from '$lib/types';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	let collapsed = $state(untrack(() => data.navCollapsed));

	async function toggleCollapse() {
		collapsed = !collapsed;
		await persistNavCollapse(collapsed);
	}

	let query = $state('');
	let selectedAudience = $state<DocAudience | null>(null);
	let pageSize = $state('20');

	// Keep the input + filters in sync with the URL params.
	$effect(() => {
		query = data.q;
		selectedAudience = data.audience;
		pageSize = String(data.limit);
	});

	function buildParams(): URLSearchParams {
		const params = new URLSearchParams();
		const q = query.trim();
		if (q) params.set('q', q);
		if (selectedAudience) params.set('audience', selectedAudience);
		if (pageSize !== '20') params.set('limit', pageSize);
		return params;
	}

	function submit(e: SubmitEvent) {
		e.preventDefault();
		const params = buildParams();
		goto(params.toString() ? `/docs/search?${params}` : '/docs/search');
	}

	function pickAudience(a: DocAudience | null) {
		selectedAudience = a;
		const params = buildParams();
		goto(params.toString() ? `/docs/search?${params}` : '/docs/search');
	}

	function pickPageSize(v: string) {
		pageSize = v;
		if (!data.q) return;
		const params = buildParams();
		goto(`/docs/search?${params}`);
	}

	// Any in-flight navigation back to the search route shows the skeleton —
	// comparing only ?q= missed audience/limit changes (BUG-HUNT: stale state
	// during filter switches).
	const searching = $derived(
		!!navigating.to && navigating.to.url.pathname.endsWith('/docs/search')
	);
</script>

<svelte:head>
	<SeoHead meta={data.meta} noindex />
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

		<div class="filter-row">
			<div class="audience-filters" role="group" aria-label={t('search.filter_audience')}>
				<button
					type="button"
					class="chip"
					class:active={selectedAudience === null}
					onclick={() => pickAudience(null)}
				>
					{t('search.all_audiences')}
				</button>
				{#each data.audienceOrder.filter((a) => data.accessibleAudiences.includes(a)) as audience (audience)}
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
			<div class="per-page">
				<label class="per-page-label" for="search-per-page">{t('search.per_page', 'Per page')}</label>
				<AutocompleteSelect
					id="search-per-page"
					bind:value={pageSize}
					onValueChange={pickPageSize}
					options={['10', '20', '50'].map((n) => ({ value: n, label: n }))}
					ariaLabel={t('search.per_page', 'Per page')}
					class="per-page-select"
				/>
			</div>
		</div>

		{#if !data.q}
			<p class="hint">{t('search.start_typing')}</p>
		{:else if searching}
			<!-- Loading skeleton (§54: loading states) -->
			<div class="results-loading" aria-busy="true" aria-live="polite">
				{#each Array(5) as _, i (i)}
					<div class="result-skeleton">
						<div class="skeleton-line skeleton-title"></div>
						<div class="skeleton-line skeleton-meta"></div>
					</div>
				{/each}
			</div>
		{:else if data.results.length === 0}
			<p class="hint">{t('search.empty')}</p>
		{:else}
			<p class="results-for">
				{t('search.results_for', '', { q: data.q })}
				{#if data.audience} · {t(AUDIENCE_LABEL_KEY[data.audience], data.audience)}{/if}
				· {data.results.length}
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
		font-family: var(--font-heading, var(--font-sans));
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

	.filter-row {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 1rem;
		margin-bottom: 1.5rem;
		flex-wrap: wrap;
	}
	.audience-filters {
		display: flex;
		flex-wrap: wrap;
		gap: 0.375rem;
	}
	.per-page {
		display: flex;
		align-items: center;
		gap: 0.5rem;
	}
	.per-page-label {
		font-size: 0.75rem;
		color: var(--color-muted);
		white-space: nowrap;
	}
	.per-page :global(.per-page-select) {
		width: 5.5rem;
	}
	.per-page :global(.per-page-select .autocomplete-input-wrapper) {
		height: 2rem;
		padding: 0 0.625rem;
		font-size: 0.8125rem;
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
	.results-loading {
		display: flex;
		flex-direction: column;
		gap: 0.75rem;
	}
	.result-skeleton {
		padding: 1rem;
		border: 1px solid var(--color-border);
		border-radius: var(--radius-card);
	}
	.skeleton-line {
		height: 0.875rem;
		border-radius: 4px;
		background: var(--color-surface-subtle);
		animation: pulse 1.5s ease-in-out infinite;
	}
	.skeleton-title {
		width: 60%;
		height: 1.125rem;
		margin-bottom: 0.5rem;
	}
	.skeleton-meta {
		width: 85%;
	}
	@keyframes pulse {
		0%, 100% { opacity: 1; }
		50% { opacity: 0.5; }
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
