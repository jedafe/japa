<script lang="ts">
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { t } from '$lib/i18n';
	import { BookOpen, ArrowRight } from 'lucide-svelte';
	import { EmptyState, UrlTabs, type UrlTab } from '$lib/components/ui';
	import { cardStyleFor, gridMode, type CardStyleConfig } from '$lib/feed/cardStyle';
	import type { FeedItem } from '$lib/types';
	import FeedCard from '$lib/components/feed/FeedCard.svelte';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	const config = $derived((page.data.cardStyleConfig as CardStyleConfig) ?? { default: 'reddit', byType: {} });

	// Map ContentArticle → FeedItem so blog posts render through FeedCard.
	function toFeedItem(post: typeof data.items[number]): FeedItem {
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
			excerpt: post.excerpt,
			created_at: post.created_at,
			view_count: post.view_count,
			comment_count: post.comment_count,
			reading_time_minutes: post.read_time_minutes,
			cover_image_url: post.featured_image_url,
			tags: post.tags
		};
	}

	const feedItems = $derived(data.items.map(toFeedItem));
	const gridLayout = $derived(gridMode(feedItems.map((i) => i.content_type), config));

	// Tab bar values — switching calls goto() so SvelteKit re-runs the load
	// function for the new tab's per-tab data.
	const tabs = $derived<UrlTab[]>([
		{ value: 'latest', label: t('blog.tab_latest', 'Latest') },
		{ value: 'featured', label: t('blog.tab_featured', 'Featured') },
		{ value: 'series', label: t('blog.tab_series', 'Series') }
	]);

	const title = $derived(t('blog.list_title', 'Blog — JapaConnect'));
	const description = $derived(t('blog.list_description', 'Stories, guides, and insights from the Nigerian diaspora community.'));

	// Pagination: use meta.total when available (accurate), fall back to
	// the items-length heuristic (items < pageSize → last page).
	const hasNext = $derived(
		data.meta?.total != null
			? data.page * data.pageSize < data.meta.total
			: data.items.length >= data.pageSize
	);
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
	<!-- Blog tab bar (Latest / Featured / Series) -->
	<UrlTabs {tabs} value={data.tab} onValueChange={(v) => goto(`?tab=${v}`)} />

	<div class="p-4">
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
								<span
									class="grid h-10 w-10 shrink-0 place-items-center rounded-[var(--radius)] bg-[var(--color-surface-subtle)] text-[var(--color-primary)]"
								>
									<BookOpen size={20} />
								</span>
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

				<!-- Pagination (BUG-005: offset-based per Rule 5) -->
				{#if data.pageSize > 0}
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
							href="?tab={data.tab}&page={Math.max(1, data.page - 1)}"
							class="rounded-lg border border-[var(--color-border)] px-4 py-2 text-sm font-medium text-[var(--color-text)] transition-colors hover:border-[var(--color-primary)]/40 disabled:pointer-events-none {data.page <= 1 ? 'pointer-events-none opacity-40' : ''}"
							aria-disabled={data.page <= 1}
						>
							{t('blog.prev_page', 'Previous')}
						</a>
						<span class="text-sm text-[var(--color-text-subtle)]">{data.page}</span>
						<a
							href="?tab={data.tab}&page={data.page + 1}"
							class="rounded-lg border border-[var(--color-border)] px-4 py-2 text-sm font-medium text-[var(--color-text)] transition-colors hover:border-[var(--color-primary)]/40 {!hasNext ? 'pointer-events-none opacity-40' : ''}"
							aria-disabled={!hasNext}
						>
							{t('blog.next_page', 'Next')}
						</a>
					</div>
				{/if}
			{/if}
		{/if}
	</div>
</div>

<style>
	/* When nav is expanded (wider), show 2 columns; when collapsed (narrow), show 3 columns */
	:global(.shell[data-nav-collapsed="false"]) .blog-grid {
		grid-template-columns: repeat(2, 1fr);
	}
	:global(.shell[data-nav-collapsed="true"]) .blog-grid {
		grid-template-columns: repeat(3, 1fr);
	}
</style>
