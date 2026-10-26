// Ref: japaconnect-engineering/sections/07-organization-profiles-claim-ownership.md (Verification Requests)
// /verification-request — auth-gated. Three modes:
//   ?org={slug}  → org verification request form (owner/admin members only)
//   ?self=1      → profile verification request form
//   (none)       → "My verification requests" list (GET /me/verification-requests)
// The dynamic form comes from the Form Builder via GET
// /forms/verification-request/embed (system_settings 'verification.form_slug').
// The submission is created client-side via POST /forms/{id}/submissions
// (with payment_method + return_url for paid forms), then the request is
// created via POST /verification-requests with the form_submission_id
// linking the two. ?paid=1 marks a return from hosted checkout so the page
// can resume the request creation for the just-paid submission.
//
// Eligibility is pre-checked here so a user who is already verified, already
// has a pending request, or is not an org owner/admin sees a notice instead
// of a form that would charge a submission fee and then fail.
import { error, redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiGet, apiList } from '$lib/server/api';
import type {
	PublicOrganization,
	FormEmbed,
	MyVerificationRequest,
	MeProfile,
	ComposerOrganization
} from '$lib/types';

export type VerificationBlock =
	| 'already_verified'
	| 'pending_exists'
	| 'not_org_admin'
	| 'no_permission'
	| null;

export const load: PageServerLoad = async (event) => {
	if (!event.locals.user) {
		throw redirect(307, `/login?redirect=${encodeURIComponent(event.url.pathname + event.url.search)}`);
	}

	const ctx = apiContext(event);
	const slug = event.url.searchParams.get('org');
	const self = event.url.searchParams.get('self') === '1';
	const paid = event.url.searchParams.get('paid') === '1';

	// Requester-scoped list — fetched in every mode: it drives both the
	// bare-mode list view and the pending-request pre-check in form mode.
	const requestsPromise = apiGet<MyVerificationRequest[]>(ctx, '/me/verification-requests')
		.then((r) => r ?? [])
		.catch(() => [] as MyVerificationRequest[]);

	// Request form — same wave for both form modes. Absent → empty state.
	// The slug is admin-configurable (verification.form_slug); resolve it
	// from the public site settings instead of hardcoding (Rule 16).
	const formPromise =
		slug || self
			? apiGet<Record<string, unknown>>(ctx, '/site/settings/public')
					.then((s) => {
						const formSlug =
							typeof s?.['verification.form_slug'] === 'string' && s['verification.form_slug']
								? String(s['verification.form_slug'])
								: 'verification-request';
						return apiGet<FormEmbed>(ctx, `/forms/${formSlug}/embed`);
					})
					.catch(() => null)
			: Promise.resolve(null);
	// Badge-type vocabulary for the suggested-type select (Rule 16).
	const typesPromise =
		slug || self
			? apiGet<{ types: string[] }>(ctx, '/verification-requests/types')
					.then((r) => r?.types ?? [])
					.catch(() => [] as string[])
			: Promise.resolve([] as string[]);
	// Credit balance for the paid-form "pay with credits" option — lets the
	// UI disable it before submit rather than failing with
	// insufficient_credits after the user has filled the whole form.
	const creditsPromise =
		slug || self
			? apiGet<{ balance: number }>(ctx, '/me/wallet/credits')
					.then((r) => r?.balance ?? 0)
					.catch(() => 0)
			: Promise.resolve(0);

	if (!slug && !self) {
		return {
			org: null,
			self: false,
			form: null,
			types: [],
			credits: 0,
			blocked: null as VerificationBlock,
			resumePayment: false,
			requests: await requestsPromise
		};
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

	// Eligibility pre-checks (§07): resolve everything that POST
	// /verification-requests would reject BEFORE the user fills the form —
	// the submission fee is charged at submission time, so a late failure
	// would strand a paid-or-consumed submission.
	let blocked: VerificationBlock = null;
	const checks: Promise<void>[] = [];

	// RBAC pre-check: POST /verification-requests is gated by the
	// 'verification.request' node — when the session carries the resolved
	// permission list and the node is absent, the submit would 403.
	const perms = event.locals.user.permissions;
	if (perms && !perms.includes('verification.request')) blocked = 'no_permission';

	if (org) {
		if (org.verified) blocked = 'already_verified';
		// Requester must be the org owner or an owner/admin member —
		// member_role comes from GET /me/organizations (authoritative).
		checks.push(
			apiList<ComposerOrganization>(ctx, '/me/organizations?page_size=100')
				.then(({ items }) => {
					if (blocked) return;
					const mine = items.find((o) => o.id === org!.id);
					const isOwner = org!.owner?.id === event.locals.user!.id;
					if (!isOwner && mine?.member_role !== 'owner' && mine?.member_role !== 'admin') {
						blocked = 'not_org_admin';
					}
				})
				.catch(() => {})
		);
		// Org-scoped pending lookup (GET /organizations/{id}/verification-request)
		// catches a pending request filed by ANOTHER admin — the requester-scoped
		// list only sees the caller's own filings. 404 → no pending.
		checks.push(
			apiGet<MyVerificationRequest>(ctx, `/organizations/${org.id}/verification-request`)
				.then((r) => {
					if (r?.id && !blocked) blocked = 'pending_exists';
				})
				.catch(() => {})
		);
	} else {
		// Profile request: verified flag lives on /me (SessionUser lacks it).
		checks.push(
			apiGet<MeProfile>(ctx, '/me')
				.then((me) => {
					if (me?.verified) blocked = 'already_verified';
				})
				.catch(() => {})
		);
	}

	const [form, types, credits, requests] = await Promise.all([
		formPromise,
		typesPromise,
		creditsPromise,
		requestsPromise,
		...checks.map((p) => p.then(() => null))
	]);

	if (!blocked) {
		const pending = requests.find(
			(r) =>
				r.status === 'pending' &&
				(org
					? r.entity_type === 'organization' && r.organization_id === org.id
					: r.entity_type === 'user')
		);
		if (pending) blocked = 'pending_exists';
	}

	return { org, self, form, types, credits, blocked, resumePayment: paid, requests };
};
