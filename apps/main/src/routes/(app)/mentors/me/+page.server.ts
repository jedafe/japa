// Ref: japaconnect-engineering/sections/102-mentorship-matching.md
// Legacy redirect (services/mentorship unification): the "My Mentorship"
// dashboard moved to the /messages/mentorship inbox (sent/received
// mentorship requests as service_bookings rows). The inbox load handles
// the feature-flag + auth gates itself.
import { redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';

export const load: PageServerLoad = async () => {
	throw redirect(308, '/messages/mentorship');
};
