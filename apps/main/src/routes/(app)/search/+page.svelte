<script lang="ts">
	import { untrack } from 'svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { Search as SearchIcon, FileText, Users, MessageSquare, Building2, Hash, Layers, Loader2, BookOpen } from 'lucide-svelte';
	import { docsUrl } from '$lib/appOrigins';
	import { t } from '$lib/i18n';
	import { SearchInput, type SearchSuggestion } from '$lib/components/ui';
	import { api } from '$lib/api/client';
	import UserHoverCard from '$lib/components/ui/UserHoverCard.svelte';
	import EntityHoverChip from '$lib/components/ui/EntityHoverChip.svelte';
	import type { PageData } from './$types';
	import type { SearchResult } from '$lib/types/search';

	let { data }: { data: PageData } = $props();

	let query = $state(untrack(() => data.query));
	let loading = $state(false);
	const activeType = $derived(page.url.searchParams.get('type') ?? 'all');

	// §54: 200ms debounce per spec.
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
			// Preserve §54 params from the current URL.
			const sort = page.url.searchParams.get('sort');
			if (sort) params.set('sort', sort);
			const contentType = page.url.searchParams.get('content_type');
			if (contentType) params.set('content_type', contentType);
			const spaceId = page.url.searchParams.get('space_id');
			if (spaceId) params.set('space_id', spaceId);
			const tagSlug = page.url.searchParams.get('tag_slug');
			if (tagSlug) params.set('tag_slug', tagSlug);
			const geo = page.url.searchParams.get('geo');
			if (geo) params.set('geo', geo);
			goto(`/search?${params.toString()}`, { keepFocus: true, noScroll: true });
		}, 200);
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

	// DOC-006: the backend groups doc results as type `doc_page`, but the
	// filter bar (and the ?type= param) uses `doc`.
	function filterTypeForGroup(type: string): string {
		return type === 'doc_page' ? 'doc' : type;
	}

	function activeForFilter(key: string): boolean {
		return activeType === key || (key === 'doc' && activeType === 'doc_page');
	}

	function autocompleteUrl(s: { type: string; url?: string; slug?: string }): string {
		if (s.url) return s.url;
		switch (s.type) {
			case 'user':
				return s.slug ? `/u/${s.slug}` : '/search';
			case 'space':
				return s.slug ? `/spaces/${s.slug}` : '/search';
			case 'doc_page':
			// DOC-016: the backend should always provide a `url` for doc pages;
			// fall back to search if it is missing.
				return '/search';
			default:
				return s.slug ? `/${s.type}/${s.slug}` : '/search';
		}
	}

	async function autocompleteFetcher(q: string): Promise<SearchSuggestion[]> {
		const res = await api.get<{ data: { suggestions: { type: string; id: string; title?: string; excerpt?: string; url?: string; slug?: string; username?: string; display_name?: string; name?: string; content_type?: string }[] } }>(
			`/search/autocomplete?q=${encodeURIComponent(q)}&limit=8`
		);
		if (res.error || !res.data) return [];
		return (res.data.data?.suggestions ?? []).map((s) => ({
			id: s.id,
			title: s.title ?? s.name ?? s.username ?? s.display_name ?? '',
			url: autocompleteUrl(s),
			excerpt: s.excerpt,
			type: s.type
		}));
	}

	// Filter types — driven by the backend search index types.
	const filterTypes = $derived([
		{ key: 'all', label: t('search.filter_all', 'All'), icon: Layers },
		{ key: 'content', label: t('search.filter_content', 'Posts'), icon: FileText },
		{ key: 'user', label: t('search.filter_people', 'People'), icon: Users },
		{ key: 'space', label: t('search.filter_spaces', 'Spaces'), icon: Hash },
		{ key: 'forum_thread', label: t('search.filter_discussions', 'Discussions'), icon: MessageSquare },
		{ key: 'doc', label: t('search.filter_docs', 'Docs'), icon: BookOpen },
		{ key: 'organization', label: t('search.filter_orgs', 'Organizations'), icon: Building2 }
	]);

	// ADR-017: the backend now groups results server-side when no type
	// filter is active. Use the server-provided groups directly instead of
	// re-grouping the flat results client-side. Fall back to client-side
	// grouping only if groups is empty but results is not (defensive).
	const grouped = $derived.by(() => {
		if (data.groups && data.groups.length > 0) {
			return data.groups;
		}
		// Defensive fallback: re-group flat results if backend didn't group.
		const buckets: Record<string, typeof data.results> = {};
		for (const r of data.results) {
			if (!buckets[r.type]) buckets[r.type] = [];
			buckets[r.type].push(r);
		}
		return Object.entries(buckets).map(([type, results]) => ({ type, results, total: results.length }));
	});

	// Reactive label map so a locale switch re-resolves the type labels.
	const groupLabels = $derived<Record<string, string>>({
		content: t('search.filter_content', 'Posts'),
		user: t('search.filter_people', 'People'),
		space: t('search.filter_spaces', 'Spaces'),
		forum_thread: t('search.filter_discussions', 'Discussions'),
		organization: t('search.filter_orgs', 'Organizations'),
		course: t('search.filter_content', 'Courses'),
		tag: t('search.filter_all', 'Tags'),
		doc_page: t('search.filter_docs', 'Docs'),
		doc: t('search.filter_docs', 'Docs')
	});

	// Helper to get a display title from a nested result item (§54).
	function resultTitle(r: { item: { title?: string; name?: string; username?: string; display_name?: string } }): string {
		return r.item.title ?? r.item.name ?? r.item.username ?? r.item.display_name ?? '';
	}
	function resultExcerpt(r: { item: { excerpt?: string } }): string {
		return r.item.excerpt ?? '';
	}
	function resultURL(r: { item: { url?: string } }): string {
		return r.item.url ?? '#';
	}
	function resultID(r: { item: { id: string } }): string {
		return r.item.id;
	}
</script>

<svelte:head>
	<title>{data.query ? `${data.query} — ${t('app.name')}` : t('search.title', 'Search — JapaConnect')}</title>
	<meta name="description" content={t('search.description', 'Search for posts, people, spaces, and discussions on JapaConnect.')} />
	<meta name="robots" content="noindex, nofollow" />
</svelte:head>

{#snippet resultCard(result: SearchResult, showTypeBadge = false)}
	{@const item = result.item}
	{@const title = resultTitle(result)}
	{@const excerpt = resultExcerpt(result)}
	{@const hoverType =
		result.type === 'user' || result.type === 'space' || result.type === 'organization' || result.type === 'source'
			? result.type
			: null}
	{#if hoverType}
		{#if hoverType === 'user'}
			<UserHoverCard
				id={item.id}
				slug={item.username}
				name={item.display_name ?? item.username ?? item.name ?? title}
				avatar={item.avatar_url}
				class="block w-full rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4 text-left transition-colors hover:bg-[var(--color-surface-subtle)]"
			>
				{#snippet trigger()}
					{#if showTypeBadge}
						<div class="mb-1 flex items-center gap-2">
							<span class="rounded-full bg-[var(--color-surface-subtle)] px-2 py-0.5 text-xs text-[var(--color-text-subtle)]">{groupLabels[result.type] ?? result.type}</span>
						</div>
					{/if}
					<h3 class="font-medium text-[var(--color-text)]">{title}</h3>
					{#if excerpt}<p class="mt-1 line-clamp-2 text-sm text-[var(--color-text-subtle)]">{excerpt}</p>{/if}
				{/snippet}
			</UserHoverCard>
		{:else}
			<EntityHoverChip
				type={hoverType}
				id={item.id}
				slug={item.slug}
				name={item.name ?? title}
				class="block w-full rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4 text-left transition-colors hover:bg-[var(--color-surface-subtle)]"
			>
				{#snippet trigger()}
					{#if showTypeBadge}
						<div class="mb-1 flex items-center gap-2">
							<span class="rounded-full bg-[var(--color-surface-subtle)] px-2 py-0.5 text-xs text-[var(--color-text-subtle)]">{groupLabels[result.type] ?? result.type}</span>
						</div>
					{/if}
					<h3 class="font-medium text-[var(--color-text)]">{title}</h3>
					{#if excerpt}<p class="mt-1 line-clamp-2 text-sm text-[var(--color-text-subtle)]">{excerpt}</p>{/if}
				{/snippet}
			</EntityHoverChip>
		{/if}
	{:else}
		<a href={resultURL(result)} class="block rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4 transition-colors hover:bg-[var(--color-surface-subtle)]">
			{#if showTypeBadge}
				<div class="mb-1 flex items-center gap-2">
					<span class="rounded-full bg-[var(--color-surface-subtle)] px-2 py-0.5 text-xs text-[var(--color-text-subtle)]">{groupLabels[result.type] ?? result.type}</span>
				</div>
			{/if}
			<h3 class="font-medium text-[var(--color-text)]">{title}</h3>
			{#if excerpt}<p class="mt-1 line-clamp-2 text-sm text-[var(--color-text-subtle)]">{excerpt}</p>{/if}
		</a>
	{/if}
{/snippet}

<div class="mx-auto max-w-3xl px-4 py-8">
	<h1 class="mb-6 text-2xl font-bold text-[var(--color-text)]">{t('search.heading', 'Search')}</h1>

	<form onsubmit={submitSearch} class="mb-4">
		<SearchInput
			bind:value={query}
			placeholder={t('search.placeholder', 'Search stories, spaces, people...')}
			ariaLabel={t('search.heading', 'Search')}
			fetcher={autocompleteFetcher}
			debounceMs={200}
			onInput={(v) => { query = v; onInput(); }}
			inputClass="py-2.5"
		/>
	</form>

	{#if data.query}
		<!-- Filter bar (Phase 10.02) -->
		<div class="mb-6 flex flex-wrap gap-2">
			{#each filterTypes as ft (ft.key)}
				<button
					type="button"
					onclick={() => filterByType(ft.key)}
					class="inline-flex items-center gap-1.5 rounded-full px-3 py-1.5 text-xs font-medium transition-colors {activeForFilter(ft.key)
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

		{#if loading}
			<!-- Loading skeleton (§54: loading states) -->
			<div class="space-y-3" aria-busy="true" aria-live="polite">
				{#each Array(5) as _, i (i)}
					<div class="animate-pulse rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
						<div class="mb-2 h-4 w-3/4 rounded bg-[var(--color-surface-subtle)]"></div>
						<div class="h-3 w-full rounded bg-[var(--color-surface-subtle)]"></div>
						<div class="mt-1 h-3 w-2/3 rounded bg-[var(--color-surface-subtle)]"></div>
					</div>
				{/each}
			</div>
		{:else if data.loadError}
			<!-- The backend returned a non-2xx (e.g. 503 search_unavailable when
			     Meilisearch is down). Distinguish this from a genuine empty result
			     so the user knows to retry rather than assuming nothing exists. -->
			<p class="py-12 text-center text-[var(--color-text-subtle)]">{t('search.search_unavailable', 'Search is temporarily unavailable. Please try again in a moment.')}</p>
		{:else if data.results.length === 0 && (!data.groups || data.groups.length === 0)}
			<p class="py-12 text-center text-[var(--color-text-subtle)]">{t('search.no_results', 'No results found. Try a different query.')}</p>
		{:else if activeType === 'all'}
			<!-- Grouped results (ADR-017: server-side grouping) -->
			<div class="space-y-6">
				{#each grouped as group (group.type)}
					<section>
						<h2 class="mb-2 text-sm font-medium text-[var(--color-text)]">
							{groupLabels[group.type] ?? group.type}
							<span class="ml-1 text-xs text-[var(--color-muted)]">({group.total})</span>
						</h2>
						<div class="space-y-2">
							{#each group.results as result (resultID(result))}
								{@render resultCard(result)}
							{/each}
							{#if group.total > group.results.length}
								<button
									type="button"
									onclick={() => filterByType(filterTypeForGroup(group.type))}
									class="text-sm text-[var(--color-primary)] hover:underline"
								>
									{t('search.see_all', 'See all {n}').replace('{n}', String(group.total))} →
								</button>
							{/if}
						</div>
					</section>
				{/each}
			</div>
		{:else}
			<!-- Filtered flat list (§54 nested item shape) -->
			<div class="space-y-3">
				{#each data.results as result (resultID(result))}
					{@render resultCard(result, true)}
				{/each}
			</div>
		{/if}
	{:else}
		<p class="py-12 text-center text-[var(--color-text-subtle)]">{t('search.start_typing', 'Start typing to search the community.')}</p>
	{/if}
</div>
