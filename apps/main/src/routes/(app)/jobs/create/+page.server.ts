import { redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiList } from '$lib/server/api';
import type { CustomFieldGroup, ComposerOrganization, JobListing } from '$lib/types';

const JOB_EDITOR_ROLES = new Set(['owner', 'admin', 'editor']);

export const load: PageServerLoad = async (event) => {
	// Job creation is auth-gated; the POST endpoint also enforces org
	// membership with an editor-or-above role (JOBS-012).
	if (!event.locals.user) {
		throw redirect(307, `/login?redirect=${encodeURIComponent('/jobs/create')}`);
	}

	const ctx = apiContext(event);

	// Probe the jobs list endpoint; if the board is disabled it returns 404
	// with code `jobs_disabled` (Section 63, ADR-081).
	let disabled = false;
	try {
		await apiList<JobListing>(ctx, '/jobs?limit=1');
	} catch (e: unknown) {
		const code = (e as { code?: string }).code;
		const status = (e as { status?: number }).status;
		if (code === 'jobs_disabled' || status === 404) {
			disabled = true;
		}
	}

	const [groupsRes, orgsRes] = await Promise.allSettled([
		apiList<CustomFieldGroup>(ctx, '/custom-field-groups?entity_type=content'),
		apiList<ComposerOrganization>(ctx, '/me/organizations')
	]);

	const allGroups = groupsRes.status === 'fulfilled' ? groupsRes.value.items : [];
	const fieldGroup = allGroups.find((g) => g.slug === 'job_listing_fields') ?? null;

	const organizations =
		orgsRes.status === 'fulfilled'
			? orgsRes.value.items.filter((o) => JOB_EDITOR_ROLES.has(o.member_role))
			: [];

	return { fieldGroup, organizations, disabled };
};
