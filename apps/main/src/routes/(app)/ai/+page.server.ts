import { redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiGet } from '$lib/server/api';
import type { AgentCapabilities } from '$lib/ai/agentTypes';

/**
 * /ai — the AI Agent chat surface. Auth-gated by the `agent.use` permission
 * (the backend returns 403 `agent_permission_required` if the user lacks it;
 * the page renders a warning banner and the composer surfaces the error on
 * send — redirecting to /ai/chat would silently switch assistants).
 *
 * Capabilities (tool catalog + geo context) are loaded server-side from
 * GET /api/v1/ai/agent/capabilities and passed to the AgentChat component.
 * The live SSE stream is opened client-side (POST /api/v1/ai/agent) because
 * SSE is a long-lived client connection, not a server-rendered resource.
 *
 * Ref: japaconnect-engineering/sections/103-ai-agent-executor.md (API Routes)
 * Ref: japaconnect-engineering/roadmaps/ROADMAP-AI-AGENT.md (Phase 10, tasks 10.02-10.03)
 */
export const load: PageServerLoad = async (event) => {
	if (!event.locals.user) {
		throw redirect(307, '/login?redirect=/ai');
	}

	const ctx = apiContext(event);
	let capabilities: AgentCapabilities | null = null;
	try {
		capabilities = await apiGet<AgentCapabilities>(ctx, '/ai/agent/capabilities');
	} catch {
		// If capabilities fail (agent disabled, no permission, not configured),
		// capabilities stays null — the page renders an error state. The chat
		// component will surface the specific error when the user tries to send.
		capabilities = null;
	}

	return { capabilities };
};
