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

	// Tool catalog + geo context. Drives the composer's category chips +
	// per-tool toggles (empty categories → plain chat). Best-effort —
	// gated server-side by feature.ai_agent + agent.use, so a denied
	// request renders the composer without tool controls.
	let capabilities: AgentCapabilities | null = null;
	try {
		capabilities = await apiGet<AgentCapabilities>(ctx, '/ai/agent/capabilities');
	} catch {
		capabilities = null;
	}

	// Conversation list (V2 unions ai_conversations with legacy Honcho
	// threads) + composer picker options. All best-effort: empty picker
	// lists just hide the selects so the backend default applies.
	let conversations: ConversationSummary[] = [];
	let models: ChatModelOption[] = [];
	let voices: ChatVoiceOption[] = [];
	let projects: AiProject[] = [];
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

	// ADR-130: user AI preferences (default mode, disabled categories/tools)
	// seed the composer's tools popover. Best-effort — defaults are chat mode
	// with nothing disabled.
	let prefs: AIPrefs | null = null;
	try {
		prefs = await apiGet<AIPrefs>(ctx, '/me/ai-preferences');
	} catch {
		prefs = null;
	}

	// Chat template panel catalogs (content types, spaces, series,
	// status/visibility options, assessment dimensions) — the same lists
	// the studio prompt rail loads (Rule 16). All best-effort: empty
	// lists degrade the picker rather than the page.
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
