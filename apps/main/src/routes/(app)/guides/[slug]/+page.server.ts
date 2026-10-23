// Ref: japaconnect-engineering/sections/14-content-type-system.md
//      ADR-058 (route families per content type)
// Legacy alias: /guides/[slug] → /guide/[slug] (the canonical guide detail
// route). Mirrors /stories/[slug] → /story/[slug]; without it, a natural
// plural guess 404s even though /guide → /guides aliases the index.
import { redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';

export const load: PageServerLoad = async (event) => {
	throw redirect(301, `/guide/${event.params.slug}${event.url.search}`);
};
