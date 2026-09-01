import { redirect, error } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiGet, ApiError } from '$lib/server/api';
import type { AssessmentResult } from '$lib/types';

/**
 * /tools/readiness/:slug — result permalink (§71, ADR-071).
 *
 * The assessment results are shareable by result id. This route validates the
 * id exists and then redirects to the canonical /tools/readiness?result=<id>
 * page, which already handles result loading and display. This keeps the
 * result UI in one place while giving users a clean, shareable URL.
 */
export const load: PageServerLoad = async (event) => {
	const slug = event.params.slug;
	if (!slug || !/^[0-9a-fA-F-]{36}$/.test(slug)) {
		throw error(404, { code: 'result_not_found', message: 'Result not found' });
	}

	const ctx = apiContext(event);
	try {
		await apiGet<AssessmentResult>(ctx, `/assessment/result/${slug}`);
	} catch (e) {
		if (e instanceof ApiError) {
			if (e.status === 401) {
				throw redirect(302, `/login?next=${encodeURIComponent(`/tools/readiness?result=${encodeURIComponent(slug)}`)}`);
			}
			if (e.status === 403) {
				throw error(403, { code: 'result_forbidden', message: e.message ?? 'You do not have access to this result' });
			}
		}
		throw error(404, { code: 'result_not_found', message: 'Result not found' });
	}

	throw redirect(302, `/tools/readiness?result=${encodeURIComponent(slug)}`);
};
