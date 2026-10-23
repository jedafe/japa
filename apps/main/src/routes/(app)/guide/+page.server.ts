// Ref: japaconnect-engineering/sections/14-content-type-system.md
// Convenience alias: /guide → /guides (the canonical guides index —
// mirrors /story → /stories for the singular/plural route families).
import { redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';

export const load: PageServerLoad = async (event) => {
	throw redirect(301, `/guides${event.url.search}`);
};
