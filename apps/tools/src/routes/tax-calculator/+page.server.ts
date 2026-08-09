import type { PageServerLoad } from './$types';
import { apiContext, apiList } from '$lib/server/api';
import { getTool, isToolEnabled } from '$lib/tools/registry';
import type { GeoSpaceOption } from '$lib/types';

/**
 * Tax Calculator — §100, ADR-088.
 * SSR loads the destination country list from /spaces?level=country (the
 * geo-aware pattern). No initial tax data is fetched — the tool requires both
 * an origin and a destination selection before querying the backend.
 *
 * API: GET /spaces?level=country — destination + origin countries (geo selector).
 *      GET /tax-calculator?dest={space_id}&origin={space_id}&employment={status}
 *          — tax guides for an origin→destination pair.
 *      GET /tax-calculator/treaty?dest={space_id}&origin={space_id}
 *          — double taxation treaty info.
 *
 * Ref: §100 (Tax Calculator), ROADMAP-TOOLS 02.06.
 */
export const load: PageServerLoad = async (event) => {
	// Gate: skip all backend calls when tax_calculator.enabled is false.
	// Ref: conventions/system-settings-catalog.md, Rule 3.
	const tool = getTool('tax-calculator')!;
	const settings = event.locals.siteSettings;
	if (!isToolEnabled(tool, settings)) {
		return {
			enabled: false,
			tool,
			destinations: [],
			user: event.locals.user
		};
	}

	const ctx = apiContext(event);

	// Destination list — used for BOTH origin and destination selectors
	// (Rule 16 — the country list comes from the backend API, never hardcoded).
	let destinations: GeoSpaceOption[] = [];
	try {
		destinations = (await apiList<GeoSpaceOption>(ctx, '/spaces?level=country')).items;
	} catch {
		destinations = [];
	}

	return {
		enabled: true,
		tool,
		destinations,
		user: event.locals.user
	};
};
