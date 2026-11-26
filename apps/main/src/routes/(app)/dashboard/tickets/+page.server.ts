// Ref: japaconnect-engineering/sections/38-tickets.md (user portal)
// The member tickets surface lives in the Messages support workbench —
// this route only forwards (query params map 1:1: status, page).
import { redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';

export const load: PageServerLoad = async (event) => {
	throw redirect(307, `/messages/support${event.url.search}`);
};
