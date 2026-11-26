// Ref: japaconnect-engineering/sections/38-tickets.md (user portal)
// Single-ticket threads live in the Messages support workbench
// (?t=<id|JC-n>) — this route only forwards.
import { redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';

export const load: PageServerLoad = async (event) => {
	throw redirect(307, `/messages/support?t=${encodeURIComponent(event.params.id)}`);
};
