<script lang="ts">
	/**
	 * Linked-discussion panel for content singles — Section 49 thread links.
	 * - No linked discussions: "Start A Conversation" CTA → POST
	 *   /content/{id}/discussions (derives a discussion thread + link).
	 * - Linked discussions exist: the linked thread(s) with comment counts
	 *   and a "Join this Conversation →" link to the thread page.
	 * Mounted inside CommentThread when `showDiscussions` is set — direct
	 * comments on the content itself remain the primary engine (documented
	 * divergence from §49's preview-only model; BUG-004).
	 */
	import { onMount } from 'svelte';
	import { MessageSquare, ArrowRight, ChevronDown } from 'lucide-svelte';
	import { api } from '#lib/api/client.js';
	import { t } from '#lib/i18n/index.js';
	import { tiptapExcerpt } from '#lib/tiptap/serialize.js';
	import type { Comment } from '#lib/types/index.js';

	interface LinkedDiscussion {
		id: string;
		thread_id: string;
		thread_title: string;
		thread_slug: string;
		thread_comment_count: number;
		thread_author_id: string;
		link_count: number;
		created_at: string;
	}

	interface Props {
		contentId: string;
		authed: boolean;
	}
	let { contentId, authed }: Props = $props();

	let discussions = $state<LinkedDiscussion[]>([]);
	let total = $state(0);
	let loaded = $state(false);
	let starting = $state(false);
	let error = $state<string | null>(null);
	let expanded = $state(false);
	let preview = $state<Comment[]>([]);

	async function fetchDiscussions() {
		// Page the panel generously so "View all" can expand inline — most
		// content has a handful of linked threads at most.
		const res = await api.getEnvelope<LinkedDiscussion[]>(`/content/${contentId}/discussions?page_size=50`);
		loaded = true;
		if (res.error || !res.data) return;
		discussions = res.data;
		const meta = (res.envelope as Record<string, unknown> | null)?.meta as { total?: number } | undefined;
		total = meta?.total ?? res.data.length;
	}

	onMount(fetchDiscussions);

	// §49 — "Join this Conversation" targets the primary (most active) linked
	// thread, not just the most recently linked one.
	const primary = $derived(
		discussions.length === 0
			? null
			: discussions.reduce((a, b) => (b.thread_comment_count > a.thread_comment_count ? b : a))
	);

	const visibleDiscussions = $derived(expanded ? discussions : discussions.slice(0, 5));

	// §49 — with linked discussion(s), show the 5 most recent top-level
	// comments from the primary thread as a preview before the Join CTA.
	$effect(() => {
		const p = primary;
		if (!p) return;
		api.getEnvelope<Comment[]>(`/content/${p.thread_id}/comments?page_size=5`).then((res) => {
			if (!res.error && res.data) preview = res.data;
		});
	});

	async function startConversation() {
		if (starting || !authed) return;
		starting = true;
		error = null;
		const res = await api.post(`/content/${contentId}/discussions`, {});
		starting = false;
		if (res.error) {
			error = res.error.message;
			return;
		}
		// Refetch to resolve the new link's thread slug/title for the join link.
		await fetchDiscussions();
	}
</script>

{#if loaded && (discussions.length > 0 || authed)}
	<div class="mb-6 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
		{#if discussions.length === 0}
			{#if authed}
				<div class="flex flex-wrap items-center justify-between gap-3">
					<p class="text-sm text-[var(--color-muted)]">{t('comments.no_discussions', 'No forum discussions are linked to this yet.')}</p>
					<button
						type="button"
						onclick={startConversation}
						disabled={starting}
						class="inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-3.5 py-1.5 text-xs font-medium text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)] disabled:opacity-50"
					>
						<MessageSquare size={14} />
						{starting ? t('common.loading') : t('comments.start_conversation', 'Start A Conversation')}
					</button>
				</div>
				{#if error}
					<p class="mt-2 text-xs text-[var(--color-danger)]">{error}</p>
				{/if}
			{/if}
		{:else}
			<h3 class="mb-2 flex items-center gap-1.5 text-sm font-semibold text-[var(--color-text)]">
				<MessageSquare size={15} />
				{t('comments.linked_discussions', 'Linked discussions')}
			</h3>
			<ul class="space-y-1.5">
				{#each visibleDiscussions as disc (disc.id)}
					<li>
						<a href="/thread/{disc.thread_slug}" class="group flex items-baseline justify-between gap-2 text-sm">
							<span class="truncate text-[var(--color-text)] group-hover:text-[var(--color-primary-text)] group-hover:underline">{disc.thread_title}</span>
							<span class="shrink-0 text-xs text-[var(--color-muted)]">
								{t('comments.discussion_replies', '{count} replies').replace('{count}', String(disc.thread_comment_count))}
							</span>
						</a>
					</li>
				{/each}
			</ul>
			{#if preview.length > 0}
				<ul class="mt-3 space-y-2 border-t border-[var(--color-border)] pt-3">
					{#each preview as c (c.id)}
						<li class="flex gap-2 text-sm">
							{#if c.avatar_url && !c.is_anonymous}
								<img src={c.avatar_url} alt="" class="h-6 w-6 shrink-0 rounded-full object-cover" loading="lazy" />
							{:else}
								<span class="grid h-6 w-6 shrink-0 place-items-center rounded-full bg-[var(--color-primary)]/10 text-[0.65rem] font-bold text-[var(--color-primary-text)]">
									{(c.is_anonymous ? 'A' : (c.display_name || c.username || '?')).charAt(0).toUpperCase()}
								</span>
							{/if}
							<div class="min-w-0">
								<span class="text-xs font-semibold text-[var(--color-text)]">
									{c.is_anonymous ? t('comments.anonymous', 'Anonymous') : c.display_name || c.username || t('forum.member', 'Member')}
								</span>
								<p class="truncate text-[var(--color-muted)]">{tiptapExcerpt(c.body, 140)}</p>
							</div>
						</li>
					{/each}
				</ul>
			{/if}
			{#if primary}
				<a href="/thread/{primary.thread_slug}" class="mt-3 inline-flex items-center gap-1 text-sm font-medium text-[var(--color-primary-text)] transition-colors hover:text-[var(--color-primary-hover)]">
					{t('comments.join_conversation', 'Join this Conversation')}
					<ArrowRight size={14} class="rtl:-scale-x-100" />
				</a>
			{/if}
			{#if discussions.length > 5 && !expanded}
				<button
					type="button"
					onclick={() => (expanded = true)}
					class="ms-3 inline-flex items-center gap-1 text-xs font-medium text-[var(--color-primary-text)] transition-colors hover:text-[var(--color-primary-hover)]"
				>
					{t('comments.view_all_discussions', 'View all {count} discussions').replace('{count}', String(total))}
					<ChevronDown size={12} />
				</button>
			{/if}
		{/if}
	</div>
{/if}
