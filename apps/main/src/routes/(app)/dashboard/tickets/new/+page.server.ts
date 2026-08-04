// Ref: japaconnect-engineering/sections/38-tickets.md (user portal)
// Loads the public ticket types so the user can pick one when filing a new
// ticket. Auth-gated; the backend filters to is_public=true types for users.
import { redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiList } from '$lib/server/api';
import type { TicketType } from '$lib/types';

export const load: PageServerLoad = async (event) => {
	if (!event.locals.user) {
		throw redirect(307, `/login?redirect=${encodeURIComponent(event.url.pathname + event.url.search)}`);
	}

	const ctx = apiContext(event);
	const types = (await apiList<TicketType>(ctx, '/ticket-types?is_public=true')).items;
	return { types };
};
