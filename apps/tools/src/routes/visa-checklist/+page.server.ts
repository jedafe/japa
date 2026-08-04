import type { PageServerLoad } from './$types';
import { apiContext, apiGet, apiGetOrNull } from '$lib/server/api';
import type { ChecklistSelector, ChecklistGuide, ChecklistProgress } from '$lib/types';

/**
 * Visa Document Checklist — §70, ADR-070.
 * SSR loads the selector options (countries + visa types). The checklist
 * itself is loaded after the user selects origin → destination → visa type.
 *
 * API: GET /checklist/selector — country + visa-type options from published
 * visa guides. GET /checklist/:guide_id — checklist items. GET /checklist/
 * :guide_id/progress — user's progress (auth only).
 *
 * Ref: §70 (Visa Document Checklist), ROADMAP-TOOLS 02.02.
 */
export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);

	let selector: ChecklistSelector | null = null;
	try {
		selector = await apiGet<ChecklistSelector>(ctx, '/checklist/selector');
	} catch {
		selector = null;
	}

	// If the URL has a pre-selected guide_id (shareable link), load it directly.
	const guideId = event.url.searchParams.get('guide_id');
	let guide: ChecklistGuide | null = null;
	let progress: ChecklistProgress | null = null;
	if (guideId) {
		try {
			guide = await apiGetOrNull<ChecklistGuide>(ctx, `/checklist/${guideId}`);
		} catch {
			guide = null;
		}
		if (guide && event.locals.user) {
			try {
				progress = await apiGetOrNull<ChecklistProgress>(
					ctx,
					`/checklist/${guideId}/progress`
				);
			} catch {
				progress = null;
			}
		}
	}

	return {
		selector,
		guide,
		progress,
		user: event.locals.user
	};
};
