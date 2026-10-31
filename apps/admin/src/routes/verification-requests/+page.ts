import { redirect } from '@sveltejs/kit';
import type { PageLoad } from './$types';

/** The queue defaults to pending requests (the reviewable set). Any other
 *  filter combination is expressed in the URL by the EntityPage scaffold, so
 *  a bare visit is canonicalized to ?status=pending. */
export const load: PageLoad = (event) => {
	if (!event.url.searchParams.has('status')) {
		const params = new URLSearchParams(event.url.searchParams);
		params.set('status', 'pending');
		throw redirect(301, `/verification-requests?${params.toString()}`);
	}
};
