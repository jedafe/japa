import type { PageServerLoad, Actions } from './$types';
import { apiContext, apiGet, apiPost, ApiError } from '$lib/server/api';
import { error } from '@sveltejs/kit';
import type { JobDetail } from '$lib/types';

/**
 * Job detail at /jobs/[slug] (Section 63, ADR-079).
 * SSR-fetches the job detail and renders the apply form for authed users.
 *
 * Ref: japaconnect-engineering/sections/63-jobs-board.md
 */
export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);
	const slug = event.params.slug;
	const authed = !!event.locals.user;

	let job: JobDetail | null = null;
	try {
		job = await apiGet<JobDetail>(ctx, `/jobs/${slug}`);
	} catch (e) {
		if (e instanceof ApiError && e.status === 404) {
			throw error(404, { code: 'not_found', message: 'Job not found' });
		}
	}

	return { job, authed };
};

export const actions: Actions = {
	apply: async (event) => {
		if (!event.locals.user) {
			return { error: 'auth_required' };
		}

		const ctx = apiContext(event);
		const form = await event.request.formData();
		const jobId = (form.get('job_id') as string | null)?.trim() ?? '';

		if (!jobId) {
			return { error: 'missing_job_id' };
		}

		try {
			const result = await apiPost<{ id: string; status: string }>(
				ctx,
				`/jobs/${jobId}/apply`,
				{}
			);
			return { ok: true, applicationId: result.id, status: result.status };
		} catch (err) {
			if (err instanceof ApiError) {
				return { error: err.code, message: err.message };
			}
			return { error: 'apply_failed' };
		}
	}
};
