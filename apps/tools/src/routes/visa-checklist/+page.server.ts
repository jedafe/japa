import type { PageServerLoad } from './$types';
import { apiContext, apiGet, apiGetOrNull } from '$lib/server/api';
import { loadCountries } from '$lib/server/geo';
import { getTool, isToolEnabled } from '$lib/tools/registry';
import type { ChecklistSelector, ChecklistGuide, ChecklistProgress, GeoSpaceOption } from '$lib/types';

/**
 * Visa Document Checklist — §70, ADR-070.
 * SSR loads the origin/destination country lists from /geo/countries and the
 * checklist selector (origin/destination metadata + visa types) from
 * /checklist/selector. The checklist itself is loaded after the user selects
 * origin → destination → visa type.
 *
 * API: GET /geo/countries — the full 250 primary geo space countries (geo selector).
 *      GET /checklist/selector — origin/destination metadata + visa-type options.
 *      GET /checklist/:guide_id — checklist items.
 *      GET /checklist/:guide_id/progress — user's progress (auth only).
 *
 * Ref: §70 (Visa Document Checklist), ROADMAP-TOOLS 02.02.
 */
export const load: PageServerLoad = async (event) => {
	// Gate: skip all backend calls when checklist.enabled is false.
	// Ref: conventions/system-settings-catalog.md, Rule 3.
	const tool = getTool('visa-checklist')!;
	if (!isToolEnabled(tool, event.locals.features)) {
		return { enabled: false, tool, origins: [], destinations: [], selector: null, guide: null, progress: null, user: event.locals.user };
	}

	const ctx = apiContext(event);

	// Country list (origins and destinations share the same canonical source),
	// the checklist selector, and a deep-linked guide are independent — one
	// wave. Progress still waits on the guide since it only exists when the
	// guide resolves for a signed-in user.
	const guideId = event.url.searchParams.get('guide_id');
	const [countries, selector, guide] = await Promise.all([
		loadCountries(ctx).catch(() => [] as GeoSpaceOption[]),
		apiGet<ChecklistSelector>(ctx, '/checklist/selector').catch(() => null),
		guideId
			? apiGetOrNull<ChecklistGuide>(ctx, `/checklist/${guideId}`).catch(() => null)
			: Promise.resolve(null)
	]);
	const origins = countries;
	const destinations = countries;

	let progress: ChecklistProgress | null = null;
	if (guide && event.locals.user && guideId) {
		try {
			progress = await apiGetOrNull<ChecklistProgress>(
				ctx,
				`/checklist/${guideId}/progress`
			);
		} catch {
			progress = null;
		}
	}

	return {
		enabled: true,
		tool,
		origins,
		destinations,
		selector,
		guide,
		progress,
		user: event.locals.user
	};
};
