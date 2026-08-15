import type { PageServerLoad } from './$types';
import { apiContext, apiGet, apiGetOrNull, apiList } from '$lib/server/api';
import { getTool, isToolEnabled } from '$lib/tools/registry';
import type { AssessmentForm, AssessmentResult, AssessmentHistoryItem } from '$lib/types';

/**
 * Japa Readiness Assessment — §71, ADR-071.
 * SSR loads the active assessment form + scoring metadata. If a result ID is
 * in the URL (shareable link), load the full result directly. For authenticated
 * users, also load their past results history (§71: "authenticated users get
 * persistent history"). History items are summaries; the full result is
 * fetched client-side when a user opens one.
 *
 * API: GET /assessment — active form + scoring metadata.
 *      GET /assessment/result/:id — full result + roadmap.
 *      GET /assessment/history — user's past result summaries (auth only).
 *
 * Ref: §71 (Readiness Assessment), ROADMAP-TOOLS 02.03.
 */
export const load: PageServerLoad = async (event) => {
	// Gate: skip all backend calls when assessment.enabled is false.
	// Ref: conventions/system-settings-catalog.md, Rule 3.
	const tool = getTool('readiness')!;
	if (!isToolEnabled(tool, event.locals.features)) {
		return { enabled: false, tool, form: null, result: null, history: [], user: event.locals.user };
	}

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

	// Load the authenticated user's past results (§71 history).
	// Ref: §71 — "authenticated users get persistent history".
	let history: AssessmentHistoryItem[] = [];
	if (event.locals.user) {
		try {
			const res = await apiList<AssessmentHistoryItem>(ctx, '/assessment/history');
			history = res.items;
		} catch {
			history = [];
		}
	}

	return {
		enabled: true,
		tool,
		form,
		result,
		history,
		user: event.locals.user
	};
};
