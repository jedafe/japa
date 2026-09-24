import type { PageServerLoad, Actions } from './$types';
import { apiContext, apiGet, apiList, apiPatch, ApiError } from '$lib/server/api';
import { error, fail, redirect } from '@sveltejs/kit';
import type { ComposerOrganization, JobApplication, JobListing, PublicOrganization } from '$lib/types';

// Application management is org-admin only — mirrors the JOBS-012 check in
// the backend (requireOrgJobAdmin: owner/admin, job must belong to the org).
const JOB_ADMIN_ROLES = new Set(['owner', 'admin']);
const PAGE_SIZE = 20;
const VALID_STATUSES = new Set([
	'applied',
	'reviewing',
	'shortlisted',
	'rejected',
	'offered',
	'withdrawn'
]);

/**
 * Org job applications at /org/[slug]/jobs/[jobId] (Section 63, ADR-081).
 * Employer-facing view consuming GET/PATCH
 * /api/v1/org/{org_id}/jobs/{id}/applications[/{app_id}].
 *
 * Ref: japaconnect-engineering/sections/63-jobs-board.md
 */
export const load: PageServerLoad = async (event) => {
	if (!event.locals.user) {
		throw redirect(307, `/login?redirect=${encodeURIComponent(event.url.pathname)}`);
	}
	const ctx = apiContext(event);
	const slug = event.params.slug;
	const jobId = event.params.jobId;

	let org: PublicOrganization;
	try {
		org = await apiGet<PublicOrganization>(ctx, `/organizations/${slug}`);
	} catch (e) {
		const status = e instanceof ApiError ? e.status : 500;
		throw error(status === 404 ? 404 : 500, {
			code: status === 404 ? 'not_found' : 'server_error',
			message: status === 404 ? 'Organization not found' : 'Failed to load organization'
		});
	}

	const orgs = await apiList<ComposerOrganization>(ctx, '/me/organizations')
		.then((r) => r.items)
		.catch(() => []);
	const membership = orgs.find((o) => o.id === org.id);
	if (!membership || !JOB_ADMIN_ROLES.has(membership.member_role)) {
		throw error(403, {
			code: 'not_org_admin',
			message: 'Only organization admins can manage job applications'
		});
	}

	const page = event.url.searchParams.get('page') ?? '1';
	const status = event.url.searchParams.get('status') ?? '';
	const params = new URLSearchParams({ page, limit: String(PAGE_SIZE) });
	if (status) params.set('status', status);

	let applications: JobApplication[] = [];
	let meta: { total?: number; page?: number; limit?: number; total_pages?: number } | undefined;
	try {
		const res = await apiList<JobApplication>(
			ctx,
			`/org/${org.id}/jobs/${jobId}/applications?${params.toString()}`
		);
		applications = res.items;
		meta = res.meta;
	} catch (e) {
		if (e instanceof ApiError) {
			throw error(e.status === 404 ? 404 : e.status >= 500 ? 500 : e.status, {
				code: e.code,
				message: e.message
			});
		}
		throw error(500, { code: 'applications_list_failed', message: 'Failed to load applications' });
	}

	// The applications payload does not carry the job title — resolve it from
	// the org's listings (JOBS-012 already verified the job belongs here).
	const job = await apiList<JobListing>(ctx, `/org/${org.id}/jobs?limit=50`)
		.then((r) => r.items.find((j) => j.id === jobId) ?? null)
		.catch(() => null);

	return { org, job, jobId, applications, meta, statusFilter: status };
};

export const actions: Actions = {
	updateStatus: async (event) => {
		if (!event.locals.user) {
			throw redirect(307, `/login?redirect=${encodeURIComponent(event.url.pathname)}`);
		}
		const ctx = apiContext(event);
		const slug = event.params.slug;
		const jobId = event.params.jobId;

		const form = await event.request.formData();
		const appId = String(form.get('app_id') ?? '');
		const status = String(form.get('status') ?? '');
		const employerNotes = String(form.get('employer_notes') ?? '');

		if (!appId || !VALID_STATUSES.has(status)) {
			return fail(400, { error: 'invalid_status' });
		}

		let org: PublicOrganization;
		try {
			org = await apiGet<PublicOrganization>(ctx, `/organizations/${slug}`);
		} catch {
			return fail(500, { error: 'org_lookup_failed' });
		}

		try {
			await apiPatch(ctx, `/org/${org.id}/jobs/${jobId}/applications/${appId}`, {
				status,
				employer_notes: employerNotes || null
			});
			return { ok: true };
		} catch (e) {
			if (e instanceof ApiError) {
				return fail(e.status >= 500 ? 500 : e.status, { error: e.code });
			}
			return fail(500, { error: 'update_failed' });
		}
	}
};
