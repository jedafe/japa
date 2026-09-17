import { loadPublishedPage } from '$lib/server/published-page';
import type { PageServerLoad } from './$types';

/**
 * Root-level published page route: /{slug} (e.g. /about-us, /terms).
 * Static routes always win over this param, so existing top-level routes
 * (/sources, /discover, /me, …) are unaffected; any other single-segment
 * path resolves against published Page Builder pages and 404s otherwise.
 *
 * Ref: japaconnect-engineering/sections/27-page-builder.md
 */
export const load: PageServerLoad = (event) => loadPublishedPage(event);
