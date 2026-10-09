import { error } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiGet, apiGetEnvelope } from '$lib/server/api';
import type { VisaRouteWithAi } from '$lib/types';

/**
 * Visa Route detail page — Section 90, ADR-098.
 *
 * API: GET /visa-routes/:id
 *
 * Ref: japaconnect-engineering/sections/90-visa-route-comparison.md
 */
export const load: PageServerLoad = async (event) => {
	const { features } = await event.parent();
	if (!features['visa_routes.enabled']) {
		throw error(404, { code: 'feature_disabled', message: 'Visa Route Comparison is not available' });
	}

	const ctx = apiContext(event);
	const routeId = event.params.id;

	let route: VisaRouteWithAi | null = null;
	let jsonLd: unknown = null;
	try {
		const envelope = await apiGetEnvelope<VisaRouteWithAi>(ctx, `/visa-routes/${routeId}`);
		route = envelope.data;
		jsonLd = (envelope as { json_ld?: unknown }).json_ld ?? null;
	} catch {
		route = null;
	}

	if (!route) {
		throw error(404, { code: 'visa_route_not_found', message: 'Visa route not found' });
	}

	return { route, jsonLd };
};
