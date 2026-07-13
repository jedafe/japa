<script lang="ts">
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import {
		ArrowLeft,
		ArrowBigUp,
		ArrowBigDown,
		CheckCircle2,
		MessageSquare,
		Bookmark,
		BookmarkCheck,
		Loader2,
		BarChart3
	} from 'lucide-svelte';
	import { EmptyState } from '$lib/components/ui';
	import { api, getToken, type ForumThread, type ForumReply, type ForumPoll } from '$lib/api';
	import { isFeatureEnabled } from '$lib/stores/features.svelte';
	import { onMount } from 'svelte';
	import ReactionBar from '$lib/components/ReactionBar.svelte';
	import RevisionHistory from '$lib/components/RevisionHistory.svelte';
	import PinButton from '$lib/components/PinButton.svelte';

	let thread = $state<ForumThread | null>(null);
	let replies = $state<ForumReply[]>([]);
	let poll = $state<ForumPoll | null>(null);
	let loading = $state(true);
	let error = $state('');
	let replyBody = $state('');
	let replyAnonymous = $state(false);
	let submittingReply = $state(false);
	let replyError = $state('');
	let bookmarked = $state(false);
	let bookmarkLoading = $state(false);

	const slug = $derived(page.params.slug);

	async function loadThread() {
		if (!slug) return;
		loading = true;
		error = '';
		try {
			thread = await api.getForumThread(slug);
			if (thread?.poll_id) {
				try {
					poll = await api.getForumPoll(thread.poll_id);
				} catch {
					poll = null;
				}
			}
			await loadReplies();
		} catch (err) {
			error = err instanceof Error ? err.message : 'Failed to load thread.';
		} finally {
			loading = false;
		}
	}

	async function loadReplies() {
		if (!thread) return;
		try {
			const result = await api.getForumReplies(thread.id, { page_size: 50 });
			replies = result ?? [];
		} catch {
			replies = [];
		}
	}

	async function handleVote(entityType: 'thread' | 'reply', entityId: string, value: 1 | -1) {
		try {
			await api.voteForum({ entity_type: entityType, entity_id: entityId, value });
			if (entityType === 'thread' && thread) {
				thread = { ...thread, score: thread.score + value };
			} else {
				replies = replies.map((r) =>
					r.id === entityId ? { ...r, score: r.score + value } : r
				);
			}
		} catch {
			// ignore vote errors
		}
	}

	async function handleSolve() {
		if (!thread) return;
		try {
			await api.solveForumThread(thread.id);
			thread = { ...thread, solved: !thread.solved };
		} catch {
			// ignore
		}
	}

	async function handleSubmitReply(e: Event) {
		e.preventDefault();
		if (!thread || !replyBody.trim()) return;
		replyError = '';
		submittingReply = true;
		try {
			await api.createForumReply(thread.id, {
				body: replyBody.trim(),
				anonymous: replyAnonymous
			});
			replyBody = '';
			replyAnonymous = false;
			thread = { ...thread, reply_count: thread.reply_count + 1 };
			await loadReplies();
		} catch (err) {
			replyError = err instanceof Error ? err.message : 'Failed to post reply.';
		} finally {
			submittingReply = false;
		}
	}

	async function handleBookmark() {
		if (!thread) return;
		bookmarkLoading = true;
		try {
			if (bookmarked) {
				await api.removeForumBookmark(thread.id);
				bookmarked = false;
			} else {
				await api.addForumBookmark({ thread_id: thread.id });
				bookmarked = true;
			}
		} catch {
			// ignore
		} finally {
			bookmarkLoading = false;
		}
	}

	async function handlePollVote(optionIndex: number) {
		if (!poll) return;
		try {
			poll = await api.voteForumPoll(poll.id, { option_index: optionIndex });
		} catch {
			// ignore
		}
	}

	function timeAgo(dateStr: string): string {
		const diff = Date.now() - new Date(dateStr).getTime();
		const mins = Math.floor(diff / 60000);
		if (mins < 60) return `${mins}m ago`;
		const hours = Math.floor(mins / 60);
		if (hours < 24) return `${hours}h ago`;
		const days = Math.floor(hours / 24);
		return `${days}d ago`;
	}

	onMount(loadThread);
</script>

<svelte:head>
	<title>{thread?.title ?? 'Thread'} | Forum | JapaConnect</title>
</svelte:head>

{#if !isFeatureEnabled('feature.forum')}
	<main class="mx-auto max-w-2xl px-6 py-16">
		<div class="text-center">
			<h1 class="text-2xl font-bold" style="color: var(--color-foreground)">Forum is not available</h1>
			<p class="mt-2" style="color: var(--color-muted)">This feature is currently disabled.</p>
			<a href="/" class="mt-4 inline-block" style="color: var(--color-primary)">Go home</a>
		</div>
	</main>
{:else}
	<main class="mx-auto max-w-3xl px-4 py-8">
	<a
		href="/forum"
		class="mb-6 inline-flex items-center gap-1 text-sm transition-colors hover:underline"
		style="color: var(--color-muted)"
	>
		<ArrowLeft class="h-4 w-4" />
		Back to Forum
	</a>

	{#if loading}
		<div class="space-y-4">
			<div class="h-8 w-3/4 animate-pulse rounded-lg" style="background-color: var(--color-surface)"></div>
			<div class="h-32 animate-pulse rounded-xl" style="background-color: var(--color-surface)"></div>
		</div>
	{:else if error}
		<div
			class="rounded-lg border px-4 py-3 text-sm"
			style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 8%, transparent); color: var(--color-danger)"
		>
			{error}
		</div>
	{:else if thread}
		<!-- Thread -->
		<article class="mb-8">
			<div class="flex gap-4">
				<!-- Votes -->
				<div class="flex flex-col items-center gap-1 pt-1">
					<button
						onclick={() => handleVote('thread', thread!.id, 1)}
						class="rounded p-1 transition-colors hover:bg-black/10 dark:hover:bg-white/10"
						style="color: var(--color-muted)"
						aria-label="Upvote"
					>
						<ArrowBigUp class="h-5 w-5" />
					</button>
					<span class="text-sm font-semibold" style="color: var(--color-foreground)">
						{thread.score}
					</span>
					<button
						onclick={() => handleVote('thread', thread!.id, -1)}
						class="rounded p-1 transition-colors hover:bg-black/10 dark:hover:bg-white/10"
						style="color: var(--color-muted)"
						aria-label="Downvote"
					>
						<ArrowBigDown class="h-5 w-5" />
					</button>
				</div>

				<!-- Content -->
				<div class="flex-1 min-w-0">
					<div class="flex items-start justify-between gap-3">
						<h1 class="text-xl font-bold leading-snug" style="color: var(--color-foreground)">
							{thread.title}
						</h1>
						<div class="flex items-center gap-2 shrink-0">
							<PinButton contentId={thread.id} />
							{#if thread.solved}
								<span
									class="inline-flex items-center gap-1 rounded-full px-2.5 py-1 text-xs font-medium"
									style="background-color: color-mix(in srgb, var(--color-success, #16a34a) 12%, transparent); color: var(--color-success, #16a34a)"
								>
									<CheckCircle2 class="h-3.5 w-3.5" />
									Solved
								</span>
							{/if}
							<button
								onclick={handleBookmark}
								disabled={bookmarkLoading}
								class="rounded p-1.5 transition-colors hover:bg-black/10 dark:hover:bg-white/10"
								style="color: {bookmarked ? 'var(--color-primary)' : 'var(--color-muted)'}"
								aria-label={bookmarked ? 'Remove bookmark' : 'Bookmark thread'}
							>
								{#if bookmarkLoading}
									<Loader2 class="h-4 w-4 animate-spin" />
								{:else if bookmarked}
									<BookmarkCheck class="h-4 w-4" />
								{:else}
									<Bookmark class="h-4 w-4" />
								{/if}
							</button>
						</div>
					</div>

					<div class="mt-2 flex flex-wrap items-center gap-2 text-xs" style="color: var(--color-muted)">
						<span>{thread.author_name}</span>
						<span>&middot;</span>
						<span>{timeAgo(thread.created_at)}</span>
						{#if thread.space_name}
							<span>&middot;</span>
							<span class="rounded-full px-2 py-0.5" style="background-color: var(--color-background)">
								{thread.space_name}
							</span>
						{/if}
					</div>

					{#if thread.tags.length > 0}
						<div class="mt-3 flex flex-wrap gap-1.5">
							{#each thread.tags as tag}
								<span
									class="rounded-full px-2 py-0.5 text-[11px]"
									style="background-color: var(--color-background); color: var(--color-muted)"
								>
									{tag}
								</span>
							{/each}
						</div>
					{/if}

					<div class="prose prose-sm mt-4 max-w-none" style="color: var(--color-foreground)">
						{thread.body}
					</div>

					{#if !thread.solved}
						<button
							onclick={handleSolve}
							class="mt-4 inline-flex items-center gap-1.5 rounded-lg px-3 py-1.5 text-sm font-medium transition-colors"
							style="color: var(--color-success, #16a34a); background-color: color-mix(in srgb, var(--color-success, #16a34a) 8%, transparent)"
						>
							<CheckCircle2 class="h-4 w-4" />
							Mark as solved
						</button>
					{/if}
				</div>
			</div>
		</article>

		<!-- Poll -->
		{#if poll}
			<div
				class="mb-8 rounded-xl border p-5"
				style="border-color: var(--color-border); background-color: var(--color-surface)"
			>
				<div class="mb-3 flex items-center gap-2">
					<BarChart3 class="h-4 w-4" style="color: var(--color-primary)" />
					<h3 class="text-sm font-semibold" style="color: var(--color-foreground)">
						{poll.question}
					</h3>
				</div>
				<div class="space-y-2">
					{#each poll.options as option, i}
						{@const pct = poll.total_votes > 0 ? Math.round((option.votes / poll.total_votes) * 100) : 0}
						<button
							onclick={() => handlePollVote(i)}
							disabled={poll.user_vote !== undefined}
							class="relative w-full overflow-hidden rounded-lg border px-4 py-3 text-left text-sm transition-colors"
							style="border-color: {poll.user_vote === i ? 'var(--color-primary)' : 'var(--color-border)'}; background-color: {poll.user_vote === i ? 'color-mix(in srgb, var(--color-primary) 8%, transparent)' : 'var(--color-background)'}"
						>
							{#if poll.user_vote !== undefined}
								<div
									class="absolute inset-y-0 left-0 rounded-l-lg transition-all"
									style="width: {pct}%; background-color: color-mix(in srgb, var(--color-primary) 12%, transparent)"
								></div>
							{/if}
							<span class="relative flex items-center justify-between">
								<span style="color: var(--color-foreground)">{option.text}</span>
								{#if poll.user_vote !== undefined}
									<span class="font-medium" style="color: var(--color-muted)">{pct}%</span>
								{/if}
							</span>
						</button>
					{/each}
				</div>
				<p class="mt-3 text-xs" style="color: var(--color-muted)">
					{poll.total_votes} vote{poll.total_votes !== 1 ? 's' : ''}
				</p>
			</div>
		{/if}

		<!-- Replies -->
		<section>
			<h2 class="mb-4 flex items-center gap-2 text-lg font-semibold" style="color: var(--color-foreground)">
				<MessageSquare class="h-5 w-5" />
				{thread.reply_count} {thread.reply_count === 1 ? 'Reply' : 'Replies'}
			</h2>

			{#if replies.length === 0}
				<p class="mb-6 text-sm" style="color: var(--color-muted)">
					No replies yet. Be the first to respond.
				</p>
			{:else}
				<div class="mb-8 divide-y rounded-xl border" style="border-color: var(--color-border); divide-color: var(--color-border)">
					{#each replies as reply (reply.id)}
						<div class="flex gap-3 p-4">
							<!-- Vote -->
							<div class="flex flex-col items-center gap-0.5 pt-0.5">
								<button
									onclick={() => handleVote('reply', reply.id, 1)}
									class="rounded p-0.5 transition-colors hover:bg-black/10 dark:hover:bg-white/10"
									style="color: var(--color-muted)"
									aria-label="Upvote reply"
								>
									<ArrowBigUp class="h-4 w-4" />
								</button>
								<span class="text-xs font-semibold" style="color: var(--color-foreground)">
									{reply.score}
								</span>
								<button
									onclick={() => handleVote('reply', reply.id, -1)}
									class="rounded p-0.5 transition-colors hover:bg-black/10 dark:hover:bg-white/10"
									style="color: var(--color-muted)"
									aria-label="Downvote reply"
								>
									<ArrowBigDown class="h-4 w-4" />
								</button>
							</div>

							<!-- Reply content -->
							<div class="flex-1 min-w-0">
								<div class="flex items-center gap-2 text-xs" style="color: var(--color-muted)">
									<span class="font-medium" style="color: var(--color-foreground)">
										{reply.author_name}
									</span>
									<span>{timeAgo(reply.created_at)}</span>
								</div>
								<p class="mt-2 text-sm leading-relaxed" style="color: var(--color-foreground)">
									{reply.body}
								</p>
							</div>
						</div>
					{/each}
				</div>
			{/if}

			<!-- Reply form -->
			{#if replyError}
				<div
					class="mb-4 rounded-lg border px-4 py-3 text-sm"
					style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 8%, transparent); color: var(--color-danger)"
				>
					{replyError}
				</div>
			{/if}

			<form onsubmit={handleSubmitReply} class="space-y-3">
				<textarea
					placeholder="Write your reply..."
					bind:value={replyBody}
					rows="4"
					class="w-full resize-y rounded-lg border px-3 py-2.5 text-sm outline-none transition-colors focus:ring-2"
					style="border-color: var(--color-border); background-color: var(--color-surface); color: var(--color-foreground)"
				></textarea>
				<div class="flex items-center justify-between">
					<label class="flex cursor-pointer items-center gap-2 text-sm" style="color: var(--color-muted)">
						<input type="checkbox" bind:checked={replyAnonymous} class="h-4 w-4 rounded" />
						Reply anonymously
					</label>
					<button
						type="submit"
						disabled={submittingReply || !replyBody.trim()}
						class="flex items-center gap-1.5 rounded-lg px-4 py-2 text-sm font-medium text-white transition-opacity hover:opacity-90 disabled:opacity-50"
						style="background-color: var(--color-primary)"
					>
						{#if submittingReply}
							<Loader2 class="h-4 w-4 animate-spin" />
							Posting...
						{:else}
							Post Reply
						{/if}
					</button>
				</div>
			</form>
		</section>

		<ReactionBar contentId={thread.id} />
		<RevisionHistory contentId={thread.id} />
	{:else}
		<EmptyState
			icon={MessageSquare}
			title="Thread not found"
			message="This thread may have been deleted or the link is invalid."
		/>
	{/if}
	</main>
{/if}
