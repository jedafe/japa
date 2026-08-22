import { error } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiGetOrNull } from '$lib/server/api';
import { getTool, isToolEnabled } from '$lib/tools/registry';
import type { VisaRouteWithAi } from '$lib/types';

/**
 * Visa route detail page — §90, ADR-104.
 *
 * API: GET /visa-routes/{id}
 *
 * Ref: §90 (Visa Route Comparison), ROADMAP-TOOLS 02.06.
 */
export const load: PageServerLoad = async (event) => {
	// Gate: if the tool is disabled, surface a 404 so the route is unreachable.
	const tool = getTool('visa-routes')!;
	if (!isToolEnabled(tool, event.locals.features)) {
		throw error(404, { code: 'tool_disabled', message: 'Visa Route Comparison is not available' });
	}

	const ctx = apiContext(event);
	const route = await apiGetOrNull<VisaRouteWithAi>(ctx, `/visa-routes/${event.params.id}`);

	if (!route) {
		throw error(404, { code: 'visa_route_not_found', message: 'Visa route not found' });
	}

	return { route, user: event.locals.user };
};
