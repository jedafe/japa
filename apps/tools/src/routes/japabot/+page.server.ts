import type { PageServerLoad } from './$types';
import { apiContext, apiList } from '$lib/server/api';
import type { ChatConversation } from '$lib/types';

/**
 * JapaBot standalone — §20, §69.
 * SSR loads the user's conversation list (if authenticated). The chat
 * interaction is client-side via POST /ai/chat.
 *
 * API: GET /ai/conversations — user's conversation history (auth only).
 *      POST /ai/chat — send message (client-side).
 *
 * Ref: §20 (AI Core), §69 (RAG Knowledge Layer), ROADMAP-TOOLS 02.07.
 */
export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);

	let conversations: ChatConversation[] = [];
	if (event.locals.user) {
		try {
			const res = await apiList<ChatConversation>(ctx, '/ai/conversations');
			conversations = res.items;
		} catch {
			conversations = [];
		}
	}

	return {
		conversations,
		user: event.locals.user
	};
};
