import type { PageServerLoad } from './$types';
import { apiContext, apiList } from '$lib/server/api';
import { getTool, isToolEnabled } from '$lib/tools/registry';
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
	// Gate: skip all backend calls when ai.japabot_enabled is false.
	// Ref: conventions/system-settings-catalog.md, Rule 3.
	const tool = getTool('japabot')!;
	if (!isToolEnabled(tool, event.locals.siteSettings)) {
		return { enabled: false, tool, conversations: [], user: event.locals.user };
	}

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
		enabled: true,
		tool,
		conversations,
		user: event.locals.user
	};
};
