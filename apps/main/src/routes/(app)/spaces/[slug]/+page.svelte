<script lang="ts">
	import { untrack } from 'svelte';
	import { t } from '$lib/i18n';
	import { page } from '$app/state';
	import {
		Rss, MessageSquare, Users, Info, PenSquare, Search, BarChart3,
		UserPlus, UserCheck, Loader2, BellOff, Bell, Flag, Pin, PinOff,
		Check, X, Inbox, ChevronUp, ChevronDown, BookOpen, Star
	} from 'lucide-svelte';
	import { Dialog } from 'bits-ui';
	import SetSidebar from '$lib/components/shell/SetSidebar.svelte';
	import SpaceSidebar from '$lib/components/spaces/SpaceSidebar.svelte';
	import SpaceSettings from '$lib/components/spaces/SpaceSettings.svelte';
	import ForumList from '$lib/components/forum/ForumList.svelte';
	import { EmptyState, UrlTabs } from '$lib/components/ui';
	import type { UrlTab } from '$lib/components/ui';
	import { api } from '$lib/api/client';
	import { requireAuthOrRedirect } from '$lib/auth/guest';
	import { decodeBase64Text } from '$lib/tiptap/serialize';
	import type { FeedItem } from '$lib/types';
	import FeedCard from '$lib/components/feed/FeedCard.svelte';
	import DigestWidget from '$lib/components/digest/DigestWidget.svelte';
	import { breadcrumbJsonLd } from '$lib/seo/breadcrumbs';
	import type {
		Space, SpaceContentItem, SpaceMember, SpaceModerator, SpaceRule,
		SpacePermissionPreset, SpaceJoinRequest, WikiPageIndex, SpaceReview
	} from '$lib/types';
	import type { PageData } from './$types';
import { safeJsonLdScript } from '$lib/utils/safeHtml';

	let { data }: { data: PageData } = $props();

	const space = $derived(data.space);
	const authed = $derived(!!page.data.user);
	const wikiEnabled = $derived(!!data.wikiEnabled);
	const wikiPages = $derived<WikiPageIndex[]>(data.wikiPages ?? []);
	const isDestinationWiki = $derived(['country', 'state', 'city'].includes(space.level));
	const isUniversity = $derived(data.isUniversity ?? false);
	const reviews = $derived<SpaceReview[]>(data.reviews ?? []);

	function formatWikiCategory(category: string): string {
		return category.replace(/_/g, ' ').replace(/\b\w/g, (l) => l.toUpperCase());
	}

	// Map SpaceContentItem → FeedItem so space feed renders through FeedCard.
	function toFeedItem(item: SpaceContentItem): FeedItem {
		return {
			id: item.id,
			title: item.title,
			slug: item.slug,
			content_type: item.content_type,
			author_id: item.author_id,
			created_at: item.published_at,
			view_count: item.view_count,
			comment_count: item.comment_count,
			reading_time_minutes: item.read_time_minutes,
			space_id: item.space_id,
			space_name: space.name,
			space_slug: space.slug
		};
	}

	// Parse wiki_content JSON (stored base64-encoded in the API response).
	// The auto-populate task stores {title, extract, description, thumbnail, content_url}.
	const wikiData = $derived.by(() => {
		const raw = space.wiki_content;
		if (!raw) return null;
		try {
			const json = typeof raw === 'string' ? JSON.parse(decodeBase64Text(raw)) : raw;
			return json;
		} catch {
			return null;
		}
	});
	let activeTab = $state(untrack(() => page.url.searchParams.get('tab') ?? 'feed'));
	const canManage = $derived(data.isOwner || data.isModerator);

	// Hydrate the Join button's initial state from the detail bundle's
	// viewer-scoped flags (is_member / is_pending from GET /spaces/{slug}).
	// The previously used page.data.userSpaces (/me/spaces) is unreliable for
	// super-admins — that endpoint intentionally lists every space, and it is
	// also capped at the first page for regular members (audit fix).
	const initialJoined = $derived(authed && (data.isMember ?? false));
	const initialPending = $derived(authed && (data.isPending ?? false));

	// Shared follow state — lifted to the page so the sidebar's Join button
	// (auto-follow on join) and the header Follow button stay in sync.
	let following = $state(untrack(() => data.initiallyFollowing ?? false));
	let followPending = $state(false);

	async function toggleFollow() {
		if (followPending) return;
		if (!(await requireAuthOrRedirect())) return;
		const was = following;
		following = !was;
		followPending = true;
		const res = was
			? await api.delete<{ following: boolean }>(`/spaces/${space.slug}/follow`)
			: await api.post<{ following: boolean }>(`/spaces/${space.slug}/follow`);
		if (res.error) {
			following = was;
		} else if (res.data && typeof res.data.following === 'boolean') {
			following = res.data.following;
		}
		followPending = false;
	}

	// Mute state — SSR-seeded, toggled client-side.
	let muted = $state(untrack(() => data.isMuted ?? false));
	let mutePending = $state(false);

	async function toggleMute() {
		if (mutePending) return;
		if (!(await requireAuthOrRedirect())) return;
		const was = muted;
		muted = !was;
		mutePending = true;
		const res = was
			? await api.delete<{ muted: boolean }>(`/spaces/${space.slug}/mute`)
			: await api.post<{ muted: boolean }>(`/spaces/${space.slug}/mute`);
		if (res.error) {
			muted = was;
		} else if (res.data && typeof res.data.muted === 'boolean') {
			muted = res.data.muted;
		}
		mutePending = false;
	}

	// Report dialog state.
	let reportOpen = $state(false);
	let reportReason = $state('');
	let reportNotes = $state('');
	let reportSubmitting = $state(false);
	let reportError = $state('');
	let reportSubmitted = $state(false);

	// Rule 16: report reason codes are admin-configurable via
	// system_settings.moderation.report_reasons (Section 51).
	const reportReasons = $derived<string[]>(
		Array.isArray(page.data.siteSettings?.['moderation.report_reasons'])
			? (page.data.siteSettings['moderation.report_reasons'] as string[])
			: []
	);

	function humanize(code: string): string {
		return code.replace(/_/g, ' ').replace(/\b\w/g, (c) => c.toUpperCase());
	}

	async function submitReport() {
		if (!reportReason.trim() || reportSubmitting) return;
		reportSubmitting = true;
		reportError = '';
		const res = await api.post<{ id: string }>(`/spaces/${space.id}/report`, {
			reason: reportReason.trim(),
			notes: reportNotes.trim() || null,
			evidence_urls: []
		});
		if (res.error) {
			reportError = res.error.message;
		} else {
			reportSubmitted = true;
			reportReason = '';
			reportNotes = '';
		}
		reportSubmitting = false;
	}

	function closeReport() {
		reportOpen = false;
		reportSubmitted = false;
		reportError = '';
	}

	const analytics = $derived(data.analytics as {
		total_views?: number;
		total_posts?: number;
		total_members?: number;
		total_followers?: number;
		posts_last_30d?: number;
	} | null);

	const analyticsError = $derived(data.analyticsError as string | null);

	// Pinned list is component state (not just SSR data) so pin/unpin/reorder
	// reflect immediately in the Pinned tab — previously the tab read
	// data.pinned while mutations only touched feedItems (audit fix).
	let pinnedItems = $state<SpaceContentItem[]>(untrack(() => data.pinned));

	const tabs = $derived<UrlTab[]>([
		{ value: 'feed', label: t('space.tab.feed', 'Feed') },
		...(pinnedItems.length > 0 ? [{ value: 'pinned', label: t('space.tab.pinned', 'Pinned') }] : []),
		{ value: 'forum', label: t('space.tab.forum', 'Forum') },
		{ value: 'search', label: t('space.tab.search', 'Search') },
		{ value: 'members', label: t('space.tab.members', 'Members') },
		...(isUniversity ? [{ value: 'reviews', label: t('space.tab.reviews', 'Reviews') }] : []),
		{ value: 'about', label: t('space.tab.about', 'About') },
		// Analytics dashboard — space-admin only per JEOS §215 [auth,
		// space-admin]. The server only returns analytics data to the space
		// owner/moderators (403 → null otherwise), so the tab is hidden for
		// guests and non-admin members.
		...(authed && analytics
			? [{ value: 'analytics', label: t('space.tab.analytics', 'Analytics') }]
			: []),
		// Join requests — only shown for owner/moderator when joining_perm
		// is 'request' (otherwise the tab is empty/irrelevant).
		...(canManage && space.joining_perm === 'request'
			? [{ value: 'requests', label: t('space.tab.requests', 'Requests') }]
			: []),
		// Settings — visible only to the space owner or a moderator. The
		// PATCH endpoint still enforces the same permission server-side.
		...(canManage
			? [{ value: 'settings', label: t('space.tab.settings', 'Settings') }]
			: [])
	]);

	// Clamp the active tab to the rendered set — an invalid ?tab= value (or a
	// tab that disappears, e.g. 'pinned' after the last unpin) would
	// otherwise render a blank content area (audit fix).
	$effect(() => {
		if (!tabs.some((tab) => tab.value === activeTab)) {
			activeTab = 'feed';
		}
	});

	// Search tab state (Phase 08.09).
	let searchQuery = $state('');
	let searchResults = $state<SpaceContentItem[]>([]);
	let searching = $state(false);

	async function runSearch() {
		if (!searchQuery.trim()) {
			searchResults = [];
			return;
		}
		searching = true;
		const q = encodeURIComponent(searchQuery.trim());
		// /spaces/:slug/content?q=... returns the SpaceContentItem[] shape the
		// search tab expects. The Meilisearch /spaces/:id/search endpoint
		// returns a SearchResponse envelope and is not yet mapped here.
		const res = await api.get<SpaceContentItem[]>(`/spaces/${space.slug}/content?q=${q}&page_size=20`);
		searchResults = res.data ?? [];
		searching = false;
	}

	const title = $derived(`${space.name} — ${t('app.name')}`);
	const description = $derived(space.description ?? `${space.name} on ${t('app.name')}`);

	// BreadcrumbList JSON-LD (Section 26 — all pages with hierarchy).
	const breadcrumbLd = $derived(
		JSON.stringify(
			breadcrumbJsonLd([
				{ name: 'Home', path: page.url.origin },
				{ name: 'Spaces', path: `${page.url.origin}/spaces` },
				{ name: space.name, path: page.url.href }
			])
		)
	);

	// Client-side load-more for the space feed (cursor-less page_offset paging).
	let feedItems = $state<SpaceContentItem[]>(untrack(() => data.content));
	let feedLoading = $state(false);
	let feedEnd = $derived(data.content.length < data.pageSize);
	let feedError = $state('');

	// Client-side load-more for the members list (page-based paging).
	let membersItems = $state(untrack(() => data.members));
	let membersLoading = $state(false);
	let membersPage = $state(1);
	let membersEnd = $derived(data.members.length < data.pageSize);

	// Moderators + rules + parent spaces — seeded from SSR, mutated client-side
	// in the Settings tab. Re-seeds when the slug changes.
	let moderators = $state<SpaceModerator[]>(untrack(() => data.moderators));
	let rules = $state<SpaceRule[]>(untrack(() => data.rules));
	let parentSpaces = $state<Space[]>(untrack(() => data.parentSpaces));

	$effect(() => {
		// Re-seed on SSR payload change (slug change).
		feedItems = data.content;
		pinnedItems = data.pinned;
		membersItems = data.members;
		moderators = data.moderators;
		rules = data.rules;
		parentSpaces = data.parentSpaces;
		membersPage = 1;
		membersEnd = data.members.length < data.pageSize;
	});

	async function loadMoreFeed() {
		if (feedLoading || feedEnd) return;
		feedLoading = true;
		feedError = '';
		try {
			const res = await api.get<SpaceContentItem[]>(
				`/spaces/${space.slug}/content?page_size=${data.pageSize}&page_offset=${feedItems.length}`
			);
			if (res.data) {
				feedItems = [...feedItems, ...res.data];
				if (res.data.length < data.pageSize) feedEnd = true;
			}
		} catch {
			feedError = 'Failed to load more content. Please try again.';
		}
		feedLoading = false;
	}

	// reloadFeed refetches the first page of feed items. Used by the pinned
	// reorder rollback path to restore server-side order after a failed
	// PATCH /spaces/{id}/pinned/reorder.
	async function reloadFeed() {
		try {
			const res = await api.get<SpaceContentItem[]>(
				`/spaces/${space.slug}/content?page_size=${data.pageSize}&page_offset=0`
			);
			if (res.data) {
				feedItems = res.data;
				feedEnd = res.data.length < data.pageSize;
			}
		} catch {
			// Best-effort — leave the current order in place.
		}
	}

	async function loadMoreMembers() {
		if (membersLoading || membersEnd) return;
		membersLoading = true;
		const nextPage = membersPage + 1;
		const offset = nextPage * data.pageSize;
		const res = await api.get<SpaceMember[]>(
			`/spaces/${space.slug}/members?page_size=${data.pageSize}&page_offset=${offset}`
		);
		if (res.data) {
			membersItems = [...membersItems, ...res.data];
			membersPage = nextPage;
			if (res.data.length < data.pageSize) membersEnd = true;
		}
		membersLoading = false;
	}

	// --- Pinned post management (Ref: §05; POST/DELETE /spaces/{slug}/pinned) ---
	let pinningId = $state<string | null>(null);

	async function togglePin(item: SpaceContentItem) {
		if (pinningId) return;
		pinningId = item.id;
		const was = item.is_pinned;
		// Optimistic update — feed items AND the pinned-tab list.
		feedItems = feedItems.map((f) => f.id === item.id ? { ...f, is_pinned: !was } : f);
		const prevPinned = pinnedItems;
		pinnedItems = was
			? pinnedItems.filter((f) => f.id !== item.id)
			: [...pinnedItems, { ...item, is_pinned: true }];
		try {
			const res = was
				? await api.delete<{ unpinned: boolean }>(`/spaces/${space.slug}/pinned/${item.id}`)
				: await api.post<{ id: string }>(`/spaces/${space.slug}/pinned`, {
						content_id: item.id,
						position: prevPinned.length + 1
					});
			if (res.error) throw new Error(res.error.message);
		} catch {
			// Rollback on failure
			feedItems = feedItems.map((f) => f.id === item.id ? { ...f, is_pinned: was } : f);
			pinnedItems = prevPinned;
		}
		pinningId = null;
	}

	// reloadPinned refetches the pinned list — used by the reorder rollback
	// path to restore server-side order after a failed reorder PATCH.
	async function reloadPinned() {
		try {
			const res = await api.get<SpaceContentItem[]>(`/spaces/${space.slug}/pinned`);
			if (res.data) pinnedItems = res.data;
		} catch {
			// Best-effort — leave the current order in place.
		}
	}

	// --- Pinned post reorder (Ref: §05; PATCH /spaces/{id}/pinned/reorder) ---
	let reordering = $state(false);

	async function reorderPinned(direction: 'up' | 'down', item: SpaceContentItem) {
		if (reordering) return;
		const idx = pinnedItems.findIndex((f) => f.id === item.id);
		if (idx < 0) return;
		const swapIdx = direction === 'up' ? idx - 1 : idx + 1;
		if (swapIdx < 0 || swapIdx >= pinnedItems.length) return;
		// Optimistic: swap the two pinned items in the pinned list (and in the
		// feed list so the Feed tab shows the same order).
		const newPinned = [...pinnedItems];
		[newPinned[idx], newPinned[swapIdx]] = [newPinned[swapIdx], newPinned[idx]];
		pinnedItems = newPinned;
		const aId = newPinned[swapIdx].id;
		const bId = newPinned[idx].id;
		const aIdx = feedItems.findIndex((f) => f.id === aId);
		const bIdx = feedItems.findIndex((f) => f.id === bId);
		if (aIdx >= 0 && bIdx >= 0) {
			const newItems = [...feedItems];
			[newItems[aIdx], newItems[bIdx]] = [newItems[bIdx], newItems[aIdx]];
			feedItems = newItems;
		}
		reordering = true;
		try {
			const newOrder = pinnedItems.map((f) => f.id);
			const res = await api.patch<{ reordered: boolean }>(`/spaces/${space.id}/pinned/reorder`, {
				ordered_content_ids: newOrder
			});
			if (res.error) throw new Error(res.error.message);
		} catch {
			// Rollback: refetch both lists to restore server-side order
			await Promise.all([reloadFeed(), reloadPinned()]);
		}
		reordering = false;
	}

	// --- Join request queue (Ref: §05; GET /admin/spaces/{id}/join-requests) ---
	let joinRequests = $state<SpaceJoinRequest[]>([]);
	let requestsLoading = $state(false);
	let requestsLoaded = $state(false);
	let requestActionId = $state<string | null>(null);
	let requestError = $state('');

	async function loadJoinRequests() {
		if (requestsLoaded) return;
		requestsLoading = true;
		requestError = '';
		const res = await api.get<SpaceJoinRequest[]>(`/admin/spaces/${space.id}/join-requests`);
		if (res.error) {
			requestError = res.error.message;
		} else if (res.data) {
			joinRequests = res.data;
			requestsLoaded = true;
		}
		requestsLoading = false;
	}

	async function updateJoinRequest(userId: string, status: 'approved' | 'rejected') {
		if (requestActionId) return;
		requestActionId = userId;
		requestError = '';
		const res = await api.patch<{ status: string }>(
			`/admin/spaces/${space.id}/join-requests/${userId}`, { status }
		);
		if (res.error) {
			requestError = res.error.message;
		} else {
			joinRequests = joinRequests.filter((r) => r.user_id !== userId);
		}
		requestActionId = null;
	}

	// Load join requests when the requests tab becomes active.
	$effect(() => {
		if (activeTab === 'requests' && canManage) {
			loadJoinRequests();
		}
	});
</script>

<svelte:head>
	<title>{title}</title>
	<meta name="description" content={description} />
	<meta property="og:title" content={title} />
	<meta property="og:description" content={description} />
	<meta property="og:type" content="website" />
	<meta property="og:url" content={page.url.href} />
	{@html safeJsonLdScript(breadcrumbLd)}
</svelte:head>

<SetSidebar>
	<SpaceSidebar {space} rules={data.rules} {authed} logoUrl={data.logoUrl} {initialJoined} {initialPending} following={data.initiallyFollowing} onFollowChange={(v) => (following = v)} achievements={data.achievements} members={data.members} />
</SetSidebar>

<!-- Space header — cover banner + overlapping logo, follows ProfileHeader pattern -->
<header class="border-b border-[var(--color-border)] bg-[var(--color-surface)]">
	<!-- Cover banner — image with dark fade at bottom, or gradient fallback.
	     The dark fade at the bottom improves logo/title legibility. -->
	<div class="relative h-32 overflow-hidden sm:h-40">
		{#if data.headerUrl}
			<img src={data.headerUrl} alt="" class="h-full w-full object-cover" />
		{:else}
			<!-- Visible gradient fallback — works in both light and dark mode -->
			<div
				class="h-full w-full"
				style="background: linear-gradient(135deg, color-mix(in srgb, var(--color-primary) 30%, transparent) 0%, color-mix(in srgb, var(--color-primary) 8%, var(--color-surface-subtle)) 100%);"
			></div>
		{/if}
		<!-- Dark fade at bottom — always applied, over both image and fallback -->
		<div class="pointer-events-none absolute inset-x-0 bottom-0 h-2/3 bg-gradient-to-t from-black/70 via-black/30 to-transparent"></div>
	</div>

	<div class="relative px-4 pb-4">
		<div class="-mt-12 flex flex-col gap-3 sm:-mt-16 sm:flex-row sm:items-end sm:gap-4">
			<!-- Logo — overlaps the cover, falls back to first letter -->
			{#if data.logoUrl}
				<img
					src={data.logoUrl}
					alt={space.name}
					class="h-24 w-24 shrink-0 rounded-[var(--radius-card)] border-4 border-[var(--color-surface)] bg-[var(--color-surface)] object-cover shadow-lg sm:h-28 sm:w-28"
				/>
			{:else}
				<span
					class="grid h-24 w-24 shrink-0 place-items-center rounded-[var(--radius-card)] border-4 border-[var(--color-surface)] bg-[var(--color-surface-subtle)] text-3xl font-bold text-[var(--color-primary)] shadow-lg sm:h-28 sm:w-28"
				>
					{space.name.charAt(0).toUpperCase()}
				</span>
			{/if}

			<div class="flex-1 pb-1">
				<h1 class="text-xl font-semibold text-[var(--color-text)]">{space.name}</h1>
				<p class="text-sm capitalize text-[var(--color-muted)]">{space.level} · {space.visibility}</p>
				{#if space.description}
					<p class="mt-1 line-clamp-2 text-sm text-[var(--color-text-subtle)]">{space.description}</p>
				{/if}
			</div>

			{#if authed}
				<div class="flex shrink-0 items-center gap-2 pb-1">
					<button
						type="button"
						onclick={toggleMute}
						disabled={mutePending}
						aria-busy={mutePending}
						title={muted ? t('space.unmute', 'Unmute') : t('space.mute', 'Mute')}
						class="inline-flex shrink-0 items-center gap-1.5 rounded-full border border-[var(--color-border)] px-3 py-1.5 text-sm font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)] disabled:opacity-50"
					>
						{#if mutePending}
							<Loader2 size={16} class="animate-spin" />
						{:else if muted}
							<BellOff size={16} />
						{:else}
							<Bell size={16} />
						{/if}
					</button>
					<button
						type="button"
						onclick={() => (reportOpen = true)}
						title={t('space.report', 'Report')}
						class="inline-flex shrink-0 items-center gap-1.5 rounded-full border border-[var(--color-border)] px-3 py-1.5 text-sm font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)]"
					>
						<Flag size={16} />
					</button>
					<button
						type="button"
						onclick={toggleFollow}
						disabled={followPending}
						aria-busy={followPending}
						class="inline-flex shrink-0 items-center gap-1.5 rounded-full px-4 py-1.5 text-sm font-medium transition-colors disabled:opacity-50 {following
							? 'border border-[var(--color-border)] text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]'
							: 'bg-[var(--color-primary)] text-[var(--color-primary-contrast)] hover:bg-[var(--color-primary-hover)]'}"
					>
						{#if followPending}
							<Loader2 size={16} class="animate-spin" />
						{:else if following}
							<UserCheck size={16} />
							{t('profile.following_label', 'Following')}
						{:else}
							<UserPlus size={16} />
							{t('profile.follow', 'Follow')}
						{/if}
					</button>
					<a
						href="/wiki/{space.slug}"
						class="inline-flex shrink-0 items-center gap-1.5 rounded-full border border-[var(--color-border)] px-4 py-1.5 text-sm font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)]"
					>
						<BookOpen size={16} /> {t('space.wiki', 'Wiki')}
					</a>
					<a
						href="/create?type=blog_post&space={space.slug}"
						class="inline-flex shrink-0 items-center gap-1.5 rounded-full bg-[var(--color-primary)] px-4 py-1.5 text-sm font-medium text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)]"
					>
						<PenSquare size={16} /> {t('space.post', 'Post')}
					</a>
				</div>
			{/if}
		</div>
	</div>
</header>

<UrlTabs {tabs} bind:value={activeTab} />

<div class="mx-auto w-full max-w-full">
	<div class="p-3">
		{#if activeTab === 'feed'}
			<div class="mb-3">
				<DigestWidget spaceId={space.id} digest={data.digest} />
			</div>
			{#if feedItems.length === 0}
				<EmptyState icon={Rss} title={t('space.feed_empty', 'No posts yet')} />
			{:else}
				<div class="divide-y divide-[var(--color-border)] overflow-hidden rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)]">
					{#each feedItems as item (item.id)}
						<div class="group relative block">
							<FeedCard
								item={toFeedItem(item)}
								style="daily"
								layout="stream"
							/>
							{#if item.is_pinned}
								<span class="pointer-events-none absolute left-2 top-2 z-10 flex items-center gap-0.5 rounded-full bg-[var(--color-primary)]/10 px-2 py-0.5 text-[10px] font-medium text-[var(--color-primary)]">
									<Pin size={10} /> {t('space.pinned', 'Pinned')}
								</span>
							{/if}
							{#if canManage}
								<div class="absolute right-3 top-3 z-10 flex items-center gap-0.5 opacity-0 transition-opacity group-hover:opacity-100">
									{#if item.is_pinned}
										<button
											type="button"
											disabled={reordering}
											onclick={() => reorderPinned('up', item)}
											class="grid h-7 w-7 place-items-center rounded-full text-[var(--color-muted)] hover:bg-[var(--color-surface)] disabled:opacity-50"
											title={t('space.pinned_move_up', 'Move pinned up')}
											aria-label={t('space.pinned_move_up', 'Move pinned up')}
										>
											<ChevronUp size={13} />
										</button>
										<button
											type="button"
											disabled={reordering}
											onclick={() => reorderPinned('down', item)}
											class="grid h-7 w-7 place-items-center rounded-full text-[var(--color-muted)] hover:bg-[var(--color-surface)] disabled:opacity-50"
											title={t('space.pinned_move_down', 'Move pinned down')}
											aria-label={t('space.pinned_move_down', 'Move pinned down')}
										>
											<ChevronDown size={13} />
										</button>
									{/if}
									<button
										type="button"
										disabled={pinningId === item.id}
										onclick={() => togglePin(item)}
										class="grid h-7 w-7 place-items-center rounded-full text-[var(--color-muted)] hover:bg-[var(--color-surface)] disabled:opacity-50"
										title={item.is_pinned ? t('space.unpin', 'Unpin') : t('space.pin', 'Pin')}
										aria-label={item.is_pinned ? t('space.unpin', 'Unpin') : t('space.pin', 'Pin')}
									>
										{#if pinningId === item.id}
											<Loader2 size={13} class="animate-spin" />
										{:else if item.is_pinned}
											<PinOff size={13} />
										{:else}
											<Pin size={13} />
										{/if}
									</button>
								</div>
							{/if}
						</div>
					{/each}
				</div>
				{#if !feedEnd}
					{#if feedError}
						<p class="mx-auto mt-3 text-center text-sm text-red-600 dark:text-red-400">{feedError}</p>
					{/if}
					<button
						type="button"
						onclick={loadMoreFeed}
						disabled={feedLoading}
						class="mx-auto mt-3 block rounded-[var(--radius)] border border-[var(--color-border)] px-4 py-2 text-sm text-[var(--color-text-subtle)] hover:bg-[var(--color-surface-subtle)] disabled:opacity-50"
					>
						{feedLoading ? t('common.loading') : t('feed.load_more', 'Load more')}
					</button>
				{/if}
			{/if}
		{:else if activeTab === 'pinned'}
			{#if pinnedItems.length === 0}
				<EmptyState icon={Pin} title={t('space.pinned_empty', 'No pinned posts yet')} />
			{:else}
				<div class="divide-y divide-[var(--color-border)] overflow-hidden rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)]">
					{#each pinnedItems as item (item.id)}
						<div class="group relative block">
							<FeedCard
								item={toFeedItem(item)}
								style="daily"
								layout="stream"
							/>
							<span class="pointer-events-none absolute left-2 top-2 z-10 flex items-center gap-0.5 rounded-full bg-[var(--color-primary)]/10 px-2 py-0.5 text-[10px] font-medium text-[var(--color-primary)]">
								<Pin size={10} /> {t('space.pinned', 'Pinned')}
							</span>
							{#if canManage}
								<div class="absolute right-3 top-3 z-10 flex items-center gap-0.5 opacity-0 transition-opacity group-hover:opacity-100">
									<button
										type="button"
										disabled={reordering}
										onclick={() => reorderPinned('up', item)}
										class="grid h-7 w-7 place-items-center rounded-full text-[var(--color-muted)] hover:bg-[var(--color-surface)] disabled:opacity-50"
										title={t('space.pinned_move_up', 'Move pinned up')}
										aria-label={t('space.pinned_move_up', 'Move pinned up')}
									>
										<ChevronUp size={13} />
									</button>
									<button
										type="button"
										disabled={reordering}
										onclick={() => reorderPinned('down', item)}
										class="grid h-7 w-7 place-items-center rounded-full text-[var(--color-muted)] hover:bg-[var(--color-surface)] disabled:opacity-50"
										title={t('space.pinned_move_down', 'Move pinned down')}
										aria-label={t('space.pinned_move_down', 'Move pinned down')}
									>
										<ChevronDown size={13} />
									</button>
									<button
										type="button"
										disabled={pinningId === item.id}
										onclick={() => togglePin(item)}
										class="grid h-7 w-7 place-items-center rounded-full text-[var(--color-muted)] hover:bg-[var(--color-surface)] disabled:opacity-50"
										title={t('space.unpin', 'Unpin')}
										aria-label={t('space.unpin', 'Unpin')}
									>
										{#if pinningId === item.id}
											<Loader2 size={13} class="animate-spin" />
										{:else}
											<PinOff size={13} />
										{/if}
									</button>
								</div>
							{/if}
						</div>
					{/each}
				</div>
			{/if}
		{:else if activeTab === 'forum'}
			{#if data.forumThreads.length === 0}
				<EmptyState icon={MessageSquare} title={t('space.forum_empty', 'No discussions yet')} />
			{:else}
				<ForumList threads={data.forumThreads} pageSize={data.forumPageSize} loadError={false} />
			{/if}
		{:else if activeTab === 'members'}
			{#if membersItems.length === 0}
				<EmptyState icon={Users} title={t('space.members_empty', 'No members yet')} />
			{:else}
				<ul class="space-y-2">
					{#each membersItems as member (member.user_id)}
						<li>
							<a
								href="/u/{member.username}"
								class="flex items-center gap-3 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-3 transition-colors hover:bg-[var(--color-surface-subtle)]"
							>
								{#if member.avatar_url}
									<img src={member.avatar_url} alt="" class="h-10 w-10 rounded-full object-cover" />
								{:else}
									<span class="grid h-10 w-10 shrink-0 place-items-center rounded-full bg-[var(--color-surface-subtle)] text-sm font-medium text-[var(--color-text-subtle)]">
										{(member.display_name ?? member.username).charAt(0).toUpperCase()}
									</span>
								{/if}
								<span class="min-w-0 flex-1">
									<span class="block truncate text-sm font-medium text-[var(--color-text)]">{member.display_name ?? member.username}</span>
									<span class="block text-xs text-[var(--color-muted)]">@{member.username}</span>
								</span>
							</a>
						</li>
					{/each}
				</ul>
				{#if !membersEnd}
					<button
						type="button"
						onclick={loadMoreMembers}
						disabled={membersLoading}
						class="mx-auto mt-3 block rounded-[var(--radius)] border border-[var(--color-border)] px-4 py-2 text-sm text-[var(--color-text-subtle)] hover:bg-[var(--color-surface-subtle)] disabled:opacity-50"
					>
						{membersLoading ? t('common.loading') : t('feed.load_more', 'Load more')}
					</button>
				{/if}
			{/if}
		{:else if activeTab === 'about'}
			<section class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
				{#if space.description}
					<p class="text-sm leading-relaxed text-[var(--color-text)]">{space.description}</p>
				{:else}
					<EmptyState icon={Info} title={t('space.no_about', 'No description yet')} />
				{/if}

				{#if isDestinationWiki && wikiEnabled}
					<div class="mt-4 border-t border-[var(--color-border)] pt-4">
						<h2 class="mb-2 text-sm font-medium text-[var(--color-text)]">{t('space.community_wiki', 'Community Wiki')}</h2>
						{#if wikiPages.length > 0}
							<ul class="mb-3 space-y-2">
								{#each wikiPages as page (page.id)}
									<li>
										<a
											href="/wiki/{encodeURIComponent(space.slug)}/{encodeURIComponent(page.slug)}"
											class="flex items-center justify-between rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface-subtle)] px-3 py-2 text-sm transition-colors hover:border-[var(--color-primary)] hover:bg-[var(--color-surface)]"
										>
											<span class="font-medium text-[var(--color-text)]">{page.title}</span>
											<span class="text-xs uppercase tracking-wide text-[var(--color-muted)]">{formatWikiCategory(page.category)}</span>
										</a>
									</li>
								{/each}
							</ul>
						{:else}
							<p class="mb-3 text-sm text-[var(--color-text-subtle)]">{t('space.no_wiki_pages', 'No community wiki pages yet.')}</p>
						{/if}
						<a
							href="/wiki/{encodeURIComponent(space.slug)}"
							class="inline-flex items-center gap-1 text-sm font-medium text-[var(--color-primary)] hover:underline"
						>
							{t('space.view_wiki', 'View wiki')}
							<span aria-hidden="true">→</span>
						</a>
					</div>
				{/if}

				{#if wikiData}
					<div class="mt-4 border-t border-[var(--color-border)] pt-4">
						<h2 class="mb-2 text-sm font-medium text-[var(--color-text)]">{t('space.wiki_summary', 'From Wikipedia')}</h2>
						{#if wikiData.thumbnail}
							<img src={wikiData.thumbnail} alt={wikiData.title ?? space.name} class="float-right mb-3 ml-3 h-24 w-24 rounded-[var(--radius)] object-cover" />
						{/if}
						{#if wikiData.description}
							<p class="mb-2 text-xs font-medium uppercase tracking-wide text-[var(--color-muted)]">{wikiData.description}</p>
						{/if}
						{#if wikiData.extract}
							<p class="text-sm leading-relaxed text-[var(--color-text-subtle)]">{wikiData.extract}</p>
						{/if}
						{#if wikiData.content_url}
							<p class="mt-3 text-xs">
								<a href={wikiData.content_url} target="_blank" rel="noopener noreferrer" class="text-[var(--color-primary)] hover:underline">
									{t('space.wiki_source', 'Read more on Wikipedia →')}
								</a>
							</p>
						{/if}
					</div>
				{/if}
			</section>
		{:else if activeTab === 'search'}
			<!-- Search tab (Phase 08.09) -->
			<form class="mb-3" onsubmit={(e) => { e.preventDefault(); runSearch(); }}>
				<div class="flex items-center gap-2 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 focus-within:border-[var(--color-primary)]">
					<Search size={16} class="text-[var(--color-muted)]" />
					<input
						type="search"
						bind:value={searchQuery}
						placeholder={t('space.search_placeholder', 'Search this space…')}
						class="w-full appearance-none border-0 bg-transparent text-sm text-[var(--color-text)] outline-none ring-0 placeholder:text-[var(--color-muted)] [&::-webkit-search-cancel-button]:appearance-none [&::-webkit-search-cancel-button]:hidden"
					/>
				</div>
			</form>
			{#if searching}
				<p class="py-8 text-center text-sm text-[var(--color-muted)]">{t('space.searching', 'Searching…')}</p>
			{:else if searchQuery.trim() && searchResults.length === 0}
				<EmptyState icon={Search} title={t('space.no_search_results', 'No results found')} />
			{:else if searchResults.length > 0}
				<div class="divide-y divide-[var(--color-border)] overflow-hidden rounded-[var(--radius-card)] bg-[var(--color-surface)]">
					{#each searchResults as item (item.id)}
						<a href="/content/{item.slug}" class="block p-4 transition-colors hover:bg-[var(--color-surface-subtle)]">
							<h3 class="text-sm font-medium text-[var(--color-text)]">{item.title}</h3>
							<p class="mt-1 text-xs text-[var(--color-muted)]">{item.content_type}</p>
						</a>
					{/each}
				</div>
			{/if}
		{:else if activeTab === 'analytics'}
			<!-- Analytics tab (Phase 08.10) -->
			{#if analyticsError === 'permission_denied'}
				<EmptyState icon={BarChart3} title={t('space.analytics.forbidden', 'You do not have permission to view analytics for this space.')} />
			{:else if analyticsError === 'unavailable'}
				<EmptyState icon={BarChart3} title={t('space.analytics.unavailable', 'Analytics unavailable')} />
			{:else if analytics}
				<div class="grid grid-cols-2 gap-3 sm:grid-cols-3">
					<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
						<div class="flex items-center gap-2 text-xs text-[var(--color-muted)]">
							<BarChart3 size={14} /> {t('space.analytics.views', 'Total views')}
						</div>
						<p class="mt-1 text-lg font-semibold text-[var(--color-text)]">{analytics.total_views ?? 0}</p>
					</div>
					<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
						<div class="flex items-center gap-2 text-xs text-[var(--color-muted)]">
							<Rss size={14} /> {t('space.analytics.posts', 'Total posts')}
						</div>
						<p class="mt-1 text-lg font-semibold text-[var(--color-text)]">{analytics.total_posts ?? 0}</p>
					</div>
					<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
						<div class="flex items-center gap-2 text-xs text-[var(--color-muted)]">
							<Users size={14} /> {t('space.analytics.members', 'Members')}
						</div>
						<p class="mt-1 text-lg font-semibold text-[var(--color-text)]">{analytics.total_members ?? 0}</p>
					</div>
					<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
						<div class="flex items-center gap-2 text-xs text-[var(--color-muted)]">
							<Users size={14} /> {t('space.analytics.followers', 'Followers')}
						</div>
						<p class="mt-1 text-lg font-semibold text-[var(--color-text)]">{analytics.total_followers ?? 0}</p>
					</div>
					<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
						<div class="flex items-center gap-2 text-xs text-[var(--color-muted)]">
							<Rss size={14} /> {t('space.analytics.posts_30d', 'Posts (30d)')}
						</div>
						<p class="mt-1 text-lg font-semibold text-[var(--color-text)]">{analytics.posts_last_30d ?? 0}</p>
					</div>
				</div>
			{:else}
				<EmptyState icon={BarChart3} title={t('space.analytics.unavailable', 'Analytics unavailable')} />
			{/if}
		{:else if activeTab === 'requests'}
			<!-- Join request queue — owner/moderator only (Ref: §05; GET /admin/spaces/{id}/join-requests) -->
			<section>
				<h2 class="mb-3 text-sm font-semibold text-[var(--color-text)]">
					{t('space.requests.title', 'Pending join requests')}
				</h2>
				{#if requestError}
					<p class="mb-3 rounded-lg bg-red-50 px-3 py-2 text-xs text-red-700 dark:bg-red-950/30 dark:text-red-400">{requestError}</p>
				{/if}
				{#if requestsLoading}
					<p class="py-8 text-center text-sm text-[var(--color-muted)]">{t('common.loading', 'Loading…')}</p>
				{:else if joinRequests.length === 0}
					<EmptyState icon={Inbox} title={t('space.requests.empty', 'No pending requests')} />
				{:else}
					<ul class="space-y-2">
						{#each joinRequests as req (req.user_id)}
							<li class="flex items-center gap-3 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-3">
								{#if req.avatar_url}
									<img src={req.avatar_url} alt="" class="h-10 w-10 rounded-full object-cover" />
								{:else}
									<span class="grid h-10 w-10 shrink-0 place-items-center rounded-full bg-[var(--color-surface-subtle)] text-sm font-medium text-[var(--color-text-subtle)]">
										{(req.display_name ?? req.username).charAt(0).toUpperCase()}
									</span>
								{/if}
								<a href="/u/{req.username}" class="min-w-0 flex-1">
									<span class="block truncate text-sm font-medium text-[var(--color-text)]">{req.display_name ?? req.username}</span>
									<span class="block text-xs text-[var(--color-muted)]">@{req.username}</span>
								</a>
								<div class="flex shrink-0 items-center gap-2">
									<button
										type="button"
										disabled={requestActionId === req.user_id}
										onclick={() => updateJoinRequest(req.user_id, 'approved')}
										class="inline-flex items-center gap-1 rounded-lg bg-green-600 px-2.5 py-1.5 text-xs font-medium text-white hover:bg-green-700 disabled:opacity-50"
									>
										{#if requestActionId === req.user_id}
											<Loader2 size={12} class="animate-spin" />
										{:else}
											<Check size={12} />
										{/if}
										{t('space.requests.approve', 'Approve')}
									</button>
									<button
										type="button"
										disabled={requestActionId === req.user_id}
										onclick={() => updateJoinRequest(req.user_id, 'rejected')}
										class="inline-flex items-center gap-1 rounded-lg border border-[var(--color-border)] px-2.5 py-1.5 text-xs font-medium text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)] disabled:opacity-50"
									>
										<X size={12} />
										{t('space.requests.reject', 'Reject')}
									</button>
								</div>
							</li>
						{/each}
					</ul>
				{/if}
			</section>
		{:else if activeTab === 'reviews'}
			<section>
				<h2 class="mb-3 text-sm font-semibold text-[var(--color-text)]">
					{t('space.reviews.title', 'Reviews')}
				</h2>
				{#if reviews.length === 0}
					<EmptyState icon={Star} title={t('space.reviews.empty', 'No reviews yet')} />
				{:else}
					<div class="space-y-3">
						{#each reviews as review (review.id)}
							<article class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
								<div class="mb-2 flex items-center justify-between">
									<span class="text-sm font-medium text-[var(--color-text)]">{review.author_name}</span>
									<span class="text-xs text-[var(--color-muted)]">{new Date(review.created_at).toLocaleDateString()}</span>
								</div>
								<div class="mb-2 flex items-center gap-0.5">
									{#each { length: 5 } as _, i (i)}
										<Star size={14} class={i < review.rating ? 'fill-amber-400 text-amber-400' : 'text-[var(--color-muted)]'} />
									{/each}
								</div>
								{#if review.title}
									<h3 class="mb-1 text-sm font-semibold text-[var(--color-text)]">{review.title}</h3>
								{/if}
								{#if review.body}
									<p class="text-sm leading-relaxed text-[var(--color-text-subtle)]">{review.body}</p>
								{/if}
							</article>
						{/each}
					</div>
				{/if}
			</section>
		{:else if activeTab === 'settings'}
			<SpaceSettings
				{space}
				keywords={data.keywords}
				logoUrl={data.logoUrl}
				headerUrl={data.headerUrl}
				isOwner={data.isOwner}
				isModerator={data.isModerator}
				presets={data.presets}
				bind:moderators
				bind:rules
				bind:parentSpaces
			/>
		{/if}
	</div>
</div>

<!-- Report space dialog — bind:open keeps the parent state in sync when the
     dialog is dismissed via Escape/backdrop click, so it can reopen. -->
<Dialog.Root bind:open={reportOpen}>
		<Dialog.Portal>
			<Dialog.Overlay class="fixed inset-0 z-40 bg-black/50 backdrop-blur-sm" />
			<Dialog.Content class="fixed left-1/2 top-1/2 z-50 w-full max-w-md -translate-x-1/2 -translate-y-1/2 rounded-[12px] border border-[var(--color-border)] bg-[var(--color-surface)] p-6 shadow-xl">
				<Dialog.Close class="absolute right-4 top-4 text-[var(--color-muted)] hover:text-[var(--color-text)]" onclick={closeReport}>
					<X size={20} />
				</Dialog.Close>
				<div class="mb-4 flex items-center gap-2">
					<Flag size={20} class="text-red-500" />
					<h2 class="text-lg font-semibold text-[var(--color-text)]">{t('space.report_title', 'Report this space')}</h2>
				</div>
				{#if reportSubmitted}
					<p class="rounded-lg bg-green-50 px-4 py-3 text-sm text-green-700 dark:bg-green-950/30 dark:text-green-400">
						{t('space.report_submitted', 'Your report has been submitted. Our team will review it.')}
					</p>
					<button
						type="button"
						onclick={closeReport}
						class="mt-4 rounded-lg bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-white hover:opacity-90"
					>
						{t('common.close', 'Close')}
					</button>
				{:else}
					<form onsubmit={(e) => { e.preventDefault(); submitReport(); }} class="space-y-3">
						{#if reportError}
							<p class="rounded-lg bg-red-50 px-3 py-2 text-xs text-red-700 dark:bg-red-950/30 dark:text-red-400">{reportError}</p>
						{/if}
						<div>
							<label for="report-reason" class="mb-1 block text-xs font-medium text-[var(--color-muted)]">
								{t('space.report_reason', 'Reason')}
							</label>
							<select
								id="report-reason"
								bind:value={reportReason}
								class="w-full rounded-lg border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)]"
								required
							>
								<option value="">{t('space.report_reason_select', 'Select a reason…')}</option>
								{#each reportReasons as reason (reason)}
									<option value={reason}>{t(`report.reason.${reason}`, humanize(reason))}</option>
								{/each}
							</select>
						</div>
						<div>
							<label for="report-notes" class="mb-1 block text-xs font-medium text-[var(--color-muted)]">
								{t('space.report_notes', 'Additional notes (optional)')}
							</label>
							<textarea
								id="report-notes"
								bind:value={reportNotes}
								rows="3"
								class="w-full rounded-lg border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)]"
							></textarea>
						</div>
						<div class="flex items-center gap-2">
							<button
								type="submit"
								disabled={reportSubmitting || !reportReason.trim()}
								class="rounded-lg bg-red-600 px-4 py-2 text-sm font-medium text-white hover:bg-red-700 disabled:opacity-50"
							>
								{#if reportSubmitting}<Loader2 size={14} class="animate-spin" />{:else}
								{t('space.report_submit', 'Submit report')}{/if}
							</button>
							<button
								type="button"
								onclick={closeReport}
								class="rounded-lg border border-[var(--color-border)] px-4 py-2 text-sm text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]"
							>
								{t('common.cancel', 'Cancel')}
							</button>
						</div>
					</form>
				{/if}
			</Dialog.Content>
		</Dialog.Portal>
	</Dialog.Root>
