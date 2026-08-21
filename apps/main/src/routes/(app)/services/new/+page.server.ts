// Ref: japaconnect-engineering/sections/98-service-directory.md
import { redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiGet, apiList } from '$lib/server/api';
import type { GeoCountry, ServiceCategory } from '$lib/types';

/**
 * Create-service form at /services/new — auth-gated. Guests are
 * redirected to login with a return redirect to this page. The form
 * itself is submitted client-side via the browser API client
 * (POST /services), which is org-linked on the backend.
 */
export const load: PageServerLoad = async (event) => {
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

	let serviceCategories: ServiceCategory[] = [];
	try {
		const cat = await apiGet<{ value: string; label: string }[]>(ctx, '/services/categories');
		serviceCategories = (cat ?? []).map((o) => o.value as ServiceCategory);
	} catch {
		serviceCategories = [];
	}

	return { authed: true, countries, serviceCategories };
};
