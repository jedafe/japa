import { error } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiGet, ApiError } from '$lib/server/api';
import type { PageBuilderPage } from '$lib/types';

/**
 * Page Builder landing page at /pages/{slug} — public, SSR. Renders the
 * published page via the shared LandingPage component (full-width, no app
 * container constraint).
 *
 * Ref: japaconnect-engineering/sections/27-page-builder.md
 */
export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);
	const slug = event.params.slug;
	try {
		const page = await apiGet<PageBuilderPage>(ctx, `/pages/${slug}`);
		return { page };
	} catch (err) {
		if (err instanceof ApiError && err.status === 404) {
			throw error(404, { code: 'not_found', message: 'Page not found' });
		}
		throw err;
	}
};
