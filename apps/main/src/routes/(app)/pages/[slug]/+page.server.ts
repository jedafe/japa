import { redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';

/**
 * Legacy /pages/{slug} path — published pages render at the canonical
 * root-level URL (/{slug}); this route permanently redirects so existing
 * links keep working.
 *
 * Ref: japaconnect-engineering/sections/27-page-builder.md
 */
export const load: PageServerLoad = (event) => {
	throw redirect(308, `/${event.params.slug}`);
};
