// Ref: japaconnect-engineering/sections/98-service-directory.md
import { error, redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiGet, apiList, ApiError } from '$lib/server/api';
import type { ServiceListing, GeoCountry, ServiceCategoryInfo } from '$lib/types';

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
		service = await apiGet<ServiceListing>(ctx, `/services/${encodeURIComponent(slug)}?raw=1`);
	} catch (err) {
		if (err instanceof ApiError && err.status === 404) {
			throw error(404, { code: 'not_found', message: 'Service not found' });
		}
		throw error(500, { code: 'load_failed', message: 'Failed to load service' });
	}

	// 000824: mentorship shadow listings are managed via PATCH
	// /me/mentor-profile (/settings/mentorship) — the generic edit form
	// would be rejected with mentorship_listing_managed anyway, so the
	// route 404s like the backend's edit guard.
	if (service.listing_kind === 'mentorship') {
		throw error(404, { code: 'not_found', message: 'Service not found' });
	}

	// Mirrors EditOwn (§98): author or service.edit_any may edit.
	// (The 'package' listing kind was removed from the feature in
	// 2026-10 — its org-membership and service.package.create
	// gates went with it.)
	const isEditAny = !!event.locals.user.permissions?.includes('service.edit_any');
	const canEdit = isEditAny || service.author_id === event.locals.user.id;
	if (!canEdit) {
		throw error(403, { code: 'not_owner', message: 'You can only edit your own service listings' });
	}

	const [countriesRes, categoriesRes] = await Promise.allSettled([
		apiList<GeoCountry>(ctx, '/geo/countries'),
		apiGet<ServiceCategoryInfo[]>(ctx, '/services/categories')
	]);

	const countries = countriesRes.status === 'fulfilled' ? countriesRes.value.items : [];
	// §114: keep the taxonomy label — the category dropdown renders the
	// API label (dynamic slugs have no i18n cat_* key to re-derive it).
	const serviceCategories: ServiceCategoryInfo[] =
		categoriesRes.status === 'fulfilled' ? (categoriesRes.value ?? []) : [];

	return {
		service,
		countries,
		serviceCategories,
		user: event.locals.user,
		features
	};
};
