// The member content manager moved to Studio → Contents (/studio/contents).
// Keep the old dashboard URL working — the dashboard_rail menu and any
// bookmarks still link here. Static route wins over the [view] slug.
import { redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';

export const load: PageServerLoad = async (event) => {
	throw redirect(308, `/studio/contents${event.url.search}`);
};
