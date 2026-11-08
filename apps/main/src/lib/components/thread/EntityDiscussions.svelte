<script lang="ts">
	// Entity discussion block — the 5-comment preview mounted at the bottom
	// of content singles, wiki pages and tool pages (thread-rename spec).
	// Discussion itself happens at /thread/{slug}; this block previews the
	// conversation (Popular | Latest tabs) and links in:
	//   - "Join this Conversation" when comments exist
	//   - "Start a conversation" when none do
	//
	// Resolution is uniform via GET /threads/anchor/{entityType}/{entityKey}:
	//   - 'content'   — the contents row IS its own thread; key = content UUID
	//   - 'tool'      — key = tool slug; thread minted on demand
	//   - 'wiki_page' — key = wiki_pages.id; thread minted on demand
	import { t, i18n } from '#lib/i18n/index.js';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { api } from '#lib/api/client.js';
	import { requireAuthOrRedirect } from '#lib/auth/guest.js';
	import { relativeTime } from '#lib/feed/time.js';
	import { tiptapExcerpt } from '#lib/tiptap/serialize.js';
	import { MessageSquare, Loader2 } from 'lucide-svelte';
	import type { Comment } from '#lib/types/index.js';

	interface Props {
		entityType: 'content' | 'tool' | 'wiki_page';
		entityKey: string;
		authed: boolean;
	}

	interface ThreadAnchor {
		content_id: string;
		slug: string;
		comment_count: number;
	}

	let { entityType, entityKey, authed }: Props = $props();

	let tab = $state<'popular' | 'latest'>('popular');
	let comments = $state<Comment[]>([]);
	let anchor = $state<ThreadAnchor | null>(null);
	let resolved = $state(false);
	let loading = $state(true);
	let starting = $state(false);
	let loadError = $state<string | null>(null);

	const sortParam = $derived(tab === 'popular' ? 'top' : 'new');
	const threadHref = $derived(anchor ? `/thread/${anchor.slug}` : '');
	const hasConversation = $derived(comments.length > 0 || (anchor?.comment_count ?? 0) > 0);

	async function loadComments() {
		if (!anchor?.content_id) return;
		const res = await api.get<Comment[]>(
			`/content/${anchor.content_id}/comments?sort=${sortParam}&page_size=5`
		);
		if (!res.error && res.data) comments = res.data;
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
			} else {
				loadError = res.error?.message ?? t('comments.load_error', 'Could not load discussions');
			}
		} finally {
			resolved = true;
			loading = false;
		}
	}

	async function changeTab(next: 'popular' | 'latest') {
		if (next === tab) return;
		tab = next;
		comments = [];
		await loadComments();
	}

	// "Start a conversation" — content threads already exist (no-op resolve);
	// tool/wiki anchors are minted on demand, then deep-linked into /thread.
	async function startConversation() {
		if (!(await requireAuthOrRedirect())) return;
		if (entityType === 'content') {
			await goto(threadHref);
			return;
		}
		if (anchor) {
			await goto(threadHref);
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
			await goto(`/thread/${res.data.slug}`);
		}
	}

	$effect(() => {
		load();
	});
</script>

{#if resolved}
	<section class="entity-discussions" aria-label={t('thread.discussions', 'Discussions')}>
		<div class="ed-head">
			<h2 class="ed-title">
				<MessageSquare size={18} />
				{t('thread.discussions', 'Discussions')}
			</h2>
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
		</div>

		{#if loading}
			<div class="ed-state"><Loader2 size={16} class="animate-spin" /> {t('common.loading', 'Loading')}</div>
		{:else if loadError}
			<div class="ed-state ed-error">{loadError}</div>
		{:else if comments.length > 0}
			<ul class="ed-list">
				{#each comments as comment (comment.id)}
					<li>
						<a href={threadHref} class="ed-comment">
							{#if comment.avatar_url && !comment.is_anonymous}
								<img src={comment.avatar_url} alt="" class="ed-avatar" loading="lazy" />
							{:else}
								<span class="ed-avatar ed-avatar-fallback">
									{(comment.display_name || comment.username || '?').charAt(0).toUpperCase()}
								</span>
							{/if}
							<span class="ed-comment-body">
								<span class="ed-comment-meta">
									<span class="ed-author">
										{comment.is_anonymous
											? t('forum.anonymous', 'Anonymous')
											: comment.display_name || comment.username}
									</span>
									<span class="ed-time">{relativeTime(comment.created_at, new Date(), i18n.locale)}</span>
								</span>
								<span class="ed-excerpt">{tiptapExcerpt(comment.body, 160)}</span>
								{#if comment.reply_count > 0}
									<span class="ed-replies">
										{t('thread.reply_count', '{count} replies').replace('{count}', String(comment.reply_count))}
									</span>
								{/if}
							</span>
						</a>
					</li>
				{/each}
			</ul>
		{:else}
			<p class="ed-state">{t('thread.no_discussions', 'No discussions yet.')}</p>
		{/if}

		<div class="ed-cta">
			{#if hasConversation}
				<a href={threadHref} class="ed-btn">
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

	.ed-list {
		list-style: none;
		margin: 0;
		padding: 0;
		display: flex;
		flex-direction: column;
	}

	.ed-comment {
		display: flex;
		gap: 0.625rem;
		padding: 0.625rem 0.5rem;
		border-radius: 0.5rem;
		text-decoration: none;
		color: inherit;
		transition: background 0.15s;
	}

	.ed-comment:hover {
		background: var(--color-surface-subtle, #f4f4f5);
	}

	.ed-avatar {
		width: 1.75rem;
		height: 1.75rem;
		border-radius: 9999px;
		object-fit: cover;
		flex-shrink: 0;
	}

	.ed-avatar-fallback {
		display: grid;
		place-items: center;
		background: var(--color-primary, #3b49df);
		color: var(--color-primary-contrast, #fff);
		font-size: 0.75rem;
		font-weight: 700;
	}

	.ed-comment-body {
		min-width: 0;
		display: flex;
		flex-direction: column;
		gap: 0.125rem;
	}

	.ed-comment-meta {
		display: flex;
		align-items: baseline;
		gap: 0.5rem;
	}

	.ed-author {
		font-size: 0.8125rem;
		font-weight: 600;
		color: var(--color-text, #18181b);
	}

	.ed-time {
		font-size: 0.75rem;
		color: var(--color-muted, #71717a);
	}

	.ed-excerpt {
		font-size: 0.8125rem;
		color: var(--color-text-subtle, #52525b);
		line-height: 1.45;
		display: -webkit-box;
		line-clamp: 2;
		-webkit-line-clamp: 2;
		-webkit-box-orient: vertical;
		overflow: hidden;
	}

	.ed-replies {
		font-size: 0.75rem;
		color: var(--color-muted, #71717a);
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
