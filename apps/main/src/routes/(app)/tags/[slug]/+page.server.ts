import { error } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiGet, apiList } from '$lib/server/api';
import type { TagDetail } from '$lib/types';
import type { SearchResponse } from '$lib/types/search';
import type { ContentArticle } from '$lib/types/content';

/**
 * Tag hub at /tags/[slug] — SSR, indexable. Center column shows the tag
 * header (name, description, follower count, follow button) and a grid of
 * tagged content. The right sidebar shows the tag card with follow action.
 *
 * Ref: japaconnect-engineering/sections/08-tags.md
 *      ADR-058 (renders inside the app shell; not a documented tab surface)
 */
const PAGE_SIZE = 12;

interface TagFieldDef {
	key: string;
	label: string;
	field_type: string;
	placeholder?: string;
	default_value?: string;
	is_required: boolean;
	show_in_frontend?: boolean;
}

interface TagFollower {
	id: string;
	username: string;
	display_name: string;
	avatar_url?: string | null;
	bio?: string | null;
	is_following?: boolean;
}

export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);
	const slug = event.params.slug;
	const rawTab = event.url.searchParams.get('tab') ?? 'content';
	// Only known tabs — an arbitrary ?tab= value would leave UrlTabs with a
	// value matching no trigger (no active state) while rendering content.
	const tab = rawTab === 'search' || rawTab === 'followers' ? rawTab : 'content';
	const query = event.url.searchParams.get('q') ?? '';

	// Tag detail — 404 propagates as a SvelteKit error.
	let tag: TagDetail;
	try {
		tag = await apiGet<TagDetail>(ctx, `/tags/${slug}`);
	} catch (e) {
		const status = e instanceof Error && 'status' in e ? (e as { status: number }).status : 500;
		throw error(status === 404 ? 404 : 500, {
			code: status === 404 ? 'not_found' : 'server_error',
			message: status === 404 ? 'Tag not found' : 'Failed to load tag'
		});
	}

	// Tab-specific data. Content is the default; search and followers are
	// loaded on demand. Keep failures per-tab so one failed tab doesn't blank
	// the tag header.
	let items: ContentArticle[] = [];
	let loadError = false;
	let nextCursor = '';
	let nextCursorId = '';
	let searchResults: SearchResponse['results'] = [];
	let searchAuthRequired = false;
	let followers: TagFollower[] = [];
	let followersAuthRequired = false;

	if (tab === 'search') {
		if (!event.locals.user) {
			searchAuthRequired = true;
		} else if (query) {
			try {
				const res = await apiGet<SearchResponse>(
					ctx,
					`/tags/${slug}/search?q=${encodeURIComponent(query)}&limit=20`
				);
				searchResults = res.results ?? [];
			} catch {
				searchResults = [];
			}
		}
	} else if (tab === 'followers') {
		// GET /tags/{slug}/followers is auth-gated. For guests, skip the call
		// and surface a sign-in prompt instead of a misleading "No followers
		// yet" empty state.
		if (!event.locals.user) {
			followersAuthRequired = true;
		} else {
			try {
				followers = (await apiList<TagFollower>(ctx, `/tags/${slug}/followers?page_size=50`)).items;
			} catch {
				followers = [];
			}
		}
	} else {
		// Tagged content — degrades to an empty list on failure so a missing feed
		// never blanks the whole tag view. Supports composite cursor-based
		// pagination (Section 14): the initial SSR load fetches the first page;
		// subsequent "Load More" clicks are handled client-side via ?cursor=
		// and ?cursor_id=. The composite (created_at, id) cursor avoids skipping
		// items that share the same created_at timestamp.
		try {
			const cursorParam = event.url.searchParams.get('cursor') ?? '';
			const cursorIdParam = event.url.searchParams.get('cursor_id') ?? '';
			const params = new URLSearchParams({ page_size: String(PAGE_SIZE) });
			if (cursorParam) params.set('cursor', cursorParam);
			if (cursorIdParam) params.set('cursor_id', cursorIdParam);
			items = (await apiList<ContentArticle>(ctx, `/tags/${slug}/content?${params.toString()}`)).items;
			// Compute the next cursor from the last item's (created_at, id) so
			// the client can fetch the following page without a server round-trip
			// for metadata.
			if (items.length >= PAGE_SIZE && items.length > 0) {
				const last = items[items.length - 1];
				if (last?.created_at) nextCursor = last.created_at;
				if (last?.id) nextCursorId = last.id;
			}
		} catch {
			loadError = true;
		}
	}

	// Field definitions come embedded in the tag payload — GET /tags/{slug}
	// emits the "tag" entity-class defs as field_defs so guests can render
	// labeled field_values too (/custom-field-groups is auth-gated). The
	// backend already dedupes by key across groups.
	const fieldDefs: TagFieldDef[] = (tag.field_defs as TagFieldDef[] | undefined) ?? [];

	// Hydrate the follow button's initial state. Best-effort: a failure
	// degrades to `false` (not following). Only fetch when the viewer is
	// authenticated — guests can't follow.
	const initiallyFollowing = event.locals.user
		? apiGet<{ following: boolean }>(ctx, `/users/${tag.id}/follow-status?entity_type=tag`)
				.then((r) => r.following)
				.catch(() => false)
		: Promise.resolve(false);

	// Check if the viewer can manage tags (tags.manage permission or super_admin
	// group membership — super_admin bypasses all permission checks at the RBAC
	// middleware level, but /me/permissions only returns explicitly resolved
	// nodes, so the group check is needed to surface the edit button for them).
	// Best-effort: degrades to false so a failing lookup never blocks the page.
	let canManage = false;
	if (event.locals.user) {
		const isSuperAdmin = (event.locals.user.groups ?? []).some(
			(g) => g === 'Super Admin' || g === 'super_admin'
		);
		if (isSuperAdmin) {
			canManage = true;
		} else {
			try {
				const permRes = await apiGet<{ permissions: string[] }>(ctx, '/me/permissions');
				canManage = (permRes?.permissions ?? []).includes('tags.manage');
			} catch {
				canManage = false;
			}
		}
	}

	const isFollowing = await initiallyFollowing;

	return {
		tag,
		tab,
		query,
		items,
		pageSize: PAGE_SIZE,
		loadError,
		fieldDefs,
		canManage,
		initiallyFollowing: isFollowing,
		nextCursor,
		nextCursorId,
		searchResults,
		searchAuthRequired,
		followers,
		followersAuthRequired
	};
};
