import { error, redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiGet, apiGetEnvelope, apiList, apiRadarList } from '#lib/server/api.js';
import type { RadarItem } from '#lib/types/index.js';
import type {
	SpaceDetail,
	Space,
	SpaceMember,
	SpaceModerator,
	SpaceContentItem,
	SpacePermissionPreset,
	DigestEdition,
	WikiPageIndex,
	Achievement,
	SpaceReview,
	CalendarEntry,
	SpaceLeader,
	SpaceContributor,
	SpaceTypeOption,
	MySpaceInvite
} from '#lib/types/index.js';

/**
 * Space hub at /spaces/[slug] — SSR, indexable. Center column swaps between
 * Feed / Forum / Members / About tabs; the right sidebar shows the space
 * card (member/follower counts, join + follow actions, rules).
 *
 * Ref: japaconnect-engineering/sections/05-geo-first-space-system.md
 *      conventions/system-settings-catalog.md (spaces.* settings)
 *      ADR-058 (Space: Feed/Forum/Members/About tabs inside the shell)
 */
const DEFAULT_HUB_PAGE_SIZE = 20;
const DEFAULT_FORUM_PAGE_SIZE = 50;

/** §80 canonical category set scoped to the space type — the same shape
 *  /wiki/{space_slug} uses to order its section cards; the About tab
 *  groups the space's wiki pages under these categories in this order. */
type WikiApiCategory = {
	slug: string;
	label_key: string;
	label: string;
	/** Stub page slug for /wiki/{space}/{page_slug} links. */
	page_slug?: string;
	/** §80 directories — the stub page is the child-space listing. */
	directory?: boolean;
	/** §80 directories — true when the space has no matching descendants. */
	empty?: boolean;
};

export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);
	const slug = event.params.slug;

	// Independent fetches kick off immediately — settings are a shared public
	// catalog, mute status only needs the slug. None depend on the space
	// detail fetch below, so they shouldn't wait for it.
	const siteSettingsPromise = apiGet<Record<string, unknown>>(ctx, '/site/settings/public')
		.then((s): Record<string, unknown> => s ?? {})
		.catch((): Record<string, unknown> => ({}));
	const muteStatusPromise = event.locals.user
		? apiGet<{ muted: boolean }>(ctx, `/spaces/${slug}/mute-status`)
				.then((r) => r.muted)
				.catch(() => false)
		: Promise.resolve(false);
	const parentPromise = event.parent();

	// Rule 3 / Rule 16: configurable page sizes from system_settings.
	const siteSettings = await siteSettingsPromise;
	const pageSize =
		typeof siteSettings['spaces.hub_page_size'] === 'number'
			? Math.max(1, Math.round(siteSettings['spaces.hub_page_size'] as number))
			: DEFAULT_HUB_PAGE_SIZE;
	const forumPageSize =
		typeof siteSettings['spaces.forum_page_size'] === 'number'
			? Math.max(1, Math.round(siteSettings['spaces.forum_page_size'] as number))
			: DEFAULT_FORUM_PAGE_SIZE;

	// Feature flags from the root layout cache; used to decide whether the
	// digest widget should make an SSR fetch (§84, ADR-092).
	const parent = await parentPromise;
	const features = (parent?.features as Record<string, boolean> | undefined) ?? {};

	// Space detail bundle: { space, rules, keywords, logo_url, header_url }.
	// 404 propagates as a SvelteKit error.
	let detail: SpaceDetail;
	try {
		detail = await apiGet<SpaceDetail>(ctx, `/spaces/${slug}`);
	} catch (e) {
		const status = e instanceof Error && 'status' in e ? (e as { status: number }).status : 500;
		throw error(status === 404 ? 404 : 500, {
			code: status === 404 ? 'not_found' : 'server_error',
			message: status === 404 ? 'Space not found' : 'Failed to load space'
		});
	}

	const space = detail.space;
	const spaceId = space.id;

	// §80 attractions: wiki-only spaces have no community surface — redirect
	// to the wiki index before the tab fan-out below runs. The from_space
	// flag tells the wiki index to show the wiki-only explainer.
	if (detail.wiki_only) {
		throw redirect(302, `/wiki/${encodeURIComponent(slug)}?from_space=1`);
	}

	// §05 amendment — universities are education-typed spaces marked by
	// level='university' (the 'university' space type was retired), so the
	// level is the discriminator for the Reviews tab.
	const isUniversity = space.level === 'university';

	// §05 hub-redesign (2026-11-27) — tab gating by space class:
	// is_primary marks platform-run spaces (geo + universities); geo
	// covers country/state/city; custom = member-created community.
	const isGeo = ['country', 'state', 'city'].includes(space.level);
	const isPrimary = space.is_primary === true || isGeo || isUniversity;

	// Feature flags + result shapes needed by the batch below — resolved
	// once so each promise can gate on them without extra awaits.
	const wikiEnabled = !!features['wiki.enabled'];
	// Phase 08.10 — analytics fetch result: data on success, an error
	// sentinel on 403/unavailable (space-admin only per JEOS §215).
	type AnalyticsResult = { data: unknown } | { error: string };
	type WikiTypeRow = { space_type_id: string; enabled: boolean };

	// Viewer-scoped management flags from the detail bundle (Ref: §05 lines
	// 198-224). is_owner gates the moderator assignment UI; is_moderator is
	// surfaced for future moderator-only controls. Both default to false for
	// guests — the mutating routes re-check server-side.
	const isOwner = detail.is_owner ?? false;
	const isModerator = detail.is_moderator ?? false;

	// Hydrate the follow button's initial state from the detail bundle's
	// viewer-scoped is_following flag (added to GET /spaces/{slug} — one
	// batch query covers member/pending/follow for the space).
	const initiallyFollowing = Promise.resolve(detail.is_following ?? false);

	// Mute status — only fetched for authed viewers (guests always get false).
	const muteStatus = muteStatusPromise;

	// Tab data — each degrades to an empty list on failure so a missing tab
	// never blanks the whole space view.
	const canManage = isOwner || isModerator;

	const [
		contentRes, pinned, members, discussionFeed, moderators, isFollowing,
		isMuted, presets, parentSpaces, achievements, digest,
		blogFeed, storyFeed, guideFeed, events, subspaces, leaders, contributors, spaceTypes,
		reviews, analyticsResult, wikiTypes, opportunities, myInvite
	] = await Promise.all([
		// items + meta.total (§05 hub-redesign — the header stats row shows
		// real counts without paginating the full list).
		apiList<SpaceContentItem>(ctx, `/spaces/${slug}/content?page_size=${pageSize}`)
			.then((r) => ({ items: r.items, total: r.meta?.total ?? null }))
			.catch(() => ({ items: [] as SpaceContentItem[], total: null })),
		apiList<SpaceContentItem>(ctx, `/spaces/${slug}/pinned`)
			.then((r) => r.items)
			.catch(() => []),
		apiList<SpaceMember>(ctx, `/spaces/${slug}/members?page_size=${pageSize}`)
			.then((r) => r.items)
			.catch(() => []),
		// §05 hub-redesign — the Discussions tab is a contents feed over the
		// thread types: forum_thread (POST /thread backing rows) + discussion
		// (canonical anchor threads for tools/wiki). /thread?space_id= alone
		// would miss every anchored discussion row.
		apiList<SpaceContentItem>(ctx, `/spaces/${slug}/content?type=forum_thread,discussion&page_size=${forumPageSize}`)
			.then((r) => ({ items: r.items, total: r.meta?.total ?? null }))
			.catch(() => ({ items: [] as SpaceContentItem[], total: null })),
		// Moderators list — only fetched for the owner/moderator so the
		// assignment panel has data to render. The sidebar's leaders card
		// uses the public /leaders route instead.
		canManage
			? apiList<SpaceModerator>(ctx, `/spaces/${slug}/moderators`)
					.then((r) => r.items)
					.catch(() => [])
			: Promise.resolve([] as SpaceModerator[]),
		initiallyFollowing,
		muteStatus,
		// Permission presets — only fetched for authed viewers (the route
		// requires auth). Used in the Settings tab to show preset shortcuts.
		event.locals.user
			? apiList<SpacePermissionPreset>(ctx, `/space-permission-presets`)
					.then((r) => r.items)
					.catch(() => [])
			: Promise.resolve([] as SpacePermissionPreset[]),
		// Parent spaces — OptionalAuth since the hub-redesign amendment:
		// the About card's location line resolves ancestors for guests too.
		apiList<Space>(ctx, `/spaces/${slug}/parents`)
			.then((r) => r.items)
			.catch(() => []),
		// Sidebar achievement widget — fetch server-side so SSR and client
		// render the same DOM and avoid a hydration mismatch (HYDRATION-001).
		apiList<Achievement>(ctx, `/spaces/${slug}/achievements?limit=5`)
			.then((r) => r.items)
			.catch(() => []),
		// Latest digest for this destination (§84, flag-gated). Both
		// inputs — spaceId and the flag — are already resolved, so the
		// fetch rides the same batch instead of a sequential round-trip
		// (audit BUG-019). A 404/no_digest is expected when none has been
		// published yet.
		features['digest.enabled']
			? apiGet<DigestEdition>(ctx, `/digest/latest?dest=${encodeURIComponent(spaceId)}`).catch(
					() => null
				)
			: Promise.resolve(null),
		// §05 hub-redesign — type-filtered feeds for the Blog/Stories/Guides
		// tabs. Each rides the same ?type= listing and keeps meta.total for
		// the tab count + header stats. Primary-only except Stories (geo).
		isPrimary
			? apiList<SpaceContentItem>(ctx, `/spaces/${slug}/content?type=blog_post&page_size=${pageSize}`)
					.then((r) => ({ items: r.items, total: r.meta?.total ?? null }))
					.catch(() => ({ items: [] as SpaceContentItem[], total: null }))
			: Promise.resolve({ items: [] as SpaceContentItem[], total: null }),
		isGeo
			? apiList<SpaceContentItem>(ctx, `/spaces/${slug}/content?type=story&page_size=${pageSize}`)
					.then((r) => ({ items: r.items, total: r.meta?.total ?? null }))
					.catch(() => ({ items: [] as SpaceContentItem[], total: null }))
			: Promise.resolve({ items: [] as SpaceContentItem[], total: null }),
		isPrimary
			? apiList<SpaceContentItem>(ctx, `/spaces/${slug}/content?type=guide&page_size=${pageSize}`)
					.then((r) => ({ items: r.items, total: r.meta?.total ?? null }))
					.catch(() => ({ items: [] as SpaceContentItem[], total: null }))
			: Promise.resolve({ items: [] as SpaceContentItem[], total: null }),
		// Events tab + sidebar "Upcoming events" (§86) — geo + university
		// destinations only; customs have no calendar surface.
		features['calendar.enabled'] && (isGeo || isUniversity)
			? apiList<CalendarEntry>(
					ctx,
					`/calendar?dest=${encodeURIComponent(spaceId)}&view=list&from=${encodeURIComponent(new Date().toISOString())}&page_size=5`
				)
					.then((r) => ({ items: r.items, total: r.meta?.total ?? null }))
					.catch(() => ({ items: [] as CalendarEntry[], total: null }))
			: Promise.resolve({ items: [] as CalendarEntry[], total: null }),
		// Subspaces tab — member-created communities under this geo parent
		// (custom spaces can't be parents, SPACES-008; universities don't
		// get the tab either).
		isGeo
			? apiList<Space>(ctx, `/spaces?within=${encodeURIComponent(slug)}&member_created=1&page_size=${pageSize}`)
					.then((r) => r.items)
					.catch(() => [])
			: Promise.resolve([] as Space[]),
		// Sidebar "Space leaders" card — owners ∪ moderators, OptionalAuth.
		apiList<SpaceLeader>(ctx, `/spaces/${slug}/leaders?page_size=10`)
			.then((r) => r.items)
			.catch(() => []),
		// Wiki-tab "Top contributors" card.
		apiList<SpaceContributor>(ctx, `/spaces/${slug}/contributors?page_size=5`)
			.then((r) => r.items)
			.catch(() => []),
		// Space types catalog — resolves the About card's Category line
		// (space_type_id → name). Public route.
		apiList<SpaceTypeOption>(ctx, `/space-types`)
			.then((r) => r.items)
			.catch(() => []),
		// Reviews tab — university spaces only; the route is public and
		// returns an empty list otherwise, but the fetch is gated to keep
		// non-university spaces from paying for it.
		isUniversity
			? apiGet<SpaceReview[]>(ctx, `/spaces/${spaceId}/reviews?page_size=20`).catch(() => [])
			: Promise.resolve([] as SpaceReview[]),
		// Analytics dashboard — space-admin only per JEOS §215 [auth,
		// space-admin]. Only fetched for authenticated viewers; the route
		// 403s for non-admins (degrades to the error sentinel → tab hidden).
		event.locals.user
			? apiGet<unknown>(ctx, `/spaces/${spaceId}/analytics`)
					.then((data): AnalyticsResult => ({ data }))
					.catch((err: any): AnalyticsResult => {
						if (err?.status === 403 || err?.code === 'forbidden') {
							return { error: 'permission_denied' };
						}
						return { error: 'unavailable' };
					})
			: Promise.resolve({ data: null } as AnalyticsResult),
		// §80 Wiki Types probe — resolves hasWiki: a space has a wiki when
		// its space_type has an enabled wiki_type_configs row. Primary-only
		// surface (§05 hub matrix), so custom spaces skip the round-trip
		// entirely; the index fetch below is the only dependent wait.
		wikiEnabled && isPrimary && space.space_type_id
			? apiGetEnvelope<WikiTypeRow[]>(ctx, '/wiki/types').catch(
					() => ({ data: [] as WikiTypeRow[] })
				)
			: Promise.resolve({ data: [] as WikiTypeRow[] }),
		// Opportunities tab — geo-filtered intel radar feed. Only fetched
		// when the URL deep-links ?tab=opportunities; UrlTabs switches
		// shallowly otherwise, so the tab lazy-fetches client-side on
		// activation. Public for guests; degrades to an empty list.
		event.url.searchParams.get('tab') === 'opportunities'
			? apiRadarList<RadarItem>(ctx, `/intel/radar?space_id=${space.id}&page_size=24`)
					.then((r) => r?.items ?? [])
					.catch(() => [] as RadarItem[])
			: Promise.resolve([] as RadarItem[]),
		// The viewer's pending invite to THIS space — feeds the accept/
		// decline banner that space_invite notifications link to. Custom
		// spaces only (primary spaces have no invites) and only for authed
		// non-members; a member's resolved invite can't be pending.
		event.locals.user && !(detail.is_member ?? false) && !isPrimary
			? apiList<MySpaceInvite>(ctx, `/me/space-invites?page_size=50`)
					.then((r) => r.items.find((i) => i.space_id === space.id) ?? null)
					.catch(() => null)
			: Promise.resolve(null)
	]);

	// Destination wiki index for this space, gated by the wiki feature flag.
	// §80 Wiki Types amendment: a space has a wiki when its space_type has an
	// enabled wiki_type_configs row — resolved from /wiki/types so newly
	// enabled wiki types appear automatically instead of a hardcoded level
	// list (Rule 16). A 404 from the wiki index simply means the space has
	// no wiki yet.
	let wikiPages: WikiPageIndex[] | null = null;
	let wikiCategories: WikiApiCategory[] = [];
	const hasWiki =
		wikiEnabled && isPrimary && !!space.space_type_id
			? (wikiTypes.data ?? []).some(
					(wt) => wt.enabled && wt.space_type_id === space.space_type_id
				)
			: false;
	// Wiki is a primary-space surface (§05 hub matrix) — skip the index
	// fetch for custom spaces even when their type has a wiki config.
	if (hasWiki) {
		[wikiPages, wikiCategories] = await Promise.all([
			apiList<WikiPageIndex>(ctx, `/wiki/${slug}`)
				.then((r) => r.items)
				.catch(() => null),
			// Scoped canonical categories — the About tab orders its grouped
			// wiki list by this set, matching /wiki/{space_slug}.
			apiGetEnvelope<WikiApiCategory[]>(
				ctx,
				`/wiki/categories?space=${encodeURIComponent(slug)}`
			)
				.then((r) => r.data ?? [])
				.catch(() => [] as WikiApiCategory[])
		]);
	}

	return {
		space,
		opportunities,
		seoIndexable: detail.seo_indexable ?? true,
		rules: detail.rules,
		keywords: detail.keywords ?? [],
		logoUrl: detail.logo_url ?? '',
		headerUrl: detail.header_url ?? '',
		isUniversity,
		isGeo,
		isPrimary,
		reviews,
		isOwner,
		isModerator,
		isMember: detail.is_member ?? false,
		isPending: detail.is_pending ?? false,
		moderators,
		presets,
		parentSpaces,
		content: contentRes.items,
		contentTotal: contentRes.total,
		pinned,
		members,
		discussionFeed,
		analytics: 'data' in analyticsResult ? analyticsResult.data : null,
		analyticsError: 'error' in analyticsResult ? analyticsResult.error : null,
		isMuted,
		pageSize,
		forumPageSize,
		initiallyFollowing: isFollowing,
		achievements,
		digest,
		wikiEnabled,
		hasWiki,
		wikiPages,
		wikiCategories,
		// §05 hub-redesign — typed tab feeds + sidebar card data.
		blogFeed,
		storyFeed,
		guideFeed,
		events,
		subspaces,
		leaders,
		contributors,
		spaceTypes,
		myInvite,
		calendarEnabled: !!features['calendar.enabled']
	};
};
