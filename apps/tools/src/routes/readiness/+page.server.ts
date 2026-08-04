import type { PageServerLoad } from './$types';
import { apiContext, apiGet, apiGetOrNull } from '$lib/server/api';
import type { AssessmentForm, AssessmentResult } from '$lib/types';

/**
 * Japa Readiness Assessment — §71, ADR-071.
 * SSR loads the active assessment form + scoring metadata. If a result ID is
 * in the URL (shareable link), load the result directly.
 *
 * API: GET /assessment — active form + scoring metadata.
 *      GET /assessment/result/:id — result + roadmap.
 *      GET /assessment/history — user's past results (auth only).
 *
 * Ref: §71 (Readiness Assessment), ROADMAP-TOOLS 02.03.
 */
export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);

	let form: AssessmentForm | null = null;
	try {
		form = await apiGet<AssessmentForm>(ctx, '/assessment');
	} catch {
		form = null;
	}

	// If a result ID is in the URL, load it directly (shareable results page).
	const resultId = event.url.searchParams.get('result');
	let result: AssessmentResult | null = null;
	if (resultId) {
		try {
			result = await apiGetOrNull<AssessmentResult>(ctx, `/assessment/result/${resultId}`);
		} catch {
			result = null;
		}
	}

	return {
		form,
		result,
		user: event.locals.user
	};
};
