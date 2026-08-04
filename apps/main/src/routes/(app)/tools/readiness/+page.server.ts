import type { PageServerLoad } from './$types';
import { apiContext, apiGet } from '$lib/server/api';
import { ApiError } from '$lib/server/api';
import type { AssessmentForm } from '$lib/types';

/**
 * Readiness Assessment page at /tools/readiness (Section 71, ADR-071).
 * SSR-fetches the active assessment form so the multi-section form renders
 * without a client round-trip. Auth-gated routes (history) are fetched
 * client-side after hydration.
 *
 * Ref: japaconnect-engineering/sections/71-readiness-assessment.md
 */
export const load: PageServerLoad = async (event) => {
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

	return { form, disabled, authed };
};
