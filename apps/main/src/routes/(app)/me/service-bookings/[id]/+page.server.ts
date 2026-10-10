// Ref: japaconnect-engineering/sections/111-service-packages-bookings.md
// Legacy redirect (services/mentorship unification): booking detail moved
// into the /messages/bookings inbox as a ?b=<id> deep link that opens the
// detail pane.
import { redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';

export const load: PageServerLoad = async (event) => {
	throw redirect(308, `/messages/bookings?b=${encodeURIComponent(event.params.id)}`);
};
