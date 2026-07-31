import { error, fail, redirect } from '@sveltejs/kit';
import type { PageServerLoad, Actions } from './$types';
import { apiContext, apiGet, apiPost, ApiError } from '$lib/server/api';
import type { PublicOrganization } from '$lib/types';

/**
 * Claim-ownership form at /claim-ownership?org={slug} — auth-gated. Resolves
 * the org by slug (so the user lands on a pre-filled form), then POSTs
 * {organization_id, evidence_url, notes} to /claim-ownership.
 *
 * Ref: japaconnect-engineering/sections/07-organization-profiles-claim-ownership.md
 */
export const load: PageServerLoad = async (event) => {
	if (!event.locals.user) {
		throw error(403, { code: 'forbidden', message: 'Authentication required' });
	}

	const slug = event.url.searchParams.get('org');
	if (!slug) {
		throw error(400, { code: 'bad_request', message: 'Missing organization slug' });
	}

	const ctx = apiContext(event);
	let org: PublicOrganization;
	try {
		org = await apiGet<PublicOrganization>(ctx, `/organizations/${slug}`);
	} catch (e) {
		const status = e instanceof Error && 'status' in e ? (e as { status: number }).status : 500;
		throw error(status === 404 ? 404 : 500, {
			code: status === 404 ? 'not_found' : 'server_error',
			message: status === 404 ? 'Organization not found' : 'Failed to load organization'
		});
	}

	return { org };
};

export const actions: Actions = {
	default: async (event) => {
		if (!event.locals.user) return fail(403, { message: 'Auth required' });

		const ctx = apiContext(event);
		const form = await event.request.formData();

		const organizationId = String(form.get('organization_id') ?? '').trim();
		const evidenceUrl = String(form.get('evidence_url') ?? '').trim();
		const notes = String(form.get('notes') ?? '').trim();

		if (!organizationId) {
			return fail(400, { message: 'Organization ID is required' });
		}

		const payload: Record<string, unknown> = { organization_id: organizationId };
		if (evidenceUrl) payload.evidence_url = evidenceUrl;
		if (notes) payload.notes = notes;

		try {
			await apiPost(ctx, '/claim-ownership', payload);
			throw redirect(303, `/org/${String(form.get('org_slug') ?? '')}`);
		} catch (e) {
			if (e instanceof ApiError) {
				return fail(e.status >= 400 && e.status < 500 ? e.status : 500, {
					message: e.message,
					organization_id: organizationId,
					evidence_url: evidenceUrl,
					notes
				});
			}
			throw e;
		}
	}
};
