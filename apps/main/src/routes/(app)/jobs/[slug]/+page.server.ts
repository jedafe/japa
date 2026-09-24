import type { PageServerLoad, Actions } from './$types';
import { apiContext, apiGet, apiList, apiPost, ApiError } from '$lib/server/api';
import { error } from '@sveltejs/kit';
import type { ComposerOrganization, FormEmbed, JobDetail } from '$lib/types';

const JOB_EDITOR_ROLES = new Set(['owner', 'admin', 'editor']);

/**
 * Job detail at /jobs/[slug] (Section 63, ADR-081).
 * SSR-fetches the job detail and renders the apply form for authed users.
 *
 * Ref: japaconnect-engineering/sections/63-jobs-board.md
 */
export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);
	const slug = event.params.slug;
	const authed = !!event.locals.user;

	let job: JobDetail;
	try {
		job = await apiGet<JobDetail>(ctx, `/jobs/${slug}`);
	} catch (e) {
		if (e instanceof ApiError && e.status === 404) {
			throw error(404, { code: 'not_found', message: 'Job not found' });
		}
		if (e instanceof ApiError) {
			throw error(e.status >= 500 ? 500 : e.status, {
				code: e.code ?? 'load_failed',
				message: 'Failed to load job'
			});
		}
		throw error(500, { code: 'load_failed', message: 'Failed to load job' });
	}

	// Editor link: the user can edit when they hold an editor-or-above role in
	// the org that owns the listing (same roles the PATCH endpoint enforces).
	let canEdit = false;
	if (authed && job.organization_id) {
		try {
			const orgs = await apiList<ComposerOrganization>(ctx, '/me/organizations');
			canEdit = orgs.items.some(
				(o) => o.id === job.organization_id && JOB_EDITOR_ROLES.has(o.member_role)
			);
		} catch {
			canEdit = false;
		}
	}

	// The on-platform apply flow links a form_submissions row (§06) to the
	// job_applications record — resolve the system job-application form so the
	// client can attach CV/cover-letter field_values (JOBS apply flow).
	let applicationFormId: string | null = null;
	if (authed && !job.field_values?.application_url && !job.my_application) {
		const form = await apiGet<FormEmbed>(ctx, '/forms/job_application_form/embed').catch(() => null);
		applicationFormId = form?.id ?? null;
	}

	return { job, authed, canEdit, applicationFormId };
};

export const actions: Actions = {
	apply: async (event) => {
		if (!event.locals.user) {
			return { error: 'auth_required' };
		}

		const ctx = apiContext(event);
		const form = await event.request.formData();
		const jobId = (form.get('job_id') as string | null)?.trim() ?? '';
		const submissionId = (form.get('submission_id') as string | null)?.trim() ?? '';

		if (!jobId) {
			return { error: 'missing_job_id' };
		}

		try {
			const result = await apiPost<{ id: string; status: string }>(
				ctx,
				`/jobs/${jobId}/apply`,
				submissionId ? { submission_id: submissionId } : {}
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
