// Ref: japaconnect-engineering/sections/98-service-directory.md
import { error, redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiGet, apiList } from '$lib/server/api';
import type { GeoCountry, ServiceCategoryInfo } from '$lib/types';

/**
 * Create-service form at /services/new — auth-gated. Guests are
 * redirected to login with a return redirect to this page. The form
 * itself is submitted client-side via the browser API client
 * (POST /services), which is org-linked on the backend.
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

	let countries: GeoCountry[] = [];
	try {
		const { items } = await apiList<GeoCountry>(ctx, '/geo/countries');
		countries = items;
	} catch {
		countries = [];
	}

	// §114: keep the taxonomy label — the category dropdown renders the
	// API label (dynamic slugs have no i18n cat_* key to re-derive it).
	let serviceCategories: ServiceCategoryInfo[] = [];
	try {
		const cat = await apiGet<ServiceCategoryInfo[]>(ctx, '/services/categories');
		serviceCategories = cat ?? [];
	} catch {
		serviceCategories = [];
	}

	// §114: the booking/pricing section is gated on the
	// packages flag (it now means "bookings & pricing" —
	// inquiry/external modes and tiers for every kind).
	const packagesEnabled = !!features['service_packages.enabled'];

	return { authed: true, countries, serviceCategories, packagesEnabled };
};
