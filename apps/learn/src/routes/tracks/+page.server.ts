import { redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';

/**
 * /tracks — alias for the Explore catalog (canonical at /).
 * The catalog lives at the root route; /tracks redirects there so the
 * bare path is never a dead 404 and a single canonical URL is preserved
 * (SEO). Query params (q, level) are forwarded.
 *
 * Ref: ADR-060 (Explore is the center column), Section 36 (GET /learn/tracks).
 */
export const load: PageServerLoad = async ({ url }) => {
	const qs = url.searchParams.toString();
	throw redirect(307, qs ? `/?${qs}` : '/');
};
