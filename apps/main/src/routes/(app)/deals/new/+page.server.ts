// Ref: japaconnect-engineering/sections/55-coupons-external-affiliates.md
import type { PageServerLoad } from './$types';
import { redirect } from '@sveltejs/kit';
import { apiContext, apiList } from '$lib/server/api';
import { hasPerm } from '$lib/server/auth';
import type { DealPartnerRef, MyOrganization } from '$lib/types';

/**
 * /deals/new — auth-gated deal creation with two audiences:
 * - Staff (coupons.manage RBAC node): full form incl. provider select and
 *   moderation controls; saves via POST /admin/coupons.
 * - Everyone else: must hold an owner/admin/editor role in at least one
 *   organization (GET /me/organizations); the org form submits to
 *   POST /organizations/{id}/deals where the server forces status=pending.
 */
export const load: PageServerLoad = async (event) => {
	if (!event.locals.user) {
		throw redirect(307, `/login?redirect=${encodeURIComponent(event.url.pathname)}`);
	}
	const perms = event.locals.user.permissions ?? [];
	const isStaff = hasPerm(perms, 'coupons.manage');
	const ctx = apiContext(event);

	// Provider select options (staff form only — org submissions derive the
	// provider server-side from the org, so non-staff never needs this fetch).
	// The endpoint returns the full active list under `data`; apiList reads it
	// regardless of pagination.
	let providers: DealPartnerRef[] = [];
	if (isStaff) {
		try {
			providers = (await apiList<DealPartnerRef>(ctx, '/deals/partners?page_size=100')).items;
		} catch {
			providers = [];
		}
	}

	// Org submission eligibility mirrors the /deals index canSubmit check —
	// only owner/admin/editor members may submit a deal on an org's behalf.
	let orgs: MyOrganization[] = [];
	if (!isStaff) {
		try {
			orgs = (await apiList<MyOrganization>(ctx, '/me/organizations?page_size=50')).items.filter(
				(o) => ['owner', 'admin', 'editor'].includes(o.member_role)
			);
		} catch {
			orgs = [];
		}
	}

	return {
		isStaff,
		providers,
		orgs,
		canManageProviders: hasPerm(perms, 'affiliate.providers.manage')
	};
};
