import { redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';

/**
 * Legacy/shorthand /visa-checklist URL — the tool lives at the canonical
 * /tools/visa-checklist (Section 70). Permanently redirect so external links
 * and the single-tool surface URL shape keep working on the main site.
 */
export const load: PageServerLoad = () => {
	throw redirect(308, '/tools/visa-checklist');
};
