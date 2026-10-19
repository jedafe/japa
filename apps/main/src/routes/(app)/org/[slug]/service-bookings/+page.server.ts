// Ref: japaconnect-engineering/sections/111-service-packages-bookings.md
import { error, redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiGet, apiList, ApiError } from '$lib/server/api';
import type { ServiceBooking, MyOrganization, PublicOrganization } from '$lib/types';

/**
 * Org bookings inbox at /org/[slug]/service-bookings (§111) — the selling
 * org's leads (intent=inquiry) and paid bookings (intent=booking).
 * Auth-gated to org members; the whole surface is behind the
 * service_packages.enabled flag like the backend routes it calls.
 */
export const load: PageServerLoad = async (event) => {
	const { features } = await event.parent();
	if (!features['service_packages.enabled']) {
		throw error(404, { code: 'feature_disabled', message: 'Service packages are not available' });
	}

	if (!event.locals.user) {
		throw redirect(
			307,
			`/login?redirect=${encodeURIComponent(event.url.pathname + event.url.search)}`
		);
	}

	const ctx = apiContext(event);
	const slug = event.params.slug;

	// service.booking.manage = org_member on the owning org (§111) — the
	// backend re-checks membership on every call, so this gate mirrors it.
	// Independent of the org lookup, so it rides the same wave.
	const myOrgsPromise = apiList<MyOrganization>(ctx, '/me/organizations')
		.then((r) => r.items)
		.catch(() => [] as MyOrganization[]);

	let org: PublicOrganization;
	try {
		org = await apiGet<PublicOrganization>(ctx, `/organizations/${slug}`);
	} catch (err) {
		if (err instanceof ApiError && err.status === 404) {
			throw error(404, { code: 'not_found', message: 'Organization not found' });
		}
		throw error(500, { code: 'load_failed', message: 'Failed to load organization' });
	}

	const myOrgs = await myOrgsPromise;
	const memberRole = myOrgs.find((o) => o.id === org.id)?.member_role ?? '';
	if (!memberRole) {
		throw error(403, {
			code: 'not_org_member',
			message: 'Only organization members can view its bookings inbox'
		});
	}

	const status = event.url.searchParams.get('status') ?? '';
	const intent = event.url.searchParams.get('intent') ?? '';
	const page = Number(event.url.searchParams.get('page') ?? '1') || 1;

	const params = new URLSearchParams();
	if (status) params.set('status', status);
	if (intent) params.set('intent', intent);
	params.set('page', String(page));
	params.set('limit', '20');

	let bookings: ServiceBooking[] = [];
	let meta: { total?: number; page?: number; limit?: number } | undefined;
	try {
		const res = await apiList<ServiceBooking>(
			ctx,
			`/org/${org.id}/service-bookings?${params.toString()}`
		);
		bookings = res.items;
		meta = res.meta;
	} catch (err) {
		if (err instanceof ApiError && (err.status === 403 || err.status === 404)) {
			throw error(403, {
				code: 'not_org_member',
				message: 'Only organization members can view its bookings inbox'
			});
		}
		bookings = [];
	}

	return { org, bookings, meta, status, intent, currentPage: page, memberRole, features };
};
