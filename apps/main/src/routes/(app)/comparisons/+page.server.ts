import { redirect, error } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';

/**
 * /comparisons public directory is served by the standalone apps/jce surface.
 * When feature.jce is enabled, redirect to the JCE origin so the browser loads
 * the JCE public directory. When disabled, this route does not exist (404),
 * matching the pre-JCE main app surface.
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
	throw redirect(307, `${jceOrigin}/comparisons${search}`);
};
