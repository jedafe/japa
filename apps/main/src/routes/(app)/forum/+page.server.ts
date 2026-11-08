import { redirect } from '@sveltejs/kit';

import type { PageServerLoad } from './$types';

// Legacy forum URL — the thread list moved to /thread (thread-rename spec).
// 301 keeps bookmarks and external links working while transferring SEO.
export const load: PageServerLoad = ({ url }) => {
	throw redirect(301, `/thread${url.search}`);
};
