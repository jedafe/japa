import { redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiList } from '$lib/server/api';

/** DM message request item from GET /dm/requests. */
export interface DMRequest {
	id: string;
	conversation_id?: string;
	requester_id: string;
	requester_username: string;
	requester_display_name: string;
	requester_avatar_url?: string;
	preview_message: string;
	status: string;
	created_at: string;
}

/**
 * Message requests at /messages/requests — auth-gated DM requests surface
 * (feature.dm). Lists pending requests with accept/decline/block actions.
 *
 * Ref: japaconnect-engineering/sections/48-direct-messages.md
 */
const PAGE_SIZE = 30;

export const load: PageServerLoad = async (event) => {
	if (!event.locals.user) {
		throw redirect(307, '/login?redirect=/messages/requests');
	}

	const ctx = apiContext(event);

	let requests: DMRequest[] = [];

	try {
		requests = (await apiList<DMRequest>(ctx, `/dm/requests?page_size=${PAGE_SIZE}`)).items;
	} catch {
		requests = [];
	}

	return { requests, pageSize: PAGE_SIZE };
};
