// Ref: japaconnect-engineering/sections/80-destination-wiki.md
import type { PageServerLoad } from './$types';
import { error } from '@sveltejs/kit';
import { apiContext, apiGet, ApiError } from '$lib/server/api';
import type { WikiPage, SpaceDetail } from '$lib/types';

/**
 * Wiki page view at /wiki/{space_slug}/{page_slug} — fetches the current
 * approved revision's content via GET /wiki/{space_slug}/{page_slug}.
 */
export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);
	const spaceSlug = event.params.space_slug;
	const pageSlug = event.params.page_slug;
	const authed = !!event.locals.user;
	const userPermissions = event.locals.user?.permissions ?? [];

	try {
		const page = await apiGet<WikiPage>(
			ctx,
			`/wiki/${encodeURIComponent(spaceSlug)}/${encodeURIComponent(pageSlug)}`
		);
		const spaceDetail = await apiGet<SpaceDetail>(
			ctx,
			`/spaces/${encodeURIComponent(spaceSlug)}`
		).catch(() => null);
		return { spaceSlug, pageSlug, page, authed, userPermissions, loadError: false, spaceDetail };
	} catch (e) {
		if (e instanceof ApiError && e.status === 404) {
			error(404, { code: 'wiki_page_not_found', message: 'Wiki page not found' });
		}
		return { spaceSlug, pageSlug, page: null, authed, userPermissions, loadError: true, spaceDetail: null };
	}
};
