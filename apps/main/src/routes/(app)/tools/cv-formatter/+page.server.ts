import type { PageServerLoad } from './$types';
import { apiContext, apiList, ApiError } from '$lib/server/api';
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
 * Ref: §95 (CV Formatter), ROADMAP-Tools 02.06, ADR-088.
 */
export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);
	const authed = !!event.locals.user;

	let disabled = false;
	let destinations: GeoSpaceOption[] = [];
	let standards: CvStandard[] = [];

	// All CV standards — primary call; a 404 with cv_formatter_disabled (or any
	// 404) means the tool is turned off. Also used to filter the destination
	// dropdown to only show destinations that have a standard available, and to
	// display the standard inline when a destination is selected.
	try {
		standards = (await apiList<CvStandard>(ctx, '/cv-formatter/standards')).items;
	} catch (e) {
		if (e instanceof ApiError && (e.code === 'cv_formatter_disabled' || e.status === 404)) {
			disabled = true;
		}
		// Other errors: leave standards empty; page shows an empty state.
	}

	// Destination country list — the primary data axis (ADR-088). Rule 16.
	if (!disabled) {
		try {
			destinations = (await apiList<GeoSpaceOption>(ctx, '/spaces?level=country')).items;
		} catch {
			destinations = [];
		}
	}

	return { destinations, standards, disabled, authed };
};
