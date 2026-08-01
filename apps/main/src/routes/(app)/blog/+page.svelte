<script lang="ts">
	import { page } from '$app/state';
	import { t } from '$lib/i18n';
	import { Clock, Eye } from 'lucide-svelte';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	const tabs = $derived([
		{ id: 'latest', label: t('blog.tab_latest', 'Latest'), href: '?tab=latest' },
		{ id: 'featured', label: t('blog.tab_featured', 'Featured'), href: '?tab=featured' },
		{ id: 'series', label: t('blog.tab_series', 'Series'), href: '/blog/series' }
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

<div class="mx-auto w-full max-w-4xl">
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
		{#if data.loadError}
			<p class="py-12 text-center text-[var(--color-text-subtle)]">{t('blog.load_error', 'Failed to load posts. Please try again later.')}</p>
		{:else if data.items.length === 0}
			<p class="py-12 text-center text-[var(--color-text-subtle)]">{t('blog.empty', 'No posts yet. Check back soon.')}</p>
		{:else}
			<!-- Ghost.org magazine card grid with serif titles -->
			<div class="grid gap-6 sm:grid-cols-2 lg:grid-cols-3">
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
				<div class="mt-8 flex items-center justify-center gap-4">
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
	</div>
</div>
