// Ref: japaconnect-engineering/sections/79-community-story-wall.md
// Legacy alias: /stories/[slug] → /story/[slug] (the canonical
// story detail route — ADR-058 route family).
import { redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';

export const load: PageServerLoad = async (event) => {
	throw redirect(301, `/story/${event.params.slug}${event.url.search}`);
};
