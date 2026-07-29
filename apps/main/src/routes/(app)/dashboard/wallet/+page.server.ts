import { redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';

/**
 * Phase 19 — Wallet lives in settings (`/settings/wallet`), not under
 * /dashboard/wallet as the roadmap originally specified. This static route
 * redirects the roadmap URL to the canonical settings URL so existing links
 * and bookmarks keep working. The static path takes precedence over the
 * `/dashboard/[view]` dynamic route, which would otherwise 404 on `wallet`.
 */
export const load: PageServerLoad = async () => {
	throw redirect(307, '/settings/wallet');
};
