// Ref: japaconnect-engineering/sections/98-service-directory.md
// Ref: japaconnect-engineering/sections/111-service-packages-bookings.md
import { error, redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiGet, apiList, ApiError } from '$lib/server/api';
import type {
	ServiceListing,
	GeoCountry,
	ServiceCategory,
	PackageDeparture,
	MyOrganization
} from '$lib/types';

/**
 * Edit-service form at /services/[slug]/edit — auth-gated, author-or-edit_any.
 * Loads the listing by slug and the country/category lists for the form.
 */
export const load: PageServerLoad = async (event) => {
	// Feature gate before the auth redirect — a disabled feature must 404
	// for guests too, matching /services and the backend route Gate.
	const { features } = await event.parent();
	if (!features['service_directory.enabled']) {
		throw error(404, { code: 'feature_disabled', message: 'Service directory is not available' });
	}

	if (!event.locals.user) {
		throw redirect(307, `/login?redirect=${encodeURIComponent(event.url.pathname + event.url.search)}`);
	}

	const ctx = apiContext(event);
	const slug = event.params.slug;

	let service: ServiceListing;
	try {
		service = await apiGet<ServiceListing>(ctx, `/services/${slug}?raw=1`);
	} catch (err) {
		if (err instanceof ApiError && err.status === 404) {
			throw error(404, { code: 'not_found', message: 'Service not found' });
		}
		throw error(500, { code: 'load_failed', message: 'Failed to load service' });
	}

	// Mirrors EditOwn (§98/§111): author, owning-org member, or
	// service.edit_any may edit; packages additionally require
	// service.package.create. Org membership resolves via /me/organizations.
	const isEditAny = !!event.locals.user.permissions?.includes('service.edit_any');
	const isPackage = service.listing_kind === 'package';

	let isOrgMember = false;
	let myOrgs: MyOrganization[] = [];
	if (!isEditAny && (service.author_id !== event.locals.user.id || isPackage)) {
		try {
			myOrgs = await apiList<MyOrganization>(ctx, '/me/organizations').then((r) => r.items);
		} catch {
			myOrgs = [];
		}
		isOrgMember = !!service.org_id && myOrgs.some((o) => o.id === service.org_id);
	}

	const canEdit = isEditAny || service.author_id === event.locals.user.id || isOrgMember;
	if (!canEdit) {
		throw error(403, { code: 'not_owner', message: 'You can only edit your own service listings' });
	}
	if (
		isPackage &&
		!isEditAny &&
		!event.locals.user.permissions?.includes('service.package.create')
	) {
		throw error(403, {
			code: 'package_create_required',
			message: 'You do not have permission to edit packages'
		});
	}

	// §111: the public detail only embeds open+future departures — the
	// editor reads the unmanaged set (past/full/cancelled included) from
	// the owner-gated departures route so booked rows can't be silently
	// dropped and cancelled stays editable.
	let departures: PackageDeparture[] = service.departures ?? [];
	if (isPackage) {
		try {
			departures =
				(await apiGet<PackageDeparture[]>(ctx, `/services/${service.id}/departures`)) ?? [];
		} catch {
			// Fail closed — falling back to the detail DTO's open+future subset
			// would silently drop unbooked cancelled/past departures on save.
			throw error(500, { code: 'load_failed', message: 'Failed to load departures' });
		}
	}

	const [countriesRes, categoriesRes] = await Promise.allSettled([
		apiList<GeoCountry>(ctx, '/geo/countries'),
		apiGet<{ value: string; label: string }[]>(ctx, '/services/categories')
	]);

	const countries = countriesRes.status === 'fulfilled' ? countriesRes.value.items : [];
	const serviceCategories = (
		categoriesRes.status === 'fulfilled' ? categoriesRes.value ?? [] : []
	).map((o) => o.value as ServiceCategory);

	return {
		service,
		departures,
		countries,
		serviceCategories,
		user: event.locals.user,
		features
	};
};
