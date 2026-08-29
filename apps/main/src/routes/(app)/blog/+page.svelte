<script lang="ts">
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { t } from '$lib/i18n';
	import { BookOpen, ArrowRight, Search, X } from 'lucide-svelte';
	import { EmptyState, UrlTabs, type UrlTab } from '$lib/components/ui';
	import { cardStyleFor, gridMode, type CardStyleConfig } from '$lib/feed/cardStyle';
	import type { FeedItem } from '$lib/types';
	import FeedCard from '$lib/components/feed/FeedCard.svelte';
	import SelectField from '$lib/components/ui/SelectField.svelte';
	import { api } from '$lib/api/client';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	const config = $derived((page.data.cardStyleConfig as CardStyleConfig) ?? { default: 'reddit', byType: {} });

	interface SpaceOption {
		id: string;
		name: string;
		slug: string;
	}

	interface TagOption {
		id: string;
		name: string;
		slug: string;
	}

	interface UserOption {
		id: string;
		username: string;
		display_name?: string;
	}

	let spaces = $state<SpaceOption[]>([]);
	let tags = $state<TagOption[]>([]);
	let users = $state<UserOption[]>([]);
	let filterTag = $state('');
	let filterSpaceId = $state('');
	let filterAuthorId = $state('');

	// Sync local filter state with the URL-driven server data on navigation.
	$effect(() => {
		filterTag = data.tag ?? '';
		filterSpaceId = data.spaceId ?? '';
		filterAuthorId = data.authorId ?? '';
	});

	// Load filter option lists once on client mount (Rule 16 — all data from API).
	$effect(() => {
		(async () => {
			const [sRes, tRes, uRes] = await Promise.all([
				api.get<SpaceOption[]>('/spaces?page_size=50&sort=name'),
				api.get<TagOption[]>('/tags?page_size=50&sort=name'),
				api.get<UserOption[]>('/users?page_size=50')
			]);
			if (sRes.data) spaces = sRes.data;
			if (tRes.data) tags = tRes.data;
			if (uRes.data) users = uRes.data;
		})();
	});

	function toFeedItem(post: (typeof data.items)[number]): FeedItem {
		return {
			id: post.id,
			title: post.title,
			slug: post.slug,
			content_type: 'blog_post',
			author_id: post.author_id ?? '',
			author_name: post.author_name,
			author_avatar_url: post.author_avatar_url,
			space_id: post.space_id,
			space_name: post.space_name,
			space_slug: post.space_slug,
			excerpt: post.excerpt,
			created_at: post.created_at,
			view_count: post.view_count,
			comment_count: post.comment_count,
			reading_time_minutes: post.read_time_minutes,
			cover_image_url: post.featured_image_url,
			vote_score: post.reshare_count ?? post.share_count ?? 0,
			tags: post.tags
		};
	}

	const feedItems = $derived(data.items.map(toFeedItem));
	const gridLayout = $derived(gridMode(feedItems.map((i) => i.content_type), config));

	const tabs = $derived<UrlTab[]>([
		{ value: 'latest', label: t('blog.tab_latest', 'Latest') },
		{ value: 'top', label: t('blog.tab_top', 'Top') },
		{ value: 'featured', label: t('blog.tab_featured', 'Featured') },
		{ value: 'series', label: t('blog.tab_series', 'Series') }
	]);

	const title = $derived(t('blog.list_title', 'Blog — JapaConnect'));
	const description = $derived(
		t('blog.list_description', 'Stories, guides, and insights from the diaspora community.')
	);

	const hasNext = $derived(
		data.meta?.total != null
			? data.page * data.pageSize < data.meta.total
			: data.items.length >= data.pageSize
	);

	const activeFilters = $derived(
		[filterTag, filterSpaceId, filterAuthorId].some((v) => v !== '')
	);

	function switchTab(tab: string) {
		// Filters only apply to the main blog list; clear them for
		// featured and series to avoid misleading URLs.
		if (tab === 'latest' || tab === 'top') {
			goto(buildUrl({ tab, page: 1 }));
		} else {
			filterTag = '';
			filterSpaceId = '';
			filterAuthorId = '';
			goto(buildUrl({ tab, page: 1, tag: '', space_id: '', author_id: '' }));
		}
	}

	function buildUrl(next: Partial<{ tab: string; page: number; tag: string; space_id: string; author_id: string }>): string {
		const params = new URLSearchParams(page.url.searchParams);
		if (next.tab != null) params.set('tab', next.tab);
		if (next.page != null) {
			if (next.page <= 1) params.delete('page');
			else params.set('page', String(next.page));
		} else {
			params.delete('page');
		}
		if (next.tag != null) {
			if (next.tag === '') params.delete('tag');
			else params.set('tag', next.tag);
		}
		if (next.space_id != null) {
			if (next.space_id === '') params.delete('space_id');
			else params.set('space_id', next.space_id);
		}
		if (next.author_id != null) {
			if (next.author_id === '') params.delete('author_id');
			else params.set('author_id', next.author_id);
		}
		return `?${params.toString()}`;
	}

	function applyFilters() {
		goto(buildUrl({ page: 1, tag: filterTag, space_id: filterSpaceId, author_id: filterAuthorId }));
	}

	function clearFilters() {
		filterTag = '';
		filterSpaceId = '';
		filterAuthorId = '';
		goto(buildUrl({ page: 1, tag: '', space_id: '', author_id: '' }));
	}
</script>

<svelte:head>
	<title>{title}</title>
	<meta name="description" content={description} />
	<meta property="og:title" content={title} />
	<meta property="og:description" content={description} />
	<meta property="og:type" content="website" />
	<meta property="og:url" content={page.url.href} />
</svelte:head>

<div class="mx-auto w-full max-w-full">
	<UrlTabs {tabs} value={data.tab} onValueChange={(v) => switchTab(v)} />

	<div class="p-4">
		{#if data.tab === 'latest' || data.tab === 'top'}
			<div class="mb-4 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-3">
				<div class="flex flex-wrap items-end gap-3">
					<div class="min-w-0 flex-1">
						<label for="blog-filter-tag" class="mb-1 block text-xs font-medium text-[var(--color-text-subtle)]">
							{t('blog.filter.tag', 'Tag')}
						</label>
						<SelectField
							id="blog-filter-tag"
							value={filterTag}
							options={[{ value: '', label: t('blog.filter.any_tag', 'Any tag') }, ...tags.map((t) => ({ value: t.slug, label: t.name }))]}
							placeholder={t('blog.filter.any_tag', 'Any tag')}
							onValueChange={(v) => (filterTag = v)}
						/>
					</div>
					<div class="min-w-0 flex-1">
						<label for="blog-filter-space" class="mb-1 block text-xs font-medium text-[var(--color-text-subtle)]">
							{t('blog.filter.space', 'Space')}
						</label>
						<SelectField
							id="blog-filter-space"
							value={filterSpaceId}
							options={[{ value: '', label: t('blog.filter.any_space', 'Any space') }, ...spaces.map((s) => ({ value: s.id, label: s.name }))]}
							placeholder={t('blog.filter.any_space', 'Any space')}
							onValueChange={(v) => (filterSpaceId = v)}
						/>
					</div>
					<div class="min-w-0 flex-1">
						<label for="blog-filter-author" class="mb-1 block text-xs font-medium text-[var(--color-text-subtle)]">
							{t('blog.filter.author', 'Author')}
						</label>
						<SelectField
							id="blog-filter-author"
							value={filterAuthorId}
							options={[{ value: '', label: t('blog.filter.any_author', 'Any author') }, ...users.map((u) => ({ value: u.id, label: u.display_name || u.username }))]}
							placeholder={t('blog.filter.any_author', 'Any author')}
							onValueChange={(v) => (filterAuthorId = v)}
						/>
					</div>
					<div class="flex items-center gap-2">
						<button
							type="button"
							onclick={applyFilters}
							class="inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-3 py-2 text-sm font-medium text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary)]/90"
						>
							<Search size={14} /> {t('blog.filter.apply', 'Apply')}
						</button>
						{#if activeFilters}
							<button
								type="button"
								onclick={clearFilters}
								class="inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)]"
							>
								<X size={14} /> {t('blog.filter.clear', 'Clear')}
							</button>
						{/if}
					</div>
				</div>
			</div>
		{/if}

		{#if data.tab === 'series'}
			{#if data.loadError}
				<p class="py-12 text-center text-[var(--color-text-subtle)]">{t('blog.load_error', 'Failed to load posts. Please try again later.')}</p>
			{:else if data.series.length === 0}
				<EmptyState icon={BookOpen} title={t('blog_series.empty', 'No series yet')} />
			{:else}
				<div class="grid gap-3 sm:grid-cols-2">
					{#each data.series as series (series.id)}
						<a
							href="/blog/series/{series.slug}"
							class="group flex flex-col rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4 transition-colors hover:bg-[var(--color-surface-subtle)]"
						>
							<div class="flex items-start gap-3">
								{#if series.cover_image_url}
									<img
										src={series.cover_image_url}
										alt=""
										class="h-10 w-10 shrink-0 rounded-[var(--radius)] object-cover"
									/>
								{:else}
									<span
										class="grid h-10 w-10 shrink-0 place-items-center rounded-[var(--radius)] bg-[var(--color-surface-subtle)] text-[var(--color-primary)]"
									>
										<BookOpen size={20} />
									</span>
								{/if}
								<div class="min-w-0 flex-1">
									<h2 class="truncate text-sm font-medium text-[var(--color-text)] group-hover:text-[var(--color-primary)]">
										{series.title}
									</h2>
									{#if series.status}
										<span class="mt-0.5 inline-block text-xs capitalize text-[var(--color-muted)]">
											{series.status}
										</span>
									{/if}
								</div>
							</div>
							{#if series.description}
								<p class="mt-3 line-clamp-2 flex-1 text-sm text-[var(--color-text-subtle)]">
									{series.description}
								</p>
							{/if}
							<div class="mt-3 flex items-center justify-end text-xs text-[var(--color-text-subtle)]">
								<span class="flex items-center gap-1 group-hover:text-[var(--color-primary)]">
									{t('blog_series.view', 'View series')} <ArrowRight size={14} />
								</span>
							</div>
						</a>
					{/each}
				</div>

				{#if data.meta?.total != null && data.meta.total > data.pageSize}
					<div class="mt-4 flex items-center justify-center gap-4">
						<a
							href="?tab=series&page={Math.max(1, data.page - 1)}{data.q ? '&q=' + encodeURIComponent(data.q) : ''}"
							class="rounded-lg border border-[var(--color-border)] px-4 py-2 text-sm font-medium text-[var(--color-text)] transition-colors hover:border-[var(--color-primary)]/40 disabled:pointer-events-none {data.page <= 1 ? 'pointer-events-none opacity-40' : ''}"
							aria-disabled={data.page <= 1}
						>
							{t('blog.prev_page', 'Previous')}
						</a>
						<span class="text-sm text-[var(--color-text-subtle)]">{data.page}</span>
						<a
							href="?tab=series&page={data.page + 1}{data.q ? '&q=' + encodeURIComponent(data.q) : ''}"
							class="rounded-lg border border-[var(--color-border)] px-4 py-2 text-sm font-medium text-[var(--color-text)] transition-colors hover:border-[var(--color-primary)]/40 {data.page * data.pageSize >= (data.meta?.total ?? 0) ? 'pointer-events-none opacity-40' : ''}"
						>
							{t('blog.next_page', 'Next')}
						</a>
					</div>
				{/if}
			{/if}
		{:else}
			{#if data.loadError}
				<p class="py-12 text-center text-[var(--color-text-subtle)]">{t('blog.load_error', 'Failed to load posts. Please try again later.')}</p>
			{:else if data.items.length === 0}
				<p class="py-12 text-center text-[var(--color-text-subtle)]">{t('blog.empty', 'No posts yet. Check back soon.')}</p>
			{:else}
				{#if gridLayout}
					<div class="blog-grid grid gap-3.5 sm:grid-cols-2">
						{#each feedItems as item (item.id)}
							<FeedCard {item} style={cardStyleFor(item.content_type, config)} layout="grid-cell" />
						{/each}
					</div>
				{:else}
					<div class="divide-y divide-[var(--color-border)] overflow-hidden rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)]">
						{#each feedItems as item (item.id)}
							<FeedCard {item} style={cardStyleFor(item.content_type, config)} layout="stream" />
						{/each}
					</div>
				{/if}

				{#if data.meta?.total != null && data.meta.total > 0}
					<p class="mt-6 text-center text-xs text-[var(--color-muted)]">
						{t('common.showing', 'Showing {start}-{end} of {total}')
							.replace('{start}', String((data.page - 1) * data.pageSize + 1))
							.replace('{end}', String(Math.min(data.page * data.pageSize, data.meta.total)))
							.replace('{total}', String(data.meta.total))}
					</p>
				{/if}
				<div class="mt-2 flex items-center justify-center gap-4">
					<a
						href={buildUrl({ page: Math.max(1, data.page - 1) })}
						class="rounded-lg border border-[var(--color-border)] px-4 py-2 text-sm font-medium text-[var(--color-text)] transition-colors hover:border-[var(--color-primary)]/40 disabled:pointer-events-none {data.page <= 1 ? 'pointer-events-none opacity-40' : ''}"
						aria-disabled={data.page <= 1}
					>
						{t('blog.prev_page', 'Previous')}
					</a>
					<span class="text-sm text-[var(--color-text-subtle)]">{data.page}</span>
					<a
						href={buildUrl({ page: data.page + 1 })}
						class="rounded-lg border border-[var(--color-border)] px-4 py-2 text-sm font-medium text-[var(--color-text)] transition-colors hover:border-[var(--color-primary)]/40 {!hasNext ? 'pointer-events-none opacity-40' : ''}"
						aria-disabled={!hasNext}
					>
						{t('blog.next_page', 'Next')}
					</a>
				</div>
			{/if}
		{/if}
	</div>
</div>

<style>
	:global(.shell[data-nav-collapsed="false"]) .blog-grid {
		grid-template-columns: repeat(2, 1fr);
	}
	:global(.shell[data-nav-collapsed="true"]) .blog-grid {
		grid-template-columns: repeat(3, 1fr);
	}
</style>
