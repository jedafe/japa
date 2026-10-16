import { redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiGet, apiList } from '$lib/server/api';
import { isMainToolEnabled, MAIN_TOOLS } from '$lib/tools/registry';
import type { AgentCapabilities, AIPrefs, AiProject } from '$lib/ai/agentTypes';
import type { MeSpace, BlogSeries, AssessmentForm, ContentTypeMeta } from '$lib/types';

/**
 * /ai — the unified AI surface (ADR-130): a single chat view served by
 * JapaAIChat. The backend gates POST /ai/chat on feature.japabot +
 * ai.context.use OR agent.use; the composer's tools toggle is what
 * switches between chat and agent turns.
 *
 * /ai/chat 308-redirects here so old links and the nav FAB keep working.
 *
 * Ref: japaconnect-engineering/sections/103-ai-agent-executor.md (API Routes)
 * Ref: japaconnect-engineering/adr/ADR-130-unified-japa-ai.md
 */
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
	const japabotTool = MAIN_TOOLS.find((t) => t.labelKey === 'tools.japabot')!;
	const chatEnabled = isMainToolEnabled(japabotTool, parent.features ?? {});

	if (!chatEnabled) {
		return {
			chatEnabled: false,
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

	// Every call below is independent, so they all fire in one parallel
	// batch — a serial waterfall here dominated SSR time. All best-effort:
	// a denied/failed endpoint degrades its picker or sidebar, never the
	// page. Empty picker lists hide the selects so the backend default
	// applies.
	const [
		capsRes,
		convsRes,
		modelsRes,
		voicesRes,
		projectsRes,
		prefsRes,
		typesRes,
		spacesRes,
		seriesRes,
		statusRes,
		visRes,
		assessmentRes
	] = await Promise.allSettled([
		// Tool catalog + geo context. Drives the composer's category chips +
		// per-tool toggles (empty categories → plain chat). Gated server-side
		// by feature.ai_agent + agent.use — a denied request renders the
		// composer without tool controls.
		apiGet<AgentCapabilities>(ctx, '/ai/agent/capabilities'),
		// Conversation list (V2 unions ai_conversations with legacy Honcho
		// threads).
		apiList<ConversationSummary>(ctx, '/ai/conversations'),
		apiList<ChatModelOption>(ctx, '/ai/models?category=chat'),
		// ?slim=1 — the {id, name} picker projection; full rows carry
		// voice_prompt (~1.7KB each → ~2.6MB).
		apiList<ChatVoiceOption>(ctx, '/ai/voices?slim=1&page_size=5000'),
		// §103 AI Projects — sidebar grouping collections. page_size=100
		// matches the ai package's parsePageSize ceiling — anything larger
		// is silently clamped server-side. An empty list renders a flat
		// conversation sidebar.
		apiList<AiProject>(ctx, '/ai/projects?page_size=100'),
		// ADR-130: user AI preferences (default mode, disabled
		// categories/tools) seed the composer's tools popover. Defaults are
		// chat mode with nothing disabled.
		apiGet<AIPrefs>(ctx, '/me/ai-preferences'),
		// Chat template panel catalogs (content types, spaces, series,
		// status/visibility options, assessment dimensions) — the same
		// lists the studio prompt rail loads (Rule 16).
		apiList<ContentTypeMeta>(ctx, '/content-types'),
		// member_only cap — SpaceSelector live-searches via /me/spaces?q=
		// anyway; the prop only seeds initial suggestions. Uncapped this
		// returns every platform space (~10k rows / 1.7MB for admins).
		apiList<MeSpace>(ctx, '/me/spaces?member_only=1&page_size=50'),
		apiList<BlogSeries>(ctx, '/series'),
		apiGet<{ value: string; label: string }[]>(ctx, '/content/status-options'),
		apiGet<{ value: string; label: string }[]>(ctx, '/content/visibility-options'),
		// §71 — active assessment form; dimensions populate the Journey
		// tab's roadmap pickers.
		apiGet<AssessmentForm>(ctx, '/assessment')
	]);

	// The composer reads only capabilities.categories (chips + toggles) and
	// geo_context — the flat tools[] catalog ships a full JSON Schema per
	// tool (~120KB serialized) that no UI element reads, so it is dropped
	// before the data lands in the SSR payload.
	const capabilities: AgentCapabilities | null =
		capsRes.status === 'fulfilled' && capsRes.value
			? { ...capsRes.value, tools: [] }
			: null;
	const conversations: ConversationSummary[] =
		convsRes.status === 'fulfilled' ? convsRes.value.items : [];
	const models: ChatModelOption[] =
		modelsRes.status === 'fulfilled' ? modelsRes.value.items : [];
	// page_size=5000 = the handler's max (parsePaginationWithMax) — the full
	// catalog is needed so client-side tone filtering sees every voice, not
	// just the first alphabetical page. The map stays as a cheap guard so
	// only {id, name} ever lands in the SSR payload.
	const voices: ChatVoiceOption[] =
		voicesRes.status === 'fulfilled'
			? voicesRes.value.items.map((v) => ({ id: v.id, name: v.name }))
			: [];
	const projects: AiProject[] =
		projectsRes.status === 'fulfilled' ? projectsRes.value.items : [];
	const prefs: AIPrefs | null = prefsRes.status === 'fulfilled' ? prefsRes.value : null;

	return {
		chatEnabled,
		capabilities,
		conversations,
		models,
		voices,
		projects,
		prefs,
		contentTypes: typesRes.status === 'fulfilled' ? typesRes.value.items : [],
		spaces: spacesRes.status === 'fulfilled' ? spacesRes.value.items : [],
		series: seriesRes.status === 'fulfilled' ? seriesRes.value.items : [],
		statusOptions: statusRes.status === 'fulfilled' ? statusRes.value : [],
		visibilityOptions: visRes.status === 'fulfilled' ? visRes.value : [],
		assessmentDimensions:
			assessmentRes.status === 'fulfilled' ? (assessmentRes.value?.dimensions ?? []) : []
	};
};
