import { redirect, fail } from '@sveltejs/kit';
import type { PageServerLoad, Actions } from './$types';
import { apiContext, apiList, apiPost, ApiError } from '$lib/server/api';

/**
 * JapaBot chat at /ai/chat — auth-gated conversational AI surface
 * (feature.ai_chat + ai.context.use). Lists prior conversations from Honcho
 * and accepts new messages via a form action that POSTs to the backend
 * /ai/chat route.
 *
 * Ref: japaconnect-engineering/sections/20-ai-core.md
 */
export const load: PageServerLoad = async (event) => {
	if (!event.locals.user) {
		throw redirect(307, '/login?redirect=/ai/chat');
	}

	const ctx = apiContext(event);

	type ConversationEntry = {
		id: string;
		content?: string;
		role?: string;
		created_at?: string;
	};
	let conversations: ConversationEntry[] = [];
	try {
		conversations = (await apiList<ConversationEntry>(ctx, '/ai/conversations')).items;
	} catch {
		conversations = [];
	}

	return { conversations };
};

export const actions: Actions = {
	send: async (event) => {
		if (!event.locals.user) {
			throw redirect(307, '/login');
		}

		const ctx = apiContext(event);
		const form = await event.request.formData();
		const message = (form.get('message') as string | null)?.trim() ?? '';
		const conversationId = (form.get('conversation_id') as string | null) ?? '';

		if (!message) {
			return fail(400, { error: 'message_required' });
		}

		try {
			const reply = await apiPost<{
				reply: string;
				conversation_id: string;
				provider_ref?: string;
				sources?: unknown[];
			}>(ctx, '/ai/chat', { message, conversation_id: conversationId || undefined });

			return { ok: true, reply, message };
		} catch (err) {
			if (err instanceof ApiError) {
				return fail(err.status, { error: err.code, message });
			}
			return fail(500, { error: 'ai_error', message });
		}
	}
};
