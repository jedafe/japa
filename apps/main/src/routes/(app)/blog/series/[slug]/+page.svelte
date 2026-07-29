<script lang="ts">
	import { t } from '$lib/i18n';
	import { page } from '$app/state';
	import { BookOpen, FileText, ArrowLeft } from 'lucide-svelte';
	import { EmptyState } from '$lib/components/ui';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	const series = $derived(data.series);

	const title = $derived(
		t('blog_series_detail.title', '{name} — JapaConnect').replace('{name}', series.title)
	);
	const description = $derived(
		series.description ??
			t('blog_series_detail.description', 'Posts in the {name} series.').replace(
				'{name}',
				series.title
			)
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

<div class="mx-auto w-full max-w-2xl">
	<!-- Series header -->
	<div class="border-b border-[var(--color-border)] px-4 py-4">
		<a
			href="/blog/series"
			class="mb-2 inline-flex items-center gap-1.5 text-sm text-[var(--color-text-subtle)] transition-colors hover:text-[var(--color-text)]"
		>
			<ArrowLeft size={16} /> {t('blog_series_detail.back', 'All series')}
		</a>
		<div class="flex items-center gap-3">
			<span
				class="grid h-12 w-12 shrink-0 place-items-center rounded-[var(--radius)] bg-[var(--color-surface-subtle)] text-[var(--color-primary)]"
			>
				<BookOpen size={24} />
			</span>
			<div class="min-w-0 flex-1">
				<h1 class="truncate text-xl font-semibold text-[var(--color-text)]">{series.title}</h1>
				{#if series.status}
					<span class="text-xs capitalize text-[var(--color-muted)]">{series.status}</span>
				{/if}
			</div>
		</div>
		{#if series.description}
			<p class="mt-3 text-sm text-[var(--color-text-subtle)]">{series.description}</p>
		{/if}
	</div>

	<div class="p-3">
		{#if data.posts.length === 0}
			<EmptyState icon={FileText} title={t('blog_series_detail.empty', 'No posts in this series yet')} />
		{:else}
			<ol class="space-y-2">
				{#each data.posts as post (post.content_id)}
					<li>
						<a
							href="/blog/{post.slug}"
							class="flex items-center gap-3 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4 transition-colors hover:bg-[var(--color-surface-subtle)]"
						>
							<span
								class="grid h-8 w-8 shrink-0 place-items-center rounded-[var(--radius)] bg-[var(--color-surface-subtle)] text-sm font-semibold text-[var(--color-text-subtle)]"
							>
								{post.position}
							</span>
							<span class="min-w-0 flex-1">
								<span class="flex items-center gap-1.5 text-sm font-medium text-[var(--color-text)]">
									<FileText size={14} class="shrink-0 text-[var(--color-text-subtle)]" />
									<span class="truncate">{post.title}</span>
								</span>
								<span class="mt-0.5 block text-xs capitalize text-[var(--color-muted)]">
									{post.status}
								</span>
							</span>
						</a>
					</li>
				{/each}
			</ol>
		{/if}
	</div>
</div>
