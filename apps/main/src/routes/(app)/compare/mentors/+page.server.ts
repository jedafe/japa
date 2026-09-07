import { error, redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';

export const load: PageServerLoad = async (event) => {
	const { features } = await event.parent();
	if (features['mentorship.comparison'] !== true) {
		throw error(404, { code: 'feature_disabled', message: 'Mentorship comparison is not available' });
	}

	const ids = event.url.searchParams.get('ids') ?? '';
	const target = ids
		? `/compare?type=mentors&preselect=${encodeURIComponent(ids)}`
		: '/compare?type=mentors';
	throw redirect(307, target);
};
