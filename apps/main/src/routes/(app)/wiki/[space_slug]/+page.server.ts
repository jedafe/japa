// Ref: japaconnect-engineering/sections/80-destination-wiki.md
import type { PageServerLoad } from './$types';
import { error } from '@sveltejs/kit';
import { apiContext, apiList, apiGet, ApiError } from '$lib/server/api';
import type { WikiPageIndex, SpaceDetail } from '$lib/types';

/**
 * Wiki index for a space at /wiki/{space_slug} — lists all wiki pages for the
 * space, grouped by category on the client. The backend returns the page
 * summaries (no content) via GET /wiki/{space_slug}.
 */
export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);
	const spaceSlug = event.params.space_slug;
	const authed = !!event.locals.user;
	const userPermissions = event.locals.user?.permissions ?? [];

	try {
		const { items } = await apiList<WikiPageIndex>(ctx, `/wiki/${encodeURIComponent(spaceSlug)}`);
		const spaceDetail = await apiGet<SpaceDetail>(
			ctx,
			`/spaces/${encodeURIComponent(spaceSlug)}`
		).catch(() => null);
		return { spaceSlug, index: items, authed, userPermissions, loadError: false, spaceDetail };
	} catch (e) {
		if (e instanceof ApiError && e.status === 404) {
			error(404, { code: 'wiki_not_found', message: 'Wiki not found for this destination' });
		}
		return { spaceSlug, index: [] as WikiPageIndex[], authed, userPermissions, loadError: true, spaceDetail: null };
	}
};
