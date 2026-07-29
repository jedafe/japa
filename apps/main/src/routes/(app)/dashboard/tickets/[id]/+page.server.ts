// Ref: japaconnect-engineering/sections/38-tickets.md (user portal)
// Loads a single ticket with its comments and attachments. Auth-gated; the
// backend self-403s if the caller is not the submitter.
import { error } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiGet } from '$lib/server/api';
import type { TicketDetailResponse } from '$lib/types';

export const load: PageServerLoad = async (event) => {
	if (!event.locals.user) {
		throw error(403, { code: 'forbidden', message: 'Authentication required to view tickets' });
	}

	const ctx = apiContext(event);
	try {
		const detail = await apiGet<TicketDetailResponse>(ctx, `/me/tickets/${event.params.id}`);
		return { detail };
	} catch (err) {
		const status = err instanceof Error && 'status' in err ? (err as { status: number }).status : 500;
		const code = err instanceof Error && 'code' in err ? (err as { code: string }).code : 'api_error';
		throw error(status, { code, message: 'Ticket not found' });
	}
};
