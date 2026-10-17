import { redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';

/**
 * The docs app root redirects to /docs (the docs landing). The canonical
 * surface is docs.japa.to/docs; the bare origin is a convenience.
 */
export const load: PageServerLoad = async () => {
	throw redirect(308, '/docs');
};
