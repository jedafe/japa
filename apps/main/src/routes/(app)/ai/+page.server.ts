import { redirect, fail } from '@sveltejs/kit';
import type { PageServerLoad, Actions } from './$types';
import { apiContext, apiGet, apiList, apiPost, ApiError } from '$lib/server/api';
import { isMainToolEnabled, MAIN_TOOLS } from '$lib/tools/registry';
import type { AgentCapabilities, AIPrefs, AiProject } from '$lib/ai/agentTypes';
import type { MeSpace, BlogSeries, AssessmentForm, ContentTypeMeta } from '$lib/types';

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
	title?: string;
	preview: string;
	turn_count: number;
	last_entry_at: string;
	/** ADR-130: Honcho-era thread — read-only, not deletable. */
	legacy?: boolean;
	/** §103 AI Projects — owning project's id; absent when ungrouped. */
	project_id?: string;
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
		throw redirect(
			307,
			`/login?redirect=${encodeURIComponent(event.url.pathname + event.url.search)}`
		);
	}

	const parent = await event.parent();
	const agentTool = MAIN_TOOLS.find((t) => t.labelKey === 'tools.ai_agent')!;
	const japabotTool = MAIN_TOOLS.find((t) => t.labelKey === 'tools.japabot')!;
	const agentEnabled = isMainToolEnabled(agentTool, parent.features ?? {});
	const chatEnabled = isMainToolEnabled(japabotTool, parent.features ?? {});
	// ADR-130: when the rollout flag is on, /ai renders the single unified
	// chat surface instead of the chat/agent tab switcher. The surface still
	// requires feature.japabot (the backend gates /ai/chat on it).
	const unifiedEnabled =
		(parent.features ?? {})['feature.japa_ai_unified'] === true && chatEnabled;

	if (!agentEnabled && !chatEnabled) {
		return {
			agentEnabled: false,
			chatEnabled: false,
			unifiedEnabled: false,
			capabilities: null,
			conversations: [],
			models: [],
			voices: [],
			projects: [],
			prefs: null,
			contentTypes: [],
			spaces: [],
			series: [],
			statusOptions: [],
			visibilityOptions: [],
			assessmentDimensions: []
		};
	}

	const ctx = apiContext(event);

	// Tool catalog + geo context. On the unified surface this also drives the
	// composer's category chips + per-tool toggles (empty categories → plain
	// chat). Best-effort — null renders without tool controls.
	let capabilities: AgentCapabilities | null = null;
	if (agentEnabled || unifiedEnabled) {
		try {
			capabilities = await apiGet<AgentCapabilities>(ctx, '/ai/agent/capabilities');
		} catch {
			capabilities = null;
		}
	}

	// Conversation list (V2 unions ai_conversations with legacy Honcho
	// threads) + composer picker options. All best-effort: empty picker
	// lists just hide the selects so the backend default applies.
	let conversations: ConversationSummary[] = [];
	let models: ChatModelOption[] = [];
	let voices: ChatVoiceOption[] = [];
	let projects: AiProject[] = [];
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
			// page_size=5000 = the handler's max (parsePaginationWithMax) — the
			// full catalog is needed so client-side tone filtering sees every
			// voice, not just the first alphabetical page.
			voices = (await apiList<ChatVoiceOption>(ctx, '/ai/voices?page_size=5000')).items;
		} catch {
			voices = [];
		}
		// §103 AI Projects — sidebar grouping collections. Best-effort like
		// the rest: an empty list renders a flat conversation sidebar.
		try {
			// page_size=100 matches the ai package's parsePageSize ceiling —
			// anything larger is silently clamped server-side.
			projects = (await apiList<AiProject>(ctx, '/ai/projects?page_size=100')).items;
		} catch {
			projects = [];
		}
	}

	// ADR-130: user AI preferences (default mode, disabled categories/tools)
	// seed the composer's tools popover. Best-effort — defaults are chat mode
	// with nothing disabled.
	let prefs: AIPrefs | null = null;
	if (unifiedEnabled) {
		try {
			prefs = await apiGet<AIPrefs>(ctx, '/me/ai-preferences');
		} catch {
			prefs = null;
		}
	}

	// Chat template panel catalogs (content types, spaces, series,
	// status/visibility options, assessment dimensions) — the same lists
	// the studio prompt rail loads (Rule 16). All best-effort: empty
	// lists degrade the picker rather than the page.
	let contentTypes: ContentTypeMeta[] = [];
	let spaces: MeSpace[] = [];
	let series: BlogSeries[] = [];
	let statusOptions: { value: string; label: string }[] = [];
	let visibilityOptions: { value: string; label: string }[] = [];
	let assessmentDimensions: string[] = [];
	if (unifiedEnabled) {
		const [typesRes, spacesRes, seriesRes, statusRes, visRes, assessmentRes] =
			await Promise.allSettled([
				apiList<ContentTypeMeta>(ctx, '/content-types'),
				apiList<MeSpace>(ctx, '/me/spaces'),
				apiList<BlogSeries>(ctx, '/series'),
				apiGet<{ value: string; label: string }[]>(ctx, '/content/status-options'),
				apiGet<{ value: string; label: string }[]>(ctx, '/content/visibility-options'),
				// §71 — active assessment form; dimensions populate the Journey
				// tab's roadmap pickers.
				apiGet<AssessmentForm>(ctx, '/assessment')
			]);
		contentTypes = typesRes.status === 'fulfilled' ? typesRes.value.items : [];
		spaces = spacesRes.status === 'fulfilled' ? spacesRes.value.items : [];
		series = seriesRes.status === 'fulfilled' ? seriesRes.value.items : [];
		statusOptions = statusRes.status === 'fulfilled' ? statusRes.value : [];
		visibilityOptions = visRes.status === 'fulfilled' ? visRes.value : [];
		assessmentDimensions =
			assessmentRes.status === 'fulfilled' ? (assessmentRes.value?.dimensions ?? []) : [];
	}

	return {
		agentEnabled,
		chatEnabled,
		unifiedEnabled,
		capabilities,
		conversations,
		models,
		voices,
		projects,
		prefs,
		contentTypes,
		spaces,
		series,
		statusOptions,
		visibilityOptions,
		assessmentDimensions
	};
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
