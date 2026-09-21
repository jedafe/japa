import type { PageServerLoad } from './$types';
import { apiContext, apiList } from '$lib/server/api';
import { loadCountries } from '$lib/server/geo';
import { loadCatalog } from '$lib/server/catalog';
import { getTool, isToolEnabled } from '$lib/tools/registry';
import type { GeoSpaceOption, VisaTimeline, CatalogOption } from '$lib/types';

/**
 * My Applications — the personal tracker half of the Visa Processing Time
 * Tracker (§83 personal milestones, §93 tool surface).
 *
 * Authenticated users log milestone dates on their live visa applications
 * and can optionally share them (anonymised) into the community aggregate.
 * Anonymous visitors get a sign-in prompt — there is no anonymous state.
 *
 * API: GET /me/visa-timelines — user's own timelines (auth only).
 *      POST /me/visa-timelines, PATCH /me/visa-timelines/{id},
 *      POST/PATCH /me/visa-timelines/{id}/milestones* — client-side.
 *      GET /catalogs/visa_timeline.* — milestone/outcome/method options (Rule 16).
 *
 * Ref: §83 (Visa Timeline Tracker), §93 (tool surface), ROADMAP-TOOLS 05.04.
 */
export const load: PageServerLoad = async (event) => {
	const tool = getTool('processing-times')!;
	if (!isToolEnabled(tool, event.locals.features)) {
		return {
			enabled: false,
			tool,
			destinations: [],
			timelines: [],
			milestoneTypeOptions: [],
			outcomeOptions: [],
			methodOptions: [],
			user: event.locals.user
		};
	}

	const ctx = apiContext(event);

	let destinations: GeoSpaceOption[] = [];
	try {
		destinations = await loadCountries(ctx);
	} catch {
		destinations = [];
	}

	// Catalog-backed option lists (Rule 16 — no hardcoded option lists).
	let milestoneTypeOptions: CatalogOption[] = [];
	let outcomeOptions: CatalogOption[] = [];
	let methodOptions: CatalogOption[] = [];
	try {
		[milestoneTypeOptions, outcomeOptions, methodOptions] = await Promise.all([
			loadCatalog(ctx, 'visa_timeline.milestone_type'),
			loadCatalog(ctx, 'visa_timeline.outcome'),
			loadCatalog(ctx, 'visa_timeline.application_method')
		]);
	} catch {
		milestoneTypeOptions = [];
		outcomeOptions = [];
		methodOptions = [];
	}

	let timelines: VisaTimeline[] = [];
	if (event.locals.user) {
		try {
			timelines = (await apiList<VisaTimeline>(ctx, '/me/visa-timelines')).items;
		} catch {
			timelines = [];
		}
	}

	return {
		enabled: true,
		tool,
		destinations,
		timelines,
		milestoneTypeOptions,
		outcomeOptions,
		methodOptions,
		user: event.locals.user
	};
};
