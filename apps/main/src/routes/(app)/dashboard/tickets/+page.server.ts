// Ref: japaconnect-engineering/sections/38-tickets.md (user portal)
// Lists the current user's submitted tickets. Auth-gated; the backend returns
// only tickets where submitter_user_id = the caller.
import { redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiList } from '$lib/server/api';
import type { TicketListItem } from '$lib/types';

export const load: PageServerLoad = async (event) => {
	if (!event.locals.user) {
		throw redirect(307, `/login?redirect=${encodeURIComponent(event.url.pathname + event.url.search)}`);
	}

	const ctx = apiContext(event);
	const tickets = (await apiList<TicketListItem>(ctx, '/me/tickets')).items;
	return { tickets };
};
