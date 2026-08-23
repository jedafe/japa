import type { PageServerLoad } from './$types';
import { redirect } from '@sveltejs/kit';
import { LEARN_ORIGIN } from '$lib/appOrigins';

/**
 * ADR-060 / P9 (one backend, multiple client surfaces): the Learning Platform
 * is a dedicated SvelteKit app (apps/learn, port 3003 in dev). The main app
 * has no learning catalog of its own — it links into the Learn app from the
 * profile dashboard (LearningView) and university pages.
 *
 * A bare `/learn` (or `/learn/...`) on the main app is therefore a cross-app
 * deep link, not a 404. Redirect to the Learn app, preserving the remainder of
 * the path and the query string so bookmarks like `/learn/tracks/japa-101` land
 * on the right track. Ref: audit finding — `/learn` returned 404.
 */
export const load: PageServerLoad = async ({ url, params }) => {
	const rest = params.rest ?? '';
	const path = rest ? `/${rest}` : '';
	const search = url.search; // preserves ?q=... etc.
	throw redirect(302, `${LEARN_ORIGIN}${path}${search}`);
};

export const prerender = false;
