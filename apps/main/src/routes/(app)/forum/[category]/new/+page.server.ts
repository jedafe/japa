import { redirect } from '@sveltejs/kit';

import type { PageServerLoad } from './$types';

// Legacy composer URL → /thread/{category}/new.
export const load: PageServerLoad = ({ params, url }) => {
	throw redirect(301, `/thread/${params.category}/new${url.search}`);
};
