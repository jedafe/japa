<script lang="ts">
	import { t } from '$lib/stores/i18n.svelte';
	import { ArrowLeft, Star, MessageCircle } from 'lucide-svelte';
	import { formatTimestamp } from '$lib/utils/timezone';

	let { data } = $props();
	const reviews = $derived(data.reviews ?? []);

	function formatDate(dateStr: string): string {
		if (!dateStr) return '';
		return formatTimestamp(new Date(dateStr));
	}

	function renderStars(rating: number): string {
		return '★'.repeat(rating) + '☆'.repeat(5 - rating);
	}
</script>

<svelte:head>
	<title>Blog Reviews | JapaConnect</title>
</svelte:head>

<main class="mx-auto max-w-3xl px-4 py-8">
	<a
		href="/blog"
		class="mb-6 inline-flex items-center gap-1.5 text-sm font-medium hover:underline"
		style="color: var(--color-muted)"
	>
		<ArrowLeft class="h-4 w-4" />
		Back to blog
	</a>

	<h1 class="text-2xl font-bold tracking-tight" style="color: var(--color-foreground)">{t('blog.reviews.blog_reviews')}</h1>
	<p class="mt-2 text-sm" style="color: var(--color-muted)">
		Reviews and ratings from the community
	</p>

	{#if reviews.length === 0}
		<div class="mt-12 text-center">
			<MessageCircle class="mx-auto h-10 w-10" style="color: var(--color-muted)" />
			<p class="mt-3 text-sm" style="color: var(--color-muted)">{t('blog.reviews.no_reviews_yet')}</p>
		</div>
	{:else}
		<div class="mt-6 space-y-4">
			{#each reviews as review (review.id)}
				<div
					class="rounded-xl border p-5"
					style="border-color: var(--color-border); background: var(--color-surface)"
				>
					<div class="flex items-start justify-between gap-3">
						<div class="flex-1">
							{#if review.title}
								<h3 class="font-semibold" style="color: var(--color-foreground)">{review.title}</h3>
							{/if}
							<div class="mt-1 text-sm" style="color: #eab308">
								{renderStars(review.rating)}
							</div>
							{#if review.body}
								<p class="mt-2 text-sm leading-relaxed" style="color: var(--color-muted)">{review.body}</p>
							{/if}
						</div>
						<span class="shrink-0 text-xs" style="color: var(--color-muted)">
							{formatDate(review.created_at)}
						</span>
					</div>
					{#if review.status !== 'published'}
						<span
							class="mt-2 inline-block rounded-full px-2 py-0.5 text-xs font-medium"
							style="background: var(--color-primary-muted); color: var(--color-primary)"
						>
							{review.status}
						</span>
					{/if}
				</div>
			{/each}
		</div>
	{/if}
</main>
