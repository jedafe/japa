import type { PageServerLoad } from './$types';
import { buildMeta } from '$lib/seo/meta';

/**
 * Access-denied page — the destination for 403s on gated audiences (ADR-060).
 * Public (no session needed); the page offers sign-in and a link to the public
 * API reference.
 */
export const load: PageServerLoad = async () => {
	const meta = buildMeta({
		title: 'Access denied · JapaConnect Docs',
		description: 'You do not have access to this documentation section.',
		path: '/docs/access-denied'
	});
	return { meta };
};
