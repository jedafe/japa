<script lang="ts">
	import { untrack } from 'svelte';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { t } from '$lib/i18n';
	import { Hash, Search, Users, ShieldX, ChevronLeft, ChevronRight } from 'lucide-svelte';
	import { EmptyState, FollowButton, SearchInput, type SearchSuggestion } from '$lib/components/ui';
	import { api } from '$lib/api/client';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	const title = $derived(t('tags.title', 'Tags — JapaConnect'));
	const description = $derived(
		t('tags.description', 'Browse all tags on JapaConnect and find content that matters to you.')
	);

	const jsonLd = $derived(
		JSON.stringify({
			'@context': 'https://schema.org',
			'@type': 'CollectionPage',
			name: 'Tags',
			description,
			url: page.url.href
		})
	);

	let query = $state(untrack(() => data.q));
	let selectedSort = $state(untrack(() => data.sort));
	const searchPlaceholder = $derived(t('tags.search_placeholder', 'Search tags…'));

	const sortOptions = $derived([
		{ value: '', label: t('tags.sort_newest', 'Newest') },
		{ value: 'popular', label: t('tags.sort_popular', 'Popular') },
		{ value: 'followers', label: t('tags.sort_followers', 'Most Followed') },
		{ value: 'content', label: t('tags.sort_content', 'Most Content') }
	]);

	function buildHref(pageNum: number): string {
		const params = new URLSearchParams();
		if (data.q) params.set('q', data.q);
		if (data.sort) params.set('sort', data.sort);
		if (pageNum > 0) params.set('page', String(pageNum));
		const qs = params.toString();
		return qs ? `/tags?${qs}` : '/tags';
	}

	function onFilterChange() {
		const params = new URLSearchParams();
		if (query.trim()) params.set('q', query.trim());
		if (selectedSort) params.set('sort', selectedSort);
		const qs = params.toString();
		goto(qs ? `/tags?${qs}` : '/tags');
	}

	async function tagSearchFetcher(q: string): Promise<SearchSuggestion[]> {
		const res = await api.get<{ data: { results: { type: string; id: string; title: string; excerpt?: string; url: string }[] } }>(
			`/search?q=${encodeURIComponent(q)}&type=tag&limit=8`
		);
		if (res.error || !res.data) return [];
		return (res.data.data?.results ?? []).map((r) => ({
			id: r.id,
			title: r.title,
			url: r.url,
			excerpt: r.excerpt,
			type: r.type
		}));
	}
</script>

<svelte:head>
	<title>{title}</title>
	<meta name="description" content={description} />
	<meta property="og:title" content={title} />
	<meta property="og:description" content={description} />
	<meta property="og:type" content="website" />
	<meta property="og:url" content={page.url.href} />
	<meta name="twitter:card" content="summary" />
	<meta name="twitter:title" content={title} />
	<meta name="twitter:description" content={description} />
	{@html `<script type="application/ld+json">${jsonLd}</script>`}
</svelte:head>

<div class="mx-auto w-full max-w-full">
	<!-- Header + search + filters -->
	<div class="border-b border-[var(--color-border)] px-4 py-4">
		<h1 class="flex items-center gap-2 text-lg font-semibold text-[var(--color-text)]">
			<Hash size={20} /> {t('tags.heading', 'Tags')}
		</h1>
		<p class="mt-1 text-sm text-[var(--color-text-subtle)]">
			{t('tags.subtitle', 'Follow tags to shape your feed.')}
		</p>

		<form class="mt-3" action="/tags" method="get" role="search">
			<SearchInput
				bind:value={query}
				placeholder={searchPlaceholder}
				ariaLabel={t('tags.heading', 'Tags')}
				fetcher={tagSearchFetcher}
			/>
			{#if selectedSort}
				<input type="hidden" name="sort" value={selectedSort} />
			{/if}
		</form>

		<!-- Sort filter chips -->
		<div class="mt-3 flex flex-wrap gap-2">
			{#each sortOptions as so (so.value)}
				<button
					type="button"
					onclick={() => { selectedSort = so.value; onFilterChange(); }}
					class="rounded-full border px-3 py-1 text-xs transition-colors {selectedSort === so.value
						? 'border-[var(--color-primary)] bg-[var(--color-primary)]/10 text-[var(--color-primary)]'
						: 'border-[var(--color-border)] text-[var(--color-muted)] hover:border-[var(--color-primary)]/40'}"
				>
					{so.label}
				</button>
			{/each}
		</div>
	</div>

	<div class="p-4">
		{#if data.forbidden}
			<div class="flex flex-col items-center gap-3 py-16 text-center">
				<ShieldX size={40} class="text-[var(--color-muted)]" />
				<p class="text-sm font-medium text-[var(--color-text)]">
					{t('tags.forbidden', 'You do not have permission to view this directory.')}
				</p>
			</div>
		{:else if data.loadError}
			<p class="py-12 text-center text-[var(--color-text-subtle)]">
				{t('tags.load_error', 'Failed to load tags. Please try again later.')}
			</p>
		{:else if data.tags.length === 0}
			<EmptyState icon={Hash} title={t('tags.empty', 'No tags yet.')} />
		{:else}
			<p class="mb-3 text-xs text-[var(--color-muted)]">
				{data.total} {t('tags.results', 'tags')}
			</p>
			<!-- 2 columns as requested -->
			<div class="grid gap-3 sm:grid-cols-2">
				{#each data.tags as tag (tag.id)}
					<div
						class="group flex items-center gap-3 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4 transition-colors hover:border-[var(--color-primary)]/40 hover:bg-[var(--color-surface-subtle)]"
					>
						<a href="/tags/{tag.slug}" class="flex min-w-0 flex-1 items-center gap-3">
							{#if tag.logo_url}
								<img
									src={tag.logo_url}
									alt={tag.name}
									loading="lazy"
									class="h-10 w-10 shrink-0 rounded-[var(--radius)] border border-[var(--color-border)] object-cover"
								/>
							{:else}
								<span
									class="grid h-10 w-10 shrink-0 place-items-center rounded-[var(--radius)] bg-[var(--color-surface-subtle)] text-[var(--color-primary)]"
								>
									<Hash size={18} />
								</span>
							{/if}
							<span class="min-w-0 flex-1">
								<span
									class="block truncate text-sm font-medium text-[var(--color-text)] group-hover:text-[var(--color-primary)]"
								>
									#{tag.name}
								</span>
								<span class="mt-1 flex items-center gap-1 text-xs text-[var(--color-muted)]">
									<Users size={12} />
									{tag.follower_count}
									{t('tags.followers', 'followers')}
								</span>
							</span>
						</a>
						{#if page.data.user}
							<FollowButton entityType="tag" entityId={tag.id} entitySlug={tag.slug} compact />
						{/if}
					</div>
				{/each}
			</div>

			<!-- Pagination -->
			{#if data.totalPages > 1}
				<div class="mt-6 flex items-center justify-center gap-4">
					<a
						href={buildHref(data.page - 1)}
						class="inline-flex items-center gap-1 rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1.5 text-sm text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)] {data.page === 0 ? 'pointer-events-none opacity-40' : ''}"
					>
						<ChevronLeft size={16} /> {t('common.prev', 'Prev')}
					</a>
					<span class="text-xs text-[var(--color-muted)]">
						{data.page + 1} / {data.totalPages}
					</span>
					<a
						href={buildHref(data.page + 1)}
						class="inline-flex items-center gap-1 rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1.5 text-sm text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)] {data.page >= data.totalPages - 1 ? 'pointer-events-none opacity-40' : ''}"
					>
						{t('common.next', 'Next')} <ChevronRight size={16} />
					</a>
				</div>
			{/if}
		{/if}
	</div>
</div>
