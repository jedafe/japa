import { redirect } from '@sveltejs/kit';
import type { PageLoad } from './$types';

/** Legacy redirect: /organisations/claims → /organizations/claims */
export const load: PageLoad = (event) => {
	throw redirect(301, `/organizations/claims${event.url.search}`);
};
