<script lang="ts">
	// Entity discussion block — the 5-conversation preview mounted at the
	// bottom of content singles, wiki pages and tool pages (thread-rename
	// spec). Renders the SAME ReplyThread/ReplyItem components as
	// /thread/{slug} — identical format, votes, inline reply/edit/delete —
	// limited to the first 5 top-level comments (with their children);
	// /thread/{slug} carries the complete conversation.
	//
	// Resolution is uniform via GET /threads/anchor/{entityType}/{entityKey}:
	//   - 'content'   — the contents row IS its own thread; key = content UUID
	//   - 'tool'      — key = tool slug; thread minted on demand
	//   - 'wiki_page' — key = wiki_pages.id; thread minted on demand
	import { t } from '#lib/i18n/index.js';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { api } from '#lib/api/client.js';
	import { requireAuthOrRedirect } from '#lib/auth/guest.js';
	import { MessageSquare, Loader2 } from 'lucide-svelte';
	import ReplyThread from '#lib/components/forum/ReplyThread.svelte';
	import { commentsApi, commentsConversation } from '#lib/components/forum/conversationApi.js';
	import type { Comment } from '#lib/types/index.js';

	interface Props {
		entityType: 'content' | 'tool' | 'wiki_page';
		entityKey: string;
		authed: boolean;
		// Canonical thread URL slug — content entities link to
		// /thread/{article-slug} (the canonical discussion URL), not the
		// minted discussion row's own slug. Tools/wiki use the anchor slug.
		contentSlug?: string;
	}

	interface ThreadAnchor {
		content_id: string;
		slug: string;
		comment_count: number;
	}

	let { entityType, entityKey, authed, contentSlug }: Props = $props();

	let tab = $state<'popular' | 'latest'>('popular');
	let comments = $state<Comment[]>([]);
	let total = $state(0);
	let anchor = $state<ThreadAnchor | null>(null);
	let resolved = $state(false);
	let loading = $state(true);
	let tabLoading = $state(false);
	let starting = $state(false);
	let loadError = $state<string | null>(null);

	const sortParam = $derived(tab === 'popular' ? 'top' : 'new');
	const threadHref = $derived(
		contentSlug ? `/thread/${contentSlug}` : anchor ? `/thread/${anchor.slug}` : ''
	);
	const hasConversation = $derived(comments.length > 0 || total > 0 || (anchor?.comment_count ?? 0) > 0);
	const currentUserId = $derived((page.data.user as { id?: string } | null)?.id ?? null);
	const convApi = $derived(anchor?.content_id ? commentsApi(anchor.content_id) : null);
	const conversation = $derived(
		anchor?.content_id ? commentsConversation(comments, anchor.content_id) : { replies: [], userVotes: {} }
	);

	// Fetch the first 5 top-level comments plus every descendant of those
	// five (parent_id filter, same walk as listAllComments but scoped to
	// the preview set — depth is capped at 3 server-side).
	async function loadComments() {
		if (!anchor?.content_id) return;
		const res = await api.getEnvelope<Comment[]>(
			`/content/${anchor.content_id}/comments?sort=${sortParam}&page_size=5`
		);
		if (res.error || !res.data) {
			// Clear rather than leave the previous tab's order rendered under
			// the new label — an honest empty state beats wrong-sort data.
			comments = [];
			return;
		}
		const top = res.data;
		total = (res.envelope as { meta?: { total?: number } } | null)?.meta?.total ?? top.length;
		const all = [...top];
		let parents = top.filter((c) => (c.reply_count ?? 0) > 0);
		while (parents.length > 0) {
			const next: Comment[] = [];
			for (const p of parents) {
				const child = await api.getEnvelope<Comment[]>(
					`/content/${anchor.content_id}/comments?parent_id=${p.id}&page_size=100`
				);
				if (!child.error && child.data) next.push(...child.data);
			}
			all.push(...next);
			parents = next.filter((c) => (c.reply_count ?? 0) > 0);
		}
		comments = all;
	}

	async function load() {
		loading = true;
		loadError = null;
		try {
			const res = await api.get<ThreadAnchor>(`/threads/anchor/${entityType}/${entityKey}`);
			if (!res.error && res.data) {
				anchor = res.data;
				await loadComments();
			} else if (res.error?.status === 404) {
				anchor = null;
				comments = [];
				total = 0;
			} else {
				loadError = res.error?.message ?? t('comments.load_error', 'Could not load discussions');
			}
		} finally {
			resolved = true;
			loading = false;
		}
	}

	async function changeTab(next: 'popular' | 'latest') {
		if (next === tab || tabLoading) return;
		tab = next;
		tabLoading = true;
		await loadComments();
		tabLoading = false;
	}

	// "Start a conversation" — deep-links into the canonical thread when one
	// exists; otherwise mints the anchor (a real discussion row for content,
	// tool and wiki entities alike) then lands on its comment box.
	async function startConversation() {
		if (!(await requireAuthOrRedirect())) return;
		if (anchor) {
			await goto(`${threadHref}#replies`);
			return;
		}
		starting = true;
		const res = await api.post<ThreadAnchor>('/threads/anchor', {
			entity_type: entityType,
			entity_key: entityKey
		});
		starting = false;
		if (!res.error && res.data) {
			anchor = res.data;
			await goto(`${threadHref}#replies`);
		}
	}

	$effect(() => {
		load();
	});
</script>

{#if resolved}
	<!-- id="comments" — the ReactionRail comment icon (href="#comments")
	     scrolls here, same affordance as #replies on /thread/{slug}. -->
	<section id="comments" class="entity-discussions scroll-mt-24" aria-label={t('thread.discussions', 'Discussions')}>
		<div class="ed-head">
			<h2 class="ed-title">
				<MessageSquare size={18} />
				{t('thread.discussions', 'Discussions')}
			</h2>
			<!-- Tabs key off the conversation existing (total/comment_count),
			     not the loaded page — they stay put during a sort refetch. -->
			{#if hasConversation}
				<div class="ed-tabs" role="tablist">
					<button
						type="button"
						role="tab"
						aria-selected={tab === 'popular'}
						class="ed-tab"
						class:ed-tab-active={tab === 'popular'}
						onclick={() => changeTab('popular')}
					>
						{t('thread.tab.popular', 'Popular')}
					</button>
					<button
						type="button"
						role="tab"
						aria-selected={tab === 'latest'}
						class="ed-tab"
						class:ed-tab-active={tab === 'latest'}
						onclick={() => changeTab('latest')}
					>
						{t('thread.tab.latest', 'Latest')}
					</button>
				</div>
			{/if}
		</div>

		{#if loading || tabLoading}
			<div class="ed-state"><Loader2 size={16} class="animate-spin" /> {t('common.loading', 'Loading')}</div>
		{:else if loadError}
			<div class="ed-state ed-error">{loadError}</div>
		{:else if comments.length > 0 && convApi && anchor}
			<!-- Same ReplyThread as /thread/{slug} — identical format and
			     interaction (vote, reply, edit, pin); only the top-level set is
			     capped at 5 here. Replies refresh via the component's own
			     invalidateAll after mutations. -->
			<div class="ed-thread">
				<!-- sort matches the tab's fetch order — ReplyThread would
				     otherwise re-rank top-level by votes and Latest would look
				     identical to Popular. -->
				<ReplyThread
					replies={conversation.replies}
					threadId={anchor.content_id}
					backend={convApi}
					preview
					sort={tab === 'popular' ? 'top' : 'newest'}
					{authed}
					canSolve={false}
					acceptedReplyId={null}
					userVotes={conversation.userVotes}
					canModerate={false}
					{currentUserId}
					deletedUserIds={undefined}
				/>
			</div>
			{#if total > comments.filter((c) => !c.parent_id).length || total > 5}
				<a href="{threadHref}#replies" class="ed-more">
					{t('thread.view_all', 'View the full conversation →')}
				</a>
			{/if}
		{:else}
			<p class="ed-state">{t('thread.no_discussions', 'No discussions yet.')}</p>
		{/if}

		<div class="ed-cta">
			{#if hasConversation}
				<a href="{threadHref}#replies" class="ed-btn">
					{t('thread.join', 'Join this Conversation')}
				</a>
			{:else if authed}
				<button type="button" class="ed-btn" onclick={startConversation} disabled={starting}>
					{#if starting}<Loader2 size={14} class="animate-spin" />{/if}
					{t('thread.start', 'Start a conversation')}
				</button>
			{:else}
				<a href="/login?redirect={encodeURIComponent(page.url.pathname)}" class="ed-btn">
					{t('thread.start', 'Start a conversation')}
				</a>
			{/if}
		</div>
	</section>
{/if}

<style>
	.entity-discussions {
		margin-top: 2rem;
		border-top: 1px solid var(--color-border, #e4e4e7);
		padding-top: 1.25rem;
	}

	.ed-head {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 0.75rem;
		margin-bottom: 0.75rem;
	}

	.ed-title {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		font-size: 1rem;
		font-weight: 600;
		color: var(--color-text);
		margin: 0;
	}

	.ed-tabs {
		display: flex;
		gap: 0.25rem;
		border: 1px solid var(--color-border, #e4e4e7);
		border-radius: 9999px;
		padding: 0.125rem;
	}

	.ed-tab {
		border: 0;
		background: transparent;
		padding: 0.25rem 0.75rem;
		border-radius: 9999px;
		font-size: 0.75rem;
		font-weight: 500;
		color: var(--color-muted, #71717a);
		cursor: pointer;
		transition:
			background 0.15s,
			color 0.15s;
	}

	.ed-tab-active {
		background: var(--color-surface-subtle, #f4f4f5);
		color: var(--color-text, #18181b);
	}

	.ed-state {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		padding: 0.75rem 0;
		font-size: 0.8125rem;
		color: var(--color-muted, #71717a);
	}

	.ed-error {
		color: var(--color-danger, #dc2626);
	}

	.ed-more {
		display: inline-block;
		margin-top: 0.5rem;
		font-size: 0.8125rem;
		font-weight: 500;
		color: var(--color-primary, #3b49df);
		text-decoration: none;
	}

	.ed-more:hover {
		text-decoration: underline;
	}

	.ed-cta {
		margin-top: 0.75rem;
	}

	.ed-btn {
		display: inline-flex;
		align-items: center;
		gap: 0.375rem;
		border: 0;
		border-radius: var(--radius, 0.5rem);
		background: var(--color-primary, #3b49df);
		color: var(--color-primary-contrast, #fff);
		padding: 0.5rem 1rem;
		font-size: 0.875rem;
		font-weight: 600;
		text-decoration: none;
		cursor: pointer;
		transition: opacity 0.15s;
	}

	.ed-btn:hover {
		opacity: 0.9;
	}

	.ed-btn:disabled {
		opacity: 0.6;
		cursor: default;
	}
</style>
