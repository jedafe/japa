import { redirect } from '@sveltejs/kit';
import type { PageLoad } from './$types';

/** Legacy redirect: /organisations → /organizations (British → American path,
 *  matching the /admin/organizations API resource). Query string forwarded so
 *  shared/bookmarked list states keep working. */
export const load: PageLoad = (event) => {
	throw redirect(301, `/organizations${event.url.search}`);
};
