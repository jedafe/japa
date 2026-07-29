import { redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiList } from '$lib/server/api';
import type { DMConversation, DMMessage } from '$lib/types';

/**
 * Messages at /messages — auth-gated DM surface (feature.dm). Renders the
 * conversation list; the active conversation's messages are fetched either
 * server-side (when ?c= is present) or client-side on selection.
 *
 * Ref: japaconnect-engineering/sections/48-dm-websocket.md
 */
const PAGE_SIZE = 30;

export const load: PageServerLoad = async (event) => {
	if (!event.locals.user) {
		throw redirect(307, '/login?redirect=/messages');
	}

	const ctx = apiContext(event);
	const activeId = event.url.searchParams.get('c');

	let conversations: DMConversation[] = [];
	let activeMessages: DMMessage[] = [];

	try {
		conversations = (await apiList<DMConversation>(ctx, `/dm/conversations?page_size=${PAGE_SIZE}`)).items;
	} catch {
		conversations = [];
	}

	if (activeId) {
		try {
			activeMessages = (
				await apiList<DMMessage>(ctx, `/dm/conversations/${activeId}/messages?page_size=${PAGE_SIZE}`)
			).items;
		} catch {
			activeMessages = [];
		}
	}

	return { conversations, activeId, activeMessages, pageSize: PAGE_SIZE };
};
