<script module lang="ts">
	// Forum threads bookmark through the forum bookmark store
	// (entity_type='forum_thread') — the same store the thread detail page
	// and /dashboard/bookmarks consume. /me/content-states is content-scoped
	// so it never reflects them, and a 'content' bookmark on the backing
	// contents row would render as a /blog link on the dashboard and not
	// update thread.bookmark_count (§08). Shared across all card instances:
	// one GET /me/bookmarks serves every forum card on the page.
	let forumBookmarkMap: Map<string, string> | null = null;
	let forumBookmarkPromise: Promise<Map<string, string>> | null = null;

	async function loadForumBookmarks(): Promise<Map<string, string>> {
		if (forumBookmarkMap) return forumBookmarkMap;
		if (!forumBookmarkPromise) {
			forumBookmarkPromise = api
				.get<{ id: string; entity_type: string; entity_id: string }[]>('/me/bookmarks?page_size=500')
				.then((res) => {
					const m = new Map<string, string>();
					for (const b of res.data ?? []) {
						if (b.entity_type === 'forum_thread') m.set(b.entity_id, b.id);
					}
					forumBookmarkMap = m;
					return m;
				})
				.catch(() => {
					forumBookmarkPromise = null;
					return new Map<string, string>();
				});
		}
		return forumBookmarkPromise;
	}

	function invalidateForumBookmarks() {
		forumBookmarkMap = null;
		forumBookmarkPromise = null;
	}
</script>

<script lang="ts">
	/**
	 * Daily.dev-inspired feed card.
	 * Ref: japaconnect-engineering/sections/23-main-site-frontend.md
	 *      docs/superpowers/specs/2026-09-10-daily-feed-style-and-social-post-page-design.md
	 */
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { Bookmark, Flag, MessageSquare, MoreVertical, Play } from 'lucide-svelte';
	import { Menu, MenuItem } from '$lib/components/ui';
	import { t, i18n } from '$lib/i18n';
	import { relativeTime } from '$lib/feed/time';
	import { contentTypeMeta, readPostLabel } from '$lib/feed/contentTypeMeta';
	import { contentHref } from '$lib/utils/contentLink';
	import { formatDuration } from '$lib/utils/duration';
	import { api } from '$lib/api/client';
	import { getViewerState, updateViewerState } from '$lib/api/viewerState';
	import EntityHoverChip from '$lib/components/ui/EntityHoverChip.svelte';
	import ReactionButton from '$lib/components/ui/ReactionButton.svelte';
	import VoteRail from '$lib/components/forum/VoteRail.svelte';
	import ShareButton from '$lib/components/ui/ShareButton.svelte';
	import type { FeedItem } from '$lib/types';

	interface Props {
		item: FeedItem;
	}

	let { item }: Props = $props();

	let reported = $state(false);
	let bookmarked = $state(false);
	let bookmarkCount = $state(0);
	let bookmarkPending = $state(false);
	let bookmarkInitialized = $state(false);

	const authed = $derived(page.data.isAuthed as boolean);

	const time = $derived(relativeTime(item.created_at, new Date(), i18n.locale));

	$effect(() => {
		bookmarkCount = item.bookmark_count ?? 0;
		bookmarked = false;
		bookmarkInitialized = false;
	});

	$effect(() => {
		if (bookmarkInitialized || !authed) return;
		bookmarkInitialized = true;
		// Batched: one GET /me/content-states serves every card on the page.
		// Forum cards resolve against the forum bookmark store instead.
		if (isForum && item.forum_thread_id) {
			loadForumBookmarks().then((m) => {
				bookmarked = m.has(item.forum_thread_id ?? '');
			});
			return;
		}
		getViewerState(item.id).then((state) => {
			bookmarked = state.bookmarked;
		});
	});
	const duration = $derived(formatDuration(item.duration_secs));
	const isVideo = $derived(item.content_type === 'video' || !!item.video_source_id);
	// Forum threads use up/down votes (Section 08) instead of emoji reactions —
	// same card chrome, but the engagement bar shows a functional VoteRail in
	// place of the love button.
	const isForum = $derived(item.content_type === 'forum_thread' || item.content_type === 'discussion');
	const canonicalHref = $derived(contentHref(item));
	const actionLabel = $derived(readPostLabel(item.content_type));
	const meta = $derived(contentTypeMeta(item.content_type));
	const Icon = $derived(meta.icon);

	const sourceEntity = $derived({
		type: 'user' as const,
		id: item.author_id,
		slug: item.author_username ?? '',
		name: item.author_name ?? t('feed.anonymous', 'Unknown'),
		avatar: item.author_avatar_url
	});

	const regionNames = $derived(
		(() => {
			try {
				return new Intl.DisplayNames([i18n.locale], { type: 'region' });
			} catch {
				return null;
			}
		})()
	);
	function countryName(code?: string | null) {
		if (!code) return '';
		const upper = code.toUpperCase();
		return regionNames?.of(upper) ?? upper;
	}
	const route = $derived(
		item.source_country && item.target_country
			? `${countryName(item.source_country)} → ${countryName(item.target_country)}`
			: item.target_country
				? countryName(item.target_country)
				: item.source_country
					? countryName(item.source_country)
					: null
	);

	const secondary = $derived(
		item.organization
			? `${item.organization.name} • ${time}`
			: route
				? `${route} • ${time}`
				: item.space_name
					? `${item.space_name} • ${time}`
					: item.reading_time_minutes
						? `${item.reading_time_minutes} ${t('feed.min_read', 'min read')} • ${time}`
						: time
	);

	// The API can return the same tag name more than once (tag attached via
	// multiple relations) — dedupe so the keyed each block and the +N counter
	// stay consistent.
	const uniqueTags = $derived(item.tags ? [...new Set(item.tags)] : []);
	const visibleTags = $derived(uniqueTags.slice(0, 3));
	const extraTagCount = $derived(Math.max(uniqueTags.length - visibleTags.length, 0));

	function navigate(e: MouseEvent) {
		const target = e.target as HTMLElement | null;
		if (target && (target.closest('a') || target.closest('button'))) return;
		goto(canonicalHref);
	}

	function onKeyDown(e: KeyboardEvent) {
		const target = e.target as HTMLElement | null;
		if (target && (target.closest('a') || target.closest('button'))) return;
		if (e.key === 'Enter') goto(canonicalHref);
	}

	function openPost(e: Event) {
		e.stopPropagation();
		goto(canonicalHref);
	}

	const shareUrl = $derived(`${page.url.origin}${canonicalHref}`);

	function requireAuth(): boolean {
		if (!authed) {
			goto(`/login?redirect=${encodeURIComponent(page.url.pathname + page.url.search)}`);
			return false;
		}
		return true;
	}

	async function toggleBookmark(e: Event) {
		e.stopPropagation();
		if (!requireAuth() || bookmarkPending) return;
		bookmarkPending = true;
		const next = !bookmarked;
		bookmarked = next;
		bookmarkCount += next ? 1 : -1;
		const forumId = isForum ? (item.forum_thread_id ?? null) : null;
		if (!forumId) updateViewerState(item.id, { bookmarked: next });
		try {
			if (forumId) {
				if (next) {
					const res = await api.post<{ id: string }>(`/forum/bookmarks`, {
						entity_type: 'forum_thread',
						entity_id: forumId
					});
					if (res.error) {
						// A duplicate still means the bookmark exists — sync
						// rather than fail.
						const m = await loadForumBookmarks();
						if (!m.has(forumId)) throw new Error(res.error.message);
					}
					if (res.data?.id) forumBookmarkMap?.set(forumId, res.data.id);
				} else {
					let m = await loadForumBookmarks();
					let bmId = m.get(forumId);
					if (!bmId) {
						// Stale cache — refetch once before giving up.
						invalidateForumBookmarks();
						m = await loadForumBookmarks();
						bmId = m.get(forumId);
					}
					if (bmId) {
						const res = await api.delete(`/forum/bookmarks/${bmId}`);
						if (res.error) throw new Error(res.error.message);
						m.delete(forumId);
					}
				}
			} else if (next) {
				const res = await api.post(`/content/${item.id}/bookmark`, {});
				if (res.error) throw new Error(res.error.message);
			} else {
				const res = await api.delete(`/content/${item.id}/bookmark`);
				if (res.error) throw new Error(res.error.message);
			}
		} catch {
			bookmarked = !next;
			bookmarkCount += next ? -1 : 1;
			if (!forumId) updateViewerState(item.id, { bookmarked: !next });
			bookmarkPending = false;
			return;
		}
		// No finally: Svelte 5 $state set in finally does not re-render.
		bookmarkPending = false;
	}

	async function reportPost(e: Event) {
		e.stopPropagation();
		const reason = window.prompt(t('feed.report_prompt', 'Why are you reporting this post?'));
		if (!reason) return;
		const res = await api.post<void>(`/content/${item.id}/report`, { reason });
		if (res.error) {
			window.alert(res.error.message ?? t('error.500_body'));
		} else {
			reported = true;
		}
	}
</script>

<div
	class="group relative flex cursor-pointer flex-col gap-4 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4 outline-none transition-colors hover:bg-[color-mix(in_srgb,var(--color-surface-subtle)_40%,transparent)]"
	role="link"
	tabindex="0"
	onclick={navigate}
	onkeydown={onKeyDown}
>
	<!-- Source row: avatar + entity name + secondary metadata -->
	<div class="flex items-center gap-2">
		<EntityHoverChip
			type={sourceEntity.type}
			id={sourceEntity.id}
			slug={sourceEntity.slug}
			name={sourceEntity.name}
			avatar={sourceEntity.avatar}
			variant="row"
			secondary={secondary}
		/>
		<div class="ms-auto flex shrink-0 items-center gap-3">
			<a
				href={canonicalHref}
				class="flex shrink-0 items-center gap-2 whitespace-nowrap rounded-full bg-[var(--color-primary)]/10 px-3 py-1.5 text-[13px] font-semibold text-[var(--color-primary)] transition-colors hover:bg-[var(--color-primary)]/20"
				aria-label={actionLabel}
			>
				<Icon size={15} />
				{actionLabel}
			</a>
			<Menu contentClass="min-w-44">
				{#snippet trigger(props)}
					<button
						type="button"
						{...props}
						class="flex shrink-0 items-center text-[var(--color-muted)] transition-colors hover:text-[var(--color-text)]"
						aria-label={t('feed.card.menu', 'More actions')}
					>
						<MoreVertical size={16} />
					</button>
				{/snippet}
				<!-- Share lives in the bottom action row (ShareButton) —
				     the ⋮ menu holds moderation actions only. -->
				<MenuItem icon={Flag} onSelect={reportPost} disabled={reported}>
					{reported ? t('feed.reported', 'Reported') : t('feed.card.report', 'Report')}
				</MenuItem>
			</Menu>
		</div>
	</div>

	<div class="flex gap-4">
		<div class="flex min-w-0 flex-1 flex-col gap-3">
			<!-- Title -->
			<h2 class="text-balance text-[17px] font-semibold leading-snug tracking-tight text-[var(--color-text)]">
				<a href={canonicalHref} class="transition-colors hover:text-[var(--color-primary)]">{item.title}</a>
			</h2>

			{#if item.excerpt}
				<p class="line-clamp-2 text-sm text-[var(--color-text-subtle)]">{item.excerpt}</p>
			{/if}

			<!-- Tags -->
			{#if uniqueTags.length > 0}
				<div class="flex flex-wrap items-center gap-2">
					{#each visibleTags as tag (tag)}
						<span class="rounded-[var(--radius)] bg-[var(--color-surface-subtle)] px-2 py-1 text-[11px] font-medium text-[var(--color-text-subtle)]">
							#{tag}
						</span>
					{/each}
					{#if extraTagCount > 0}
						<span class="rounded-[var(--radius)] bg-[var(--color-surface-subtle)] px-2 py-1 text-[11px] font-medium text-[var(--color-muted)]">
							+{extraTagCount}
						</span>
					{/if}
				</div>
			{/if}

			<!-- Mobile cover image -->
			{#if item.cover_image_url}
				<div class="relative aspect-video w-full overflow-hidden rounded-[var(--radius)] border border-[var(--color-border)] sm:hidden">
					<img src={item.cover_image_url} alt={item.title} loading="lazy" class="h-full w-full object-cover" />
					{#if isVideo}
						<span class="pointer-events-none absolute inset-0 grid place-items-center bg-black/25">
							<Play size={22} class="ms-0.5 text-white" />
						</span>
						{#if duration}
							<span class="absolute bottom-1 end-1 rounded bg-black/75 px-1 py-0.5 text-[10px] font-semibold text-white">{duration}</span>
						{/if}
					{/if}
				</div>
			{/if}

			<!-- Metrics / actions -->
			<div class="flex flex-wrap items-center gap-6 text-sm text-[var(--color-muted)] [font-variant-numeric:tabular-nums]">
				<button
					type="button"
					class="group/btn flex items-center gap-2 transition-colors"
					onclick={openPost}
					aria-label={t('feed.card.comments', 'Comments')}
				>
					<span class="rounded-full p-2 transition-colors group-hover/btn:bg-[var(--color-primary)]/10 group-hover/btn:text-[var(--color-primary)]">
						<MessageSquare size={16} />
					</span>
					<span class="transition-colors group-hover/btn:text-[var(--color-primary)]">{item.comment_count ?? 0}</span>
				</button>
				{#if isForum}
					<VoteRail
						entityType="thread"
						entityId={item.forum_thread_id ?? ''}
						slug={item.slug}
						score={item.vote_score ?? 0}
						layout="horizontal"
						{authed}
					/>
				{:else}
					<ReactionButton
						contentId={item.id}
						count={item.reaction_count ?? 0}
						ariaLabel={t('feed.card.like', 'Like')}
						class="group/btn flex items-center gap-1.5 text-[var(--color-muted)] transition-colors hover:text-[var(--color-primary)]"
						side="top"
						align="center"
					/>
				{/if}
				<button
					type="button"
					class="group/btn flex items-center transition-colors {bookmarked ? 'text-[var(--color-primary)]' : ''}"
					onclick={toggleBookmark}
					disabled={bookmarkPending}
					aria-label={t('feed.card.bookmark', 'Bookmark')}
					aria-pressed={bookmarked}
				>
					<span class="rounded-full p-2 transition-colors {bookmarked ? 'bg-[var(--color-primary)]/10 text-[var(--color-primary)]' : 'group-hover/btn:bg-[var(--color-primary)]/10 group-hover/btn:text-[var(--color-primary)]'}">
						<Bookmark size={16} fill={bookmarked ? 'currentColor' : 'none'} />
					</span>
				</button>
				<ShareButton
					contentId={item.id}
					url={shareUrl}
					title={item.title}
					size={18}
					class="group/btn flex shrink-0 items-center justify-center rounded-full p-2 text-[var(--color-muted)] transition-colors hover:bg-[var(--color-primary)]/10 hover:text-[var(--color-primary)]"
				/>
			</div>
		</div>

		<!-- Cover image (right on desktop) -->
		{#if item.cover_image_url}
			<div class="relative hidden aspect-video w-[220px] shrink-0 overflow-hidden rounded-[var(--radius)] border border-[var(--color-border)] sm:block">
				<img src={item.cover_image_url} alt={item.title} loading="lazy" class="h-full w-full object-cover" />
				{#if isVideo}
					<span class="pointer-events-none absolute inset-0 grid place-items-center bg-black/25">
						<Play size={22} class="ms-0.5 text-white" />
					</span>
					{#if duration}
						<span class="absolute bottom-1 end-1 rounded bg-black/75 px-1 py-0.5 text-[10px] font-semibold text-white">{duration}</span>
					{/if}
				{/if}
			</div>
		{:else if isVideo}
			<div class="relative hidden aspect-video w-[220px] shrink-0 place-items-center rounded-[var(--radius)] border border-[var(--color-border)] bg-gradient-to-br from-[var(--color-primary)]/12 via-[var(--color-surface-subtle)] to-[var(--color-surface-subtle)] sm:grid">
				<Play size={22} class="text-[var(--color-muted)]" />
				{#if duration}
					<span class="absolute bottom-1 end-1 rounded bg-black/75 px-1 py-0.5 text-[10px] font-semibold text-white">{duration}</span>
				{/if}
			</div>
		{/if}
	</div>
</div>
