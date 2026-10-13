import { redirect, fail } from '@sveltejs/kit';
import type { PageServerLoad, Actions } from './$types';
import { apiContext, apiGet, apiList, apiPost, ApiError } from '$lib/server/api';
import { isMainToolEnabled, MAIN_TOOLS } from '$lib/tools/registry';
import type { AgentCapabilities } from '$lib/ai/agentTypes';

/**
 * /ai — the unified AI surface. Two modes share one route behind a tab
 * switcher (?mode=chat|agent):
 *
 * - chat  — JapaBot: conversational RAG assistant (Section 20/69), gated by
 *           feature.japabot + the backend's ai.context.use permission.
 * - agent — AI Agent: tool-augmented SSE executor (Section 103/ADR-111),
 *           gated by feature.ai_agent + agent.use.
 *
 * Each mode loads only its own data. The chat composer posts to ?/send.
 * /ai/chat 308-redirects here so old links and the nav FAB keep working.
 *
 * Ref: japaconnect-engineering/sections/103-ai-agent-executor.md (API Routes)
 * Ref: japaconnect-engineering/sections/20-ai-core.md
 * Ref: japaconnect-engineering/sections/69-rag-knowledge-layer.md
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

export interface ChatModelOption {
	id: string;
	label: string;
}

export interface ChatVoiceOption {
	id: string;
	name: string;
}

export const load: PageServerLoad = async (event) => {
	if (!event.locals.user) {
		throw redirect(307, '/login?redirect=/ai');
	}

	const parent = await event.parent();
	const agentTool = MAIN_TOOLS.find((t) => t.labelKey === 'tools.ai_agent')!;
	const japabotTool = MAIN_TOOLS.find((t) => t.labelKey === 'tools.japabot')!;
	const agentEnabled = isMainToolEnabled(agentTool, parent.features ?? {});
	const chatEnabled = isMainToolEnabled(japabotTool, parent.features ?? {});

	if (!agentEnabled && !chatEnabled) {
		return {
			agentEnabled: false,
			chatEnabled: false,
			capabilities: null,
			conversations: [],
			models: [],
			voices: []
		};
	}

	const ctx = apiContext(event);

	// Agent mode: tool catalog + geo context (best-effort — a null renders
	// the capabilities warning and the composer surfaces the send error).
	let capabilities: AgentCapabilities | null = null;
	if (agentEnabled) {
		try {
			capabilities = await apiGet<AgentCapabilities>(ctx, '/ai/agent/capabilities');
		} catch {
			capabilities = null;
		}
	}

	// Chat mode: Honcho conversation list + composer picker options. All
	// best-effort: empty picker lists just hide the selects so the backend
	// default applies.
	let conversations: ConversationSummary[] = [];
	let models: ChatModelOption[] = [];
	let voices: ChatVoiceOption[] = [];
	if (chatEnabled) {
		try {
			conversations = (await apiList<ConversationSummary>(ctx, '/ai/conversations')).items;
		} catch {
			conversations = [];
		}
		try {
			models = (await apiList<ChatModelOption>(ctx, '/ai/models?category=chat')).items;
		} catch {
			models = [];
		}
		try {
			voices = (await apiList<ChatVoiceOption>(ctx, '/ai/voices?page_size=100')).items;
		} catch {
			voices = [];
		}
	}

	return { agentEnabled, chatEnabled, capabilities, conversations, models, voices };
};

export const actions: Actions = {
	// JapaBot send — posts the user message to the backend /ai/chat route.
	// Returns the reply + RAG citations; failures echo `message` so the
	// composer can restore the user's draft.
	send: async (event) => {
		if (!event.locals.user) {
			throw redirect(307, '/login');
		}

		const ctx = apiContext(event);
		const form = await event.request.formData();
		const message = (form.get('message') as string | null)?.trim() ?? '';
		const conversationId = (form.get('conversation_id') as string | null) ?? '';
		const model = (form.get('model') as string | null)?.trim() ?? '';
		const voiceId = (form.get('voice_id') as string | null)?.trim() ?? '';

		if (!message) {
			return fail(400, { error: 'message_required' });
		}

		try {
			const result = await apiPost<{
				reply: string;
				conversation_id: string;
				provider_ref?: string;
				sources?: AiChatSource[];
			}>(ctx, '/ai/chat', {
				message,
				conversation_id: conversationId || undefined,
				model: model || undefined,
				voice_id: voiceId || undefined
			});

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
