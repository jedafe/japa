<script lang="ts">
	import { page } from '$app/state';
	import { t } from '$lib/i18n';
	import { Clock, Eye, BookOpen, ArrowRight } from 'lucide-svelte';
	import { EmptyState } from '$lib/components/ui';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	const tabs = $derived([
		{ id: 'latest', label: t('blog.tab_latest', 'Latest'), href: '?tab=latest' },
		{ id: 'featured', label: t('blog.tab_featured', 'Featured'), href: '?tab=featured' },
		{ id: 'series', label: t('blog.tab_series', 'Series'), href: '?tab=series' }
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
	<div class="border-b border-[var(--color-border)]">
		<div class="flex gap-1 px-4">
			{#each tabs as tab (tab.id)}
				<a
					href={tab.href}
					class="border-b-2 px-4 py-3 text-sm font-medium transition-colors {data.tab === tab.id
						? 'border-[var(--color-primary)] text-[var(--color-primary)]'
						: 'border-transparent text-[var(--color-text-subtle)] hover:text-[var(--color-text)]'}"
				>
					{tab.label}
				</a>
			{/each}
		</div>
	</div>

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
				<!-- Ghost.org magazine card grid with serif titles -->
				<div class="blog-grid grid gap-6 sm:grid-cols-2 lg:grid-cols-3">
					{#each data.items as post (post.id)}
						<a href="/blog/{post.slug}" class="group flex flex-col overflow-hidden rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] transition-all hover:border-[var(--color-primary)]/40 hover:shadow-lg">
							{#if post.featured_image_url}
								<div class="aspect-[16/9] overflow-hidden">
									<img src={post.featured_image_url} alt={post.title} class="h-full w-full object-cover transition-transform duration-300 group-hover:scale-105" loading="lazy" />
								</div>
							{:else}
								<div class="aspect-[16/9] bg-gradient-to-br from-[var(--color-primary)]/12 via-[var(--color-surface-subtle)] to-[var(--color-surface-subtle)]"></div>
							{/if}
							<div class="flex flex-1 flex-col p-5">
								<span class="mb-2 text-xs font-medium uppercase tracking-wide text-[var(--color-primary)]">{t('blog.card_label', 'Blog post')}</span>
								<h2 class="text-xl font-semibold leading-snug text-[var(--color-text)] group-hover:text-[var(--color-primary)]" style="font-family: var(--font-serif);">{post.title}</h2>
								<p class="mt-2 line-clamp-3 flex-1 text-sm text-[var(--color-text-subtle)]">{post.excerpt}</p>
								<div class="mt-4 flex items-center gap-3 text-xs text-[var(--color-muted)]">
									{#if post.read_time_minutes > 0}
										<span class="flex items-center gap-1"><Clock size={12} /> {post.read_time_minutes} {t('blog.min_read', 'min read')}</span>
									{/if}
									<span class="flex items-center gap-1"><Eye size={12} /> {post.view_count}</span>
								</div>
							</div>
						</a>
					{/each}
				</div>

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
							class="rounded-lg border border-[var(--color-border)] px-4 py-2 text-sm font-medium text-[var(--color-text)] transition-colors hover:border-[var(--color-primary)]/40 !hasNext ? 'pointer-events-none opacity-40' : ''}"
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
