// Ref: japaconnect-engineering/sections/07-organization-profiles-claim-ownership.md (Verification Requests)
// /verification-request — auth-gated. Three modes:
//   ?org={slug}  → org verification request form (owner/admin members only)
//   ?self=1      → profile verification request form
//   (none)       → "My verification requests" list (GET /me/verification-requests)
// The dynamic form comes from the Form Builder via GET
// /forms/verification-request/embed (system_settings 'verification.form_slug').
// The submission is created client-side via POST /forms/{id}/submissions
// (with payment_method for paid forms), then the request is created via
// POST /verification-requests with the form_submission_id linking the two.
import { error, redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiGet } from '$lib/server/api';
import type { PublicOrganization, FormEmbed, MyVerificationRequest } from '$lib/types';

export const load: PageServerLoad = async (event) => {
	if (!event.locals.user) {
		throw redirect(307, `/login?redirect=${encodeURIComponent(event.url.pathname + event.url.search)}`);
	}

	const ctx = apiContext(event);
	const slug = event.url.searchParams.get('org');
	const self = event.url.searchParams.get('self') === '1';

	// Request form — same wave for both form modes. Absent → empty state.
	const formPromise = (slug || self)
		? apiGet<FormEmbed>(ctx, '/forms/verification-request/embed').catch(() => null)
		: Promise.resolve(null);
	// Badge-type vocabulary for the suggested-type select (Rule 16).
	const typesPromise = apiGet<{ types: string[] }>(ctx, '/verification-requests/types')
		.then((r) => r?.types ?? [])
		.catch(() => [] as string[]);

	if (!slug && !self) {
		let requests: MyVerificationRequest[] = [];
		try {
			requests = (await apiGet<MyVerificationRequest[]>(ctx, '/me/verification-requests')) ?? [];
		} catch {
			requests = [];
		}
		return { org: null, self: false, form: null, types: [], requests };
	}

	let org: PublicOrganization | null = null;
	if (slug) {
		try {
			org = await apiGet<PublicOrganization>(ctx, `/organizations/${slug}`);
		} catch (e) {
			const status = e instanceof Error && 'status' in e ? (e as { status: number }).status : 500;
			throw error(status === 404 ? 404 : 500, {
				code: status === 404 ? 'not_found' : 'server_error',
				message: status === 404 ? 'Organization not found' : 'Failed to load organization'
			});
		}
	}

	const [form, types] = await Promise.all([formPromise, typesPromise]);

	return { org, self, form, types, requests: [] };
};
