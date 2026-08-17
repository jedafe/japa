import type { PageServerLoad } from './$types';
import { buildMeta } from '$lib/seo/meta';
import { env } from '$env/dynamic/public';

/**
 * Access-denied page — the destination for 403s on gated audiences (ADR-060).
 * Public (no session needed); the page offers sign-in and a link to the public
 * API reference.
 */
export const load: PageServerLoad = async (event) => {
	const meta = buildMeta({
		title: 'Access denied · JapaConnect Docs',
		description: 'You do not have access to this documentation section.',
		path: '/docs/access-denied'
	});
	// Login lives on the Main app — point the CTA at the main origin so it
	// does not 404 on the docs origin.
	const mainOrigin = (env.PUBLIC_MAIN_ORIGIN ?? 'http://localhost:3000').replace(/\/$/, '');
	return { meta, loginUrl: `${mainOrigin}/login?redirect=${encodeURIComponent(event.url.href)}` };
};
