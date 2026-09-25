import { redirect, fail } from '@sveltejs/kit';
import type { PageServerLoad, Actions } from './$types';
import { apiContext, apiList, apiPost, ApiError } from '$lib/server/api';
import { isMainToolEnabled, MAIN_TOOLS } from '$lib/tools/registry';

/**
 * JapaBot chat at /ai/chat — auth-gated conversational AI surface
 * (feature.ai_chat + ai.context.use). Lists prior conversations from Honcho
 * and accepts new messages via a form action that POSTs to the backend
 * /ai/chat route.
 *
 * Section 69 / ADR-069: RAG-grounded responses include source citations
 * (entity_type, entity_id, similarity, tier) which the client renders
 * beneath each assistant reply.
 *
 * Ref: japaconnect-engineering/sections/20-ai-core.md
 *      japaconnect-engineering/sections/69-rag-knowledge-layer.md
 */
export interface AiChatSource {
	entity_type?: string;
	entity_id?: string;
	similarity?: number;
	tier?: string;
	// §69 Citation Source Contract — emitted by the backend; empty when the
	// entity no longer resolves. Never derive hrefs from entity_id.
	title?: string;
	slug?: string;
	url?: string;
}

export interface ConversationSummary {
	conversation_id: string;
	preview: string;
	turn_count: number;
	last_entry_at: string;
}

export const load: PageServerLoad = async (event) => {
	if (!event.locals.user) {
		throw redirect(307, '/login?redirect=/ai/chat');
	}

	// Gate the chat surface with the feature.japabot flag. When disabled the
	// page renders a disabled state instead of calling the backend
	// (BUG-014, Section 42). Parent data comes from the root layout's
	// feature-flags fetch.
	const parent = await event.parent();
	const japabotTool = MAIN_TOOLS.find((t) => t.href === '/ai/chat')!;
	const enabled = isMainToolEnabled(japabotTool, parent.features ?? {});
	if (!enabled) {
		return { enabled: false, conversations: [] };
	}

	const ctx = apiContext(event);

	let conversations: ConversationSummary[] = [];
	try {
		conversations = (await apiList<ConversationSummary>(ctx, '/ai/conversations')).items;
	} catch {
		conversations = [];
	}

	return { enabled: true, conversations };
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
			const result = await apiPost<{
				reply: string;
				conversation_id: string;
				provider_ref?: string;
				sources?: AiChatSource[];
			}>(ctx, '/ai/chat', { message, conversation_id: conversationId || undefined });

			return {
				ok: true,
				reply: result.reply,
				conversation_id: result.conversation_id,
				sources: result.sources ?? [],
				message
			};
		} catch (err) {
			if (err instanceof ApiError) {
				return fail(err.status, { error: err.code, detail: err.message, message });
			}
			return fail(500, { error: 'ai_error', detail: 'JapaBot is unavailable', message });
		}
	}
};
