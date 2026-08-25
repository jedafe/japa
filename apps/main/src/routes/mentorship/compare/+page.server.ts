// Ref: japaconnect-engineering/adr/ADR-110-mentorship-matching.md
// Legacy redirect: /mentorship/compare → /compare?type=mentors&preselect=<ids> (301 permanent).
// Preserves the ?ids= query parameter so old deep links keep working.
import { redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';

export const load: PageServerLoad = async (event) => {
	const ids = event.url.searchParams.get('ids') ?? '';
	const target = ids ? `/compare?type=mentors&preselect=${ids}` : '/compare?type=mentors';
	throw redirect(301, target);
};
