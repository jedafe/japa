<script lang="ts">
	import { t } from '$lib/stores/i18n.svelte';
	import { ArrowLeft, Clock, Edit, Eye, MessageCircle, Send, Share2, ThumbsUp, ThumbsDown, Trash2 } from 'lucide-svelte';
	import { api } from '$lib/api';
	import { isFeatureEnabled } from '$lib/stores/features.svelte';
	import { formatTimestampLong } from '$lib/utils/timezone';
	import ReactionBar from '$lib/components/ReactionBar.svelte';
	import RevisionHistory from '$lib/components/RevisionHistory.svelte';
	import PinButton from '$lib/components/PinButton.svelte';

	let { data } = $props();
	let post = $derived(data.post);

	let upvoting = $state(false);
	let downvoting = $state(false);
	let reactionMessage = $state('');
	let submitting = $state(false);
	let deleting = $state(false);

	async function handleReaction(type: 'upvote' | 'downvote') {
		if (type === 'upvote') upvoting = true;
		else downvoting = true;
		reactionMessage = '';

		try {
			await api.addReaction(post.id, type);
			reactionMessage = type === 'upvote' ? 'Upvoted!' : 'Downvoted!';
		} catch {
			reactionMessage = 'Could not record reaction.';
		} finally {
			upvoting = false;
			downvoting = false;
		}
	}

	async function handleSubmitForReview() {
		if (submitting) return;
		submitting = true;
		try {
			await api.submitForReview(post.id);
			reactionMessage = 'Submitted for review!';
		} catch {
			reactionMessage = 'Failed to submit for review.';
		} finally {
			submitting = false;
		}
	}

	async function handleDelete() {
		if (deleting || !confirm('Delete this post? This cannot be undone.')) return;
		deleting = true;
		try {
			await api.deleteContent(post.id);
			window.location.href = '/blog';
		} catch {
			reactionMessage = 'Failed to delete post.';
			deleting = false;
		}
	}

	function formatDate(dateStr: string): string {
		if (!dateStr) return '';
		const d = new Date(dateStr);
		return formatTimestampLong(d);
	}
</script>

<svelte:head>
	<title>{post.title} | JapaConnect Blog</title>
	<meta name="description" content={post.title} />
</svelte:head>

{#if !isFeatureEnabled('feature.blog')}
	<main class="mx-auto max-w-2xl px-6 py-16">
		<div class="text-center">
			<h1 class="text-2xl font-bold" style="color: var(--color-foreground)">{t('blog.slug.blog_is_not_available')}</h1>
			<p class="mt-2" style="color: var(--color-muted)">{t('blog.slug.this_feature_is_currently_disabled')}</p>
			<a href="/" class="mt-4 inline-block" style="color: var(--color-primary)">{t('blog.slug.go_home')}</a>
		</div>
	</main>
{:else}
	<main class="mx-auto max-w-3xl px-4 py-8">
	<a
		href="/blog"
		class="mb-6 inline-flex items-center gap-1.5 text-sm font-medium hover:underline"
		style="color: var(--color-muted)"
	>
		<ArrowLeft class="h-4 w-4" />
		Back to blog
	</a>

	<article>
		<header class="mb-8">
			<div class="flex items-start justify-between gap-4">
				<h1 class="text-3xl font-bold tracking-tight sm:text-4xl" style="color: var(--color-foreground)">
					{post.title}
				</h1>
				<div class="flex items-center gap-2">
					<a
						href="/blog/{post.slug}/edit"
						class="inline-flex items-center gap-1.5 rounded-lg border px-3 py-1.5 text-sm font-medium transition-colors hover:opacity-80"
						style="border-color: var(--color-border); color: var(--color-foreground)"
					>
						<Edit class="h-3.5 w-3.5" />
						Edit
					</a>
					{#if post.status === 'draft'}
						<button
							onclick={handleSubmitForReview}
							disabled={submitting}
							class="inline-flex items-center gap-1.5 rounded-lg border px-3 py-1.5 text-sm font-medium transition-colors hover:opacity-80 disabled:opacity-50"
							style="border-color: var(--color-border); color: var(--color-primary)"
						>
							<Send class="h-3.5 w-3.5" />
							{submitting ? 'Submitting...' : 'Submit for Review'}
						</button>
					{/if}
					<button
						onclick={handleDelete}
						disabled={deleting}
						class="inline-flex items-center gap-1.5 rounded-lg border px-3 py-1.5 text-sm font-medium transition-colors hover:bg-red-50 dark:hover:bg-red-900/20 disabled:opacity-50"
						style="border-color: var(--color-border); color: #ef4444"
					>
						<Trash2 class="h-3.5 w-3.5" />
					</button>
					<PinButton contentId={post.id} />
				</div>
			</div>
			<div class="mt-4 flex flex-wrap items-center gap-4 text-sm" style="color: var(--color-muted)">
				{#if post.published_at}
					<time datetime={post.published_at}>{formatDate(post.published_at)}</time>
				{:else if post.created_at}
					<time datetime={post.created_at}>{formatDate(post.created_at)}</time>
				{/if}
				{#if post.read_time_minutes > 0}
					<span class="flex items-center gap-1">
						<Clock class="h-4 w-4" />
						{post.read_time_minutes} min read
					</span>
				{/if}
			</div>
		</header>

		<!-- Post Content -->
		<div class="prose-custom" style="color: var(--color-foreground)">
			{@html post.body_html}
		</div>

		<!-- Stats -->
		<div class="mt-8 flex items-center gap-6 border-t pt-6 text-sm" style="border-color: var(--color-border); color: var(--color-muted)">
			{#if post.view_count > 0}
				<span class="flex items-center gap-1.5">
					<Eye class="h-4 w-4" />
					{post.view_count} views
				</span>
			{/if}
			{#if post.comment_count > 0}
				<span class="flex items-center gap-1.5">
					<MessageCircle class="h-4 w-4" />
					{post.comment_count} comments
				</span>
			{/if}
			{#if post.share_count > 0}
				<span class="flex items-center gap-1.5">
					<Share2 class="h-4 w-4" />
					{post.share_count} shares
				</span>
			{/if}
		</div>

		<!-- Reactions -->
		<div class="mt-6 flex items-center gap-3">
			<button
				onclick={() => handleReaction('upvote')}
				disabled={upvoting}
				class="flex items-center gap-1.5 rounded-lg border px-3 py-1.5 text-sm font-medium transition-colors hover:opacity-80 disabled:opacity-50"
				style="border-color: var(--color-border); color: var(--color-foreground)"
			>
				<ThumbsUp class="h-4 w-4" />
				Upvote
			</button>
			<button
				onclick={() => handleReaction('downvote')}
				disabled={downvoting}
				class="flex items-center gap-1.5 rounded-lg border px-3 py-1.5 text-sm font-medium transition-colors hover:opacity-80 disabled:opacity-50"
				style="border-color: var(--color-border); color: var(--color-foreground)"
			>
				<ThumbsDown class="h-4 w-4" />
				Downvote
			</button>
			{#if reactionMessage}
				<span class="text-xs" style="color: var(--color-muted)">{reactionMessage}</span>
			{/if}
		</div>

		<ReactionBar contentId={post.id} />
		<RevisionHistory contentId={post.id} />
	</article>
	</main>
{/if}

<style>
	.prose-custom :global(h2) {
		font-size: 1.5rem;
		font-weight: 700;
		margin-top: 2rem;
		margin-bottom: 0.75rem;
	}
	.prose-custom :global(h3) {
		font-size: 1.25rem;
		font-weight: 600;
		margin-top: 1.5rem;
		margin-bottom: 0.5rem;
	}
	.prose-custom :global(p) {
		margin-bottom: 1rem;
		line-height: 1.75;
	}
	.prose-custom :global(ul),
	.prose-custom :global(ol) {
		margin-bottom: 1rem;
		padding-left: 1.5rem;
	}
	.prose-custom :global(li) {
		margin-bottom: 0.25rem;
	}
	.prose-custom :global(blockquote) {
		border-left: 3px solid var(--color-primary);
		padding-left: 1rem;
		margin: 1.5rem 0;
		font-style: italic;
		color: var(--color-muted);
	}
	.prose-custom :global(pre) {
		background: var(--color-background);
		border-radius: 0.5rem;
		padding: 1rem;
		overflow-x: auto;
		margin: 1.5rem 0;
	}
	.prose-custom :global(code) {
		font-size: 0.875rem;
	}
	.prose-custom :global(img) {
		max-width: 100%;
		border-radius: 0.5rem;
		margin: 1.5rem 0;
	}
	.prose-custom :global(a) {
		color: var(--color-primary);
		text-decoration: underline;
	}
</style>
