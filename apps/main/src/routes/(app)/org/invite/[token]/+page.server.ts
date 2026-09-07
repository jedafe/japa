import { redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';

/**
 * Organization invitation accept page at /org/invite/[token] — auth-gated.
 * The token arrives via the invitation email link (which points here, not at
 * the POST-only API route). Guests are redirected to login and return here
 * after signing in — the invited email must match the signed-in account.
 *
 * Ref: japaconnect-engineering/sections/07-organization-profiles-claim-ownership.md
 */
export const load: PageServerLoad = async (event) => {
	if (!event.locals.user) {
		throw redirect(307, `/login?redirect=${encodeURIComponent(event.url.pathname + event.url.search)}`);
	}

	return { token: event.params.token };
};
