import { error } from '@sveltejs/kit';
import { redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiGet, ApiError } from '$lib/server/api';
import type { AgentSessionDetail } from '$lib/ai/agentTypes';

/**
 * /ai/sessions/[id] — single agent session detail. SSR fetches
 * GET /api/v1/ai/agent/sessions/{id} which returns the session row plus
 * its full tool-call audit log. The backend enforces ownership at the DB
 * layer (GetAgentSessionForUser: user_id + id) so a 404 here
 * means either the session doesn't exist or belongs to another user.
 *
 * Ref: japaconnect-engineering/sections/103-ai-agent-executor.md (API Routes)
 * Ref: japaconnect-engineering/roadmaps/ROADMAP-AI-AGENT.md (Phase 10, task 10.04)
 */
export const load: PageServerLoad = async (event) => {
	if (!event.locals.user) {
		throw redirect(307, `/login?redirect=/ai/sessions/${event.params.id}`);
	}

	const ctx = apiContext(event);
	const id = event.params.id;

	let detail: AgentSessionDetail | null = null;
	let loadError = false;
	try {
		detail = await apiGet<AgentSessionDetail>(ctx, `/ai/agent/sessions/${id}`);
	} catch (err) {
		if (err instanceof ApiError && err.status === 404) {
			throw error(404, { code: 'session_not_found', message: 'Agent session not found' });
		}
		detail = null;
		loadError = true;
	}

	return { detail, loadError };
};
