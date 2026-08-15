import { error } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';

/**
 * Travel Search at /tools/travel (Section 67, ADR-085).
 * The backend gate (travel.enabled) is enforced inside the handler; the
 * page renders the search form SSR and fetches results client-side.
 *
 * Ref: japaconnect-engineering/sections/67-travel-search.md
 */
export const load: PageServerLoad = async (event) => {
	const { features } = await event.parent();
	if (!features['travel.enabled']) {
		throw error(404, { code: 'feature_disabled', message: 'Travel Search is not available' });
	}

	return { authed: !!event.locals.user };
};
