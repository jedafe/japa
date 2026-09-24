import { error, redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiGet, apiList, ApiError } from '$lib/server/api';
import type { CustomFieldGroup, ComposerOrganization, JobDetail, JobListing } from '$lib/types';

const JOB_EDITOR_ROLES = new Set(['owner', 'admin', 'editor']);

/**
 * Edit-job form at /jobs/[slug]/edit — auth-gated, organization editor.
 * Loads the listing by slug, the user's editable orgs, and the custom
 * field group so the existing field_values can be pre-filled.
 */
export const load: PageServerLoad = async (event) => {
	if (!event.locals.user) {
		throw redirect(307, `/login?redirect=${encodeURIComponent(event.url.pathname + event.url.search)}`);
	}

	const { features } = await event.parent();
	const ctx = apiContext(event);

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

	const slug = event.params.slug;

	let job: JobDetail;
	try {
		job = await apiGet<JobDetail>(ctx, `/jobs/${slug}?raw=1`);
	} catch (err) {
		if (err instanceof ApiError && err.status === 404) {
			throw error(404, { code: 'not_found', message: 'Job not found' });
		}
		throw error(500, { code: 'load_failed', message: 'Failed to load job' });
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

	const canEdit =
		job.org_id && organizations.some((o) => o.id === job.org_id);
	if (!canEdit) {
		throw error(403, { code: 'not_owner', message: 'You can only edit jobs for organizations you help manage' });
	}

	return { job, organizations, fieldGroup, disabled, user: event.locals.user, features };
};
