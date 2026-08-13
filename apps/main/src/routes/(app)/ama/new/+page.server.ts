// Ref: japaconnect-engineering/sections/88-ask-an-expat.md
import type { PageServerLoad } from './$types';
import { redirect } from '@sveltejs/kit';
import { apiContext, apiList } from '$lib/server/api';
import type { Space } from '$lib/types';

export const load: PageServerLoad = async (event) => {
	if (!event.locals.user) {
		throw redirect(307, `/login?redirect=${encodeURIComponent(event.url.pathname)}`);
	}

	// Defense-in-depth: the list page hides the "Start an AMA" button unless
	// the caller holds `forum.ama.create`, but a user without the permission
	// could still navigate directly to /ama/new. Gate the form on the same
	// node (§26 RBAC wiring) so they are redirected instead of filling a
	// form that 403s on submit.
	if (!event.locals.user.permissions?.includes('forum.ama.create')) {
		throw redirect(307, '/ama');
	}

	const ctx = apiContext(event);

	let countries: Space[] = [];
	try {
		const { items } = await apiList<Space>(ctx, '/spaces?level=country');
		countries = items;
	} catch {
		countries = [];
	}

	return { countries };
};
