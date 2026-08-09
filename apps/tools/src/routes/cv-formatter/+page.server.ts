import type { PageServerLoad } from './$types';
import { apiContext, apiList } from '$lib/server/api';
import { getTool, isToolEnabled } from '$lib/tools/registry';
import type { GeoSpaceOption, CvStandard } from '$lib/types';

/**
 * CV Formatter — §95.
 * SSR loads the destination country list and all CV standards. The standards
 * list is used to filter the destination dropdown to only show destinations
 * that have a standard available.
 *
 * API: GET /spaces?level=country — destination selector options.
 *      GET /cv-formatter/standards — all CV standards (public).
 *      GET /cv-formatter/standards/{dest} — single standard for a destination.
 *      POST /cv-formatter/format — AI-assisted formatting (auth, 202 Accepted).
 *      GET /cv-formatter/sessions/{id} — poll session status (auth, owner).
 *      GET /cv-formatter/sessions/{id}/download — binary download (auth, owner).
 *
 * Ref: §95 (CV Formatter), ROADMAP-TOOLS 02.06, ADR-088.
 */
export const load: PageServerLoad = async (event) => {
	// Gate: skip all backend calls when cv_formatter.enabled is false.
	// Ref: conventions/system-settings-catalog.md, Rule 3.
	const tool = getTool('cv-formatter')!;
	const settings = event.locals.siteSettings;
	if (!isToolEnabled(tool, settings)) {
		return {
			enabled: false,
			tool,
			destinations: [],
			standards: [],
			user: event.locals.user
		};
	}

	const ctx = apiContext(event);

	// Destination country list — the primary data axis (ADR-088). Rule 16.
	let destinations: GeoSpaceOption[] = [];
	try {
		destinations = (await apiList<GeoSpaceOption>(ctx, '/spaces?level=country')).items;
	} catch {
		destinations = [];
	}

	// All CV standards — used to filter the destination dropdown to only
	// show destinations that have a standard available, and to display the
	// standard inline when a destination is selected.
	let standards: CvStandard[] = [];
	try {
		standards = (await apiList<CvStandard>(ctx, '/cv-formatter/standards')).items;
	} catch {
		standards = [];
	}

	return {
		enabled: true,
		tool,
		destinations,
		standards,
		user: event.locals.user
	};
};
