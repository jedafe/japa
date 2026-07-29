import { redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';

/**
 * Phase 19 — Affiliate lives in settings (`/settings/affiliate`), not under
 * /dashboard/affiliate as the roadmap originally specified. This static route
 * redirects the roadmap URL to the canonical settings URL so existing links
 * and bookmarks keep working. The static path takes precedence over the
 * `/dashboard/[view]` dynamic route, which would otherwise 404 on `affiliate`.
 */
export const load: PageServerLoad = async () => {
	throw redirect(307, '/settings/affiliate');
};
