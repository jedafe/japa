import { redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';

/**
 * /ai/chat — legacy JapaBot route. The chat surface merged into /ai
 * (ADR-130); this redirect keeps old links, the nav FAB, and bookmarks
 * working. Permanent (308) so crawlers update.
 */
export const load: PageServerLoad = async () => {
	throw redirect(308, '/ai');
};
