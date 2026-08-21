import type { PageServerLoad } from './$types';
import { apiContext } from '$lib/server/api';
import { loadCatalog } from '$lib/server/catalog';
import { loadCountries } from '$lib/server/geo';
import { getTool, isToolEnabled } from '$lib/tools/registry';
import type { GeoSpaceOption, CatalogOption } from '$lib/types';

/**
 * Tax Calculator — §100, ADR-088.
 * SSR loads the destination country list from GET /geo/countries (the
 * geo-aware pattern). No initial tax data is fetched — the tool requires both
 * an origin and a destination selection before querying the backend.
 *
 * API: GET /geo/countries — the full 250 primary geo space countries (geo selector).
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
	if (!isToolEnabled(tool, event.locals.features)) {
		return {
			enabled: false,
			tool,
			destinations: [],
			employmentOptions: [],
			user: event.locals.user
		};
	}

	const ctx = apiContext(event);

	// Destination list — used for BOTH origin and destination selectors
	// (Rule 16 — the country list comes from the backend API, never hardcoded).
	let destinations: GeoSpaceOption[] = [];
	try {
		destinations = await loadCountries(ctx);
	} catch {
		destinations = [];
	}

	// Employment status filter options from the public catalog (Rule 16).
	let employmentOptions: CatalogOption[] = [];
	try {
		employmentOptions = await loadCatalog(ctx, 'tax_calculator.employment_status');
	} catch {
		employmentOptions = [];
	}

	return {
		enabled: true,
		tool,
		destinations,
		employmentOptions,
		user: event.locals.user
	};
};
