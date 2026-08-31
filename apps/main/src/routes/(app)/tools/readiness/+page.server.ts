import { redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiGet, apiList } from '$lib/server/api';
import { ApiError } from '$lib/server/api';
import type { AssessmentForm } from '$lib/types';

/**
 * Readiness Assessment page at /tools/readiness (Section 71, ADR-071).
 * SSR-fetches the active assessment form so the multi-section form renders
 * without a client round-trip. For authenticated users, also fetches score
 * history (/assessment/history) so the page can render the score history
 * LineChart (Phase 33.02).
 *
 * Ref: japaconnect-engineering/sections/71-readiness-assessment.md
 */
export interface AssessmentHistoryEntry {
	id: string;
	total_score: number;
	threshold_label?: string;
	submission_id?: string;
	created_at: string;
}

export const load: PageServerLoad = async (event) => {
	const { features } = await event.parent();
	if (!features['assessment.enabled']) {
		// ADR-042: disabled features redirect to / (not 403, not an error page).
		throw redirect(302, '/');
	}

	const ctx = apiContext(event);
	const authed = !!event.locals.user;

	let form: AssessmentForm | null = null;
	let disabled = false;
	try {
		form = await apiGet<AssessmentForm>(ctx, '/assessment');
	} catch (e) {
		if (e instanceof ApiError && (e.code === 'assessment_disabled' || e.code === 'no_active_assessment' || e.status === 404)) {
			disabled = true;
		}
	}

	// Phase 33.02 — fetch the authenticated user's score history so the page
	// can render the score history LineChart. Best-effort: guests and any
	// backend error degrade to an empty list.
	let history: AssessmentHistoryEntry[] = [];
	if (authed) {
		try {
			history = (await apiList<AssessmentHistoryEntry>(ctx, '/assessment/history?limit=50')).items;
		} catch {
			history = [];
		}
	}

	return { form, disabled, authed, history };
};
