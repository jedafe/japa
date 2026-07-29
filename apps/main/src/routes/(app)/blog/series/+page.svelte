<script lang="ts">
	import { t } from '$lib/i18n';
	import { page } from '$app/state';
	import { BookOpen, ArrowRight } from 'lucide-svelte';
	import { EmptyState } from '$lib/components/ui';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	const title = $derived(t('blog_series.title', 'Blog Series — JapaConnect'));
	const description = $derived(
		t('blog_series.description', 'Browse curated blog series on JapaConnect.')
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

<div class="mx-auto w-full max-w-3xl">
	<!-- Header -->
	<div class="border-b border-[var(--color-border)] px-4 py-3">
		<h1 class="flex items-center gap-2 text-lg font-semibold text-[var(--color-text)]">
			<BookOpen size={20} /> {t('blog_series.heading', 'Blog Series')}
		</h1>
		<p class="mt-1 text-sm text-[var(--color-text-subtle)]">
			{t('blog_series.subtitle', 'Curated collections of posts')}
		</p>
	</div>

	<div class="p-3">
		{#if data.series.length === 0}
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
	</div>
</div>
