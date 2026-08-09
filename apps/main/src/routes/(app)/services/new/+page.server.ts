// Ref: japaconnect-engineering/sections/98-service-directory.md
import { redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiList } from '$lib/server/api';
import type { Space } from '$lib/types';

/**
 * Create-service form at /services/new — auth-gated. Guests are
 * redirected to login with a return redirect to this page. The form
 * itself is submitted client-side via the browser API client
 * (POST /services), which is org-linked on the backend.
 */
export const load: PageServerLoad = async (event) => {
	if (!event.locals.user) {
		throw redirect(307, `/login?redirect=${encodeURIComponent(event.url.pathname + event.url.search)}`);
	}
	const ctx = apiContext(event);

	let countries: Space[] = [];
	try {
		const { items } = await apiList<Space>(ctx, '/spaces?level=country');
		countries = items;
	} catch {
		countries = [];
	}

	return { authed: true, countries };
};
