<script lang="ts">
	import { untrack } from 'svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { Search as SearchIcon, FileText, Users, MessageSquare, Building2, Hash, Layers, Loader2 } from 'lucide-svelte';
	import { t } from '$lib/i18n';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	let query = $state(untrack(() => data.query));
	let loading = $state(false);
	const activeType = $derived(page.url.searchParams.get('type') ?? 'all');

	// Live debounced search — updates the URL (and thus SSR results) as the
	// user types, with a 300ms debounce to avoid hammering the backend.
	let debounceTimer: ReturnType<typeof setTimeout> | null = null;

	function onInput() {
		if (debounceTimer) clearTimeout(debounceTimer);
		const q = query.trim();
		// If the query hasn't changed from what's already in the URL, skip.
		if (q === data.query && activeType === (page.url.searchParams.get('type') ?? 'all')) {
			loading = false;
			return;
		}
		if (!q) {
			loading = false;
			if (data.query) goto('/search', { keepFocus: true, noScroll: true });
			return;
		}
		loading = true;
		debounceTimer = setTimeout(() => {
			const params = new URLSearchParams();
			if (q) params.set('q', q);
			if (activeType !== 'all') params.set('type', activeType);
			goto(`/search?${params.toString()}`, { keepFocus: true, noScroll: true });
		}, 300);
	}

	// Sync the input from the URL only when data.query changes externally
	// (sidebar search → /search?q=foo, browser back/forward). We track the
	// last-seen data.query so the effect only re-fires on data changes —
	// NOT when the user types (which would clobber the input).
	let lastDataQuery = $state(untrack(() => data.query));
	$effect(() => {
		if (data.query !== lastDataQuery) {
			lastDataQuery = data.query;
			query = data.query;
		}
		loading = false;
	});

	function submitSearch(e: SubmitEvent) {
		e.preventDefault();
		if (debounceTimer) clearTimeout(debounceTimer);
		const q = query.trim();
		if (q) {
			loading = true;
			goto(`/search?q=${encodeURIComponent(q)}`, { keepFocus: true, noScroll: true });
		}
	}

	function filterByType(type: string) {
		const q = query.trim();
		const params = new URLSearchParams();
		if (q) params.set('q', q);
		if (type !== 'all') params.set('type', type);
		loading = true;
		goto(`/search?${params.toString()}`, { keepFocus: true, noScroll: true });
	}

	// Filter types — driven by the backend search index types.
	const filterTypes = $derived([
		{ key: 'all', label: t('search.filter_all', 'All'), icon: Layers },
		{ key: 'content', label: t('search.filter_content', 'Posts'), icon: FileText },
		{ key: 'user', label: t('search.filter_people', 'People'), icon: Users },
		{ key: 'space', label: t('search.filter_spaces', 'Spaces'), icon: Hash },
		{ key: 'forum_thread', label: t('search.filter_discussions', 'Discussions'), icon: MessageSquare },
		{ key: 'organization', label: t('search.filter_orgs', 'Organizations'), icon: Building2 }
	]);

	// Group results by type for the "All" view.
	const grouped = $derived.by(() => {
		const groups: Record<string, typeof data.results> = {};
		for (const r of data.results) {
			if (!groups[r.type]) groups[r.type] = [];
			groups[r.type].push(r);
		}
		return groups;
	});

	// Reactive label map so a locale switch re-resolves the type labels.
	// Previously a plain const object computed once at init, which left the
	// group headings stale after a language change.
	const groupLabels = $derived<Record<string, string>>({
		content: t('search.filter_content', 'Posts'),
		user: t('search.filter_people', 'People'),
		space: t('search.filter_spaces', 'Spaces'),
		forum_thread: t('search.filter_discussions', 'Discussions'),
		organization: t('search.filter_orgs', 'Organizations'),
		course: t('search.filter_content', 'Courses'),
		tag: t('search.filter_all', 'Tags'),
		doc_page: t('search.filter_content', 'Docs')
	});
</script>

<svelte:head>
	<title>{data.query ? `${data.query} — ${t('app.name')}` : t('search.title', 'Search — JapaConnect')}</title>
	<meta name="description" content={t('search.description', 'Search for posts, people, spaces, and discussions on JapaConnect.')} />
	<meta name="robots" content="noindex, nofollow" />
</svelte:head>

<div class="mx-auto max-w-full px-4 py-8">
	<h1 class="mb-6 text-2xl font-bold text-[var(--color-text)]">{t('search.heading', 'Search')}</h1>

	<form onsubmit={submitSearch} class="mb-4">
		<div class="relative">
			<SearchIcon class="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-[var(--color-text-subtle)]" size={18} />
			<input
				type="search"
				bind:value={query}
				oninput={onInput}
				placeholder={t('search.placeholder', 'Search stories, spaces, people...')}
				class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] py-2.5 pl-10 pr-10 text-sm text-[var(--color-text)] focus:border-[var(--color-primary)] focus:outline-none"
			/>
			{#if loading}
				<Loader2 class="pointer-events-none absolute right-3 top-1/2 -translate-y-1/2 animate-spin text-[var(--color-muted)]" size={16} />
			{/if}
		</div>
	</form>

	{#if data.query}
		<!-- Filter bar (Phase 10.02) -->
		<div class="mb-6 flex flex-wrap gap-2">
			{#each filterTypes as ft (ft.key)}
				<button
					type="button"
					onclick={() => filterByType(ft.key)}
					class="inline-flex items-center gap-1.5 rounded-full px-3 py-1.5 text-xs font-medium transition-colors {activeType === ft.key || (activeType === 'all' && ft.key === 'all')
						? 'bg-[var(--color-primary)] text-[var(--color-primary-contrast)]'
						: 'border border-[var(--color-border)] text-[var(--color-text-subtle)] hover:bg-[var(--color-surface-subtle)]'}"
				>
					<ft.icon size={13} />
					{ft.label}
				</button>
			{/each}
		</div>

		<p class="mb-4 text-sm text-[var(--color-text-subtle)]">
			{t('search.results_for', 'Results for')} "<span class="font-medium text-[var(--color-text)]">{data.query}</span>"
		</p>

		{#if data.loadError}
			<!-- The backend returned a non-2xx (e.g. 503 search_unavailable when
			     Meilisearch is down). Distinguish this from a genuine empty result
			     so the user knows to retry rather than assuming nothing exists. -->
			<p class="py-12 text-center text-[var(--color-text-subtle)]">{t('search.search_unavailable', 'Search is temporarily unavailable. Please try again in a moment.')}</p>
		{:else if data.results.length === 0}
			<p class="py-12 text-center text-[var(--color-text-subtle)]">{t('search.no_results', 'No results found. Try a different query.')}</p>
		{:else if activeType === 'all'}
			<!-- Grouped results (Phase 10.01) -->
			<div class="space-y-6">
				{#each Object.entries(grouped) as [type, results] (type)}
					<section>
						<h2 class="mb-2 text-sm font-medium text-[var(--color-text)]">
							{groupLabels[type] ?? type}
							<span class="ml-1 text-xs text-[var(--color-muted)]">({results.length})</span>
						</h2>
						<div class="space-y-2">
							{#each results.slice(0, 5) as result (result.id)}
								<a href={result.url} class="block rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4 transition-colors hover:bg-[var(--color-surface-subtle)]">
									<h3 class="font-medium text-[var(--color-text)]">{result.title}</h3>
									{#if result.excerpt}
										<p class="mt-1 line-clamp-2 text-sm text-[var(--color-text-subtle)]">{result.excerpt}</p>
									{/if}
								</a>
							{/each}
							{#if results.length > 5}
								<button
									type="button"
									onclick={() => filterByType(type)}
									class="text-sm text-[var(--color-primary)] hover:underline"
								>
									{t('search.see_all', 'See all {n}').replace('{n}', String(results.length))} →
								</button>
							{/if}
						</div>
					</section>
				{/each}
			</div>
		{:else}
			<!-- Filtered flat list -->
			<div class="space-y-3">
				{#each data.results as result (result.id)}
					<a href={result.url} class="block rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4 transition-colors hover:bg-[var(--color-surface-subtle)]">
						<div class="mb-1 flex items-center gap-2">
							<span class="rounded-full bg-[var(--color-surface-subtle)] px-2 py-0.5 text-xs text-[var(--color-text-subtle)]">{groupLabels[result.type] ?? result.type}</span>
						</div>
						<h3 class="font-medium text-[var(--color-text)]">{result.title}</h3>
						{#if result.excerpt}
							<p class="mt-1 line-clamp-2 text-sm text-[var(--color-text-subtle)]">{result.excerpt}</p>
						{/if}
					</a>
				{/each}
			</div>
		{/if}
	{:else}
		<p class="py-12 text-center text-[var(--color-text-subtle)]">{t('search.start_typing', 'Start typing to search the community.')}</p>
	{/if}
</div>
