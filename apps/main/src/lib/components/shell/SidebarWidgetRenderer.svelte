<script lang="ts">
	// Ref: Section 59 (Sidebar Management System), ADR-066.
	// Renders a single sidebar widget based on its widget_type. Each widget type
	// maps to a specific UI component. Widget props configure behavior (limit,
	// title, slot, format, etc.). Data-fetching widgets (trending_tags,
	// suggested_spaces) receive their data from the parent RightSidebar via
	// props, since the data is already fetched SSR in +layout.server.ts.

	import { Sun, Moon, Languages, Check, Bell, Mail, Share2, Calendar, Trophy, Hash, Star, Bookmark, BarChart3, MousePointerClick, UserCircle, FolderTree, MessageSquare, PlayCircle, GraduationCap, BookOpen, AtSign, Globe, Link, Rss, Users, TrendingUp, Activity } from 'lucide-svelte';
	import { theme } from '$lib/stores/theme.svelte';
	import { t, i18n } from '$lib/i18n';
	import { api } from '$lib/api/client';
	import { page } from '$app/state';
	import { DropdownMenu } from 'bits-ui';
	import { untrack } from 'svelte';
	import SearchAutocomplete from './SearchAutocomplete.svelte';
	import UserDropdown from './UserDropdown.svelte';
	import NotificationCentre from './NotificationCentre.svelte';
	import { AdSlot } from '$lib/components/ui';
	import ProgressRing from '$lib/components/ui/ProgressRing.svelte';
	import { sanitizeSidebarHtml, safeHref } from '$lib/utils/safeHtml';
	import { contentHref } from '$lib/utils/contentLink';
	import { learnUrl } from '$lib/appOrigins';
	import type { SessionUser, TrendingTag, SpaceSuggestion, LearnEnrollment, CatalogTrack } from '$lib/types';

	interface SidebarWidget {
		id: string;
		widget_type: string;
		label: string;
		props: Record<string, unknown>;
		sort_order: number;
	}

	interface Props {
		widget: SidebarWidget;
		user: SessionUser | null;
		trendingTags?: TrendingTag[];
		suggestedSpaces?: SpaceSuggestion[];
		enabledLanguages?: string;
		isDashboard?: boolean;
	}

	let {
		widget,
		user,
		trendingTags = [],
		suggestedSpaces = [],
		enabledLanguages = 'en,fr',
		isDashboard = false
	}: Props = $props();

	const widgetProps = $derived(widget.props ?? {});
	const limit = $derived(typeof widgetProps.limit === 'number' ? widgetProps.limit : 8);
	const title = $derived(typeof widgetProps.title === 'string' ? widgetProps.title : '');
	const adSlot = $derived(typeof widgetProps.slot === 'string' ? widgetProps.slot : 'sidebar_top');
	const adFormat = $derived(
		(typeof widgetProps.format === 'string' && ['square', 'banner', 'inline'].includes(widgetProps.format))
			? widgetProps.format as 'square' | 'banner' | 'inline'
			: 'square' as const
	);

	let showLanguageToggle = $derived(enabledLanguages.split(',').filter(Boolean).length > 1);

	// Default language from site settings (for unprefixed URL generation).
	// Ref: §18 — English unprefixed at root, other languages prefixed.
	const defaultLanguage = $derived(
		(page.data?.siteSettings?.['i18n.default_language'] as string | undefined) ?? 'en'
	);

	// Parse the enabled-languages list for the dropdown.
	const langList = $derived(
		enabledLanguages.split(',').map((l) => l.trim()).filter(Boolean)
	);

	// Resolve locale codes to display names via Intl.DisplayNames (platform
	// API, not app data — Rule 16 safe). Falls back to the code itself.
	const displayNames = (() => {
		try { return new Intl.DisplayNames(undefined, { type: 'language' }); }
		catch { return null; }
	})();
	function localeLabel(code: string): string {
		return displayNames?.of(code) ?? code;
	}

	// Build the switch URL for a given language: unprefixed for the default
	// language, prefixed for all others. Ref: §18. Strip an existing locale
	// prefix first — on prefixed URLs page.url.pathname can retain the prefix,
	// which would otherwise produce /fr/fr/... double-prefixed links.
	function localeHref(lang: string): string {
		let path = currentPath || '';
		const firstSeg = path.split('/')[0];
		if (langList.includes(firstSeg)) path = path.slice(firstSeg.length).replace(/^\//, '');
		return lang === defaultLanguage ? `/${path}` : `/${lang}${path ? `/${path}` : ''}`;
	}

	// Switch locale: persist the choice in the jc_locale cookie AND navigate
	// to the locale-prefixed URL. The cookie write is required — the hooks
	// fall back to the cookie when the URL has no prefix, so without it,
	// switching to the default language keeps rendering the previous locale.
	function switchLanguage(lang: string) {
		document.cookie = `jc_locale=${lang}; path=/; max-age=${60 * 60 * 24 * 365}; samesite=lax`;
		window.location.href = localeHref(lang);
	}

	// SSR-safe current path for locale-switch links. Uses $app/state's page
	// store (available during SSR) instead of window.location to avoid
	// hydration mismatches (Guide §16).
	const currentPath = $derived(page.url.pathname.replace(/^\//, ''));

	// learnUrl throws when the cross-app origin env vars are unset — a
	// throwing href expression would crash the whole renderer, so all
	// learning-widget links go through this guard (returns '' → the
	// template falls back to '#').
	function safeLearnUrl(path: string): string {
		try { return learnUrl(path); } catch { return ''; }
	}

	// lucide icon lookup for admin-configured icon names (metric_card.icon,
	// social_links.platform). Unknown names fall back to a neutral icon so a
	// typo in the admin panel never crashes the renderer.
	const iconByName: Record<string, typeof BarChart3> = {
		'bar-chart-3': BarChart3, bar_chart_3: BarChart3, barchart3: BarChart3,
		users: Users, 'trending-up': TrendingUp, trending_up: TrendingUp,
		activity: Activity, globe: Globe, star: Star, trophy: Trophy,
		calendar: Calendar, 'message-square': MessageSquare, message_square: MessageSquare,
		'book-open': BookOpen, book_open: BookOpen, bookmark: Bookmark, bell: Bell, mail: Mail,
		hash: Hash,
	};
	// lucide 1.x removed brand icons — map platforms to generic icons:
	// social handles → AtSign, web/blog → Globe, email → Mail, feeds → Rss.
	const socialIconByPlatform: Record<string, typeof Share2> = {
		website: Globe, web: Globe, blog: Globe, url: Globe,
		email: Mail, mail: Mail, newsletter: Mail,
		rss: Rss, feed: Rss,
		twitter: AtSign, x: AtSign, instagram: AtSign, facebook: AtSign,
		youtube: AtSign, github: AtSign, linkedin: AtSign, tiktok: AtSign,
		mastodon: AtSign, threads: AtSign, bluesky: AtSign,
		link: Link,
	};

	// --- Client-side data fetching for data-driven widgets (Rule 16: API-only) ---
	// Each fetchable widget type has its own $state + $effect. Widgets without a
	// backend endpoint render an empty state instead of a permanent "Loading…".

	interface FeaturedPost { id: string; title: string; slug: string; content_type?: string; }
	interface SpaceItem { id: string; name: string; slug: string; member_count: number; }
	interface SpaceTypeItem { id: string; name: string; slug: string; icon?: string | null; }
	interface RecentContentItem { id: string; title: string; slug: string; content_type: string; }
	interface BookmarkFolderItem { id: string; name: string; bookmark_count?: number; }
	interface LeaderboardUser { id: string; username: string; display_name?: string; avatar_url?: string; score: number; }
	interface EventItem { id: string; title: string; starts_at?: string; url?: string; }

	let featuredPosts = $state<FeaturedPost[]>([]);
	let spaces = $state<SpaceItem[]>([]);
	let recentContent = $state<RecentContentItem[]>([]);
	let suggestedContent = $state<RecentContentItem[]>([]);
	let bookmarkFolders = $state<BookmarkFolderItem[]>([]);
	let enrollments = $state<LearnEnrollment[]>([]);
	let leaderboard = $state<LeaderboardUser[]>([]);
	let events = $state<EventItem[]>([]);
	let spaceTypes = $state<SpaceTypeItem[]>([]);
	// Last path for which the context widgets resolved, so they recompute on
	// client navigation without depending on this state directly.
	let resolvedForPath = $state(page.url.pathname);

	// --- Learning widgets state ---
	let featuredTrack = $state<CatalogTrack | null>(null);
	let featuredTrackLoading = $state(false);

	// --- Context widgets (user_card, related_discussions on /org/* and /u/* pages) ---
	interface UserCardData { id: string; username: string; display_name?: string; avatar_url?: string; bio?: string; }
	interface RelatedDiscussion { id: string; title: string; slug: string; reply_count?: number; }

	// Resolve context widgets from the already-loaded page data where possible.
	// Returns `undefined` when the data is not present and a network call is needed.
	// CPT single routes share one page context (ADR-058 Exception #1):
	// /content/[slug] and /blog/[slug] expose `article`; /story/[slug]
	// exposes `story`. Both carry {id, author_id, author_name, avatar}.
	function isContentSingleRoute(): boolean {
		const id = page.route.id ?? '';
		return (
			id.endsWith('/content/[slug]') ||
			id.endsWith('/blog/[slug]') ||
			id.endsWith('/guide/[slug]') ||
			id.endsWith('/story/[slug]')
		);
	}

	interface ContentArticleContext {
		id?: string;
		author_id?: string;
		author_username?: string;
		author_name?: string;
		author_avatar_url?: string | null;
		author_avatar?: string | null;
	}

	function contentArticle(): ContentArticleContext | undefined {
		const d = page.data as Record<string, unknown>;
		return (d.article ?? d.story) as ContentArticleContext | undefined;
	}

	function resolveUserCardFromPageData(): UserCardData | null | undefined {
		const source = widgetProps.source;
		const pathParts = page.url.pathname.split('/').filter(Boolean);

		if (source === 'org_owner') {
			// Only resolve on the actual org profile route. Reserved static
			// segments under /org (e.g. /org/new, /org/invite/[token]) are not
			// org slugs — treating them as such causes 404 fetch noise.
			if (!page.route.id?.endsWith('/org/[slug]')) return null;
			const orgSlug = pathParts[1];
			if (!orgSlug) return null;
			const pageData = page.data as Record<string, unknown>;
			// /org/[slug] load returns the org under `data.org` (not
			// `data.organization`) — reading the right key avoids a redundant
			// client-side fetch of the same endpoint. Distinguish "org data
			// absent" (fetch needed) from "owner absent" (null → no fetch).
			const orgData = pageData.org as { owner?: UserCardData | null } | undefined;
			if (orgData === undefined) return undefined;
			return orgData.owner ?? null;
		}

		if (pathParts[0] === 'u' && pathParts[1]) {
			const pageData = page.data as Record<string, unknown>;
			const profileUser = pageData.user as
				| { id: string; username: string; display_name?: string; avatar_url?: string | null; bio?: string | null }
				| undefined;
			if (profileUser?.username === pathParts[1]) {
				return {
					id: profileUser.id,
					username: profileUser.username,
					display_name: profileUser.display_name,
					avatar_url: profileUser.avatar_url ?? undefined,
					bio: profileUser.bio ?? undefined
				};
			}
			// On a profile route but the page data is not the profile user — may still be loading.
			return undefined;
		}

		// Content single pages (/content/[slug], /blog/[slug], /story/[slug]):
		// the widget's natural context is the article author. An explicitly
		// configured user_id/username prop wins and requires a fetch by username.
		if (isContentSingleRoute()) {
			const configured =
				(typeof widgetProps.user_id === 'string' && widgetProps.user_id.trim()) ||
				(typeof widgetProps.username === 'string' && widgetProps.username.trim()) ||
				'';
			if (configured) return undefined;
			const article = contentArticle();
			if (!article?.author_id) return null;
			return {
				id: article.author_id,
				username: article.author_username ?? '',
				display_name: article.author_name,
				avatar_url: article.author_avatar_url ?? article.author_avatar ?? undefined
			};
		}

		// Not a context page that has a user card.
		return null;
	}

	function resolveRelatedDiscussionsFromPageData(): RelatedDiscussion[] | undefined {
		const pathParts = page.url.pathname.split('/').filter(Boolean);
		const orgSlug = pathParts[0] === 'org' ? pathParts[1] : null;
		const profileUsername = pathParts[0] === 'u' ? pathParts[1] : null;

		if (orgSlug) {
			// Org pages do not load the org's discussion list into page data; fetch it.
			return undefined;
		}

		if (profileUsername) {
			const pageData = page.data as Record<string, unknown>;
			const profileUser = pageData.user as { id: string; username: string } | undefined;
			const posts = pageData.posts as
				| Array<{ id: string; content_type: string; title: string; slug: string; comment_count?: number }>
				| undefined;
			if (profileUser?.username === profileUsername && posts) {
				return posts
					.filter((item) => item.content_type === 'discussion')
					.slice(0, limit)
					.map((item) => ({
						id: item.id,
						title: item.title,
						slug: item.slug,
						reply_count: item.comment_count
					}));
			}
			return undefined;
		}

		// Content single pages (/content/[slug], /blog/[slug], /story/[slug]):
		// linked discussion threads come from GET /content/{id}/discussions
		// (§49) — not present in page data, so a fetch is required.
		if (isContentSingleRoute()) {
			return undefined;
		}

		// Not a context page with related discussions.
		return [];
	}

	const initialUserCard = resolveUserCardFromPageData();
	const initialRelatedDiscussions = resolveRelatedDiscussionsFromPageData();

	let userCardData = $state<UserCardData | null>(initialUserCard ?? null);
	let relatedDiscussions = $state<RelatedDiscussion[]>(initialRelatedDiscussions ?? []);
	let dataLoaded = $state<Record<string, boolean>>({
		...(initialUserCard !== undefined ? { user_card: true } : {}),
		...(initialRelatedDiscussions !== undefined ? { related_discussions: true } : {})
	});

	// Newsletter subscribe form state (Rule 16: calls POST /newsletter/subscribe).
	// The endpoint is gated behind the feature.newsletter flag — when the
	// feature is off the widget hides entirely instead of 404ing on submit.
	const newsletterEnabled = $derived(
		(page.data?.features as Record<string, boolean> | undefined)?.['feature.newsletter'] !== false
	);
	let newsletterEmail = $state('');
	let newsletterSubmitting = $state(false);
	let newsletterMessage = $state<{ kind: 'success' | 'error' | 'info'; text: string } | null>(null);

	async function handleNewsletterSubmit(e: SubmitEvent) {
		e.preventDefault();
		if (newsletterSubmitting) return;
		const email = newsletterEmail.trim();
		if (!email || !email.includes('@') || !email.includes('.')) {
			newsletterMessage = { kind: 'error', text: t('sidebar.newsletter_invalid') };
			return;
		}
		newsletterSubmitting = true;
		newsletterMessage = null;
		const res = await api.post<{ status?: string }>('/newsletter/subscribe', { email });
		newsletterSubmitting = false;
		if (res.error) {
			newsletterMessage = { kind: 'error', text: t('sidebar.newsletter_error') };
			return;
		}
		// The backend returns the subscriber object with status='subscribed'
		// for both new and re-subscribes (idempotent upsert, §62). There is
		// no 'already_subscribed' status code — treat any success as subscribed.
		newsletterMessage = { kind: 'success', text: t('sidebar.newsletter_success') };
		newsletterEmail = '';
	}

	$effect(() => {
		const wt = widget.widget_type;
		const currentPath = page.url.pathname;
		if (untrack(() => resolvedForPath) !== currentPath) {
			resolvedForPath = currentPath;
			if (wt === 'user_card' || wt === 'related_discussions') {
				dataLoaded = { ...dataLoaded, [wt]: false };
			}
		}
		if (dataLoaded[wt]) return;

		async function fetchFeatured() {
			const path = page.url.pathname;
			const isForum = path === '/forum' || path.startsWith('/forum/');
			const endpoint = isForum
				? `/content?type=discussion&featured=1&page_size=${limit}`
				: '/blog/featured';
			const res = await api.get<FeaturedPost[]>(endpoint);
			featuredPosts = res.data ?? [];
			dataLoaded = { ...dataLoaded, featured_content: true };
		}
		async function fetchSpaces() {
			// The registry advertises an optional `level` prop — /spaces accepts
			// a level filter (e.g. country, city) so it must be forwarded.
			const levelFilter = typeof widgetProps.level === 'string' ? widgetProps.level.trim() : '';
			const res = await api.get<SpaceItem[]>(`/spaces?page_size=${limit}${levelFilter ? `&level=${encodeURIComponent(levelFilter)}` : ''}`);
			spaces = res.data ?? [];
			dataLoaded = { ...dataLoaded, space_list: true };
		}
		async function fetchRecent() {
			if (!user) { dataLoaded = { ...dataLoaded, recent_content: true }; return; }
			// GET /content has no sort param — it always returns newest-first
			// (ORDER BY created_at DESC), which is what this widget wants. The
			// registry's optional space_id prop scopes the list to one space.
			const spaceID = typeof widgetProps.space_id === 'string' ? widgetProps.space_id.trim() : '';
			const res = await api.get<RecentContentItem[]>(`/content?page_size=${limit}${spaceID ? `&space_id=${encodeURIComponent(spaceID)}` : ''}`);
			recentContent = res.data ?? [];
			dataLoaded = { ...dataLoaded, recent_content: true };
		}
		async function fetchSuggestedContent() {
			if (!user) { dataLoaded = { ...dataLoaded, suggested_content: true }; return; }
			const res = await api.get<RecentContentItem[]>(`/suggestions/content?limit=${limit}`);
			suggestedContent = res.data ?? [];
			dataLoaded = { ...dataLoaded, suggested_content: true };
		}
		async function fetchBookmarks() {
			if (!user) { dataLoaded = { ...dataLoaded, bookmark_folders: true }; return; }
			const res = await api.get<BookmarkFolderItem[]>('/me/bookmark-folders');
			bookmarkFolders = res.data ?? [];
			dataLoaded = { ...dataLoaded, bookmark_folders: true };
		}
		async function fetchEnrollments() {
			// learning_enrollments, continue_learning and progress_ring all share
			// the same /me/enrollments payload — every variant key must be marked
			// loaded, otherwise dataLoaded[wt] stays falsy and the effect
			// re-fetches in an infinite loop with a stuck "Loading…" state.
			const loaded = {
				...dataLoaded,
				learning_enrollments: true,
				continue_learning: true,
				progress_ring: true
			};
			if (!user) { dataLoaded = loaded; return; }
			const res = await api.get<LearnEnrollment[]>('/me/enrollments');
			enrollments = res.data ?? [];
			dataLoaded = loaded;
		}
		async function fetchFeaturedTrack() {
			const configuredSlug = typeof widgetProps.track_slug === 'string' ? widgetProps.track_slug : '';
			featuredTrackLoading = true;
			try {
				if (configuredSlug) {
					// /learn/university/tracks/{slug} returns the flat TrackDetail
					// DTO (enrolled_count, not the catalog's total_enrolled) — the
					// render reads both so either shape works.
					const res = await api.get<CatalogTrack>(`/learn/university/tracks/${encodeURIComponent(configuredSlug)}`);
					featuredTrack = res.data ?? null;
				} else {
					const res = await api.get<CatalogTrack[]>('/learn/university/catalog?limit=1');
					featuredTrack = (res.data ?? [])[0] ?? null;
				}
			} catch {
				featuredTrack = null;
			} finally {
				featuredTrackLoading = false;
				dataLoaded = { ...dataLoaded, track_card: true };
			}
		}
		async function fetchLeaderboard() {
			// Gamification leaderboard (Section 64) — the same points ranking the
			// /leaderboard page renders. The endpoint is gated behind the
			// gamification.enabled feature flag — skip the request when the
			// feature is off so the widget doesn't render a misleading
			// "No leaderboard data" empty state (consistent with
			// upcoming_events / calendar.enabled).
			const gamificationEnabled =
				(page.data?.features as Record<string, boolean> | undefined)?.['gamification.enabled'] === true;
			if (!gamificationEnabled) {
				leaderboard = [];
				dataLoaded = { ...dataLoaded, leaderboard: true };
				return;
			}
			// The response uses `points`/`user_id`; map into the widget's
			// LeaderboardUser shape (score/id).
			const res = await api.get<Array<{
				user_id: string;
				username: string;
				display_name?: string;
				avatar_url?: string | null;
				points: number;
			}>>(`/gamification/leaderboard?period=all_time&limit=${limit}`);
			leaderboard = (res.data ?? []).map((e) => ({
				id: e.user_id,
				username: e.username,
				display_name: e.display_name,
				avatar_url: e.avatar_url ?? undefined,
				score: e.points
			}));
			dataLoaded = { ...dataLoaded, leaderboard: true };
		}
		async function fetchEvents() {
			// Community Calendar (§86) supplies upcoming events via GET /calendar.
			// The endpoint is gated behind the calendar.enabled feature flag —
			// skip the request entirely when the feature is off so guests/users
			// don't generate console 404s.
			const calendarEnabled =
				(page.data?.features as Record<string, boolean> | undefined)?.['calendar.enabled'] === true;
			if (!calendarEnabled) {
				events = [];
				dataLoaded = { ...dataLoaded, upcoming_events: true };
				return;
			}
			interface CalendarEntryLite { id: string; title: string; occurs_at?: string; }
			const from = new Date().toISOString();
			const dest = page.url.searchParams.get('dest')
				?? user?.current_geo_space_id
				?? user?.primary_space?.id
				?? '';
			const destParam = dest ? `&dest=${encodeURIComponent(dest)}` : '';
			const res = await api.get<CalendarEntryLite[]>(
				`/calendar?view=list&from=${encodeURIComponent(from)}${destParam}&page_size=${limit}`
			);
			events = (res.data ?? []).map((e) => ({
				id: e.id,
				title: e.title,
				starts_at: e.occurs_at,
				url: `/calendar/${e.id}`
			}));
			dataLoaded = { ...dataLoaded, upcoming_events: true };
		}
		// space_categories fetches the tenant's space types (Section 05/39) —
		// the widget links each type to the filtered /spaces directory.
		async function fetchSpaceCategories() {
			const res = await api.get<SpaceTypeItem[]>('/space-types');
			spaceTypes = res.data ?? [];
			dataLoaded = { ...dataLoaded, space_categories: true };
		}
		// fetchUserCard resolves the user to display in the user_card widget.
		// Two contexts:
		//   1. source=org_owner on /org/[slug] — reads the owner from the org API.
		//   2. /u/[username] profile page — fetches the profile user by username.
		async function fetchUserCard() {
			const source = widgetProps.source;
			const pathParts = page.url.pathname.split('/').filter(Boolean);
			if (source === 'org_owner') {
				// Only fetch on the org profile route — reserved static segments
				// under /org (e.g. /org/new, /org/invite/[token]) are not slugs.
				const orgSlug = page.route.id?.endsWith('/org/[slug]') ? pathParts[1] : null;
				if (!orgSlug) {
					dataLoaded = { ...dataLoaded, user_card: true };
					return;
				}
				try {
					const orgRes = await api.get<{ owner?: UserCardData }>(`/organizations/${orgSlug}`);
					userCardData = orgRes.data?.owner ?? null;
				} catch {
					userCardData = null;
				}
				dataLoaded = { ...dataLoaded, user_card: true };
				return;
			}
			// Profile page: /u/[username]
			if (pathParts[0] === 'u' && pathParts[1]) {
				const pageData = page.data as Record<string, unknown>;
				const profileUser = pageData.user as
					| { id: string; username: string; display_name?: string; avatar_url?: string | null; bio?: string | null }
					| undefined;
				if (profileUser?.username === pathParts[1]) {
					userCardData = {
						id: profileUser.id,
						username: profileUser.username,
						display_name: profileUser.display_name,
						avatar_url: profileUser.avatar_url ?? undefined,
						bio: profileUser.bio ?? undefined
					};
				} else {
					try {
						const res = await api.get<UserCardData>(`/users/${pathParts[1]}`);
						userCardData = res.data ?? null;
					} catch {
						userCardData = null;
					}
				}
				dataLoaded = { ...dataLoaded, user_card: true };
				return;
			}
			// Content single pages (/content/[slug], /blog/[slug],
			// /story/[slug]): a configured user_id/username prop fetches that
			// profile; otherwise the widget shows the article author already
			// present in page data.
			if (isContentSingleRoute()) {
				const configured =
					(typeof widgetProps.user_id === 'string' && widgetProps.user_id.trim()) ||
					(typeof widgetProps.username === 'string' ? widgetProps.username.trim() : '');
				if (configured) {
					try {
						const res = await api.get<UserCardData>(`/users/${encodeURIComponent(configured)}`);
						userCardData = res.data ?? null;
					} catch {
						userCardData = null;
					}
				} else {
					userCardData = resolveUserCardFromPageData() ?? null;
				}
				dataLoaded = { ...dataLoaded, user_card: true };
				return;
			}
			dataLoaded = { ...dataLoaded, user_card: true };
		}
		// fetchRelatedDiscussions fetches forum threads related to the current
		// page context. On /org/[slug] it fetches the org's discussions; on
		// /u/[username] it fetches the user's discussion-type content.
		async function fetchRelatedDiscussions() {
			const pathParts = page.url.pathname.split('/').filter(Boolean);
			// Only treat the second segment as an org slug on the actual
			// /org/[slug] route — /org/new and /org/invite/[token] are static
			// routes, not org profiles.
			const orgSlug = page.route.id?.endsWith('/org/[slug]') ? pathParts[1] : null;
			const profileUsername = pathParts[0] === 'u' ? pathParts[1] : null;
			if (orgSlug) {
				try {
					const orgRes = await api.get<{ id: string }>(`/organizations/${orgSlug}`);
					if (!orgRes.data?.id) { dataLoaded = { ...dataLoaded, related_discussions: true }; return; }
					const res = await api.get<RelatedDiscussion[]>(`/organizations/${orgRes.data.id}/content`);
					relatedDiscussions = (res.data ?? [])
						.filter((item) => (item as unknown as Record<string, unknown>).content_type === 'discussion')
						.slice(0, limit);
				} catch {
					relatedDiscussions = [];
				}
				dataLoaded = { ...dataLoaded, related_discussions: true };
				return;
			}
			if (profileUsername) {
				const pageData = page.data as Record<string, unknown>;
				const profileUser = pageData.user as { id: string; username: string } | undefined;
				const posts = pageData.posts as Array<{ id: string; content_type: string; title: string; slug: string; comment_count?: number }> | undefined;
				if (profileUser?.id && posts && profileUser.username === profileUsername) {
					relatedDiscussions = posts
						.filter((item) => item.content_type === 'discussion')
						.slice(0, limit)
						.map((item) => ({
							id: item.id,
							title: item.title,
							slug: item.slug,
							reply_count: item.comment_count
						}));
				} else {
					try {
						const userRes = await api.get<{ id: string }>(`/users/${profileUsername}`);
						if (!userRes.data?.id) { dataLoaded = { ...dataLoaded, related_discussions: true }; return; }
						const res = await api.get<RelatedDiscussion[]>(`/users/${userRes.data.id}/content`);
						relatedDiscussions = (res.data ?? [])
							.filter((item) => (item as unknown as Record<string, unknown>).content_type === 'discussion')
							.slice(0, limit);
					} catch {
						relatedDiscussions = [];
					}
				}
				dataLoaded = { ...dataLoaded, related_discussions: true };
				return;
			}
			// Content single pages (/content/[slug], /blog/[slug],
			// /story/[slug]): §49 thread links — GET /content/{id}/discussions
			// returns the discussion threads cross-linked to this article.
			if (isContentSingleRoute()) {
				const article = contentArticle();
				if (article?.id) {
					interface LinkedDiscussion {
						thread_id: string;
						thread_title: string;
						thread_slug: string;
						thread_comment_count?: number;
					}
					try {
						const res = await api.get<LinkedDiscussion[]>(`/content/${article.id}/discussions?page_size=${limit}`);
						relatedDiscussions = (res.data ?? []).map((d) => ({
							id: d.thread_id,
							title: d.thread_title,
							slug: d.thread_slug,
							reply_count: d.thread_comment_count
						}));
					} catch {
						relatedDiscussions = [];
					}
				}
				dataLoaded = { ...dataLoaded, related_discussions: true };
				return;
			}
			dataLoaded = { ...dataLoaded, related_discussions: true };
		}

		if (wt === 'featured_content') fetchFeatured();
		else if (wt === 'space_list') fetchSpaces();
		else if (wt === 'recent_content') fetchRecent();
		else if (wt === 'suggested_content') fetchSuggestedContent();
		else if (wt === 'bookmark_folders') fetchBookmarks();
		else if (wt === 'learning_enrollments' || wt === 'continue_learning' || wt === 'progress_ring') fetchEnrollments();
		else if (wt === 'track_card') fetchFeaturedTrack();
		else if (wt === 'leaderboard') fetchLeaderboard();
		else if (wt === 'upcoming_events') fetchEvents();
		else if (wt === 'user_card') fetchUserCard();
		else if (wt === 'related_discussions') fetchRelatedDiscussions();
		else if (wt === 'space_categories') fetchSpaceCategories();
	});
</script>

{#if widget.widget_type === 'search'}
	<SearchAutocomplete />
{:else if widget.widget_type === 'theme_toggle'}
	<button
		type="button"
		onclick={() => theme.toggle()}
		class="grid h-9 w-9 shrink-0 place-items-center rounded-full text-[var(--color-muted)] hover:bg-[var(--color-surface-subtle)]"
		aria-label={theme.current === 'dark' ? t('theme.toggle_light') : t('theme.toggle_dark')}
	>
		{#if theme.current === 'dark'}<Sun size={18} />{:else}<Moon size={18} />{/if}
	</button>
{:else if widget.widget_type === 'language_toggle'}
	{#if showLanguageToggle}
		<DropdownMenu.Root>
			<DropdownMenu.Trigger
				class="grid h-9 w-9 shrink-0 place-items-center rounded-full text-[var(--color-muted)] hover:bg-[var(--color-surface-subtle)]"
				aria-label={t('locale.switch_language', 'Switch language')}
				title={localeLabel(i18n.locale)}
			>
				<Languages size={18} />
			</DropdownMenu.Trigger>
			<DropdownMenu.Portal>
				<DropdownMenu.Content class="z-50 max-h-60 overflow-y-auto rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-1 shadow-lg">
					{#each langList as lang (lang)}
						<DropdownMenu.Item
							onclick={() => switchLanguage(lang)}
							class="flex cursor-pointer items-center justify-between rounded px-2 py-1.5 text-sm text-[var(--color-text)] data-[highlighted]:bg-[var(--color-surface-subtle)]"
						>
							<span>{localeLabel(lang)}</span>
							{#if lang === i18n.locale}<Check size={14} class="text-[var(--color-primary)]" />{/if}
						</DropdownMenu.Item>
					{/each}
				</DropdownMenu.Content>
			</DropdownMenu.Portal>
		</DropdownMenu.Root>
	{/if}
{:else if widget.widget_type === 'user_dropdown'}
	<UserDropdown {user} />
{:else if widget.widget_type === 'ad_slot'}
	{#if !isDashboard}
		<AdSlot slot={adSlot} format={adFormat} />
	{/if}
{:else if widget.widget_type === 'trending_tags'}
	{#if trendingTags.length > 0}
		<section class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
			<h2 class="mb-3 text-sm font-medium text-[var(--color-text)]">{title || t('sidebar.trending_tags')}</h2>
			<ul class="flex flex-wrap gap-2">
				{#each trendingTags.slice(0, limit) as tag (tag.id)}
					<li>
						<a
							href={`/tags/${tag.slug}`}
							class="inline-block rounded-full bg-[var(--color-surface-subtle)] px-3 py-1 text-xs text-[var(--color-text-subtle)] hover:text-[var(--color-primary)]"
							>#{tag.name}</a
						>
					</li>
				{/each}
			</ul>
		</section>
	{/if}
{:else if widget.widget_type === 'suggested_spaces'}
	{#if suggestedSpaces.length > 0}
		<section class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
			<h2 class="mb-3 text-sm font-medium text-[var(--color-text)]">{title || t('sidebar.suggested_spaces')}</h2>
			<ul class="space-y-2">
				{#each suggestedSpaces.slice(0, limit) as space (space.id)}
					<li>
						<a href={`/spaces/${space.slug}`} class="flex items-center gap-2 rounded-[var(--radius)] p-1.5 hover:bg-[var(--color-surface-subtle)]">
							<span class="grid h-8 w-8 shrink-0 place-items-center rounded-[var(--radius)] bg-[var(--color-surface-subtle)] text-xs font-medium text-[var(--color-text-subtle)]">{space.name.charAt(0).toUpperCase()}</span>
							<span class="min-w-0 flex-1">
								<span class="block truncate text-sm text-[var(--color-text)]">{space.name}</span>
								{#if space.member_count != null}
									<span class="block text-xs text-[var(--color-muted)]">{t('meta.members', '{n} members').replace('{n}', String(space.member_count))}</span>
								{/if}
							</span>
						</a>
					</li>
				{/each}
			</ul>
		</section>
	{/if}
{:else if widget.widget_type === 'rich_text'}
	<!-- Rich text widget — renders admin-authored HTML. Sanitized server-side
	     via internal/content.SanitizeHTML (BE-4) and client-side via
	     sanitizeSidebarHtml as defense in depth. Ref: §5.1, Section 59. -->
	{#if typeof widgetProps.content === 'string' && widgetProps.content.trim()}
		<section class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4 prose prose-sm max-w-none">
			{@html sanitizeSidebarHtml(widgetProps.content)}
		</section>
	{/if}
{:else if widget.widget_type === 'custom_html'}
	<!-- Custom HTML widget — admin-authored HTML rendered in a sandboxed
	     iframe (no allow-scripts) so any injected script cannot run. Also
	     sanitized server-side (BE-4) and via sanitizeSidebarHtml as defense
	     in depth. Ref: §5.1, Section 59. -->
	{#if typeof widgetProps.html === 'string' && widgetProps.html.trim()}
		<iframe
			class="sidebar-widget-custom-html w-full rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)]"
			sandbox="allow-same-origin"
			srcdoc={sanitizeSidebarHtml(widgetProps.html)}
			title={widget.label || 'Custom HTML'}
		></iframe>
	{/if}
{:else if widget.widget_type === 'recent_content'}
	<section class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
		<h2 class="mb-3 text-sm font-medium text-[var(--color-text)]">{title || t('sidebar.recent_posts', 'Recent Posts')}</h2>
		{#if !user}
			<p class="text-xs text-[var(--color-muted)]">{t('sidebar.sign_in_to_see', 'Sign in to see recent posts')}</p>
		{:else if recentContent.length === 0}
			<p class="text-xs text-[var(--color-muted)]">{t('sidebar.no_content', 'No recent posts')}</p>
		{:else}
			<ul class="space-y-2">
				{#each recentContent.slice(0, limit) as item (item.id)}
					<li>
						<a href={contentHref(item)} class="block truncate text-sm text-[var(--color-text)] hover:text-[var(--color-primary)]">{item.title}</a>
					</li>
				{/each}
			</ul>
		{/if}
	</section>
{:else if widget.widget_type === 'suggested_content'}
	<section class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
		<h2 class="mb-3 text-sm font-medium text-[var(--color-text)]">{title || t('sidebar.suggested_content', 'Suggested for you')}</h2>
		{#if !user}
			<p class="text-xs text-[var(--color-muted)]">{t('sidebar.sign_in_for_suggestions', 'Sign in to see suggestions')}</p>
		{:else if suggestedContent.length === 0}
			<p class="text-xs text-[var(--color-muted)]">{t('sidebar.no_suggestions', 'No suggestions right now')}</p>
		{:else}
			<ul class="space-y-2">
				{#each suggestedContent.slice(0, limit) as item (item.id)}
					<li>
						<a href={contentHref(item)} class="block truncate text-sm text-[var(--color-text)] hover:text-[var(--color-primary)]">{item.title}</a>
					</li>
				{/each}
			</ul>
		{/if}
	</section>
{:else if widget.widget_type === 'space_list'}
	<section class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
		<h2 class="mb-3 text-sm font-medium text-[var(--color-text)]">{title || t('sidebar.spaces', 'Spaces')}</h2>
		{#if spaces.length === 0}
			<p class="text-xs text-[var(--color-muted)]">{t('sidebar.no_spaces', 'No spaces available')}</p>
		{:else}
			<ul class="space-y-2">
				{#each spaces.slice(0, limit) as space (space.id)}
					<li>
						<a href={`/spaces/${space.slug}`} class="flex items-center gap-2 rounded-[var(--radius)] p-1.5 hover:bg-[var(--color-surface-subtle)]">
							<span class="grid h-8 w-8 shrink-0 place-items-center rounded-[var(--radius)] bg-[var(--color-surface-subtle)] text-xs font-medium text-[var(--color-text-subtle)]">{space.name.charAt(0).toUpperCase()}</span>
							<span class="min-w-0 flex-1">
								<span class="block truncate text-sm text-[var(--color-text)]">{space.name}</span>
								<span class="block text-xs text-[var(--color-muted)]">{t('meta.members', '{n} members').replace('{n}', String(space.member_count))}</span>
							</span>
						</a>
					</li>
				{/each}
			</ul>
		{/if}
	</section>
{:else if widget.widget_type === 'notification_centre'}
	{#if user}
		<NotificationCentre side="bottom" align="end" />
	{/if}
{:else if widget.widget_type === 'metric_card'}
	{@const MetricIcon = iconByName[String(widgetProps.icon ?? '').toLowerCase().replace(/[\s]+/g, '-')] ?? BarChart3}
	<section class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
		<div class="flex items-center gap-2 text-[var(--color-text-subtle)]">
			<MetricIcon size={16} />
			<span class="text-xs font-medium">{typeof widgetProps.label === 'string' ? widgetProps.label : t('sidebar.metric')}</span>
		</div>
		<p class="mt-2 text-2xl font-bold text-[var(--color-text)]">{typeof widgetProps.value === 'string' ? widgetProps.value : '—'}</p>
		{#if typeof widgetProps.delta === 'string' && widgetProps.delta}
			<p class="mt-1 text-xs text-[var(--color-text-subtle)]">{widgetProps.delta}</p>
		{/if}
	</section>
{:else if widget.widget_type === 'newsletter_signup'}
	{#if newsletterEnabled}
	<section class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
		<div class="mb-3 flex items-center gap-2">
			<Mail size={18} class="text-[var(--color-primary)]" />
			<h2 class="text-sm font-medium text-[var(--color-text)]">{title || t('sidebar.newsletter')}</h2>
		</div>
		<form class="space-y-2" onsubmit={handleNewsletterSubmit}>
			<input
				type="email"
				bind:value={newsletterEmail}
				placeholder={typeof widgetProps.placeholder === 'string' ? widgetProps.placeholder : t('sidebar.email_placeholder')}
				disabled={newsletterSubmitting}
				class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-bg)] px-3 py-2 text-sm text-[var(--color-text)] focus:border-[var(--color-primary)] focus:outline-none disabled:opacity-60"
			/>
			<button
				type="submit"
				disabled={newsletterSubmitting}
				class="w-full rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)] disabled:opacity-60"
			>
				{typeof widgetProps.button_text === 'string' ? widgetProps.button_text : t('sidebar.subscribe')}
			</button>
		</form>
		{#if newsletterMessage}
			<p
				class="mt-2 text-xs {newsletterMessage.kind === 'error'
					? 'text-[var(--color-danger)]'
					: newsletterMessage.kind === 'success'
						? 'text-[var(--color-success)]'
						: 'text-[var(--color-muted)]'}"
			>
				{newsletterMessage.text}
			</p>
		{/if}
	</section>
	{/if}
{:else if widget.widget_type === 'social_links'}
	<!-- Only render when at least one configured link has a URL — an empty
	     "Follow Us" card is dead space on every page otherwise. -->
	{@const validLinks = (Array.isArray(widgetProps.links) ? widgetProps.links : []).filter(
		(l) => typeof l === 'object' && l !== null && typeof (l as { url?: unknown }).url === 'string' && ((l as { url: string }).url).trim() !== ''
	)}
	{#if validLinks.length > 0}
		<section class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
			{#if title}<h2 class="mb-3 text-sm font-medium text-[var(--color-text)]">{title}</h2>{/if}
			<div class="flex flex-wrap gap-2">
				{#each validLinks as link, i (String(link.platform ?? '') + '-' + i)}
					{@const SocialIcon = socialIconByPlatform[String(link.platform ?? '').toLowerCase().trim()] ?? Share2}
					<a
						href={safeHref(link.url)}
						target="_blank"
						rel="noopener noreferrer"
						class="grid h-9 w-9 place-items-center rounded-full bg-[var(--color-surface-subtle)] text-[var(--color-text-subtle)] hover:bg-[var(--color-primary)] hover:text-[var(--color-primary-contrast)]"
						aria-label={link.platform}
					>
						<SocialIcon size={16} />
					</a>
				{/each}
			</div>
		</section>
	{/if}
{:else if widget.widget_type === 'cta_button'}
	{@const ctaVariant = String(widgetProps.variant ?? 'primary').toLowerCase()}
	<section class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
		<a
			href={safeHref(typeof widgetProps.url === 'string' ? widgetProps.url : '/signup') || '/signup'}
			class="flex w-full items-center justify-center gap-2 rounded-[var(--radius)] px-4 py-2.5 text-sm font-semibold transition-colors {ctaVariant === 'secondary'
				? 'bg-[var(--color-surface-subtle)] text-[var(--color-text)] hover:bg-[var(--color-border)]'
				: ctaVariant === 'outline'
					? 'border border-[var(--color-primary)] text-[var(--color-primary)] hover:bg-[var(--color-primary)] hover:text-[var(--color-primary-contrast)]'
					: 'bg-[var(--color-primary)] text-[var(--color-primary-contrast)] hover:bg-[var(--color-primary-hover)]'}"
		>
			<MousePointerClick size={16} />
			{typeof widgetProps.label === 'string' ? widgetProps.label : t('sidebar.get_started')}
		</a>
	</section>
{:else if widget.widget_type === 'tag_cloud'}
	{#if trendingTags.length > 0}
		<section class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
			<h2 class="mb-3 text-sm font-medium text-[var(--color-text)]">{title || t('sidebar.popular_tags')}</h2>
			<div class="flex flex-wrap gap-1.5">
				{#each trendingTags.slice(0, limit) as tag, i (tag.id)}
					<a
						href={`/tags/${tag.slug}`}
						class="inline-block rounded-full bg-[var(--color-surface-subtle)] px-2.5 py-1 text-[var(--color-text-subtle)] hover:text-[var(--color-primary)]"
						style="font-size: {0.7 + (i < 5 ? 0.3 : 0)}rem"
					>#{tag.name}</a>
				{/each}
			</div>
		</section>
	{/if}
{:else if widget.widget_type === 'leaderboard'}
	<section class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
		<div class="mb-3 flex items-center gap-2">
			<Trophy size={18} class="text-[var(--color-primary)]" />
			<h2 class="text-sm font-medium text-[var(--color-text)]">{title || t('sidebar.top_contributors')}</h2>
		</div>
		{#if leaderboard.length === 0}
			<p class="text-xs text-[var(--color-muted)]">{t('sidebar.no_leaderboard')}</p>
		{:else}
			<ul class="space-y-2">
				{#each leaderboard.slice(0, limit) as entry, i (entry.id)}
					<li>
						<a href={`/u/${entry.username}`} class="flex items-center gap-2 rounded-[var(--radius)] p-1.5 hover:bg-[var(--color-surface-subtle)]">
							<span class="grid h-6 w-6 shrink-0 place-items-center rounded-full bg-[var(--color-surface-subtle)] text-xs font-semibold text-[var(--color-text-subtle)]">{i + 1}</span>
							{#if entry.avatar_url}
								<img src={entry.avatar_url} alt={entry.display_name ?? entry.username} class="h-8 w-8 shrink-0 rounded-full object-cover" />
							{:else}
								<span class="grid h-8 w-8 shrink-0 place-items-center rounded-full bg-[var(--color-surface-subtle)] text-xs font-medium text-[var(--color-text-subtle)]">{(entry.display_name ?? entry.username).charAt(0).toUpperCase()}</span>
							{/if}
							<span class="min-w-0 flex-1">
								<span class="block truncate text-sm text-[var(--color-text)]">{entry.display_name ?? entry.username}</span>
								<span class="block text-xs text-[var(--color-muted)]">@{entry.username}</span>
							</span>
							<span class="shrink-0 text-xs font-medium text-[var(--color-muted)]">
								{entry.score.toLocaleString()} {t('gamification.points', 'pts')}
							</span>
						</a>
					</li>
				{/each}
			</ul>
		{/if}
	</section>
{:else if widget.widget_type === 'featured_content'}
	<section class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
		<div class="mb-3 flex items-center gap-2">
			<Star size={18} class="text-[var(--color-primary)]" />
			<h2 class="text-sm font-medium text-[var(--color-text)]">{title || t('sidebar.featured', 'Featured')}</h2>
		</div>
		{#if featuredPosts.length === 0}
			<p class="text-xs text-[var(--color-muted)]">{t('sidebar.no_featured', 'No featured content')}</p>
		{:else}
			<ul class="space-y-2">
				{#each featuredPosts.slice(0, limit) as post (post.id)}
					<li>
						<a href={contentHref(post)} class="block truncate text-sm text-[var(--color-text)] hover:text-[var(--color-primary)]">{post.title}</a>
					</li>
				{/each}
			</ul>
		{/if}
	</section>
{:else if widget.widget_type === 'related_discussions'}
	<section class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
		<div class="mb-3 flex items-center gap-2">
			<MessageSquare size={18} class="text-[var(--color-text-subtle)]" />
			<h2 class="text-sm font-medium text-[var(--color-text)]">{title || t('sidebar.related_discussions', 'Related Discussions')}</h2>
		</div>
		{#if relatedDiscussions.length === 0}
			<p class="text-xs text-[var(--color-muted)]">{t('sidebar.no_discussions', 'No related discussions')}</p>
		{:else}
			<ul class="space-y-2">
				{#each relatedDiscussions as disc (disc.id)}
					<li>
						<a href={`/forum/${disc.slug}`} class="block truncate text-sm text-[var(--color-text)] hover:text-[var(--color-primary)]">{disc.title}</a>
					</li>
				{/each}
			</ul>
		{/if}
	</section>
{:else if widget.widget_type === 'upcoming_events'}
	<!-- Hidden while empty — there is no /events backend route yet, so this
	     widget must not render a permanent "No upcoming events" card on every
	     page (consistent with trending_tags/suggested_spaces hide-when-empty). -->
	{#if events.length > 0}
		<section class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
			<div class="mb-3 flex items-center gap-2">
				<Calendar size={18} class="text-[var(--color-text-subtle)]" />
				<h2 class="text-sm font-medium text-[var(--color-text)]">{title || t('sidebar.upcoming_events')}</h2>
			</div>
			<ul class="space-y-2">
				{#each events.slice(0, limit) as event (event.id)}
					<li>
						<a href={safeHref(event.url) || '#'} class="block text-sm text-[var(--color-text)] hover:text-[var(--color-primary)]">
							<span class="block truncate">{event.title}</span>
							{#if event.starts_at}
								<span class="mt-0.5 block text-xs text-[var(--color-muted)]">{new Date(event.starts_at).toLocaleDateString()}</span>
							{/if}
						</a>
					</li>
				{/each}
			</ul>
		</section>
	{/if}
{:else if widget.widget_type === 'bookmark_folders'}
	{#if user}
		<section class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
			<div class="mb-3 flex items-center gap-2">
				<Bookmark size={18} class="text-[var(--color-text-subtle)]" />
				<h2 class="text-sm font-medium text-[var(--color-text)]">{title || t('sidebar.bookmarks', 'Bookmarks')}</h2>
			</div>
			{#if bookmarkFolders.length === 0}
				<p class="text-xs text-[var(--color-muted)]">{t('sidebar.no_bookmarks', 'No bookmark folders')}</p>
			{:else}
				<ul class="space-y-1.5">
					{#each bookmarkFolders.slice(0, limit) as folder (folder.id)}
						<li>
							<a href={`/dashboard/bookmarks?folder=${folder.id}`} class="flex items-center justify-between text-sm text-[var(--color-text)] hover:text-[var(--color-primary)]">
								<span class="truncate">{folder.name}</span>
								{#if folder.bookmark_count != null}
									<span class="text-xs text-[var(--color-muted)]">{folder.bookmark_count}</span>
								{/if}
							</a>
						</li>
					{/each}
				</ul>
			{/if}
		</section>
	{/if}
{:else if widget.widget_type === 'user_card'}
	<section class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
		<div class="mb-3 flex items-center gap-2">
			<UserCircle size={18} class="text-[var(--color-text-subtle)]" />
			<h2 class="text-sm font-medium text-[var(--color-text)]">{title || t('sidebar.featured_user', 'Featured User')}</h2>
		</div>
		{#if userCardData}
			<!-- Article authors may lack a username in the DTO — render without
			     the /u/ link rather than linking to /u/ (empty). -->
			<svelte:element
				this={userCardData.username ? 'a' : 'div'}
				href={userCardData.username ? `/u/${userCardData.username}` : undefined}
				class="flex items-center gap-3 rounded-[var(--radius)] p-1.5 {userCardData.username ? 'hover:bg-[var(--color-surface-subtle)]' : ''}"
			>
				{#if userCardData.avatar_url}
					<img src={userCardData.avatar_url} alt={userCardData.display_name ?? userCardData.username} class="h-10 w-10 rounded-full object-cover" />
				{:else}
					<span class="grid h-10 w-10 shrink-0 place-items-center rounded-full bg-[var(--color-surface-subtle)] text-sm font-medium text-[var(--color-text-subtle)]">
						{(userCardData.display_name?.charAt(0) ?? userCardData.username?.charAt(0) ?? '?').toUpperCase()}
					</span>
				{/if}
				<span class="min-w-0 flex-1">
					<span class="block truncate text-sm font-medium text-[var(--color-text)]">{userCardData.display_name ?? userCardData.username}</span>
					{#if userCardData.username}
						<span class="block text-xs text-[var(--color-muted)]">@{userCardData.username}</span>
					{/if}
				</span>
			</svelte:element>
		{:else}
			<p class="text-xs text-[var(--color-muted)]">{t('sidebar.no_user', 'No user to display')}</p>
		{/if}
	</section>
{:else if widget.widget_type === 'space_categories'}
	<section class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
		<div class="mb-3 flex items-center gap-2">
			<FolderTree size={18} class="text-[var(--color-text-subtle)]" />
			<h2 class="text-sm font-medium text-[var(--color-text)]">{title || t('sidebar.browse_spaces', 'Browse Spaces')}</h2>
		</div>
		{#if spaceTypes.length > 0}
			<ul class="flex flex-wrap gap-2">
				{#each spaceTypes.slice(0, limit) as st (st.id)}
					<li>
						<a
							href={`/spaces?type=${st.slug}`}
							class="inline-block rounded-full bg-[var(--color-surface-subtle)] px-3 py-1 text-xs text-[var(--color-text-subtle)] hover:text-[var(--color-primary)]"
						>
							{st.name}
						</a>
					</li>
				{/each}
			</ul>
		{:else}
			<p class="text-xs text-[var(--color-muted)]">{t('sidebar.no_categories', 'No categories available')}</p>
		{/if}
	</section>
{:else if widget.widget_type === 'continue_learning' || widget.widget_type === 'learning_enrollments'}
	{#if user}
		<section class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
			<div class="mb-3 flex items-center gap-2">
				{#if widget.widget_type === 'continue_learning'}
					<PlayCircle size={18} class="text-[var(--color-primary)]" />
				{:else}
					<GraduationCap size={18} class="text-[var(--color-primary)]" />
				{/if}
				<h2 class="text-sm font-medium text-[var(--color-text)]">
					{title || t(widget.widget_type === 'continue_learning' ? 'sidebar.continue_learning' : 'sidebar.my_learning')}
				</h2>
			</div>
			{#if !dataLoaded[widget.widget_type]}
				<p class="text-xs text-[var(--color-muted)]">{t('common.loading', 'Loading…')}</p>
			{:else if enrollments.length === 0}
				<p class="text-xs text-[var(--color-muted)]">{t('sidebar.no_enrollments', 'No active enrollments')}</p>
			{:else}
				<!-- continue_learning shows only the most recent enrollment per
				     its registry contract; learning_enrollments shows the full list. -->
				<ul class="space-y-2">
					{#each enrollments.slice(0, widget.widget_type === 'continue_learning' ? 1 : limit) as enr (enr.id)}
						<li>
							<a href={safeLearnUrl(`/tracks/${encodeURIComponent(enr.track_slug)}`) || '#'} class="block text-sm text-[var(--color-text)] hover:text-[var(--color-primary)]">
								<span class="block truncate">{enr.track_title}</span>
								{#if (enr.lessons_total ?? 0) > 0}
									<span class="mt-0.5 block text-xs text-[var(--color-muted)]">
										{Math.round(enr.progress_pct ?? 0)}% · {enr.lessons_completed ?? 0}/{enr.lessons_total ?? 0} {t('learn.lessons', 'lessons')}
									</span>
								{:else}
									<span class="mt-0.5 block text-xs text-[var(--color-muted)]">{Math.round(enr.progress_pct ?? 0)}%</span>
								{/if}
							</a>
						</li>
					{/each}
				</ul>
			{/if}
		</section>
	{/if}
{:else if widget.widget_type === 'progress_ring'}
	{#if user}
		<section class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4 text-center">
			<h2 class="mb-3 text-sm font-medium text-[var(--color-text)]">{title || t('sidebar.my_progress', 'My Progress')}</h2>
			{#if !dataLoaded['progress_ring']}
				<div class="mx-auto grid h-20 w-20 place-items-center">
					<div class="jc-skeleton h-16 w-16 rounded-full"></div>
				</div>
				<p class="mt-2 text-xs text-[var(--color-muted)]">{t('common.loading', 'Loading…')}</p>
			{:else if enrollments.length === 0}
				<div class="mx-auto grid h-20 w-20 place-items-center">
					<ProgressRing value={0} />
				</div>
				<p class="mt-2 text-xs text-[var(--color-muted)]">{t('learn.empty', 'No learning tracks yet.')}</p>
				<a
					href={safeLearnUrl('/tracks') || '#'}
					class="mt-2 inline-block rounded-[var(--radius)] bg-[var(--color-primary)] px-3 py-1.5 text-sm font-medium text-[var(--color-primary-contrast)] hover:bg-[var(--color-primary-hover)]"
				>
					{t('learn.view_track', 'View track')}
				</a>
			{:else}
				{@const totalCompleted = enrollments.reduce((sum, e) => sum + (e.lessons_completed ?? 0), 0)}
				{@const totalLessons = enrollments.reduce((sum, e) => sum + (e.lessons_total ?? 0), 0)}
				{@const avgPct = totalLessons > 0 ? Math.round((totalCompleted / totalLessons) * 100) : 0}
				<div class="mx-auto grid h-20 w-20 place-items-center">
					<ProgressRing value={avgPct} />
				</div>
				<p class="mt-2 text-sm font-medium text-[var(--color-text)]">{avgPct}%</p>
				<p class="text-xs text-[var(--color-muted)]">{totalCompleted}/{totalLessons} {t('learn.lessons', 'lessons')}</p>
			{/if}
		</section>
	{/if}
{:else if widget.widget_type === 'track_card'}
	<section class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
		<div class="mb-3 flex items-center gap-2">
			<BookOpen size={18} class="text-[var(--color-primary)]" />
			<h2 class="text-sm font-medium text-[var(--color-text)]">{title || t('sidebar.featured_track', 'Featured Track')}</h2>
		</div>
		{#if !dataLoaded['track_card'] || featuredTrackLoading}
			<div class="space-y-2">
				<div class="jc-skeleton h-20 w-full rounded"></div>
				<div class="jc-skeleton h-4 w-3/4 rounded"></div>
				<div class="jc-skeleton h-3 w-1/2 rounded"></div>
			</div>
		{:else if featuredTrack}
			{@const enrolledCount = featuredTrack.total_enrolled ?? (featuredTrack as unknown as { enrolled_count?: number }).enrolled_count}
			<a
				href={safeLearnUrl(`/tracks/${encodeURIComponent(featuredTrack.slug)}`) || '#'}
				class="block rounded-[var(--radius)] p-2 hover:bg-[var(--color-surface-subtle)]"
			>
				{#if featuredTrack.thumbnail_url}
					<img
						src={featuredTrack.thumbnail_url}
						alt={featuredTrack.title}
						class="mb-2 h-20 w-full rounded-[var(--radius)] object-cover"
					/>
				{/if}
				<p class="truncate text-sm font-medium text-[var(--color-text)]">{featuredTrack.title}</p>
				<div class="mt-1.5 flex items-center gap-2 text-xs text-[var(--color-muted)]">
					{#if featuredTrack.level}
						<span class="rounded border border-[var(--color-border)] px-1.5 py-0.5 text-[10px] font-semibold uppercase">{featuredTrack.level}</span>
					{/if}
					{#if enrolledCount != null}
						<span>{enrolledCount} {t('learn.enrolled', 'enrolled')}</span>
					{/if}
				</div>
				{#if featuredTrack.description}
					<p class="mt-2 line-clamp-2 text-xs text-[var(--color-text-subtle)]">{featuredTrack.description}</p>
				{/if}
			</a>
		{:else}
			<p class="text-xs text-[var(--color-muted)]">{t('learn.empty', 'No learning tracks yet.')}</p>
		{/if}
	</section>
{:else if widget.widget_type === 'table_of_contents'}
	<!-- TOC widget — rendered by the Docs app; no-op in Main app. -->
{:else if widget.widget_type === 'search_launcher'}
	<!-- Search launcher — Docs/Learn variant; Main app uses 'search' widget. -->
{:else if widget.widget_type === 'helpful_widget'}
	<!-- Helpful widget — rendered by the Docs app; no-op in Main app. -->
{:else if widget.widget_type === 'curriculum_rail'}
	<!-- Curriculum rail — rendered by the Learn app on lesson pages; no-op in Main. -->
{:else}
	<!-- Unknown widget type — render nothing rather than crashing. -->
{/if}
