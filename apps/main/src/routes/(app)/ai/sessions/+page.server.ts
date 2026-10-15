import { redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiList } from '$lib/server/api';
import type { AgentSession, AiProject } from '$lib/ai/agentTypes';

/**
 * /ai/sessions — agent session history list. Auth-gated. Loads the first
 * page of the current user's agent sessions server-side from
 * GET /api/v1/ai/agent/sessions (paginated, offset-based).
 *
 * Ref: japaconnect-engineering/sections/103-ai-agent-executor.md (API Routes)
 * Ref: japaconnect-engineering/roadmaps/ROADMAP-AI-AGENT.md (Phase 10, task 10.04)
 */
export const load: PageServerLoad = async (event) => {
	if (!event.locals.user) {
		throw redirect(
			307,
			`/login?redirect=${encodeURIComponent(event.url.pathname + event.url.search)}`
		);
	}

	const ctx = apiContext(event);
	const pageSize = 25;
	const offsetParam = event.url.searchParams.get('offset');
	const offset = offsetParam ? Math.max(0, parseInt(offsetParam, 10) || 0) : 0;

	let sessions: AgentSession[] = [];
	let total = 0;
	try {
		const result = await apiList<AgentSession>(ctx, `/ai/agent/sessions?page_size=${pageSize}&page_offset=${offset}`);
		sessions = result.items;
		// ListMeta.total is optional; fall back to items length when absent.
		total = result.meta?.total ?? result.items.length;
	} catch {
		// Degrade to empty list — the page renders an empty state.
		sessions = [];
		total = 0;
	}

	// §103 AI Projects — the per-row "Move to project" menu needs the
	// catalog. Best-effort: an empty list hides the control.
	let projects: AiProject[] = [];
	try {
		projects = (await apiList<AiProject>(ctx, '/ai/projects?page_size=100')).items;
	} catch {
		projects = [];
	}

	return { sessions, total, pageSize, offset, projects };
};
