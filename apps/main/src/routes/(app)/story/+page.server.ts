// Ref: japaconnect-engineering/sections/79-community-story-wall.md
// Convenience alias: /story → /stories (the canonical Story Wall route).
import { redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';

export const load: PageServerLoad = async (event) => {
	throw redirect(301, `/stories${event.url.search}`);
};
