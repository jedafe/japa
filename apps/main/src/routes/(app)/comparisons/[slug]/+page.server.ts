import { redirect, error } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';

/**
 * /comparisons/[slug] detail is served by the standalone apps/jce surface.
 * Redirect to the matching JCE public comparison detail page.
 */
export const load: PageServerLoad = async (event) => {
	const { features, siteSettings } = await event.parent();

	if (!features?.['feature.jce']) {
		throw error(404, { code: 'not_found', message: 'Not found' });
	}

	const jceOrigin = String(siteSettings?.['jce.origin'] ?? 'http://localhost:3006').replace(/\/$/, '');
	if (!jceOrigin) {
		throw error(404, { code: 'not_found', message: 'Not found' });
	}

	const search = event.url.search;
	throw redirect(307, `${jceOrigin}/comparisons/${event.params.slug}${search}`);
};
