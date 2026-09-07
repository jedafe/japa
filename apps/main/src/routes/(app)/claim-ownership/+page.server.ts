// Ref: japaconnect-engineering/sections/07-organization-profiles-claim-ownership.md
// Claim-ownership form at /claim-ownership?org={slug} — auth-gated. Loads the
// org by slug AND the claim-ownership form from the Form Builder (Section 06)
// via GET /forms/claim-ownership/embed. The form fields are rendered
// dynamically (Rule 16 — no hardcoded fields). The form submission is created
// client-side via POST /forms/{id}/submissions (with payment_method for paid
// forms), then the claim is created via POST /claim-ownership with the
// form_submission_id linking the two.
import { error, redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiGet } from '$lib/server/api';
import type { PublicOrganization, FormEmbed, MyOwnershipClaim } from '$lib/types';

export const load: PageServerLoad = async (event) => {
	if (!event.locals.user) {
		throw redirect(307, `/login?redirect=${encodeURIComponent(event.url.pathname + event.url.search)}`);
	}

	const slug = event.url.searchParams.get('org');
	const ctx = apiContext(event);

	// Without ?org=, the page is the claimant's "My claims" surface —
	// GET /me/claim-ownership is the only way a claimant can discover their
	// claim IDs for GET/DELETE /claim-ownership/{id} (Section 07).
	if (!slug) {
		let claims: MyOwnershipClaim[] = [];
		try {
			claims = (await apiGet<MyOwnershipClaim[]>(ctx, '/me/claim-ownership')) ?? [];
		} catch {
			claims = [];
		}
		return { org: null, form: null, claims };
	}

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

	// Load the claim-ownership form from the Form Builder (Section 06).
	// The form slug is configurable via system_settings
	// 'claim_ownership.form_slug' (default: 'claim-ownership').
	let form: FormEmbed | null = null;
	try {
		form = await apiGet<FormEmbed>(ctx, '/forms/claim-ownership/embed');
	} catch {
		form = null;
	}

	return { org, form, claims: [] };
};
