// Legacy alias: /stories/[slug]/edit → /story/[slug]/edit (canonical
// edit route for the story single — ADR-058 route family).
import { redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';

export const load: PageServerLoad = async (event) => {
	throw redirect(301, `/story/${event.params.slug}/edit${event.url.search}`);
};
