// Ref: japaconnect-engineering/sections/111-service-packages-bookings.md
// Legacy redirect (services/mentorship unification): the traveler bookings
// inbox moved to /messages/bookings. The query string rides along so
// ?status=/?intent=/?page= deep links land on the same filtered inbox view;
// the inbox load handles the feature-flag + auth gates itself.
import { redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';

export const load: PageServerLoad = async (event) => {
	throw redirect(308, `/messages/bookings${event.url.search}`);
};
