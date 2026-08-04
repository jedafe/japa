import { redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { getTool } from '$lib/tools/registry';

/**
 * Tools hub page — the landing page at `/`.
 *
 * In hub shell mode (tools.japaconnect.com): renders the tool cards grid.
 * In single-tool shell mode (e.g. remittance.japaconnect.com): redirects to
 * the active tool's route so the user lands directly on the tool, not the hub.
 *
 * Ref: §78 (Tools Web Surface), ROADMAP-TOOLS 02.01.
 */
export const load: PageServerLoad = async ({ locals }) => {
	if (locals.toolSurface !== 'hub') {
		const tool = getTool(locals.toolSurface);
		if (tool) {
			throw redirect(307, tool.route);
		}
	}
	return {};
};
