import type { PageServerLoad } from './$types';
import { redirect } from '@sveltejs/kit';
import { ACADEMY_ORIGIN } from '$lib/appOrigins';

/**
 * ADR-060 / P9 (one backend, multiple client surfaces): the Academy Platform
 * is a dedicated SvelteKit app (apps/academy, port 3003 in dev). The main app
 * has no course catalog of its own — it links into the Academy app from the
 * profile dashboard (AcademyView).
 *
 * A bare `/academy/...` on the main app is therefore a cross-app deep link,
 * not a 404. Redirect to the Academy app, preserving the remainder of the
 * path and the query string so bookmarks like `/academy/tracks/japa-101` land
 * on the right track.
 */
export const load: PageServerLoad = async ({ url, params }) => {
	const rest = params.rest ?? '';
	const path = rest ? `/${rest}` : '';
	const search = url.search; // preserves ?q=... etc.
	throw redirect(302, `${ACADEMY_ORIGIN}${path}${search}`);
};

export const prerender = false;
