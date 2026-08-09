// Ref: japaconnect-engineering/sections/98-service-directory.md
import { error } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiGet, ApiError } from '$lib/server/api';
import type { ServiceListing } from '$lib/types';

/**
 * Service detail at /services/[slug] — SSR detail. Fetches
 * GET /services/{slug} which returns the listing plus reviews (§29).
 * On 404, throws a SvelteKit 404; other errors degrade to
 * loadError: true so the page can render a fallback.
 */
export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);
	const slug = event.params.slug;
	const authed = !!event.locals.user;

	try {
		const service = await apiGet<ServiceListing>(ctx, `/services/${slug}`);
		return { service, authed, loadError: false };
	} catch (err) {
		if (err instanceof ApiError && err.status === 404) {
			throw error(404, { code: 'not_found', message: 'Service not found' });
		}
		return { service: null, authed, loadError: true };
	}
};
