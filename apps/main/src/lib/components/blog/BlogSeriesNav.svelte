<script lang="ts">
	import { t } from '$lib/i18n';
	import type { SeriesPost } from '$lib/types/content';

	interface Props {
		title: string;
		slug: string;
		total: number;
		posts: SeriesPost[];
		currentSlug: string;
		currentPart: number;
		/** Route family prefix for part links (ADR-058): '/blog' by default;
		 *  '/guide', '/stories', '/content' on other content-type singles. */
		basePath?: string;
		/** Series landing page link for the title — only /blog/series exists. */
		seriesBasePath?: string;
	}

	let { title, slug, total, posts, currentSlug, currentPart, basePath = '/blog', seriesBasePath = '/blog/series' }: Props = $props();

	// Keep the series card narrow and collapsed by default. Visible parts: 3 — the
	// first two plus the currently selected part when it isn't among them.
	const currentIndex = $derived(posts.findIndex((p) => p.slug === currentSlug));
	const visibleCount = 3;
	const topCount = 2;
	let showAll = $state(false);

	// Reset the fold state whenever the reader navigates to a different part.
	$effect(() => {
		currentSlug;
		showAll = false;
	});

	const visiblePosts = $derived(
		showAll || posts.length <= 4
			? posts.map((p, i) => ({ ...p, partIndex: i }))
			: (() => {
					const top = posts.slice(0, topCount).map((p, i) => ({ ...p, partIndex: i }));
					// The 3rd slot is the selected part whenever it is not among the first two.
					// When the selected is one of the first two, the 3rd slot is the next part,
					// giving the default 1, 2, 3 view before the user clicks deeper into the series.
					if (currentIndex >= topCount && currentIndex < posts.length) {
						return [...top, { ...posts[currentIndex], partIndex: currentIndex }];
					}
					return [...top, { ...posts[topCount], partIndex: topCount }];
				})()
	);

	const hiddenCount = $derived(Math.max(0, posts.length - visibleCount));
</script>

<nav aria-label={t('blog.series_nav', 'Series navigation')} class="mx-auto max-w-md rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
	<div class="mb-3 flex flex-wrap items-center justify-between gap-2">
		<a
			href={`${seriesBasePath}/${slug}`}
			class="text-sm font-semibold text-[var(--color-primary)] transition-colors hover:underline"
		>
			{title}
			<span class="ml-1 text-xs font-normal text-[var(--color-text-subtle)]">({total} Part Series)</span>
		</a>
		<span class="text-xs text-[var(--color-text-subtle)]">
			{t('blog.series_part_label', 'Part {part} of {total}')
				.replace('{part}', String(currentPart + 1))
				.replace('{total}', String(total))}
		</span>
	</div>

	<ul class="flex flex-col gap-2">
		{#each visiblePosts as post (post.slug)}
			{@const isCurrent = post.slug === currentSlug}
			<li>
				<a
					href={`${basePath}/${post.slug}`}
					class="group flex items-center gap-3 rounded-[var(--radius)] px-2 py-1.5 text-sm transition-colors {isCurrent ? 'bg-[var(--color-surface-subtle)] text-[var(--color-text)]' : 'text-[var(--color-text-subtle)] hover:bg-[var(--color-surface-subtle)] hover:text-[var(--color-text)]'}"
					aria-current={isCurrent ? 'page' : undefined}
				>
					<span
						class="grid h-7 w-7 shrink-0 place-items-center rounded-full text-xs font-semibold [font-variant-numeric:tabular-nums] {isCurrent ? 'bg-[var(--color-primary)] text-[var(--color-primary-contrast)]' : 'bg-[var(--color-surface-subtle)] text-[var(--color-text-subtle)] group-hover:bg-[var(--color-border)]'}"
					>
						{post.partIndex + 1}
					</span>
					<span class="line-clamp-1 {isCurrent ? 'font-semibold text-[var(--color-text)]' : ''}">{post.title}</span>
				</a>
			</li>
		{/each}

		{#if !showAll && posts.length > 4}
			<li>
				<button
					type="button"
					onclick={() => (showAll = true)}
					class="group flex w-full items-center gap-3 rounded-[var(--radius)] px-2 py-1.5 text-sm text-[var(--color-text-subtle)] transition-colors hover:bg-[var(--color-surface-subtle)] hover:text-[var(--color-text)]"
				>
					<span
						class="grid h-7 w-7 shrink-0 place-items-center rounded-full bg-[var(--color-surface-subtle)] text-[var(--color-text-subtle)] text-xs font-semibold group-hover:bg-[var(--color-border)]"
					>
						…
					</span>
					<span>
						{hiddenCount} more {hiddenCount === 1 ? t('blog.series_part_singular', 'part') : t('blog.series_part_plural', 'parts')}
					</span>
				</button>
			</li>
		{/if}
	</ul>
</nav>
