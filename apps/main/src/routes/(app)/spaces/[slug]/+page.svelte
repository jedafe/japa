<script lang="ts">
	import { untrack } from 'svelte';
	import { invalidateAll } from '$app/navigation';
	import { t } from '#lib/i18n/index.js';
	import UserHoverCard from '#lib/components/ui/UserHoverCard.svelte';
	import VerifiedBadge from '#lib/components/ui/VerifiedBadge.svelte';
	import { page } from '$app/state';
	import {
		Rss, MessageSquare, Users, Info, Search, BarChart3,
		Loader2, Pin, PinOff,
		Check, X, Inbox, ChevronUp, ChevronDown, BookOpen, Star,
		CalendarDays, Layers, ArrowRight, UserPlus
	, TriangleAlert, Radar } from 'lucide-svelte';
	import ReportDialog from '#lib/components/profile/ReportDialog.svelte';
	import SetSidebar from '#lib/components/shell/SetSidebar.svelte';
	import SpaceSidebar from '#lib/components/spaces/SpaceSidebar.svelte';
	import SpaceHeader from '#lib/components/spaces/SpaceHeader.svelte';
	import SpaceComposer from '#lib/components/spaces/SpaceComposer.svelte';
	import SpaceSettings from '#lib/components/spaces/SpaceSettings.svelte';
	import SpaceCreationModal from '#lib/components/spaces/SpaceCreationModal.svelte';
	import InviteModal from '#lib/components/spaces/InviteModal.svelte';
	import { EmptyState, UrlTabs, DirectoryCard } from '#lib/components/ui/index.js';
	import SpaceJoinButton from '#lib/components/ui/SpaceJoinButton.svelte';
	import RadarItemRow from '#lib/components/RadarItemRow.svelte';
	import type { UrlTab } from '#lib/components/ui/index.js';
	import { api } from '#lib/api/client.js';
	import { imgLightbox } from '#lib/lightbox.svelte.js';
	import { requireAuthOrRedirect } from '#lib/auth/guest.js';
	import { decodeBase64Text } from '#lib/tiptap/serialize.js';
	import type { FeedItem } from '#lib/types/index.js';
	import FeedCard from '#lib/components/feed/FeedCard.svelte';
	import NativeAd from '#lib/components/ads/NativeAd.svelte';
	import DigestWidget from '#lib/components/digest/DigestWidget.svelte';
	import { breadcrumbJsonLd } from '#lib/seo/breadcrumbs.js';
	import { categoryIconResolved, categoryTint } from '#lib/components/wiki/icons.js';
	import type {
		Space, SpaceContentItem, SpaceMember, SpaceModerator, SpaceRule,
		SpacePermissionPreset, SpaceJoinRequest, WikiPageIndex, SpaceReview,
		CalendarEntry, RadarItem, SpaceInvite, MySpaceInvite
	} from '#lib/types/index.js';
	import type { PageData } from './$types';
import { safeJsonLdScript } from '#lib/utils/safeHtml.js';
import { flagUrl } from '#lib/utils/flags.js';
import { ogCardUrl, ogMediaUrl } from '#lib/seo/og.js';
	import { remoteImageUrl } from '#lib/utils/remoteImage.js';

	let { data }: { data: PageData } = $props();

	const space = $derived(data.space);

	// BUG-021: §81 sticky warnings — confirmed scam reports pinned to this
	// space surface as a banner above the tabs (list already returns
	// pinned-first; we only need the pinned ones).
	let pinnedScamAlerts = $state<{ id: string; scam_type: string; description: string }[]>([]);
	$effect(() => {
		const sid = space?.id;
		if (!sid || !page.data.features?.['scam_alert.enabled']) return;
		api
			.get<{ id: string; scam_type: string; description: string; is_pinned: boolean }[]>(
				`/scam-alerts?dest=${sid}&limit=10`
			)
			.then((res) => {
				if (!res.error && res.data) pinnedScamAlerts = res.data.filter((a) => a.is_pinned);
			});
	});
	// Country spaces use the bundled flag as their identity artwork —
	// logo_url remains for other space types and unresolvable codes.
	const spaceLogo = $derived(
		space?.level === 'country' ? (flagUrl(space.geo_code) ?? data.logoUrl) : data.logoUrl
	);
	// og:image — cover banner, else raster logo, else the generated space
	// title card (ogMediaUrl drops SVG/data: — share cards ignore them).
	const ogImage = $derived(
		ogMediaUrl(page.url.origin, data.headerUrl) ||
		ogMediaUrl(page.url.origin, data.logoUrl) ||
		ogCardUrl(page.url.origin, `space:${space?.slug ?? page.params.slug}`)
	);
	const authed = $derived(!!page.data.user);
	// Native ad insertion interval — public system setting (§25, Rule 3).
	const adInterval = $derived(
		Math.max(0, Number(page.data.siteSettings?.['ads.native_feed_interval'] ?? 5) || 0)
	);
	const wikiEnabled = $derived(!!data.wikiEnabled);
	const hasWiki = $derived(!!data.hasWiki);
	const wikiPages = $derived<WikiPageIndex[]>(data.wikiPages ?? []);
	const isUniversity = $derived(data.isUniversity ?? false);
	// §05 hub-redesign — tab gating mirrors the server load: geo covers
	// country/state/city; primary = platform-run (geo ∪ university ∪
	// explicitly marked); custom = member-created.
	const isGeo = $derived(data.isGeo ?? false);
	const isPrimary = $derived(data.isPrimary ?? false);
	// Geo spaces are follow-only surfaces; the composer/Post CTA hide on them.
	const isGeoSpace = $derived(isGeo);
	const reviews = $derived<SpaceReview[]>(data.reviews ?? []);
	const events = $derived<CalendarEntry[]>(data.events?.items ?? []);
	const eventsTotal = $derived(data.events?.total ?? null);
	const subspaces = $derived<Space[]>(data.subspaces ?? []);
	const leaders = $derived(data.leaders ?? []);
	const contributors = $derived(data.contributors ?? []);
	// Header meta: "Lagos · Education" — space-type name (parents joined
	// into locationLine further down, after the state declarations).
	const typeName = $derived(
		data.spaceTypes?.find((s) => s.id === space.space_type_id)?.name ?? ''
	);

	function formatWikiCategory(category: string): string {
		return category.replace(/_/g, ' ').replace(/\b\w/g, (l) => l.toUpperCase());
	}

	// §80: the About tab lists wiki pages grouped by category in the same
	// canonical order /wiki/{space_slug} uses for its section cards — the
	// scoped set comes from GET /wiki/categories?space={slug}; featured
	// pages lead each group. Directory categories with no descendants are
	// hidden, and stub-only categories keep a link to their stub page.
	type WikiApiCategory = {
		slug: string;
		label_key: string;
		label: string;
		page_slug?: string;
		directory?: boolean;
		empty?: boolean;
	};
	const apiCategories = $derived<WikiApiCategory[]>(data.wikiCategories ?? []);
	const wikiGroups = $derived.by(() => {
		const hiddenDirCats = new Set(
			apiCategories.filter((c) => c.directory && c.empty).map((c) => c.slug)
		);
		const order = new Map(apiCategories.map((c, i) => [c.slug, i]));
		const groups = new Map<string, WikiPageIndex[]>();
		for (const p of wikiPages) {
			if (hiddenDirCats.has(p.category)) continue;
			const arr = groups.get(p.category);
			if (arr) arr.push(p);
			else groups.set(p.category, [p]);
		}
		const cats = new Set(groups.keys());
		for (const c of apiCategories) {
			if (c.page_slug && !hiddenDirCats.has(c.slug)) cats.add(c.slug);
		}
		return [...cats]
			.map((category) => {
				const pages = [...(groups.get(category) ?? [])].sort(
					(a, b) => Number(b.is_featured) - Number(a.is_featured)
				);
				const apiCat = apiCategories.find((c) => c.slug === category);
				return {
					category,
					label: t(
						apiCat?.label_key ?? `wiki.category_${category}`,
						apiCat?.label ?? formatWikiCategory(category)
					),
					pages,
					pageSlug: apiCat?.page_slug
				};
			})
			.sort((a, b) => {
				const ca = order.get(a.category) ?? Number.MAX_SAFE_INTEGER;
				const cb = order.get(b.category) ?? Number.MAX_SAFE_INTEGER;
				return ca !== cb ? ca - cb : a.label.localeCompare(b.label);
			});
	});

	// Map SpaceContentItem → FeedItem so space feed renders through FeedCard.
	function toFeedItem(item: SpaceContentItem): FeedItem {
		return {
			id: item.id,
			title: item.title,
			slug: item.slug,
			content_type: item.content_type,
			author_id: item.author_id,
			author_name: item.author_name || item.author_display_name || item.author_username || null,
			author_username: item.author_username ?? null,
			author_avatar_url: item.author_avatar_url || null,
			author_verified: item.author_verified,
			author_verification_type: item.author_verification_type,
			organization: item.organization ?? null,
			created_at: item.published_at,
			published_at: item.published_at,
			excerpt: item.excerpt,
			cover_image_url: item.cover_image_url ?? null,
			view_count: item.view_count,
			comment_count: item.comment_count,
			reaction_count: item.reaction_count,
			bookmark_count: item.bookmark_count,
			vote_score: item.vote_score,
			reading_time_minutes: item.read_time_minutes,
			tags: item.tags,
			journey_stage: item.journey_stage ?? null,
			visa_route: item.visa_route ?? null,
			source_country: item.source_country ?? null,
			target_country: item.target_country ?? null,
			guide_category: item.guide_category ?? null,
			video_source_id: item.video_source_id ?? null,
			duration_secs: item.video_duration_secs ?? null,
			space_id: item.space_id,
			space_name: item.space_name ?? space.name,
			space_slug: item.space_slug ?? space.slug,
			space_avatar_url: item.space_avatar_url ?? null
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
	// §05 hub-redesign — the tab set was renamed (feed→overview,
	// forum→discussions, pinned folded into overview). Deep links carrying
	// the old ?tab= values remap here so bookmarks keep working.
	const LEGACY_TABS: Record<string, string> = {
		feed: 'overview',
		forum: 'discussions',
		pinned: 'overview'
	};
	let activeTab = $state(untrack(() => {
		const raw = page.url.searchParams.get('tab') ?? 'overview';
		return LEGACY_TABS[raw] ?? raw;
	}));
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

	// Report dialog — shared ReportDialog posts to /spaces/{id}/report (Section 51).
	let reportOpen = $state(false);

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

	// §05 hub-redesign tab matrix — the canonical visible order is
	// Overview | Discussion | Guide | Blog | Wiki | Stories | List |
	// Events | Sub Spaces | Search | About; the remaining conditional/admin
	// surfaces (Opportunities, Reviews, Requests, Analytics, Settings)
	// append after About and drop into the "More" dropdown (UrlTabs
	// autoOverflow) on narrow widths.
	const hasEvents = $derived(data.calendarEnabled && (isGeo || isUniversity));
	const hasSubspacesTab = $derived(isGeo);
	const tabs = $derived<UrlTab[]>([
		{ value: 'overview', label: t('space.tab.overview', 'Overview') },
		{ value: 'discussions', label: t('space.tab.discussions', 'Discussions') },
		...(isPrimary ? [{ value: 'guides', label: t('space.tab.guides', 'Guides') }] : []),
		...(isPrimary ? [{ value: 'blog', label: t('space.tab.blog', 'Blog') }] : []),
		// Wiki is a primary-space surface (§05 hub matrix) — a custom space
		// never shows the tab even if its space_type has a wiki config.
		...(wikiEnabled && hasWiki && isPrimary
			? [{ value: 'wiki', label: t('space.tab.wiki', 'Wiki') }]
			: []),
		...(isGeo ? [{ value: 'stories', label: t('space.tab.stories', 'Stories') }] : []),
		{ value: 'members', label: t('space.tab.members', 'Members') },
		...(hasEvents ? [{ value: 'events', label: t('space.tab.events', 'Events') }] : []),
		...(hasSubspacesTab ? [{ value: 'subspaces', label: t('space.tab.subspaces', 'Subspaces') }] : []),
		{ value: 'search', label: t('space.tab.search', 'Search') },
		{ value: 'about', label: t('space.tab.about', 'About') },
		...(space.level === 'country' ? [{ value: 'opportunities', label: t('space.tab.opportunities', 'Opportunities') }] : []),
		...(isUniversity ? [{ value: 'reviews', label: t('space.tab.reviews', 'Reviews') }] : []),
		// Join requests — only shown for owner/moderator when joining_perm
		// is 'request' (otherwise the tab is empty/irrelevant).
		...(canManage && space.joining_perm === 'request'
			? [{ value: 'requests', label: t('space.tab.requests', 'Requests') }]
			: []),
		// Analytics dashboard — space-admin only per JEOS §215 [auth,
		// space-admin]. The server only returns analytics data to the space
		// owner/moderators (403 → null otherwise), so the tab is hidden for
		// guests and non-admin members.
		...(authed && analytics
			? [{ value: 'analytics', label: t('space.tab.analytics', 'Analytics') }]
			: []),
		// Settings — visible only to the space owner or a moderator. The
		// PATCH endpoint still enforces the same permission server-side.
		...(canManage
			? [{ value: 'settings', label: t('space.tab.settings', 'Settings') }]
			: [])
	]);

	// Clamp the active tab to the rendered set — an invalid ?tab= value (or a
	// tab that disappears, e.g. 'stories' on a custom space) would
	// otherwise render a blank content area (audit fix).
	$effect(() => {
		if (!tabs.some((tab) => tab.value === activeTab)) {
			activeTab = 'overview';
		}
	});

	// Search tab state (Phase 08.09).
	let searchQuery = $state('');
	let searchResults = $state<SpaceContentItem[]>([]);
	let searching = $state(false);
	let searchError = $state('');
	// True when results came from the ?q= content endpoint (full card rows)
	// rather than Meilisearch's thin {id,title,slug,content_type} hits.
	let searchResultsAreCards = $state(false);

	async function runSearch() {
		if (!searchQuery.trim()) {
			searchResults = [];
			searchError = '';
			return;
		}
		searching = true;
		searchError = '';
		const q = encodeURIComponent(searchQuery.trim());
		if (authed) {
			// GET /spaces/{id}/search — Meilisearch-backed space search. The
			// backend filters status=published and post-filters hits through
			// the §14 visibility gate, and the tab renders only the fields
			// SearchResultItem provides (id/title/slug/content_type).
			const res = await api.get<{
				results: { type: string; item: { id: string; title: string; slug: string; content_type: string } }[];
			}>(`/spaces/${space.id}/search?q=${q}&limit=20`);
			if (res.error) {
				// Search backend down (503 search_unavailable) — degrade to the
				// same public substring search guests use instead of showing a
				// misleading "No results found".
				const fallback = await api.get<SpaceContentItem[]>(
					`/spaces/${space.slug}/content?q=${q}&page_size=20`
				);
				if (fallback.error) {
					searchError = res.error.message;
					searchResults = [];
				} else {
					searchResults = fallback.data ?? [];
					searchResultsAreCards = true;
				}
			} else {
				searchResults = (res.data?.results ?? []).map(
					(r) => r.item as unknown as SpaceContentItem
				);
				searchResultsAreCards = false;
			}
		} else {
			// Guests: /spaces/{id}/search requires auth — fall back to the
			// public content listing's ?q= substring search, which is
			// visibility-gated by OptionalAuth on that route.
			const res = await api.get<SpaceContentItem[]>(`/spaces/${space.slug}/content?q=${q}&page_size=20`);
			if (res.error) {
				searchError = res.error.message;
				searchResults = [];
			} else {
				searchResults = res.data ?? [];
				searchResultsAreCards = true;
			}
		}
		searching = false;
	}

	const title = $derived(`${space.name} — ${t('app.name')}`);
	const description = $derived(space.description ?? `${space.name} on ${t('app.name')}`);
	// Section 26 — self-referencing canonical on the clean slug URL (never
	// the query-string form) so filtered/paginated hits consolidate.
	const canonical = $derived(`${page.url.origin}/spaces/${space.slug}`);

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

	// Client-side load-more for the space feed (?cursor= paging on the last
	// item's created_at — the backend's default listing ignores page_offset).
	let feedItems = $state<SpaceContentItem[]>(untrack(() => data.content));
	let feedLoading = $state(false);
	let feedEnd = $derived(data.content.length < data.pageSize);
	let feedError = $state('');

	// Client-side load-more for the members list (page_offset paging).
	let membersItems = $state(untrack(() => data.members));
	let membersLoading = $state(false);
	let membersEnd = $derived(data.members.length < data.pageSize);

	// §05 hub-redesign — typed tab feeds (blog/stories/guides). Each keeps
	// the SSR page + total and cursor-paginates on created_at like the
	// overview feed.
	interface TypedFeed {
		items: SpaceContentItem[];
		total: number | null;
		loading: boolean;
		end: boolean;
		error: string;
	}
	function seedTypedFeed(feed: { items: SpaceContentItem[]; total: number | null }, pageSize = data.pageSize): TypedFeed {
		return {
			items: feed.items,
			total: feed.total,
			loading: false,
			end: feed.items.length < pageSize,
			error: ''
		};
	}
	let blogFeed = $state<TypedFeed>(untrack(() => seedTypedFeed(data.blogFeed ?? { items: [], total: null })));
	let storyFeed = $state<TypedFeed>(untrack(() => seedTypedFeed(data.storyFeed ?? { items: [], total: null })));
	let guideFeed = $state<TypedFeed>(untrack(() => seedTypedFeed(data.guideFeed ?? { items: [], total: null })));
	// Discussions tab — contents feed over the thread types (forum_thread +
	// discussion anchor rows — §05 hub-redesign), seeded at forumPageSize.
	let discussionFeed = $state<TypedFeed>(untrack(() => seedTypedFeed(data.discussionFeed ?? { items: [], total: null }, data.forumPageSize)));

	function typedFeedFor(type: string): TypedFeed {
		if (type === 'story') return storyFeed;
		if (type === 'guide') return guideFeed;
		if (type === 'forum_thread,discussion') return discussionFeed;
		return blogFeed;
	}

	async function loadMoreTyped(type: 'blog_post' | 'story' | 'guide' | 'forum_thread,discussion') {
		const feed = typedFeedFor(type);
		if (feed.loading || feed.end) return;
		const last = feed.items[feed.items.length - 1];
		if (!last?.created_at) {
			feed.end = true;
			return;
		}
		feed.loading = true;
		feed.error = '';
		const pageSize = type === 'forum_thread,discussion' ? data.forumPageSize : data.pageSize;
		const res = await api.get<SpaceContentItem[]>(
			`/spaces/${space.slug}/content?type=${type}&page_size=${pageSize}&cursor=${encodeURIComponent(last.created_at)}&cursor_id=${encodeURIComponent(last.id)}`
		);
		if (res.error) {
			// Surface the error and stay retryable — ending the feed on a
			// transient failure would brick "Load more" until a reload.
			feed.error = res.error.message;
		} else if (res.data) {
			feed.items = [...feed.items, ...res.data];
			if (res.data.length < pageSize) feed.end = true;
		}
		feed.loading = false;
	}

	// Wiki tab — client-side title filter + popular list (featured first,
	// then most recently updated — the index carries no view_count).
	let wikiQuery = $state('');
	const wikiFiltered = $derived.by(() => {
		const q = wikiQuery.trim().toLowerCase();
		if (!q) return wikiPages;
		return wikiPages.filter((p) => p.title.toLowerCase().includes(q));
	});
	const popularWikiPages = $derived(
		[...wikiPages]
			.sort((a, b) =>
				Number(b.is_featured) - Number(a.is_featured) ||
				new Date(b.updated_at ?? 0).getTime() - new Date(a.updated_at ?? 0).getTime()
			)
			.slice(0, 5)
	);

	// Invite (custom spaces only) + create-subspace modals.
	let inviteOpen = $state(false);
	let createSubspaceOpen = $state(false);

	// Moderators + rules + parent spaces — seeded from SSR, mutated client-side
	// in the Settings tab. Re-seeds when the slug changes.
	let moderators = $state<SpaceModerator[]>(untrack(() => data.moderators));
	let rules = $state<SpaceRule[]>(untrack(() => data.rules));
	let parentSpaces = $state<Space[]>(untrack(() => data.parentSpaces));

	$effect(() => {
		// Re-seed on SSR payload change (slug change) — the page component
		// persists across [slug] navigations, so every mutable that seeds
		// from data must reset or the previous space's state bleeds over.
		feedItems = data.content;
		pinnedItems = data.pinned;
		membersItems = data.members;
		moderators = data.moderators;
		rules = data.rules;
		parentSpaces = data.parentSpaces;
		blogFeed = seedTypedFeed(data.blogFeed ?? { items: [], total: null });
		storyFeed = seedTypedFeed(data.storyFeed ?? { items: [], total: null });
		guideFeed = seedTypedFeed(data.guideFeed ?? { items: [], total: null });
		discussionFeed = seedTypedFeed(data.discussionFeed ?? { items: [], total: null }, data.forumPageSize);
		wikiQuery = '';
		membersEnd = data.members.length < data.pageSize;
		following = data.initiallyFollowing ?? false;
		muted = data.isMuted ?? false;
		feedError = '';
		searchQuery = '';
		searchResults = [];
		searchResultsAreCards = false;
		searching = false;
		pinnedScamAlerts = [];
		joinRequests = [];
		requestsLoaded = false;
		requestError = '';
		requestActionId = null;
		pendingInvites = [];
		invitesLoaded = false;
		invitesLoading = false;
		invitesError = '';
		inviteActionId = null;
		myInvite = data.myInvite ?? null;
		myInviteAction = null;
		myInviteError = '';
		inviteOpen = false;
		createSubspaceOpen = false;
		opportunitiesItems = data.opportunities;
		opportunitiesLoaded = (data.opportunities ?? []).length > 0;
		opportunitiesLoading = false;
		opportunitiesError = '';
		// Re-sync the tab to the URL on each new load — the component
		// persists across [slug] navigations, so without this a carried-over
		// ?tab= (or a stale in-state tab) renders a surface the URL doesn't
		// ask for. untracked: UrlTabs' shallow ?tab= goto must NOT re-run
		// this whole re-seed — only a fresh SSR payload (data.*) does.
		const rawTab = untrack(() => page.url.searchParams.get('tab')) ?? 'overview';
		activeTab = LEGACY_TABS[rawTab] ?? rawTab;
	});

	// Header meta line — parents joined ("Lagos · Delta State" for a city).
	// Reads the mutable parentSpaces state so Settings edits reflect.
	const locationLine = $derived(parentSpaces.map((p) => p.name).join(' · '));
	// Stats strip over the cover — per-class set (§05 hub-redesign matrix).
	const headerStats = $derived.by(() => {
		const stats: { label: string; value: number }[] = [];
		if (space.is_primary) {
			stats.push({ label: t('space.stats.followers', 'Followers'), value: space.follower_count });
		} else {
			stats.push({ label: t('space.stats.members', 'Members'), value: space.member_count });
		}
		stats.push({ label: t('space.stats.discussions', 'Discussions'), value: space.discussion_count ?? 0 });
		if (events.length > 0 || (eventsTotal ?? 0) > 0) {
			stats.push({ label: t('space.stats.events', 'Events'), value: eventsTotal ?? events.length });
		}
		if ((guideFeed.total ?? 0) > 0) {
			stats.push({ label: t('space.stats.guides', 'Guides'), value: guideFeed.total ?? 0 });
		}
		if (!space.is_primary) {
			stats.push({ label: t('space.stats.posts', 'Posts'), value: space.post_count });
		}
		return stats;
	});

	async function loadMoreFeed() {
		if (feedLoading || feedEnd) return;
		// Cursor-paginate on the last item's created_at — the backend's
		// ?cursor= is the only pagination the default listing supports;
		// page_offset is silently ignored and would refetch page 1,
		// producing duplicate keyed-each items.
		const last = feedItems[feedItems.length - 1];
		if (!last?.created_at) {
			feedEnd = true;
			return;
		}
		feedLoading = true;
		feedError = '';
		try {
			// Composite (created_at, id) cursor — items sharing the boundary
			// timestamp (bulk-seeded rows) would otherwise be skipped.
			const res = await api.get<SpaceContentItem[]>(
				`/spaces/${space.slug}/content?page_size=${data.pageSize}&cursor=${encodeURIComponent(last.created_at)}&cursor_id=${encodeURIComponent(last.id)}`
			);
			if (res.error) {
				feedError = res.error.message;
			} else if (res.data) {
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
				`/spaces/${space.slug}/content?page_size=${data.pageSize}`
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
		// Offset = items already loaded — the previous (page+1)*pageSize math
		// skipped an entire page on every click.
		const res = await api.get<SpaceMember[]>(
			`/spaces/${space.slug}/members?page_size=${data.pageSize}&page_offset=${membersItems.length}`
		);
		if (res.data) {
			membersItems = [...membersItems, ...res.data];
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
			// Keep the sidebar count in sync — approval converts a pending
			// request into an approved member (SPACES-004).
			if (status === 'approved') {
				data.space.member_count += 1;
			}
		}
		requestActionId = null;
	}

	// Load join requests when the requests tab becomes active.
	$effect(() => {
		if (activeTab === 'requests' && canManage) {
			loadJoinRequests();
		}
	});

	// Pending member invites — the Members-tab admin section that makes
	// GET/DELETE /spaces/{slug}/invites reachable. Custom spaces only:
	// primary spaces can't hold invites (§05), so the section stays off.
	let pendingInvites = $state<SpaceInvite[]>([]);
	let invitesLoading = $state(false);
	let invitesLoaded = $state(false);
	let invitesError = $state('');
	let inviteActionId = $state<string | null>(null);

	async function loadPendingInvites() {
		if (invitesLoaded || invitesLoading) return;
		invitesLoading = true;
		invitesError = '';
		const res = await api.get<SpaceInvite[]>(`/spaces/${space.slug}/invites?page_size=50`);
		if (res.error) {
			invitesError = res.error.message;
		} else {
			pendingInvites = res.data ?? [];
		}
		// Mark loaded even on failure — the effect re-fires whenever a
		// tracked dep changes, so an uncached error would retry-storm.
		// The error row's Retry button resets invitesLoaded instead.
		invitesLoaded = true;
		invitesLoading = false;
	}

	$effect(() => {
		if (activeTab === 'members' && canManage && !isPrimary) {
			loadPendingInvites();
		}
	});

	async function revokeInvite(inviteId: string) {
		if (inviteActionId) return;
		inviteActionId = inviteId;
		invitesError = '';
		const res = await api.delete(`/spaces/${space.slug}/invites/${inviteId}`);
		if (res.error) {
			invitesError = res.error.message;
		} else {
			pendingInvites = pendingInvites.filter((i) => i.id !== inviteId);
		}
		inviteActionId = null;
	}

	// The viewer's own pending invite to THIS space — SSR-seeded; the
	// space_invite notification's action_url lands here, so the banner is
	// the decline surface the API can't otherwise reach. Accept runs the
	// normal join flow (which resolves the invite server-side); decline
	// flips it to 'declined' and frees the (space, invitee) pair.
	let myInvite = $state<MySpaceInvite | null>(untrack(() => data.myInvite ?? null));
	let myInviteAction = $state<'accept' | 'decline' | null>(null);
	let myInviteError = $state('');

	async function acceptInvite() {
		if (!myInvite || myInviteAction) return;
		myInviteAction = 'accept';
		myInviteError = '';
		const res = await api.post(`/spaces/${space.slug}/join`);
		if (res.error) {
			myInviteError = res.error.message;
			myInviteAction = null;
			return;
		}
		myInvite = null;
		myInviteAction = null;
		// Membership gates the composer, settings surface, and stats —
		// re-run the load so the page reflects the new member state.
		await invalidateAll();
	}

	async function declineInvite() {
		if (!myInvite || myInviteAction) return;
		myInviteAction = 'decline';
		myInviteError = '';
		const res = await api.post(`/me/space-invites/${myInvite.id}/decline`);
		if (res.error) {
			myInviteError = res.error.message;
		} else {
			myInvite = null;
		}
		myInviteAction = null;
	}

	// Opportunities (§73 intel radar, country spaces) — SSR-fetched only on
	// a ?tab=opportunities deep link; UrlTabs switches shallowly (no load
	// re-run), so a tab click lazy-fetches here instead.
	let opportunitiesItems = $state<RadarItem[]>(untrack(() => data.opportunities));
	let opportunitiesLoading = $state(false);
	let opportunitiesLoaded = $state(untrack(() => (data.opportunities ?? []).length > 0));
	let opportunitiesError = $state('');

	$effect(() => {
		if (
			activeTab === 'opportunities' &&
			space.level === 'country' &&
			!opportunitiesLoaded &&
			!opportunitiesLoading
		) {
			opportunitiesLoading = true;
			opportunitiesError = '';
			// The radar envelope is {items, next_cursor, …} — no data wrapper.
			api.get<{ items?: RadarItem[] }>(
				`/intel/radar?space_id=${space.id}&page_size=24`
			).then((res) => {
				if (res.error) {
					opportunitiesError = res.error.message;
				} else {
					opportunitiesItems = res.data?.items ?? [];
				}
				opportunitiesLoaded = true;
				opportunitiesLoading = false;
			});
		}
	});
</script>

<svelte:head>
	<title>{title}</title>
	<meta name="description" content={description} />
	<meta property="og:title" content={title} />
	<meta property="og:description" content={description} />
	<meta property="og:type" content="website" />
	<link rel="canonical" href={canonical} />
	<meta property="og:url" content={canonical} />
	<meta property="og:image" content={ogImage} />
	<meta name="twitter:card" content="summary_large_image" />
	<meta name="twitter:title" content={title} />
	<meta name="twitter:description" content={description} />
	<meta name="twitter:image" content={ogImage} />
	{#if data.seoIndexable === false}
		<meta name="robots" content="noindex,nofollow" />
	{:else}
		<!-- §26 — per-space RSS autodiscovery (public spaces only). -->
		<link
			rel="alternate"
			type="application/rss+xml"
			title="{space.name} — {t('app.name')}"
			href="/spaces/{space.slug}/feed.xml"
		/>
	{/if}
	{@html safeJsonLdScript(breadcrumbLd)}
</svelte:head>

<SetSidebar>
	<SpaceSidebar
		{space}
		rules={data.rules}
		{authed}
		logoUrl={spaceLogo}
		{initialJoined}
		{initialPending}
		isOwner={data.isOwner}
		following={data.initiallyFollowing}
		onFollowChange={(v) => (following = v)}
		achievements={data.achievements}
		members={data.members}
		{parentSpaces}
		{typeName}
		{events}
		{leaders}
		{contributors}
		keywords={data.keywords}
	/>
</SetSidebar>

<!-- Space header — mockup cover card w/ overlaid stats + actions -->
<div class="mx-auto w-full max-w-3xl px-4 pt-4">
	<SpaceHeader
		{space}
		logoUrl={spaceLogo}
		headerUrl={data.headerUrl}
		{locationLine}
		{typeName}
		stats={headerStats}
		{authed}
		{following}
		{followPending}
		{muted}
		{mutePending}
		showWikiLink={wikiEnabled && hasWiki && isPrimary}
		canInvite={authed && !isPrimary}
		onFollow={toggleFollow}
		onMute={toggleMute}
		onReport={() => (reportOpen = true)}
		onInvite={() => (inviteOpen = true)}
	/>
</div>

<!-- Pending-invite banner — the space_invite notification's action_url
     lands on this page, so the banner is the invitee's accept/decline
     surface (GET /me/space-invites seed, decline via /me/space-invites). -->
{#if myInvite}
	<div class="mx-auto w-full max-w-3xl px-4 pt-3">
		<div class="flex items-center gap-3 rounded-[var(--radius-card)] border border-[var(--color-primary)]/30 bg-[var(--color-primary)]/5 px-4 py-3">
			<UserPlus size={18} class="shrink-0 text-[var(--color-primary-text)]" />
			<p class="min-w-0 flex-1 text-sm text-[var(--color-text)]">
				{t('space.invited_banner', '{name} invited you to join this space', {
					name: myInvite.inviter_display_name ?? myInvite.inviter_username ?? t('space.someone', 'Someone')
				})}
			</p>
			{#if myInviteError}
				<span class="text-xs text-red-600 dark:text-red-400">{myInviteError}</span>
			{/if}
			<button
				type="button"
				onclick={acceptInvite}
				disabled={myInviteAction !== null}
				class="shrink-0 rounded-[var(--radius)] bg-[var(--color-primary)] px-3.5 py-1.5 text-sm font-medium text-[var(--color-primary-contrast)] transition-opacity hover:opacity-90 disabled:opacity-50"
			>
				{myInviteAction === 'accept' ? t('common.loading') : t('space.invite_accept', 'Accept')}
			</button>
			<button
				type="button"
				onclick={declineInvite}
				disabled={myInviteAction !== null}
				class="shrink-0 rounded-[var(--radius)] border border-[var(--color-border)] px-3.5 py-1.5 text-sm font-medium text-[var(--color-text-subtle)] transition-colors hover:bg-[var(--color-surface-subtle)] disabled:opacity-50"
			>
				{myInviteAction === 'decline' ? t('common.loading') : t('space.invite_decline', 'Decline')}
			</button>
		</div>
	</div>
{/if}

<div class="sticky top-0 z-[15] mx-auto w-full max-w-3xl"><UrlTabs {tabs} bind:value={activeTab} autoOverflow /></div>

{#if pinnedScamAlerts.length > 0}
	<div class="mx-auto w-full max-w-3xl px-4 pt-3">
		<div class="rounded-[var(--radius-card)] border border-red-500/40 bg-red-500/10 px-4 py-3" role="alert">
			<p class="flex items-center gap-1.5 text-sm font-semibold text-red-600">
				<TriangleAlert size={15} /> {t('scam_alerts.space_warning_title', 'Active scam warning')}
			</p>
			{#each pinnedScamAlerts.slice(0, 2) as alert (alert.id)}
				<a href="/scam-alerts/{alert.id}" class="mt-1.5 block text-sm text-red-700 hover:underline">
					{alert.description.length > 120 ? alert.description.slice(0, 120).trimEnd() + '…' : alert.description}
				</a>
			{/each}
			<a href="/scam-alerts?dest={space.id}" class="mt-2 inline-block text-xs font-medium text-red-700 hover:underline">
				{t('scam_alerts.space_warning_more', 'View all scam alerts for this destination')} →
			</a>
		</div>
	</div>
{/if}

<div class="isolate mx-auto w-full max-w-full" style="--page-gutter: 1rem">
	<div class="mx-auto w-full max-w-3xl p-4">
		{#if activeTab === 'overview'}
			<!-- Composer — custom/university spaces; geo hubs keep posting
			     gated to their primary feed (§05 amendment). -->
			{#if authed && !isGeoSpace}
				<div class="mb-3">
					<SpaceComposer {space} />
				</div>
			{/if}
			<div class="mb-3">
				<DigestWidget spaceId={space.id} digest={data.digest} />
			</div>
			<!-- Pinned fold-in (§05 hub-redesign): pinned items lead the
			     overview stream, deduped from the feed below. -->
			{#if pinnedItems.length > 0}
				<div class="mb-3 flex flex-col gap-3">
					{#each pinnedItems as item (item.id)}
						<div class="group relative block">
							<FeedCard item={toFeedItem(item)} style="daily" layout="stream" />
							<span class="pointer-events-none absolute start-2 top-2 z-10 flex items-center gap-0.5 rounded-full bg-[var(--color-primary)]/10 px-2 py-0.5 text-[10px] font-medium text-[var(--color-primary-text)]">
								<Pin size={10} /> {t('space.pinned', 'Pinned')}
							</span>
							{#if canManage}
								<div class="absolute end-3 top-3 z-10 flex items-center gap-0.5 opacity-0 transition-opacity group-hover:opacity-100">
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
			{@const pinnedIds = new Set(pinnedItems.map((p) => p.id))}
			{@const streamItems = feedItems.filter((f) => !pinnedIds.has(f.id))}
			{#if feedItems.length === 0 && pinnedItems.length === 0}
				<EmptyState icon={Rss} title={t('space.feed_empty', 'No posts yet')} />
			{:else if streamItems.length > 0}
				<div class="flex flex-col gap-3">
					{#each streamItems as item, i (item.id)}
						{#if adInterval > 0 && i > 0 && i % adInterval === 0 && i / adInterval < 2 && i < feedItems.length - 1}
							<NativeAd slot="native_space_feed" spaceId={space.id} pickIndex={Math.floor(i / adInterval) - 1} />
						{/if}
						<div class="group relative block">
							<FeedCard
								item={toFeedItem(item)}
								style="daily"
								layout="stream"
							/>
							{#if item.is_pinned}
								<span class="pointer-events-none absolute start-2 top-2 z-10 flex items-center gap-0.5 rounded-full bg-[var(--color-primary)]/10 px-2 py-0.5 text-[10px] font-medium text-[var(--color-primary-text)]">
									<Pin size={10} /> {t('space.pinned', 'Pinned')}
								</span>
							{/if}
							{#if canManage}
								<div class="absolute end-3 top-3 z-10 flex items-center gap-0.5 opacity-0 transition-opacity group-hover:opacity-100">
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
		{:else if activeTab === 'events'}
			<!-- Events tab (§86 calendar, geo + university) — upcoming entries
			     for this destination with a link to the full calendar. -->
			{#if events.length === 0}
				<EmptyState icon={CalendarDays} title={t('space.events.empty', 'No upcoming events')} />
			{:else}
				<ul class="divide-y divide-[var(--color-border)] rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)]">
					{#each events as ev (ev.id)}
						{@const dt = new Date(ev.occurs_at)}
						<li class="flex items-center gap-3 p-3">
							<span class="grid h-12 w-12 shrink-0 place-items-center rounded-[var(--radius)] bg-[var(--color-primary)]/10 text-[var(--color-primary-text)]">
								<span class="text-center leading-tight">
									<span class="block text-[10px] font-semibold uppercase">{dt.toLocaleDateString(undefined, { month: 'short' })}</span>
									<span class="block text-lg font-bold">{dt.getDate()}</span>
								</span>
							</span>
							<span class="min-w-0 flex-1">
								<span class="block truncate text-sm font-medium text-[var(--color-text)]">{ev.title}</span>
								<span class="block truncate text-xs text-[var(--color-muted)]">
									{dt.toLocaleString(undefined, { weekday: 'short', hour: 'numeric', minute: '2-digit' })}
									{#if ev.category} · {ev.category.replace(/_/g, ' ')}{/if}
								</span>
							</span>
							{#if ev.source_url}
								<a
									href={ev.source_url}
									target="_blank"
									rel="noopener noreferrer"
									class="shrink-0 rounded-full border border-[var(--color-border)] px-3 py-1 text-xs font-medium text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]"
								>
									{t('space.events.source', 'Details')}
								</a>
							{/if}
						</li>
					{/each}
				</ul>
				<div class="mt-3 text-center">
					<a href="/calendar?dest={space.id}" class="text-sm font-medium text-[var(--color-primary-text)] hover:underline">
						{t('space.events.view_all', 'View all events')} →
					</a>
				</div>
			{/if}
		{:else if activeTab === 'blog' || activeTab === 'stories' || activeTab === 'guides'}
			<!-- Type-filtered feeds (§05 hub-redesign) — blog_post on primary,
			     story on geo, guide on primary; each cursor-paginates. -->
			{@const tf = activeTab === 'stories' ? storyFeed : activeTab === 'guides' ? guideFeed : blogFeed}
			{@const tfType = activeTab === 'stories' ? 'story' : activeTab === 'guides' ? 'guide' : 'blog_post'}
			{#if tf.items.length === 0}
				<EmptyState
					icon={BookOpen}
					title={activeTab === 'stories'
						? t('space.stories_empty', 'No stories yet')
						: activeTab === 'guides'
							? t('space.guides_empty', 'No guides yet')
							: t('space.blog_empty', 'No posts yet')}
				/>
			{:else}
				<div class="flex flex-col gap-3">
					{#each tf.items as item (item.id)}
						<FeedCard item={toFeedItem(item)} style="daily" layout="stream" />
					{/each}
				</div>
				{#if !tf.end}
					{#if tf.error}
						<p class="mx-auto mt-3 text-center text-sm text-red-600 dark:text-red-400">{tf.error}</p>
					{/if}
					<button
						type="button"
						onclick={() => loadMoreTyped(tfType)}
						disabled={tf.loading}
						class="mx-auto mt-3 block rounded-[var(--radius)] border border-[var(--color-border)] px-4 py-2 text-sm text-[var(--color-text-subtle)] hover:bg-[var(--color-surface-subtle)] disabled:opacity-50"
					>
						{tf.loading ? t('common.loading') : t('feed.load_more', 'Load more')}
					</button>
				{/if}
			{/if}
		{:else if activeTab === 'wiki'}
			<!-- Wiki/Knowledge tab (§05 hub-redesign, primary spaces) — search,
			     browse-by-category cards, popular articles, suggest CTA. -->
			<section class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
				<div class="flex items-start justify-between gap-3">
					<div>
						<h2 class="text-base font-semibold text-[var(--color-text)]">
							{t('space.wiki_tab.title', '{space} Community Wiki').replace('{space}', space.name)}
						</h2>
						<p class="mt-0.5 text-sm text-[var(--color-text-subtle)]">
							{t('space.wiki_tab.subtitle', 'Everything you need to know, written and maintained by the community.')}
						</p>
					</div>
					<a
						href="/wiki/{encodeURIComponent(space.slug)}/create"
						class="shrink-0 rounded-full border border-[var(--color-border)] px-3 py-1.5 text-xs font-medium text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]"
					>
						{t('space.wiki_tab.suggest', 'Suggest an article')}
					</a>
				</div>

				<div class="mt-3 flex items-center gap-2 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 focus-within:border-[var(--color-primary)]">
					<Search size={16} class="text-[var(--color-muted)]" />
					<input
						type="search"
						bind:value={wikiQuery}
						placeholder={t('space.wiki_tab.search_placeholder', 'Search the wiki…')}
						class="w-full appearance-none border-0 bg-transparent text-sm text-[var(--color-text)] outline-none ring-0 placeholder:text-[var(--color-muted)]"
					/>
				</div>

				{#if wikiQuery.trim()}
					{#if wikiFiltered.length === 0}
						<p class="py-6 text-center text-sm text-[var(--color-muted)]">{t('space.no_search_results', 'No results found')}</p>
					{:else}
						<ul class="mt-3 space-y-1.5">
							{#each wikiFiltered as wikiPage (wikiPage.id)}
								<li>
									<a
										href="/wiki/{encodeURIComponent(space.slug)}/{encodeURIComponent(wikiPage.slug)}"
										class="flex items-center justify-between rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface-subtle)] px-3 py-2 text-sm transition-colors hover:border-[var(--color-primary)] hover:bg-[var(--color-surface)]"
									>
										<span class="font-medium text-[var(--color-text)]">{wikiPage.title}</span>
										{#if wikiPage.is_featured}
											<Star size={12} class="shrink-0 text-[var(--color-warning)]" role="img" aria-label={t('wiki.featured', 'Featured')} />
										{/if}
									</a>
								</li>
							{/each}
						</ul>
					{/if}
				{:else}
					{#if wikiGroups.length > 0}
						<div class="mt-4 grid grid-cols-2 gap-2 sm:grid-cols-3">
							{#each wikiGroups as group (group.category)}
								{@const CatIcon = categoryIconResolved(group.category)}
								{@const tint = categoryTint(group.category)}
								<a
									href={group.pageSlug
										? `/wiki/${encodeURIComponent(space.slug)}/${encodeURIComponent(group.pageSlug)}`
										: `/wiki/${encodeURIComponent(space.slug)}#${encodeURIComponent(group.category)}`}
									class="rounded-[var(--radius)] border border-[var(--color-border)] p-3 transition-colors hover:border-[var(--color-primary)]"
								>
									<span
										class="mb-2 grid h-8 w-8 place-items-center rounded-[var(--radius)]"
										style="background:{tint.bg};color:{tint.fg}"
									>
										<CatIcon size={16} />
									</span>
									<span class="block text-sm font-medium text-[var(--color-text)]">{group.label}</span>
									<span class="block text-xs text-[var(--color-muted)]">
										{t('space.wiki_tab.pages_count', '{n} articles').replace('{n}', String(group.pages.length))}
									</span>
								</a>
							{/each}
						</div>
					{/if}
					{#if popularWikiPages.length > 0}
						<h3 class="mt-4 text-xs font-semibold uppercase tracking-wide text-[var(--color-muted)]">
							{t('space.wiki_tab.popular', 'Popular articles')}
						</h3>
						<ul class="mt-2 divide-y divide-[var(--color-border)]">
							{#each popularWikiPages as wikiPage (wikiPage.id)}
								<li>
									<a
										href="/wiki/{encodeURIComponent(space.slug)}/{encodeURIComponent(wikiPage.slug)}"
										class="flex items-center justify-between py-2 text-sm transition-colors hover:text-[var(--color-primary-text)]"
									>
										<span class="font-medium text-[var(--color-text)]">{wikiPage.title}</span>
										{#if wikiPage.is_featured}
											<Star size={12} class="shrink-0 text-[var(--color-warning)]" role="img" aria-label={t('wiki.featured', 'Featured')} />
										{/if}
									</a>
								</li>
							{/each}
						</ul>
					{/if}
					<a
						href="/wiki/{encodeURIComponent(space.slug)}"
						class="mt-4 inline-flex items-center gap-1 text-sm font-medium text-[var(--color-primary-text)] hover:underline"
					>
						{t('space.wiki_tab.explore_all', 'Explore all wiki articles')}
						<ArrowRight size={14} class="rtl:-scale-x-100" />
					</a>
				{/if}
			</section>
		{:else if activeTab === 'subspaces'}
			<!-- Subspaces tab (§05 hub-redesign, geo only) — member-created
			     communities under this destination + create CTA. -->
			{#if subspaces.length === 0}
				<EmptyState icon={Layers} title={t('space.subspaces_empty', 'No subspaces yet')} />
			{:else}
				<div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
					{#each subspaces as sub (sub.id)}
						<DirectoryCard
							href="/spaces/{sub.slug}"
							title={sub.name}
							description={sub.description}
							imageUrl={sub.logo_url}
							fallback={sub.name.charAt(0).toUpperCase()}
							verified={!!sub.is_primary}
							meta={[
								{ icon: Users, text: t('space.member_count', '{n} members').replace('{n}', String(sub.member_count)) },
								{ icon: MessageSquare, text: t('space.discussions_count', '{n} discussions').replace('{n}', String(sub.discussion_count ?? 0)) }
							]}
						>
							{#snippet actions()}
								<SpaceJoinButton entityId={sub.id} entitySlug={sub.slug} compact />
							{/snippet}
						</DirectoryCard>
					{/each}
				</div>
			{/if}
			{#if authed}
				<button
					type="button"
					onclick={() => (createSubspaceOpen = true)}
					class="mt-4 flex w-full items-center justify-center gap-2 rounded-[var(--radius-card)] border border-dashed border-[var(--color-border)] px-4 py-3 text-sm font-medium text-[var(--color-text-subtle)] transition-colors hover:border-[var(--color-primary)] hover:text-[var(--color-text)]"
				>
					<Layers size={16} /> {t('space.subspaces_create', 'Create a new subspace')}
				</button>
			{:else}
				<a
					href="/login?redirect={encodeURIComponent(page.url.pathname)}"
					class="mt-4 flex w-full items-center justify-center gap-2 rounded-[var(--radius-card)] border border-dashed border-[var(--color-border)] px-4 py-3 text-sm font-medium text-[var(--color-text-subtle)] transition-colors hover:border-[var(--color-primary)] hover:text-[var(--color-text)]"
				>
					<Layers size={16} /> {t('space.subspaces_create', 'Create a new subspace')}
				</a>
			{/if}
		{:else if activeTab === 'discussions'}
			<!-- §05 hub-redesign — contents feed over the thread types
			     (forum_thread + discussion anchor rows); /thread?space_id=
			     misses anchored discussions. -->
			{#if discussionFeed.items.length === 0}
				<EmptyState icon={MessageSquare} title={t('space.forum_empty', 'No discussions yet')} />
			{:else}
				<div class="flex flex-col gap-3">
					{#each discussionFeed.items as item (item.id)}
						<FeedCard item={toFeedItem(item)} style="daily" layout="stream" threadDefault voteRail />
					{/each}
				</div>
				{#if !discussionFeed.end}
					{#if discussionFeed.error}
						<p class="mx-auto mt-3 text-center text-sm text-red-600 dark:text-red-400">{discussionFeed.error}</p>
					{/if}
					<button
						type="button"
						onclick={() => loadMoreTyped('forum_thread,discussion')}
						disabled={discussionFeed.loading}
						class="mx-auto mt-3 block rounded-[var(--radius)] border border-[var(--color-border)] px-4 py-2 text-sm text-[var(--color-text-subtle)] hover:bg-[var(--color-surface-subtle)] disabled:opacity-50"
					>
						{discussionFeed.loading ? t('common.loading') : t('feed.load_more', 'Load more')}
					</button>
				{/if}
			{/if}
		{:else if activeTab === 'members'}
			<!-- Pending invites (custom spaces, owner/moderator only) — the
			     admin surface for GET/DELETE /spaces/{slug}/invites. -->
			{#if canManage && !isPrimary && (invitesLoading || invitesError || pendingInvites.length > 0)}
				<div class="mb-4 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
					<h3 class="mb-3 flex items-center gap-2 text-sm font-semibold text-[var(--color-text)]">
						<UserPlus size={15} />
						{t('space.invites_pending', 'Pending invites')}
					</h3>
					{#if invitesError}
						<p class="mb-2 flex items-center gap-2 text-sm text-red-600 dark:text-red-400">
							<span class="min-w-0 flex-1">{invitesError}</span>
							<button
								type="button"
								onclick={() => { invitesLoaded = false; loadPendingInvites(); }}
								class="shrink-0 text-xs font-medium text-[var(--color-primary-text)] hover:underline"
							>
								{t('common.retry', 'Retry')}
							</button>
						</p>
					{/if}
					{#if invitesLoading}
						<p class="py-2 text-sm text-[var(--color-muted)]">{t('common.loading', 'Loading…')}</p>
					{:else}
						<ul class="space-y-2">
							{#each pendingInvites as invite (invite.id)}
								<li class="flex items-center gap-3 rounded-[var(--radius)] border border-[var(--color-border)] p-2.5">
									{#if invite.invitee_avatar_url}
										<img src={remoteImageUrl(invite.invitee_avatar_url)} alt="" class="h-8 w-8 rounded-full object-cover" />
									{:else}
										<span class="grid h-8 w-8 shrink-0 place-items-center rounded-full bg-[var(--color-surface-subtle)] text-xs font-medium text-[var(--color-text-subtle)]">
											{(invite.invitee_display_name ?? invite.invitee_username ?? '?').charAt(0).toUpperCase()}
										</span>
									{/if}
									<span class="min-w-0 flex-1">
										<span class="block truncate text-sm font-medium text-[var(--color-text)]">{invite.invitee_display_name ?? invite.invitee_username}</span>
										<span class="block text-xs text-[var(--color-muted)]">
											{t('space.invited_by', 'Invited by {name}', { name: invite.inviter_display_name ?? invite.inviter_username ?? '' })}
										</span>
									</span>
									<button
										type="button"
										onclick={() => revokeInvite(invite.id)}
										disabled={inviteActionId === invite.id}
										class="shrink-0 rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1 text-xs font-medium text-[var(--color-text-subtle)] transition-colors hover:bg-[var(--color-surface-subtle)] hover:text-red-600 disabled:opacity-50"
									>
										{inviteActionId === invite.id ? t('common.loading') : t('space.invite_revoke', 'Revoke')}
									</button>
								</li>
							{/each}
						</ul>
					{/if}
				</div>
			{/if}
			{#if membersItems.length === 0}
				<EmptyState icon={Users} title={t('space.members_empty', 'No members yet')} />
			{:else}
				<ul class="space-y-2">
					{#each membersItems as member (member.user_id)}
						<li>
							<UserHoverCard
								id={member.user_id}
								slug={member.username}
								name={member.display_name ?? member.username}
								avatar={member.avatar_url}
								verified={member.verified}
								verificationType={member.verification_type}
								class="inline-flex w-full min-w-0 max-w-full items-center gap-3 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-3 text-start transition-colors hover:bg-[var(--color-surface-subtle)]"
							>
								{#snippet trigger()}
									{#if member.avatar_url}
										<img src={remoteImageUrl(member.avatar_url)} alt="" class="h-10 w-10 rounded-full object-cover" />
									{:else}
										<span class="grid h-10 w-10 shrink-0 place-items-center rounded-full bg-[var(--color-surface-subtle)] text-sm font-medium text-[var(--color-text-subtle)]">
											{(member.display_name ?? member.username).charAt(0).toUpperCase()}
										</span>
									{/if}
									<span class="min-w-0 flex-1">
										<span class="flex min-w-0 items-center gap-1 truncate text-sm font-medium text-[var(--color-text)]"><span class="truncate">{member.display_name ?? member.username}</span><VerifiedBadge verified={member.verified} type={member.verification_type} size={14} /></span>
										<span class="block text-xs text-[var(--color-muted)]">@{member.username}</span>
									</span>
								{/snippet}
							</UserHoverCard>
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
		{:else if activeTab === 'opportunities'}
			<div class="p-4">
				{#if opportunitiesLoading}
					<p class="py-8 text-center text-sm text-[var(--color-muted)]">{t('common.loading', 'Loading…')}</p>
				{:else if opportunitiesError}
					<p class="py-8 text-center text-sm text-red-600 dark:text-red-400">{opportunitiesError}</p>
				{:else if opportunitiesItems.length === 0}
					<EmptyState
						icon={Radar}
						title={t('space.opportunities_empty', 'No opportunities for this destination yet')}
						body={t('space.opportunities_empty_body', 'New scholarships, jobs, and alerts will appear here as sources publish them.')}
					/>
				{:else}
					<div class="space-y-2">
						{#each opportunitiesItems as item (item.id)}
							<RadarItemRow {item} />
						{/each}
					</div>
					<div class="mt-4 flex justify-center">
						<a
							href={`/radar?space_id=${space.id}`}
							class="text-sm font-medium text-[var(--color-primary-text)] hover:underline"
						>
							{t('space.opportunities_view_all', 'View all in Japa Opportunities')} →
						</a>
					</div>
				{/if}
			</div>
		{:else if activeTab === 'about'}
			<section class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
				{#if space.description}
					<p class="text-sm leading-relaxed text-[var(--color-text)]">{space.description}</p>
				{:else}
					<EmptyState icon={Info} title={t('space.no_about', 'No description yet')} />
				{/if}

				<!-- Community wiki moved to the dedicated Wiki tab (§05
				     hub-redesign); About keeps the Wikipedia extract only. -->
				{#if wikiData}
					<div class="mt-4 border-t border-[var(--color-border)] pt-4" use:imgLightbox>
						<h2 class="mb-2 text-sm font-medium text-[var(--color-text)]">{t('space.wiki_summary', 'From Wikipedia')}</h2>
						{#if spaceLogo}
							<img src={spaceLogo} alt={wikiData.title ?? space.name} class="float-end mb-3 ms-3 h-24 w-24 rounded-[var(--radius)] object-cover" />
						{/if}
						{#if wikiData.description}
							<p class="mb-2 text-xs font-medium uppercase tracking-wide text-[var(--color-muted)]">{wikiData.description}</p>
						{/if}
						{#if wikiData.extract}
							<p class="text-sm leading-relaxed text-[var(--color-text-subtle)]">{wikiData.extract}</p>
						{/if}
						{#if wikiData.content_url}
							<p class="mt-3 text-xs">
								<a href={wikiData.content_url} target="_blank" rel="noopener noreferrer" class="text-[var(--color-primary-text)] hover:underline">
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
			{:else if searchError}
				<p class="py-8 text-center text-sm text-red-600 dark:text-red-400">{searchError}</p>
			{:else if searchQuery.trim() && searchResults.length === 0}
				<EmptyState icon={Search} title={t('space.no_search_results', 'No results found')} />
			{:else if searchResults.length > 0}
				{#if searchResultsAreCards}
					<div class="flex flex-col gap-3">
						{#each searchResults as item (item.id)}
							<FeedCard item={toFeedItem(item)} style="daily" layout="stream" />
						{/each}
					</div>
				{:else}
					<div class="divide-y divide-[var(--color-border)] overflow-hidden rounded-[var(--radius-card)] bg-[var(--color-surface)]">
						{#each searchResults as item (item.id)}
							<a href="/content/{item.slug}" class="block p-4 transition-colors hover:bg-[var(--color-surface-subtle)]">
								<h3 class="text-sm font-medium text-[var(--color-text)]">{item.title}</h3>
								<p class="mt-1 text-xs text-[var(--color-muted)]">{item.content_type}</p>
							</a>
						{/each}
					</div>
				{/if}
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
									<img src={remoteImageUrl(req.avatar_url)} alt="" class="h-10 w-10 rounded-full object-cover" />
								{:else}
									<span class="grid h-10 w-10 shrink-0 place-items-center rounded-full bg-[var(--color-surface-subtle)] text-sm font-medium text-[var(--color-text-subtle)]">
										{(req.display_name ?? req.username).charAt(0).toUpperCase()}
									</span>
								{/if}
								<a href="/u/{req.username}" class="min-w-0 flex-1">
									<span class="flex items-center gap-1 truncate text-sm font-medium text-[var(--color-text)]"><span class="truncate">{req.display_name ?? req.username}</span><VerifiedBadge verified={req.verified} type={req.verification_type} size={14} /></span>
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
									<span class="flex items-center gap-1 text-sm font-medium text-[var(--color-text)]"><span class="truncate">{review.author_name}</span><VerifiedBadge verified={review.author_verified} type={review.author_verification_type} size={14} /></span>
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

<!-- Report space dialog — POST /spaces/{id}/report (Section 51) -->
<ReportDialog bind:open={reportOpen} entityType="space" entityId={space.id} entityName={space.name} />

<!-- Member-invite modal (§05 hub-redesign) — custom spaces only. -->
{#if authed && !isPrimary}
	<InviteModal bind:open={inviteOpen} {space} onsent={() => (invitesLoaded = false)} />
{/if}

<!-- Create-subspace modal (§05 hub-redesign) — geo parents pre-attach
     via parent_space_ids. -->
{#if authed && isGeo}
	<SpaceCreationModal bind:open={createSubspaceOpen} presetParentSpace={{ id: space.id, name: space.name }} />
{/if}
