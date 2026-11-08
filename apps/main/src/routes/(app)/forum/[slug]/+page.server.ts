import { redirect } from '@sveltejs/kit';

import type { PageServerLoad } from './$types';

// Legacy forum thread URL → /thread/{slug}.
export const load: PageServerLoad = ({ params, url }) => {
	throw redirect(301, `/thread/${params.slug}${url.search}`);
};
