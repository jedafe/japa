<script lang="ts">
	import { t } from '$lib/i18n';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { BookOpen, ArrowRight, Search, X } from 'lucide-svelte';
	import { EmptyState } from '$lib/components/ui';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	let q = $state('');
	$effect(() => {
		q = data.q ?? '';
	});

	const title = $derived(t('blog_series.title', 'Blog Series — JapaConnect'));
	const description = $derived(
		t('blog_series.description', 'Browse curated blog series on JapaConnect.')
	);

	const hasNext = $derived(
		data.meta?.total != null
			? data.page * data.pageSize < data.meta.total
			: data.series.length >= data.pageSize
	);

	function buildUrl(next: { page?: number; q?: string }): string {
		const params = new URLSearchParams(page.url.searchParams);
		if (next.q != null) {
			if (next.q === '') {
				params.delete('q');
			} else {
				params.set('q', next.q);
			}
		}
		if (next.page != null) {
			if (next.page <= 1) {
				params.delete('page');
			} else {
				params.set('page', String(next.page));
			}
		}
		return `?${params.toString()}`;
	}

	function applySearch() {
		goto(buildUrl({ q: q.trim(), page: 1 }));
	}

	function clearSearch() {
		q = '';
		goto(buildUrl({ q: '', page: 1 }));
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
	<!-- Header -->
	<div class="border-b border-[var(--color-border)] px-4 py-3">
		<h1 class="flex items-center gap-2 text-lg font-semibold text-[var(--color-text)]">
			<BookOpen size={20} /> {t('blog_series.heading', 'Blog Series')}
		</h1>
		<p class="mt-1 text-sm text-[var(--color-text-subtle)]">
			{t('blog_series.subtitle', 'Curated collections of posts')}
		</p>
	</div>

	<!-- Search -->
	<div class="border-b border-[var(--color-border)] px-4 py-3">
		<div class="flex items-start gap-2">
			<div class="relative min-w-0 flex-1">
				<input
					id="series-search"
					type="search"
					bind:value={q}
					placeholder={t('blog_series.search_placeholder', 'Search series...')}
					class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 pl-9 text-sm text-[var(--color-text)] placeholder:text-[var(--color-muted)] focus:border-[var(--color-primary)] focus:outline-none"
					onkeydown={(e) => e.key === 'Enter' && applySearch()}
				/>
				<Search
					size={16}
					class="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-[var(--color-muted)]"
				/>
			</div>
			<button
				type="button"
				onclick={applySearch}
				class="inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-3 py-2 text-sm font-medium text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary)]/90"
			>
				<Search size={14} /> {t('blog_series.search', 'Search')}
			</button>
			{#if q || data.q}
				<button
					type="button"
					onclick={clearSearch}
					class="inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)]"
				>
					<X size={14} /> {t('blog_series.clear', 'Clear')}
				</button>
			{/if}
		</div>
	</div>

	<div class="p-3">
		{#if data.loadError}
			<p class="py-12 text-center text-[var(--color-text-subtle)]">
				{t('blog.load_error', 'Failed to load series. Please try again later.')}
			</p>
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
