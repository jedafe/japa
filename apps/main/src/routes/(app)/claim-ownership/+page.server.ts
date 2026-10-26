// Ref: japaconnect-engineering/sections/07-organization-profiles-claim-ownership.md
// Claim-ownership form at /claim-ownership?org={slug} — auth-gated. Loads the
// org by slug AND the claim-ownership form from the Form Builder (Section 06)
// via GET /forms/claim-ownership/embed. The form fields are rendered
// dynamically (Rule 16 — no hardcoded fields). The form submission is created
// client-side via POST /forms/{id}/submissions (with payment_method +
// return_url + link intent for paid forms), then the claim is created via
// POST /claim-ownership with the form_submission_id linking the two.
// ?paid=1 marks a return from hosted checkout so the page can resume the
// claim creation for the just-paid submission (the webhook-side link intent
// also auto-creates it — whichever lands first wins).
// A pending claim for this org is pre-checked so the user never pays a
// submission fee for a claim that would be rejected as a duplicate.
import { error, redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiGet } from '$lib/server/api';
import type { PublicOrganization, FormEmbed, MyOwnershipClaim } from '$lib/types';

export const load: PageServerLoad = async (event) => {
	if (!event.locals.user) {
		throw redirect(307, `/login?redirect=${encodeURIComponent(event.url.pathname + event.url.search)}`);
	}

	const slug = event.url.searchParams.get('org');
	const paid = event.url.searchParams.get('paid') === '1';
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
		return { org: null, form: null, claims, credits: 0, blocked: null, resumePayment: false };
	}

	// Load the claim-ownership form from the Form Builder (Section 06) in the
	// same wave as the org — the form slug is configurable via system_settings
	// 'claim_ownership.form_slug' (default: 'claim-ownership').
	// The slug is admin-configurable (claim_ownership.form_slug); resolve it
	// from the public site settings instead of hardcoding (Rule 16).
	const formPromise = apiGet<Record<string, unknown>>(ctx, '/site/settings/public')
		.then((s) => {
			const formSlug =
				typeof s?.['claim_ownership.form_slug'] === 'string' && s['claim_ownership.form_slug']
					? String(s['claim_ownership.form_slug'])
					: 'claim-ownership';
			return apiGet<FormEmbed>(ctx, `/forms/${formSlug}/embed`);
		})
		.catch(() => null);

	// BUG-010 pre-check: the claimant's own claims — a pending claim for this
	// org blocks the form before any submission fee is charged.
	const claimsPromise = apiGet<MyOwnershipClaim[]>(ctx, '/me/claim-ownership')
		.then((r) => r ?? [])
		.catch(() => [] as MyOwnershipClaim[]);

	// Credit balance for the paid-form "pay with credits" option.
	const creditsPromise = apiGet<{ balance: number }>(ctx, '/me/wallet/credits')
		.then((r) => r?.balance ?? 0)
		.catch(() => 0);

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

	const [form, claims, credits] = await Promise.all([formPromise, claimsPromise, creditsPromise]);

	// POST /claim-ownership is gated by the 'organizations.claim' node, and a
	// pending claim for this org is rejected as a duplicate — block before
	// any submission fee can be charged.
	let blocked: 'pending_exists' | 'no_permission' | null = null;
	const perms = event.locals.user.permissions;
	if (perms && !perms.includes('organizations.claim')) {
		blocked = 'no_permission';
	} else if (claims.some((c) => c.status === 'pending' && c.organization_id === org.id)) {
		blocked = 'pending_exists';
	}

	return { org, form, claims, credits, blocked, resumePayment: paid };
};
