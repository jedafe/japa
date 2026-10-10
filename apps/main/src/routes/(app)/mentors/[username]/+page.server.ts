// Ref: japaconnect-engineering/sections/102-mentorship-matching.md
// Legacy redirect (services/mentorship unification): the mentor profile
// moved to the mentor's shadow service listing at /services/mentor-<username>.
// The listing is resolved server-side first so a username without a
// published shadow listing falls back to the mentorship directory instead
// of a dead 404.
import { error, redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiGet } from '$lib/server/api';
import type { ServiceListing } from '$lib/types';

export const load: PageServerLoad = async (event) => {
	const { features } = await event.parent();
	if (!features['mentorship.enabled']) {
		throw error(404, { code: 'feature_disabled', message: 'Mentorship is not available' });
	}

	const slug = `mentor-${event.params.username}`;
	const ctx = apiContext(event);
	try {
		await apiGet<ServiceListing>(ctx, `/services/${encodeURIComponent(slug)}`);
	} catch {
		throw redirect(308, '/services?kind=mentorship');
	}
	throw redirect(308, `/services/${encodeURIComponent(slug)}`);
};
