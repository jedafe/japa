// Ref: japaconnect-engineering/sections/38-tickets.md (user portal)
// Lists the current user's submitted tickets. Auth-gated; the backend returns
// only tickets where submitter_user_id = the caller.
import { error } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiList } from '$lib/server/api';
import type { TicketListItem } from '$lib/types';

export const load: PageServerLoad = async (event) => {
	if (!event.locals.user) {
		throw error(403, { code: 'forbidden', message: 'Authentication required to view tickets' });
	}

	const ctx = apiContext(event);
	const tickets = (await apiList<TicketListItem>(ctx, '/me/tickets')).items;
	return { tickets };
};
