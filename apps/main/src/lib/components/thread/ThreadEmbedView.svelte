<script lang="ts">
	// /thread/{slug} universal thread view — renders EVERY non-forum thread
	// (anchor threads, §49 linked discussions, content threads) exactly like
	// the former forum page: body section at the top (avatar, author meta,
	// title, description/excerpt), the canonical-content embed card at the
	// bottom of the body before the reaction rail, then replies.
	// Ref: docs/superpowers/specs/2026-11-08-thread-rename-unified-discussions-design.md
	import { page } from '$app/state';
	import { untrack } from 'svelte';
	import { goto } from '$app/navigation';
	import { invalidateAll } from '$app/navigation';
	import { t, i18n } from '#lib/i18n/index.js';
	import { ArrowLeft, Bookmark, Eye, MessageSquare, MoreVertical, Pencil, Save, Share2, Trash2, X } from 'lucide-svelte';
	import SeoHead from '#lib/seo/SeoHead.svelte';
	import ThreadEmbedCard from '#lib/components/thread/ThreadEmbedCard.svelte';
	import ReplyThread from '#lib/components/forum/ReplyThread.svelte';
	import VoteRail from '#lib/components/forum/VoteRail.svelte';
	import UserHoverCard from '#lib/components/ui/UserHoverCard.svelte';
	import PostEditor from '#lib/components/create/editors/PostEditor.svelte';
	import { Menu, MenuItem, MenuSeparator } from '#lib/components/ui/index.js';
	import { api } from '#lib/api/client.js';
	import { getViewerState, updateViewerState } from '#lib/api/viewerState.js';
	import { commentsApi, commentsConversation, listAllComments } from '#lib/components/forum/conversationApi.js';
	import { tiptapBodyToHtml, tiptapToHtml, type TipTapDoc } from '#lib/tiptap/serialize.js';
	import { relativeTime } from '#lib/feed/time.js';
	import { hydrateLinkPreviews } from '#lib/tiptap/hydrateLinkPreviews.js';
	import { hydrateMentions } from '#lib/tiptap/hydrateMentions.js';
	import { hydrateVideoEmbeds } from '#lib/tiptap/hydrateVideoEmbeds.js';
	import type { ContentArticle } from '#lib/types/content.js';
	import type { Comment } from '#lib/types/index.js';

	export interface ThreadEmbed {
		kind: 'entity' | 'parent' | 'self';
		title: string;
		href: string;
		excerpt?: string | null;
		imageUrl?: string | null;
		badge?: string | null;
		entityType?: string;
	}

	interface Props {
		data: {
			article: ContentArticle;
			embed: ThreadEmbed | null;
			comments: Comment[];
			commentsTotal: number | null;
			commentsLoadError: boolean;
			authed: boolean;
			currentUserId: string | null;
			canModerate: boolean;
			canEdit: boolean;
			canDelete: boolean;
			deletedUserIds: string[] | null;
			jsonLd: unknown;
			indexable: boolean;
			seoIndexable: boolean;
		};
	}

	let { data }: Props = $props();

	const article = $derived(data.article);
	const canonical = $derived(`${page.url.origin}/thread/${article.slug}`);
	const description = $derived(article.excerpt ?? data.embed?.excerpt ?? '');
	const deletedUserIds = $derived(
		data.deletedUserIds ? new Set<string>(data.deletedUserIds) : undefined
	);
	// contents.body_html is the rendered field for article rows; contents.body
	// is the raw editor source and holds different formats by row — TipTap
	// JSON (composer/anchors), base64 TipTap (legacy), or plain text (older
	// rows). Prefer body_html, then decode in that order; a plain-text body
	// is paragraph-wrapped so it still renders formatted.
	const bodyHtml = $derived.by(() => {
		if (article.body_html) return article.body_html;
		if (!article.body) return '';
		const opts = {
			deletedUserIds,
			deletedUserLabel: t('mentions.deleted_user', '[deleted user]')
		};
		try {
			return tiptapToHtml(JSON.parse(article.body) as TipTapDoc, opts);
		} catch { /* not raw JSON — try base64/plain text */ }
		const decoded = tiptapBodyToHtml(article.body, opts);
		if (decoded) return decoded;
		return article.body
			.split(/\n{2,}/)
			.map((p) => `<p>${escapeHtml(p).replace(/\n/g, '<br>')}</p>`)
			.join('');
	});
	// Content threads (a contents row that IS its canonical article) render
	// only the opening ~80 words of the body + a "Read full {type}" CTA —
	// the full article lives on the canonical page, and truncating here also
	// keeps the thread from duplicating the article's content. Discussion
	// threads (forum-style bodies) render in full.
	const isContentThread = $derived(data.embed?.kind === 'self');
	// Content threads open with the first ~80 words of the article body —
	// FORMATTED (same .prose render as a full body), not a flattened text
	// blob. Whole block-level chunks are accumulated while the running word
	// count stays ≤100; a single oversized first block falls back to a
	// plain-text truncation so the preview never spills a wall of text.
	const contentPreviewHtml = $derived.by(() => {
		if (!isContentThread) return '';
		const source = article.body_html || bodyHtml;
		if (source) {
			const chunks = source.split(/(?=<(?:p|h[1-6]|ul|ol|blockquote|pre|figure|table|div)[\s>])/i);
			let acc = '';
			let words = 0;
			for (const chunk of chunks) {
				const w = stripHtmlText(chunk).split(/\s+/).filter(Boolean).length;
				if (w === 0) continue;
				if (words > 0 && words + w > 100) break;
				if (words === 0 && w > 100) break; // oversized first block → text fallback
				acc += chunk;
				words += w;
				if (words >= 60) break;
			}
			if (acc) return acc + '<p class="text-[var(--color-muted)]">…</p>';
		}
		// Fallback: plain-text truncation of whatever text exists.
		const text = stripHtmlText(source || article.body || '');
		const words = text.split(/\s+/).filter(Boolean);
		if (words.length === 0) return article.excerpt ? `<p>${escapeHtml(article.excerpt)}</p>` : '';
		const preview = words.length > 80 ? words.slice(0, 80).join(' ') + '…' : words.join(' ');
		return `<p>${escapeHtml(preview)}</p>`;
	});
	// Empty-body threads (minted anchors, linked discussions) fall back to the
	// embed excerpt so the body section still carries a description.
	const fallbackExcerpt = $derived(
		!isContentThread && !bodyHtml ? (data.embed?.excerpt ?? article.excerpt ?? '') : ''
	);

	/** Strip HTML tags/entities to plain text (body_html → excerpt source). */
	function stripHtmlText(html: string): string {
		return html
			.replace(/<[^>]*>/g, ' ')
			.replace(/&[a-z]+;/gi, ' ')
			.replace(/\s+/g, ' ')
			.trim();
	}

	/** Escape text for interpolation into the plain-text body fallback. */
	function escapeHtml(s: string): string {
		return s
			.replace(/&/g, '&amp;')
			.replace(/</g, '&lt;')
			.replace(/>/g, '&gt;')
			.replace(/"/g, '&quot;')
			.replace(/'/g, '&#39;');
	}

	const authorLabel = $derived(article.author_name ?? article.author_username ?? t('forum.member', 'Member'));
	const time = $derived(article.published_at ? relativeTime(article.published_at, new Date(), i18n.locale) : '');
	const typeBadge = $derived(t(`content_type.${article.content_type}`, article.content_type));

	// Unified conversation (thread-rename spec: ReplyThread renders every
	// discussion). The SSR seed is the top-level page only — a client pass
	// pulls descendants so the tree renders complete. invalidateAll() after
	// any post/edit/delete re-seeds and re-runs this effect.
	const convApi = $derived(commentsApi(article.id));
	let conversation = $state(
		untrack(() => commentsConversation(data.comments, data.article.id))
	);
	$effect(() => {
		const seeded = data.comments;
		conversation = commentsConversation(seeded, data.article.id);
		if (seeded.some((c) => (c.reply_count ?? 0) > 0) || (data.commentsTotal ?? 0) > seeded.length) {
			void listAllComments(data.article.id).then((all) => {
				conversation = commentsConversation(all, data.article.id);
			});
		}
	});
	const robots = $derived(
		data.indexable === false || data.seoIndexable === false ? 'noindex,nofollow' : ''
	);

	// Inline edit — backed threads are normal threads: the author (or a
	// content.edit_any / content.delete_any holder) edits the title + body
	// in place, same affordance as the formal forum page. PATCH /content/{id}
	// accepts body_html and regenerates the stored TipTap body server-side;
	// the backend enforces the actual permission (author or edit_any) plus
	// the content.edit_window_minutes window.
	let editing = $state(false);
	let editTitle = $state('');
	let editBodyHtml = $state('');
	let savingEdit = $state(false);
	let editError = $state<string | null>(null);
	let deleting = $state(false);
	let deleteError = $state<string | null>(null);

	function startEdit() {
		if (!data.canEdit) return;
		editTitle = article.title;
		// PostEditor initialises from HTML — the stored body_html is the
		// canonical source; fall back to the rendered TipTap body for rows
		// (minted anchors) whose body lives only in the `body` column.
		editBodyHtml = article.body_html || bodyHtml;
		editError = null;
		editing = true;
	}

	function cancelEdit() {
		editing = false;
		editError = null;
	}

	async function saveEdit() {
		if (savingEdit || !editTitle.trim()) return;
		savingEdit = true;
		editError = null;
		const res = await api.patch(`/content/${article.id}`, {
			title: editTitle.trim(),
			body_html: editBodyHtml
		});
		savingEdit = false;
		if (res.error) {
			editError =
				res.error.code === 'edit_window_expired'
					? t('forum.edit_window_expired', 'The edit window has expired.')
					: res.error.message || t('forum.edit_error', 'Failed to save edits.');
			return;
		}
		editing = false;
		await invalidateAll();
	}

	async function deleteThread() {
		if (deleting || !data.canDelete) return;
		if (!confirm(t('forum.delete_confirm', 'Delete this discussion? This cannot be undone.'))) return;
		deleting = true;
		deleteError = null;
		const res = await api.delete(`/content/${article.id}`);
		deleting = false;
		if (res.error) {
			deleteError = res.error.message || t('forum.delete_error', 'Failed to delete discussion.');
			return;
		}
		await goto('/thread');
	}

	// Bookmark + share — same affordances as the formal forum rail, backed
	// by the contents bookmark endpoints (/content/{id}/bookmark).
	let bookmarked = $state(false);
	let bookmarkCount = $state(untrack(() => data.article.bookmark_count ?? 0));
	let bookmarking = $state(false);
	let shared = $state(false);

	$effect(() => {
		if (!data.authed || !article.id) return;
		getViewerState(article.id).then((state) => {
			bookmarked = state.bookmarked;
		});
	});

	async function toggleBookmark() {
		if (bookmarking || !data.authed) return;
		bookmarking = true;
		const was = bookmarked;
		bookmarked = !was;
		bookmarkCount = Math.max(0, bookmarkCount + (was ? -1 : 1));
		const res = was
			? await api.delete(`/content/${article.id}/bookmark`)
			: await api.post(`/content/${article.id}/bookmark`, {});
		if (res.error) {
			bookmarked = was;
			bookmarkCount = Math.max(0, bookmarkCount + (was ? 1 : -1));
		} else {
			updateViewerState(article.id, { bookmarked: !was });
		}
		bookmarking = false;
	}

	async function shareThread() {
		const url = canonical;
		const shareData = { title: article.title, url };
		try {
			if (navigator.share) {
				await navigator.share(shareData);
				return;
			}
		} catch {
			// user dismissed the native sheet — fall through to clipboard.
		}
		await navigator.clipboard.writeText(url);
		shared = true;
		setTimeout(() => (shared = false), 2000);
	}
</script>

<SeoHead
	title={article.title}
	{description}
	ogTitle={article.title}
	{canonical}
	ogType="article"
	{robots}
	jsonLd={data.jsonLd}
/>

<div class="mx-auto w-full max-w-3xl px-4 py-6 pb-20 sm:pb-6">
	<a
		href="/thread"
		class="mb-5 inline-flex items-center gap-1.5 text-sm text-[var(--color-text-subtle)] transition-colors hover:text-[var(--color-text)]"
	>
		<ArrowLeft class="rtl:-scale-x-100" size={16} /> {t('thread.back', 'Back to threads')}
	</a>

	<article class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4 sm:p-6">
		<!-- Header — author avatar in the left gutter (same layout as the
		     former forum thread page). -->
		<div class="flex items-start gap-3">
			{#if article.author_username}
				<UserHoverCard
					id={article.author_id ?? ''}
					slug={article.author_username}
					name={authorLabel}
					avatar={article.author_avatar_url}
					variant="row"
					class="shrink-0"
				>
					{#snippet trigger()}
						{#if article.author_avatar_url}
							<img
								src={article.author_avatar_url}
								alt=""
								class="h-10 w-10 shrink-0 rounded-full object-cover"
								loading="lazy"
							/>
						{:else}
							<span class="grid h-10 w-10 shrink-0 place-items-center rounded-full bg-[var(--color-primary)] text-sm font-bold text-[var(--color-primary-contrast)]">
								{authorLabel.charAt(0).toUpperCase()}
							</span>
						{/if}
					{/snippet}
				</UserHoverCard>
			{:else}
				<span class="grid h-10 w-10 shrink-0 place-items-center rounded-full bg-[var(--color-primary)] text-sm font-bold text-[var(--color-primary-contrast)]">
					{authorLabel.charAt(0).toUpperCase()}
				</span>
			{/if}

			<div class="min-w-0 flex-1">
				<div class="flex flex-wrap items-center gap-x-2 gap-y-1 text-xs text-[var(--color-muted)]">
					{#if article.author_username}
						<UserHoverCard
							id={article.author_id ?? ''}
							slug={article.author_username}
							name={authorLabel}
							variant="row"
						>
							{#snippet trigger()}
								<span class="text-sm font-medium text-[var(--color-text-subtle)]">{authorLabel}</span>
							{/snippet}
						</UserHoverCard>
					{:else}
						<span class="text-sm font-medium text-[var(--color-text-subtle)]">{authorLabel}</span>
					{/if}
					{#if article.space_name}
						<span>·</span>
						<span class="text-[var(--color-text-subtle)]">
							{#if article.space_slug}<a href="/spaces/{article.space_slug}" class="hover:underline">{article.space_name}</a>{:else}{article.space_name}{/if}
						</span>
					{/if}
					{#if time}<span>·</span><span>{time}</span>{/if}
					{#if article.view_count != null}
						<span>·</span>
						<span class="flex items-center gap-1">
							<Eye size={12} /> {article.view_count} {t('content.views', 'views')}
						</span>
					{/if}

					<div class="ms-auto flex flex-wrap items-center gap-2">
						<span class="rounded border border-[var(--color-border)] bg-[var(--color-surface-subtle)] px-1.5 py-0.5 text-[10.5px] font-semibold uppercase tracking-wide text-[var(--color-text-subtle)]">
							{typeBadge}
						</span>
						<!-- Thread actions — edit/delete for the author or an
						     edit_any/delete_any holder, same as a normal thread. -->
						{#if !editing && (data.canEdit || data.canDelete)}
							<Menu side="bottom" contentClass="min-w-40">
								{#snippet trigger(props)}
									<button
										type="button"
										{...props}
										class="grid h-7 w-7 place-items-center rounded-full text-[var(--color-text-subtle)] transition-colors hover:bg-[var(--color-surface-subtle)] hover:text-[var(--color-text)]"
										aria-label={t('comments.options', 'More options')}
										title={t('comments.options', 'More options')}
									>
										<MoreVertical size={15} />
									</button>
								{/snippet}
								{#if data.canEdit}
									<MenuItem icon={Pencil} onSelect={startEdit}>{t('forum.edit', 'Edit')}</MenuItem>
								{/if}
								{#if data.canDelete}
									{#if data.canEdit}<MenuSeparator />{/if}
									<MenuItem icon={Trash2} onSelect={deleteThread} disabled={deleting} destructive>{t('forum.delete', 'Delete')}</MenuItem>
								{/if}
							</Menu>
						{/if}
					</div>
				</div>

				{#if editing}
					<input
						type="text"
						bind:value={editTitle}
						class="mt-2 w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-xl font-bold leading-tight tracking-tight text-[var(--color-text)] focus:border-[var(--color-primary)] focus:outline-none sm:text-2xl"
						aria-label={t('forum.new.title_placeholder', 'Title')}
					/>
				{:else}
					<h1 class="mt-2 text-balance text-2xl font-bold leading-tight tracking-tight text-[var(--color-text)] sm:text-3xl">
						{article.title}
					</h1>
				{/if}
			</div>
		</div>

		<!-- Body (view mode) / Edit form (edit mode) — same inline editor
		     the formal forum thread uses. -->
		{#if editing}
			<div class="mt-5 space-y-3">
				{#if editError}
					<p class="rounded-[var(--radius)] border border-[var(--color-primary)]/30 bg-[color-mix(in_srgb,var(--color-primary)_8%,transparent)] px-3 py-2 text-sm text-[var(--color-primary-text)]">
						{editError}
					</p>
				{/if}
				<div class="rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] focus-within:border-[var(--color-primary)]">
					<PostEditor
						html={editBodyHtml}
						onedit={() => {}}
						onhtml={(v) => { editBodyHtml = v; }}
						onjson={() => {}}
					/>
				</div>
				<div class="flex items-center justify-end gap-2 border-t border-[var(--color-border)] pt-3">
					<button
						type="button"
						onclick={cancelEdit}
						class="inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1.5 text-sm font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)]"
					>
						<X size={14} /> {t('common.cancel', 'Cancel')}
					</button>
					<button
						type="button"
						onclick={saveEdit}
						disabled={savingEdit || !editTitle.trim()}
						class="inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-3 py-1.5 text-sm font-medium text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)] disabled:opacity-50"
					>
						<Save size={14} /> {savingEdit ? t('common.saving', 'Saving…') : t('common.save', 'Save')}
					</button>
				</div>
			</div>
		{:else if isContentThread}
			{#if contentPreviewHtml}
				<div class="prose mt-5 max-w-none text-[var(--color-text)]" use:hydrateLinkPreviews use:hydrateMentions use:hydrateVideoEmbeds>
					{@html contentPreviewHtml}
				</div>
			{/if}
			{#if data.embed}
				<a
					href={data.embed.href}
					class="mt-4 inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-semibold text-[var(--color-primary-contrast)] transition-opacity hover:opacity-90"
				>
					{t('thread.read_full', 'Read full {type}').replace('{type}', typeBadge)}
				</a>
			{/if}
		{:else if bodyHtml}
			<div class="prose mt-5 max-w-none text-[var(--color-text)]" use:hydrateLinkPreviews use:hydrateMentions use:hydrateVideoEmbeds>
				{@html bodyHtml}
			</div>
		{:else if fallbackExcerpt}
			<p class="mt-5 text-[var(--color-text-subtle)]">{fallbackExcerpt}</p>
		{/if}

		<!-- Canonical content embed — the CTA back to the entity this thread
		     discusses. Sits at the bottom of the body section, before the
		     reaction rail (unified-discussion spec). -->
		{#if data.embed}
			<div class="mt-5">
				<ThreadEmbedCard
					title={data.embed.title}
					href={data.embed.href}
					excerpt={data.embed.excerpt ?? null}
					badge={data.embed.badge ?? null}
					imageUrl={data.embed.imageUrl ?? null}
					entityType={data.embed.entityType ?? ''}
				/>
			</div>
		{/if}

		<!-- Reaction rail — identical to the formal forum thread rail:
		     up/down vote, reply count, bookmark, share. Backed threads vote
		     through /content/{id}/vote; fixed bottom bar below sm. -->
		<div class="fixed inset-x-0 bottom-0 z-50 flex w-full flex-row items-center gap-2 border-t border-[var(--color-border)] bg-[var(--color-surface)]/95 px-3 py-2 pb-[calc(0.5rem+env(safe-area-inset-bottom))] text-[var(--color-muted)] backdrop-blur sm:relative sm:z-30 sm:mt-6 sm:rounded-[var(--radius-card)] sm:border sm:bg-[var(--color-surface)] sm:py-2 sm:pb-2 sm:backdrop-blur-none">
			<div class="flex flex-1 items-center justify-center">
				<VoteRail
					entityType="thread"
					entityId={article.id}
					authorId={article.author_id}
					score={article.vote_score ?? 0}
					layout="rail"
					authed={data.authed}
					backend="content"
				/>
			</div>
			<a
				href="#replies"
				class="flex flex-1 items-center justify-center gap-1.5 text-[var(--color-text-subtle)] transition-colors hover:text-[var(--color-text)]"
				aria-label={t('forum.replies', 'Replies')}
			>
				<span class="grid h-9 w-9 place-items-center rounded-full text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)] hover:text-[var(--color-primary-text)]">
					<MessageSquare size={18} />
				</span>
				<span class="text-sm font-medium text-[var(--color-text-subtle)] [font-variant-numeric:tabular-nums]">{article.comment_count ?? 0}</span>
			</a>
			<div class="h-5 w-px bg-[var(--color-border)]"></div>

			<button
				type="button"
				onclick={toggleBookmark}
				disabled={bookmarking}
				class="flex flex-1 items-center justify-center transition-colors disabled:cursor-not-allowed disabled:opacity-50"
				aria-pressed={bookmarked}
				title={data.authed ? (bookmarked ? t('forum.remove_bookmark', 'Remove bookmark') : t('forum.add_bookmark', 'Bookmark this discussion')) : t('forum.login_to_bookmark', 'Log in to bookmark')}
			>
				<span class="grid h-9 w-9 place-items-center rounded-full transition-colors {bookmarked ? 'bg-[var(--color-primary)]/10 text-[var(--color-primary-text)]' : 'text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)] hover:text-[var(--color-primary-text)]'}">
					<Bookmark size={18} fill={bookmarked ? 'currentColor' : 'none'} />
				</span>
				<span class="text-sm font-medium text-[var(--color-text-subtle)] [font-variant-numeric:tabular-nums]">{bookmarkCount > 0 ? bookmarkCount : ''}</span>
			</button>
			<button
				type="button"
				onclick={shareThread}
				class="flex flex-1 items-center justify-center text-[var(--color-text-subtle)] transition-colors hover:text-[var(--color-text)]"
				aria-label={t('forum.share', 'Share')}
			>
				<span class="grid h-9 w-9 place-items-center rounded-full text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)] hover:text-[var(--color-primary-text)]">
					<Share2 size={18} />
				</span>
				{#if shared}<span class="text-sm text-[var(--color-success)]">{t('forum.shared', 'Copied!')}</span>{/if}
			</button>
		</div>
		{#if deleteError}
			<p class="mt-2 text-sm text-[var(--color-error)]">{deleteError}</p>
		{/if}
	</article>

	<!-- Replies — the unified conversation (§49 comments via the §08 reply
	     UI). Discussion happens here, not on the entity page. -->
	<div id="replies" class="mt-6">
		{#if data.commentsLoadError}
			<p class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4 text-sm text-[var(--color-muted)]">
				{t('comments.load_error', 'Could not load comments right now.')}
			</p>
		{/if}
		<ReplyThread
			replies={conversation.replies}
			threadId={article.id}
			backend={convApi}
			authed={data.authed}
			canSolve={false}
			acceptedReplyId={null}
			userVotes={conversation.userVotes}
			canModerate={data.canModerate}
			currentUserId={data.currentUserId}
			{deletedUserIds}
		/>
	</div>
</div>
