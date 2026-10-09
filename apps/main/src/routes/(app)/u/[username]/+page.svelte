<script lang="ts">
	import { t } from '$lib/i18n';
	import { page } from '$app/state';
	import { untrack } from 'svelte';
	import { afterNavigate, invalidate } from '$app/navigation';
	import { FileText, MessageSquare, Image, Heart, Users, Info, BarChart3, Award, Pin, Ban, VolumeX, Compass, Flag, Video, MapPin } from 'lucide-svelte';
	import { MenuItem } from '$lib/components/ui';
	import { Chart, Svg, Bars, Axis, LineChart } from 'layerchart';
	import SetSidebar from '$lib/components/shell/SetSidebar.svelte';
	import ProfileHeader from '$lib/components/profile/ProfileHeader.svelte';
	import FollowButton from '$lib/components/ui/FollowButton.svelte';
	import ReportDialog from '$lib/components/profile/ReportDialog.svelte';
	import UserProfileSidebar from '$lib/components/profile/UserProfileSidebar.svelte';
	import VideoGrid from '$lib/components/profile/VideoGrid.svelte';
	import { EmptyState, LoadingSkeleton, UrlTabs, PaginationControls, SearchInput } from '$lib/components/ui';
	import { openLightbox } from '$lib/lightbox.svelte';
	import Autocomplete from '$lib/components/create/Autocomplete.svelte';
	import type { AutocompleteItem } from '$lib/components/create/Autocomplete.svelte';
	import type { UrlTab } from '$lib/components/ui';
	import type { ProfileAnalytics, ProfilePost, FeedItem, FollowerGrowthItem, TopContentItem } from '$lib/types';
	import FeedCard from '$lib/components/feed/FeedCard.svelte';
	import { formatMetric } from '$lib/components/hub/charts/format';
	import { tiptapExcerpt, decodeTipTapDoc, tiptapToText } from '$lib/tiptap/serialize';
	import { blockUser, unblockUser, muteUser, unmuteUser } from '$lib/api/hub';
	import type { FollowableType } from '$lib/api/hub';
	import type { PageData } from './$types';
	import { safeJsonLdScript } from '$lib/utils/safeHtml';

	let { data }: { data: PageData } = $props();

	// Map ProfilePost → FeedItem so profile posts render through FeedCard.
	// The profile page's posts all belong to the same user, so the author
	// display name and avatar come from the profile header.
	function toFeedItem(post: ProfilePost): FeedItem {
		return {
			id: post.id,
			title: post.title,
			slug: post.slug,
			content_type: post.content_type,
			author_id: post.author_id,
			author_name: data.user.display_name,
			author_username: data.user.username,
			author_avatar_url: data.user.avatar_url,
			created_at: post.published_at ?? post.created_at,
			view_count: post.view_count,
			comment_count: post.comment_count,
			reaction_count: post.reaction_count,
			bookmark_count: post.bookmark_count,
			reading_time_minutes: post.read_time_minutes,
			excerpt: post.excerpt,
			space_id: post.space_id,
			space_name: post.space_name,
			space_slug: post.space_slug,
			space_avatar_url: post.space_avatar_url,
			organization: post.organization,
			cover_image_url: post.cover_image_url,
			tags: post.tags,
			journey_stage: post.journey_stage,
			visa_route: post.visa_route,
			source_country: post.source_country,
			target_country: post.target_country,
			guide_category: post.guide_category,
			video_source_id: post.video_source_id,
			// PostCard emits the linked video's length as video_duration_secs —
			// FeedItem.duration_secs is what FeedCard's duration badge reads.
			duration_secs: post.video_duration_secs ?? post.duration_secs
		};
	}

	const user = $derived(data.user);
	const currentUser = $derived(data.currentUser);
	const isOwnProfile = $derived(data.isOwnProfile);
	const isAuthenticated = $derived(data.isAuthenticated);

	// §106 video — own video content posts that should also appear on the
	// profile Videos tab alongside imported/uploaded video_sources.
	const videoPosts = $derived(data.posts.filter((p) => p.content_type === 'video'));
	const initiallyFollowing = $derived(data.initiallyFollowing);
	let activeTab = $state(untrack(() => page.url.searchParams.get('tab') ?? 'posts'));
	let reportOpen = $state(false);

	// Per-tab client-side filter / search / pagination over the SSR-loaded data.
	// NOTE: `currentPage` (not `page`) avoids shadowing SvelteKit's `page` from `$app/state`.
	let filterVal = $state('all');
	let currentPage = $state(1);
	let pageSize = $state(10);
	let q = $state('');
	let sortVal = $state<'newest' | 'oldest'>('newest');

	// Reset the filter, search, sort + page whenever the active tab changes.
	// `activeTab` is the only dependency; the writes below must not create
	// reactive deps of their own.
	$effect(() => {
		activeTab;
		filterVal = 'all';
		q = '';
		sortVal = 'newest';
		currentPage = 1;
	});

	// Unknown or unavailable ?tab= values (e.g. ?tab=bogus, or a conditional
	// tab hidden for this profile) would render no panel at all — normalize
	// to Posts so the page always shows content.
	$effect(() => {
		if (activeTab && !tabs.some((tab) => tab.value === activeTab)) {
			activeTab = 'posts';
		}
	});

	// Keep activeTab in sync when ?tab= changes via real navigation — same-route
	// links (sidebar "View all spaces", browser back/forward, shared deep links)
	// reuse the mounted component, so mount-time init alone leaves a stale tab.
	// afterNavigate (not an $effect on page.url) because UrlTabs syncs tab clicks
	// with replaceState — replaceState does not fire afterNavigate, so this never
	// fights the click handler. Reads location.href rather than to.url/page.url:
	// on popstate to a replaceState-written entry, SvelteKit reports the URL from
	// the entry's original navigation (without the shallow ?tab= param), while
	// the browser location already carries the real URL.
	afterNavigate(() => {
		const urlTab = new URL(location.href).searchParams.get('tab') ?? 'posts';
		if (urlTab !== activeTab && tabs.some((tab) => tab.value === urlTab)) {
			activeTab = urlTab;
		}
	});

	// BUG-007: Block/unblock toggle for the profile user. Mirrors the
	// FollowButton pattern — local state seeded from SSR, optimistic update,
	// invalidate the blocked-users list on success.
	let isBlocked = $state(false);
	let blockPending = $state(false);

	// Re-seed isBlocked when the SSR data changes (e.g. navigating to a
	// different user's profile). Using $effect avoids the
	// state_referenced_locally warning and keeps the toggle in sync with
	// the server-side blocked-users check on navigation.
	$effect(() => {
		isBlocked = data.initiallyBlocked;
	});

	async function toggleBlock() {
		if (blockPending) return;
		blockPending = true;
		const wasBlocked = isBlocked;
		isBlocked = !wasBlocked; // optimistic
		try {
			if (wasBlocked) {
				await unblockUser(user.id);
			} else {
				await blockUser(user.id);
			}
			await invalidate('/api/v1/me/blocked-users');
		} catch (e) {
			isBlocked = wasBlocked; // revert on error
		} finally {
			blockPending = false;
		}
	}

	// FE-1: Mute/unmute toggle for the profile user. Mirrors the block
	// toggle pattern — local state seeded from SSR, optimistic update,
	// invalidate the muted-users list on success. Ref: Section 52.
	let isMuted = $state(false);
	let mutePending = $state(false);

	$effect(() => {
		isMuted = data.initiallyMuted;
	});

	async function toggleMute() {
		if (mutePending) return;
		mutePending = true;
		const wasMuted = isMuted;
		isMuted = !wasMuted; // optimistic
		try {
			if (wasMuted) {
				await unmuteUser(user.id);
			} else {
				await muteUser(user.id);
			}
			await invalidate(`/api/v1/users/${user.id}/mute-status`);
		} catch (e) {
			isMuted = wasMuted; // revert on error
		} finally {
			mutePending = false;
		}
	}

	// Phase 34.03 — earned + unearned badges. On the own profile we also have
	// the full catalog (data.allBadges); unearned badges surface a "how to earn"
	// hint via their description. Earned badges are always shown (public route).
	const earnedSlugs = $derived(new Set((data.badges ?? []).map((b) => b.slug)));
	const unearnedBadges = $derived(
		isOwnProfile
			? (data.allBadges ?? []).filter((b) => b.is_active && !earnedSlugs.has(b.slug))
			: []
	);

	const tabs = $derived<UrlTab[]>([
		{ value: 'posts', label: t('profile.tab.posts', 'Posts') },
		{ value: 'about', label: t('profile.tab.about', 'About') },
		{ value: 'replies', label: t('profile.tab.replies', 'Replies') },
		{ value: 'media', label: t('profile.tab.media', 'Media') },
		// §106 video — Videos tab. Always shown for synthetic (imported)
		// channels; for real users only when they have video sources, video
		// posts, or the viewer is the profile owner (so they can see the empty state).
		...(user.is_synthetic || (data.videos && data.videos.length > 0) || videoPosts.length > 0 || isOwnProfile
			? [{ value: 'videos', label: t('profile.tab.videos', 'Videos') }]
			: []),
		{ value: 'likes', label: t('profile.tab.likes', 'Likes') },
		{ value: 'spaces', label: t('profile.tab.spaces', 'Spaces') },
		// §31 — Follower/following lists are public and expose an `is_following`
		// boolean for the caller so the FollowButton can be hydrated.
		{ value: 'followers', label: t('profile.tab.followers', 'Followers') },
		{ value: 'following', label: t('profile.tab.following', 'Following') },
		// Phase 34.03 — Badges tab (Section 64, ADR-080). Shown when the user
		// has earned badges (public) or when viewing own profile (unearned
		// badges from the full catalog are also surfaced with "how to earn"
		// hints).
		...((data.badges && data.badges.length > 0) || unearnedBadges.length > 0
			? [{ value: 'badges', label: t('profile.tab.badges', 'Badges') }]
			: []),
		// Analytics is a self-dashboard — owner-only per JEOS §342 [auth, self].
		// Hidden from guests and from other authenticated users.
		...(isOwnProfile
			? [{ value: 'analytics', label: t('profile.tab.analytics', 'Analytics') }]
			: [])
	]);

	const analytics = $derived(data.analytics as ProfileAnalytics | null);
	const followerGrowth = $derived((data.followerGrowth ?? []) as FollowerGrowthItem[]);
	const topContent = $derived((data.topContent ?? []) as TopContentItem[]);

	// Follower growth series — LineChart of daily new followers over 30 days.
	// Ref: ADR-059, sections/40-platform-analytics-reports.md
	const followerSeries = $derived(
		followerGrowth.map((item, i) => ({
			x: i,
			y: item.new_followers ?? 0,
			label: item.day
		}))
	);

	// Top posts series — vertical BarChart of view_count per content title.
	// Ref: ADR-059
	const topPostsSeries = $derived(
		topContent
			.slice(0, 10)
			.map((item) => ({
				id: item.id,
				x: item.title?.length > 20 ? item.title.slice(0, 20) + '…' : item.title ?? item.id,
				y: item.view_count ?? 0,
				label: item.title?.length > 30 ? item.title.slice(0, 30) + '…' : item.title ?? item.id
			}))
	);

	const displayName = $derived(user.display_name ?? user.username);
	const title = $derived(`${displayName} (@${user.username}) — ${t('app.name')}`);
	const description = $derived(user.bio ?? `${displayName} on ${t('app.name')}`);
	// Section 26 — self-referencing canonical on the clean username URL (never
	// the query-string form) so filtered/paginated hits consolidate.
	const canonical = $derived(`${page.url.origin}/u/${user.username}`);
	// JSON-LD: Person schema (Section 14 / Section 26). The backend resolves
	// the admin-managed Person template server-side and returns it in the
	// `json_ld` sidecar. Fall back to a basic schema when absent.
	const jsonLd = $derived(
		data.jsonLd
			? JSON.stringify(data.jsonLd)
			: JSON.stringify({
					'@context': 'https://schema.org',
					'@type': 'Person',
					name: displayName,
					alternateName: user.username ? `@${user.username}` : undefined,
					url: canonical,
					description: user.bio ?? '',
					...(user.avatar_url ? { image: user.avatar_url } : {})
				})
	);

	// ---- Per-tab list controls: filter + search + pagination -----------------
	// Generic, zero-`any` client-side view over an already-loaded list. Filters by
	// `fv` (skipped when 'all') and by `q` (case-insensitive substring over the
	// joined search fields, skipped when empty), then returns the sliced page
	// plus the total and total page count.
	type FilterOption = { value: string; label: string };
	type PagerView = { total: number; totalPages: number };

	function view<T>(items: T[], opts: {
		searchFields: (keyof T)[];
		filter: (item: T, fv: string) => boolean;
		q: string;
		fv: string;
		currentPage: number;
		pageSize: number;
		/** Optional date field to sort by (ISO string). sortDir 'newest' is
		 *  descending, 'oldest' ascending. Absent = preserve server order. */
		sortField?: keyof T;
		sortDir?: 'newest' | 'oldest';
	}): { items: T[]; total: number; totalPages: number; filtered: T[] } {
		const needle = opts.q.trim().toLowerCase();
		let filtered = items.filter((item) => {
			if (opts.fv !== 'all' && !opts.filter(item, opts.fv)) return false;
			if (needle.length > 0) {
				const hay = opts.searchFields
					.map((f) => (item[f] as unknown as string) ?? '')
					.join(' ')
					.toLowerCase();
				if (!hay.includes(needle)) return false;
			}
			return true;
		});
		if (opts.sortField) {
			const field = opts.sortField;
			const dir = opts.sortDir === 'oldest' ? 1 : -1;
			const ts = (v: unknown): number => {
				const t = Date.parse(typeof v === 'string' ? v : '');
				return Number.isNaN(t) ? 0 : t;
			};
			filtered = filtered.slice().sort((a, b) => dir * (ts(a[field]) - ts(b[field])));
		}
		const total = filtered.length;
		const totalPages = Math.max(1, Math.ceil(total / opts.pageSize));
		const start = (opts.currentPage - 1) * opts.pageSize;
		return { items: filtered.slice(start, start + opts.pageSize), total, totalPages, filtered };
	}

	const pageSizeOptions: FilterOption[] = [
		{ value: '10', label: '10' },
		{ value: '25', label: '25' },
		{ value: '50', label: '50' },
		{ value: '100', label: '100' }
	];

	// Tab dropdowns use the shared Autocomplete component — the option lists are
	// static, so the fetcher just filters them locally.
	function toAutocompleteItems(options: FilterOption[]): AutocompleteItem[] {
		return options.map((o) => ({ id: o.value, label: o.label }));
	}

	function staticFilterFetcher(items: AutocompleteItem[]) {
		return (query: string) => {
			const needle = query.trim().toLowerCase();
			return Promise.resolve(
				needle ? items.filter((i) => i.label.toLowerCase().includes(needle)) : items
			);
		};
	}

	const pageSizeItems = toAutocompleteItems(pageSizeOptions);

	// Per-tab contextual filter option lists, derived from the loaded data.
	const postsFilterOptions = $derived<FilterOption[]>([
		{ value: 'all', label: t('profile.filter.all_types', 'All types') },
		...Array.from(new Set(data.posts.map((p) => p.content_type).filter(Boolean))).map((ct) => ({
			value: ct,
			label: ct
		}))
	]);

	const repliesFilterOptions = $derived<FilterOption[]>([
		{ value: 'all', label: t('profile.filter.all_replies', 'All replies') },
		{ value: 'top', label: t('profile.filter.top_level', 'Top-level') },
		{ value: 'nested', label: t('profile.filter.nested', 'Nested') }
	]);

	const mediaFilterOptions = $derived<FilterOption[]>([
		{ value: 'all', label: t('profile.filter.all_media', 'All media') },
		{ value: 'image', label: t('profile.filter.images', 'Images') },
		{ value: 'video', label: t('profile.filter.videos', 'Videos') },
		{ value: 'audio', label: t('profile.filter.audio', 'Audio') },
		{ value: 'other', label: t('profile.filter.other', 'Other') }
	]);

	const likesFilterOptions = $derived<FilterOption[]>([
		{ value: 'all', label: t('profile.filter.all_types', 'All types') },
		...Array.from(new Set(data.likes.map((l) => l.content_type).filter(Boolean))).map((ct) => ({
			value: ct,
			label: ct
		}))
	]);

	const spacesFilterOptions = $derived<FilterOption[]>([
		{ value: 'all', label: t('profile.filter.all_roles', 'All roles') },
		...Array.from(new Set(data.spaces.map((s) => s.role).filter((r): r is string => Boolean(r)))).map(
			(r) => ({ value: r, label: r.charAt(0).toUpperCase() + r.slice(1) })
		)
	]);

	const followersFilterOptions = $derived<FilterOption[]>([
		{ value: 'all', label: t('profile.filter.all_users', 'All users') }
	]);

	const followingFilterOptions = $derived<FilterOption[]>([
		{ value: 'all', label: t('profile.filter.all_entity', 'All') },
		{ value: 'user', label: t('profile.filter.users', 'Users') },
		{ value: 'space', label: t('profile.filter.spaces', 'Spaces') },
		{ value: 'tag', label: t('profile.filter.tags', 'Tags') },
		{ value: 'organization', label: t('profile.filter.organizations', 'Organizations') },
		{ value: 'source', label: t('profile.filter.sources', 'Sources') }
	]);

	const videosFilterOptions = $derived<FilterOption[]>([
		{ value: 'all', label: t('profile.filter.all_sources', 'All sources') },
		...Array.from(new Set(data.videos.map((v) => v.source_type).filter(Boolean))).map((st) => ({
			value: st,
			label: st
		}))
	]);

	// Shared newest/oldest ordering for the per-tab list sort control.
	const sortOptions = $derived<FilterOption[]>([
		{ value: 'newest', label: t('profile.sort.newest', 'Newest first') },
		{ value: 'oldest', label: t('profile.sort.oldest', 'Oldest first') }
	]);
	const sortItems = $derived(toAutocompleteItems(sortOptions));

	// One derived, sliced view per tab, sharing the q/filterVal/page/pageSize state.
	const postsView = $derived(
		view(data.posts, {
			searchFields: ['title', 'excerpt', 'content_type'],
			filter: (i, fv) => i.content_type === fv,
			q,
			fv: filterVal,
			currentPage,
			pageSize,
			sortField: 'published_at',
			sortDir: sortVal
		})
	);

	// Reply bodies arrive base64-encoded TipTap JSON — searching the raw blob
	// can never match typed text. Decode once into search_text so tab search
	// covers what the user actually sees.
	const repliesDecoded = $derived(
		data.replies.map((r) => ({ ...r, search_text: tiptapToText(decodeTipTapDoc(r.body)) }))
	);

	const repliesView = $derived(
		view(repliesDecoded, {
			searchFields: ['search_text', 'content_title'],
			filter: (i, fv) => (fv === 'top' ? i.parent_id === null : i.parent_id !== null),
			q,
			fv: filterVal,
			currentPage,
			pageSize,
			sortField: 'created_at',
			sortDir: sortVal
		})
	);

	const mediaView = $derived(
		view(data.media, {
			searchFields: ['alt_text', 'filename'],
			filter: (i, fv) => {
				const mt = i.mime_type ?? '';
				if (fv === 'image') return mt.startsWith('image/');
				if (fv === 'video') return mt.startsWith('video/');
				if (fv === 'audio') return mt.startsWith('audio/');
				if (fv === 'other')
					return !mt.startsWith('image/') && !mt.startsWith('video/') && !mt.startsWith('audio/');
				return false;
			},
			q,
			fv: filterVal,
			currentPage,
			pageSize,
			sortField: 'created_at',
			sortDir: sortVal
		})
	);

	const likesView = $derived(
		view(data.likes, {
			searchFields: ['title', 'excerpt', 'author_name', 'content_type'],
			filter: (i, fv) => i.content_type === fv,
			q,
			fv: filterVal,
			currentPage,
			pageSize,
			sortField: 'reacted_at',
			sortDir: sortVal
		})
	);

	const videosView = $derived(
		view(data.videos, {
			searchFields: ['title', 'source_type', 'category'],
			filter: (i, fv) => i.source_type === fv,
			q,
			fv: filterVal,
			currentPage,
			pageSize,
			sortField: 'created_at',
			sortDir: sortVal
		})
	);

	// Spaces have no date field — sort is name A–Z instead of newest/oldest.
	const spacesView = $derived(
		view(data.spaces, {
			searchFields: ['name', 'slug', 'role'],
			filter: (i, fv) => i.role === fv,
			q,
			fv: filterVal,
			currentPage,
			pageSize
		})
	);

	const followersView = $derived(
		view(data.followers, {
			searchFields: ['display_name', 'username'],
			filter: (i, fv) => i.entity_type === fv,
			q,
			fv: filterVal,
			currentPage,
			pageSize,
			sortField: 'created_at',
			sortDir: sortVal
		})
	);

	const followingView = $derived(
		view(data.following, {
			searchFields: ['display_name', 'username'],
			filter: (i, fv) => i.entity_type === fv,
			q,
			fv: filterVal,
			currentPage,
			pageSize,
			sortField: 'created_at',
			sortDir: sortVal
		})
	);
</script>

<svelte:head>
	<title>{title}</title>
	<meta name="description" content={description} />
	<meta property="og:title" content={title} />
	<meta property="og:description" content={description} />
	<meta property="og:type" content="profile" />
	<link rel="canonical" href={canonical} />
	<meta property="og:url" content={canonical} />
	<!-- §26 — per-user RSS autodiscovery (proxied to the backend profile feed). -->
	<link
		rel="alternate"
		type="application/rss+xml"
		title={`${displayName} — ${t('app.name')}`}
		href="/u/{user.username}/feed.xml"
	/>
	{@html safeJsonLdScript(jsonLd)}
</svelte:head>

<SetSidebar>
	<UserProfileSidebar
		certificates={data.certificates}
		spaces={data.spaces}
	/>
</SetSidebar>

<ProfileHeader {user} {isOwnProfile} followers={data.followers} following={data.following}>
	{#if !isOwnProfile}
		<FollowButton
			entityType="user"
			entityId={user.id}
			{initiallyFollowing}
			onChange={() => invalidate(`/api/v1/users/${user.username}`)}
		/>
	{/if}
	{#snippet menuItems()}
		{#if isAuthenticated && !isOwnProfile}
			<MenuItem icon={VolumeX} disabled={mutePending} onSelect={toggleMute}>
				{isMuted ? t('profile.unmute', 'Unmute') : t('profile.mute', 'Mute')}
			</MenuItem>
			<MenuItem icon={Ban} destructive disabled={blockPending} onSelect={toggleBlock}>
				{isBlocked ? t('profile.unblock', 'Unblock') : t('profile.block', 'Block')}
			</MenuItem>
			<MenuItem icon={Flag} onSelect={() => (reportOpen = true)}>
				{t('profile.report', 'Report')}
			</MenuItem>
		{/if}
	{/snippet}
</ProfileHeader>

<ReportDialog bind:open={reportOpen} entityType="user" entityId={user.id} entityName={user.display_name ?? user.username} />


<!-- §87 — Japa Stage badge. Shown when the profile user has a public stage
set. Best-effort: data.japaStage is null when the feature flag is off, the
profile is private, or no stage is set. -->
{#if data.japaStage?.stage}
	<div class="mx-auto w-full max-w-full px-3 pt-3">
		<a
			href={`/japa-stage?stage=${data.japaStage.stage}`}
			class="inline-flex items-center gap-2 rounded-full border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-1.5 text-sm transition-colors hover:bg-[var(--color-surface-subtle)]"
		>
			<Compass class="h-4 w-4 text-[var(--color-primary)]" />
			<span class="font-medium text-[var(--color-text)]">
				{t(`japa_stage.${data.japaStage.stage}`, data.japaStage.stage)}
			</span>
			{#if data.japaStage.destination_space_name}
				<span class="text-[var(--color-text-subtle)]">
					{t('japa_stage.dest', 'Destination')}: {data.japaStage.destination_space_name}
				</span>
			{/if}
		</a>
	</div>
{/if}

<UrlTabs {tabs} bind:value={activeTab} />

<div class="mx-auto w-full max-w-full">
	{#snippet toolbar(options: FilterOption[], count: number, sortable = false)}
		<!-- Search + filter are only useful past one default page of items.
		     A single-option filter dropdown (e.g. followers = "All users")
		     adds no value, so it renders only when a real choice exists. -->
		{#if count > 10}
		{@const items = toAutocompleteItems(options)}
		<div class="flex flex-wrap items-center justify-between gap-3 mb-4">
			<div class="flex flex-wrap items-center gap-2">
				{#if options.length > 1}
					<div class="w-full sm:w-44">
						<Autocomplete
							placeholder={t('profile.filter.label', 'Filter')}
							emptyMessage={t('profile.empty.no_results', 'No results')}
							fetcher={staticFilterFetcher(items)}
							initialFetcher={() => Promise.resolve(items)}
							selectedId={filterVal}
							selectedLabel={options.find((o) => o.value === filterVal)?.label}
							onSelect={(item) => {
								filterVal = item.id;
								currentPage = 1;
							}}
							onClear={() => {
								filterVal = 'all';
								currentPage = 1;
							}}
						/>
					</div>
				{/if}
				{#if sortable}
					<div class="w-full sm:w-40">
						<Autocomplete
							placeholder={t('profile.sort.label', 'Sort')}
							ariaLabel={t('profile.sort.label', 'Sort')}
							emptyMessage={t('profile.empty.no_results', 'No results')}
							fetcher={staticFilterFetcher(sortItems)}
							initialFetcher={() => Promise.resolve(sortItems)}
							selectedId={sortVal}
							selectedLabel={sortOptions.find((o) => o.value === sortVal)?.label}
							onSelect={(item) => {
								sortVal = item.id === 'oldest' ? 'oldest' : 'newest';
								currentPage = 1;
							}}
							onClear={() => {
								sortVal = 'newest';
								currentPage = 1;
							}}
						/>
					</div>
				{/if}
			</div>
			<div class="w-full sm:w-64">
				<SearchInput
					bind:value={q}
					onInput={() => {
						currentPage = 1;
					}}
					fetcher={() => Promise.resolve([])}
					placeholder={t('profile.search.placeholder', 'Search')}
					ariaLabel={t('profile.search.placeholder', 'Search')}
				/>
			</div>
		</div>
		{/if}
	{/snippet}

	{#snippet pager(v: PagerView)}
		{#if v.total > 10}
			<div class="mt-4 flex flex-wrap items-center justify-between gap-3">
				<div class="flex items-center gap-2">
					<div class="w-24 shrink-0">
						<Autocomplete
							placeholder={t('profile.pager.page_size', 'Per page')}
							emptyMessage={t('profile.empty.no_results', 'No results')}
							fetcher={staticFilterFetcher(pageSizeItems)}
							initialFetcher={() => Promise.resolve(pageSizeItems)}
							selectedId={String(pageSize)}
							selectedLabel={String(pageSize)}
							onSelect={(item) => {
								pageSize = Number(item.id);
								currentPage = 1;
							}}
							onClear={() => {
								pageSize = 10;
								currentPage = 1;
							}}
						/>
					</div>
					<span class="text-xs text-[var(--color-muted)]">
						{t('profile.pager.showing', 'Showing {start}–{end} of {total}')
							.replace('{start}', String((currentPage - 1) * pageSize + 1))
							.replace('{end}', String(Math.min(currentPage * pageSize, v.total)))
							.replace('{total}', String(v.total))}
					</span>
				</div>
				<PaginationControls page={currentPage} totalPages={v.totalPages} onPageChange={(p) => { currentPage = p; }} />
			</div>
		{/if}
	{/snippet}

	<div class="p-3">
		{#if activeTab === 'posts'}
			{@render toolbar(postsFilterOptions, data.posts.length, true)}
			{#if data.posts.length === 0}
				<EmptyState icon={FileText} title={t('profile.posts_empty', 'No posts yet')} />
			{:else if postsView.total === 0}
				<EmptyState icon={FileText} title={t('profile.empty.no_results', 'No results')} />
			{:else}
				<div class="flex flex-col gap-3">
					{#each postsView.items as post (post.id)}
						<div class="group relative block">
							<FeedCard
								item={toFeedItem(post)}
								style="daily"
								layout="stream"
							/>
							{#if post.is_pinned}
								<span class="pointer-events-none absolute left-2 top-2 z-10 flex items-center gap-0.5 rounded-full bg-[var(--color-primary)]/10 px-2 py-0.5 text-[10px] font-medium text-[var(--color-primary)]">
									<Pin size={10} /> {t('profile.pinned', 'Pinned')}
								</span>
							{/if}
						</div>
					{/each}
				</div>
				{@render pager(postsView)}
			{/if}
		{:else if activeTab === 'about'}
			<!-- Phase 11.06 — About tab -->
			<section class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
				<h2 class="mb-3 text-sm font-medium text-[var(--color-text)]">
					{t('profile.about', 'About')}
				</h2>
				{#if user.bio}
					<p class="text-sm leading-relaxed text-[var(--color-text)]">{user.bio}</p>
				{:else}
					<EmptyState icon={Info} title={t('profile.no_about', 'No bio yet')} />
				{/if}
				<dl class="mt-4 space-y-2 border-t border-[var(--color-border)] pt-4">
					<div class="flex justify-between gap-4">
						<dt class="text-sm text-[var(--color-muted)]">{t('profile.joined', 'Joined')}</dt>
						<dd class="text-sm text-[var(--color-text)]">
							{new Date(user.created_at).toLocaleDateString(undefined, { year: 'numeric', month: 'long' })}
						</dd>
					</div>
					<div class="flex justify-between gap-4">
						<dt class="text-sm text-[var(--color-muted)]">{t('profile.followers_label', 'Followers')}</dt>
						<dd class="text-sm text-[var(--color-text)]">{user.follower_count}</dd>
					</div>
					<div class="flex justify-between gap-4">
						<dt class="text-sm text-[var(--color-muted)]">{t('profile.following_label', 'Following')}</dt>
						<dd class="text-sm text-[var(--color-text)]">{user.following_count}</dd>
					</div>
					{#if user.points != null}
						<!-- Section 28 — Points shown only when the user has opted in via show_points_publicly. Credits are never exposed. -->
						<div class="flex justify-between gap-4">
							<dt class="text-sm text-[var(--color-muted)]">{t('profile.points_label', 'Points')}</dt>
							<dd class="text-sm text-[var(--color-text)]">{user.points.toLocaleString()}</dd>
						</div>
					{/if}
				</dl>
			</section>
		{:else if activeTab === 'replies'}
			{@render toolbar(repliesFilterOptions, data.replies.length, true)}
			{#if data.replies.length === 0}
				<EmptyState icon={MessageSquare} title={t('profile.replies_empty', 'No replies yet')} />
			{:else if repliesView.total === 0}
				<EmptyState icon={MessageSquare} title={t('profile.empty.no_results', 'No results')} />
			{:else}
				<ul class="space-y-2">
					{#each repliesView.items as reply (reply.id)}
						<li class="rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
							<p class="text-sm text-[var(--color-text)]">{tiptapExcerpt(reply.body)}</p>
							<a href="/content/{reply.content_slug}" class="mt-2 block text-xs text-[var(--color-primary)] hover:underline">
								{t('profile.in_reply_to', 'In reply to')}: {reply.content_title}
							</a>
						</li>
					{/each}
				</ul>
				{@render pager(repliesView)}
			{/if}
		{:else if activeTab === 'media'}
			{@render toolbar(mediaFilterOptions, data.media.length, true)}
			{#if data.media.length === 0}
				<EmptyState icon={Image} title={t('profile.media_empty', 'No media yet')} />
			{:else if mediaView.total === 0}
				<EmptyState icon={Image} title={t('profile.empty.no_results', 'No results')} />
			{:else}
				<div class="grid grid-cols-2 gap-2 sm:grid-cols-3">
					{#each mediaView.items as item (item.id)}
						<button
							type="button"
							class="block aspect-square cursor-zoom-in overflow-hidden rounded-[var(--radius)] border border-[var(--color-border)]"
							onclick={() => openLightbox(mediaView.filtered, mediaView.filtered.indexOf(item))}
						>
							<img src={item.url} alt={item.alt_text ?? ''} class="h-full w-full object-cover" loading="lazy" />
						</button>
					{/each}
				</div>
				{@render pager(mediaView)}
			{/if}
		{:else if activeTab === 'videos'}
			<!-- §106 video — Videos tab. Synthetic channels surface their banner
			and location metadata above the grid when present. -->
			{#if user.is_synthetic && (user.banner_url || user.location)}
				<div class="mb-4 overflow-hidden rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)]">
					{#if user.banner_url}
						<div class="h-28 overflow-hidden sm:h-36">
							<img src={user.banner_url} alt="" class="h-full w-full object-cover" loading="lazy" />
						</div>
					{/if}
					<div class="flex flex-wrap items-center gap-3 p-3 text-sm text-[var(--color-muted)]">
						{#if user.synthetic_source}
							<span class="inline-flex items-center gap-1 rounded border border-[var(--color-border)] bg-[var(--color-surface-subtle)] px-1.5 py-0.5 text-[10.5px] font-semibold uppercase tracking-wide text-[var(--color-text-subtle)]">
								<Video size={12} /> {user.synthetic_source}
							</span>
						{/if}
						{#if user.location}
							<span class="inline-flex items-center gap-1"><MapPin size={14} /> {user.location}</span>
						{/if}
					</div>
				</div>
			{/if}
			{#if user.is_synthetic && isAuthenticated && !isOwnProfile}
				<!-- §106 — imported channels can be claimed or reported by their
				real owner. This is the only entry point to /u/[username]/claim. -->
				<a
					href="/u/{user.username}/claim"
					class="mb-4 inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-1.5 text-sm text-[var(--color-muted)] transition-colors hover:text-[var(--color-text)] hover:border-[var(--color-border-strong)]"
				>
					<Flag size={14} />
					{t('profile.claim_channel', 'Is this your channel? Claim or report it')}
				</a>
			{/if}
			{#if data.videos.length === 0 && videoPosts.length === 0}
				<EmptyState icon={Video} title={t('profile.videos_empty', 'No videos yet')} />
			{:else}
				{#if data.videos.length > 0}
					{@render toolbar(videosFilterOptions, data.videos.length, true)}
					{#if videosView.total === 0}
						<EmptyState icon={Video} title={t('profile.empty.no_results', 'No results')} />
					{:else}
						<VideoGrid videos={videosView.items} isChannel={user.is_synthetic ?? false} />
						{@render pager(videosView)}
					{/if}
				{/if}

				<!-- §106 video — real users' own video content posts (content_type='video')
				     are also listed on the Videos tab. They link to /video/{slug}. -->
				{#if videoPosts.length > 0}
					{#if data.videos.length > 0}
						<h2 class="mb-3 mt-6 text-sm font-semibold text-[var(--color-text)]">
							{t('profile.video_posts', 'Video posts')}
						</h2>
					{/if}
					<div class="flex flex-col gap-3">
						{#each videoPosts as post (post.id)}
							<FeedCard
								item={toFeedItem(post)}
								style="daily"
								layout="stream"
							/>
						{/each}
					</div>
				{/if}
			{/if}
		{:else if activeTab === 'likes'}
			{@render toolbar(likesFilterOptions, data.likes.length, true)}
			{#if data.likes.length === 0}
				<EmptyState icon={Heart} title={t('profile.likes_empty', 'No liked posts yet')} />
			{:else if likesView.total === 0}
				<EmptyState icon={Heart} title={t('profile.empty.no_results', 'No results')} />
			{:else}
				<ul class="space-y-2">
					{#each likesView.items as item (item.id)}
						<li>
							<a href="/content/{item.slug}" class="block rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-3 transition-colors hover:bg-[var(--color-surface-subtle)]">
								<span class="text-xs text-[var(--color-muted)]">{item.content_type}</span>
								<h3 class="mt-1 text-sm font-medium text-[var(--color-text)]">{item.title}</h3>
								{#if item.excerpt}
									<p class="mt-1 line-clamp-2 text-xs text-[var(--color-muted)]">{item.excerpt}</p>
								{/if}
								<div class="mt-1.5 flex items-center gap-2 text-xs text-[var(--color-muted)]">
									{#if item.author_name}<span>{item.author_name}</span>{/if}
									<span>· {item.view_count} views</span>
								</div>
							</a>
						</li>
					{/each}
				</ul>
				{@render pager(likesView)}
			{/if}
		{:else if activeTab === 'spaces'}
			{@render toolbar(spacesFilterOptions, data.spaces.length, false)}
			{#if data.spaces.length === 0}
				<EmptyState icon={Users} title={t('profile.spaces_empty', 'No spaces yet')} />
			{:else if spacesView.total === 0}
				<EmptyState icon={Users} title={t('profile.empty.no_results', 'No results')} />
			{:else}
				<ul class="grid grid-cols-1 gap-2 sm:grid-cols-2">
					{#each spacesView.items as space (space.id)}
						<li>
							<a
								href={`/spaces/${space.slug}`}
								class="flex items-center gap-3 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-3 transition-colors hover:bg-[var(--color-surface-subtle)]"
							>
								<span class="grid h-10 w-10 shrink-0 place-items-center rounded-[var(--radius)] bg-[var(--color-surface-subtle)] text-sm font-medium text-[var(--color-text-subtle)]">{space.name.charAt(0).toUpperCase()}</span>
								<span class="min-w-0 flex-1">
									<span class="block truncate text-sm font-medium text-[var(--color-text)]">{space.name}</span>
									{#if space.role}
										<span class="block text-xs capitalize text-[var(--color-muted)]">{space.role}</span>
									{/if}
								</span>
							</a>
						</li>
					{/each}
				</ul>
				{@render pager(spacesView)}
			{/if}
		{:else if activeTab === 'followers' || activeTab === 'following'}
			{@const isFollowers = activeTab === 'followers'}
			{@const list = isFollowers ? data.followers : data.following}
			{@const tabView = isFollowers ? followersView : followingView}
			{@const emptyTitle = isFollowers ? t('profile.followers_empty', 'No followers yet') : t('profile.following_empty', 'Not following anyone yet')}
			{@render toolbar(isFollowers ? followersFilterOptions : followingFilterOptions, list.length, true)}
			{#if list.length === 0}
				<EmptyState icon={Users} title={emptyTitle} />
			{:else if tabView.total === 0}
				<EmptyState icon={Users} title={t('profile.empty.no_results', 'No results')} />
			{:else}
				<ul class="grid grid-cols-1 gap-2 sm:grid-cols-2">
					{#each tabView.items as u (u.id)}
						{@const et = u.entity_type ?? 'user'}
						{@const followType = et as FollowableType}
						{@const href =
							et === 'space' ? `/spaces/${u.username}` :
							et === 'tag' ? `/tags/${u.username}` :
							et === 'organization' ? `/org/${u.username}` :
							et === 'source' ? `/sources/${u.username}` :
							`/u/${u.username}`}
						{@const handle =
							et === 'user' ? `@${u.username}` :
							et === 'space' ? t('profile.type.space', 'Space') :
							et === 'tag' ? t('profile.type.tag', 'Tag') :
							et === 'organization' ? t('profile.type.organization', 'Organization') :
							et === 'source' ? t('profile.type.source', 'Source') :
							t('profile.type.user', 'User')}
						<li class="flex items-center gap-3 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-3">
							<a href={href} class="flex min-w-0 flex-1 items-center gap-3">
								{#if u.avatar_url}
									<img src={u.avatar_url} alt={u.display_name ?? u.username} class="h-10 w-10 rounded-full object-cover" />
								{:else}
									<span class="grid h-10 w-10 shrink-0 place-items-center rounded-full bg-[var(--color-surface-subtle)] text-sm font-medium text-[var(--color-text-subtle)]">
										{(u.display_name?.charAt(0) ?? u.username?.charAt(0) ?? '?').toUpperCase()}
									</span>
								{/if}
								<span class="min-w-0 flex-1">
									<span class="block truncate text-sm font-medium text-[var(--color-text)]">{u.display_name ?? u.username}</span>
									<span class="block text-xs text-[var(--color-muted)]">{handle}</span>
								</span>
							</a>
							{#if !isOwnProfile && u.id !== currentUser?.id && isAuthenticated}
								<FollowButton entityType={followType} entityId={u.id} entitySlug={u.username} initiallyFollowing={u.is_following ?? false} />
							{/if}
						</li>
					{/each}
				</ul>
				{@render pager(tabView)}
			{/if}
		{:else if activeTab === 'badges'}
			<!-- Phase 34.03 — Badges tab (Section 64, ADR-080). Earned badges
			are public; unearned badges with "how to earn" hints are shown only
			on the own profile (data.allBadges is owner-only). -->
			{#if (data.badges && data.badges.length > 0) || unearnedBadges.length > 0}
				<div class="space-y-4">
					{#if data.badges && data.badges.length > 0}
						<div>
							<h3 class="mb-2 flex items-center gap-1.5 text-xs font-medium uppercase tracking-wide text-[var(--color-muted)]">
								<Award size={12} /> {t('profile.earned', 'Earned')}
							</h3>
							<div class="grid grid-cols-2 gap-2 sm:grid-cols-3 lg:grid-cols-4">
								{#each data.badges as badge (badge.id)}
									<div
										class="flex items-center gap-2 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-2.5 py-2"
										title={badge.description}
									>
										<span class="grid h-7 w-7 shrink-0 place-items-center rounded-full bg-[var(--color-surface-subtle)] text-[var(--color-primary)]">
											<Award size={14} />
										</span>
										<span class="min-w-0 flex-1">
											<span class="block truncate text-xs font-medium text-[var(--color-text)]">{badge.name}</span>
											<span class="block text-[10px] text-[var(--color-muted)]">
												{t('profile.earned', 'Earned')} {new Date(badge.awarded_at).toLocaleDateString(undefined, { year: 'numeric', month: 'short' })}
											</span>
										</span>
									</div>
								{/each}
							</div>
						</div>
					{/if}
					{#if unearnedBadges.length > 0}
						<div>
							<h3 class="mb-2 flex items-center gap-1.5 text-xs font-medium uppercase tracking-wide text-[var(--color-muted)]">
								<Award size={12} /> {t('profile.locked', 'Locked')}
							</h3>
							<div class="grid grid-cols-2 gap-2 sm:grid-cols-3 lg:grid-cols-4">
								{#each unearnedBadges as badge (badge.id)}
									<div
										class="flex items-center gap-2 rounded-[var(--radius)] border border-dashed border-[var(--color-border)] bg-[var(--color-surface-subtle)] px-2.5 py-2 opacity-70"
										title={t('profile.how_to_earn', 'How to earn') + ': ' + badge.description}
									>
										<span class="grid h-7 w-7 shrink-0 place-items-center rounded-full bg-[var(--color-surface)] text-[var(--color-muted)]">
											<Award size={14} />
										</span>
										<span class="min-w-0 flex-1">
											<span class="block truncate text-xs font-medium text-[var(--color-muted)]">{badge.name}</span>
											<span class="block text-[10px] text-[var(--color-muted)]">{t('profile.locked', 'Locked')}</span>
										</span>
									</div>
								{/each}
							</div>
						</div>
					{/if}
				</div>
			{:else}
				<EmptyState
					icon={Award}
					title={t('profile.badges', 'Badges')}
					body={t('profile.no_badges', 'No badges yet')}
				/>
			{/if}
		{:else if activeTab === 'analytics'}
			<!-- Phase 11.07 — Analytics tab with LayerChart visualizations (ADR-059) -->
			{#if analytics}
				<div class="space-y-4">
					<!-- Follower growth — LineChart (ADR-059) -->
					<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
						<h3 class="mb-3 flex items-center gap-1.5 text-sm font-medium text-[var(--color-text)]">
							<BarChart3 size={14} /> {t('dashboard.analytics.follower_growth', 'Follower growth')}
						</h3>
						{#if followerSeries.length > 0}
							<div class="h-56">
								<LineChart
									data={followerSeries}
									x="x"
									y="y"
									props={{
										line: { class: 'stroke-[var(--chart-2,var(--color-primary))]' },
										area: { class: 'fill-[var(--chart-2,var(--color-primary))]/10' }
									}}
								/>
							</div>
							<p class="mt-2 text-xs text-[var(--color-muted)]">
								{t('dashboard.analytics.last_30_days', 'Last 30 days')} ·
								{followerSeries.reduce((sum, d) => sum + d.y, 0)}
								{t('dashboard.analytics.new_followers', 'new followers')}
							</p>
						{:else}
							<p class="py-8 text-center text-sm text-[var(--color-muted)]">
								{t('dashboard.analytics.no_growth_data', 'No follower growth data yet.')}
							</p>
						{/if}
					</div>

					<!-- Top posts — vertical BarChart (ADR-059) -->
					<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
						<h3 class="mb-3 flex items-center gap-1.5 text-sm font-medium text-[var(--color-text)]">
							<FileText size={14} /> {t('dashboard.analytics.top_posts', 'Top posts')}
						</h3>
						{#if topPostsSeries.length > 0}
							<div class="h-64">
								<Chart data={topPostsSeries} x="x" y="y" bandPadding={0.3} padding={{ left: 48, bottom: 60 }}>
									<Svg>
										<Axis placement="left" ticks={4} grid rule />
										<Axis placement="bottom" format={(v: string) => v} />
										<Bars
											fill="var(--chart-1, var(--color-primary))"
											stroke="var(--color-surface)"
											strokeWidth={1}
											radius={4}
										/>
									</Svg>
								</Chart>
							</div>
							<div class="mt-2 space-y-1 text-xs text-[var(--color-muted)]">
								{#each topPostsSeries as point (point.id)}
									<div class="flex items-center justify-between">
										<span class="truncate">{point.label}</span>
										<span class="shrink-0 pl-2">{formatMetric(point.y)} {t('profile.views', 'views')}</span>
									</div>
								{/each}
							</div>
						{:else}
							<p class="py-8 text-center text-sm text-[var(--color-muted)]">
								{t('dashboard.analytics.no_content', 'No published content yet.')}
							</p>
						{/if}
					</div>

					<!-- Stat cards — summary metrics -->
					<div class="grid grid-cols-2 gap-3 sm:grid-cols-3">
						<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
							<div class="flex items-center gap-2 text-xs text-[var(--color-muted)]">
								<BarChart3 size={14} /> {t('profile.analytics.views', 'Total views')}
							</div>
							<p class="mt-1 text-lg font-semibold text-[var(--color-text)]">{formatMetric(analytics.total_views ?? 0)}</p>
						</div>
						<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
							<div class="flex items-center gap-2 text-xs text-[var(--color-muted)]">
								<Heart size={14} /> {t('profile.analytics.likes', 'Total likes')}
							</div>
							<p class="mt-1 text-lg font-semibold text-[var(--color-text)]">{formatMetric(analytics.total_likes ?? 0)}</p>
						</div>
						<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
							<div class="flex items-center gap-2 text-xs text-[var(--color-muted)]">
								<FileText size={14} /> {t('profile.analytics.posts', 'Total posts')}
							</div>
							<p class="mt-1 text-lg font-semibold text-[var(--color-text)]">{formatMetric(analytics.total_posts ?? 0)}</p>
						</div>
						<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
							<div class="flex items-center gap-2 text-xs text-[var(--color-muted)]">
								<Users size={14} /> {t('profile.analytics.followers', 'Followers')}
							</div>
							<p class="mt-1 text-lg font-semibold text-[var(--color-text)]">{formatMetric(analytics.total_followers ?? 0)}</p>
						</div>
						<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
							<div class="flex items-center gap-2 text-xs text-[var(--color-muted)]">
								<Users size={14} /> {t('profile.analytics.following', 'Following')}
							</div>
							<p class="mt-1 text-lg font-semibold text-[var(--color-text)]">{formatMetric(analytics.total_following ?? 0)}</p>
						</div>
						<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
							<div class="flex items-center gap-2 text-xs text-[var(--color-muted)]">
								<FileText size={14} /> {t('profile.analytics.shares', 'Total shares')}
							</div>
							<p class="mt-1 text-lg font-semibold text-[var(--color-text)]">{formatMetric(analytics.total_shares ?? 0)}</p>
						</div>
					</div>
				</div>
			{:else}
				<EmptyState icon={BarChart3} title={t('profile.analytics.unavailable', 'Analytics unavailable')} />
			{/if}
		{/if}
	</div>
</div>
