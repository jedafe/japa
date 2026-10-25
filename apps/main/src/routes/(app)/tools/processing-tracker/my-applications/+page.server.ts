import { error } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiGet, apiList } from '$lib/server/api';
import type { CatalogOption, GeoSpaceOption, VisaTimeline } from '$lib/types';

/**
 * My Applications — the personal tracker half of the Visa Processing Time
 * Tracker (§83 personal milestones, §93). Ported from the removed tools
 * surface (ADR-086 superseded).
 *
 * Authenticated users log milestone dates on their live visa applications
 * and can optionally share them (anonymised) into the community aggregate.
 * Anonymous visitors get a sign-in prompt — there is no anonymous state.
 *
 * API: GET /me/visa-timelines — user's own timelines (auth only).
 *      POST /me/visa-timelines, PATCH /me/visa-timelines/{id},
 *      POST/PATCH /me/visa-timelines/{id}/milestones* — client-side.
 *      GET /catalogs/visa_timeline.* — milestone/outcome/method options (Rule 16).
 */
export const load: PageServerLoad = async (event) => {
	const { features } = await event.parent();
	if (!features['visa_timeline.enabled']) {
		throw error(404, {
			code: 'feature_disabled',
			message: 'Visa Processing Time Tracker is not available'
		});
	}

	const ctx = apiContext(event);
	const authed = !!event.locals.user;

	let destinations: GeoSpaceOption[] = [];
	try {
		destinations = (await apiList<GeoSpaceOption>(ctx, '/geo/countries')).items;
	} catch {
		destinations = [];
	}

	// Catalog-backed option lists (Rule 16 — no hardcoded option lists).
	let milestoneTypeOptions: CatalogOption[] = [];
	let outcomeOptions: CatalogOption[] = [];
	let methodOptions: CatalogOption[] = [];
	try {
		[milestoneTypeOptions, outcomeOptions, methodOptions] = await Promise.all([
			apiGet<CatalogOption[]>(ctx, '/catalogs/visa_timeline.milestone_type'),
			apiGet<CatalogOption[]>(ctx, '/catalogs/visa_timeline.outcome'),
			apiGet<CatalogOption[]>(ctx, '/catalogs/visa_timeline.application_method')
		]);
	} catch {
		milestoneTypeOptions = [];
		outcomeOptions = [];
		methodOptions = [];
	}

	let timelines: VisaTimeline[] = [];
	if (authed) {
		try {
			timelines = (await apiList<VisaTimeline>(ctx, '/me/visa-timelines')).items;
		} catch {
			timelines = [];
		}
	}

	return {
		destinations,
		timelines,
		milestoneTypeOptions,
		outcomeOptions,
		methodOptions,
		authed
	};
};
