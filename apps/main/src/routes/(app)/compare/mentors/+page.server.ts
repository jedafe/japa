import { redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';

export const load: PageServerLoad = async (event) => {
	const ids = event.url.searchParams.get('ids') ?? '';
	const target = ids
		? `/compare?type=mentors&preselect=${encodeURIComponent(ids)}`
		: '/compare?type=mentors';
	throw redirect(307, target);
};
