import type { PageServerLoad } from './$types';
import { redirect } from '@sveltejs/kit';
import { DOCS_ORIGIN } from '$lib/appOrigins';

/**
 * ADR-060 / P9 (one backend, multiple client surfaces): the Documentation site
 * is a dedicated SvelteKit app at docs.japa.to (apps/docs, port 3004 in
 * dev). The main app has no doc tree of its own — it deep-links into Docs from
 * the profile dashboard Knowledge Base tab and bookmarks.
 *
 * A bare `/docs` (or `/docs/...`) on the main app is therefore a cross-app deep
 * link, not a 404. Redirect to the Docs app, preserving the remainder of the
 * path and the query string so bookmarks like `/docs/api/playground` land on
 * the right page. Ref: audit finding — `/docs` returned 404.
 */
export const load: PageServerLoad = async ({ url, params }) => {
	const rest = params.rest ?? '';
	const path = rest ? `/${rest}` : '';
	const search = url.search; // preserves ?q=... etc.
	throw redirect(308, `${DOCS_ORIGIN}${path}${search}`);
};

export const prerender = false;
