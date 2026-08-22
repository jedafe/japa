// Ref: japaconnect-engineering/sections/98-service-directory.md
import { error, redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiGet, apiList, ApiError } from '$lib/server/api';
import type { ServiceListing, GeoCountry, ServiceCategory } from '$lib/types';

/**
 * Edit-service form at /services/[slug]/edit — auth-gated, author-or-edit_any.
 * Loads the listing by slug and the country/category lists for the form.
 */
export const load: PageServerLoad = async (event) => {
	if (!event.locals.user) {
		throw redirect(307, `/login?redirect=${encodeURIComponent(event.url.pathname + event.url.search)}`);
	}

	const { features } = await event.parent();
	if (!features['service_directory.enabled']) {
		throw error(404, { code: 'feature_disabled', message: 'Service directory is not available' });
	}

	const ctx = apiContext(event);
	const slug = event.params.slug;

	let service: ServiceListing;
	try {
		service = await apiGet<ServiceListing>(ctx, `/services/${slug}`);
	} catch (err) {
		if (err instanceof ApiError && err.status === 404) {
			throw error(404, { code: 'not_found', message: 'Service not found' });
		}
		throw error(500, { code: 'load_failed', message: 'Failed to load service' });
	}

	const canEdit =
		service.author_id === event.locals.user.id ||
		event.locals.user.permissions?.includes('service.edit_any');
	if (!canEdit) {
		throw error(403, { code: 'not_owner', message: 'You can only edit your own service listings' });
	}

	const [countriesRes, categoriesRes] = await Promise.allSettled([
		apiList<GeoCountry>(ctx, '/geo/countries'),
		apiGet<{ value: string; label: string }[]>(ctx, '/services/categories')
	]);

	const countries = countriesRes.status === 'fulfilled' ? countriesRes.value.items : [];
	const serviceCategories = (
		categoriesRes.status === 'fulfilled' ? categoriesRes.value ?? [] : []
	).map((o) => o.value as ServiceCategory);

	return { service, countries, serviceCategories, user: event.locals.user, features };
};
