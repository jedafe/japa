<script lang="ts">
	import { untrack } from 'svelte';
	import { navigating, page } from '$app/state';
	import { MessageSquarePlus, TriangleAlert } from 'lucide-svelte';
	import { api } from '$lib/api/client';
	import { EmptyState, LoadingSkeleton } from '$lib/components/ui';
	import { t } from '$lib/i18n';
	import { compose } from '$lib/components/shell/compose.svelte';
	import type { ForumThread, FeedItem } from '$lib/types';
	import FeedCard from '$lib/components/feed/FeedCard.svelte';
	import { cardStyleFor, type CardStyleConfig } from '$lib/feed/cardStyle';

	interface Props {
		threads: ForumThread[];
		pageSize: number;
		loadError: boolean;
		config?: CardStyleConfig;
	}
	let { threads: initial, pageSize, loadError, config }: Props = $props();

	let threads = $state<ForumThread[]>(untrack(() => initial));
	let atEnd = $state(false);
	let loading = $state(false);
	let sentinel = $state<HTMLElement | null>(null);

	// Re-seed on a new SSR payload — replace, never append.
	$effect(() => {
		const seed = initial;
		untrack(() => {
			threads = seed;
			atEnd = seed.length < pageSize;
			loading = false;
		});
	});

	// Keyset pagination: for `latest` the cursor is (created_at, id); for `top`
	// it is (vote_score, created_at, id). Ref: Section 08.
	function cursor(): { createdAt: string; id: string; voteScore: number } | null {
		const last = threads[threads.length - 1];
		if (!last) return null;
		return { createdAt: last.created_at, id: last.id, voteScore: last.vote_score };
	}

	$effect(() => {
		const el = sentinel;
		if (!el || atEnd) return;
		const obs = new IntersectionObserver(
			(entries) => {
				if (entries[0]?.isIntersecting) loadMore();
			},
			{ rootMargin: '600px' }
		);
		obs.observe(el);
		return () => obs.disconnect();
	});

	async function loadMore() {
		if (loading || atEnd) return;
		const c = cursor();
		if (!c) return;
		loading = true;

		const params = new URLSearchParams(page.url.searchParams);
		params.set('page_size', String(pageSize));
		params.set('cursor_created_at', c.createdAt);
		params.set('cursor_id', c.id);

		const sort = params.get('sort') ?? 'latest';
		if (sort === 'top') {
			params.set('cursor_vote_score', String(c.voteScore));
		}

		// Map UI `status` filter to backend `solved` query param.
		const status = params.get('status');
		if (status && status !== 'all') {
			params.delete('status');
			params.set('solved', status === 'solved' ? 'true' : 'false');
		}

		const path = `/forum?${params.toString()}`;
		const res = await api.get<ForumThread[]>(path);
		if (res.error) {
			loading = false;
			return;
		}
		const batch = res.data ?? [];
		threads = [...threads, ...batch];
		atEnd = batch.length < pageSize;
		loading = false;
	}

	// M1 — Map ForumThread → FeedItem so forum threads render through FeedCard
	// (ADR-058 single FeedCard rule). The mapping is lossless: all forum-specific
	// fields (solved, is_anonymous) are preserved via the title/tags.
	function toFeedItem(thread: ForumThread): FeedItem {
		return {
			id: thread.id,
			title: thread.title,
			slug: thread.slug,
			content_type: 'forum_thread',
			author_id: thread.author_user_id,
			author_name: thread.is_anonymous
				? t('forum.anonymous', 'Anonymous')
				: thread.author_display_name || thread.author_username || t('forum.member', 'Member'),
			author_username: thread.author_username,
			author_avatar_url: thread.author_avatar_url,
			space_id: thread.space_id,
			space_name: thread.space_name,
			space_slug: thread.space_slug,
			excerpt: undefined,
			created_at: thread.created_at,
			view_count: thread.view_count,
			comment_count: thread.reply_count,
			vote_score: thread.vote_score,
			bookmark_count: thread.bookmark_count,
			tags: thread.solved ? [t('forum.solved', 'Solved')] : []
		};
	}

	// Card style config — use the global config from the layout, falling back
	// to a sensible default (compact for forum threads) when not provided.
	const resolvedConfig = $derived<CardStyleConfig>(
		config ?? { default: 'reddit', byType: {} }
	);
</script>

{#if loadError}
	<EmptyState icon={TriangleAlert} title={t('error.500_title')} body={t('error.500_body')} />
{:else if threads.length === 0}
	<EmptyState
		icon={MessageSquarePlus}
		title={t('forum.empty_title', 'No discussions yet')}
		body={t('forum.empty_body', 'Be the first to start a discussion.')}
		action={
			page.data.user
				? { label: t('forum.start_discussion', 'Start a discussion'), onclick: () => compose.openDiscussion() }
				: { label: t('forum.start_discussion', 'Start a discussion'), href: '/login?redirect=' + encodeURIComponent('/forum') }
		}
	/>
{:else}
	{#if navigating.to}
		<LoadingSkeleton variant="card" count={3} class="p-2" />
	{/if}

	<div
		class="divide-y divide-[var(--color-border)] overflow-hidden rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)]"
	>
		{#each threads as thread (thread.id)}
			<FeedCard
				item={toFeedItem(thread)}
				style={cardStyleFor('forum_thread', resolvedConfig)}
				layout="stream"
			/>
		{/each}
	</div>

	{#if !atEnd}
		<div bind:this={sentinel} class="h-8" aria-hidden="true"></div>
		{#if loading}<LoadingSkeleton variant="lines" count={2} class="p-4" />{/if}
	{:else}
		<p class="py-6 text-center text-xs text-[var(--color-muted)]">{t('feed.end', "You're all caught up")}</p>
	{/if}
{/if}
